# S2 · 离屏渲染探针

固定 MyGo v0.7.3。正式结论、版本/许可证和限制见 [REPORT.md](REPORT.md)。

在此目录使用 PowerShell 7：

```powershell
go build -o build/s2-render.exe .
./build/s2-render.exe -out evidence
./build/s2-render.exe -force-unavailable -out evidence
go test -count=1 -v ./...
```

原型打开 Native 对照窗口，测量结束自动退出。全部渲染资源通过 `go:embed` 打包；运行时无需 npm、网络或本地 HTTP 服务。渲染 WebView 从未传给 `ui.WebView`。`-force-unavailable` 只注入缺失后端状态，不修改系统 WebView2 安装。

`evidence/results.json` 是一次完整运行的数据；`run.log` 是对应输出。`evidence/svg/` 是实际 Eval 返回的规范化 SVG，也是离线回归测试夹具。浏览器参考 PNG 和 Native SVG 的逐例图在 `comparison-01.png` 至 `comparison-10.png`，每张五例。

Windows 内存采样只统计本次 WebView2 browser PID 及其子进程。其他平台可构建运行，内存采样明确返回未验证；本交付没有宣称三平台有窗口验证。

`build/` 是被仓库忽略的构建/下载临时目录，不提交。重跑结果会覆盖 evidence；在有未提交取证变更时先保存需要保留的运行数据。
