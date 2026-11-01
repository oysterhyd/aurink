# 01 · 技术可行性报告

> 依据等级：**[实测]** 本机实际运行验证；**[源码]** 阅读 MyGo v0.7.3 源码或官方文档确认；**[待验证]** 需 Spike 验证，未验证前不得作为既定事实使用。
>
> 证据位置：MyGo 源码 `C:\Users\oyster\go\pkg\mod\github.com\egoist\mygo@v0.7.3`；探针测试 `spikes/00-probe/`（独立 Go 模块，`go test -v ./...` 可复现）。

## 1. 环境盘点

| 项 | 状态 | 依据 |
|---|---|---|
| 项目目录 | 空目录，非 git 仓库 | [实测] |
| Go | 本机 1.27.1；MyGo 要求 ≥1.27.3，`GOTOOLCHAIN=auto` 自动下载 1.27.3 成功 | [实测] |
| MyGo CLI | `mygo 0.7.3`（`~/go/bin/mygo.exe`），`mygo doctor`：go / bun 1.4.2 / WebView2 156 / signtool / gh 均就绪；NSIS 缺失（`mygo build` 会自动下载） | [实测] |
| 模板 | `mygo init -template native` 生成纯 Go 原生 UI 工程（`tool github.com/egoist/mygo/cmd/mygo`） | [实测] |
| 构建 | Windows amd64 构建成功；交叉编译 darwin/arm64、darwin/amd64、linux/amd64、windows/arm64、js/wasm 全部成功（`CGO_ENABLED=0`） | [实测] |
| 体积 | 最小原生 UI 应用 release（`-trimpath -tags mygo_noinspector -s -w`）9.9 MB | [实测] |
| 无窗口测试 | `ui.NewTester` 模板测试通过 | [实测] |
| 平台限制 | 本机仅 Windows 11。macOS 签名 / dmg / 公证必须在 macOS 上执行；MyGo 的 `internal/e2e` GUI 测试只在 macOS 桌面会话运行 | [源码] AGENTS.md、`mygo build -h` |

## 2. MyGo 框架能力矩阵（与本项目相关部分）

MyGo 是一个约 29 万行 Go 的桌面框架：窗口可以显示系统 WebView 页面，也可以显示 MyGo 用 GPU 自绘的 **Native UI**（`package ui`），两者可混用。本项目采用 Native UI。

### 2.1 UI 运行模型

| 能力 | 结论 | 依据 |
|---|---|---|
| 视图模型 | 即时模式 API（类 egui / Dear ImGui）+ 框架保留的元素状态；视图是 `func(c *ui.Context)`，每帧按需重建；空闲不绘制 | [源码] docs/ui/views.md |
| 生命周期约束 | `ui.Element` 只在本次 build pass 有效，**严禁**存入应用状态 / goroutine；用 `ui.Handle`、`ui.Local`、Key 保持身份；`mygo vet` 静态检查 | [源码] views.md、mygo-maintenance skill |
| 线程 | 视图、输入回调在主线程；后台 goroutine 通过 `Window.Update(fn)` 回到主线程并触发重绘 | [源码] views.md |
| 渲染后端 | Metal / D3D11（WARP 软件兜底）/ OpenGL / WebGL2，CPU 渲染器最终兜底；`MYGO_FRAME_STATS` 输出逐帧分解耗时 | [源码] rendering.md |
| 布局 | Flexbox + Grid、Absolute、Overlay | [源码] layout.md |
| 虚拟列表 | `ui.List`：仅构建可见行、变高行、按行锚定、百万行 14.5 µs/帧 | [源码] list.md、performance.md |
| 组件 | Outline（树）、Sidebar、Split、Tabs、Popover、Dialog、Toast、Tooltip、ContextMenu、FindBar、Combobox、SearchField、Toolbar 等，均有无样式 Base 版本可自定义外观 | [源码] docs/ui/*.md |
| 测试 | `ui.NewTester`：无窗口渲染、点击/键入/**IME 合成**/快捷键/剪贴板/虚拟时钟/截图；automation 插件支持对真实构建做 E2E | [源码] testing.md + [实测] |

