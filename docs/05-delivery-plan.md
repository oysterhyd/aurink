# 05 · 交付计划：Worktree 分工、任务拆分与验收

## 1. 角色

| 代号 | 角色 | 负责范围（详见 §3 归属表） |
|---|---|---|
| **CEO** | 本文档作者（Claude Code） | 规划、契约审批、CR 审批、阶段门评审、最终验收。不写产品代码 |
| **INT** | 集成 / 架构工程师 | 仓库脚手架、契约落地、门禁脚本与 CI、`internal/app` 组合根、菜单与窗口、WebView 宿主与 Printer、i18n 合并、打包发布、分支合并 |
| **BE-1** | 后端 · 文档核心 | `core/text` `core/markdown` `core/projection` `core/document` `core/editing` `adapters/richpaste` |
| **BE-2** | 后端 · 平台服务 | `core/fsstore` `core/docsession` `core/workspace` `core/search` `core/export` `core/render` `core/settings` `core/commands` `adapters/webviewhost` `resources/render` |
| **FE-1** | 前端 · 编辑引擎 | `views/liveedit` `views/source` `views/editorapi`（实现） |
| **FE-2** | 前端 · 外壳与设计系统 | `views/theme` `views/motion` `views/effects` `views/kit` `views/shell` `views/palette` `views/panels` `resources/icons` `resources/fonts` |
| **QA** | 测试 | `internal/mock` `internal/contracttest` `testdata/corpus` `tests/**` `reports/quality.md` |

> 若可用 Agent 少于 6 个：INT 可由用户本人或 FE-2 兼任；QA 不建议与开发角色合并（独立验证是验收前提）。

## 2. 仓库与 Worktree 布局

```
D:\mygo\project\
├─ Aurink_claude\                 main 分支（仅集成；INT 在此执行合并与门禁）
└─ Aurink_claude-wt\
   ├─ int\        分支 int/app
   ├─ be-core\    分支 be/core
   ├─ be-svc\     分支 be/services
   ├─ fe-editor\  分支 fe/editor
   ├─ fe-shell\   分支 fe/shell
   └─ qa\         分支 qa/tests
```

创建（Phase 0 第一天由用户执行，步骤见 `06-phase0-kickoff.md` §1；INT-0.1 合入 `main` 后各分支 `git rebase main`）：

```sh
cd /d/mygo/project/Aurink_claude
git worktree add ../Aurink_claude-wt/int       -b int/app      main
git worktree add ../Aurink_claude-wt/be-core   -b be/core      main
git worktree add ../Aurink_claude-wt/be-svc    -b be/services  main
git worktree add ../Aurink_claude-wt/fe-editor -b fe/editor    main
git worktree add ../Aurink_claude-wt/fe-shell  -b fe/shell     main
git worktree add ../Aurink_claude-wt/qa        -b qa/tests     main
```

Spike 在各自 worktree 的 `spikes/<id>/`（独立 `go.mod`）中进行，不污染主模块。

## 3. 文件归属与冲突预防

### 3.1 归属表（只能修改自己拥有的路径）

| 路径 | 所有者 | 备注 |
|---|---|---|
| `docs/**`（含 `docs/design/**` 界面基线） | CEO | 例外：任何人可新增 `docs/cr/CR-*.md`、`docs/adr/` 由 CEO 写 |
| `internal/contract/**`、`internal/views/editorapi/**` | INT 落地，**CEO 审批** | 冻结后只能经 CR 修改 |
| `internal/core/{text,markdown,projection,document,editing}/**`、`internal/adapters/richpaste/**` | BE-1 | |
| `internal/core/{fsstore,docsession,workspace,search,export,render,settings,commands}/**`、`internal/adapters/webviewhost/**`、`resources/render/**` | BE-2 | |
| `internal/views/{liveedit,source}/**` | FE-1 | |
| `internal/views/{theme,motion,effects,kit,shell,palette,panels}/**`、`resources/{icons,fonts}/**` | FE-2 | |
| `internal/mock/**`、`internal/contracttest/**`、`testdata/**`、`tests/**` | QA | 各包自己的 `_test.go` 归包所有者 |
| `main.go`、`internal/app/**`、`internal/adapters/{watch,clipboard}/**`、`mygo.json`、`scripts/**`、`tools/**`、`.github/**`、`.agents/**`、`.gitignore`、`.gitattributes`、`locales/**`（生成物） | INT | |
| `i18n/fragments/<代号小写>.{en,zh-Hans}.json` | 各所有者各一对 | 键名规则见 §3.4，如 `shell_tabs_close` |
| `spikes/<id>/**` | Spike 负责人 | |
| `reports/<agent>.md` | 各 Agent | 每日进度 |
| `reports/parity/<基线名>.png` | 该基线的负责人（`docs/design/README.md`） | Native 对照截图；同名基线由两人共担时，各加后缀 `-fe1`、`-fe2` |
| `go.mod` / `go.sum` | 任何人可新增依赖 | 冲突由 INT 以 `go mod tidy` 解决；新增第三方依赖须 CEO 批准（§3.3） |

