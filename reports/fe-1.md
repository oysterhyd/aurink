# FE-1 reports

## Merge request: S1 · 编辑引擎原型

- Branch: fe/editor, rebased on main at 042b9e1
- Changes:
  - `spikes/s1-liveedit/**`：独立模块 `aurink.local/spikes/s1-liveedit`，含自定义元素与 TextInputClient、最小投影、按块排版缓存、Fenwick 虚拟滚动、滚动锚定、虚拟预编辑，`go run .` 演示窗口，`REPORT.md`。
  - `docs/cr/CR-001-blank-lines.md`、`docs/cr/CR-002-projection-offsets.md`。
  - `reports/fe-1.md`。
  - 未修改 `internal/contract`、`internal/views/editorapi`、`spikes/01-contract-rc`。
- 结论：建议 Go（详见 `spikes/s1-liveedit/REPORT.md` §1）。
- New tests and results: `go test ./...` 50 PASS、0 FAIL、1 SKIP（`TestSnapshot`，仅在设置 `S1_SNAPSHOT` 时运行）[实测]。断言按计数，不依赖墙钟。
  - 击键，真窗口，200 KB：引擎 p95 1.81 ms，加整帧 p95 2.3 ms，合计上界约 4.1 ms，低于 8 ms [实测]。
  - 滚动，真窗口，1 万块，120 DIP × 300：平均每次重排约 1.5 块，大多数帧 1–3 块，最多一帧 6 块；不含一次光标跳转（22 块）[实测]。
  - 微软拼音：用户真机检查，`REPORT.md` §4 共 10 项，答复「全部通过」（整体结论，无逐项记录）。
  - [待验证]：`RevealInline` 下的光标移动；macOS、Linux 及其他输入法的 IME；中端笔记本上的性能。
- Gate: GATE_RESULT
- INT wiring needed: none。S1 是独立模块，不接入 `internal/app`。
- Related CR: `docs/cr/CR-001-blank-lines.md`、`docs/cr/CR-002-projection-offsets.md`，均待 CEO 批复。S1 不依赖批复，已用私有实现。
