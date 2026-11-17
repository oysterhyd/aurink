# 09 · Phase 4 实施计划

> 状态：**已完成（自动化门禁与真窗口实测通过，待用户 Windows 真机终审）**（2026-11-18）。进度见 §10，最终评估见 [adr/README.md](adr/README.md)。
>
> 本文说明 Phase 4（加固与发布，M4 / v1.0.0）怎么做：范围、已确认的用户决定、批次顺序、设计决定与最终验收标准。任务卡交付物见 [05 §6](05-delivery-plan.md)，二者冲突时以本文为准。工作方式沿用 05 §1、[06 §4](06-phase1.md)、[07 §4](07-phase2.md) 与 [08](08-phase3.md)。

## 1. 目标

Phase 4 结束时（M4）交付 **Aurink v1.0.0（Windows 正式版）**：满足 [03 §10](03-architecture.md) 全部性能与资源预算、完成 P2 加固与体验收口、接入基于 GitHub Releases 的签名自动更新，并按 [05 §8](05-delivery-plan.md) 完成最终验收。

1. **大文档与性能预算：** 超过 `LargeFileBytes`（默认 5 MB，硬上限 64 MB）的文档以**大文件模式**打开（不建 AST、不启渲染服务、直接进入源码模式流畅编辑、保留无损保存与撤销）；1 MB 文档在实时模式下消除块数变化时的全表重建，击键帧工作 p95 < 16 ms，03 §10 全部指标有真窗口实测数据支撑。
2. **旧编码兼容（P2）：** 无 BOM 的 GB18030（含 GBK / GB2312）文件可只读打开、正常预览与复制，顶部横幅提供「转为 UTF-8 并编辑」与「另存为」，绝不静默改写原编码字节。
3. **HTML 块安全子集渲染与细节打磨（P2）：**
   - 实时模式下对常见 README 的安全 HTML 块（居中图片/徽章段落 `<p align="center">…</p>`、折叠块 `<details><summary>…</summary>…</details>` 等白名单结构）做原生组件渲染，光标进入时展开源码，其余回退淡色源码，源码字节 100% 无损。
   - **单击图片展开源码时在下方保留图片预览**（对齐 Typora 及现有公式/图表激活体验）。
   - **标签条横向溢出时边缘渐隐遮罩**（消除标签被硬裁切感）。
4. **自定义主题与快捷键修改（P2）：** 支持加载用户目录 `themes/*.json` 自定义主题令牌；设置面板「快捷键」页支持点击录制改键、冲突检测提示、单条重置与全部恢复默认。
5. **自动更新（P2）：** 仓库公开后接入 MyGo 原生 `plugins/updater/native`（无需 WebView），基于 Ed25519 签名与 GitHub Releases 分发增量/全量更新；支持后台检查开关与手动「检查更新…」。
6. **无障碍、日志与 Windows 发布工程：** 补齐自定义控件与编辑表面的 `ui.Role` / `Label` 及无障碍树测试；落地 `PathUserData/logs/aurink.log` 滚动文件日志；清理 `resources/` 重复资源；产出 `1.0.0` Windows 安装包、签名更新归档、CI 发布流水线 `.github/workflows/release.yml`、`CHANGELOG.md` 与用户 `README.md`。

## 2. 开工前的状态

### 2.1 M3

**M3 已通过。** 用户于 2026-11-12 确认 0.3.0 真机审查通过，`BE-TYPORA-EDIT` 及 M3 证据已合入并推送 `origin/main`（`a73aeaf`），评估见 [adr/README.md](adr/README.md)。

### 2.2 用户已确认的决定（2026-11-18）

