# CR-005 · 数学 SVG 不支持内容的 PNG 回退

（原编号 CR-003，与另一份 CR-003 重复，2026-11-01 改号。）

状态：**待 CEO 评审，尚未批准**。提出者：BE-2；任务卡：S2；日期：2026-11-01。

## 动机

[实测] S2 固定 50 例语料中，MathJax 3.2.2 `fontCache: 'none'` 的 SVG 均可被 MyGo v0.7.3 `ui.ParseSVG` 解析，但 F48–F50 的中文使用 `<text>`，Native 绘制时丢失。完整显示比例为 **47/50 = 94%**，未达到 95% 的 Go 标准。证据见 `spikes/s2-render/REPORT.md`、`evidence/comparison-10.png`、`evidence/svg/F48.svg`。

[源码] `internal/contract/render.go` 的现有注释将 SVG 限为“数学：只含 path”，将 PNG 限为“图表”。MathJax 也会输出受 MyGo 支持的 `<g>`、`<rect>`、`<line>`；只检查 ParseSVG 返回值无法判定内容完整。希望明确数学结果允许在 SVG 子集不适用时选择 PNG，避免契约消费者默认所有数学都必有 SVG。

## 建议 Go diff

只扩展结果语义，不增加字段或更改签名；由 INT 在批准后落地，并同步契约文档。

```diff
 type RenderResult struct {
-    SVG    []byte // 数学：只含 path 的 SVG
-    PNG    []byte // 图表：PNG，像素 = DIP × Scale
+    // SVG 和 PNG 成功时恰有一个非空。SVG 必须完整落在 Native
+    // 支持的静态子集内（path/基本形状/分组等），不得残留 text。
+    SVG    []byte
+    // PNG 用于图表，以及 SVG 子集不能完整表示的数学公式。
+    // 像素 = DIP × Scale；数学的 Ascent 仍须有效。
+    PNG    []byte
     Width  float32
     Height float32
     Ascent float32
     Scale  float32
 }
```

## 影响和兼容性

- 兼容级别：次版本的行为扩展，二进制字段布局不变；已有消费者需要检查 PNG。请 CEO 确认该兼容级别。
- BE-2：串行渲染后检查 SVG 未支持节点和 MathJax `merror`；含 `<text>` 的有效公式改走 canvas PNG；解析错误返回 CodeRender，不缓存错误。缓存键包含输出格式/Scale；测量正确的行内 Ascent。
- FE-1：公式组件同时接受 SVG/PNG；PNG 按 Scale 显示；均不可用时保留可编辑源码，不能默默显示残缺结果。
- INT：宿主仍是不挂视图的 WebView；首个页面创建时启用 ThrottlingOff；保持懒启动和销毁生命周期。无新增依赖。
- QA：mock Renderer 提供数学 PNG 场景；contracttest 增加 SVG/PNG 恰一非空、有效尺寸/Scale、CJK 不丢字、失败不进缓存的断言；三平台真机复核。
- CEO：如采纳，更新 01 §4.4/§6、03 §5.5、04 的相应语义及 ADR；原 S2 的纯 SVG Go 标准仍为 No-Go，不能把本提案当成已获豁免。

## 批准前的处理

[实测] S2 仅使用自己独立模块的私有 `mathResult`，记录 `<text>` 数量并拒绝将残缺结果放入缓存。参考 PNG 用于视觉评估，不接入产品契约。未修改 `internal/contract`、`internal/views/editorapi` 或 `spikes/01-contract-rc`。

[待验证] PNG 数学回退的最终端到端延迟、缩放清晰度、行内基线与三平台表现仍需在批准后的对应任务验证。本提案不授权开始 BE-2.1 或 BE-2.9。