### 3.2 共享点的处理
- **组装代码**：只有 INT 修改 `internal/app`。其他 Agent 需要接线时，在合并说明中写明"需要 INT 注入 X"。
- **命令注册**：各模块在自己包内提供 `func Commands(deps…) []contract.Command`，由 INT 在 app 中统一注册——不存在共享的命令清单文件。
- **i18n**：每个所有者只写自己的分片文件；`tools/i18nmerge` 合并生成 `locales/{en,zh-Hans}.json`（INT 运行，CI 检查生成物最新）。
- **测试数据**：只有 QA 写 `testdata/corpus`；开发者的包内测试数据放在包内 `testdata/`。

### 3.3 第三方依赖白名单（已预审，可直接使用）
纯 Go、无 cgo、许可证 MIT/BSD/Apache/ISC/OFL：

| 依赖 | 用途 | 使用方 |
|---|---|---|
| `github.com/egoist/mygo v0.7.3`（锁定） | 框架 | 全部 |
| `golang.org/x/text` | 编码转换、Unicode | BE-1、BE-2 |
| `github.com/rivo/uniseg` | 字素/词边界 | BE-1、FE-1 |
| `gopkg.in/yaml.v3` | Front Matter 页眉 | BE-1 |
| MathJax 3（Apache-2.0）、Mermaid（MIT）离线包 | 渲染服务 | BE-2 |
| Newsreader、Noto Serif SC（子集）、JetBrains Mono（OFL） | 内置字体，许可证随 `resources/fonts` 分发 | FE-2 |
| Lucide 图标（ISC） | 图标 SVG | FE-2 |
| gemoji 数据（MIT） | emoji 短码表（生成 Go 表） | BE-1 |

白名单外的依赖须提 CR。MyGo 版本升级只由 INT 在独立分支执行（`go tool mygo migrate-ui`），CEO 批准后合并。

### 3.4 i18n 文案键规则（2026-11-01 CEO 决定，回应 INT-0.1 报告）

MyGo 对键名的约束 [源码 `internal/i18ngen/load.go:180`、MyGo `docs/i18n.md`]：
- 键必须匹配 `^[A-Za-z][A-Za-z0-9_]*$`；点号只留给 MyGo 自带的 `mygo.<key>`。
- 生成的 Go 函数名 = 键按 `_` 切分后各段首字母大写再拼接（`goName`）。两个键生成同名函数时，`mygo generate`、`mygo build`、`mygo i18n check` 都会报错。

本项目在此基础上收紧为：

| # | 规则 | 例 |
|---|---|---|
| K1 | 键全小写蛇形：`^[a-z][a-z0-9]*(_[a-z][a-z0-9]*)+$`。至少两段，每段以字母开头，不出现连续、开头或结尾的 `_`。这样生成的函数名里大写字母正好标出段边界，任意两个合法键都不会生成同名函数 | `shell_tabs_close` → `locales.ShellTabsClose()`；写 `liveedit_heading1`，不写 `liveedit_heading_1`（两者都会生成 `LiveeditHeading1`） |
| K2 | 第一段 = 所有者的 Go 包名（§3.1）：INT 用 `app` `watch` `clipboard`；BE-1 用 `text` `markdown` `projection` `document` `editing` `richpaste`；BE-2 用 `fsstore` `docsession` `workspace` `search` `export` `render` `settings` `commands` `webviewhost`；FE-1 用 `liveedit` `source`；FE-2 用 `theme` `motion` `effects` `kit` `shell` `palette` `panels` | `palette_empty_results`、`docsession_conflict_title` |
| K3 | 第二段起依次是区域、条目，从大到小。命令标题建议写成 `<包名>_cmd_<CommandID 转写>`（`.` 和驼峰边界都换成 `_`），分类写成 `<包名>_category_<名>` | 注册 `file.openFolder` 的包是 shell 时，标题键为 `shell_cmd_file_open_folder` |
| K4 | 只有 INT 的分片可以含 `mygo.<key>`，而且只能用来覆盖 MyGo 已有的文案 | `mygo.appName`、`mygo.appDescription`（打包时写入系统应用列表） |
| K5 | 占位符名用小驼峰 `^[a-z][A-Za-z0-9]*$`，避开 Go 关键字 | `{fileName}`、`{count}` |
| K6 | 每个所有者一对分片：`i18n/fragments/<代号小写>.en.json` 与 `.zh-Hans.json`（`int`、`be-1`、`be-2`、`fe-1`、`fe-2`）。同一所有者的两种语言键集合相同、占位符相同；英文分片的每个键都要有 `@<键>.description`，写明文案出现在哪 | `i18n/fragments/fe-2.en.json` |

