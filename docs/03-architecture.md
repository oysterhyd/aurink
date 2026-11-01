# 03 · 系统架构

## 1. 总体分层

Aurink 是单进程原生桌面应用（Go + MyGo Native UI）。模块间通信全部为**进程内 Go 调用**（接口 + 不可变数据 + 主线程派发），不使用 HTTP/IPC。所谓"前端/后端"指同一进程中的两层：

```
┌───────────────────────────────────────────────────────────────────────────┐
│ main.go  →  internal/app   组合根（Composition Root）：创建服务、注入依赖、    │
│                            窗口/菜单/生命周期、把 contract 实现交给 views      │
├───────────────────────────────┬───────────────────────────────────────────┤
│ 前端 internal/views/**        │ 后端 internal/core/**                       │
│  (import mygo/ui)             │  (禁止 import mygo、mygo/ui；纯 Go)          │
│  liveedit  编辑引擎            │  text       不可变文本快照                    │
│  source    源码模式            │  markdown   解析适配 + 扩展 + BlockID         │
│  shell     窗口外壳/侧栏/标签   │  projection 显示投影（纯函数）                │
│  palette   命令面板/快速打开     │  document   文档模型/事务/历史               │
│  panels    设置/导出/欢迎       │  editing    编辑意图/命令/输入规则            │
│  kit       设计系统组件/滑动药丸 │  fsstore    文件读写（原子、编码、换行）       │
│  theme     令牌                │  docsession 打开文档会话/自动保存/恢复/冲突     │
│  motion    动效令牌             │  workspace  工作区/文件树/监听/资源导入        │
│  effects   材质/极光/低功耗切换 │  search     查找/全文搜索/模糊匹配            │
│                               │  export     HTML/PDF 导出编排                  │
│                               │  render     数学/Mermaid 渲染服务              │
│                               │  settings   设置存储                         │
│                               │  commands   命令注册表/快捷键                  │
├───────────────────────────────┴───────────────────────────────────────────┤
│ internal/contract   ★契约：类型 + 接口（只依赖标准库）                         │
│ internal/adapters   平台适配：mygo 文件监听、WebView 宿主、richtext 粘贴转换     │
│ internal/mock       契约的内存实现（测试 / 演示用）                             │
├───────────────────────────────────────────────────────────────────────────┤
│ MyGo v0.7.3（ui、ui/editor、ui/markdown/parser、ui/richtext、plugins/glass…）  │
└───────────────────────────────────────────────────────────────────────────┘
```

### 1.1 依赖规则（由 `tests/arch` 自动检查，违反即 CI 失败）

| 包 | 允许依赖 | 禁止 |
|---|---|---|
| `internal/contract` | 标准库 | 项目内任何包、mygo 任何包 |
| `internal/core/**` | contract、标准库、`github.com/egoist/mygo/ui/markdown/parser`（不依赖 ui）、`golang.org/x/text`、经批准的纯 Go 库 | `github.com/egoist/mygo`、`.../mygo/ui`（parser 除外）、views、app、adapters |
| `internal/adapters/**` | contract、mygo 任意包 | core、views、app |
| `internal/views/**` | contract、`internal/views/**`、mygo/ui 及子包、plugins/glass | **core、adapters、app** |
| `internal/mock` | contract、标准库、parser | core、views、adapters |
| `internal/app` | 全部 | — |

要点：views 只面向 contract 编程，因此在 core 未完成时可以用 mock 开发；core 不依赖 UI，因此可以纯单元测试、可以在任意 goroutine 使用不可变快照。

## 2. 线程与并发模型（MyGo 约束）

