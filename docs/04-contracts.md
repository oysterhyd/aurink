# 04 · 统一接口契约（Contract v1.0-rc）

> **权威源码**：`spikes/01-contract-rc/contract/*.go` 与 `spikes/01-contract-rc/editorapi/editorapi.go`（独立模块，已通过 `gofmt`、`go vet`、`go build`，依赖仅 `context errors io iter time`）。
> Phase 0 由 INT 原样复制到 `internal/contract/` 与 `internal/views/editorapi/`（仅改 import 路径），CEO 审核后打标签 `contract-v1.0` 冻结。本文档解释语义与规则；**代码与本文冲突时以代码为准，并提 CR 修正本文**。

## 1. 全局约定（所有模块必须遵守）

| 约定 | 规则 |
|---|---|
| 偏移单位 | 一律 **UTF-8 字节偏移**（与 `ui/markdown/parser`、`ui/editor` 一致）。UTF-16 只出现在 `views/liveedit` 内部与 IME 边界；rune 不出现在契约中（`FuzzyHit.Positions` 除外，明确为 rune 下标） |
| 区间 | `Range{Start, End}` 半开区间；`Transaction.Edits` 基于事务前文本、有序、不重叠 |
| 文本 | 保留原始字节：不归一化换行、不剥尾空白；BOM 不进入 `Text`，由 `DocMeta.BOM` 记录 |
| 不可变性 | `Text`、`AST`（某版本）、`BlockProjection` 为不可变值，可跨 goroutine 读取；`*Block` 只读 |
| 线程 | 见 §3。阻塞接口禁止主线程调用；异步接口的回调与事件保证在主线程 |
| 错误 | 导出方法返回的非 nil 错误必须是 `*contract.Error`；按 `Code` 判断，不比较字符串；不得跨模块 panic |
| 身份 | 文档用 `DocID`（会话级，与路径无关）；块用 `BlockID`（编辑中存活的块保持不变）；命令用 `CommandID` |
| 文案 | 契约中 `Title`/`Category`/`Label` 字段是 **i18n 文案键**（格式见 05 §3.4），由视图本地化；`Notice.Message`/`Detail` 例外，为已本地化文本 |
| 依赖 | `contract` 只依赖标准库；`editorapi` 只依赖 `contract` |

## 2. 契约总览

| 文件 | 内容 | 实现方 | 主要使用方 |
|---|---|---|---|
| `text.go` | `Range` `Version` `TextEdit` `Change` `Text` `EOLStyle` `DocMeta` | BE-1 `core/text` | 全部 |
| `document.go` | `DocID` `Selection` `Origin` `Transaction` `DocEvent` **`DocumentModel`** `DocumentFactory` | BE-1 `core/document` | FE-1、BE-2、INT |
| `ast.go` | `BlockID` `BlockKind` `Block` `Inline` `Extensions` `Heading` `LinkDef` **`AST`** | BE-1 `core/markdown` | FE-1、FE-2（大纲）、BE-2（导出/搜索） |
| `projection.go` | `Reveal` `Segment` `ContainerDecor` `BlockProjection` **`Projector`** | BE-1 `core/projection` | FE-1 |
| `editing.go` | `Intent` `EditContext` `EditCommand` `FormatState` **`Editing`** `PasteConverter` | BE-1 `core/editing`、`adapters/richpaste` | FE-1、INT |
| `commands.go` | 命令 ID 常量、`Chord` `KeyBinding` `Command` **`CommandRegistry`** | BE-2 `core/commands` | 全部（注册）、FE-2（面板/菜单）、INT |
| `services.go` | `FileStore` `Session` `Sessions` `Workspace` `Assets` `Recent` `Finder` `WorkspaceSearch` `Fuzzy` | BE-2 | FE-2、FE-1、INT |
| `render.go` | `Renderer` `Exporter` `Printer` | BE-2；`Printer` 由 INT 用 WebView 实现 | FE-1、FE-2、INT |
| `settings.go` | `Settings` 及子结构、`SettingsStore` | BE-2 `core/settings` | 全部 |
| `runtime.go` | `Dispatcher` `Notifier` `Notice` `NoticeHandle` | INT（Dispatcher）、FE-2（Notifier） | BE-2、全部 |
| `errors.go` | `Code` `Error` `E` `CodeOf` | — | 全部 |
| `editorapi` | `Controller` `Registry` `Mode` | FE-1 | FE-2、INT |

