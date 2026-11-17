# 09 · Phase 4 实施计划

> 状态：**规划**（2026-11-17 写）。进度见 §10。
>
> 本文说明 Phase 4（加固与发布，M4）怎么做：范围、待用户决定的问题、批次顺序、设计决定与最终验收标准。任务卡交付物见 [05 §6](05-delivery-plan.md)，二者冲突时以本文为准。工作方式沿用 05 §1、[06 §4](06-phase1.md)、[07 §4](07-phase2.md) 与 [08](08-phase3.md)。

## 1. 目标

Phase 4 结束时（M4）达到 1.0 发布候选状态：满足 [03 §10](03-architecture.md) 全部性能与资源预算、完成 [05 §6](05-delivery-plan.md) 列入的 P2 加固功能、产出三平台安装包与发布流水线，并按 [05 §8](05-delivery-plan.md) 九项清单完成最终验收。

1. **大文档与性能预算：** 超过 `LargeFileBytes`（默认 5 MB，上限 64 MB）的文档以**大文件模式**打开（不建 AST、不启渲染服务、直接进入源码模式流畅编辑、保留无损保存与撤销）；1 MB 文档在实时模式下消除块数变化时的全表重建，击键帧工作 p95 < 16 ms，03 §10 全部指标有实测数据支撑。
2. **旧编码兼容（P2）：** 无 BOM 的 GB18030（含 GBK / GB2312）文件可只读打开、正常预览与复制，顶部横幅提供「转为 UTF-8 并编辑」与「另存为」，绝不静默改写原编码字节。
3. **HTML 块安全子集渲染（P2）：** 实时模式下对常见 README 的安全 HTML 块（居中图片/徽章段落 `<p align="center">…</p>`、折叠块 `<details><summary>…</summary>…</details>` 等白名单结构）做原生组件渲染；光标进入时展开源码，未知或含危险属性的 HTML 块仍显示淡色源码，源码字节 100% 无损。
4. **自定义主题与快捷键修改（P2）：** 支持加载用户目录 `themes/*.json` 自定义主题令牌；设置面板「快捷键」页支持点击录制改键、冲突检测提示、单条重置与全部恢复默认。
5. **无障碍与日志加固：** 补齐自定义控件、浮层与编辑表面的 `ui.Role`、`Label` 及状态属性并通过自动化无障碍树检查；落地 `PathUserData/logs/aurink.log` 滚动文件日志（03 §9）。
6. **打包瘦身、CI 发布与用户文档：** 清理 `resources/` 中已被 `go:embed` 内嵌的重复大文件与 `.go` 源文件（保留图标与开源许可证），补齐三平台构建发布工作流 `.github/workflows/release.yml` 与根目录用户文档 `README.md`。

## 2. 开工前的状态

### 2.1 M3

**M3 已通过。** 用户于 2026-11-12 确认 0.3.0 真机审查通过，`BE-TYPORA-EDIT` 及 M3 证据已合入并推送 `origin/main`（`a73aeaf`），评估见 [adr/README.md](adr/README.md)。

### 2.2 待用户决定的问题

开工前请确认以下 3 项（给出决定即可启动批次 1）：

| # | 问题 | 背景与依据 | 默认建议 |
|---|---|---|---|
| U1 | 是否接入自动更新插件 `updater/native`（BE-2.11，05 标“可选”） | [源码] MyGo v0.7.3 自带纯原生 UI 的 `plugins/updater/native`（无 WebView），但要求 `mygo.json` 配置公开可达的 `updates` 地址与 `mygo keygen` 公钥；当前 `github.com/oysterhyd/aurink` 为私有仓库，未公开 Release 时客户端无法匿名拉取更新 | **暂不启用自动更新**（仅保留静态版本号与关于页；若你计划公开 Release 或配置 S3 更新桶，可改为接入） |
| U2 | 05 未列入 Phase 4 卡片的其余 P2 项，以及 M3 留下的 3 个细节差异 | 02 标 P2 但不在 05 Phase 4 表内的有：分屏、多窗口、跨文件替换、冲突差异对比；M3 记为小差异的有：设置「跟随系统强调色」、标签条溢出边缘渐隐、单击图片展开源码时保留下方预览 | **维持 05 范围**（分屏等 4 项归 1.x 不做；M3 的 3 个小细节如你想顺手补哪项可点名加入 FE-1.8 / FE-2.8） |
| U3 | 发布版本号、签名证书与真机条件 | 当前 `mygo.json` 为 `0.3.0`；Windows Authenticode / Apple Developer ID 签名需对应证书；本机仅 Windows | 版本号升为 **`1.0.0`**；无证书平台使用未签名 / macOS ad-hoc（`-sign -`）构建并在验收单据实记录；无真机的 macOS / Linux 运行时项按 05 §8 标为**未验证** |