1. **主线程（UI 线程）**：视图构建、输入回调、`DocumentModel` 的全部写操作（`Apply/Undo/Redo/SetSelection`）、服务回调。
2. **不可变快照**：`contract.Text`、`contract.AST`（某版本）不可变，可在任何 goroutine 读取（保存、搜索、导出、渲染）。
3. **阻塞 I/O**：`FileStore`、`WorkspaceSearch`、`Renderer`、`Exporter`、`Assets` 的方法是阻塞的，**禁止在主线程调用**；调用方在 goroutine 中执行，结果经 `contract.Dispatcher.Post` 回到主线程。
4. **异步服务**（`Sessions`、`Workspace`、`SettingsStore`）：方法在主线程调用、立即返回，内部自行起 goroutine；所有回调与 `Subscribe` 事件**保证在主线程**触发（通过注入的 Dispatcher）。
5. `Dispatcher` 在 app 中实现为 `Window.Update(fn)`；测试中实现为同步/队列执行器（`mock.Dispatcher`）。
6. 禁止：在视图函数里做 I/O 或启动无条件副作用；保存 `ui.Element` 到状态或 goroutine（`mygo vet` 检查）；在主线程等待需要主线程的工作。

## 3. 数据流（单向）

```
 用户输入（键盘/IME/鼠标/粘贴/拖放/菜单/命令面板）
        │
        ▼
 views/liveedit  ──映射──▶  contract.Intent / EditCommand
        │                         │
        │                         ▼
        │                core/editing（纯函数：上下文 → Transaction）
        │                         │
        ▼                         ▼
 contract.DocumentModel.Apply(Transaction)            ← core/document
        │  1. 校验 → 应用字节替换 → 新 Text 版本
        │  2. 记录历史（逆操作 + 选区前后，合并策略）
        │  3. markdown.Update(changes) 增量解析 → 新 AST 版本，计算 DirtyBlocks
        │  4. 发出 DocEvent{Changes, Dirty, Origin}
        ▼
 订阅者（主线程）：
   liveedit：作废脏块的投影/排版缓存 → 下一帧只重排脏块
   docsession：标记已编辑 → 去抖自动保存（goroutine 写快照）
   shell：大纲/字数/标题刷新（基于 AST，O(块数)）
   search：文档内查找结果失效重算（goroutine）
```

外部来源（源码模式编辑、外部文件重载、撤销）同样走 `Apply/Replace`，只是 `Origin` 不同。**不存在第二条写路径。**

## 4. 文档核心

### 4.1 文本存储 `core/text`
- `Text` = 不可变 `string` + 惰性构建的行起点索引 + `Version`。编辑生成新 `string`（复制约 0.1 ms/MB），旧版本自然成为快照。
- 理由：实测解析器每次编辑本身 O(n)（6 ms/MB），rope 无法改善瓶颈；`string` 方案最简单、零锁、天然可跨 goroutine。接口抽象保留未来替换为 rope 的余地。
- **保留原始字节**（不归一化换行）；BOM 在读入时剥离、记录于 `DocMeta`。

### 4.2 解析适配 `core/markdown`
包装 `ui/markdown/parser`，输出 contract.AST：
1. **Front Matter 预处理**：文档以 `---\n` 开头且存在闭合 `---`/`...` 行 → 生成 `BlockFrontMatter`，其后正文交给 parser，偏移整体平移。编辑触及首块时重新检测。
2. **块信息补全**：`Content`（去标记后的内容区间）、`Markers`（`## `、围栏行、`$$` 行）、`Prefixes`（叶子块内每行的容器前缀 `> `、列表缩进——parser 私有字段不可得，需按容器链重新扫描行首）。不变式：`源码去掉 Prefixes 与首行标记 == parser.Lines(i)`（S3 验证）。
3. **内联补全**：`Open/Close` 定界符区间（由父子 Range 差推导）；在 Text 节点内二次扫描生成 `InlineEscape`、`InlineEntity`；扩展语法（`==`、`^`、`~`、`:emoji:`、`[[ ]]`）在 Text 节点上做后处理，按设置开关。`[TOC]` 独占段落 → `BlockTOC`。
4. **BlockID 稳定性**：增量解析后，未与任何 Change 相交的块通过偏移映射继承旧 ID；与 Change 相交但"种类相同且映射后起点相同"的块也继承（打字时段落 ID 不变）；其余分配新 ID。ID 用于排版缓存键和视图 `Key`。
5. **派生数据**：大纲（标题文本、级别、slug——与 GitHub 规则一致，重名加 `-1`）、脚注表、链接定义表。
6. 增量策略：`parser.Edit` 逐个应用 Change（从后往前）；`Inlines` 惰性解析，只在投影/导出需要时触发。