### 2.2 文本与编辑（决定编辑器架构的核心）

| 能力 | 结论 | 依据 |
|---|---|---|
| `ui/markdown` | **只读**渲染视图，面向 LLM 流式回复；增量解析；50 KB 文档整篇显示 1.3 ms/帧，官方建议"长文档分段显示" | [源码] markdown.md |
| `ui/markdown/parser` | 独立包、不依赖 ui；CommonMark 0.31.2 全部 652 例 + GFM（表格/任务/删除线/自动链接）+ 脚注 + 数学；块和内联均带源码**字节范围**；`Edit(from,to,text)` 增量重解析 | [源码] + [实测] |
| `ui/richtext` | 属性文本编辑器（NSTextView 模型）。**Markdown 往返有损**：表格变成"制表符分隔段落"、数学按原文、对齐/字体丢失、源码风格（`*`/`-`、`__`/`**`、Setext、引用式链接、硬换行、HTML、Front Matter）无法保持 | [源码] rich-text-editor.md |
| `ui/editor` | Zed 风格代码编辑器：rope、多光标、折叠、查找替换、Tokenizer/Highlighter、Markdown 语言、百万行流畅。**等宽、统一行高、不能隐藏/替换区间、无块级组件** | [源码] code-editor.md |
| `ui.TextInputClient` | 应用自有文本模型接入系统输入法（NSTextInputClient / IMM32 / GTK IM），UTF-16 坐标；文本、选区、预编辑、撤销全部由应用负责 | [源码] + [实测] |
| `ui.ShapeRichTextWith` | 富文本段落排版：混合字号/字重/字体/颜色/背景圆角/基线偏移/内联对象（U+FFFC）；提供 `Caret`、`IndexAt`、`SelectionRects`、`Lines`、`MoveCaret`（双向文本）、`Objects` | [源码] + [实测] |
| `ui.TextBuffer` | 公开 rope（字节/rune/UTF-16/换行索引、持久快照、O(1) Restore），编辑以 rune 计 | [源码] |
| `ui/richtext` 实现方式 | 仅使用公开原语（`HandleTextInput`、`ShapeRichTextWith`、`ui.Local`、`Draw`）+ `internal/rope`、`internal/text`。证明**同等级自定义编辑器可用公开 API 实现**；其规模（≈8.8K 行）可作为工作量参照 | [源码] |

### 2.3 视觉、动效与交互

| 用户要求的能力 | 结论 | 依据 |
|---|---|---|
| Liquid Glass | ✅ `plugins/glass`：Regular/Clear 玻璃、Tint、Interactive、Clarity、ScrollEdge（软/硬）、渐进模糊；GPU 与 CPU 输出一致，三平台外观一致 | [源码] plugins/glass.md |
| 窗口级材质 | ✅ `Vibrancy`：macOS 全系、Windows 11 22H2+（Mica/Acrylic）；Linux 无 | [源码] ui/windows.md |
| Spring | ✅ 解析解弹簧（不受掉帧影响），`Smooth/Snappy/Bouncy`、Duration+Bounce 或物理参数；中途改目标保持速度 | [源码] motion.md |
| Shared Element | ✅ `ElementTransition.Match`（对标 SwiftUI `matchedGeometryEffect`），跨父容器飞行 | [源码] transitions.md |
| Command Palette | ⚠️ **无内置组件**，需基于 Overlay + TextInput + List + 模糊匹配自建；`examples/launcher` 可参考 | [源码] |
| 微交互 | ✅ `AnimateTo/AnimateColor`、`AnimatePose`（仅重绘不重建视图，最便宜）、`ContentTransition`（数字滚动/图标切换）、`Keyframes`、`Timeline`、`Draggable`（甩动+吸附）、Enter/Exit/Collapse 过渡 | [源码] motion.md |
| 滚动联动 | ✅ `ScrollState`（读写偏移、`ScrollTo` 弹簧滚动）、`InView()`、`glass.ScrollEdge` | [源码] scroll.md |
| GPU Shader | ✅ `ui/shader`（Kage/Go 语法 → Metal/HLSL/GLSL），Fragment / ColorEffect / DistortionEffect / LayerEffect / Backdrop；CPU 解释执行"数秒/整窗"，必须用 `p.Accelerated()` 门控 | [源码] shaders.md |
| 降低动态效果 | ✅ 系统 Reduce Motion 自动生效；`c.DisableAnimations()` | [源码] |
| 自定义标题栏 | ✅ `TitleBarHidden` + `c.TitleBar()` + `DragWindow()`，保留系统窗口按钮 | [源码] ui/windows.md |
| 缩放 | ✅ `c.SetZoom`，矢量清晰缩放 | [源码] |

