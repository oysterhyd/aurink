# Aurink 规划文档包（v1.2 · 2026-11-01）

> 状态：**规划完成；§5 决策项已于 2026-10-31 全部确认；M0–M3 均已通过（2026-11-12 用户确认 M3 真机验收），待进入 Phase 4**。评估见 [adr/README.md](adr/README.md)，实施记录见 [06](06-phase1.md)、[07](07-phase2.md)、[08](08-phase3.md)。
>
> 仓库：<https://github.com/oysterhyd/aurink>（私有）· 集成分支 `main` · CI：`.github/workflows/ci.yml`

## 1. 一句话结论

**可行，但必须自研编辑引擎。** MyGo v0.7.3 的 Native UI 提供了自研所需的全部底层能力（带字节范围的增量 Markdown 解析器、自定义 `TextInputClient` 的 IME、富文本排版与命中测试、Glass/Spring/Shader/Shared Element），已在本机实测确认；但它**没有**可直接使用的"无损所见即所得 Markdown 编辑器"。因此 Aurink 采用自研的 **Live Markdown 引擎**：源码是唯一真相，AST 按块投影为显示片段，光标处显示源码标记、其余隐藏或渲染成组件，所有编辑都是源码字节替换——**构造上无损**。

## 2. 文档索引

| 文档 | 内容 |
|---|---|
| [01-feasibility.md](01-feasibility.md) | 环境盘点、MyGo 能力矩阵、探针实测 P1–P14、难点与方案、风险 R1–R10、Spike S1–S4（含 Go/No-Go 标准） |
| [02-product-ux.md](02-product-ux.md) | 产品定位、P0–P3 功能清单、布局、设计令牌、动效令牌、交互规范、材质/Shader 规范、快捷键、无障碍与 i18n |
| [design/README.md](design/README.md) | Ink & Aurora 界面基线：可运行原型、18 张基线截图、基线与任务卡和阶段门的对照表、验收方法 |
| [03-architecture.md](03-architecture.md) | 分层与依赖规则、线程模型、单向数据流、各模块设计、状态管理、扩展机制、错误处理、性能预算、目录结构 |
| [04-contracts.md](04-contracts.md) | 统一接口契约：线程规则、不变式、事件、错误码、Mock 与一致性测试 |
| [05-delivery-plan.md](05-delivery-plan.md) | 工作方式、代码规则、门禁与 CI、Phase 0–4 任务卡、阶段门、最终验收清单 |
| [06-phase1.md](06-phase1.md) | Phase 1 实施计划：M1 范围、批次顺序、Codex 流程、任务要点、风险、验收 |
| [07-phase2.md](07-phase2.md) | Phase 2 实施计划：M2 范围、M1 留下的缺口、批次顺序、设计决定、风险、验收 |
| [08-phase3.md](08-phase3.md) | Phase 3 实施计划：M3 范围、待用户决定的问题、批次顺序、渲染与扩展语法的设计决定、风险、验收 |
| [09-phase4.md](09-phase4.md) | Phase 4 实施计划：M4 范围、大文档与性能加固、旧编码与安全 HTML 块、自定义主题与改键、三平台发布与最终验收 |
| `../spikes/00-probe/` | 可行性探针（独立模块，`go test -v ./...` 复现 P1–P14） |
| `../spikes/01-contract-rc/` | 契约 Go 代码 RC（独立模块，仅依赖标准库，`go vet`/`go build` 通过） |

依据标记：**[实测]** 本机运行验证 · **[源码]** 阅读 MyGo 源码/文档确认 · **[待验证]** 需 Spike 验证，未验证前不作为既定事实。

## 3. 关键决策（ADR 摘要）

| # | 决策 | 理由 | 依据 |
|---|---|---|---|
| D1 | 使用 MyGo **Native UI**（不用 WebView 页面），锁定 v0.7.3 | 原生性能、GPU 特效、纯 Go 无 cgo、三平台交叉编译 | [实测] 构建与交叉编译、[源码] |
| D2 | **自研 Live Markdown 引擎**（方案 C）；`ui/richtext` 否决；`ui/editor` 仅作源码模式 | richtext 的 Markdown 往返结构性有损；editor 为等宽统一行高 | [实测] P1–P5、P13；[源码] |
| D3 | **源码文本 = 唯一真相**，不可变 string 快照 + 增量解析 | 无损的前提；解析器才是 O(n) 瓶颈，rope 收益有限 | [实测] P11、P12 |
| D4 | 按块缓存排版 + Fenwick 高度树虚拟滚动 | 排版约 230 µs/段，全量重排不可接受 | [实测] P4 |
| D5 | IME 文档 = 活动块显示文本；预编辑不写入源码 | 显示文本与源码不一一对应；预编辑不应进入历史/自动保存 | [实测] P1、P2；三平台行为 [待验证] S1 |
| D6 | 自建 `docsession`/`fsstore`，不用 `mygo.NewDocuments` | 需要工作区 + 多标签；NewDocuments 是每文档一窗口且原子写未导出 | [源码] |
| D7 | 数学/Mermaid/PDF 使用**懒启动离屏 WebView 渲染服务**，不可用时降级为源码 | 框架无原生能力；MathJax SVG 全为 path，适配 MyGo SVG 子集 | [源码]；可行性 [待验证] S2 |
| D8 | 进程内 Go 接口通信（无 HTTP）；`Dispatcher` 回主线程；跨模块错误统一为 `*contract.Error` | 原生单进程应用，最低延迟、类型安全、易 Mock | 设计决策 |
| D9 | 分层 contract → core → adapters → views → app，导入规则由 `tests/arch` 强制 | 保持依赖清晰、core 可纯单元测试 | 设计决策 |
| D10 | 面向 contract 接口编程；不冻结，变更与实现、测试同一次提交 | 2026-11-01 取消多 Agent 并行后，冻结和 CR 流程没有必要 | 用户决定 |
| D11 | 界面基线为 **Ink & Aurora**（`docs/design`）；M1–M4 按基线对照验收 | 用户审查原型后确认 | 用户确认 2026-11-01；Native 观感已由用户在 S4 确认 |
| D12 | **打包衬线字体**：Newsreader + Noto Serif SC 子集（OFL），另含 JetBrains Mono | 编辑式排版是基线的核心 | 用户决定 2026-11-01；S4 实测 6.3 MB |

