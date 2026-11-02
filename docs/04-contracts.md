# 04 · 统一接口契约（Contract v1.0-rc）

> **权威源码**：`spikes/01-contract-rc/contract/*.go` 与 `spikes/01-contract-rc/editorapi/editorapi.go`（独立模块，已通过 `gofmt`、`go vet`、`go build`，依赖仅 `context errors io iter time`）。
> 当前代码在 `internal/contract/` 与 `internal/views/editorapi/`，`spikes/01-contract-rc` 是历史记录。本文档解释语义与规则；**代码与本文冲突时以代码为准，并修正本文**。

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

| 文件 | 内容 | 实现位置 |
|---|---|---|
| `text.go` | `Range` `Version` `TextEdit` `Change` `Text` `EOLStyle` `DocMeta` | `core/text` |
| `document.go` | `DocID` `Selection` `Origin` `Transaction` `DocEvent` **`DocumentModel`** `DocumentFactory` | `core/document` |
| `ast.go` | `BlockID` `BlockKind` `Block` `Inline` `Extensions` `Heading` `LinkDef` **`AST`** | `core/markdown` |
| `projection.go` | `Reveal` `Segment` `ContainerDecor` `BlockProjection` **`Projector`** | `core/projection` |
| `editing.go` | `Intent` `EditContext` `EditCommand` `FormatState` **`Editing`** `PasteConverter` | `core/editing`、`adapters/richpaste` |
| `commands.go` | 命令 ID 常量、`Chord` `KeyBinding` `Command` **`CommandRegistry`** | `core/commands` |
| `services.go` | `FileStore` `Session` `Sessions` `Workspace` `Assets` `Recent` `Finder` `WorkspaceSearch` `Fuzzy` | `core/fsstore` `core/docsession` `core/workspace` `core/search` |
| `render.go` | `Renderer` `Exporter` `Printer` | `core/render`、`core/export`；`Printer` 在 `internal/app` 用 WebView 实现 |
| `settings.go` | `Settings` 及子结构、`SettingsStore` | `core/settings` |
| `runtime.go` | `Dispatcher` `Notifier` `Notice` `NoticeHandle` | `internal/app`（Dispatcher）、`views/shell`（Notifier） |
| `errors.go` | `Code` `Error` `E` `CodeOf` | — |
| `editorapi` | `Controller` `Registry` `Mode` | `views/liveedit` |

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

违反规则的代码视为缺陷。`contracttest` 中用"主线程守卫"（mock Dispatcher 记录调用 goroutine）验证异步服务的回调线程。

## 4. 核心语义与不变式

### 4.1 Text / DocumentModel
1. `Version` 严格单调；`Apply` 成功恰好 +1（空 `Edits` 且选区不变时返回原版本、不发事件）。
2. `Apply` 校验失败返回 `CodeInvalid`，文本、选区、历史均不变。只读时返回 `CodeReadOnly`。
3. `Selection == nil` 时按以下规则映射旧选区：插入点上的光标移到插入文本之后；区间起点在插入点时不扩张，终点在插入点时不扩张；整个落在被替换范围内的区间收拢成光标，不改变方向。`SetSelection` 设置相同选区时不发事件。
4. 撤销后选区恢复为该步的"选区前"；重做恢复为"选区后"。
5. `Replace` 为一步历史，`Origin` 记入事件。`OriginReload` 的事件中 `Changes` 为单个整篇替换。选区保持原偏移，截断到新文本长度并落在字符起点，不按第 3 条映射。
6. 订阅者收到的事件按版本递增；回调内发起的编辑立即返回，事件排在当前事件之后。
6a. `DocEvent.Dirty`：包含所有内容变化或新建的块 ID；未变化且仅偏移平移的块不在其中。
7. 不变式（属性测试）：对任意事务序列，`model.Text().String()` 等于对原始字符串依次执行相同字节替换的结果；任意步数撤销后再重做，文本与选区回到原状态。

