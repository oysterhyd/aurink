# INT 报告

## INT-0.1 仓库脚手架

变更摘要
- 根模块 `aurink`（go 1.27.3，MyGo v0.7.3）、`mygo.json`、`main.go` → `internal/app.Run`（空窗口）。
- `docs/03` §11 目录骨架：31 个 `doc.go` 加 `internal/app`。`internal/contract`、`internal/views/editorapi` 由 INT-0.2 落地，不在此处占位。
- `scripts/gate.ps1`、`scripts/gate.sh`：05 §5.1 第 1–6 项，覆盖根模块和 `spikes/*`。
- `ci.yml` 调用 `scripts/gate.sh`：Linux 跑 `--all-targets`（5 个目标），Windows、macOS 跑 `--test-only`（与基线 CI 的分工相同）。
- `reports/README.md`：合并请求格式。

偏离与决定
- `locales/en.json`、`zh-Hans.json`、`catalog_gen.go` 已提交：`mygo.json` 开启 i18n 后需要目录内有目录文件，`catalog_gen.go` 由 `mygo dev` 生成，`internal/app` 导入该包。目前只有一个键 `appName`，`i18nmerge` 出现后由它生成。
- `tools/` 只有 README，没有 Go 包：只有 `doc.go` 的 `package main` 无法构建。门禁以 `tools/i18nmerge/main.go` 是否存在决定跳过。
- 收紧了"contract 只依赖标准库"：原检查放行模块内所有包，`internal/contract` 导入 `aurink/internal/core` 也不会报错；现在只放行 `aurink/internal/contract` 自身。

需要 CEO 注意
- 05 §3.1 的文案键示例 `shell.tabs.close` 不符合 MyGo 规则 [源码 docs/i18n.md]：键只能是字母开头的字母、数字和 `_`。写 `tools/i18nmerge` 前需要定下规则，比如 `shell_tabs_close`。

门禁结果（Windows，本机）
- [实测] `scripts/gate.ps1`、`scripts/gate.sh` 都输出 `Gate passed`，退出码 0。第 6 项输出 `SKIPPED: tools/i18nmerge does not exist yet`。
- [实测] 反向检查：放入未格式化文件、放入失败的测试，两个脚本都返回退出码 1。
- [实测] `go tool mygo dev` 构建并启动 `Aurink Dev.exe`，出现标题为 `Aurink` 的顶层窗口；我查的是进程和窗口标题，没有截图看窗口内容。
- [待验证] macOS、Linux 上的有窗口运行。CI 只跑无窗口测试。
- [实测-CI] 三平台全绿：<https://github.com/oysterhyd/aurink/actions/runs/41301818863>（`5cf3305`）。Linux 跑全部检查和 5 个目标，Windows、macOS 跑测试。

## INT-0.2 契约落地

变更摘要
- `spikes/01-contract-rc/contract` → `internal/contract`（12 个文件），`editorapi` → `internal/views/editorapi`，只改 import 路径。RC 目录保留未动。

门禁结果
- [实测] `diff -r spikes/01-contract-rc/contract internal/contract`：无差异，退出码 0。
- [实测] `diff -r spikes/01-contract-rc/editorapi internal/views/editorapi`：只有第 7 行不同：
  `import "aurink.local/spikes/contract-rc/contract"` → `import "aurink/internal/contract"`。
- [实测] `go list -deps` 显示 `internal/contract` 的非标准库依赖只有它自身。
- [实测] `scripts/gate.ps1`：`Gate passed`（第 6 项跳过，原因同上）。
- [实测-CI] 三平台全绿：<https://github.com/oysterhyd/aurink/actions/runs/41302344666>（`53dd09a`）。
- 已合入 main 并打标签 `contract-v1.0-rc`（指向 `53dd09a`，已推送）。

## 合并记录

| main 提交 | 内容 |
|---|---|
| `5cf3305` | int/app：INT-0.1 |
| `53dd09a` | int/app：INT-0.2，标签 `contract-v1.0-rc` |