### 2.3 代码核查发现的缺口（开工前盘点）

对照 02、03、05 与当前源码核查到的具体问题，全部纳入本阶段对应批次：

| 位置 | 现状 [源码 / 实测] | 归属 |
|---|---|---|
| `core/docsession/manager.go:205` | `readOptions()` 把 `Settings.Files.LargeFileBytes`（默认 5 MB）直接传给了 `ReadOptions.MaxSize`，导致 5–64 MB 文件直接返回 `CodeTooLarge` 报错拒绝打开，`DocState.LargeFile` 从未被设为 `true` | 批次 2 BE-1.8 |
| `core/document/document.go:26,176` | 所有文档打开和每次 `Apply` 都固定运行 `markdown.New` / `Update`；5 MB 文档全量适配约 384 ms（S3 [实测]），大文件模式下仍解析 AST 会卡顿 | 批次 2 BE-1.8 |
| `views/source/source.go:192` | `Sync()` 每次击键调用 `Buffer.String()` 复制全文并调 `e.index()` 全文重扫 `\r\n`；5 MB 文档下每次击键会产生十余 MB 扫描与复制 | 批次 5 FE-1.8 |
| `views/liveedit/editor.go:466,502` | `syncRows` 仅在块数与叶子数都不变时走快路径；`Enter` 拆段等改变叶子数的编辑会走 `rebuildRows` 遍历全篇叶子重建切片与 `rowOf`（1 MB 实测约 48.4 万次分配，真窗口帧工作 p95 15.1–15.4 ms 紧贴 16 ms 预算） | 批次 5 FE-1.8 |
| `resources/` | `fonts/*.ttf`（6.3 MB）和 `render/vendor/*.js`（5.4 MB）既通过 `go:embed` 编入可执行文件，又随 `resources/` 被 `mygo build` 复制到安装目录，连同 `fonts.go`、`render.go` 一起落盘，安装目录重复占用约 11.8 MiB | 批次 7 INT-4.1 |
| `internal/app/app.go` | `slog` 仅走默认标准错误输出，尚未按 03 §9 写入滚动文件日志 `PathUserData/logs/aurink.log`（5×5 MB） | 批次 7 INT-4.1 |
| `reports/quality.md` | 03 §10 中的「冷启动到首帧 < 500 ms」「打开 1 MB 文档到首屏 < 300 ms（真窗口）」「空闲内存 < 150 MB」尚缺真窗口实测记录 | 批次 8 QA-4.1 |

## 3. M4 的范围

### 3.1 功能与加固交付

| 项目 | M4 的表现 | 任务卡 |
|---|---|---|
| 大文件模式（5–64 MB） | ≥ `LargeFileBytes` 打开时 `DocState.LargeFile = true`、跳过 AST 解析与渲染服务、直接进入源码模式并显示大文件横幅；保存、自动保存、恢复、查找替换、撤销重做保持完整无损；> 64 MB 报 `CodeTooLarge` | BE-1.8、FE-1.8 |
| 1 MB 实时编辑与局部行同步 | 块增删时 `liveedit` 按变更区间局部拼接行表与 Fenwick 高度，不再遍历整篇全部叶子；1 MB 击键帧工作 p95 < 16 ms 并留出安全余量 | BE-1.8、FE-1.8 |
| GB18030 旧编码打开与转换 | 自动识别非 UTF-8、非二进制的合法 GB18030（兼容 GBK/GB2312）文件并以只读模式打开；横幅显示当前编码，支持「转为 UTF-8 并编辑」和「另存为」 | BE-2.11、FE-1.8 |
| HTML 块安全子集渲染 | 未激活的 `BlockHTML` 若完全匹配安全白名单（居中段落/标题/图片/链接徽章、`<details><summary>` 折叠块、`<hr>`/`<br>`），按原生块组件渲染；点击 `<summary>` 在视图状态切换展开收起（不改源码）；光标移入展开源码；超白名单保持淡色源码 | FE-1.8 |
| 自定义主题 JSON | 启动与打开设置时读取 `PathUserData/themes/*.json`，校验色值与基础明暗，出现在设置「外观」页并支持实时切换与溶解过渡；提供「打开主题文件夹」入口 | FE-2.8 |
| 快捷键查看与修改 | 设置「快捷键」页支持搜索筛选、点击命令录制新组合键、冲突命令提示与确认覆盖、单条恢复默认、全部恢复默认；立即同步 `Commands` 注册表生效 | FE-2.8 |
| 无障碍支持 | 核对并补全所有图标按钮、开关、滑块、分段控件、对话框、横幅、命令面板、斜杠菜单与编辑表面的无障碍角色/名称/状态；`Tester` 无障碍树断言通过 | FE-1.8 |
| 日志、打包与 CI 发布 | 滚动日志 `logs/aurink.log`；`resources/` 去重瘦身（保留 `icon.png` 与 `licenses/`）；`mygo build` 三平台产物与 `.github/workflows/release.yml`；根目录用户 `README.md` | INT-4.1 |
| 性能全表与跨平台回归 | 03 §10 性能预算全表实测、全套回归、跨平台验收记录与 05 §8 最终验收清单 | QA-4.1 |

