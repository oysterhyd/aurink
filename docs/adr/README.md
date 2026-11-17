# M0 评估（2026-11-01）

**结论：M0 通过。** 用户已于 2026-11-01 确认下面 3 项。用户要求暂不进入 Phase 1。每项依据在各 ADR 和对应的 `spikes/*/REPORT.md` 中。

| M0 条件 | 状态 |
|---|---|
| 门禁 Windows 通过，三平台交叉编译通过 | 通过：`gate.ps1 -AllTargets`；最近的 CI 通过 |
| CR-001 覆盖规则在语料上验证 | 通过：无缝无叠。隐藏文本的 2 种情形留给 `core/markdown` |
| S1–S4 报告齐全，有 Go/No-Go 结论 | S1 Go；S2、S3 按原标准 No-Go，后备方案已确认；S4 Go |
| No-Go 的后备方案由用户确认 | 已确认 |
| S4 的观感差异和字体预算由用户确认 | 已确认 |
| ADR 记录 | 本目录 |

## 用户已确认

1. **S2 公式后备（[ADR-002](ADR-002-render.md)）：** 能转成矢量的公式用 SVG，含中文等 `<text>` 的公式用 PNG。渲染服务懒启动，空闲后退出，用来控制 361–565 MiB 的 WebView2 工作集。
2. **S3 标准调整（[ADR-003](ADR-003-ast.md)）：** 不再要求“删除前缀后内容等于 `parser.Lines`”对所有叶子块成立，改用 `Block.Lines` 的逐行映射，该映射 1,934/1,934 通过。
3. **S4 观感（[ADR-004](ADR-004-look.md)）：** 真机运行原型，判断与基线的差异能否接受。

# M2 评估（2026-11-02）

**结论：M2 通过。** 用户 2026-11-02 确认真机检查已过（0.2.0 安装包）：真实文件夹加外部编辑、微软拼音、粘贴网页和 Word 内容、观感与基线（07 §9 归用户的四项），没有报告问题。负责人的证据见 07 §9、§10 和 `reports/quality.md`（`517d178`，三平台 CI 通过）。

| M2 条件 | 状态 |
|---|---|
| 门禁、三平台 CI | 通过 |
| 语料保真、1 万步属性测试 | 通过 |
| 文件树、监听、回收站、冲突矩阵（19 格） | 通过 |
| 标签和会话恢复、kill 后恢复 | 通过 |
| 面板、查找替换、大纲、全文搜索 | 通过 |
| 表格、图片、脚注、导出 HTML | 通过 |
| 真窗口 E2E 和原生对话框 | 通过，只在本机 Windows 运行，不在 CI |
| 性能 | 200 KB 帧工作 p95 约 5 ms；1 MB 约 15 ms，记录在案，留给 Phase 4 |
| 基线对照（含 150% 缩放和中文界面） | 通过：`reports/parity/` |
| 用户真机四项 | 用户确认 |

只在本机一种硬件上验证。macOS、Linux 的真机行为仍是“未验证”。

**产品定位（用户 2026-11-02）：** Aurink 是 Typora 的升级版。Typora 已有的行为和观感以 Typora 为准；Typora 没有的功能（斜杠菜单、格式条、结构导轨、Wiki 链接等）照 02 保留。扩展语法默认全部开启。

# M4 最终评估（v1.0.0 · 2026-11-18）

**结论：负责人侧自动化门禁、性能全表与 Windows 1.0 打包安装验证全部通过（PASS），提交用户进行 Windows 真机终审。** 对照 [05-delivery-plan.md](../05-delivery-plan.md) §8 与 [09-phase4.md](../09-phase4.md) §9 九项最终验收标准逐项核查如下（数据详见 `reports/quality.md`）：

