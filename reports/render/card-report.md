# BE-2.9 report

Status: implementation and Windows render evidence are ready for review. The owner confirmed that race testing is required for this card's packages and that the earlier repository-wide race failures in unrelated existing tests are not the acceptance bar. No commits or branches were created.

## Files

- `internal/core/render/render.go`: `contract.Renderer`, lazy and serial host lifecycle, cancellation and timeouts, failed-start cooldown, a separate configurable startup deadline (default 10 s), generation-based failure of queued requests when the page goes away, idle stop/restart, page protocol and answer validation. The request timeout covers queueing and evaluation while pausing during startup; caller cancellation remains active throughout.
- `internal/core/render/cache.go`: page-versioned request SHA-256 keys, defensive-copy memory LRU (256 entries / 32 MiB), atomic disk entries and oldest-modified eviction at 64 MiB.
- `internal/core/render/conformance_test.go`: contract suite and cache-hit benchmark.
- `internal/core/render/lifecycle_test.go`: queued crash handling, cooldown, slow startup beyond the request timeout, bounded startup, queue timeout and cancellation, cache isolation and eviction, disk page-version changes, invalid SVG/baseline validation.
- `internal/adapters/webviewhost/host.go`: native window's unmounted MyGo WebView, `ThrottlingOff`, nonempty base URL, ready/load and render-process-gone handling, bounded evaluation, and `Destroy` on stop. The blocking/threading requirements are documented in the type comment.
- `resources/render/page.html`: offline CSP, MathJax SVG, array rule preservation, inline ascent, SVG-to-PNG fallback and scaled Mermaid PNG, colors and dark theme, error reporting.
- `resources/render/render.go`: `go:embed` assets and `Page()` assembly. `resources/render/vendor/` contains the two unchanged libraries and their two licenses. SHA-256: MathJax `d4295dc33744836935c1399feece5159577b34c5c8ffb9f1c6324cd82e03a882`; Mermaid `5a8ec91820bd55afef049068489369910e5d6ce70c8103952f27e29d3e76e8bc`, matching S2.
- `testdata/render/formulas.json`: 220 cases, the original S2 50 plus 160 additional valid varied formulas and 10 invalid inputs. `testdata/render/diagrams.json`: S2's six diagrams plus CJK and a 30-edge diagram.
- `tools/rendercheck/main.go`: real-window corpus, timing, parsing/ink, raster scale/baseline, theme, offline resource, idle/restart, and source-fallback checks. `memory_windows.go` tracks the WebView2 browser PID and descendants; `memory_other.go` reports that memory measurement is Windows-only.
- `reports/render/`: `summary.md`, `results.json`, `run.log`, prior successful run results/summaries, `startup-first-attempt.md`, `gate.log`, rendered formula grid, array rule, scale, color, diagram, and fallback images and fallback JSON.

## Card deliverables and measurements

1. Renderer, serial lifecycle, cache, answer validation, and suite: implemented. Focused race suite passes, including queued requests and cooldown. Cache hit benchmark: 769.6 ns/op (416 B/op, five allocations) in the final run.
2. Embedded offline resources: implemented; hashes and licenses above. Rendercheck observed zero HTTP(S) resource loads. Color was visible in native SVG, PNG, and a dark Mermaid image.
3. MyGo host: implemented and exercised on Windows. `Destroy` returns and the WebView is recreated successfully after idle. The process tree still had six processes and 341.51 MiB working set immediately after idle; a longer-term process exit was not measured. Do not assume that closing the view immediately terminates the shared WebView2 process tree.
4. Corpus: 210 valid formulas (187 SVG, 23 PNG, all parsed/decoded and had visible ink), 10 expected `CodeRender` errors, eight rendered diagrams. Formula grid and diagram PNGs are in `reports/render/`.
5. Real-window measurements: service plus `ui.ParseSVG` or PNG decode p50 3.906 ms, p95 9.242 ms, max 934.732 ms; p95 target <50 ms passed on this run. The maximum includes cold start. Cached p50 0.001568 ms, p95 0.004057 ms, max 0.009023 ms; these are means of 1,000 calls per sample, not individual worst-case calls. Standalone benchmark also passes <1 ms. CJK inline PNG: scale 1, 11.350 ms, 133×24 DIP and 133×24 pixels, ascent 18.87 DIP; scale 2, 7.295 ms, 133×23.5 DIP and 266×47 pixels, ascent 18.87 DIP. These checks and ink passed, but are not a subjective baseline-alignment study.
6. Cold start was 908.831 ms (excluding HTML assembly) on the final run; restart after idle was 402.674 ms. Process-tree working set was 361.19 MiB after start, 508.74 MiB after corpus, and 341.51 MiB immediately after idle stop; compare S2's 361–565 MiB. Working-set sums can include shared pages. `-force-unavailable` returned `CodeUnavailable`, never started a page, and displayed both formula and Mermaid source (`fallback-window.png`). Runtime measurements are Windows only; the gate cross-builds Windows, Linux, and macOS.
7. Before the owner-requested timeout change, the first actual startup attempt hit the then-shared 2 s deadline: Start 2000.909 ms, first request 2002.414 ms, followed by the required one-minute cooldown. Its cause was not determined. Start now has its own default 10 s deadline; the focused regression test holds Start beyond the request timeout and verifies that both the initiating and queued request still succeed. Subsequent fresh processes succeeded; do not infer that every cold start meets the new deadline. This failed attempt is documented in `reports/render/startup-first-attempt.md`, rather than discarded. Final successful run reports no corpus or p95 budget failures.