### 3.2 明确不做

- **通用 Markdown 分片解析器：** S3 [实测] 已证明空行与顶层块边界在未闭合围栏、超长容器与全局引用定义下不能限定最坏成本，且 BE-1.2 的 24 KiB 窗口局部重解析已将 200 KB / 1 MB 常规解析控制在 0.3–3.4 ms；≥ 5 MB 按 S3 结论走大文件源码模式。
- **分屏、多窗口、跨文件替换、冲突差异对比：** 归 1.x（除非用户在 U2 另行指定）。
- **HTML 块任意 CSS / 表格 / WebView 内嵌渲染：** 只做原生 `mygo/ui` 安全白名单组件，不把编辑区拆出 WebView 子树，彻底杜绝 XSS 与滚动割裂。
- **GB18030 原编码写回：** 02 §2.3 规定旧编码只读打开并提示转换；编辑保存统一走转换后的 UTF-8，避免字符集子集映射有损。

## 4. 执行顺序

| 批次 | 执行 | 任务卡 | 为什么排在这里 |
|---|---|---|---|
| 1 | 负责人 | 确认 U1–U3；落实 §5 契约与 `contracttest` 套件更新（大文件无 AST 模式、GB18030 读取与转换、主题加载接口或约定） | Codex 开工前锁定边界与验收测试 |
| 2 | Codex，并行 | **BE-1.8**（`core/document` 无 AST 大文件模式、`core/docsession` 大文件打开与恢复）、**BE-2.11**（`core/fsstore` GB18030 解码、`core/docsession` 旧编码只读与转 UTF-8） | 注意两者都碰 `docsession`：若改同一批文件则按 BE-1.8 → BE-2.11 串行，或合并为一张后端卡交 Codex 一次完成避免合并冲突 |
| 3 | 负责人 | **FE-1.8 第一部分（性能与大文件前端）**：`shell` 大文件模式接线与旧编码转换横幅、`source` 5 MB 增量同步优化、`liveedit` 局部行表更新（消除 `Enter` 全量遍历） | 紧接后端大文件与编码能力，先把性能硬指标跑通 |
| 4 | 负责人 | **FE-1.8 第二部分（HTML 块安全子集与无障碍）**：`liveedit` 安全 HTML 块解析与渲染组件、全界面无障碍标签走查与测试 | 独立于设置界面改动 |
| 5 | 负责人 | **FE-2.8 主题与键位**：`views/theme` 自定义 JSON 主题解析、`shell/settings.go` 自定义主题选择与快捷键录制/冲突/重置 | 收口全部前端功能 |
| 6 | 负责人 | **INT-4.1 发布工程**：内嵌资源移至 `internal/assets`（瘦身安装目录）、滚动文件日志、`mygo.json` 1.0 配置、`.github/workflows/release.yml`、根目录 `README.md`、本机安装包构建与验证 | 所有代码与文案稳定后再打正式包 |
| 7 | 负责人 + 用户 | **QA-4.1 与 M4 最终验收**：03 §10 全套性能/内存测量、全套回归、跨平台报告、05 §8 九项最终验收单，用户真机终审 | 收官阶段门 |

## 5. 设计决定