## 3. 线程与调用规则（逐接口）

| 接口 | 调用线程 | 是否阻塞 | 回调/事件线程 |
|---|---|---|---|
| `DocumentModel` 写方法（`Apply` `Replace` `Undo` `Redo` `SetSelection` `SetMeta` `SetReadOnly` `BreakCoalescing`） | 主线程 | 否 | `Subscribe` 回调在主线程，于方法返回前同步触发 |
| `DocumentModel.Text()/AST()` 返回值 | 任意（读快照） | 否 | — |
| `Projector` `Editing` `Finder` `Fuzzy` | 任意（纯函数） | 否（CPU） | — |
| `FileStore` `Assets` `WorkspaceSearch` `Renderer` `Exporter` `Printer` | **非主线程** | 是 | `WorkspaceSearch.onBatch` 在调用者 goroutine |
| `Sessions` `Session` `Workspace` `SettingsStore` `Recent` | 主线程 | 否（内部异步） | `done` 与事件在主线程 |
| `CommandRegistry` | 主线程 | 否 | `Command.Run` 在主线程，不得阻塞 |
| `Dispatcher.Post` | 任意 | 否 | fn 在主线程 |
| `Notifier` `editorapi.*` | 主线程 | 否 | — |

违反规则的代码不予合并。QA 在 `contracttest` 中用"主线程守卫"（mock Dispatcher 记录调用 goroutine）验证异步服务的回调线程。

## 4. 核心语义与不变式

### 4.1 Text / DocumentModel
1. `Version` 严格单调；`Apply` 成功恰好 +1（空 `Edits` 且选区不变时返回原版本、不发事件）。
2. `Apply` 校验失败返回 `CodeInvalid`，文本、选区、历史均不变。只读时返回 `CodeReadOnly`。
3. `Selection == nil` 时按以下规则映射旧选区：插入点上的光标移到插入文本之后；区间起点在插入点时不扩张，终点在插入点时不扩张。
4. 撤销后选区恢复为该步的"选区前"；重做恢复为"选区后"。
5. `Replace` 为一步历史，`Origin` 记入事件。`OriginReload` 的事件中 `Changes` 为单个整篇替换。
6. `DocEvent.Dirty`：包含所有内容变化或新建的块 ID；未变化且仅偏移平移的块不在其中。
7. 不变式（属性测试）：对任意事务序列，`model.Text().String()` 等于对原始字符串依次执行相同字节替换的结果；任意步数撤销后再重做，文本与选区回到原状态。

### 4.2 AST
1. `AST.Version() == Text.Version()`（同一快照）。
2. 块按文档顺序；`Block(0)` 为根；子块位于 `(i, Block(i).Next)`。
3. 叶子块（`Paragraph Heading ThematicBreak Code HTML Math TableCell FrontMatter TOC`）构成 `Leaves()` 序列，即编辑器虚拟化的行；表格单元格作为叶子，表格行/表格本身为容器。
4. 前缀不变式：对段落/标题叶子，`源码[Range] 去掉 Prefixes 与 Markers 后` 与 parser 的内容行一致（S3 验证）。
5. 内联：`Open`/`Close` 为空区间表示无定界符；子节点范围位于 `[Open.End, Close.Start)` 内。
6. `BlockID` 稳定性：未与任何 `Change` 相交的块保持 ID；与 `Change` 相交但"种类相同且映射后起点相同"的块保持 ID；其余为新 ID。ID 永不复用。
7. Front Matter 仅在 `ExtFrontMatter` 开启且位于文档首行时识别。