## Commands and results

All commands were run from `D:\mygo\project\Aurink_claude-wt\be-2.9` in PowerShell 7.

- `go test -count=1 -race ./internal/core/render ./internal/adapters/webviewhost` (after final changes): `ok aurink/internal/core/render 2.371s`; webviewhost has no test files.
- `go test -run '^$' -bench BenchmarkCacheHit -benchmem ./internal/core/render`: `BenchmarkCacheHit-16 1514558 769.6 ns/op 416 B/op 5 allocs/op`, `PASS`.
- `go build -o tools/rendercheck/rendercheck.exe ./tools/rendercheck`: passed.
- `& 'tools/rendercheck/rendercheck.exe' *> 'reports/render/run.log'`: exit 0 on final run; evidence in `reports/render/results.json` and `summary.md`. The first run's startup timeout is separately documented above; prior subsequent run results are retained.
- `& 'tools/rendercheck/rendercheck.exe' -force-unavailable`: exit 0; fallback JSON and screenshot written.
- `Get-FileHash -Algorithm SHA256 -LiteralPath 'resources/render/vendor/mathjax-3.2.2-tex-svg.js','resources/render/vendor/mermaid-10.9.3.min.js'`: matches both S2 hashes above.
- `go vet ./...`: exit 0, no output, after final changes.
- `gofmt -l .`: exit 0, no output, after final changes.
- `pwsh -NoProfile -File scripts/gate.ps1`: exit 0, after final changes. Last lines: `=== 6. i18ncheck`, `160 messages in en, zh-Hans`, `Gate passed`. The full log is `reports/render/gate.log`.
- `go test -count=1 -race ./internal/... ./tests/...`: **failed**. `aurink/internal/core/markdown` timed out at the default 10 minutes while running `TestCorpusWindowWork/2/1024/list_marker`; `aurink/tests/ui` failed `TestKeystrokeFrameBudget`, p95 28.0767 ms against its 16 ms assertion. The command began before the final renderer validation changes; the changed renderer was rechecked separately under race afterwards. The failure was not a renderer race finding.
- `go test -count=1 -race -run '^TestKeystrokeFrameBudget$' -v ./tests/ui`: **failed** on a separate attempt: p50 9.402 ms, p95 17.708 ms, max 66.6297 ms; assertion requires p95 <16 ms. The non-race gate's UI suite passed. No tests, thresholds, or unrelated code were modified.

No contract test case was altered. Per the owner's follow-up, the two repository-wide race failures are existing issues outside this card's acceptance bar; macOS/Linux real WebView behavior and delayed process-tree release remain unmeasured.

## Acceptance follow-up: disk eviction

`internal/core/render/lifecycle_test.go` now fills the disk cache through `cache.put` with three 22 MiB entries. The second entry is assigned an older modification time than the first, so the test checks eviction by modification time rather than insertion order. It verifies the oldest entry is removed, both newer entries remain, and the total stays at or below 64 MiB. It does not call `evictDisk` directly.

- `go test -count=1 -run '^TestDiskEvictsOldest$' -v ./internal/core/render`: passed (`--- PASS: TestDiskEvictsOldest (0.13s)`).
- Mutant check: the following PowerShell temporarily removed the `c.evictDisk()` call from `cache.put`, ran the same test, and restored the original file in `finally`:

```powershell
$targetPath = 'internal/core/render/cache.go'
$originalText = [System.IO.File]::ReadAllText((Resolve-Path -LiteralPath $targetPath))
$pattern = '(?m)^\tc\.evictDisk\(\)\r?\n'
if ([regex]::Matches($originalText, $pattern).Count -ne 1) { throw 'Expected exactly one eviction call' }
$mutationExit = -1
try {
    $mutantText = [regex]::Replace($originalText, $pattern, '')
    [System.IO.File]::WriteAllText((Resolve-Path -LiteralPath $targetPath), $mutantText, [System.Text.UTF8Encoding]::new($false))
    & go test -count=1 -run '^TestDiskEvictsOldest$' -v ./internal/core/render
    $mutationExit = $LASTEXITCODE
}
finally {
    [System.IO.File]::WriteAllText((Resolve-Path -LiteralPath $targetPath), $originalText, [System.Text.UTF8Encoding]::new($false))
}
if ($mutationExit -ne 1) { throw "Expected mutant test exit 1, got $mutationExit" }
Write-Output "Mutant test exited $mutationExit as expected; original cache.go restored."
```

  The mutant failed as expected: `disk cache has 69206382 bytes, limit 67108864` and `oldest file retained: <nil>`. The original call was restored.
- `go test -count=1 -race ./internal/core/render ./internal/adapters/webviewhost`: passed (`ok aurink/internal/core/render 3.124s`; webviewhost has no test files).
- `go vet ./...` and `gofmt -l .`: passed, no output.
- `pwsh -NoProfile -File scripts/gate.ps1`: passed after the follow-up; final lines were `160 messages in en, zh-Hans` and `Gate passed`.