| # | 问题 | 决定 |
|---|---|---|
| U1 | 仓库可见性与自动更新 | **已将 `github.com/oysterhyd/aurink` 改为公开仓库**；接入 `github.com/egoist/mygo/plugins/updater/native`，使用 `mygo keygen` 生成 Ed25519 密钥对（公钥写入 `mygo.json`，私钥存入 GitHub Actions Secret `MYGO_UPDATER_PRIVATE_KEY`），通过公开 GitHub Releases 分发签名更新 |
| U2 | P2 范围与 M3 遗留细节取舍 | 分屏、多窗口、跨文件替换、冲突差异对比维持 05 范围留到 1.x；「跟随系统强调色」不做（已有 4 套极光色相 + 本阶段自定义主题 JSON）；**纳入两项高价值体验打磨：① 单击图片展开源码时保留下方图片预览；② 标签条横向溢出时两侧边缘渐隐** |
| U3 | 目标平台与发布范围 | **聚焦 Windows（`windows/amd64`）**，发布版本号 **`1.0.0`**。安装包构建、自动更新产物、CI 发布流水线与真机最终验收只针对 Windows；日常门禁保留跨平台纯 Go 编译检查以维护代码整洁，不做 macOS / Linux 打包与真机工作 |

### 2.3 代码核查发现的缺口（开工前盘点）

对照 02、03、05 与当前源码核查到的具体问题，全部纳入本阶段对应批次：

| 位置 | 现状 [源码 / 实测] | 归属 |
|---|---|---|
| `core/docsession/manager.go:205` | `readOptions()` 把 `Settings.Files.LargeFileBytes`（默认 5 MB）直接传给了 `ReadOptions.MaxSize`，导致 5–64 MB 文件直接返回 `CodeTooLarge` 报错拒绝打开，`DocState.LargeFile` 从未被设为 `true` | 批次 2 BE-1.8 |
| `core/document/document.go:26,176` | 所有文档打开和每次 `Apply` 都固定运行 `markdown.New` / `Update`；5 MB 文档全量适配约 384 ms（S3 [实测]），大文件模式下仍解析 AST 会卡顿 | 批次 2 BE-1.8 |
| `views/source/source.go:192` | `Sync()` 每次击键调用 `Buffer.String()` 复制全文并调 `e.index()` 全文重扫 `\r\n`；5 MB 文档下每次击键会产生十余 MB 扫描与复制 | 批次 3 FE-1.8 |
| `views/liveedit/editor.go:466,502` | `syncRows` 仅在块数与叶子数都不变时走快路径；`Enter` 拆段等改变叶子数的编辑会走 `rebuildRows` 遍历全篇叶子重建切片与 `rowOf`（1 MB 实测约 48.4 万次分配，真窗口帧工作 p95 15.1–15.4 ms 紧贴 16 ms 预算） | 批次 3 FE-1.8 |
| `resources/` | `fonts/*.ttf`（6.3 MB）和 `render/vendor/*.js`（5.4 MB）既通过 `go:embed` 编入可执行文件，又随 `resources/` 被 `mygo build` 复制到安装目录，连同 `fonts.go`、`render.go` 一起落盘，安装目录重复占用约 11.8 MiB | 批次 6 INT-4.1 |
| `internal/app/app.go` | `slog` 仅走默认标准错误输出，尚未按 03 §9 写入滚动文件日志 `PathUserData/logs/aurink.log`（5×5 MB） | 批次 6 INT-4.1 |
| `reports/quality.md` | 03 §10 中的「冷启动到首帧 < 500 ms」「打开 1 MB 文档到首屏 < 300 ms（真窗口）」「空闲内存 < 150 MB」尚缺真窗口实测记录 | 批次 7 QA-4.1 |

## 3. M4 的范围

### 3.1 功能与加固交付