### 2.4 桌面与文件

| 能力 | 结论 | 依据 |
|---|---|---|
| `mygo.NewDocuments` | NSDocument 级：原子保存（保元数据，Windows `ReplaceFileW`）、自动保存、崩溃恢复副本、外部修改冲突、文件菜单、最近文档、文件关联。**但模型是"一文档一窗口"**，且原子写函数未导出 | [源码] documents.md、document_file.go |
| 文件监听 | ✅ `mygo.WatchFiles`（FSEvents / inotify / ReadDirectoryChangesW），去抖批量事件，`MaxEntries` 上限 8192（macOS 65536） | [源码] native.md |
| 对话框/剪贴板/Shell | ✅ 打开/保存/消息框；剪贴板文本/HTML/图片/多格式/文件；`Shell.TrashItem`（回收站）、`ShowItemInFolder`、`OpenExternal` | [源码] |
| 拖放 | ✅ 文件拖入、应用间拖放、`transfer` 多表示格式 | [源码] |
| 菜单 | ✅ 原生菜单 + 角色（含 Format 菜单角色）、上下文菜单、全局快捷键、最近文档、单实例 | [源码] |
| i18n | ✅ ARB 目录（每语言一个 JSON）→ 生成类型化函数；内置 zh-Hans/zh-Hant/ja | [源码] i18n.md |
| WebView 嵌入 | ✅ `ui.WebView` + `Window.NewWebView`；`Page.Eval`（返回 JSON、可 await）、`LoadHTML`、`PrintToPDF`、`CapturePage` | [源码] |
| SVG | ⚠️ 支持 path/基本形状/use/symbol/渐变/裁剪/遮罩；**不支持 `<text>`、滤镜、外链图片** | [源码] image.md |
| 打包 | ✅ Windows NSIS 安装包、macOS .app/.dmg（需 macOS）、Linux deb/tar；签名更新与增量更新 | [源码] |

## 3. 探针实测结果（`spikes/00-probe`）

| # | 验证项 | 结果 |
|---|---|---|
| P1 | 自定义 `TextInputClient` 接收键入 | ✅ `tt.Type("# Hi")` → `ReplaceText("# Hi")` |
| P2 | 自定义客户端接收 IME 合成 | ✅ `Compose("nihao")` → `SetMarkedText`，标记区间 [4,9)；提交 `你好` 后替换预编辑，文本为 `# Hi你好` |
| P3 | 混排排版 | ✅ 28px 粗体 + 15px 正文 + 中文 + 等宽代码背景 + 40×20 内联对象；两行行高 37.2 / 24（逐行自适应）；`IndexAt`、`Caret` 正常 |
| P4 | 排版开销 | ⚠️ 约 **230 µs / 段**（≈330 字节中英混排，680 宽，DirectWrite）。一屏 40 段全量重排 ≈ 9 ms → **必须按块缓存排版** |
| P5 | 语法标记可定位 | ✅ `**bold**` 节点 [8,16)，子文本 [10,14) → 标记区间可推导；链接、强调、行内代码同理 |
| P6 | 转义/实体 | ⚠️ `\*`、`&amp;` 合并进 Text 节点，`Text` 字面值 ≠ 源码 → 适配层需再扫描生成 Escape/Entity 节点 |
| P7 | CRLF | ✅ 解析器原生处理 `\r\n`，SoftBreak 节点覆盖 `\r\n` 两字节 |
| P8 | YAML Front Matter | ❌ 被解析为"分隔线 + Setext 标题"→ 需预处理层 |
| P9 | 扩展语法 | ❌ `==高亮==`、`^上标^`、`:emoji:`、`[[wiki]]`、`[TOC]` 均为普通文本；`~x~` 按 GFM 为删除线（与 Typora 下标冲突） |
| P10 | 表格/数学/任务/脚注 | ✅ 表格（含列对齐）、`$$` 数学块、任务项、脚注定义均正确 |
| P11 | 增量解析正确性 | ✅ 890 KB 文档 200 次中部插入后，与全量解析 HTML 完全一致、块数一致 |
| P12 | 增量解析性能 | ⚠️ 全量（含内联）18.9 ms / 9 万块；**单次编辑 6 ms**（O(n)：源码串复制 + 块偏移平移）。≤200 KB 文档约 ≤1.5 ms，可接受；>1 MB 需"大文件模式" |
| P13 | `editor.Buffer` 保真 | ✅ LF / CRLF / BOM / 无尾换行均字节一致；❌ **混合换行被归一化**（`a\r\nb\nc\r\n` → 全 CRLF） |
| P14 | `ui.TextBuffer` | ✅ CRLF + CJK + emoji 往返一致 |

