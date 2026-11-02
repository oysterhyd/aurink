# DesignSystem

截图：`screens/DesignSystem.png`（`go run ./tools/windowcheck -design`），基线：`docs/design/baseline/DesignSystem.png`。

这张图是工具画的样张，不是产品界面，排布不必和基线相同，只比较内容。

| 项 | 结果 |
|---|---|
| Paper、Midnight Ink 的 Window、Sheet、Ink、Muted、Faint、Mark | 和基线标注的色值相同 |
| Accent | 目视一致 |
| 字体层级 H1、H2、H3、正文、等宽 | 一致 |
| 玻璃等级 1–3 的模糊值 16、24、36 | 和基线相同 |
| 主次按钮、KeyHint、分段控件、胶囊 | 一致 |

## 差异

| 差异 | 处理 |
|---|---|
| 没有开关控件 | M1 没有用到；设置界面在 M3（FE 设置卡）补上 |
| 样张没有画 Aurora 1–3 色块、品牌横幅和圆角比例尺 | 只是样张没画，品牌图形和极光属于 M3 |