| 项目 | M4 的表现 | 任务卡 |
|---|---|---|
| 大文件模式（5–64 MB） | ≥ `LargeFileBytes` 打开时 `DocState.LargeFile = true`、跳过 AST 解析与渲染服务、直接进入源码模式并显示大文件横幅；保存、自动保存、恢复、查找替换、撤销重做保持完整无损；> 64 MB 报 `CodeTooLarge` | BE-1.8、FE-1.8 |
| 1 MB 实时编辑与局部行同步 | 块增删时 `liveedit` 按变更区间局部拼接行表与 Fenwick 高度，不再遍历整篇全部叶子；1 MB 击键帧工作 p95 < 16 ms 并留出安全余量 | BE-1.8、FE-1.8 |
| GB18030 旧编码打开与转换 | 自动识别非 UTF-8、非二进制的合法 GB18030（兼容 GBK/GB2312）文件并以只读模式打开；横幅显示当前编码，支持「转为 UTF-8 并编辑」和「另存为」 | BE-2.11、FE-1.8 |
| HTML 块安全子集渲染 | 未激活的 `BlockHTML` 若完全匹配安全白名单（居中段落/标题/图片/链接徽章、`<details><summary>` 折叠块、`<hr>`/`<br>`），按原生块组件渲染；点击 `<summary>` 展开收起（不改源码）；光标移入展开源码；超白名单保持淡色源码 | FE-1.8 |
| 图片源码预览与标签条渐隐 | 单击独占段落的图片进入源码编辑时，像公式/Mermaid 一样在源码下方保留图片预览（双击仍打开灯箱）；标签条左右溢出时绘制平滑边缘渐隐遮罩 | FE-1.8、FE-2.8 |
| 自定义主题 JSON | 启动与打开设置时读取 `PathUserData/themes/*.json`，校验色值与基础明暗，出现在设置「外观」页并支持实时切换与溶解过渡；提供「打开主题文件夹」入口 | FE-2.8 |
| 快捷键查看与修改 | 设置「快捷键」页支持搜索筛选、点击命令录制新组合键、冲突命令提示与确认覆盖、单条恢复默认、全部恢复默认；立即同步 `Commands` 注册表生效 | FE-2.8 |
| 自动更新（`updater/native`） | `mygo.Use(native.Plugin)`；文件菜单、关于页与命令面板提供「检查更新…」；关于页/设置提供「自动检查更新」开关；`CHANGELOG.md` 提供版本说明 | BE-2.11、INT-4.1 |
| 无障碍支持 | 核对并补全所有图标按钮、开关、滑块、分段控件、对话框、横幅、命令面板、斜杠菜单与编辑表面的无障碍角色/名称/状态；`Tester` 无障碍树断言通过 | FE-1.8 |
| 日志、瘦身打包与 Windows CI 发布 | 滚动日志 `logs/aurink.log`；`resources/` 去重瘦身（保留 `icon.png` 与 `licenses/`）；`1.0.0` Windows 安装包与签名更新产物；`.github/workflows/release.yml`（推送 `v*` 标签自动构建并发布到 GitHub Releases）；根目录用户 `README.md` | INT-4.1 |
| 性能全表与回归验收 | 03 §10 性能预算全表实测、全套回归、05 §8 最终验收清单 | QA-4.1 |

### 3.2 明确不做

- **通用 Markdown 分片解析器：** S3 [实测] 已证明空行与顶层块边界在未闭合围栏、超长容器与全局引用定义下不能限定最坏成本，且 BE-1.2 的 24 KiB 窗口局部重解析已将 200 KB / 1 MB 常规解析控制在 0.3–3.4 ms；≥ 5 MB 按 S3 结论走大文件源码模式。
- **分屏、多窗口、跨文件替换、冲突差异对比、「跟随系统强调色」：** 留待 1.x（U2）。
- **HTML 块任意 CSS / 表格 / WebView 内嵌渲染：** 只做原生 `mygo/ui` 安全白名单组件，杜绝 XSS 与滚动割裂。
- **GB18030 原编码写回：** 旧编码只读打开，编辑保存统一走显式转换后的 UTF-8。
- **macOS / Linux 安装包与真机验收：** 1.0 只面向 Windows 发布与验收（U3）。

## 4. 执行顺序