| # | 问题 | 决定 |
|---|---|---|
| D1 | 大文件阈值与硬上限 | `FileStore.Read` 的硬上限保持 64 MB（`ReadOptions.MaxSize == 0`），超过 64 MB 返回 `CodeTooLarge` 弹窗提示无法打开。文件字节数 ≥ `Settings.Files.LargeFileBytes`（默认 5 MB）且 ≤ 64 MB 时正常读入，会话标记 `DocState.LargeFile = true` |
| D2 | 大文件模式的文档模型 | 新增纯文本无解析模式（如 `DocMeta.Large bool` 或 `DocumentFactory` 识别）：不构造 `markdown.Parser`，`AST()` 返回仅含根节点的空快照，`Apply` / `Undo` / `Redo` 只操作 `text.Text` 与历史栈。这样 5–64 MB 打开与编辑都不跑 Markdown 解析，且单向事务流、自动保存、崩溃恢复、字节无损完全复用现有 `Session` |
| D3 | 大文件模式的前端行为 | `DocState.LargeFile` 为真时：`shell.attach` 直接以 `editorapi.ModeSource` 打开，不创建常驻重排版缓存，禁用切到实时/阅读模式（视图菜单置灰说明原因），顶部显示 `bannerLarge` 横幅，大纲面板显示「大文件模式下不生成大纲」，渲染服务不启动（08 D19） |
| D4 | 5 MB 源码模式的击键开销 | 改造 `views/source`：利用已知编辑偏移增量更新 `crlf` 行尾表，不在每次普通击键后用 `strings.Index` 全篇重扫 5 MB 文本；补 `BenchmarkSourceType5MB` 断言单键耗时与分配 |
| D5 | 1 MB 实时模式的块增删局部化 | `liveedit.syncRows` 在叶子数变化但 `ev.Changes` 局限于局部区间时，由旧前驱/后继未变叶子偏移直接拼接 `e.rows` 中段，只为新增/脏叶子更新 `rowOf` 和 Fenwick 树差值，避免全篇 O(N) 重建 |
| D6 | GB18030 识别规则 | `fsstore` 在无 BOM 且 `!utf8.Valid(data)` 且 `!isBinary(data)` 时，用 `simplifiedchinese.GB18030.NewDecoder()` 解码并检查是否含非原生的 `\uFFFD` 替换符：全部合法解码则设 `Meta.Encoding = "gb18030"` 返回文本，否则仍返回 `CodeEncoding`。`fsstore.Write` 拒绝直接写 `gb18030`（返回 `CodeReadOnly` 或 `CodeEncoding`），防止有损回写 |
| D7 | 旧编码会话转换流程 | `docsession` 对 `Meta.Encoding == "gb18030"` 的文件初始化 `ReadOnly = true`。`Session` 增加显式转换动作（如 `ConvertToUTF8()`）：在主线程将 `Model.SetMeta` 的 `Encoding` 改为 `"utf-8"`、解除 `ReadOnly` 并标记为已编辑（用户保存或自动保存时按原子写落盘为 UTF-8，且在横幅文案中写明保存后文件将变为 UTF-8） |
| D8 | HTML 块安全白名单范围 | 用标准合规的 `golang.org/x/net/html`（已在 05 §3.3 白名单）解析单个 `BlockHTML`：允许容器 `<p>`、`<div>`、`<h1>`–`<h6>`、`<blockquote>`、`<details>`、`<summary>`、`<hr>`、`<br>` 与内联 `<a>`、`<img>`、`<strong>`/`<b>`、`<em>`/`<i>`、`<code>`、`<kbd>`、`<sub>`、`<sup>`；属性仅允许 `align`（`left`/`center`/`right`）、`src`（仅本地相对/绝对路径，默认不加载 `http(s)://`，与正文图片策略一致）、`alt`、`width`、`height`、`href`（仅 `http(s):`、`mailto:` 与相对路径，严禁 `javascript:` / `data:`）、`open`。出现任何白名单外的标签、内联 `style`、`on*` 事件属性或解析残缺时，整块回退为现有的淡色源码显示 |
| D9 | 自定义主题 JSON 格式 | 目录为 `PathUserData/themes/*.json`。结构：`{"name": "Solarized Paper", "dark": false, "accentHue": 182, "colors": {"window": "#...", "sheet": "#...", "text": "#...", ...}}`。基于 `theme.For(Options{Dark: dark, Hue: accentHue})` 生成默认令牌后，按字段覆盖合法 `#RRGGBB` / `#RRGGBBAA` 色值。解析失败时记录日志并弹一次非阻塞通知，回退默认主题 |
| D10 | 快捷键持久化与冲突处理 | 复用 `Settings.Keymap []contract.KeyBinding` 与 `core/commands` 现有的 `SetUserBindings` / `Conflicts`：录制时忽略单独的修饰键，禁止占用 `Esc`、`Tab`、`Enter`、`Backspace` 等基础编辑键（无 Ctrl/Cmd/Alt 修饰时拒绝录制）；与现有命令冲突时显示冲突命令名，确认覆盖则同时为被占命令写入空绑定或替换绑定 |
| D11 | 无障碍范围与边界 | 基于 MyGo `ui/access.go` 的原生 `AccessTree`：每个无文字图标按钮必须有本地化 `Label`；模态框用 `RoleDialog`；开关/复选框报告 `Checked`；滑块报告 `Range`；命令面板与斜杠菜单输入框通过 `ActiveDescendant` 指向高亮项；`liveedit` 表面保持 `RoleTextField` 并暴露文档名与只读状态（02 §8 已声明的自定义画布读屏限制在用户文档与验收单如实写明） |
| D12 | 内嵌资源与安装目录瘦身 | 把 `resources/fonts/*.ttf` 与 `resources/render/*` 迁入 `internal/assets/{fonts,render}` 做 `go:embed`（同步更新 `03 §1.1` 与 `tests/arch` 规则）；`resources/` 下只保留 `icon.png` 与 `licenses/`（存放 OFL、MathJax、Mermaid 许可证文本），既满足 05 §3.3 随包分发许可证的要求，又消除安装目录中重复的 11.8 MiB 二进制资源与 `.go` 文件 |
| D13 | 日志落盘 | 在 `internal/app` 启动时初始化 `PathUserData/logs/aurink.log` 写入器：单文件上限 5 MB，保留最近 5 个滚动备份（`aurink.log.1`..`4`），开发模式同时输出 stderr，生产模式只写文件 |
| D14 | 三平台发布流水线 | 新增 `.github/workflows/release.yml`（`v*` tag 与手动 `workflow_dispatch` 触发）：先在三平台跑门禁，再分别在 `windows-latest`（NSIS `.exe`）、`macos-latest`（`darwin/universal` `.dmg`，未配置证书 Secret 时传 `-sign - -skip-notarize`）、`ubuntu-latest`（`.deb` + `.tar.gz`）运行 `go tool mygo build` 并上传构建产物与 SHA-256 校验表 |