## 4. 已验证 vs 待验证

**已验证 [实测]**（证据在 `spikes/00-probe`、`spikes/01-contract-rc`，2026-10-31 在本机重新运行通过）
- Go 1.27.3 工具链与 `mygo` 0.7.3 CLI 可用；Native UI 工程在 Windows 构建成功，可交叉编译到 macOS/Linux/Windows arm64；最小 release 体积 9.9 MB。
- 自定义 `TextInputClient` 能接收键入与 IME 组字/提交（Tester 模拟）。
- 混合字号/粗细/CJK/等宽背景/内联对象的排版、光标、命中测试。
- 解析器字节范围可推导语法标记；CRLF 原生支持；表格/数学块/任务项/脚注正确；增量解析与全量一致（890 KB、200 次编辑）。
- 已量化的限制：排版约 230 µs/段；增量解析单次约 6 ms/MB；Front Matter 与扩展语法不识别；转义/实体合并进 Text 节点；`editor.Buffer` 会把混合换行归一化。**每条都有对应方案**（见 01 §4）。
- 契约 RC（2026-11-01 按界面基线修订后重新 `go vet`、`go build`）共 13 个 Go 文件，可编译，`go vet` 清洁，只依赖标准库。
- **[实测-CI]** 2026-11-01 首次 CI（run 41280628576）：探针测试（无窗口 Tester 键入/IME 模拟、混排排版、解析器、增量解析一致性、缓冲区保真）在 **Windows、Linux、macOS** 全部通过；5 个目标交叉构建、`go vet`、`mygo vet` 通过。注意：这只证明无窗口逻辑可跨平台运行，**不代表**真机 IME、字体渲染、Vibrancy 已验证。

**待验证 [待验证]**（Phase 0 Spike 负责，未通过前不得视为可行）
- S1：自研引擎的显隐编辑、跨块光标/选区、1 万块虚拟滚动、击键 p95 < 8 ms、**Windows 微软拼音真机**候选框位置。
- S2：离屏 WebView 不挂载时 `Eval` 是否稳定；MathJax SVG 被 `ui.ParseSVG` 正确解析的比例；渲染延迟与内存。
- S3：前缀不变式在语料上 100% 成立；BlockID 在随机编辑下保持稳定；5 MB 解析曲线。
- S4：设计基线的窗口材质、三级玻璃、共享元素、滑动药丸、极光在 GPU 路径 60 fps；低功耗路径；打包字体的体积与注册耗时；与基线的观感差异由用户确认。
- **macOS 与 Linux 上有窗口的运行时行为**（IME、字体、Vibrancy、菜单、文件关联、签名）：本机只有 Windows；CI 只能跑无窗口测试。这些项必须在 CI 或真机上验证，验证前一律标记为"未验证"。

## 5. 已确认的决策（2026-10-31，用户确认）

| # | 问题 | 决定 |
|---|---|---|
| Q1 | 平台优先级 | **Windows 先行**；macOS/Linux 构建与无窗口测试在 CI 上跑，Phase 4 前在真机人工验收 |
| Q2 | 数学/Mermaid/PDF 依赖系统 WebView | **接受**（WebView2 / WKWebView / WebKitGTK，懒启动，不可用时降级为源码） |
| Q3 | 开发方式 | 原定 6 个并行 Agent；**2026-11-01 改为一位负责人、只用 `main`**，并行没有节约时间，每次合并等 CI 的时间比改代码还长；同日决定后端交给 Codex 实现，负责人验收，前端由负责人实现 |
| Q4 | Go 模块路径与应用标识 | 模块 `aurink`；identifier `app.aurink.editor` |
| Q5 | `git init` | **已执行 `git init -b main`**；worktree 已于 2026-11-01 全部移除 |
| Q6 | GitHub 与 Actions | **已创建私有仓库 `oysterhyd/aurink` 并启用 CI**（三平台矩阵，见 05 §5.2） |

## 6. 下一步

M0–M3 均已通过，见 [adr/README.md](adr/README.md) 与 [08-phase3.md](08-phase3.md)。Phase 4 按 [09-phase4.md](09-phase4.md) 进行。
