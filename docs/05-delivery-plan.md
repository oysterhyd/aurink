# 05 · 交付计划

> 2026-11-01 起改为单人负责：一位负责人（Claude Code）、一个 `main` 分支，不再有多 Agent、worktree 和合并流程。任务卡 ID 的前缀（BE、FE、INT、QA）只表示工作领域，历史报告仍用这些 ID。

## 1. 工作方式

**负责人（Claude Code）** 负责规划、设计、编码、测试和阶段门评估，并直接决定技术问题。

**用户** 决定产品方向和取舍，比如功能范围变化、Spike 的 No-Go 是否接受后备方案；也负责真机检查（输入法、观感）。

结论标注 [实测] / [源码] / [待验证]。没有实际运行验证的，不写成“完成”或“通过”。

## 2. 仓库

- 工作目录 `D:\mygo\project\Aurink_claude`，唯一分支 `main`，远端 `github.com/oysterhyd/aurink`（私有）。
- 直接提交到 `main`。提交前跑本地门禁（§5.1），小步提交，提交信息说明做了什么。

## 3. 代码规则

### 3.1 依赖方向
分层和导入规则见 03 §1.1，由 `tests/arch` 检查。

### 3.2 命令与测试数据
- 每个模块在自己包内提供 `func Commands(deps…) []contract.Command`，由 `internal/app` 统一注册。
- 模块自己的测试数据放包内 `testdata/`；共享语料放 `testdata/corpus`。

### 3.3 第三方依赖白名单
纯 Go、无 cgo、许可证 MIT/BSD/Apache/ISC/OFL：

| 依赖 | 用途 |
|---|---|
| `github.com/egoist/mygo v0.7.3`（锁定） | 框架 |
| `golang.org/x/text` | 编码转换、Unicode |
| `github.com/rivo/uniseg` | 字素/词边界 |
| `gopkg.in/yaml.v3` | Front Matter 页眉 |
| MathJax 3（Apache-2.0）、Mermaid（MIT）离线包 | 渲染服务 |
| Newsreader、Noto Serif SC（子集）、JetBrains Mono（OFL） | 内置字体，许可证随 `resources/fonts` 分发 |
| Lucide 图标（ISC） | 图标 SVG |
| gemoji 数据（MIT） | emoji 短码表（生成 Go 表） |

白名单外的依赖要先确认纯 Go、无 cgo、许可证合规，再加入本表。MyGo 升级用 `go tool mygo migrate-ui`，单独一次提交。

### 3.4 i18n 文案键规则

MyGo 要求键匹配 `^[A-Za-z][A-Za-z0-9_]*$`，点号只留给 MyGo 自带的 `mygo.<key>`；生成的函数名是键按 `_` 切分后各段首字母大写再拼接，两个键生成同名函数时报错 [源码 `internal/i18ngen/load.go`、MyGo `docs/i18n.md`]。本项目收紧为：

| # | 规则 | 例 |
|---|---|---|
| K1 | 键全小写蛇形：`^[a-z][a-z0-9]*(_[a-z][a-z0-9]*)+$`，至少两段。这样任意两个合法键都不会生成同名函数 | `shell_tabs_close` → `locales.ShellTabsClose()`；写 `liveedit_heading1`，不写 `liveedit_heading_1` |
| K2 | 第一段是模块的 Go 包名（`tools/i18ncheck` 里有清单） | `palette_empty_results` |
| K3 | 两种语言键集合相同、占位符相同；`en.json` 的每个键都有 `@<键>.description`，写明文案出现在哪 | |
| K4 | `mygo.<key>` 只用来覆盖 MyGo 已有的文案 | `mygo.appName` |
| K5 | 占位符用小驼峰 `^[a-z][A-Za-z0-9]*$`，避开 Go 关键字 | `{fileName}` |

- 文案直接写在 `locales/en.json`、`locales/zh-Hans.json`，改完运行 `go tool mygo generate`。
- 命令标题键写成 `<包名>_cmd_<CommandID 转写>`，如注册 `file.openFolder` 的包是 shell 时为 `shell_cmd_file_open_folder`。
- 静态文案调用生成函数，如 `locales.ShellTabsClose()`；契约字段里传键（`Command.Title`、`Command.Category`、`Transaction.Label`），视图用 `locales.Catalog.Format(i18n.Locale(), key, args)` 取文本。