落地与使用：
- **检查**：`tools/i18nmerge -check`（INT）检查 K1–K6，并检查同一个键不出现在两个分片里。
- **合并生成**：i18nmerge 生成 `locales/en.json`、`locales/zh-Hans.json`（按键排序），重新生成 `locales/catalog_gen.go`，要求已提交的生成物与重新生成的结果一致，最后运行 `go tool mygo i18n check`。
- **在代码里取文案**：
  - 静态文案直接调用生成的函数，如 `locales.ShellTabsClose()`；
  - 契约字段里传的是键（`Command.Title`、`Command.Category`、`Transaction.Label`），视图用 `locales.Catalog.Format(i18n.Locale(), key, args)` 取文本 [源码 `locales/catalog_gen.go`、MyGo `i18n/catalog.go`]。
- **现有键 `appName`**：改名为 `app_name`，生成函数仍是 `AppName()`，调用方不用改；i18nmerge 出现后移入 `i18n/fragments/int.*.json`。
- **契约注释**：`internal/contract/document.go` 中的示例 `"undo.typing"` 改为 `"editing_undo_typing"`。这只改注释、不改语义，由 INT 在冻结 `contract-v1.0` 时一并落地；`spikes/01-contract-rc` 作为历史记录保持原样。

## 4. 契约冻结、Mock 与变更流程

### 4.1 时间线
| 时间 | 事件 |
|---|---|
| D0 | INT 将 `spikes/01-contract-rc` 落地为 `internal/contract`、`internal/views/editorapi`；标签 `contract-v1.0-rc` |
| D0–D3 | 四个 Spike 基于 rc 进行，发现的问题以 CR 提交 |
| D1–D3 | QA 交付 `internal/mock` 与 `internal/contracttest` 骨架 |
| D4 | **CEO 阶段门评审**：Spike Go/No-Go、CR 处理 → 标签 `contract-v1.0`（冻结） |
| 之后 | 只允许 CR 驱动的次版本变更；破坏性变更仅在阶段边界 |

### 4.2 变更申请（CR）
1. 提出者新建 `docs/cr/CR-<序号>-<短名>.md`：动机、变更内容（Go diff）、兼容级别（次/主）、受影响模块、mock 与一致性测试需要的改动。
2. CEO 一个工作日内批复（批准/修改后批准/拒绝），写回该文件。
3. 批准后由 INT 在**一次合并**中提交：契约代码 + mock（QA 提供补丁）+ contracttest + `04-contracts.md` 更新 → 打标签 `contract-v1.<n>` → 在各 `reports/` 中通知。
4. 等待期间，提出者在自己包内用私有适配类型过渡，**禁止**直接修改契约。

### 4.3 Mock 使用规则
- 前端在对应后端实现合并前，一律通过 `internal/app` 注入 mock 开发与演示；`views/**` 不得直接 import `internal/mock`（测试文件除外）。
- 每个真实实现合并的前提：通过对应 `contracttest` 套件。

## 5. 分支、门禁与合并

### 5.1 门禁脚本 `scripts/gate.ps1` / `gate.sh`（INT 在 Phase 0 交付）
1. `gofmt -l .` 输出为空
2. `go vet ./...`，并分别以 `GOOS=windows/linux/darwin` 执行；`go vet -tags mygo_noinspector ./...`
3. `go build ./...`：windows/amd64、linux/amd64、darwin/arm64（`CGO_ENABLED=0`）
4. `go tool mygo vet .`（UI 构建生命周期检查）
5. `CGO_ENABLED=0 go test ./...`（含 `tests/arch` 导入规则、`contracttest`、语料保真）
6. `go run ./tools/i18nmerge -check`
7. 夜间（CI）：E2E 冒烟（automation 插件）、性能基准（计数断言）、三平台矩阵

