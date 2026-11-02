# M1 基线对照

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