## 4. 关键技术难点与方案选择

### 4.1 难点一：所见即所得 + Markdown 无损（最高风险，决定全局架构）

| 方案 | 描述 | 无损 | WYSIWYG 品质 | 结论 |
|---|---|---|---|---|
| A. `ui/richtext` + 导入导出 | 打开时 Markdown→属性文本，保存时导出 | ❌ 结构性有损（见 2.2） | 高，开箱即用 | **否决** |
| B. `ui/editor` + 装饰 | 代码编辑器上加高亮 | ✅ | ❌ 等宽统一行高，无法隐藏标记、无块组件 | 否决作为主编辑器；**采用为源码模式** |
| C. 自研 Live Markdown 引擎 | **源码是唯一真相**；解析器 AST（字节范围）→ 按块"投影"成显示片段（光标处显示源码标记，其余隐藏/替换为组件）；自定义元素 + `TextInputClient` + `ShapeRichTextWith` + 视口虚拟化 | ✅ **构造上无损**：所有用户操作都是源码字节替换，从不从模型反序列化 | 高（Typora / Obsidian Live Preview 模式） | **采用** |

方案 C 的无损定义（写入验收标准）：
1. 未编辑区域逐字节不变（含换行风格、缩进、列表符号、强调符号风格、引用式链接、HTML、尾随空白）。
2. 编辑只产生最小源码改动；不做隐式重排（表格对齐、列表重编号等仅在用户执行显式命令时发生）。
3. 打开→不编辑→保存：字节完全一致（含 BOM、混合换行、无尾换行）。

### 4.2 难点二：IME 与自定义编辑器
- 系统输入法通过 `TextInputClient` 查询"当前文档"的 UTF-16 文本与几何。我们的显示文本是投影后的文本（标记隐藏），与源码不一一对应。
- 方案：IME 文档 = **活动块的显示文本 + 预编辑**；预编辑**不写入源码**（不进历史、不触发解析/自动保存），只作为投影中的虚拟片段；提交时生成一个 `OriginIME` 事务。
- [待验证] Windows 微软拼音/日文 IME、macOS 拼音与长按变音、Linux Fcitx5 在"块级文档"下候选框定位与重转换行为。→ Spike S1。

### 4.3 难点三：实时渲染性能
- 排版 230 µs/段 → 按块缓存 `TextLayout`，键为 `(BlockID, 投影修订号, 宽度, 主题修订号)`；击键只重排一个块。
- 视口虚拟化：Fenwick 树维护"已测量高度 + 未测量计数 × 估计值"，O(log n) 求顶部偏移与命中块（与 `ui/richtext/heights.go` 同思路）。
- 解析：`parser.Edit` 增量，O(n) 常数小；文本存储用不可变 `string` 快照（编辑时复制约 0.1 ms/MB，远小于解析成本，因此 rope 收益有限，可后置）。
- 预算（Windows 中端笔记本，`MYGO_FRAME_STATS`）：≤200 KB 文档击键帧 p95 < 8 ms；≤1 MB < 16 ms；>1 MB 默认进入大文件模式（源码模式 + 只读预览）。