## 6. 任务要点

### 批次 2：BE-1.8 大文档 + BE-2.11 旧编码（Codex）

- 由于两张卡都修改 `internal/core/docsession`，合为同一 worktree 一次交付，避免人工解冲突。
- **测试要求：**
  - 5.5 MB Markdown 文件 `Open` 成功，`State().LargeFile == true`，编辑、`Save`、崩溃恢复后字节 100% 一致，且 `markdown.Parse` 调用次数为 0。
  - 65 MB 文件（用稀疏或合成测试覆盖边界）返回 `CodeTooLarge`。
  - GB18030 / GBK 编码样本（含中文标点、表格、代码块）打开后 `State().ReadOnly == true`、`Meta().Encoding == "gb18030"`；直接 `Apply` 返回 `CodeReadOnly`；调用转换后 `ReadOnly == false`、`Meta().Encoding == "utf-8"`，`Save` 后磁盘为合法 UTF-8 且内容逐字相等。
  - 非法字节流（既非 UTF-8 也非合法 GB18030）仍返回 `CodeEncoding`；含 NUL 字节的二进制文件仍返回 `CodeBinary`。

### 批次 3：FE-1.8 第一部分（性能、大文件前端、旧编码横幅）

- `shell` 对 `LargeFile` 标签锁定源码视图，横幅说明「大文件模式：已关闭实时预览以保证流畅」；对 `gb18030` 只读标签在横幅显示「GB18030 编码（只读）」及「转为 UTF-8 并编辑」「另存为」两个按钮，补 `tests/qa` 场景测试。
- `source` 补 `BenchmarkSourceType5MB`（300 次击键），验证单键不随 5 MB 线性扫全文。
- `liveedit` 局部行同步完成后，更新 `tests/perf/work_test.go` 中 1 MB `Enter` 的分配上限断言（从 1,000,000 收紧到实测值的 2 倍以内），并在真窗口 `tools/windowcheck` 复测 200 KB 与 1 MB 的帧工作 p50/p95。