| 批次 | 执行 | 任务卡 | 为什么排在这里 |
|---|---|---|---|
| 1 | 负责人 | 落实 §5 契约与 `contracttest` 套件更新（大文件无 AST 模式、GB18030 读取与转换、`04` 文档同步） | Codex 开工前锁定边界与验收测试 |
| 2 | Codex | **BE-1.8 + BE-2.11**（`core/document` 无 AST 大文件模式、`core/fsstore` GB18030 解码、`core/docsession` 大文件打开与旧编码只读/转 UTF-8） | 两张卡同改 `docsession`，合为同一 worktree 一次交付免冲突 |
| 3 | 负责人 | **FE-1.8 第一部分（性能与大文件/旧编码前端）**：`shell` 大文件锁定源码与旧编码转换横幅、`source` 5 MB 增量同步优化、`liveedit` 局部行表更新（消除 `Enter` 全量遍历） | 紧接后端大文件与编码能力，先把性能硬指标跑通 |
| 4 | 负责人 | **FE-1.8 第二部分（HTML 块安全子集、图片源码预览、无障碍）**：`liveedit` 安全 HTML 块组件、激活图片下方保留预览、全界面无障碍标签走查与测试 | 集中在 `liveedit` 与 `kit` 内部交互 |
| 5 | 负责人 | **FE-2.8（自定义主题、快捷键修改、标签条渐隐）**：`views/theme` 自定义 JSON 主题解析、`shell/settings.go` 主题选择与快捷键录制/冲突/重置、标签条溢出边缘渐隐 | 集中在 `theme` 与 `shell` 外壳 |
| 6 | 负责人 | **INT-4.1（自动更新、日志、瘦身打包、CI 发布与文档）**：接入 `updater/native`、内嵌资源移至 `internal/assets`、滚动文件日志、`mygo.json` v1.0.0 与 `updates` 配置、`.github/workflows/release.yml`、`CHANGELOG.md`、根目录 `README.md`、Windows 安装包实测 | 所有功能与文案稳定后再打 1.0.0 正式包 |
| 7 | 负责人 + 用户 | **QA-4.1 与 M4 最终验收**：03 §10 全套性能/内存真窗口测量、全套回归、05 §8 九项最终验收单，用户 Windows 真机终审 | 收官阶段门 |

## 5. 设计决定