## 4. 契约

`internal/contract` 是模块之间的边界，不再冻结，也没有 CR 流程。需要改就直接改，同一次提交里同步实现、`internal/mock`、`internal/contracttest` 和 04 文档。`docs/cr/` 只留作设计记录。

`internal/mock` 只在测试或演示确实需要时才写，不要求每个接口都有。真实实现通过 `contracttest` 验证不变式。

## 5. 门禁与 CI

### 5.1 门禁 `scripts/gate.ps1` / `gate.sh`
1. `gofmt -l .` 输出为空
2. `go vet ./...`（windows/linux/darwin），`go vet -tags mygo_noinspector ./...`（本机平台）
3. `go build ./...`：windows/amd64、linux/amd64、darwin/arm64（`CGO_ENABLED=0`）
4. `go tool mygo vet .`
5. `CGO_ENABLED=0 go test ./...`
6. `go run ./tools/i18ncheck`

默认只查根模块，`-Spikes` / `--spikes` 才查 `spikes/*`。

### 5.2 CI
- 推送 `main`：Linux 跑完整门禁，Windows、macOS 只跑测试。只改 `.md` 不触发。
- 每周和手动运行：再加 5 个构建目标和 `spikes/*`。
- 不用等 CI 跑完再继续工作；CI 失败当天修。

## 6. 分阶段计划与任务卡

工期是估算（工作日）。所有任务的通用完成定义（DoD）：
- 通过门禁；新增代码有测试（core 包语句覆盖率 ≥ 80%，`projection`/`editing`/`document` ≥ 90%）；
- 遵守线程规则与 `mygo vet`；公开标识符有 godoc；
- 无未登记的 TODO（格式 `TODO(任务卡ID): …`）；
- 在 `reports/<agent>.md` 记录完成情况。

### Phase 0 · 地基（当前阶段）

**现状（2026-11-01）**
- 已完成：INT-0.1、INT-0.2、S1（建议 Go）。
- 已交付但结论是 No-Go，要在 M0 处理：S2（视觉完整 47/50、WebView2 内存高）、S3（严格前缀 1,925/1,934）。
- 未开始：S4、QA-0.1、QA-0.2。
- 待落地：CR-001、CR-002 已批准的契约修改（04 文档末尾的附录），`internal/contract` 还没改。
- 待决定：CR-003、CR-004、CR-005。

下表保留原任务卡。QA-0.1 的 mock 按 §4 缩减。

| 卡 | 依赖 | 交付物 | 验收标准 | 估时 |
|---|---|---|---|---|
| INT-0.1 仓库脚手架 | — | （已完成：`git init -b main`、`.gitignore`、`.gitattributes`、私有仓库 `oysterhyd/aurink`、基线 CI；MyGo 技能已安装）`go.mod`（模块 `aurink`）；`mygo.json`（name Aurink、identifier `app.aurink.editor`、i18n、fileAssociations md/markdown）；`main.go`；§11 架构目录骨架（每包 `doc.go`）；`scripts/gate.*`；把 `ci.yml` 的内联步骤改为调用 `scripts/gate.sh`、`setup-go` 改用 `go-version-file: go.mod`；`reports/` 模板 | 门禁在 Windows 通过；三平台交叉编译通过；`go tool mygo dev` 能启动空窗口 | 1.5 |
| INT-0.2 契约落地 | 0.1 | `internal/contract/*`、`internal/views/editorapi/*`（复制 rc，改 import 路径）；"contract 只依赖标准库"由门禁/CI 检查（`ci.yml` 已含），QA-0.2 再写入 `tests/arch` | 编译通过；与 rc 逐字一致（除 import）；标签 `contract-v1.0-rc` | 0.5 |
| QA-0.1 Mock 与一致性套件 | INT-0.2 | `internal/mock/*`（§7 of 04 全部）、`internal/contracttest/*` 骨架（每接口至少覆盖 04 文档列出的不变式） | 全部 mock 通过 contracttest；FE 可用 mock 组装一个可运行窗口（由 INT 演示） | 3 |
| QA-0.2 架构规则与语料 | INT-0.1 | `tests/arch`：03 文档 §1.1 依赖规则表全部条目；`testdata/corpus`：CommonMark/GFM 规范示例（可从 MyGo `ui/markdown/parser/testdata` 获取）、CJK、CRLF、BOM、混合换行、无尾换行、Front Matter、真实 README 样本 ≥ 50、生成器（1 MB / 5 MB） | 规则测试能检出故意违规；语料清单 `testdata/corpus/INDEX.md` | 2 |
| S1 编辑引擎原型 | rc | `spikes/s1-liveedit/` + `REPORT.md` | 见 01 文档 §6 Go 标准 | 3 |
| S2 渲染服务原型 | rc | `spikes/s2-render/` + `REPORT.md` | 同上 | 3 |
| S3 AST 适配原型 | rc | `spikes/s3-ast/` + `REPORT.md` | 同上 | 3 |
| S4 外观性能原型 | — | `spikes/s4-look/` + `REPORT.md`（含与基线的对照截图、字体方案） | 同上 | 4 |
| **M0 阶段门** | 以上全部 | `docs/adr/ADR-001..` 记录 Spike 结论 | 见 §7 | — |

