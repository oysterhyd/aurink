# Motion

对照 02 §4.4 的令牌和 7 条运动规则（Phase 3 批次 8）。证据是 QA 测试（`tests/qa`，Tester，CPU 路径）和代码位置。Tester 只能看出"还有没有下一帧"和元素位置，看不出曲线手感和帧率。手感、GPU 路径下的极光和辉光，由用户在真机确认 [待验证]。

## 令牌

`internal/views/motion/motion.go` 定义了 `Quick`、`Snappy`、`Smooth`、`Gentle`、`Stagger` 和 `PressScale = 0.965`，取值与 §4.4 的表一致。`Stagger` 已定义，但还没有地方用它（见下方"未做"）。

## 规则

| # | 规则 | 结果 | 证据 |
|---|---|---|---|
| 1 | 一源一终 | 做到 | 命令面板从 omnibox 长出：`TestPressAndPaletteUnlessMotionIsReduced`。斜杠菜单从光标处长出：`TestSlash*`。灯箱从图片飞入并飞回：`TestLightboxFlight`。格式条在选区上方出现：`TestFormatBar*`。 |
| 2 | 一个选中药丸 | 做到 | 标签条、文件树（`filePill`）、大纲（`outlinePill`）、设置分类、分段控件、命令面板、斜杠菜单，每处都只有一个 Match 药丸在滑动，没有逐行底色。标签和大纲由 `TestPillsSlideUnlessMotionIsReduced` 验证；其他位置只看了代码。 |
| 3 | 文字不弹 | 部分 | 标记出现或消失时宽度立即变化，没有宽度动画。但标记目前是直接出现，没有按规则用 `Quick` 淡入（见"未做"）。 |
| 4 | 空闲 0 帧 | 做到 | 场景 "an idle window asks for no frames"，以及 "idle with a folder watched, each panel and the find bar open"。极光 12 s 后停止：`TestAurora`、`TestAuroraPhase`。主题溶解结束后不再请求帧：场景 "the theme toggle persists and settles"。 |
| 5 | 减少动态效果 | 做到 | `motion.Still` 去掉 Match 飞行、位移和缩放，只保留颜色和淡入：`TestMotionStill`。药丸改为直接跳到位：`TestPillsSlideUnlessMotionIsReduced`。按压没有回弹，命令面板没有飞行：`TestPressAndPaletteUnlessMotionIsReduced`。骨架不扫光：`TestLoadingStillWithMotionReduced`。斜杠菜单：`TestSlashReducedMotion`。这些对应用设置和系统设置都生效。 |
| 6 | 可打断 | 由框架保证 | 用的都是 MyGo 的 Spring（`ElementTransition`、`AnimateTo`），新目标会接着当前速度走。没有单独为此写测试。 |
| 7 | 微交互 | 做到 | 悬停 `t.Hover` 着色，见 `kit.go`。按压缩到 0.965 后 `Snappy` 回弹：`TestPressAndPaletteUnlessMotionIsReduced`。光标滑动和辉光：`TestSmoothCaret*`。 |

## 批次 8 新增

- 滚动边缘：文档滚动后，标题栏下出现 28 px 渐变；低功耗下是 1 px 发丝线。见 `TestScrollEdge`。
- 极光：欢迎页和空的实时文档上显示（铺满下方 55%），12 s 后停止请求帧。低功耗或没有 GPU 时用静态渐变。见 `TestAurora`。
- 文档骨架：切换文档超过 100 ms 才显示，在 100 ms 内完成的不显示。见 `TestLoadingSkeleton`、`TestLoadingQuickShowsNothing`。

## 未做

- 标记用 `Quick` 淡入淡出（规则 3）。目前是直接显示，宽度变化正确。
- `Stagger` 逐项进入。图表渲染结果是一整张图，没法逐项进入（§5.9）。列表和面板也还没用上。

> 模式切换时内容交叉淡化（§5.10）、主题从点击点径向溶解等，在各自批次里验证，不在本表。