| # | 问题 | 决定 |
|---|---|---|
| D1 | 大文件阈值与硬上限 | `FileStore.Read` 的硬上限保持 64 MB（`ReadOptions.MaxSize == 0`），超过 64 MB 返回 `CodeTooLarge` 通知无法打开。文件字节数 ≥ `Settings.Files.LargeFileBytes`（默认 5 MB）且 ≤ 64 MB 时正常读入，会话标记 `DocState.LargeFile = true` |
| D2 | 大文件模式的文档模型 | `DocMeta.Large bool` 为真时，`document.Factory` 不构造 `markdown.Parser`（用轻量 `plainAST` 实现 `contract.AST` 不变式：根节点 + 覆盖全文的单个纯文本叶子，零解析开销），`Apply` / `Undo` / `Redo` 只操作 `text.Text` 与历史栈。5–64 MB 打开与编辑都不跑 Markdown 解析，单向事务流、自动保存、崩溃恢复、字节无损完全复用现有 `Session` |
| D3 | 大文件模式的前端行为 | `DocState.LargeFile` 为真时：`shell.attach` 直接以 `editorapi.ModeSource` 打开，禁用切到实时/阅读模式（视图菜单置灰），顶部显示 `bannerLarge` 横幅，大纲面板显示「大文件模式下不生成大纲」，渲染服务不启动（08 D19） |
| D4 | 5 MB 源码模式的击键开销 | 改造 `views/source`：利用已知编辑区间 `[from, oldEnd) -> [from, newEnd)` 增量更新 `crlf` 行尾偏移表，不在每次击键后用 `strings.Index` 全篇重扫 5 MB 文本；补 `BenchmarkSourceType5MB` 断言单键耗时与分配 |
| D5 | 1 MB 实时模式的块增删局部化 | `liveedit.syncRows` 在叶子数变化但 `ev.Changes` 局限于局部区间时，复用前后未变叶子段拼接 `e.rows`，只更新变更段内的 `rowOf` 和 Fenwick 高度树，消除 `Enter` 拆段时的全篇 O(N) 重建 |
| D6 | GB18030 识别规则 | `fsstore` 在无 BOM 且 `!utf8.Valid(data)` 且 `!isBinary(data)` 时，用 `simplifiedchinese.GB18030.NewDecoder()` 解码并检查原字节流不含非法多字节序列（解码不产生新增 `\uFFFD`）：合法则设 `Meta.Encoding = "gb18030"` 返回文本，否则仍返回 `CodeEncoding`。`fsstore.Write` 拒绝直接写 `gb18030`（返回 `CodeEncoding`），防止有损回写 |
| D7 | 旧编码会话转换流程 | `docsession` 对 `Meta.Encoding == "gb18030"` 的文件初始化 `ReadOnly = true`。`Session` 增加 `ConvertToUTF8()`：在主线程将 `Model.SetMeta` 的 `Encoding` 改为 `"utf-8"`（保留原换行风格）、解除 `ReadOnly` 并标记为已编辑，用户保存或自动保存时按原子写落盘为 UTF-8 |
| D8 | HTML 块安全白名单范围 | 用 `golang.org/x/net/html` 解析单个未激活 `BlockHTML`：允许容器 `<p>`、`<div>`、`<h1>`–`<h6>`、`<blockquote>`、`<details>`、`<summary>`、`<hr>`、`<br>` 与内联 `<a>`、`<img>`、`<strong>`/`<b>`、`<em>`/`<i>`、`<code>`、`<u>`、`<s>`/`<del>`、`<kbd>`、`<mark>`、`<sub>`、`<sup>`；属性仅允许 `align`（`left`/`center`/`right`）、`src`（仅本地路径，遵循 `RemoteImages` 策略）、`alt`、`title`、`width`、`height`、`href`（仅 `http(s):`、`mailto:`、`#anchor` 与相对路径，严禁 `javascript:` / `data:` / `vbscript:`）、`open`。出现任何未知标签、`style`、`on*` 事件属性或残缺标签时，整块回退淡色源码 |
| D9 | 单击图片保留下方预览 | 在 `liveedit` 中，独占一段的单张图片块（`![alt](src)`）被光标激活展开源码时，参照数学块与 Mermaid 块的做法，在等宽源码行下方继续绘制当前已加载的图片预览（双击图片或预览区仍打开灯箱） |
| D10 | 自定义主题 JSON 格式 | 目录为 `PathUserData/themes/*.json`。结构：`{"name": "Solarized Paper", "dark": false, "accentHue": 182, "colors": {"window": "#...", "sheet": "#...", "text": "#...", ...}}`。先由 `theme.For(Options{Dark: dark, Hue: accentHue})` 生成完备默认令牌，再覆盖合法 `#RRGGBB` / `#RRGGBBAA` 字段。损坏文件记录日志并通知一次，回退默认主题 |
| D11 | 快捷键持久化与冲突处理 | 复用 `Settings.Keymap []contract.KeyBinding` 与 `core/commands` 现有的 `SetUserBindings` / `Conflicts`：录制要求含 `Ctrl`/`Cmd`/`Alt` 修饰键或 `F1`–`F12` 功能键（拒绝裸字符键）；与现有命令冲突时显示冲突命令名，确认覆盖后更新 `Settings.Keymap` 并立即同步注册表 |
| D12 | 自动更新接线 | 在 `internal/app` 注册 `mygo.Use(native.New(updater.Options{}))`；`Platform` 接口增加 `CheckForUpdates()` 与 `AutoUpdate() / SetAutoUpdate(bool)`（测试中为假实现），由文件菜单、关于页和命令面板 `app.checkUpdates` 调用；`mygo.json` 配置 `"updates": {"publicKey": "yql1LBiAqM209MYjz0ShS5Fk0C409222SHeYsw51okE=", "github": "oysterhyd/aurink"}` |
| D13 | 无障碍范围 | 基于 MyGo `ui/access.go` 的 `AccessTree`：所有无文字图标按钮补本地化 `Label`；模态框设 `RoleDialog`；开关/复选框报告 `Checked`；滑块报告 `Range`；命令面板与斜杠菜单输入框设 `ActiveDescendant`；`liveedit` 表面暴露文档标签与只读状态 |
| D14 | 内嵌资源去重与滚动日志 | 将 `resources/fonts/*.ttf` 与 `resources/render/*` 迁入 `internal/assets/{fonts,render}` 做 `go:embed`（同步更新 `03 §1.1` 与 `tests/arch`）；`resources/` 仅保留 `icon.png` 与 `licenses/`，消除安装目录约 11.8 MiB 重复文件与 `.go` 源文件。`internal/app` 启动时将 `slog` 接入 `PathUserData/logs/aurink.log`（5×5 MB 轮转） |
| D15 | Windows CI 发布流水线 | 新增 `.github/workflows/release.yml`（推送 `v*` 标签或手动触发）：跑完门禁后在 `windows-latest` 执行 `go tool mygo build -platform windows/amd64 -upload`（注入 `MYGO_UPDATER_PRIVATE_KEY` 与 `GITHUB_TOKEN`），自动生成 NSIS 安装包、签名更新包、`update-windows-amd64.json` 与 `checksums.txt` 并上传至 GitHub Releases 草稿 |