| # | 05 §8 / 09 §9 验收项 | 结论 | 实测证据 |
|---|---|---|---|
| 1 | **需求覆盖**（02 P0 100%、P1 100%、09 §3.1 P2 100%） | **PASS** | P0 全项在 M1/M2 验证通过；P1 21 项在 M3 逐项验证通过（`reports/quality.md`）；09 §3.1 纳入的全部 P2 与体验打磨项（5–64 MB 大文件模式、GB18030 只读打开与转 UTF-8、安全 HTML 块子集渲染、单击图片保留下方预览、标签条溢出边缘渐隐、自定义 JSON 主题、快捷键录制与冲突覆盖、`updater/native` 自动更新、全界面无障碍、滚动文件日志）100% 具备自动化测试与本机实测证据。分屏、多窗口、跨文件替换、冲突差异对比、「跟随系统强调色」按 2026-11-18 用户决定 U2 留待 1.x |
| 2 | **架构一致性** | **PASS** | `tests/arch`（含新增 `internal/assets` 依赖规则）通过；契约变更记录于 `docs/04-contracts.md` 并由 `internal/contracttest` 守护；`go tool mygo vet .` 零告警；`internal/contract` 仅依赖标准库 |
| 3 | **代码质量** | **PASS** | `scripts/gate.ps1 -AllTargets`（`gofmt`、五目标 `go vet`/`go build`、`-tags mygo_noinspector`、`mygo vet`、全套 `go test -count=1 ./...`、`tools/i18ncheck` 334 条双语文案）全绿；全仓 Go 源码未登记 `TODO`/`FIXME` 数为 **0**；命令注册表与后台入口具备错误包装与 panic 防护 |
| 4 | **无损保真** | **PASS** | 83 份语料库文件（扩展全开与全关双模式）打开保存字节 100% 一致；3 种子 × 10,000 步随机编辑/撤销/重做/保存属性测试（LF/混合、CRLF、UTF-8 BOM）100% 匹配纯字符串模型；Typora 104 场景对拍 0 pending；5.5 MB CRLF 大文件、GB18030 转 UTF-8、26 种非常规/危险 HTML 块旁路编辑保存字节 100% 无损 |
| 5 | **性能与资源预算（03 §10 全表）** | **PASS** | 本机 Windows 11 真窗口双轮实测（`tools/windowcheck -perf`，见 `reports/quality.md`）：冷启动首帧 **`103.8–177.8 ms`**（预算 `< 500 ms`）；打开 1 MB 首屏 **`81.7–83.7 ms`**（`< 300 ms`）；打开 5.5 MB 大文件首屏 **`58.3–69.3 ms`**（`< 200 ms`，`Sync` 单键 **`71 µs`**）；200 KB 击键帧工作 p95 **`2.17–2.26 ms`**（`< 8 ms`）；1 MB 击键帧工作 p95 **`3.00–3.12 ms`**、`Enter` 拆段 p95 **`4.58–4.89 ms`**（`< 16 ms`，`Enter` 分配从 48.4 万降至 **`~1,055`**）；1 万块滚动 `1.28–1.34 ms/op`（8.2 shapes/frame）；空闲 **0 帧**；100 KB 文档无渲染服务空闲内存 Working Set **`109.2–133.2 MiB`** / Private Bytes **`122.4–140.2 MiB`**（`< 150 MB`）；`Aurink.exe` **`32.38 MiB`**、安装目录总计 **`32.73 MiB`**（`< 40 MB`） |
| 6 | **可靠性** | **PASS** | `fsstore` 原子写故障注入、`tests/e2e` kill 进程崩溃恢复、`docsession` 19 格外部修改冲突矩阵、`internal/app` 5×5 MB 滚动日志轮转与真机启动落盘（`%APPDATA%\Aurink\logs\aurink.log`）全部通过 |
| 7 | **体验与无障碍** | **PASS** | 18 张设计基线对照齐全（M3 遗留的「单击图片保留预览」与「标签条边缘渐隐」两项小差异已在本阶段补齐）；中英文 334 条文案及 `en-XA` 伪本地化零截断；`tests/qa/a11y_test.go` 双语全界面运行时无障碍断言 + `internal/views` 静态 AST 标签审计通过 |
| 8 | **发布与自动更新（Windows 1.0）** | **PASS（Windows）/ 未验证（macOS·Linux）** | 公开仓库 `oysterhyd/aurink` + Ed25519 密钥与 GitHub Secret 就绪；`go tool mygo build -platform windows/amd64` 产出 `aurink-setup-1.0.0-amd64.exe`（`11.09 MiB`）、签名归档 `aurink-1.0.0-windows-amd64.tar.gz` 与 `update-windows-amd64.json`；本机实测旧版卸载清零、1.0.0 静默安装、安装目录零 `.go`/`.ttf`/`.js` 冗余、许可证齐全、`.md` 文件关联 ProgID 注册、启动与退出正常；`.github/workflows/release.yml`、`CHANGELOG.md`、根目录 `README.md` 就绪。**macOS 与 Linux 安装包及真机运行按用户决定 U3 不在 1.0 发布范围，明确标记为「未构建 / 未验证」** |
| 9 | **遗留风险与 1.x 演进清单** | **已登记** | 见下节 |

### 1.0 已知限制、风险与 1.x 演进建议

1. **平台与硬件覆盖边界：** v1.0.0 仅面向 `windows/amd64` 发布与真机验证；`windows/arm64` 物理机、低功耗纯核显/无独显老机型、非 100%/150% DPI 缩放档位，以及 macOS / Linux 有窗口运行（系统菜单、IME、Vibrancy、WKWebView/WebKitGTK 渲染、打印与安装包签名公证）均为**未验证**，后续若扩展跨平台发行需按 05 §8 补齐真机验证与平台证书签名。
2. **Windows 代码签名（SmartScreen）：** 当前构建已启用 Ed25519 自动更新归档签名（保障更新防篡改），但尚未配置商业 Windows Authenticode 代码签名证书（EV/OV 或 Azure Trusted Signing），首次下载运行安装包时 Windows SmartScreen 会弹出未知发布者提示；可在 1.x 按 `release.yml` 预留机制接入证书。
3. **超大单块极端场景：** 常规 Markdown 编辑走 24 KiB 局部重解析窗口（200 KB p95 < 0.81 ms，1 MB p95 < 3.4 ms），≥ 5 MB 文档自动进入无 AST 源码模式；若在 1–5 MB 文档中编写单个超过 24 KiB 的巨型嵌套容器或未闭合围栏，解析器会按设计回退受计数的全量保守重解析（约 11–14 ms）。
4. **1.x 功能演进候选：** 源码+预览同步分屏、多窗口同时编辑、跨文件全文替换、外部冲突行级 Diff 视图、导出长图、可选跟随后端系统强调色。