### 4.4 难点四：数学公式与 Mermaid（框架无原生能力）
- MyGo 的 `ui/markdown` 仅把 TeX 显示为等宽源码，提供 `InlineMath` 钩子让应用自行渲染；无 Mermaid。
- 方案：**懒启动的离屏 WebView 渲染服务**——本地打包 MathJax（tex→svg 输出全为 `<path>`，与 MyGo SVG 子集兼容）与 Mermaid；Go 侧通过 `Page.Eval` 请求渲染，返回 SVG（数学）或 PNG（Mermaid 含 `<text>`，用截图/canvas 导出）；结果按内容哈希缓存（内存 LRU + 磁盘）。
- 降级：无 WebView 运行时（Linux 未装 WebKitGTK、Win10 无 WebView2）时显示带样式的源码，不阻塞编辑。
- [待验证] 隐藏 WebView 不放入视图时能否稳定执行 `Eval`；MathJax SVG 在 `ui.ParseSVG` 的兼容度；单公式延迟（目标 < 30 ms，缓存命中 < 1 ms）；WebView 进程内存（预计 80–150 MB，仅含公式文档时启动）。→ Spike S2。
- 备选：纯 Go TeX 子集排版（质量与覆盖度明显弱于 MathJax，仅作兜底调研）。

### 4.5 难点五：多文档工作区 vs `NewDocuments` 的单文档窗口模型
- 产品形态需要"工作区 + 侧边栏文件树 + 多标签"。`NewDocuments` 要求每个文档一个窗口，且原子写未导出。
- 方案：自建 `docsession` + `fsstore` 服务，参照 `document_file.go` / `document_recovery.go` 的算法（同目录临时文件 + fsync + 替换；Windows 用 `ReplaceFileW` 保留 ACL/属性；保存前比对 mtime+size 检测冲突；恢复副本）。MIT 许可允许参照。

### 4.6 其他
| 难点 | 方案 |
|---|---|
| Front Matter / 扩展语法 | 解析适配层：前置识别 `---\n…\n---`，对其后正文解析并整体偏移；内联扩展在 Text 节点上二次扫描；`~x~` 默认遵循 GFM（删除线），下标作为可选设置 |
| 混合换行 | 文本存储保留原始字节（不用 `editor.Buffer` 做主存储）；新插入换行采用文档主导风格 |
| BOM / 编码 | 读入剥离 BOM、记入 `DocMeta`，保存还原；UTF-16 带 BOM 转换；GB18030 等旧编码 P2（`golang.org/x/text` 已是依赖） |
| 源码模式同步 | 进入源码模式时用当前文本构造 `editor.Buffer`；每帧比对 `Version`，变化时以公共前后缀求差生成事务写回核心模型；退出时销毁。`Highlighter.Edit` 只在视图绘制时惰性回放，不能作为通用变更订阅 [源码 syntax.go] |
| HTML 粘贴 → Markdown | 复用 `richtext.ParseHTML/ParseRTF(...).Markdown()`（粘贴转换场景可接受其有损性），隔离在 `adapters` 层 |
| 代码块高亮 | 复用 `editor.LanguageFor` + `Language.Tokenizer` + `editor.Theme` 的 Token 颜色，逐行增量 |
| 字体 | UI 用系统字体；正文默认系统无衬线 + CJK 回退（PingFang SC / Microsoft YaHei UI / Noto Sans CJK）；可选打包 Inter、JetBrains Mono（OFL，`ui.RegisterFont`）；不打包 CJK 字体（体积 10 MB+） |

## 5. 风险登记