### 5.2 合并流程
1. Agent 在自己分支完成一张任务卡 → `git fetch && git rebase main` → 本地运行门禁 → 提交。
2. 在 `reports/<agent>.md` 写合并请求：任务卡 ID、变更摘要、新增测试、门禁结果、需要 INT 接线的内容、是否涉及 CR。
3. INT 在 main worktree：`git merge --no-ff --no-commit <branch>` → 运行门禁 → 通过则提交（提交信息含任务卡 ID），失败则 `git merge --abort` 并把失败日志写回该 Agent 的 report。
4. 同日多个合并的顺序：`contract`（CR）→ `be/*` → `fe/*` → `int/app` → `qa/tests`（测试分支可随时合并）。
5. 每次合并后其他 Agent 当日内 rebase。
6. 远端为私有仓库 `github.com/oysterhyd/aurink`（`origin`）。只有 INT 推送 `main`：本地门禁通过后推送；`main` 上的三平台 CI 失败视为阻断，INT 当日修复或 `git revert`。Agent 可推送自己的分支并开 PR，以在合并前得到 macOS/Linux 结果（消耗 Actions 分钟，macOS 按 10 倍计费）；合并权限仍在 INT/用户。

### 5.3 提交规范
- 小步提交，一个任务卡对应一个或少量合并；提交信息前缀为任务卡 ID，如 `BE-1.2: compute block prefixes`。
- 禁止提交：构建产物、`.mygo/`、`build/`、`dist/`、个人编辑器配置、密钥。

## 6. 分阶段计划与任务卡

工期为单 Agent 估算（工作日），并行后的日历周期见各阶段标题。所有任务的通用完成定义（DoD）：
- 通过门禁；新增代码有测试（core 包语句覆盖率 ≥ 80%，`projection`/`editing`/`document` ≥ 90%）；
- 遵守线程规则与 `mygo vet`；公开标识符有 godoc；
- 无未登记的 TODO（格式 `TODO(任务卡ID): …`）；
- 在 `reports/<agent>.md` 记录完成情况。

### Phase 0 · 地基与契约冻结（约 1 周）