### 4.2 AST
1. `AST.Version() == Text.Version()`（同一快照）。
2. 块按文档顺序；`Block(0)` 为根；子块位于 `(i, Block(i).Next)`。
3. 叶子块（`Paragraph Heading ThematicBreak Code HTML Math TableCell FrontMatter TOC Blank LinkDef`）构成 `Leaves()` 序列，即编辑器虚拟化的行；表格单元格作为叶子，表格行/表格本身为容器。
4. 内容行不变式：对每个内容行，`Padding` 个空格 + `源码[Lines[k].Source]` 等于 `parser.Line(i,k)`（部分消耗的 Tab 靠 `Padding` 表示，不改写源码）。对段落/标题叶子，`源码[Range]` 去掉 `Prefixes` 与 `Markers` 后也与内容行一致。`Content` 只是包围区间，不能当纯文本用。
5. 内联：`Open`/`Close` 为空区间表示无定界符；子节点范围位于 `[Open.End, Close.Start)` 内。
6. `BlockID` 稳定性：种类和映射后完整区间都不变的块必须保持 ID；其余候选在种类和映射后起点相同、旧起点未被删除、旧 ID 尚未占用时按文档顺序继承（完整区间相同的块优先）；其余为新 ID，ID 永不复用。被语法吞并或合并而消失的块（如插入开放围栏）不保留身份，即使未与 `Change` 相交。ID 不表示内容未变：引用定义变化时使用该引用的块 ID 不变，但必须出现在 `DocEvent.Dirty` 中。
7. Front Matter 仅在 `ExtFrontMatter` 开启且位于文档首行时识别。
8. 覆盖不变式（CR-001）：按 `Leaves()` 顺序，第一个叶子的 `Lead.Start == 0`；每个叶子 `Lead.End == Range.Start`、`Range.End == Trailer.Start`；下一个叶子的 `Lead.Start` 等于本叶子的 `Trailer.End`；最后一个叶子的 `Trailer.End` 等于源码长度。即所有叶子的 `[Lead.Start, Trailer.End)` 无缝无叠地覆盖整个文档，`LeafAt` 对任何偏移都返回拥有它的叶子（偏移等于源码长度时返回最后一个叶子）。
9. `BlockBlank` 只在以下位置产生（CR-001）：
   1. 叶子的 `Trailer` 已含一个分隔行（去掉容器前缀后只剩空白的行）后，其后每多一个分隔行产生一个：`Lead` 为该行容器前缀，`Range` 为剩余空白，`Trailer` 为行终止符；
   2. 没有叶子后代的容器（空列表项、空引用、空脚注定义）产生一个子块，`Range` 是容器语法之后的空区间；
   3. 最后一个叶子的 `Trailer` 以分隔行结尾时，在源码末尾追加一个三个区间都为空的；
   4. 空文档恰有一个，三个区间都是 `[0,0)`。
   `BlockBlank` 的 `Parent` 是包含该行的最内层容器（没有则为根），BlockID 规则（第 6 条）同样适用。解析器不产生块的链接引用定义行归 `BlockLinkDef`。

### 4.3 Projection
1. 片段覆盖叶子块的 `[Lead.Start, Trailer.End)`，按源码顺序首尾相接（**无缝无叠**）；`Lead`、`Trailer` 一律为 `SegHidden`，容器语法的视觉由 `ContainerDecor` 表达（CR-001）。
2. `SegText`/`SegMarker` 的 `Display` 必须等于对应源码字节，但不得引用 `Text.String()` 的底层字节（投影会被长期缓存，实现可对一个块克隆一次，CR-002）；`SegHidden` 的 `Display` 为空；`SegObject` 的 `Display` 为 `"￼"`。
3. `ToSource(ToDisplay(s)) == Snap(s)`；`ToDisplay` 单调不减；`Snap` 的结果不落在 `SegHidden`/`SegObject` 内部、不切开字素。
4. `Sig` 是排版缓存键（CR-002）：覆盖 `Kind`、`Level`、`Lang`、`Revealed`、`Containers`（不含 `Block`、`TaskSrc`）与各片段的 `Kind`、`Display`、`Style`、`Object`；不含 `Block`、`Index` 与绝对偏移，块整体平移时不变；任一影响排版的输出变化必须改变 `Sig`。`Sig` 相同不代表 `Src` 映射相同（如 `&amp;` 与 `&#x26;`）。
5. `RevealNone`（阅读模式）下不存在 `SegMarker`。
6. 投影的输入（CR-002）：`Project` 的输出在平移后只取决于该叶子 `[Lead.Start, Trailer.End)` 的字节与容器链、`Reveal` 中与本块相关的部分，以及全局表 `LinkDefs`、`Footnotes`、`Outline`（`[TOC]` 用）、`Exts`、`ObjectLangs`。缓存的投影只有在本块未与任何 `Change` 相交且全局表未变时才能平移复用（`Segments[].Src` 与 `Containers[].TaskSrc` 一起平移）；不能以 `Sig` 相同为复用条件。
7. `Affinity`（CR-002）：显示索引 `d` 处夹着连续的零宽片段（`SegHidden`，含 `Lead`、`Trailer`）时，`d` 对应源码区间 `[h0, h1]`：`Upstream` 取 `h0`，`Downstream` 取 `h1`。`ToSource` 与 `Snap` 的结果限制在 `[Range.Start, Range.End]` 内（不进入 `Lead`、不越过 `Trailer`）。唯一例外是待定标记（02 §5 显隐规则：其后尚无空格的单独 `#`、`+`、`>` 等按普通文本显示）：它的源码在 `Lead` 或 `Trailer` 里（解析器把 `>` 放在 `Trailer`，`1.` 可能横跨两者），作为可见的 `SegText` 1:1 映射，结果区间扩到覆盖这些可见片段；§4.3.1 "`Lead`、`Trailer` 隐藏"对它同样例外（2026-11-02）；`ToDisplay` 对该区间内任何偏移返回同一个 `d`。

