# Render check

Platform: windows/amd64; go1.27.3.

Formulas: 220 (210 successful timings); diagrams: 8. Uncached service p50 3.680 ms, p95 8.644 ms, max 934.732 ms. Including SVG parsing or PNG decoding: p50 3.906 ms, p95 9.242 ms, max 934.732 ms (budget p95 < 50 ms). Cached p50 0.001568 ms, p95 0.004057 ms, max 0.009023 ms (budget < 1 ms); each cache sample is the mean of 1,000 calls.

Cold start 908.831 ms (page assembly excluded); idle stopped: true; restart 402.674 ms; HTTP(S) resource loads: 0. Memory is WebView2 process-tree working set, not exclusive physical memory; S2 recorded 361–565 MiB.

- after_start: {Stage:after_start PageProcessID:42564 WebViewProcesses:6 WorkingSetBytes:378732544 PrivateBytes:205799424 Error:}
- after_corpus: {Stage:after_corpus PageProcessID:42564 WebViewProcesses:6 WorkingSetBytes:533454848 PrivateBytes:345513984 Error:}
- after_idle: {Stage:after_idle PageProcessID:42564 WebViewProcesses:6 WorkingSetBytes:358100992 PrivateBytes:177774592 Error:}

PNG path:

- cjk-scale-1: 11.350 ms, DIP 133.00 × 24.00, pixels 133 × 24, ascent 18.87, error 
- cjk-scale-2: 7.295 ms, DIP 133.00 × 23.50, pixels 266 × 47, ascent 18.87, error 

Failures and missed budgets:

None.