## 6. 任务要点

### 批次 2：BE-1.8 大文档 + BE-2.11 旧编码（Codex）

- **测试要求：**
  - 5.5 MB Markdown 文件 `Open` 成功，`State().LargeFile == true`、`Model().Meta().Large == true`，编辑、`Save`、崩溃恢复后字节 100% 一致，且不触发 Markdown AST 解析。
  - 超过 64 MB 上限的文件返回 `CodeTooLarge`。
  - GB18030 / GBK 编码样本（含中文标点、表格、代码块）打开后 `State().ReadOnly == true`、`Meta().Encoding == "gb18030"`；直接 `Apply` 返回 `CodeReadOnly`；调用 `ConvertToUTF8()` 后 `ReadOnly == false`、`Meta().Encoding == "utf-8"`，`Save` 后磁盘为合法 UTF-8 且内容逐字相等。
  - 非法字节流（既非 UTF-8 也非合法 GB18030）仍返回 `CodeEncoding`；含 NUL 字节的二进制文件仍返回 `CodeBinary`。

### 批次 3：FE-1.8 第一部分（性能、大文件前端、旧编码横幅）

- `shell` 对 `LargeFile` 标签锁定源码视图并显示大文件横幅；对 `gb18030` 只读标签在横幅显示编码说明及「转为 UTF-8 并编辑」「另存为」两个按钮，补 `tests/qa` 场景测试。
- `source` 补 `BenchmarkSourceType5MB`（300 次击键），验证单键耗时 < 2 ms。
- `liveedit` 局部行同步完成后，收紧 `tests/perf/work_test.go` 中 1 MB `Enter` 的分配上限断言，并在真窗口 `tools/windowcheck` 复测 200 KB 与 1 MB 帧工作 p50/p95。

### 批次 4：FE-1.8 第二部分（HTML 块安全子集、图片源码预览、无障碍）

- HTML 块白名单必须过**安全负向测试**（`<script>`、`<img onerror=...>`、`<a href="javascript:...">`、`<iframe>`、`<style>`、远程 `https://` 图片等 15+ 样例），断言无任何网络请求、无危险跳转、未匹配的块原样回退淡色源码，且编辑其他块保存后 HTML 块字节一字不改。
- 单击图片行进入编辑时断言源码与下方图片预览同时存在；光标离开后恢复纯图片展示。
- 无障碍：在 `tests/qa/a11y_test.go` 遍历主界面、侧栏三面板、查找栏、命令面板、斜杠菜单、格式条、设置对话框的所有可聚焦/可点击元素，断言零无名按钮/控件。

### 批次 5：FE-2.8 自定义主题、快捷键修改、标签条渐隐

- 自定义主题测试：合法 JSON 覆盖色值生效、缺省字段继承基础明暗主题、非法 JSON 不崩并回退默认。
- 快捷键测试：录制新键位后按新键触发命令、旧键失效、重启后从 `settings.json` 恢复、冲突提示与覆盖、恢复默认后回到 `DefaultBindings()`。
- 标签条溢出时左右边缘渐隐遮罩绘制正常，不挡点击。