### 4.3 Projection
1. 片段覆盖叶子块完整 `Range`，按源码顺序首尾相接（**无缝无叠**）。
2. `SegText`/`SegMarker` 的 `Display` 必须等于对应源码子串；`SegHidden` 的 `Display` 为空；`SegObject` 的 `Display` 为 `"￼"`。
3. `ToSource(ToDisplay(s)) == Snap(s)`；`ToDisplay` 单调不减；`Snap` 的结果不落在 `SegHidden`/`SegObject` 内部、不切开字素。
4. 同一 `(AST 版本, leaf, Reveal)` 输入产生相同 `Sig`；任一影响显示的输入变化必须改变 `Sig`。
5. `RevealNone`（阅读模式）下不存在 `SegMarker`。

### 4.4 Editing
1. 纯函数：同输入同输出，不读写全局状态、不做 I/O。
2. 返回的事务必须满足 §4.1 的合法性；插入换行使用 `ctx.Meta.Dominant`。
3. 只做最小改动：不重排未涉及的列表编号、表格对齐、空白（显式命令 `table.format` 除外）。
4. 输入规则产生的格式化事务使用 `OriginInputRule`，且与触发它的键入分属两步历史。
5. `ok=false` 表示"不处理"，视图回退到默认插入/删除行为（由 editing 的基础意图实现，不会出现两套逻辑）。

### 4.5 Sessions / FileStore
1. 打开同一规范化路径返回同一 `Session`（Windows 大小写不敏感比较）。
2. "打开→不编辑→保存"后文件字节完全一致（含 BOM、换行、尾换行）。
3. `Write` 原子：任何时刻崩溃，磁盘上要么是旧文件、要么是新文件。
4. `expect` 戳不符返回 `CodeConflict`，不写入。
5. 自动保存只写不处于冲突/删除/只读/未命名状态的会话；其余写恢复副本。

### 4.6 CommandRegistry
1. 命令 ID 全局唯一；未登记在 `commands.go` 的 ID 只允许以 `x.` 前缀用于实验。
2. 默认快捷键冲突在注册时报告（测试中视为失败）。
3. `Enabled()`/`Checked()` 每帧可能被调用多次，必须 O(1)。

## 5. 事件清单

| 事件 | 发出方 | 触发时机 | 主要订阅方 |
|---|---|---|---|
| `DocEvent{DocTextChanged}` | DocumentModel | 每次 `Apply`/`Undo`/`Redo` | liveedit（缓存失效）、docsession（脏标记/自动保存）、shell（字数/大纲） |
| `DocEvent{DocSelectionChanged}` | DocumentModel | `SetSelection`、撤销恢复选区 | liveedit、格式条（`Formatting`） |
| `DocEvent{DocReplaced}` | DocumentModel | `Replace` | liveedit（整体失效，保持滚动锚） |
| `DocState`（`Session.OnState`） | docsession | 编辑/保存中/已保存/冲突/删除/只读/错误 | shell（标签圆点、状态胶囊、横幅） |
| `SessionsEvent` | docsession | 打开/关闭/恢复 | shell（标签栏）、app（菜单状态） |
| `WorkspaceEvent` | workspace | 根变化、目录加载、增删改名、降级 | shell（文件树）、docsession（内部订阅以跟随重命名） |
| `Settings`（`SettingsStore.Subscribe`） | settings | 任何设置变更 | 全部 |

事件传递规则：同步、按订阅顺序、在主线程；订阅回调中允许调用同一服务的方法，但不得在回调中执行阻塞操作；回调 panic 由服务捕获并记日志，不影响其他订阅者。

## 6. 错误码与呈现

