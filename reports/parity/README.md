# M1 基线对照

截图由 `go run ./tools/windowcheck`（真窗口，Windows 11，本机一种硬件）生成，放在 `screens/`。`DesignSystem` 用 `-design` 单独生成。对照方式是负责人目视比较截图和 `docs/design/baseline/`，没有做像素比对。结论是负责人的观察，观感最终由用户在真机确认（06 §8）。

所有截图里界面文字是英文，因为本机系统语言不是中文，`zh-Hans` 目录已生成，但本机没有在中文系统下实际看过。

| 基线 | 结果 | 记录 |
|---|---|---|
| Main | 一致，有 3 处小差异 | [Main.md](Main.md) |
| Dark | 一致，差异同 Main | [Dark.md](Dark.md) |
| Source | 一致 | [Source.md](Source.md) |
| LowPower | M1 已有界面一致 | [LowPower.md](LowPower.md) |
| Notify | 一致，没有进度通知 | [Notify.md](Notify.md) |
| Welcome | 一致，没有极光 | [Welcome.md](Welcome.md) |
| DesignSystem | 令牌取值一致，缺开关控件 | [DesignSystem.md](DesignSystem.md) |