### 批次 6：INT-4.1 自动更新、日志、瘦身打包、CI 发布与文档

- 接入 `updater/native`，补 `CHANGELOG.md`（含 `## 1.0.0` 章节），跑 `MYGO_UPDATER_PRIVATE_KEY=... go tool mygo build -platform windows/amd64` 实测生成 `aurink-setup-1.0.0-amd64.exe`、签名归档与 `update-windows-amd64.json`，验证安装目录无 `.go` / 重复 `.ttf` / `.js`，且许可证齐全、安装卸载与双击打开正常。
- 验证 `aurink.log` 写入与滚动轮转。
- 编写根目录 `README.md` 与 `.github/workflows/release.yml`。

### 批次 7：QA-4.1 与 M4 最终验收

- 在 `tools/windowcheck` 增加启动、1 MB 打开与 100 KB 空闲内存测量模式，在本机 Windows 真窗口实测 03 §10 全部行项并写入 `reports/quality.md`。
- 逐条核对 05 §8 九项最终验收标准，写入 `docs/adr/README.md` `# M4 最终评估`，请用户 Windows 真机终审。

## 7. 性能与资源预算（03 §10 最终核查表）

| 指标 | 03 §10 预算 | 当前基线（M3） | Phase 4 目标与验证方式 |
|---|---|---|---|
| 冷启动到首帧 | < 500 ms | 未单列真窗口埋点 | `windowcheck` 真窗口埋点实测 < 500 ms |
| 打开 1 MB 文档到首屏 | < 300 ms | Tester `BenchmarkOpen1MB` 83 ms | 真窗口首屏埋点 < 300 ms |
| 击键帧工作 p95（≤200 KB） | < 8 ms | Tester 1.33 ms；真窗口帧工作 4.9–5.1 ms | 保持帧工作 p95 < 8 ms |
| 击键帧工作 p95（≤1 MB） | < 16 ms | Tester 2.60 ms；真窗口帧工作 15.1–15.4 ms；`Enter` 48.4 万次分配 | 局部行同步优化后帧工作 p95 < 12 ms（严守 < 16 ms），`Enter` 分配显著下降 |
| 5 MB 大文件源码编辑 | 流畅打开与编辑 | 当前 >5 MB 报 `CodeTooLarge` | 打开 < 200 ms，源码模式单键 `BenchmarkSourceType5MB` < 2 ms |
| 1 万块文档滚动 | 60 fps，无整屏重排 | 1.28 ms/op，8.2 shapes/frame | 保持达标 |
| 空闲 CPU / 帧 | 0 帧（极光 12 s 后 0 帧） | 0 帧 | 保持达标 |
| 空闲内存（100 KB 文档，无渲染服务） | < 150 MB | 未单列实测 | 实测进程 Private Bytes / Working Set < 150 MB |
| 发布体积（去重后） | < 40 MB | `Aurink.exe` 27.57 MiB，安装包 13.96 MiB（安装目录另重复 11.8 MiB） | 单文件约 28 MiB、安装目录总计 < 30 MiB |

## 8. 主要风险

| 风险 | 影响 | 应对 | 发现时点 |
|---|---|---|---|
| 大文件无 AST 模式下某处代码假定 `AST` 必有叶子块 | 空指针或越界 panic | `plainAST` 严格满足 `contract.AST` 全部不变式（批次 1 用 `contracttest.TestAST` 覆盖）；`shell` 在大文件模式下锁定源码视图 | 批次 1、2 |
| GB18030 误判其它字节流 | 显示乱码 | 严格解码零替换符才识别为 `gb18030`，且默认只读不改原盘，零数据损坏风险 | 批次 2 |
| HTML 块安全渲染引入布局抖动或注入 | 破坏无损承诺或安全边界 | 严格白名单 + 禁脚本协议 + 光标进入立刻回退源码 + 15+ 例负向安全测试 | 批次 4 |
| 用户把裸字母键录成全局快捷键导致无法打字 | 编辑器无法输入 | 录制器强制要求含 `Ctrl`/`Cmd`/`Alt` 修饰键或 `F1`–`F12` 功能键 | 批次 5 |
| 自动更新私钥丢失导致旧版无法验证新版 | 已安装客户端无法升级 | 私钥存于本机 `%APPDATA%\mygo\update-keys\mygo-update.key` 并同步写入 GitHub Actions Secret `MYGO_UPDATER_PRIVATE_KEY`，`.gitignore` 显式拦截 `.key` 文件防止泄露 | 开工前 |

