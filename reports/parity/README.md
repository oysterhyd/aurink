# 基线对照（M1、M2）

截图由 `go run ./tools/windowcheck`（真窗口，Windows 11，本机一种硬件）生成，放在 `screens/`。`DesignSystem` 用 `-design` 单独生成。对照方式是负责人目视比较截图和 `docs/design/baseline/`，没有做像素比对。结论是负责人的观察，观感最终由用户在真机确认（06 §8）。

`screens/` 里界面文字是英文（本机系统语言不是中文）。Phase 2 批次 6 补了中文界面：`go run ./tools/windowcheck -locale zh-Hans -out reports/parity/screens/zh-Hans`（`-locale` 用 `MYGO_LOCALE`，只对这一次运行生效），截图在 `screens/zh-Hans/`。负责人看过 Welcome、Main、Notify：界面文字都已翻译，没有截断或溢出（标题栏“打开文件”、模式“实时”、状态胶囊“已保存 251 字”、代码块“复制”、两条通知）。一处留给用户真机判断：中文斜体（正文“离开之后”、引用块）是常规字体倾斜，没有换楷体之类的字体。这是在中文系统下以外的设置切换，系统字体回退在中文 Windows 上可能不同 [待验证]。

| 基线 | 结果 | 记录 |
|---|---|---|
| Main | 一致，有 3 处小差异 | [Main.md](Main.md) |
| Dark | 一致，差异同 Main | [Dark.md](Dark.md) |
| Source | 一致 | [Source.md](Source.md) |
| LowPower | M1 已有界面一致 | [LowPower.md](LowPower.md) |
| Notify | 一致，没有进度通知 | [Notify.md](Notify.md) |
| Welcome | 一致，没有极光 | [Welcome.md](Welcome.md) |
| DesignSystem | 令牌取值一致，缺开关控件 | [DesignSystem.md](DesignSystem.md) |
| Palette | 一致，命令用短名，有 4 处差异 | [Palette.md](Palette.md) |
| Find | 一致，引用样式按 Typora | [Find.md](Find.md) |
| Conflict | 一致，没有 P2 的对比按钮 | [Conflict.md](Conflict.md) |

## M2（批次 13）

本机显示缩放是 150%（144 DPI），`screens/` 和 `screens/zh-Hans/` 的截图都是 150% 下的真窗口，所以图片是 1898×1188，基线是 1280×800。负责人看过 150% 下的 Main、Files、Find、Conflict 和中文的 Files、Find、Conflict：没有裁切、错位或模糊。视图菜单展开的样子没有截图（§8 风险），留给用户真机看一次。100% 和其他缩放没有在本批截图，留给用户真机判断。

`Files`、`Outline`、`Search`、`Table`、`Link`、`SourceFind` 是 M2 新界面的截图，没有对应的基线，只检查有没有明显错误。用户 2026-11-02 要求引用对齐 Typora 后，所有截图里的引用都和基线不同，这是有意的。
