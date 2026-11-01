# 保真语料

文件必须按字节保持不变（`.gitattributes` 对 `testdata` 关闭了换行转换），`tests/corpus` 按本表校验。

- `spec/`：MyGo 解析器的 CommonMark、GFM 规范示例与样例（`ui/markdown/parser/testdata`）。
- `edge/`：手写的边界样本：CJK、CRLF、CR、BOM、混合换行、无尾换行、Front Matter、Tab、嵌套容器。
- `real/`：60 份真实文档，来自 MyGo v0.7.3 的 `docs/`（MIT，见 `LICENSE-mygo`）。
- 1 MB、5 MB 大文档由 `corpus.Generate` 按种子生成，不入库。

| 文件 | 字节 | SHA-256 前缀 |
|---|---|---|
| `spec/commonmark.json` | 140487 | `d431b29d97b6f73e` |
| `spec/extras.md` | 739 | `9f3c166db61364fb` |
| `spec/gfm.json` | 163447 | `89cfcb21173de246` |
| `spec/llm.md` | 49735 | `3c31b23eb0b6958d` |
| `edge/bom-utf8.md` | 196 | `9edf3b8591ba92ad` |
| `edge/cjk.md` | 193 | `838a23cd941a8d06` |
| `edge/cr.md` | 7 | `853d54fe848f1d21` |
| `edge/crlf.md` | 211 | `75300c6a51b0ef87` |
| `edge/empty-containers.md` | 14 | `d61db300ff594a95` |
| `edge/empty.md` | 0 | `e3b0c44298fc1c14` |
| `edge/footnotes.md` | 40 | `45c11619e8ec93b8` |
| `edge/front-matter-crlf.md` | 28 | `19d1021c79a750b7` |
| `edge/front-matter-unclosed.md` | 19 | `8e96b1a8830d2c17` |
| `edge/front-matter.md` | 45 | `3d7928ecedbbc820` |
| `edge/link-defs.md` | 31 | `99411b1baabce787` |
| `edge/math.md` | 25 | `c0e2d389188d5f9d` |
| `edge/mixed-eol.md` | 20 | `8a1bf9f8a2214061` |
| `edge/nested.md` | 36 | `a7af324386cc782e` |
| `edge/no-final-newline.md` | 34 | `7970aa4ee8dbdbab` |
| `edge/only-newlines.md` | 3 | `6a3cf5192354f716` |
| `edge/open-fence.md` | 15 | `8cbba501c3bb616d` |
| `edge/tabs.md` | 22 | `2b87823459ea97a5` |
| `edge/trailing-space.md` | 16 | `2fc9990966c42458` |
| `real/README.md` | 8015 | `1263d2203cb0ab20` |
| `real/app.md` | 19287 | `9de5d8a401f57410` |
| `real/architecture.md` | 339323 | `b3275a2d61c5ab35` |
| `real/audio.md` | 6550 | `a1fbdfc9f070156d` |
| `real/bindings.md` | 12304 | `a20c30f1f201f67e` |
| `real/cli.md` | 10538 | `5b7744ca55e4c273` |
| `real/configuration.md` | 12817 | `07c6f16b2929582e` |
| `real/data-transfer.md` | 8281 | `efeb7555a0d406ab` |
| `real/distribution.md` | 31352 | `934e24f9d4ee7c7a` |
| `real/documents.md` | 15160 | `a68876d6a5a0aeaf` |
| `real/frontend.md` | 22715 | `8497f644aa4874d3` |
| `real/getting-started.md` | 8784 | `f712e089707eb4b1` |
| `real/github-actions.md` | 13408 | `a543fae807b22f64` |
| `real/i18n.md` | 23711 | `3c76cc58372c88ed` |
| `real/launchers.md` | 5464 | `70c581d723c73a7d` |
| `real/menus.md` | 19973 | `76eed1efbf997391` |
| `real/native.md` | 28036 | `2d4f74630dc5faa1` |
| `real/plugins-automation.md` | 12865 | `59b632d6abce5b17` |
| `real/plugins-fetch.md` | 5807 | `985534dd6fa11926` |
| `real/plugins-game.md` | 8307 | `7483d7bef0529c43` |
| `real/plugins-glass.md` | 9763 | `5b86e8774bc7cf3c` |
| `real/plugins-loading.md` | 5318 | `e9cd96ed99fab94b` |
| `real/plugins-sqlite.md` | 7057 | `35c0d74eb096b39f` |
| `real/plugins-terminal.md` | 33961 | `43e249353f559adb` |
| `real/plugins-updater.md` | 7042 | `91b96ab2f924bf06` |
| `real/plugins-watch.md` | 6749 | `c2720fa380f5b73f` |
| `real/plugins-websocket.md` | 5330 | `ee24905c978b8dbe` |
| `real/plugins.md` | 4120 | `ed4a5267e664f418` |
| `real/ui-README.md` | 10480 | `9be2c1e682de7577` |
| `real/ui-accessibility.md` | 4802 | `c1e28d8df7fa5197` |
| `real/ui-accordion.md` | 1181 | `ab016bd161f216e8` |
| `real/ui-alert-dialog.md` | 1336 | `c155d68e98dc386b` |
| `real/ui-autocomplete.md` | 701 | `df224258535d7ef0` |
| `real/ui-avatar.md` | 505 | `294c5ec72811cb2a` |
| `real/ui-badge.md` | 540 | `cdbdd4879186186b` |
| `real/ui-breadcrumbs.md` | 658 | `746d8c4c1f945df3` |
| `real/ui-browsers.md` | 5747 | `805d92775e788227` |
| `real/ui-button.md` | 2361 | `cbab33775960cf74` |
| `real/ui-calendar.md` | 1241 | `45763c0c40725b24` |
| `real/ui-chart.md` | 10886 | `56ec9ef291c21a53` |
| `real/ui-checkbox.md` | 1427 | `e8978114755fc5f6` |
| `real/ui-code-editor.md` | 19975 | `11b81f86ee236e04` |
| `real/ui-collapsible.md` | 1296 | `e26d32a04323ba82` |
| `real/ui-color-picker.md` | 1194 | `7b3a913f2cffff8e` |
| `real/ui-combobox.md` | 1296 | `2c617fd4accc88d4` |
| `real/ui-context-menu.md` | 4911 | `0f67776fbb7b6553` |
| `real/ui-custom-widgets.md` | 5067 | `e9cc43199f3b3967` |
| `real/ui-date-input.md` | 1025 | `18b3f2cc0cc6306f` |
| `real/ui-dialog.md` | 1990 | `f92ef09bde8191c8` |
| `real/ui-diff-viewer.md` | 8823 | `9b79a0d2d825632b` |
| `real/ui-drag-and-drop.md` | 9101 | `ca37cba82e352a69` |
| `real/ui-drawing.md` | 10673 | `a8f5e7226a6daf08` |
| `real/ui-editable-text.md` | 1087 | `e163201a68218da4` |
| `real/ui-find-bar.md` | 1218 | `c31ed4856a8cf783` |
| `real/ui-form.md` | 2206 | `49f29154318c5011` |
| `real/ui-grid-view.md` | 1387 | `0a13ab7201647eb2` |
| `real/ui-grid.md` | 1683 | `3e449a1f227a2ca4` |
| `real/ui-history-buttons.md` | 918 | `383c9178f9428e08` |
| `real/ui-icon.md` | 1373 | `1d5de8fc0d522dde` |
| `real/ui-image.md` | 5497 | `23ade228a98f9237` |