### Phase 1 · 核心编辑闭环（约 3 周）

**BE-1**
| 卡 | 依赖 | 交付物 | 验收标准 | 估时 |
|---|---|---|---|---|
| BE-1.1 Text | M0 | `core/text` | 通过 `contracttest.TestText`；行索引惰性构建；1 MB 文档 `LineOf` O(log n) | 1 |
| BE-1.2 Markdown 适配 | 1.1 | `core/markdown`：AST 实现、Front Matter 预处理、`Content/Markers/Prefixes`、Escape/Entity 内联、`Open/Close`、BlockID 稳定、大纲/slug、脚注表、链接定义、增量更新 | 语料上前缀不变式 100%；随机编辑后增量结果与全量解析等价（块结构、范围、内联）；未触及块 ID 不变；slug 与 GitHub 规则一致（测试表） | 5 |
| BE-1.3 Projection | 1.2 | `core/projection`：`RevealInline/Block/Always/None`、软换行模式、容器装饰、对象片段（图片/复选框/脚注）、`PosMapper` | 04 文档 §4.3 不变式属性测试；黄金测试 ≥ 150 例（源码 + 光标 → 显示串 + 片段表） | 5 |
| BE-1.4 Document | 1.1、1.2 | `core/document`：Apply/历史/合并/选区映射/事件/只读/Replace、`DocumentFactory` | contracttest 通过；1 万步随机事务属性测试；撤销 1000 步内存上限测试 | 4 |
| BE-1.5 Editing v1 | 1.2 | `core/editing`：基础意图、列表/引用/标题/代码块内的 Enter/Backspace/Tab 规则、自动配对、内联闭合快捷、块快捷（`# `、`- `、`1. `、`[ ] `、`> `、```` ``` ````+Enter、`---`+Enter）、格式命令（strong/emphasis/strike/code/underline/link/clear）、块命令（heading/paragraph/quote/三种列表/task 切换/indent/outdent/code/hr/moveUp/moveDown）、`Formatting` | 每条规则 ≥ 3 例黄金测试（含 CRLF 文档、嵌套列表、CJK）；最小改动断言（未涉及字节不变） | 6 |

**BE-2**
| 卡 | 依赖 | 交付物 | 验收标准 | 估时 |
|---|---|---|---|---|
| BE-2.1 FileStore | M0 | `core/fsstore` | contracttest 通过；BOM/UTF-16/二进制/超大检测；原子写（Windows `ReplaceFileW`、Unix rename 保权限）；`expect` 冲突；故障注入（写一半失败）不损坏原文件；语料"读→写"字节一致 100% | 3 |
| BE-2.2 Settings | M0 | `core/settings` | 默认值表、原子写盘、版本迁移框架、损坏文件回退默认并备份 | 1.5 |
| BE-2.3 Commands | M0 | `core/commands`：注册表、平台默认键位表（02 文档 §7）、`Chord` 解析/显示（`⌘⇧P` / `Ctrl+Shift+P`）、用户覆盖、冲突检测 | 默认键位零冲突测试；三平台显示串测试 | 2 |
| BE-2.4 DocSession v1 | 2.1（用 mock 工厂，BE-1.4 合并后切换） | `core/docsession`：Open 去重、New、Save/SaveAs/Revert、编辑态、自动保存、恢复副本与启动恢复、关闭决策 | contracttest 通过；"kill 进程→重启恢复"集成测试（QA 协助）；自动保存不写冲突/只读/未命名 | 4 |

**FE-2**
| 卡 | 依赖 | 交付物 | 验收标准 | 估时 |
|---|---|---|---|---|
| FE-2.1 Theme + Kit | M0、S4 | `views/theme`（02 文档 §4 全部令牌，Paper/Midnight Ink/高对比，色相推导）、`views/motion`（令牌、按压回弹、Match 键）、`views/effects`（三级材质、窗口材质、极光、低功耗切换）、`views/kit`（GlassPanel、SlidingPill、IconButton、KeyHint、MenuList、Banner、Capsule、SegmentedTabs、Toggle、Notice、Skeleton、EmptyState）、字体注册、图标集与品牌标记。**FE-1 依赖此卡；先合入令牌与 GlassPanel，不等全部组件** | 组件 Tester 测试；亮/暗截图黄金（Windows）；`ReduceMotion`、低功耗分支有测试；`DesignSystem`、`LowPower` 基线对照 | 6 |
| FE-2.2 Shell v1 | 2.1 | `views/shell`：窗口材质 + 浮动纸张、自绘标题栏（`TitleBarHidden` + `DragWindow`，含 omnibox、视图菜单、窗口按钮）、标签条（滑动药丸、拖拽排序、未保存点、溢出滚动）、侧栏容器（Split、Spring 折叠、三段切换）、状态胶囊（进度环、保存状态动画、悬停展开）、横幅区、欢迎页（静态）；`Notifier` 实现（栈、FLIP 补位） | 基于 mock 可演示全部布局状态；键盘可达；窗口缩放到 640×480 不破版；`Main`（外壳部分）、`Dark`、`Welcome`、`Notify` 基线对照 | 6 |

**FE-1**
| 卡 | 依赖 | 交付物 | 验收标准 | 估时 |
|---|---|---|---|---|
| FE-1.1 编辑引擎核心 | M0、S1 | `views/liveedit`：Surface 元素、Fenwick 高度索引、滚动锚定、投影/排版双层缓存、绘制（文本/光标/选区）、键盘导航（字/词/行/上下 sticky-x/Home/End/Page）、指针选择（单/双/三击、拖选、Shift 扩展）、`TextInputClient`（IME）、意图映射到 `Editing`、撤销重做、只读 | Tester：键入/IME 组字提交/方向键跨块/选区/撤销共 ≥ 60 例；1 万块文档滚动时排版计数 ≤ 可见块 × 2；≤200 KB 文档击键帧 p95 < 8 ms | 10 |
| FE-1.2 块视觉 v1 | 1.1、FE-2.1 | 02 §4.2 排版（衬线 H1/H2、导语）、活动块页边标签、悬挂引号、列表符号/编号/复选框（点击切换，Spring + 绘制对勾）、代码块（头部、行号、`editor` Tokenizer 高亮、复制反馈）、分节符「···」、链接（悬停地址、Ctrl/⌘+单击打开）、图片（本地加载、内联对象、宽度适配、加载失败占位）、内联样式 | 亮/暗截图黄金；复选框点击只改 1 字节（断言）；`Main`、`Dark` 正文基线对照 | 7 |
| FE-1.3 源码模式 | 1.1 | `views/source`：`ui/editor` 视图 + 镜像 Buffer + 版本比对求差写回、模式切换保持光标与滚动、行号栏与活动行样式 | 双向切换 100 次后文本一致；源码模式编辑后切回，撤销可撤销源码模式中的修改；`Source` 基线对照 | 3 |

**INT / QA**
| 卡 | 依赖 | 交付物 | 验收标准 | 估时 |
|---|---|---|---|---|
| INT-1.1 组合根 v1 | FE-2.2、FE-1.1、BE-2.3 | `internal/app`：服务构造与注入（mock ↔ 真实可切换）、`Dispatcher`、主窗口、菜单（文件/编辑/格式/段落/视图/窗口/帮助，全部映射命令注册表）、打开对话框/命令行参数/文件关联、单实例 | 双击 .md 文件在已运行实例中打开；菜单项启用状态随上下文变化 | 4 |
| QA-1.1 保真套件 | BE-2.1、BE-1.4 | 语料"打开→保存"字节一致；"打开→随机编辑→保存"与字符串模拟一致 | 100% 通过，失败用例自动最小化输出 | 2 |
| QA-1.2 编辑器 UI 测试 | FE-1.1 | `tests/ui`：IME、撤销、复制粘贴（`tt.Clipboard`）、模式切换场景 | ≥ 40 场景；零基于时间的等待 | 3 |
| QA-1.3 性能基线 | FE-1.1 | `tests/perf`：`BenchmarkTypeFrame`（200 KB/1 MB）、`BenchmarkScroll`（1 万块）、`BenchmarkOpen`；结果写入 `reports/quality.md` | 基线建立；以排版次数/分配次数做断言 | 2 |
| **M1 阶段门** | 以上 | — | 见 §7 | — |

### Phase 2 · 工作区与效率（约 3 周）

| 卡 | 依赖 | 交付物 | 验收标准 | 估时 |
|---|---|---|---|---|
| BE-2.5 Workspace | M1 | 根目录、懒加载、过滤/忽略、递归监听（超限降级）、增删改名移动/回收站、`Assets`、`Recent` | contracttest；临时目录集成测试（外部创建/删除/改名被正确报告） | 4 |
| BE-2.6 Search | M1 | `Finder`（字面/正则/大小写/全词）、`WorkspaceSearch`（并发、可取消、分批）、`Fuzzy`（fzf 式打分） | 正确性表驱动测试；1 MB 文档查找 < 5 ms（计数/基准）；1 万文件目录搜索可取消 | 3 |
| BE-2.7 DocSession v2 | 2.5 | 外部修改：未编辑自动重载、已编辑冲突、删除、同目录改名跟随；获焦复查；会话恢复（标签、光标、滚动） | 冲突场景矩阵测试 ≥ 12 例 | 3 |
| BE-2.8 导出 HTML | BE-1.2 | `core/export` HTML：标题锚点、代码高亮 class、脚注、TOC、与 02 §4 一致的主题 CSS、图片内嵌可选、`Fragment`、`OnProgress` | 与 `parser.HTML()` 结构对照测试；HTML 校验（无未闭合标签） | 3 |
| BE-1.6 Editing v2 | M1 | 表格（插入、单元格导航、增删行列、对齐、格式化）、粘贴（HTML/RTF→MD 经 PasteConverter、URL 粘贴到选区成链接、纯文本）、插入图片/链接/脚注；`adapters/richpaste` | 表格操作不改动表格外字节；粘贴 Word/网页/Google Docs 样本黄金测试 | 6 |
| FE-1.4 块视觉 v2 | M1 | 表格（圆角容器、活动单元格环、玻璃工具条）、脚注上标与悬停预览、链接弹层（编辑/打开）、HTML 块淡色源码、结构导轨（刻度、视口滑块、查找命中） | Tester + 截图黄金；`Find` 导轨部分基线对照 | 7 |
| FE-1.5 剪贴板与拖放 | 1.4、BE-1.6 | 复制三格式（Markdown/纯文本/HTML Fragment）、粘贴、拖入图片（经 `Assets`）、`editorapi.Controller` 全部方法 | 复制→粘贴到自身往返一致 | 3 |
| FE-2.3 侧栏面板 | M1、BE-2.5 | 文件树（Outline、图标、FLIP 展开、拖拽上浮与落点高亮、撤销通知、行内重命名与同名提示、右键菜单）、大纲（scroll spy、点击平滑滚动）、搜索面板（流式结果、高亮、点击定位） | Tester；1 万条目文件树滚动流畅（只构建可见行） | 6 |
| FE-2.4 命令面板/快速打开 | M1、BE-2.6 | `views/palette`：`>` 命令、`@` 标题、文件、最近；模式徽标；匹配高亮；KeyHint；滑动药丸；从 omnibox 长出并返回；打开时飞入标签；一次性辉光边框 | 键盘全流程测试；500 命令 + 1 万文件下输入响应在一帧内；`Palette` 基线对照 | 5 |
| FE-2.5 查找替换栏与横幅 | FE-1.5 | 玻璃查找栏（基于 `editorapi`，选项、计数、替换展开）、冲突/只读/删除/大文件横幅、胶囊冲突态 | 与 BE-2.7 冲突场景联调通过；`Find`、`Conflict` 基线对照 | 3 |
| INT-2.1 工作区接线 | 以上 | 标签/会话恢复接线、最近菜单、导出 HTML 命令、窗口状态记忆 | E2E：打开文件夹→打开文件→编辑→自动保存→重启恢复 | 3 |
| QA-2.x | 以上 | 工作区/监听/冲突测试、E2E 冒烟（automation 插件）、面板 UI 测试、崩溃恢复测试 | 结果入 `reports/quality.md` | 6 |
| **M2 阶段门** | 以上 | — | 见 §7 | — |

### Phase 3 · 高级 Markdown 与体验（约 3 周）

| 卡 | 依赖 | 交付物 | 验收标准 | 估时 |
|---|---|---|---|---|
| BE-2.9 渲染服务 | M2、S2 | `core/render` + `adapters/webviewhost` + `resources/render`：懒启动、串行队列、超时、内存/磁盘缓存、亮暗主题、不可用降级 | 公式语料 ≥ 200 例 SVG 可被 `ui.ParseSVG` 解析；Mermaid 6 种图类型；缓存命中 < 1 ms | 5 |
| BE-2.10 PDF/打印 | 2.8、2.9 | PDF 导出编排；INT 实现 `Printer` | CJK、代码块、公式、图片在 PDF 中正确 | 2 |
| BE-1.7 扩展语法 | M2 | Front Matter、`==` `^` `~`（可选）、emoji 短码、`[[ ]]`、`[TOC]`、`$…$` 的解析与投影规则及编辑命令 | 每扩展开/关两种模式黄金测试；关闭时与 GFM 完全一致 | 5 |
| FE-1.6 富组件 | BE-1.7、BE-2.9 | 行内/块公式（对象 + 源码面板 + 实时预览、加载扫光）、Mermaid 块（加载与淡入）、TOC 块、Front Matter 编辑式页眉、高亮/上下标/emoji/wiki 链接样式与跳转 | 截图黄金；渲染失败与不可用态 | 6 |
| FE-1.7 模式与交互 | M2 | 阅读/专注/打字机模式的正文部分（02 §5.10）、斜杠菜单（双栏、15 项、预览）、选区格式条（转换菜单、变形为链接输入）、平滑光标与辉光、图片灯箱（Shared Element、缩放 60–160%） | Tester；`ReduceMotion` 下无位移动画；`Reading`、`Focus`、`Slash`、`Format`、`Lightbox` 基线对照 | 8 |
| FE-2.6 动效与特效 | M2 | 主题径向溶解、欢迎页与空文档极光 Shader、滚动联动标题、阅读模式外壳（纸张铺满、极光进度条、退出按钮）、专注模式外壳、文档骨架屏、ScrollEdge、02 §4.4 运动规则逐条走查 | 空闲 0 帧断言（`FramePending`），极光 12 s 后停止；低功耗路径截图；`Motion`、`Reading`、`Focus`、`Welcome`、`Loading` 基线对照 | 6 |
| FE-2.7 设置与对话框 | M2 | 设置界面（分类药丸、主题卡片、强调色色块、实时生效、Spring 开关；全部 `Settings` 字段）、导出对话框与进度通知、关于；中英文文案 | `mygo i18n check` 通过；切换语言无截断（en-XA 伪本地化测试）；`Settings`、`Notify`（进度）基线对照 | 5 |
| INT-3.1 集成 | 以上 | WebView 宿主注入、`Printer`、图标与安装包配置 | 三平台构建；Windows 安装包可安装/卸载/文件关联 | 3 |
| QA-3.x | 以上 | 渲染与扩展测试、截图黄金、键盘无障碍走查、i18n 检查、性能全套、对 `reports/parity` 做独立复核（重新截图，核对 `docs/design/README.md` 对照表） | 报告 | 7 |
| **M3 阶段门** | 以上 | — | 见 §7 | — |

### Phase 4 · 加固与发布（约 2 周）

| 卡 | 交付物 | 验收标准 |
|---|---|---|
| BE-1.8 大文档 | 大文件模式支持；按 S3 结论实现分片解析（若可行） | 1 MB 击键 p95 < 16 ms；5 MB 可打开、源码模式编辑流畅 |
| FE-1.8 性能与无障碍 | 分配优化、可访问性标签、HTML 块安全子集渲染（P2） | 性能预算全部达标 |
| FE-2.8 主题与键位 | 自定义主题 JSON、快捷键查看/修改（P2） | 测试 |
| BE-2.11 编码与更新 | GB18030 只读打开（P2）、updater 插件接入（可选） | 测试 |
| INT-4.1 发布 | `mygo build` 三平台产物、签名配置、CI 发布流程、用户文档 | 产物可安装运行；macOS 需在 Mac/CI 上构建签名 |
| QA-4.1 跨平台回归 | macOS/Linux 手工验收报告（IME、字体、Vibrancy、菜单、文件关联）、回归全套 | 无 P0/P1 缺陷 |
| **M4 最终验收** | — | 见 §8 |

### 6.1 关键依赖路径
```
INT-0.1 → INT-0.2 → QA-0.1 ─┐
S1/S2/S3/S4 ────────────────┴→ M0 → BE-1.1 → BE-1.2 → BE-1.3 ─┐
                                        │          └→ BE-1.4 ────────┤
                                        └→ FE-1.1（先用 mock）───────┴→ FE-1.2 → M1