| 卡 | 负责 | 依赖 | 交付物 | 验收标准 | 估时 |
|---|---|---|---|---|---|
| INT-0.1 仓库脚手架 | INT | — | （已完成：`git init -b main`、`.gitignore`、`.gitattributes`、私有仓库 `oysterhyd/aurink`、基线 CI；MyGo 技能与 worktree 由用户按 06 §1 创建）`go.mod`（模块 `aurink`）；`mygo.json`（name Aurink、identifier `app.aurink.editor`、i18n、fileAssociations md/markdown）；`main.go`；§11 架构目录骨架（每包 `doc.go`）；`scripts/gate.*`；把 `ci.yml` 的内联步骤改为调用 `scripts/gate.sh`、`setup-go` 改用 `go-version-file: go.mod`；`reports/` 模板 | 门禁在 Windows 通过；三平台交叉编译通过；`go tool mygo dev` 能启动空窗口 | 1.5 |
| INT-0.2 契约落地 | INT | 0.1 | `internal/contract/*`、`internal/views/editorapi/*`（复制 rc，改 import 路径）；"contract 只依赖标准库"由门禁/CI 检查（`ci.yml` 已含），QA-0.2 再写入 `tests/arch` | 编译通过；与 rc 逐字一致（除 import）；标签 `contract-v1.0-rc` | 0.5 |
| QA-0.1 Mock 与一致性套件 | QA | INT-0.2 | `internal/mock/*`（§7 of 04 全部）、`internal/contracttest/*` 骨架（每接口至少覆盖 04 文档列出的不变式） | 全部 mock 通过 contracttest；FE 可用 mock 组装一个可运行窗口（由 INT 演示） | 3 |
| QA-0.2 架构规则与语料 | QA | INT-0.1 | `tests/arch`：03 文档 §1.1 依赖规则表全部条目；`testdata/corpus`：CommonMark/GFM 规范示例（可从 MyGo `ui/markdown/parser/testdata` 获取）、CJK、CRLF、BOM、混合换行、无尾换行、Front Matter、真实 README 样本 ≥ 50、生成器（1 MB / 5 MB） | 规则测试能检出故意违规；语料清单 `testdata/corpus/INDEX.md` | 2 |
| S1 编辑引擎原型 | FE-1 | rc | `spikes/s1-liveedit/` + `REPORT.md` | 见 01 文档 §6 Go 标准 | 3 |
| S2 渲染服务原型 | BE-2 | rc | `spikes/s2-render/` + `REPORT.md` | 同上 | 3 |
| S3 AST 适配原型 | BE-1 | rc | `spikes/s3-ast/` + `REPORT.md` | 同上 | 3 |
| S4 外观性能原型 | FE-2 | — | `spikes/s4-look/` + `REPORT.md`（含与基线的对照截图、字体方案） | 同上 | 4 |
| **M0 阶段门** | CEO | 以上全部 | `docs/adr/ADR-001..` 记录 Spike 结论；`contract-v1.0` | 见 §7 | — |

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
| 卡 | 负责 | 依赖 | 交付物 | 验收标准 | 估时 |
|---|---|---|---|---|---|
| INT-1.1 组合根 v1 | INT | FE-2.2、FE-1.1、BE-2.3 | `internal/app`：服务构造与注入（mock ↔ 真实可切换）、`Dispatcher`、主窗口、菜单（文件/编辑/格式/段落/视图/窗口/帮助，全部映射命令注册表）、打开对话框/命令行参数/文件关联、单实例 | 双击 .md 文件在已运行实例中打开；菜单项启用状态随上下文变化 | 4 |
| QA-1.1 保真套件 | QA | BE-2.1、BE-1.4 | 语料"打开→保存"字节一致；"打开→随机编辑→保存"与字符串模拟一致 | 100% 通过，失败用例自动最小化输出 | 2 |
| QA-1.2 编辑器 UI 测试 | QA | FE-1.1 | `tests/ui`：IME、撤销、复制粘贴（`tt.Clipboard`）、模式切换场景 | ≥ 40 场景；零基于时间的等待 | 3 |
| QA-1.3 性能基线 | QA | FE-1.1 | `tests/perf`：`BenchmarkTypeFrame`（200 KB/1 MB）、`BenchmarkScroll`（1 万块）、`BenchmarkOpen`；结果写入 `reports/quality.md` | 基线建立；以排版次数/分配次数做断言 | 2 |
| **M1 阶段门** | CEO | 以上 | — | 见 §7 | — |

### Phase 2 · 工作区与效率（约 3 周）