### 批次 4：FE-1.8 第二部分（HTML 块安全子集与无障碍）

- HTML 块白名单必须过**安全负向测试**（`<script>`、`<img onerror=...>`、`<a href="javascript:...">`、`<iframe>`、`<style>`、远程 `https://` 图片等 15+ 攻击/越界样例），断言无任何网络请求、无危险跳转、未匹配的块原样回退淡色源码，且编辑其他块保存后 HTML 块字节一字不改。
- 无障碍：在 `tests/qa/a11y_test.go` 遍历主界面、侧栏三面板、查找栏、命令面板、斜杠菜单、格式条、设置对话框的所有可聚焦/可点击元素，断言不存在无名称（空 `Label`）的按钮或控件。

### 批次 5：FE-2.8 自定义主题与快捷键修改

- 自定义主题测试：合法 JSON 覆盖色值生效、缺省字段继承基础明暗主题、非法 JSON 不崩并回退默认。
- 快捷键测试：录制新键位后按新键触发命令、旧键失效、重启后从 `settings.json` 恢复、冲突提示与覆盖、恢复默认后回到 `DefaultBindings()`。

### 批次 6：INT-4.1 打包、日志、CI 发布与文档

- 迁移内嵌资源后跑 `go tool mygo build`，核查 `build/windows-amd64/` 目录：确认不再有 `.go`、`.ttf`、`vendor/*.js` 冗余文件，同时 `licenses/` 齐全，安装并打开公式/衬线标题正常。
- 验证 `aurink.log` 写入与 5 MB 滚动轮转。
- 编写用户向根目录 `README.md`（中英或中文主文档，覆盖特性、安装、源码构建、快捷键、自定义主题格式、架构简述与开源致谢）。

### 批次 7：QA-4.1 与 M4 最终验收

- 新建/扩展测量工具（或在 `tools/windowcheck` 增加 `-perf` / `-mem` 模式），在本机真窗口实测 03 §10 全部行项：冷启动首帧、1 MB 首屏打开、200 KB / 1 MB 击键帧工作 p95、100 KB 文档空闲进程内存（Working Set 与 Private Bytes）、发布包体积，全部写入 `reports/quality.md`。
- 编写 `reports/cross-platform.md`（Windows 真机实测结果 + macOS/Linux CI 通过项与未验证真机清单）。
- 逐条核对 05 §8 九项最终验收标准，写入 `docs/adr/README.md` `# M4 最终评估`，请用户确认。

## 7. 性能与资源预算（03 §10 最终核查表）

| 指标 | 03 §10 预算 | 当前基线（M3） | Phase 4 目标与验证方式 |
|---|---|---|---|
| 冷启动到首帧 | < 500 ms | 未单列真窗口埋点 | `windowcheck` / 启动埋点实测 < 500 ms |
| 打开 1 MB 文档到首屏 | < 300 ms | Tester `BenchmarkOpen1MB` 83 ms | 真窗口首屏埋点 < 300 ms |
| 击键帧工作 p95（≤200 KB） | < 8 ms | Tester 1.33 ms；真窗口帧工作 4.9–5.1 ms（含垂直同步总延迟 17.9 ms，M1 ADR 已记录） | 保持帧工作 p95 < 8 ms |
| 击键帧工作 p95（≤1 MB） | < 16 ms | Tester 2.60 ms；真窗口帧工作 15.1–15.4 ms；`Enter` 48.4 万次分配 | 局部行同步优化后帧工作 p95 < 12 ms（严守 < 16 ms），`Enter` 分配显著下降 |
| 5 MB 大文件源码编辑 | 流畅打开与编辑 | 当前 >5 MB 报 `CodeTooLarge` | 打开 < 200 ms，源码模式单键 `BenchmarkSourceType5MB` < 2 ms |
| 1 万块文档滚动 | 60 fps，无整屏重排 | 1.28 ms/op，8.2 shapes/frame | 保持达标 |
| 空闲 CPU / 帧 | 0 帧（极光 12 s 后 0 帧） | 0 帧 | 保持达标 |
| 空闲内存（100 KB 文档，无渲染服务） | < 150 MB | 未单列实测 | 实测进程 Private Bytes / Working Set < 150 MB |
| 发布体积（不含外置重复资源） | < 40 MB | `Aurink.exe` 27.57 MiB，安装包 13.96 MiB（安装目录另重复 11.8 MiB） | 去重后单文件约 28 MiB、安装目录总计 < 30 MiB |