### 4.4 Editing
1. 纯函数：同输入同输出，不读写全局状态、不做 I/O。
2. 返回的事务必须满足 §4.1 的合法性；插入换行使用 `ctx.Meta.Dominant`。
3. 只做最小改动：不重排未涉及的列表编号、表格对齐、空白（显式命令 `table.format` 除外）。
4. 输入规则产生的格式化事务使用 `OriginInputRule`，且与触发它的键入分属两步历史：视图应用键入的事务后调用 `FollowUp` 取得规则事务再应用。
5. `ok=false` 表示"不处理"，视图回退到默认插入/删除行为（由 editing 的基础意图实现，不会出现两套逻辑）。

### 4.5 Sessions / FileStore
1. 打开同一规范化路径返回同一 `Session`（Windows 大小写不敏感比较）。
2. "打开→不编辑→保存"后文件字节完全一致（含 BOM、换行、尾换行）。
3. `Write` 原子：任何时刻崩溃，磁盘上要么是旧文件、要么是新文件。例外：硬链接文件和无法创建临时文件的目录只能原地写，崩溃可能留下部分内容（03 §5.1）。
4. `expect` 戳不符返回 `CodeConflict`，不写入。
5. 自动保存只写不处于冲突/删除/只读/未命名状态的会话；其余写恢复副本。

### 4.6 CommandRegistry
1. 命令 ID 全局唯一；未登记在 `commands.go` 的 ID 只允许以 `x.` 前缀用于实验。
2. 默认快捷键不得冲突（测试断言）。用户键位的冲突由 `SetUserBindings` 返回，冲突的按键不生效，但不影响命令注册。Tab、Esc 的含义取决于焦点所在视图，由视图处理，不进默认键位。
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

1. **`internal/mock`（按需编写）**——契约接口的内存实现，行为满足本文不变式的"简化版"：
   - `mock.Text`（string 实现）、`mock.Document`（全量重解析 + 线性历史，无合并）、`mock.AST`（直接包装 parser，无扩展、Prefixes 粗略计算）、`mock.Projector`（全部 `SegText`，即"显示源码"）、`mock.Editing`（只处理插入/删除/换行）；
   - `mock.FileStore`（内存文件系统，可注入错误与外部修改）、`mock.Sessions`、`mock.Workspace`（内存树 + 手动触发事件）、`mock.Search`、`mock.Renderer`（返回固定尺寸灰色 SVG）、`mock.Exporter`、`mock.Settings`、`mock.Commands`；
   - `mock.Dispatcher`：两种模式——`Immediate`（同步执行）与 `Queued`（`Flush()` 执行，用于测试回调线程）。
2. **`internal/contracttest`**——一致性测试套件，签名形如 `func TestFileStore(t *testing.T, newStore func(t *testing.T) contract.FileStore)`。**mock 与真实实现必须通过同一套件**；真实实现所在包的测试文件调用它。
3. 后端未实现时前端可以先用 mock；实现落地后在 `internal/app` 替换注入，前端代码不改。
4. 任何契约变更都在同一次提交里同时更新：契约代码、mock、contracttest、本文档。

## 8. 版本

契约不冻结，随实现演进，变更规则见 05 §4。`ContractVersion` 暂保持 `1.0.0`，首次发布前再定版本策略。

## 附：为界面基线所做的 RC 修订（2026-11-01）

依据 `docs/design`，已直接修改 `spikes/01-contract-rc`：