| 卡 | 负责 | 依赖 | 交付物 | 验收标准 | 估时 |
|---|---|---|---|---|---|
| BE-2.5 Workspace | BE-2 | M1 | 根目录、懒加载、过滤/忽略、递归监听（超限降级）、增删改名移动/回收站、`Assets`、`Recent` | contracttest；临时目录集成测试（外部创建/删除/改名被正确报告） | 4 |
| BE-2.6 Search | BE-2 | M1 | `Finder`（字面/正则/大小写/全词）、`WorkspaceSearch`（并发、可取消、分批）、`Fuzzy`（fzf 式打分） | 正确性表驱动测试；1 MB 文档查找 < 5 ms（计数/基准）；1 万文件目录搜索可取消 | 3 |
| BE-2.7 DocSession v2 | BE-2 | 2.5 | 外部修改：未编辑自动重载、已编辑冲突、删除、同目录改名跟随；获焦复查；会话恢复（标签、光标、滚动） | 冲突场景矩阵测试 ≥ 12 例 | 3 |
| BE-2.8 导出 HTML | BE-2 | BE-1.2 | `core/export` HTML：标题锚点、代码高亮 class、脚注、TOC、与 02 §4 一致的主题 CSS、图片内嵌可选、`Fragment`、`OnProgress` | 与 `parser.HTML()` 结构对照测试；HTML 校验（无未闭合标签） | 3 |
| BE-1.6 Editing v2 | BE-1 | M1 | 表格（插入、单元格导航、增删行列、对齐、格式化）、粘贴（HTML/RTF→MD 经 PasteConverter、URL 粘贴到选区成链接、纯文本）、插入图片/链接/脚注；`adapters/richpaste` | 表格操作不改动表格外字节；粘贴 Word/网页/Google Docs 样本黄金测试 | 6 |
| FE-1.4 块视觉 v2 | FE-1 | M1 | 表格（圆角容器、活动单元格环、玻璃工具条）、脚注上标与悬停预览、链接弹层（编辑/打开）、HTML 块淡色源码、结构导轨（刻度、视口滑块、查找命中） | Tester + 截图黄金；`Find` 导轨部分基线对照 | 7 |
| FE-1.5 剪贴板与拖放 | FE-1 | 1.4、BE-1.6 | 复制三格式（Markdown/纯文本/HTML Fragment）、粘贴、拖入图片（经 `Assets`）、`editorapi.Controller` 全部方法 | 复制→粘贴到自身往返一致 | 3 |
| FE-2.3 侧栏面板 | FE-2 | M1、BE-2.5 | 文件树（Outline、图标、FLIP 展开、拖拽上浮与落点高亮、撤销通知、行内重命名与同名提示、右键菜单）、大纲（scroll spy、点击平滑滚动）、搜索面板（流式结果、高亮、点击定位） | Tester；1 万条目文件树滚动流畅（只构建可见行） | 6 |
| FE-2.4 命令面板/快速打开 | FE-2 | M1、BE-2.6 | `views/palette`：`>` 命令、`@` 标题、文件、最近；模式徽标；匹配高亮；KeyHint；滑动药丸；从 omnibox 长出并返回；打开时飞入标签；一次性辉光边框 | 键盘全流程测试；500 命令 + 1 万文件下输入响应在一帧内；`Palette` 基线对照 | 5 |
| FE-2.5 查找替换栏与横幅 | FE-2 | FE-1.5 | 玻璃查找栏（基于 `editorapi`，选项、计数、替换展开）、冲突/只读/删除/大文件横幅、胶囊冲突态 | 与 BE-2.7 冲突场景联调通过；`Find`、`Conflict` 基线对照 | 3 |
| INT-2.1 工作区接线 | INT | 以上 | 标签/会话恢复接线、最近菜单、导出 HTML 命令、窗口状态记忆 | E2E：打开文件夹→打开文件→编辑→自动保存→重启恢复 | 3 |
| QA-2.x | QA | 以上 | 工作区/监听/冲突测试、E2E 冒烟（automation 插件）、面板 UI 测试、崩溃恢复测试 | 结果入 `reports/quality.md` | 6 |
| **M2 阶段门** | CEO | 以上 | — | 见 §7 | — |

### Phase 3 · 高级 Markdown 与体验（约 3 周）

| 卡 | 负责 | 依赖 | 交付物 | 验收标准 | 估时 |
|---|---|---|---|---|---|
| BE-2.9 渲染服务 | BE-2 | M2、S2 | `core/render` + `adapters/webviewhost` + `resources/render`：懒启动、串行队列、超时、内存/磁盘缓存、亮暗主题、不可用降级 | 公式语料 ≥ 200 例 SVG 可被 `ui.ParseSVG` 解析；Mermaid 6 种图类型；缓存命中 < 1 ms | 5 |
| BE-2.10 PDF/打印 | BE-2 + INT | 2.8、2.9 | PDF 导出编排；INT 实现 `Printer` | CJK、代码块、公式、图片在 PDF 中正确 | 2 |
| BE-1.7 扩展语法 | BE-1 | M2 | Front Matter、`==` `^` `~`（可选）、emoji 短码、`[[ ]]`、`[TOC]`、`$…$` 的解析与投影规则及编辑命令 | 每扩展开/关两种模式黄金测试；关闭时与 GFM 完全一致 | 5 |
| FE-1.6 富组件 | FE-1 | BE-1.7、BE-2.9 | 行内/块公式（对象 + 源码面板 + 实时预览、加载扫光）、Mermaid 块（加载与淡入）、TOC 块、Front Matter 编辑式页眉、高亮/上下标/emoji/wiki 链接样式与跳转 | 截图黄金；渲染失败与不可用态 | 6 |
| FE-1.7 模式与交互 | FE-1 | M2 | 阅读/专注/打字机模式的正文部分（02 §5.10）、斜杠菜单（双栏、15 项、预览）、选区格式条（转换菜单、变形为链接输入）、平滑光标与辉光、图片灯箱（Shared Element、缩放 60–160%） | Tester；`ReduceMotion` 下无位移动画；`Reading`、`Focus`、`Slash`、`Format`、`Lightbox` 基线对照 | 8 |
| FE-2.6 动效与特效 | FE-2 | M2 | 主题径向溶解、欢迎页与空文档极光 Shader、滚动联动标题、阅读模式外壳（纸张铺满、极光进度条、退出按钮）、专注模式外壳、文档骨架屏、ScrollEdge、02 §4.4 运动规则逐条走查 | 空闲 0 帧断言（`FramePending`），极光 12 s 后停止；低功耗路径截图；`Motion`、`Reading`、`Focus`、`Welcome`、`Loading` 基线对照 | 6 |
| FE-2.7 设置与对话框 | FE-2 | M2 | 设置界面（分类药丸、主题卡片、强调色色块、实时生效、Spring 开关；全部 `Settings` 字段）、导出对话框与进度通知、关于；中英文文案 | `mygo i18n check` 通过；切换语言无截断（en-XA 伪本地化测试）；`Settings`、`Notify`（进度）基线对照 | 5 |
| INT-3.1 集成 | INT | 以上 | WebView 宿主注入、`Printer`、i18n 合并工具、图标与安装包配置 | 三平台构建；Windows 安装包可安装/卸载/文件关联 | 3 |
| QA-3.x | QA | 以上 | 渲染与扩展测试、截图黄金、键盘无障碍走查、i18n 检查、性能全套、对 `reports/parity` 做独立复核（重新截图，核对 `docs/design/README.md` 对照表） | 报告 | 7 |
| **M3 阶段门** | CEO | 以上 | — | 见 §7 | — |