### 4.3 投影 `core/projection`（纯函数，编辑器正确性的核心）
输入：`(AST, Text, 叶子块索引, Reveal)`；输出：`BlockProjection{Segments, Containers}`。

| 片段类型 | 含义 | 示例 |
|---|---|---|
| `SegText` | 原样显示的源码 | 正文文字 |
| `SegHidden` | 不显示的源码 | 未激活的 `**`、`# `、`](url)`、容器前缀 |
| `SegMarker` | 显示为淡色标记的源码（已显隐） | 激活时的 `**` |
| `SegReplaced` | 源码显示为替代文本 | `&amp;`→`&`，`\*`→`*`，软换行 `\n> `→`\n`，`:smile:`→😄 |
| `SegObject` | 一段源码显示为一个内联对象 U+FFFC | 图片、行内公式、复选框、脚注上标 |

`PosMapper` 提供源码偏移 ↔ 显示 UTF-16 索引的双向映射（带 affinity），规则：
- 隐藏片段是原子的：光标不能停在隐藏片段内部；映射到其左/右边界由 affinity 决定。
- 光标位置**永远以源码字节偏移存储**；显示索引只在排版/IME 边界临时计算。
- 光标移入某节点 → Reveal 变化 → 重新投影该块（其余块不变）→ 显示文本变化但源码光标不变，因此光标"不跳"。

### 4.4 文档模型 `core/document`
- `Apply(tx)`：校验 Edits 有序、不重叠、在范围内 → 应用 → 生成 Changes → 选区映射（插入点后的光标后移；范围末端不扩张）→ 历史 → AST 更新 → 事件。
- 历史条目：`{逆 Edits, Changes, 选区前, 选区后, Origin, Coalesce, 时间, Label}`。
  - 合并：相同 `Coalesce` 键、间隔 < 1 s、编辑区间相邻、中间无光标跳动 → 合并为一步。
  - 换行、格式命令、粘贴、拖放、IME 提交各自独立成步；输入规则（自动格式化）作为紧随其后的独立一步——撤销先撤自动格式化、保留原始输入。
  - 上限：1000 步或逆文本累计 64 MB，超出丢弃最旧。
- `Replace(text, origin)`：整篇替换（外部重载/恢复），作为一步历史（可撤销回自己的版本）。