| 文件 | 变更 | 原因 |
|---|---|---|
| `settings.go` | `Appearance.Accent string` 改为 `AccentHue int`（默认 182） | 强调色、语法色、极光都由一个色相推导 |
| `runtime.go` | `Notice` 增加 `Detail` `Progress` `Sticky`；`Notify` 返回 `NoticeHandle`（`Update` `Dismiss`） | 通知有说明、进度条和导出进度更新 |
| `render.go` | `ExportOptions.OnProgress` | 导出进度 |
| `commands.go` | `CmdViewLive`、`CmdBlockMermaid` | 视图菜单的「实时」、斜杠菜单的 Mermaid |
| `editorapi.go` | `FocusMode` `Typewriter` `ScrollOffset` `CaretBlock` | 视图菜单状态、滚动联动标题、胶囊 |

页边标签、结构导轨、灯箱属于 `liveedit` 内部，不进入契约。

## 附：CR-001、CR-002 修订（2026-11-01）

| 文件 | 变更 | 依据 |
|---|---|---|
| `ast.go` | 新增 `BlockBlank`、`BlockLinkDef`；`Block` 末尾新增 `Lead`、`Trailer`；`LeafAt` 注释改为覆盖语义 | CR-001 批复；§4.2.3、§4.2.8、§4.2.9 |
| `projection.go` | `Segment.Display` 不得引用整份源码；`Sig` 只作排版缓存键；`Affinity` 的精确定义 | CR-002 批复；§4.3.1、§4.3.2、§4.3.4、§4.3.6、§4.3.7 |
| `document.go` | 注释示例 `"undo.typing"` 改为 `"editing_undo_typing"` | 05 §3.4（只改注释） |

已在 `internal/contract` 落地。`spikes/01-contract-rc` 保持原样，作为历史记录。

## 附：CR-003、CR-004、CR-005（2026-11-01）

| 文件 | 变更 | 依据 |
|---|---|---|
| `ast.go` | 新增 `BlockLine`、`Block.Lines`；`Content`、`Prefixes` 注释澄清 | CR-003；§4.2.4 |
| `ast.go` | `BlockID` 的存活规则 | CR-004；§4.2.6 |
| `render.go` | 数学公式可回退 PNG；SVG 和 PNG 恰有一个非空 | CR-005 |

## 附：Phase 2 批次 1（2026-11-01）

依据 [07 §5](07-phase2.md) 的设计决定。套件在 `internal/contracttest`：`TestWorkspace` `TestAssets` `TestRecent` `TestFinder` `TestWorkspaceSearch` `TestFuzzy` `TestExporter`，测试替身 `SnapshotWatcher`（手动 `Flush` 比对真实目录产生事件，不靠计时）、`RecordingTrasher`。

| 文件 | 变更 | 依据 |
|---|---|---|
| `watch.go`（新） | `FileWatcher` `Watch` `WatchEvent` `WatchOptions`；事件路径以传入的路径为前缀 | D1 |
| `watch.go` | `Trasher`，core 通过它删除到回收站 | D7、D18 |
| `watch.go` | `SessionState` `TabState` `ViewState` `SessionStore` | D9、D10 |
| `services.go` | `Workspace.Ignored`；`SetShowAll` 的过滤范围和事件 | D15 |
| `services.go` | `WorkspaceSearch.Search` 增加 `SearchOptions`（忽略规则、大小上限、每文件上限）；跳过的文件种类、取消语义 | D15 |
| `services.go` | `Match` 的行号、预览截断、跨行命中；`Query` 的大小写和全词规则；`PreviewMax` | BE-2.6 |
| `services.go` | `Fuzzy` 的命中与排序规则；`Assets` 的目录模板、命名、相对路径；`RecentMax` 与去重 | BE-2.5、BE-2.6 |
| `render.go` | `ExportOptions.Highlight` 与 `CodeToken`；导出 HTML 的结构约定（id、TOC、代码、脚注、安全范围、内嵌图片） | D13、D17 |

| `services.go` | `Sessions.Recheck`、`Sessions.Moved`；外部变更的处理规则（修改、删除、同目录改名、重建、自己的保存） | D2–D6 |
| `editing.go` | `PastePayload`：转换在视图里做，Editing 只选用哪种表示；代码块和表格单元格的规则 | D11、D12 |

另加套件 `TestEditingV2` `TestPasteConverter` `TestSessionsV2` `TestSessionStore`。`docsession.Manager` 的 `Recheck`、`Moved` 暂为空实现，由 BE-2.7 完成。