### Phase 4 · 加固与发布（约 2 周）

| 卡 | 负责 | 交付物 | 验收标准 |
|---|---|---|---|
| BE-1.8 大文档 | BE-1 | 大文件模式支持；按 S3 结论实现分片解析（若可行） | 1 MB 击键 p95 < 16 ms；5 MB 可打开、源码模式编辑流畅 |
| FE-1.8 性能与无障碍 | FE-1 | 分配优化、可访问性标签、HTML 块安全子集渲染（P2） | 性能预算全部达标 |
| FE-2.8 主题与键位 | FE-2 | 自定义主题 JSON、快捷键查看/修改（P2） | 测试 |
| BE-2.11 编码与更新 | BE-2 | GB18030 只读打开（P2）、updater 插件接入（可选） | 测试 |
| INT-4.1 发布 | INT | `mygo build` 三平台产物、签名配置、CI 发布流程、用户文档 | 产物可安装运行；macOS 需在 Mac/CI 上构建签名 |
| QA-4.1 跨平台回归 | QA | macOS/Linux 手工验收报告（IME、字体、Vibrancy、菜单、文件关联）、回归全套 | 无 P0/P1 缺陷 |
| **M4 最终验收** | CEO | — | 见 §8 |

### 6.1 关键依赖路径
```
INT-0.1 → INT-0.2 → QA-0.1 ─┐
S1/S2/S3/S4 ────────────────┴→ M0(冻结) → BE-1.1 → BE-1.2 → BE-1.3 ─┐
                                        │          └→ BE-1.4 ────────┤
                                        └→ FE-1.1（先用 mock）───────┴→ FE-1.2 → M1
```
关键路径为 **BE-1.2 → BE-1.3 → FE-1.1 集成**。FE-1.1 前半段使用 `mock.Projector`（显示源码）与 `mock.Document` 开发，BE-1.3/1.4 合并后切换，不阻塞。

## 7. 阶段门验收标准

| 门 | 必须满足（任何一项不满足即 FAIL，退回整改） |
|---|---|
| **M0** | 门禁 Windows 通过、三平台交叉编译通过；`contract-v1.0` 冻结；mock 全部通过 contracttest；S1–S4 报告齐全并有 Go/No-Go 结论；S4 含用户确认的基线观感差异与字体预算；ADR 记录 |
| **M1** | 用菜单/命令行/双击打开 .md → P0 语法集正确渲染 → 光标处显隐编辑 → 保存；源码模式往返；语料打开/保存字节一致 100%；随机编辑属性测试通过；Windows 微软拼音手工验收通过（候选框位置、提交、撤销为一步）；撤销恢复选区；kill 进程后恢复未保存内容；≤200 KB 击键 p95 < 8 ms；1 万块滚动无整屏重排；对照表中阶段门为 M1 的基线全部通过 |
| **M2** | 文件夹工作区全流程（树操作、监听、冲突、回收站）；标签与会话恢复；快速打开/命令面板/查找替换/大纲/全文搜索；表格编辑、图片粘贴拖入、脚注；导出 HTML；E2E 冒烟通过；阶段门为 M2 的基线全部通过 |
| **M3** | P1 全部功能；渲染服务可用与降级两态；PDF 导出；02 §4.4 运动规则逐条走查通过且 `ReduceMotion`/`ReduceTransparency` 合规；空闲 0 帧；中英文完整；全部 18 张基线通过，用户在真机确认观感 |
| **M4** | 见 §8 |