### 4.5 编辑逻辑 `core/editing`（纯函数）
- `HandleIntent(ctx, intent)`：根据光标所在 AST 路径决定行为，例如：
  - Enter：列表项内 → 续写同级标记（有序递增，任务项为 `[ ]`）；空列表项 → 退出列表；代码块内 → 插入换行并保持缩进；标题末尾 → 新段落；表格单元格 → 下一行同列。
  - Backspace：列表项内容起点 → 移除标记（降为段落或上一级）；引用起点 → 去一层 `>`；标题起点 → 去 `#`；隐藏标记边界 → 跳过标记删除可见字符。
  - 输入字符：闭合定界符触发格式化；自动配对（`(`、`[`、`` ` ``、`*` 选区包裹）。
  - 所有插入的换行使用 `DocMeta.EOL` 主导风格；列表续写使用该列表已有的符号与缩进。
- `Run(ctx, cmd)`：格式切换（选区全部已具备则移除，否则添加；光标无选区时选中词或插入成对标记）、块类型切换、表格操作、插入组件。
- HTML/RTF 粘贴转换在 `adapters/richpaste`（依赖 `ui/richtext.ParseHTML/ParseRTF(...).Markdown()`），由 app 注入为 `contract.PasteConverter`。

## 5. 平台服务

### 5.1 `fsstore`
- Read：大小上限（默认 64 MB）、二进制检测（NUL 字节/无效 UTF-8 比例）、BOM（UTF-8/UTF-16LE/BE）、换行风格统计（LF/CRLF/混合）。
- Write：同目录临时文件 → 写入 → `fsync` → 替换（Windows `ReplaceFileW`，失败回退 `MoveFileEx`；Unix `rename` 并保留权限）；硬链接/不可创建文件目录 → 原地写；写前比对 `expect` 戳，不符返回 `CodeConflict`。参照 MyGo `document_file.go` 算法。

### 5.2 `docsession`
- 每个打开的文件一个 Session（按规范化路径去重），持有 `DocumentModel`、绑定路径、已保存版本号、文件戳、状态。
- 自动保存：编辑后去抖 `AutosaveDelay`（默认 1.5 s）、窗口失焦、切换标签、关闭时；无标题文档不自动保存但写恢复副本。
- 恢复副本：`PathUserData/Recovery/<DocID>.md + .json`（路径、戳、版本），保存/关闭后删除；启动时扫描并恢复为"已编辑"会话。
- 外部变更：`WatchFiles` 监听所在目录 + 窗口获焦时 `Stat` 复查；未编辑 → 自动 `Replace` 重载；已编辑 → `Conflict` 状态；被删除 → `Deleted` 状态（保存会重建）；同目录重命名 → 跟随。

### 5.3 `workspace`
- 根目录、懒加载目录列表（缓存 + 排序：文件夹优先、自然排序）、过滤（默认显示 Markdown 与图片，可切显示全部）、忽略（`.git`、`node_modules`、隐藏文件可配置）。
- 监听：根目录递归 `WatchFiles`；超出 `MaxEntries` 时降级为"仅监听已展开目录"并提示。
- 操作：新建、重命名、移动、`TrashItem`；全部异步、事件回主线程。
- `Assets`：图片导入策略（`./assets`、`./${filename}.assets`、自定义），文件名去重，返回相对路径（空格编码为 `%20`）。

### 5.4 `search`
- `Finder`：字面量（`strings.Index` 系列）/ 正则（`regexp`，RE2 线性时间）/ 大小写 / 全词（Unicode 词边界）；1 MB 文档 < 5 ms。
- `WorkspaceSearch`：并发遍历（worker 池 = CPU 数），按文件分批回调，可取消；跳过二进制与超大文件。
- `Fuzzy`：fzf 风格打分（连续匹配、词首、路径分隔符加权），中文按字符匹配；P2 可加拼音首字母。

### 5.5 `render`（P1）
- 宿主：`adapters/webviewhost` 在主窗口中创建一个**不放入视图**的 WebView（`ThrottlingOff`），加载内嵌的本地 HTML（MathJax + Mermaid，离线），懒启动。
- 接口：`Render(ctx, req)` 阻塞 → 内部串行队列 → `Page.Eval` → SVG/PNG；结果按 `(kind, source, dark, size)` 哈希缓存（内存 LRU 512 项 + 磁盘缓存）。
- 超时 2 s、失败返回 `CodeRender` 带错误信息；不可用返回 `CodeUnavailable`。

### 5.6 `export`
- HTML：AST 遍历生成语义 HTML（与 parser.HTML() 对齐并加扩展语法、代码高亮 class、标题锚点、脚注、TOC），内联主题 CSS；图片可选 base64 内嵌；数学/Mermaid 通过 Renderer 内嵌 SVG/PNG。
- PDF：HTML → `contract.Printer`（app 用 WebView `PrintToPDF` 实现）。

### 5.7 `settings` / `commands`
- 设置 JSON 存于 `PathUserData/settings.json`，带 `Version` 迁移；写入原子化；`Subscribe` 主线程通知。
- 命令注册表：所有模块启动时注册命令（ID 命名空间见 04 文档）；快捷键默认表按平台生成（macOS ⌘ / 其他 Ctrl），用户覆盖；注册时检测冲突；菜单、命令面板、快捷键三者共用同一注册表。

## 6. 编辑引擎 `views/liveedit`

### 6.1 组件结构
```
EditorView(c, ctrl)                       // 外层：滚动容器 + 编辑表面 + 覆盖层
 └─ ui.Scroll (TrackScroll, ScrollbarInsets)    [S1 决定：内置滚动 or 自滚动]
     └─ Surface 元素（高度 = 估算总高）
          .HandleTextInput(imeClient)      // 系统 IME
          .HandleInput(onInput)            // 键盘/指针/滚动 → Intent
          .Draw(paint)                     // 只绘制可见块：文本、光标、选区、装饰
     └─ 可见块的交互子元素（Absolute 定位）：代码块复制按钮/语言标签、表格手柄、
        图片悬停控件、数学源码与预览面板、复选框命中区
 └─ Overlay：斜杠菜单、格式条、链接与脚注弹层、图片灯箱、活动块页边标签、结构导轨、查找高亮浮层
