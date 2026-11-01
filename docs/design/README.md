# UI 设计基线：Ink & Aurora

这里是 Aurink 的界面验收基线。产品必须在 Native UI 上达到这里的效果，规格文字见 [02-product-ux.md](../02-product-ux.md)。

| 目录 | 内容 |
|---|---|
| `prototype/` | 可运行的 HTML 原型（`Main.dc.html` 含全部界面，`support.js` 是原型运行时） |
| `baseline/` | 18 张基线截图，1280×800（`DesignSystem`、`Motion` 为 1960×1400） |

`docs/design/**` 归 CEO 所有，其他 Agent 只读。

## 运行原型

```bash
python -m http.server 8932 --directory docs/design/prototype
```

然后打开 `http://127.0.0.1:8932/Main.dc.html`。字体从 Google Fonts 加载，离线时会回退到系统字体，此时不能用来比对排版。

## 如何使用原型

- 视觉取值以 02 §4 为准；02 没有写到的细节，到 `Main.dc.html` 里查。
- 原型是浏览器模拟，玻璃、Spring 和极光都不是 MyGo 实现，只规定**看起来和动起来的目标**。实现方式见 02 §6。
- 截图里的浏览器滚动条是预览环境造成的，产品使用 MyGo 的覆盖式滚动条。
- 这些是原型专属内容，不实现：命令面板里的「原型演示」分组、示例文档内容、`Tweaks`。

## 基线对照表

| 基线 | 状态 | 负责 | 任务卡 | 阶段门 |
|---|---|---|---|---|
| `DesignSystem.png` | 令牌、字体、材质、控件 | FE-2 | FE-2.1 | M1 |
| `Motion.png` | 运动语言、时间线 | FE-2 | FE-2.1、FE-2.6 | M3 |
| `Main.png` | 布局、实时编辑、块样式 | FE-2 / FE-1 | FE-2.2、FE-1.2 | M1 |
| `Dark.png` | Midnight Ink 全局 | FE-2 / FE-1 | FE-2.1、FE-1.2 | M1 |
| `Source.png` | 源码模式 | FE-1 | FE-1.3 | M1 |
| `LowPower.png` | CPU / 减少透明度路径 | FE-2 | FE-2.1 | M1 |
| `Palette.png` | 命令面板 | FE-2 | FE-2.4 | M2 |
| `Find.png` | 查找替换、导轨命中 | FE-2 / FE-1 | FE-2.5、FE-1.4 | M2 |
| `Conflict.png` | 冲突横幅与胶囊状态 | FE-2 | FE-2.5 | M2 |
| `Notify.png` | 通知栈 | FE-2 | FE-2.2（进度 FE-2.7） | M1 / M3 |
| `Welcome.png` | 欢迎页（M1 静态，M3 加极光） | FE-2 | FE-2.2、FE-2.6 | M1 / M3 |
| `Reading.png` | 阅读模式 | FE-1 / FE-2 | FE-1.7、FE-2.6 | M3 |
| `Focus.png` | 专注模式 | FE-1 / FE-2 | FE-1.7、FE-2.6 | M3 |
| `Slash.png` | 斜杠菜单 | FE-1 | FE-1.7 | M3 |
| `Format.png` | 选区格式条 | FE-1 | FE-1.7 | M3 |
| `Lightbox.png` | 图片灯箱 | FE-1 | FE-1.7 | M3 |
| `Settings.png` | 设置 | FE-2 | FE-2.7 | M3 |
| `Loading.png` | 文档骨架屏 | FE-2 | FE-2.6 | M3 |

## 验收方法

任务卡完成时，负责人按同一场景（1280×800、相同示例文档）截取 Native 截图，放到 `reports/parity/<基线名>.png`，并在合并请求中写明：

1. 令牌、版式、状态逐项对照的结果，以及每一处差异。
2. 动效逐条对照 02 §4.4 的运动规则，无法用截图证明的项目附录屏或 Tester 断言。
3. 低功耗路径和减少动态效果下的表现。

允许的差异只有字体渲染、抗锯齿，以及 S4 ADR 记录的材质差异。其他差异提 CR，由 CEO 裁决。像素差异不作为门禁，最终判断由 CEO 和用户在真机上作出。
