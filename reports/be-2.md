# BE-2 · 平台服务

## Merge request: S2

- 分支：`be/services`；已 `git fetch origin`，并 rebase 到 main **6a60ce0**。实现与证据提交：**9459819**（`S2: prototype offline WebView rendering and record No-Go evidence`）。
- 变更摘要：[源码] 新增独立模块 `spikes/s2-render/`，锁定 MyGo v0.7.3；打包 MathJax 3.2.2 / Apache-2.0、Mermaid 10.9.3 / MIT 及许可证；LoadHTML + 有界 Page.Eval、ParseSVG、Native 直接绘制、canvas PNG、私有内存缓存与缺失后端降级。完整数据、50 例 SVG、10 张对照图、Native 截图、六种 Mermaid PNG 在 `spikes/s2-render/evidence/`；结论见 [REPORT.md](../spikes/s2-render/REPORT.md)。
- Go/No-Go：[实测] **No-Go**。解析 50/50，视觉完整 47/50（94%），F48–F50 中文 text 被忽略；冷启动 813.601 ms；未缓存公式 p95 48.157 ms、最大 161.204 ms，F08/F41 超过 50 ms；缓存批量均摊 p95 0.000170 ms；不挂视图的连续 Eval 1000/1000 正确；WebView2 进程树工作集 361–565 MiB。此合并请求提交的是可复现的 No-Go 调研结果，不代表 M0 通过。
- 新增测试：[实测] 1000 次重复请求仅一次桥调用、JSON 参数安全编码、取消优先于缓存、桥失败/空 SVG/坏 SVG/TeX 错误/残留 text 不入缓存、50 例 SVG 解析、数组竖线样式、Native 公式墨迹、源码降级与虚拟时钟空闲帧检查。真实原生测量还检查异常、Promise reject、永不 resolve 超时以及超时后仍响应；没有用墙钟等待/耗时阈值做单元测试断言。
- 关键测试输出（完整见 `evidence/tests.log`）：

  ```text
  --- PASS: TestCacheAvoidsBridgeAndCanceledRequests
  --- PASS: TestUnavailableAndFailuresDoNotPoisonCache
  --- PASS: TestMeasuredCorpusParseAndNativeDrawing
  --- PASS: TestFallbackStaysNativeAndIdle
  PASS
  ok aurink.local/spikes/s2-render 0.222s
  ```

- 门禁：[实测] rebase 后执行 `scripts/gate.ps1`，根模块及 00-probe/01-contract-rc/s1-liveedit/s2-render 的格式、三目标 vet/带 mygo_noinspector 的 vet/build、mygo vet、CGO_ENABLED=0 测试通过。关键输出（完整见 `evidence/gate.log`）：

  ```text
  ok aurink.local/spikes/s1-liveedit/liveedit 14.744s
  ok aurink.local/spikes/s2-render 0.219s
  SKIPPED: tools/i18nmerge does not exist yet
  Gate passed
  ```

- INT 接线：本卡独立 Spike，**无需注入产品 app**。后续渲染服务由 INT 提供主窗口、首页面 ThrottlingOff、Dispatcher 与降级通知生命周期；不要把本原型直接接入产品。[待验证] Windows 实际卸载 WebView2/杀 renderer 的恢复、内存回收平台期、macOS/Linux 真机、数学 PNG 的基线/Scale/延迟仍未验证。
- 相关 CR：**[CR-003-math-raster-fallback.md](../docs/cr/CR-003-math-raster-fallback.md)**，允许不支持 SVG 内容的数学公式使用 PNG，兼容性与消费者语义待 CEO 批复。main 已有 CR-001/CR-002，故本提案采用 CR-003。**有 CR 需要 CEO 评审**。未修改契约或 `spikes/01-contract-rc`，批准前仅使用 Spike 内私有类型。
- 范围：只新增 S2、自有报告和获准的 CR 路径；未开始 BE-2.1，等待 M0。
