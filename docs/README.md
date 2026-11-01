# Aurink 规划文档包（v1.0 · 2026-10-31）

> 状态：**规划完成；§5 决策项已于 2026-10-31 全部确认；仓库与基线 CI 已建立；可以进入 Phase 0**。本包只包含规划与可行性证据，尚未进行任何产品开发。
>
> 仓库：<https://github.com/oysterhyd/aurink>（私有）· 集成分支 `main` · CI：`.github/workflows/ci.yml`

## 1. 一句话结论

**可行，但必须自研编辑引擎。** MyGo v0.7.3 的 Native UI 提供了自研所需的全部底层能力（带字节范围的增量 Markdown 解析器、自定义 `TextInputClient` 的 IME、富文本排版与命中测试、Glass/Spring/Shader/Shared Element），已在本机实测确认；但它**没有**可直接使用的"无损所见即所得 Markdown 编辑器"。因此 Aurink 采用自研的 **Live Markdown 引擎**：源码是唯一真相，AST 按块投影为显示片段，光标处显示源码标记、其余隐藏或渲染成组件，所有编辑都是源码字节替换——**构造上无损**。

## 2. 文档索引

| 文档 | 内容 | 读者 |
|---|---|---|
| [01-feasibility.md](01-feasibility.md) | 环境盘点、MyGo 能力矩阵、探针实测 P1–P14、难点与方案、风险 R1–R10、Spike S1–S4（含 Go/No-Go 标准） | 全员 |
| [02-product-ux.md](02-product-ux.md) | 产品定位、P0–P3 功能清单、布局、设计令牌、动效令牌、交互规范、材质/Shader 规范、快捷键、无障碍与 i18n | FE-1、FE-2、QA |
| [03-architecture.md](03-architecture.md) | 分层与依赖规则、线程模型、单向数据流、各模块设计、状态管理、扩展机制、错误处理、性能预算、目录结构 | 全员 |
| [04-contracts.md](04-contracts.md) | 统一接口契约 v1.0-rc：线程规则、不变式、事件、错误码、Mock 与一致性测试、版本冻结 | 全员（必读） |
| [05-delivery-plan.md](05-delivery-plan.md) | 角色、Worktree 布局、文件归属、CR 流程、门禁与合并、Phase 0–4 任务卡、阶段门、最终验收清单、各 Agent 任务包 | 用户（分配任务）、全员 |
| `../spikes/00-probe/` | 可行性探针（独立模块，`go test -v ./...` 复现 P1–P14） | 证据 |
| `../spikes/01-contract-rc/` | 契约 Go 代码 RC（独立模块，仅依赖标准库，`go vet`/`go build` 通过） | INT 落地用 |

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
| D9 | 分层 contract → core → adapters → views → app，导入规则由 `tests/arch` 强制 | 支撑多 Agent 并行、防契约漂移 | 设计决策 |
| D10 | Contract-First：先冻结 `contract-v1.0` + Mock + 一致性测试，再并行开发；变更只经 CR | 并行开发的前提 | 设计决策 |

## 4. 已验证 vs 待验证

**已验证 [实测]**（证据在 `spikes/00-probe`、`spikes/01-contract-rc`，2026-10-31 在本机重新运行通过）
- Go 1.27.3 工具链与 `mygo` 0.7.3 CLI 可用；Native UI 工程在 Windows 构建成功，可交叉编译到 macOS/Linux/Windows arm64；最小 release 体积 9.9 MB。
- 自定义 `TextInputClient` 能接收键入与 IME 组字/提交（Tester 模拟）。
- 混合字号/粗细/CJK/等宽背景/内联对象的排版、光标、命中测试。
- 解析器字节范围可推导语法标记；CRLF 原生支持；表格/数学块/任务项/脚注正确；增量解析与全量一致（890 KB、200 次编辑）。
- 已量化的限制：排版约 230 µs/段；增量解析单次约 6 ms/MB；Front Matter 与扩展语法不识别；转义/实体合并进 Text 节点；`editor.Buffer` 会把混合换行归一化。**每条都有对应方案**（见 01 §4）。
- 契约 RC 共 13 个 Go 文件，可编译，`go vet` 清洁，只依赖标准库。

**待验证 [待验证]**（Phase 0 Spike 负责，未通过前不得视为可行）
- S1：自研引擎的显隐编辑、跨块光标/选区、1 万块虚拟滚动、击键 p95 < 8 ms、**Windows 微软拼音真机**候选框位置。
- S2：离屏 WebView 不挂载时 `Eval` 是否稳定；MathJax SVG 被 `ui.ParseSVG` 正确解析的比例；渲染延迟与内存。
- S3：前缀不变式在语料上 100% 成立；BlockID 在随机编辑下保持稳定；5 MB 解析曲线。
- S4：玻璃工具栏/Vibrancy 侧栏/Shader 在 GPU 路径 60 fps；CPU 路径的降级方案。
- **macOS 与 Linux 上的一切运行时行为**（IME、字体、Vibrancy、菜单、文件关联、签名）：本机只有 Windows，只能交叉编译，不能运行。这些项必须在 CI 或真机上验证，验证前一律标记为"未验证"。

## 5. 已确认的决策（2026-10-31，用户确认）

| # | 问题 | 决定 |
|---|---|---|
| Q1 | 平台优先级 | **Windows 先行**；macOS/Linux 构建与无窗口测试在 CI 上跑，Phase 4 前在真机人工验收 |
| Q2 | 数学/Mermaid/PDF 依赖系统 WebView | **接受**（WebView2 / WKWebView / WebKitGTK，懒启动，不可用时降级为源码） |
| Q3 | 并行 Agent 数量 | **6 个**：INT、BE-1、BE-2、FE-1、FE-2、QA |
| Q4 | Go 模块路径与应用标识 | 模块 `aurink`；identifier `app.aurink.editor` |
| Q5 | `git init` 与 worktree | **已执行 `git init -b main`**（由 CEO 按用户要求完成）；worktree 由 INT 在 INT-0.1 完成后按 05 §2 创建 |
| Q6 | GitHub 与 Actions | **已创建私有仓库 `oysterhyd/aurink` 并启用基线 CI**（三平台矩阵，见 05 §5.1）；INT-0.1 将其切换为调用 `scripts/gate.sh` |

## 6. 下一步（用户操作）

1. 把 `05-delivery-plan.md §9` 中对应的任务包分配给各 Agent，先启动 Phase 0：**INT-0.1 → INT-0.2**，同时启动 **S1–S4** 与 **QA-0.2**。
2. 各 Spike 完成并提交 `REPORT.md` 后，通知 CEO 进行 **M0 阶段门评审**（Go/No-Go + 契约冻结 `contract-v1.0`）。

CEO 不会自行创建或调度 Agent，也不会自行编写产品代码或执行开发测试；每个阶段门和最终验收都基于实际运行证据给出 PASS / FAIL。