## 8. 主要风险

| 风险 | 影响 | 应对 | 发现时点 |
|---|---|---|---|
| 大文件无 AST 模式下某处前端代码假定 `AST` 必有正文叶子块 | 空指针或越界 panic | `emptyAST` 仍满足 `contract.AST` 不变式（含合法根块与单个覆盖全文的纯文本叶子或空集，批次 1 用 `contracttest` 锁死）；`shell` 在大文件模式下不挂载 `liveedit` | 批次 1、2 |
| GB18030 误判其它单字节编码（如 Latin-1）为乱码中文 | 显示错乱文本 | 只对含多字节序列且全部严格通过 GB18030 解码（零替换字符）的文本放行，且默认**只读**不改原盘，零数据损坏风险 | 批次 2 |
| HTML 块安全渲染引入布局抖动或安全漏洞 | 破坏无损承诺或引发注入 | 严格白名单 + 禁外链图片/脚本协议 + 光标进入立刻回退源码 + 15 例负向安全测试 | 批次 4 |
| 用户把普通字母键录成全局快捷键导致无法打字 | 编辑器无法输入 | 录制器强制要求含 `Ctrl`/`Cmd`/`Alt` 修饰键或 `F1`–`F12` 功能键（单独字母、数字、空格、`Shift+字符` 一律拒绝） | 批次 5 |
| macOS / Linux 无真机且无签名证书 | 无法完成真机手测与公证 | CI 产出未公证/ad-hoc 构建产物，严格遵循 05 §8 第 8 条将未真机验证项标注为“未验证”，绝不伪报通过 | 批次 6、7 |

## 9. M4 最终验收（对照 05 §8）

M4 关闭前逐项输出 PASS / FAIL / 未验证证据：

1. **需求覆盖：** 02 文档 P0 100%、P1 100%（已在 M3 `reports/quality.md` 核毕）、本计划 §3.1 列入的 P2 项全部对应自动化测试与真窗口验证。
2. **架构一致性：** `tests/arch` 通过；契约与 04 文档一致；`mygo vet` 零告警；`contract` 仅依赖标准库。
3. **代码质量：** `gate.ps1 -AllTargets` 五目标全绿；核心包测试覆盖率达标；无未登记 `TODO`；核心路径零 panic。
4. **无损保真：** 语料库（含扩展开/关、CRLF、BOM、混合换行、HTML 块、大文件、GB18030 转换）字节保真 100%；3 种子 × 10,000 步随机编辑属性测试通过；Typora 104 场景对拍通过。
5. **性能与内存：** §7（03 §10）全部指标附本机实测数据达标，写入 `reports/quality.md`。
6. **可靠性：** 原子写故障注入、kill 进程崩溃恢复、19 格外部冲突矩阵、滚动日志落盘全部通过。
7. **体验与无障碍：** 18 张设计基线对照记录齐全；中英文界面完整无截断；无障碍树测试通过；自定义主题与改键可用。
8. **跨平台与发布：** CI 三平台产物可构建下载；Windows 安装包实测安装/运行/关联/卸载；macOS 与 Linux 按实际真机条件出具报告，未验证项明确标为**未验证**。
9. **遗留风险：** 输出 1.0 已知限制与后续 1.x 演进建议清单。

## 10. 进度

| 批次 | 状态 | 提交 | 备注 |
|---|---|---|---|
| 1 | 待开始 | — | 待用户确认 §2.2（U1–U3）后启动 |
| 2 | 待开始 | — | BE-1.8 大文档 + BE-2.11 GB18030（Codex） |
| 3 | 待开始 | — | FE-1.8 第一部分：性能局部化、5 MB 源码同步、大文件与旧编码前端 |
| 4 | 待开始 | — | FE-1.8 第二部分：HTML 块安全子集渲染、无障碍标签与测试 |
| 5 | 待开始 | — | FE-2.8：自定义主题 JSON、快捷键录制修改与冲突处理 |
| 6 | 待开始 | — | INT-4.1：内嵌资源去重、滚动日志、`release.yml`、用户 `README.md`、1.0 安装包 |
| 7 | 待开始 | — | QA-4.1 与 M4 最终验收 |