## 9. M4 最终验收（对照 05 §8，Windows 1.0 范围）

M4 关闭前逐项输出 PASS / FAIL 证据：

1. **需求覆盖：** 02 文档 P0 100%、P1 100%、本计划 §3.1 列入的 P2 项 100% 对应自动化测试与真窗口验证。
2. **架构一致性：** `tests/arch` 通过；契约与 04 文档一致；`mygo vet` 零告警；`contract` 仅依赖标准库。
3. **代码质量：** `gate.ps1 -AllTargets` 全绿；核心包测试覆盖率达标；无未登记 `TODO`；核心路径零 panic。
4. **无损保真：** 语料库（含扩展开/关、CRLF、BOM、混合换行、HTML 块、5.5 MB 大文件、GB18030 转换）字节保真 100%；3 种子 × 10,000 步随机编辑属性测试通过；Typora 104 场景对拍通过。
5. **性能与内存：** §7（03 §10）全部指标附本机 Windows 真窗口实测数据达标，写入 `reports/quality.md`。
6. **可靠性：** 原子写故障注入、kill 进程崩溃恢复、19 格外部冲突矩阵、滚动日志落盘全部通过。
7. **体验与无障碍：** 18 张设计基线对照记录齐全；中英文界面完整无截断；无障碍树测试通过；图片源码下预览、标签条渐隐、自定义主题与改键可用。
8. **发布与自动更新：** `aurink-setup-1.0.0-amd64.exe` 与签名更新包构建成功；Windows 本机实测安装、文件关联、更新检查窗口、卸载全流程通过；`release.yml` 与根目录 `README.md` 就绪（macOS / Linux 按 U3 决定不在 1.0 支持范围，标注为未构建/未验证）。
9. **遗留风险：** 输出 1.0 已知限制与后续 1.x 演进清单。

## 10. 进度

| 批次 | 状态 | 提交 | 备注 |
|---|---|---|---|
| 1 | 已完成 | `b3caa5a` | 契约与 `contracttest` 套件（`DocMeta.Large`、GB18030 只读与 `ConvertToUTF8`、`CmdAppCheckUpdates`） |
| 2 | 已完成 | `56e23e5` | BE-1.8 大文档无 AST `plainAST` 模式 + BE-2.11 GB18030 严格解码与只读转换 |
| 3 | 已完成 | `65945f4` | FE-1.8a：1 MB `Enter` 局部化（分配 48.4 万 → ~1,055，真窗口帧工作 p95 3.0 ms）、5.5 MB 分块文本与增量 CRLF（`Sync` 71 µs）、大文件与旧编码横幅 |
| 4 | 已完成 | `c1877a1` | FE-1.8b：安全 HTML 块白名单组件（26 例负向安全测试通过）、单击独占图片源码下保留预览、全界面无障碍标签与静态/运行时双审计 |
| 5 | 已完成 | `89d65b5` | FE-2.8：`themes/*.json` 自定义主题解析与切换、快捷键搜索/录制/冲突覆盖/重置、标签条横向溢出渐隐遮罩 |
| 6 | 已完成 | `0acdea3` | INT-4.1：`internal/assets` 去重瘦身（安装包 13.96 → 11.09 MiB，安装目录零重复资源）、5×5 MB 滚动日志、`updater/native` 签名更新、`v1.0.0` 构建与本机卸载/全新安装验证 |
| 7 | 已完成 | 本次提交 | QA-4.1：`windowcheck -perf` 真窗口全表实测达标、`gate.ps1 -AllTargets` 全绿、`reports/quality.md` 与 `adr/README.md` M4 最终评估落盘 |
