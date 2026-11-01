# BE-1 · 文档核心交付记录

## 2026-11-01 · S3 合并请求

**S3 可以合并（原型、证据与评审提案）；严格全部叶子前缀标准为 No-Go，M0 尚待 CEO 裁定。** 分支 `be/core`，已 fetch 并 rebase 到本地 main `6a60ce0`；代码提交 `13cae4a`，报告与证据随本条记录提交。

变更摘要：独立模块 `spikes/s3-ast`，锁定 MyGo v0.7.3 并 replace 引用 contract-rc；用公开 Line/LineCount 生成块范围/内容/标记/前缀，以私有行映射处理部分 Tab；实现原型 BlockID 匹配、Front Matter 预处理、Escape/Entity 扫描、增量防护与分片依赖实验。完整结论见 [S3 REPORT](../spikes/s3-ast/REPORT.md)。

新增测试与结果 [实测]：

- 652 个 CommonMark + **全部** 670 个 GFM + 17 个自造样例：1,934 叶子块、2,383 内容行。严格删除规则 1,925/1,934；段落/标题 1,531/1,531；私有补空格行映射 1,934/1,934。9 个失败块不计入严格通过。
- 固定三种子各 10,000 步，带保守回退后每步的块结构、范围、内联、HTML 与全量一致；1,085,936 次存活未触及块 ID 检查全部保持。实际调用 parser.Edit 12,414 次；引用保护全量回退 17,471 次、Front Matter 重解析 115 次，分别记账。
- 关闭两项增量保护的额外两个种子各 10,000 步通过，但保留两个原始解析器失败反例（缩进代码块分裂、引用/脚注内联缓存失效）。反例测试断言缺陷确实重现且防护后等价，不能据这些测试 PASS 宣称原始 parser.Edit 总是正确。
- Front Matter 12 个识别场景与 5 次显式转换；Escape/Entity 范围与文本黄金；Setext/闭合标记/容器前缀；闭合分片共享引用、重复定义、多行标签/标题、开放围栏等测试通过。全语料补出 135 个 Escape 和 55 个 Entity。
- `CGO_ENABLED=0 go test -v -count=1 -cover ./...`：`PASS`，`coverage: 96.6% of statements`。关键输出及所有种子计数见 [tests.txt](../spikes/s3-ast/evidence/tests.txt)。

```text
samples=1339 leaf_blocks=1934 content_lines=2383
strict_pass=1925/1934 bridge_pass=1934/1934 padded_lines=10
paragraph_heading_strict_pass=1531/1531
raw parser.Edit blocks=3 full=2 (root included); guarded adapter fallback=1 equivalent=true
raw cached reference becomes stale; guarded adapter fallback=1 equivalent=true
PASS
coverage: 96.6% of statements
```

性能 [实测]：Windows/amd64，Go 1.27.3，Ryzen 7 8845H，CGO=0/GOMAXPROCS=1；每模式 10 操作×3 轮，取三轮均值中位数。5 MiB（271,847 块）全量 parser 137.42 ms、中部 Edit 54.28 ms、原型 eager 适配 Edit 454.33 ms；约 16 KiB 闭合分片局部 Edit 中位数在各组为 0.118–0.271 ms，5 MiB 引用定义全片广播 99.28 ms。此分片数字不包括全文字符串和统一 AST 的维护成本；不作为产品击键 p95。详见 [benchmark.txt](../spikes/s3-ast/evidence/benchmark.txt)。

门禁 [实测]：最终 `pwsh -NoProfile -File scripts/gate.ps1` 返回 0，输出 `Gate passed`。包含所有模块 gofmt、三目标 vet/build、已有 UI 模块 mygo vet、契约标准库依赖与本机测试；跨平台只验证编译。i18nmerge 尚未存在，脚本显式跳过，此项未验证。关键输出见 [gate.txt](../spikes/s3-ast/evidence/gate.txt)：

```text
spikes/s3-ast windows/amd64
spikes/s3-ast linux/amd64
spikes/s3-ast darwin/arm64
ok  aurink.local/spikes/s3-ast  13.957s
SKIPPED: tools/i18nmerge does not exist yet
Gate passed
```

需要 INT 接线/处理：本 Spike 不注入 app。请合入 S3 研究模块、报告和证据，提交 M0；登记报告中的两个 MyGo v0.7.3 最小反例，协调上游修复或保守回退策略。契约批复后由 INT 联同 QA 一次性更新契约、mock、contracttest 和 04 文档；与已有 CR-001/002 协调内容覆盖与缓存失效。正式文档核心没有在本任务中落地。

**有 CR 需要 CEO 评审**：[CR-003](../docs/cr/CR-003-content-line-mapping.md)（内容行/剩余 Tab 空格）及 [CR-004](../docs/cr/CR-004-surviving-block-identity.md)（存活身份和语法依赖边界）。[待验证] 批复后的生产 AST、Dirty 传播、投影/光标映射和通用分片器。任务列表停在 S3，等待 M0；不开始 BE-1.1。