| ID | 风险 | 概率 | 影响 | 缓解 | 负责 |
|---|---|---|---|---|---|
| R1 | 自研编辑引擎复杂度超预期（光标/选区/IME/虚拟化交织） | 高 | 致命 | Phase 0 Spike S1 设 Go/No-Go；MVP 语法范围收敛；投影与编辑逻辑做成纯函数并以属性测试覆盖；参照 richtext 公开实现 | FE-1 / BE-1 |
| R2 | 块级 IME 文档在某平台异常（候选框位置、重转换、Windows TSF） | 中 | 高 | S1 三平台 IME 矩阵；备选：暴露光标附近 N 块窗口 | FE-1 |
| R3 | 解析器 O(n) 编辑成本在大文档上超预算 | 中 | 中 | 大文件模式；Phase 4 评估按顶层段落分片解析（引用式链接定义需全局处理） | BE-1 |
| R4 | WebView 渲染服务不可用/慢/占内存 | 中 | 中 | 懒启动、缓存、源码降级；只在文档包含公式/图表时启动 | BE-2 |
| R5 | MyGo 为 0.x，API 可能变动 | 中 | 中 | 锁定 v0.7.3；升级只由 INT 执行（`mygo migrate-ui`），独立分支验证 | INT |
| R6 | 开发机仅 Windows，macOS/Linux 验证缺位 | 高 | 中 | GitHub Actions 三平台矩阵跑构建与 Tester 测试；macOS 签名、IME、Vibrancy 需要真机/云 Mac 人工验收 | INT / QA |
| R7 | Glass / Shader 在核显或 WARP/CPU 渲染下掉帧 | 中 | 中 | `p.Accelerated()` 门控；"减少透明度"设置；CPU 路径用纯色半透明替代 | FE-2 |
| R8 | 自定义编辑器的无障碍能力弱于系统文本控件 | 中 | 低 | 提供 TextField 角色、标签、选区；读屏全文朗读列为已知限制 | FE-1 |
| R9 | 扩展语法与 CommonMark 冲突（`~`、`$`、`==`） | 低 | 低 | 默认 GFM 语义，扩展逐项可开关 | BE-1 |
| R10 | 多 Agent 并行导致契约漂移 | 中 | 高 | 契约冻结 + 一致性测试套件 + CR 流程 + 架构导入规则测试 | CEO / QA |

## 6. Spike 清单（Phase 0，均限时，结论写入 `spikes/<id>/REPORT.md`）

| ID | 负责 | 目标 | Go 标准（全部满足） | No-Go 时的后备 |
|---|---|---|---|---|
| S1 编辑引擎原型 | FE-1 | 自定义元素 + TextInputClient；200 块文档中 `**粗体**`/标题的光标处显隐；跨块光标移动与选区；1 万块虚拟滚动；IME | 1 万块滚动无整屏重排（帧统计无全量 layout）；击键 p95 < 8 ms；Windows 微软拼音组字/提交/候选框位置正确；`Tester` 回归测试通过 | 方案降级为"块级源码切换"（活动块整块显示源码，其余渲染），仍保持无损 |
| S2 渲染服务 | BE-2 | 离屏 WebView + MathJax → SVG → `ui.ParseSVG`；Mermaid → PNG | 50 个常见公式 SVG 可解析且视觉正确 ≥ 95%；单公式冷渲染 < 50 ms、缓存 < 1 ms；不放入视图时 `Eval` 稳定 | 公式显示源码 + 外部浏览器预览；纯 Go 子集渲染器调研 |
| S3 AST 适配 | BE-1 | 前缀区间计算、BlockID 稳定性、5 MB 下解析曲线、分片可行性 | 前缀不变式（去前缀后内容 == `parser.Lines`）在语料上 100% 成立；随机编辑序列下未触及的块 ID 不变 | 保守策略：BlockID 退化为"(种类, 起始偏移)"并接受更多重排 |
| S4 外观性能 | FE-2 | 玻璃工具栏覆盖滚动内容、Win11 Vibrancy 侧栏、`MYGO_GPU=0` CPU 路径、主题切换 Shader | GPU 路径滚动 60 fps；CPU 路径有可接受降级方案 | 玻璃仅用于浮层，不用于常驻工具栏 |