## 8. 最终验收清单（CEO 执行，输出 PASS / FAIL）

1. **需求覆盖**：02 文档 P0、P1 每项 → 对应测试或演示路径 → 状态；P0 必须 100%，P1 未完成项需有 CEO 书面豁免。
2. **架构一致性**：`tests/arch` 通过；契约仅经已批准 CR 变更；`mygo vet` 无告警；无跨层 import。
3. **代码质量**：`gofmt`/`vet` 清洁；覆盖率达标；无未登记 TODO；无吞掉的错误（抽查 + 静态检查）；核心路径无 panic。
4. **无损**：语料保真 100%；属性测试 1 万步 × 多种子通过。
5. **性能**：03 文档 §10 预算表逐项实测达标（附数据）。
6. **可靠性**：原子写故障注入、崩溃恢复、冲突矩阵测试通过。
7. **体验**：`docs/design/README.md` 全部基线对照通过，差异均有 ADR 或 CR；UX 走查清单（Paper/Midnight Ink/高对比、CJK IME、键盘全流程、动效与降级、窗口尺寸）通过。
8. **跨平台**：三平台构建产物；macOS/Linux 手工验收报告（未验证项必须明确标注为"未验证"，不得计为通过）。
9. **遗留风险**：已知限制与风险清单及后续建议。

验收结论只基于实际运行的证据（测试输出、基准数据、截图、手工验收记录）；**未经验证的功能不标记为通过**。

## 9. 给各 Agent 的任务包（分配时直接附上）

每个 Agent 启动时阅读：`docs/README.md`、`docs/03-architecture.md`、`docs/04-contracts.md`、本文档 §3 §5 §6 中与自己相关的部分（FE-1、FE-2、QA 还须阅读 `docs/02-product-ux.md` 与 `docs/design/README.md`），以及 MyGo 自带的 `.agents/skills/mygo-maintenance/SKILL.md`（INT-0.1 会通过 `go tool mygo install-skills` 安装到仓库）。

通用规则（所有 Agent）：
1. 只修改归属表中属于自己的路径；需要别处改动时写入合并请求或提 CR。
2. 不修改 `internal/contract`；契约问题走 CR。
3. 遵守线程规则：视图里不做 I/O；阻塞服务不在主线程调用；`ui.Element` 不入状态。
4. 测试不依赖墙钟时间：用 `ui.Tester` 的 `Advance`，按计数断言。
5. 每完成一张任务卡：rebase → 门禁 → 写 `reports/<agent>.md` 合并请求。
6. 不得把未运行验证的功能写成"已完成"。

| Agent | worktree / 分支 | Phase 0 起始任务 | 特别注意 |
|---|---|---|---|
| INT | `Aurink_claude-wt/int` · `int/app`（合并在 `Aurink_claude` main） | INT-0.1 → INT-0.2 | 你是唯一合并者；门禁失败一律退回；MyGo 版本锁定 v0.7.3 |
| BE-1 | `be-core` · `be/core` | S3 → BE-1.1 | 投影与编辑是纯函数；正确性优先于性能；属性测试是主要武器 |
| BE-2 | `be-svc` · `be/services` | S2 → BE-2.1 | 原子写参照 MyGo `document_file.go`；Windows 路径大小写；所有阻塞方法注释"禁止主线程" |
| FE-1 | `fe-editor` · `fe/editor` | S1 → FE-1.1（基于 mock） | 光标永远存源码偏移；排版必须按块缓存；IME 预编辑不写入源码；浮层只用 `views/kit`；按基线对照交付 |
| FE-2 | `fe-shell` · `fe/shell` | S4 → FE-2.1 | 组件只用令牌；材质与动效集中在 `effects`/`motion`/`kit`；低功耗路径；空闲 0 帧；按基线对照交付 |
| QA | `qa` · `qa/tests` | QA-0.2 → QA-0.1 | mock 与真实实现跑同一套件；失败用例最小化；结论只基于实测 |