# M3 评估（2026-11-12）

**结论：M3 通过。** 用户于 2026-11-12 确认完成 M3（0.3.0）真机审查。此前真机反馈的 Typora 按键、命令与交互差异已全部通过 `tools/parity`（104 个真机对拍场景，`pending` 清零，仅保留 8 项有意偏离）及 `BE-TYPORA-KEYS`、`BE-TYPORA-EDIT` 等卡修复合入。负责人的各项证据见 [08-phase3.md](../08-phase3.md) §10、`reports/quality.md`、`reports/render/`、`reports/pdf/` 与 `reports/parity/`。

| M3 条件 | 状态 |
|---|---|
| 门禁、五目标交叉构建（`gate.ps1 -AllTargets`） | 通过 |
| 语料保真、1 万步属性测试（扩展全开与全关双模式） | 通过：`tests/fidelity`（83 文件 + 3×10,000 步 × 2 模式）、`tests/qa`（3×10,000 步 × 2 模式） |
| Typora 键位与编辑行为对拍 | 通过：`TestTyporaParity` 104 场景（0 pending，8 项记录在案的有意偏离） |
| 02 P1 功能清单逐项覆盖 | 通过：21 项全部对应测试与视觉证据，见 `reports/quality.md` |
| 渲染服务可用与不可用降级两态 | 通过：`tools/rendercheck`（210 公式 p95 9.2 ms、8 图表、缓存微秒级、空闲退出回收）+ `tests/qa/render_test.go` + `reports/render/` |
| 导出 PDF 与打印 | 通过：`tools/pdfcheck`（A4 7 页、Letter 8 页逐页转图核对，`reports/pdf/`）+ `tests/qa/pdf_test.go` |
| 02 §4.4 运动规则走查、`ReduceMotion` / `ReduceTransparency` 合规 | 通过：`reports/parity/Motion.md`、`LowPower.md`（标记 `Quick` 淡入与列表 `Stagger` 未做，记为小差异） |
| 空闲 0 帧（含极光 12 s 后停帧） | 通过：`FramePending` 断言通过 |
| 中英文界面完整与实时切换 | 通过：311 条文案 `i18ncheck` 通过、`en-XA` 伪本地化无截断、中英文双套截图 |
| 18 张设计基线对照（含 150% 缩放、中文界面与独立盲审复核） | 通过：`reports/parity/Phase3.md`（标签条边缘渐隐、设置「跟随系统强调色」、单击图片保留预览维持现状，记为小差异） |
| Windows 0.3.0 安装包安装、卸载、文件关联 | 通过：本机实操验证（08 §10 批次 9） |
| 公式/图表/Front Matter 观感、各模式/斜杠菜单/格式条/灯箱手感、设置与语言切换、微软拼音、PDF 效果 | 用户真机审查确认 |

只在本机一种 Windows 硬件上验证。核显机型以及 macOS、Linux 上的有窗口运行（IME、字体、Vibrancy、系统菜单、WebView 渲染、PDF 与打印、文件关联）均标记为**未验证**，留待 Phase 4（QA-4.1）验收。

# M1 评估（2026-11-02）

**结论：M1 通过。** 用户 2026-11-02 确认真机验收已过：日常使用（打开、编辑、保存、切模式、切主题）、微软拼音、观感与基线（`docs/06-phase1.md` §8 归用户的三项），没有报告问题。负责人的各项证据见 06 §9 和 `reports/quality.md`。

| M1 条件 | 状态 |
|---|---|
| 门禁、三平台构建 | 通过 |
| 语料打开保存字节一致 | 通过 |
| 随机编辑属性测试 | 通过：3 个种子各 1 万步 |
| ≤ 200 KB 击键 p95 < 8 ms | **未达标，接受差异**：Tester 中 3.7 ms，真窗口 9–17 ms。用户 2026-11-01 要求不再纠结耗时，2026-11-02 确认 M1 通过。标准原文不改；Phase 4 FE-1.8 的性能预算仍按 8 ms 复查 |
| 1 万块滚动无整屏重排 | 通过 |
| kill 后恢复 | 通过 |
| 基线对照 | 通过：`reports/parity/` |
| 日常使用、微软拼音、观感 | 用户真机确认 |

只在本机一种硬件上验证。macOS、Linux 的真机行为仍是“未验证”。

## 记录

- [ADR-001 编辑引擎](ADR-001-editor.md)
- [ADR-002 公式与图表渲染](ADR-002-render.md)
- [ADR-003 AST 适配](ADR-003-ast.md)
- [ADR-004 外观与字体](ADR-004-look.md)