```
关键路径为 **BE-1.2 → BE-1.3 → FE-1.1 集成**。FE-1.1 前半段使用 `mock.Projector`（显示源码）与 `mock.Document` 开发，BE-1.3/1.4 完成后切换。

## 7. 阶段门验收标准

| 门 | 必须满足（任何一项不满足即 FAIL，退回整改） |
|---|---|
| **M0** | 门禁 Windows 通过、三平台交叉编译通过；CR-001、CR-002 已落地；S1–S4 报告齐全并有 Go/No-Go 结论，No-Go 项的后备方案由用户确认；S4 含用户确认的基线观感差异与字体预算；ADR 记录 |
| **M1** | 用菜单/命令行/双击打开 .md → P0 语法集正确渲染 → 光标处显隐编辑 → 保存；源码模式往返；语料打开/保存字节一致 100%；随机编辑属性测试通过；Windows 微软拼音手工验收通过（候选框位置、提交、撤销为一步）；撤销恢复选区；kill 进程后恢复未保存内容；≤200 KB 击键 p95 < 8 ms；1 万块滚动无整屏重排；对照表中阶段门为 M1 的基线全部通过 |
| **M2** | 文件夹工作区全流程（树操作、监听、冲突、回收站）；标签与会话恢复；快速打开/命令面板/查找替换/大纲/全文搜索；表格编辑、图片粘贴拖入、脚注；导出 HTML；E2E 冒烟通过；阶段门为 M2 的基线全部通过 |
| **M3** | P1 全部功能；渲染服务可用与降级两态；PDF 导出；02 §4.4 运动规则逐条走查通过且 `ReduceMotion`/`ReduceTransparency` 合规；空闲 0 帧；中英文完整；全部 18 张基线通过，用户在真机确认观感 |
| **M4** | 见 §8 |

## 8. 最终验收清单（输出 PASS / FAIL，用户确认）

1. **需求覆盖**：02 文档 P0、P1 每项 → 对应测试或演示路径 → 状态；P0 必须 100%，P1 未完成项需用户书面同意。
2. **架构一致性**：`tests/arch` 通过；契约与 04 文档一致；`mygo vet` 无告警；无跨层 import。
3. **代码质量**：`gofmt`/`vet` 清洁；覆盖率达标；无未登记 TODO；无吞掉的错误（抽查 + 静态检查）；核心路径无 panic。
4. **无损**：语料保真 100%；属性测试 1 万步 × 多种子通过。
5. **性能**：03 文档 §10 预算表逐项实测达标（附数据）。
6. **可靠性**：原子写故障注入、崩溃恢复、冲突矩阵测试通过。
7. **体验**：`docs/design/README.md` 全部基线对照通过，差异均有 ADR 记录；UX 走查清单（Paper/Midnight Ink/高对比、CJK IME、键盘全流程、动效与降级、窗口尺寸）通过。
8. **跨平台**：三平台构建产物；macOS/Linux 手工验收报告（未验证项必须明确标注为"未验证"，不得计为通过）。
9. **遗留风险**：已知限制与风险清单及后续建议。

验收结论只基于实际运行的证据（测试输出、基准数据、截图、手工验收记录）；**未经验证的功能不标记为通过**。