```

### 6.2 每帧流水线
1. 读取 `DocumentModel.Version()`，若变化则处理积压的 DocEvent（作废脏块缓存）。
2. 由滚动偏移 + 视口高度，经高度索引（Fenwick 树）求可见叶子块区间 `[first, last]`，上下各多取 1 屏作为预排区。
3. 对每个可见块：取投影（缓存键 `BlockID + 块修订 + Reveal 签名`）→ 样式解析（主题令牌 → `ui.Span`）→ 排版（缓存键 `投影修订 + 宽度 + 主题修订`）→ 回写实测高度到 Fenwick 树。
4. 绘制：块背景/装饰（悬挂引号、列表符号、代码块底、表格容器）→ `p.TextLayout` → 内联对象（图片/公式 SVG）→ 选区矩形 → 光标（可选平滑移动）。
5. 滚动锚定：以"视口顶部块 + 块内偏移"为锚；上方块实测高度变化时补偿滚动偏移，避免跳动。

### 6.3 输入映射
| 输入 | 处理 |
|---|---|
| 可打印文本（`InputText`） | `Intent{InsertText}` → editing |
| Enter / Shift+Enter / Backspace / Delete / Tab | 对应 Intent（粒度：字符/词/行，按平台修饰键） |
| 方向键/Home/End/PageUp/Down | 纯视图逻辑：在显示布局中移动，结果转回源码偏移；上下移动保持目标 x（sticky x） |
| 鼠标单击/双击/三击/拖选/Shift+单击 | `IndexAt` → 源码偏移；双击选词、三击选块 |
| 复制/剪切 | 选区源码 → Markdown；投影可见文本 → 纯文本；Exporter → HTML |
| 粘贴 | 读取剪贴板格式 → PastePayload → editing（HTML 先经 PasteConverter） |
| 拖入文件 | 图片 → `Assets.Import`（goroutine）→ `EditCommand{insert.images}` |
| IME | 见 6.4 |
| 格式/块命令 | `CommandRegistry.Run` → editing.Run → Apply |

### 6.4 IME 客户端
- 文档 = 活动块显示文本（含预编辑虚拟片段）；`Selection()` 返回活动块内的光标/选区 UTF-16 坐标。
- `SetMarkedText`：只更新视图层预编辑状态 → 该块重新投影排版（预编辑带下划线），**不写入源码**。
- `ReplaceText`：清除预编辑，生成 `Transaction{Origin: OriginIME}`；替换范围经 PosMapper 换算为源码字节区间。
- `BoundsForRange`/`IndexForPoint`：基于活动块 `TextLayout`，坐标转换为 Surface 元素坐标。
- 跨块选区期间：报告主光标所在块，IME 输入先删除选区（一个事务）再组字。

### 6.5 块组件注册
`BlockWidget` 注册表（键：块种类 / 代码语言），每个组件提供：测量高度、绘制、交互子元素、激活/未激活两种形态。内置：代码块、表格、图片段落、数学块、Mermaid、Front Matter 编辑式页眉、TOC、分隔线、脚注定义、HTML 块。新增语法 = 解析扩展 + 投影规则 + 组件注册，三处均为注册表，不改引擎核心。

### 6.6 视图层内部契约 `views/editorapi`
供 shell/palette 等其他前端模块调用编辑器（由 `views/liveedit` 实现）：

```go
type EditorController interface {
    Session() contract.Session
    Focus()
    RevealOffset(off int, align Align)               // 滚动到源码偏移（大纲跳转/搜索结果）
    SetFindHighlights(ranges []contract.Range, current int)
    VisibleHeading() (contract.BlockID, bool)       // 大纲 scroll spy
    SelectionAnchorRect() (x, y, w, h float32, ok bool) // 浮层定位
    Mode() EditorMode; SetMode(EditorMode)          // Live / Source / Read
    FocusMode() bool; SetFocusMode(bool)            // 视图菜单显示当前状态
    Typewriter() bool; SetTypewriter(bool)
    ScrollFraction() float32                        // 胶囊进度环、阅读进度条
    ScrollOffset() float32                          // 滚动联动标题
    CaretBlock() (index, total int)                 // 胶囊「第 N 块」
}
```

以上为示意，权威定义见 `editorapi.go`。

### 6.7 视觉职责划分
`liveedit` 负责编辑区内的一切（块样式、页边标签、结构导轨、斜杠菜单、格式条、弹层、灯箱、模式下的正文变化）；`shell`、`palette`、`panels` 负责编辑区外的一切（窗口材质、标题栏、侧栏、胶囊、横幅、通知、命令面板、查找栏、设置）。编辑器的浮层一律使用 `views/kit` 与 `views/effects`，不自行实现玻璃、选中药丸或动效曲线。

## 7. 应用外壳与状态管理

即时模式 UI 中不引入全局 Store 框架，采用"**领域状态结构 + 控制器方法**"：

```go
type AppState struct {
    Workspace  *WorkspaceUIState   // 根、展开集合、选中、重命名中
    Tabs       *TabsState          // 有序 DocID、活动标签、MRU
    Editors    map[DocID]*editorState
    Chrome     ChromeState         // 侧栏开合/宽度/面板、模式、浮层（命令面板/设置）及其触发源
    Notices    *NoticeStack        // 通知栈（最多 4 条）、进度
    Settings   contract.Settings   // 订阅快照
}
```

- 状态只在主线程读写；视图读状态构建 UI，交互调用控制器方法（如 `tabs.Close(id)`），控制器调用服务；服务事件回主线程更新状态并 `Invalidate`。
- 不在状态中保存 `ui.Element`；需要持久焦点身份时用 `ui.Handle`。
- 低功耗路径 = `!p.Accelerated()` 或 `ReduceTransparency`，由 `views/effects` 统一判断；其他组件不直接判断。
- 共享元素用 `ElementTransition.Match`（omnibox→面板、面板行→标签、图片→灯箱），Match 键类型定义在 `views/motion`。
- 每个视图组件是 `struct{ deps…; View(c *ui.Context) }`，依赖通过构造函数注入窄接口（contract 接口或 views 内部接口），便于 `ui.NewTester` + mock 测试。

## 8. 扩展机制

| 扩展点 | 位置 | 形式 |
|---|---|---|
| Markdown 语法扩展 | core/markdown | `InlineExtension` / `BlockExtension` 注册表（编译期），设置开关 |
| 投影规则 | core/projection | 按 InlineKind / BlockKind 的规则表 |
| 块组件 | views/liveedit | `BlockWidget` 注册表（按种类 / 代码语言） |
| 渲染器 | core/render | 按 `RenderKind` 注册后端 |
| 命令 | core/commands | 任意模块注册 `contract.Command` |
| 导出格式 | core/export | `Exporter` 按格式注册 |
| 主题 | views/theme | 令牌结构；P2 支持 JSON 主题文件 |
| 插件（P3） | — | 预留：基于上述注册表；Go `plugin` 包不支持 Windows，未来考虑 WASM 或脚本运行时 |

## 9. 错误处理与日志
- 跨模块一律返回 `*contract.Error`（含 `Code`、`Op`、`Path`、`Detail`、底层 `Err`），`contract.CodeOf(err)` 判别；不得跨边界 panic。
- goroutine 入口 `recover` → 转为 `CodeInternal` 并记录堆栈。
- UI 呈现策略（shell 统一实现）：

| Code | 呈现 |
|---|---|
| `conflict` | 编辑区顶部冲突横幅 |
| `not_found`、`permission`、`read_only` | 横幅（文档级）或通知（操作级） |
| `too_large` | 对话框：以大文件模式打开 / 取消 |
| `binary`、`encoding` | 对话框说明，可只读打开 |
| `unavailable`（渲染服务） | 静默降级为源码 + 首次一次性提示 |
| `render` | 组件内红色细边 + 错误文本 |
| `canceled` | 不提示 |
| `io`、`internal` | 常驻通知（带"复制详情"）+ 日志 |

- 日志：`log/slog`，`component` 属性；文件日志写 `PathUserData/logs/aurink.log`（滚动 5×5 MB）；开发构建输出到控制台。

## 10. 性能预算与度量

| 指标 | 预算 | 度量方式 |
|---|---|---|
| 冷启动到首帧 | < 500 ms | 启动埋点 |
| 打开 1 MB 文档到首屏 | < 300 ms | 埋点（首屏只排可见块） |
| 击键帧 p95（≤200 KB / ≤1 MB） | < 8 ms / < 16 ms | `MYGO_FRAME_STATS`、`BenchmarkTypeFrame` |
| 1 万块文档滚动 | 60 fps，无整屏重排 | 帧统计 + 排版计数断言 |
| 空闲 CPU / 帧 | 0 帧 | Tester `FramePending()==false` |
| 空闲内存（100 KB 文档，无渲染服务） | < 150 MB | `internal/idlemem` 同类测量 |
| 发布体积（不含渲染资源，含打包字体） | < 40 MB（其中字体目标 ≤ 15 MB，S4 实测后由 ADR 确认） | CI 记录 |
| 字体注册（首次需要衬线时） | 不阻塞首帧 | 启动埋点 [待验证 S4] |
| 极光 Shader | 12 s 后 0 帧 | Tester `FramePending()==false` |

测试中按"计数"而不是"时间"断言（排版次数、分配次数），遵循 MyGo 的测试准则。

## 11. 目录结构（最终形态）

```
Aurink_claude/
├─ main.go                         # 仅调用 app.Run()
├─ go.mod / go.sum                 # 模块 aurink
├─ mygo.json                       # 应用配置、文件关联、i18n
├─ internal/
│  ├─ contract/                    # ★契约
│  ├─ core/{text,markdown,projection,document,editing}/
│  ├─ core/{fsstore,docsession,workspace,search,export,render,settings,commands}/
│  ├─ adapters/{watch,webviewhost,richpaste,clipboard}/
│  ├─ views/{liveedit,source,editorapi}/
│  ├─ views/{theme,motion,effects,kit,shell,palette,panels}/
│  ├─ mock/
│  ├─ contracttest/                # 契约一致性测试套件
│  └─ app/                         # 组合根、菜单、窗口
├─ locales/                        # 文案目录与生成的 catalog_gen.go，键名见 05 §3.4
├─ resources/{icons,fonts}/        # fonts 含子集字体与 OFL 许可证
├─ resources/render/               # MathJax/Mermaid 离线包
├─ testdata/corpus/                # 保真语料
├─ tests/{arch,e2e,perf}/
├─ tools/i18ncheck/                # 文案键检查
├─ scripts/gate.ps1, gate.sh       # 门禁
├─ spikes/                         # Spike 原型与报告（独立模块）
├─ reports/parity/                # Native 与基线的对照截图
└─ docs/                           # 规划、决策、design/ 界面基线
```