| Code | 典型来源 | 界面呈现（shell 统一实现） |
|---|---|---|
| `not_found` | 打开已删除文件 | 通知；文件树刷新 |
| `permission` | 无写权限 | 文档顶部横幅"只读"，提供"另存为" |
| `exists` | 新建/重命名重名 | 行内红字提示 |
| `conflict` | 保存时磁盘已变 | 冲突横幅：保留我的 / 载入磁盘版本 |
| `too_large` | 超过 `MaxSize` | 对话框：大文件模式打开 / 取消 |
| `binary` / `encoding` | 非文本、非 UTF | 对话框说明，可只读打开 |
| `io` | 磁盘满、网络盘断开 | 常驻通知（带"复制详情"），会话保留未保存内容并写恢复副本 |
| `invalid` | 非法区间、坏正则 | 开发期断言；正则错误在查找栏内提示 |
| `read_only` | 只读文档上编辑 | 状态胶囊抖动 + 通知（一次） |
| `canceled` | 用户取消 | 不提示 |
| `unavailable` | 无 WebView 运行时 | 公式/图表显示源码；首次一次性提示 |
| `timeout` / `render` | 渲染失败 | 组件内红色细边 + 错误文本 |
| `internal` | 不应发生 | 常驻通知 + 日志（含堆栈） |

## 7. Mock 与一致性测试（Contract-First 的执行机制）

1. **`internal/mock`（QA 在 Phase 0 交付）**——每个契约接口一个内存实现，行为满足本文不变式的"简化版"：
   - `mock.Text`（string 实现）、`mock.Document`（全量重解析 + 线性历史，无合并）、`mock.AST`（直接包装 parser，无扩展、Prefixes 粗略计算）、`mock.Projector`（全部 `SegText`，即"显示源码"）、`mock.Editing`（只处理插入/删除/换行）；
   - `mock.FileStore`（内存文件系统，可注入错误与外部修改）、`mock.Sessions`、`mock.Workspace`（内存树 + 手动触发事件）、`mock.Search`、`mock.Renderer`（返回固定尺寸灰色 SVG）、`mock.Exporter`、`mock.Settings`、`mock.Commands`；
   - `mock.Dispatcher`：两种模式——`Immediate`（同步执行）与 `Queued`（`Flush()` 执行，用于测试回调线程）。
2. **`internal/contracttest`（QA）**——一致性测试套件，签名形如 `func TestFileStore(t *testing.T, newStore func(t *testing.T) contract.FileStore)`。**mock 与真实实现必须通过同一套件**；真实实现所在包的测试文件调用它。
3. 前端在 Phase 1 前半段完全基于 mock 开发；后端实现落地后由 INT 在 `internal/app` 替换注入，前端代码不改。
4. 任何契约变更（CR）必须在同一合并中同时更新：契约代码、mock、contracttest、本文档。

## 8. 版本与冻结

- `ContractVersion = "1.0.0"`。冻结后：
  - **次版本（1.x）**：只允许新增（新类型、新常量、新接口、在结构体末尾新增字段、新命令 ID）。不得修改既有签名或语义。
  - **主版本（2.0）**：破坏性变更，只在阶段边界进行，需 CEO 批准并给出迁移说明。
- CR 流程见 `05-delivery-plan.md §4`。

## 附：为界面基线所做的 RC 修订（2026-11-01，冻结前）

依据 `docs/design`，已直接修改 `spikes/01-contract-rc`：

| 文件 | 变更 | 原因 |
|---|---|---|
| `settings.go` | `Appearance.Accent string` 改为 `AccentHue int`（默认 182） | 强调色、语法色、极光都由一个色相推导 |
| `runtime.go` | `Notice` 增加 `Detail` `Progress` `Sticky`；`Notify` 返回 `NoticeHandle`（`Update` `Dismiss`） | 通知有说明、进度条和导出进度更新 |
| `render.go` | `ExportOptions.OnProgress` | 导出进度 |
| `commands.go` | `CmdViewLive`、`CmdBlockMermaid` | 视图菜单的「实时」、斜杠菜单的 Mermaid |
| `editorapi.go` | `FocusMode` `Typewriter` `ScrollOffset` `CaretBlock` | 视图菜单状态、滚动联动标题、胶囊 |

页边标签、结构导轨、灯箱属于 `liveedit` 内部，不进入契约。
