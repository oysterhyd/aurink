# BE-1.2 Markdown

Recorded by Codex; the owner reran the tests and an independent differential. Windows, Ryzen 7 8845H.

# BE-1.2 incremental Markdown acceptance

The immutable snapshot design remains: relative block records and untouched subtrees are shared; placements are copied. The writer no longer calls parser.Document.Edit. All production and test changes are under internal/core/markdown; no commits, pushes, branch changes, dependency changes or protected-file edits were made.

## Window and reference context

The window contains complete top-level subtrees of every kind, including code, lists, quotes, tables, HTML, math and footnote definitions. Original LF/CRLF bytes are parsed directly. It starts with an unchanged context subtree and checks two unchanged suffix subtrees before sharing the suffix. A boundary cannot split a table delimiter or other overlapping source envelope. If an indented line parses differently without its preceding lazy-paragraph context, the window grows left. Lookahead grows through 4, 8, 16 and 32 following top-level subtrees; the complete window is capped at 24 KiB and 64 top-level subtrees.

The pinned parser consumes definition syntax independently of definitions it has already seen: reference lookup affects inlines, not block recognition. Window blocks therefore do not require injected definitions. Inline parsing keeps the snapshot's full document definition context. After a definition change, candidate reference leaves are compared in that context and only changed users are added to Dirty. Definition signatures include labels, including labels of empty footnotes. Tests cover additions, removals, duplicates, first-definition precedence, distant table users and empty-footnote renames.

Dirty also includes reparenting and subtree-size changes. The 30,000-step oracle compares each live ID with its predecessor, including source bytes, normalized block fields, parent identity and normalized inline nodes. Position-only movement is ignored.

The original parser.Edit witnesses remain. The table and adjacent-indented-code witnesses now assert a fresh local repair with zero full rebuilds. The ambiguous caret-label witness asserts the counted conservative path.

## Deterministic work and conservative paths

TestCorpusWindowWork covers all 13 requested edits and their inverses for LF and CRLF at 200 KiB and 1 MiB. It asserts at most **32 adapted blocks**, including lookahead and failed attempts, zero full rebuilds, and exactly the same work counts at both sizes. Observed work is 2-28 blocks. Every result is compared with a full parse, and Dirty is checked.

FullRebuilds counts every conservative update and equals the sum of these counters:

| Counter | Reason | Dedicated test |
|---|---|---|
| BoundaryFallbacks | No certified suffix after bounded growth, such as an unclosed fence swallowing the rest | TestConservativePaths/swallow |
| WindowLimitFallbacks | A complete subtree or edit span exceeds 24 KiB / 64 top-level subtrees | TestConservativePaths/release, large_paste |
| FrontMatterFallbacks | Front Matter envelope or contents change | TestConservativePaths/front |
| ReferenceFallbacks | Ambiguous caret label | TestConservativePaths/caret |

The additional size-limit exception is deliberate: the public parser has no restart state inside a top-level subtree, so an oversized container, paste, or widely separated batch cannot be safely parsed as the bounded window. It uses a counted full parse. Releasing a previously swallowed document also reaches this guard. TestWindowGrowth verifies that an HTML comment can consume multiple initial windows and still recover a suffix without a rebuild.

The existing CodeFallbacks, TableFallbacks, Resyncs and ParserEdits fields are preserved and stay zero. Those parser.Edit paths have been replaced by fresh windows.

TestCorpusRandomWork performs 500 random edits and inverses per line-ending variant across corpus.Generate(200<<10, 1331). Edits include typing ASCII/CJK, spaces, Enter and deleting a rune. Each of the 2,000 updates is compared with a full parse and checked for Dirty correctness. Inverses keep the workload on a realistic document instead of accumulating random syntax damage.

| Run | Updates | Full rebuilds | Boundary | Size limit | Caret label | Front Matter | Adapted total | Local parse attempts |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| Corpus LF | 1000 | 0 | 0 | 0 | 0 | 0 | 8926 | 413 |
| Corpus CRLF | 1000 | 0 | 0 | 0 | 0 | 0 | 8626 | 406 |
| Fragment seeds, combined | 30000 | 5780 | 0 | 0 | 5773 | 7 | 304363 | 25175 |

Raw deterministic test counts:

```text
random_test.go:180: 10000 steps; survivors=141833, disjoint disappearances=4458; counts: {AdaptedBlocks:116507 FullRebuilds:2383 BoundaryFallbacks:0 WindowLimitFallbacks:0 ReferenceFallbacks:2381 FrontMatterFallbacks:2 LocalParses:8035}
random_test.go:180: 10000 steps; survivors=84316, disjoint disappearances=3818; counts: {AdaptedBlocks:89525 FullRebuilds:1696 BoundaryFallbacks:0 WindowLimitFallbacks:0 ReferenceFallbacks:1691 FrontMatterFallbacks:5 LocalParses:8490}
random_test.go:180: 10000 steps; survivors=103544, disjoint disappearances=4050; counts: {AdaptedBlocks:98331 FullRebuilds:1701 BoundaryFallbacks:0 WindowLimitFallbacks:0 ReferenceFallbacks:1701 FrontMatterFallbacks:0 LocalParses:8650}
window_test.go:236: 1000 updates on generated corpus (500 edits and inverses): {AdaptedBlocks:8926 FullRebuilds:0 BoundaryFallbacks:0 WindowLimitFallbacks:0 ReferenceFallbacks:0 FrontMatterFallbacks:0 LocalParses:413}
window_test.go:236: 1000 updates on generated corpus (500 edits and inverses): {AdaptedBlocks:8626 FullRebuilds:0 BoundaryFallbacks:0 WindowLimitFallbacks:0 ReferenceFallbacks:0 FrontMatterFallbacks:0 LocalParses:406}
```

## Performance

Windows amd64, AMD Ryzen 7 8845H, default GOMAXPROCS=16, CGO_ENABLED=0. Each case uses corpus.Generate(size, 1331), with the same text converted to CRLF for that variant. There are 500 samples per case. QPC measures Apply, including text replacement and the placement-array copy. Every sample applies the specified forward edit to the same immutable baseline, so inverse edits and changing document shapes do not contaminate the percentile.

Cells are **p50 / p95 in milliseconds**. The conservative case inserts a four-backtick fence, for which the corpus has no matching closer. It performs exactly one full rebuild per sample. Every ordinary case performs zero full rebuilds.

| Edit | LF 200 KiB | CRLF 200 KiB | LF 1 MiB | CRLF 1 MiB | Adapted |
|---|---:|---:|---:|---:|---:|
| paragraph middle | 0.048 / 0.313 | 0.071 / 0.385 | 0.715 / 1.606 | 0.761 / 1.653 | 2 |
| paragraph first | 0.223 / 0.555 | 0.322 / 0.777 | 1.684 / 2.936 | 1.825 / 2.890 | 13 |
| code typing | 0.230 / 0.555 | 0.299 / 0.737 | 1.730 / 2.889 | 1.761 / 3.030 | 9 |
| code newline | 0.242 / 0.574 | 0.315 / 0.734 | 1.779 / 3.009 | 1.899 / 2.884 | 9 |
| paragraph enter | 0.237 / 0.595 | 0.316 / 0.771 | 1.564 / 2.740 | 2.012 / 3.034 | 13 |
| list typing | 0.046 / 0.283 | 0.065 / 0.518 | 0.880 / 1.864 | 0.794 / 1.730 | 4 |
| list enter | 0.224 / 0.555 | 0.314 / 0.777 | 2.020 / 3.340 | 1.713 / 3.404 | 13 |
| list marker | 0.237 / 0.618 | 0.324 / 0.784 | 1.769 / 3.053 | 1.807 / 3.021 | 12 |
| quote start | 0.270 / 0.773 | 0.336 / 0.799 | 1.581 / 2.763 | 1.582 / 2.657 | 15 |
| heading start | 0.283 / 0.749 | 0.318 / 0.732 | 1.502 / 2.630 | 1.582 / 2.719 | 9 |
| table cell | 0.328 / 0.788 | 0.343 / 0.790 | 1.653 / 2.664 | 1.670 / 2.944 | 27 |
| paragraph pipe | 0.280 / 0.670 | 0.334 / 0.779 | 1.550 / 2.725 | 1.632 / 2.595 | 13 |
| table enter | 0.322 / 0.759 | 0.334 / 0.808 | 1.847 / 2.933 | 1.790 / 3.059 | 28 |
| conservative fence | 1.991 / 2.823 | 1.973 / 2.772 | 11.199 / 14.726 | 10.801 / 13.710 | full rebuild |

The worst ordinary 200 KiB p95 is **0.8076 ms**. All requested ordinary cases meet 2 ms. The conservative rebuild exceeds 2 ms because it parses the complete document and matches old block identities. At 1 MiB many ordinary cases exceed 2 ms; they still adapt the same small number of blocks, while source and placement-array copies grow with document size. The performance target applies to 200 KiB.

Command:

```powershell
$env:CGO_ENABLED='0'
go test -run '^$' -bench BenchmarkWindowUpdate -benchtime=500x -count=1 ./internal/core/markdown
```

Raw benchmark output:

```text
goos: windows
goarch: amd64
pkg: aurink/internal/core/markdown
cpu: AMD Ryzen 7 8845H w/ Radeon 780M Graphics      
BenchmarkWindowUpdate/LF/200KB/paragraph_middle-16         	     500	     99866 ns/op	         2.000 adapted/update	     48100 p50-ns/update	    313000 p95-ns/update	         0 rebuilds/update	  926971 B/op	      15 allocs/op
BenchmarkWindowUpdate/LF/200KB/paragraph_first-16          	     500	    343614 ns/op	        13.00 adapted/update	    223100 p50-ns/update	    555300 p95-ns/update	         0 rebuilds/update	 1076447 B/op	     216 allocs/op
BenchmarkWindowUpdate/LF/200KB/code_typing-16              	     500	    411983 ns/op	         9.000 adapted/update	    229600 p50-ns/update	    555500 p95-ns/update	         0 rebuilds/update	 1049711 B/op	     164 allocs/op
BenchmarkWindowUpdate/LF/200KB/code_newline-16             	     500	    454229 ns/op	         9.000 adapted/update	    242100 p50-ns/update	    574400 p95-ns/update	         0 rebuilds/update	 1049805 B/op	     165 allocs/op
BenchmarkWindowUpdate/LF/200KB/paragraph_enter-16          	     500	    388398 ns/op	        13.00 adapted/update	    236900 p50-ns/update	    594800 p95-ns/update	         0 rebuilds/update	 1076456 B/op	     218 allocs/op
BenchmarkWindowUpdate/LF/200KB/list_typing-16              	     500	     90128 ns/op	         4.000 adapted/update	     45600 p50-ns/update	    282700 p95-ns/update	         0 rebuilds/update	  927744 B/op	      17 allocs/op
BenchmarkWindowUpdate/LF/200KB/list_enter-16               	     500	    418154 ns/op	        13.00 adapted/update	    223900 p50-ns/update	    555000 p95-ns/update	         0 rebuilds/update	 1074764 B/op	     213 allocs/op
BenchmarkWindowUpdate/LF/200KB/list_marker-16              	     500	    453353 ns/op	        12.00 adapted/update	    237400 p50-ns/update	    617800 p95-ns/update	         0 rebuilds/update	 1068870 B/op	     195 allocs/op
BenchmarkWindowUpdate/LF/200KB/quote_start-16              	     500	    703153 ns/op	        15.00 adapted/update	    269900 p50-ns/update	    773200 p95-ns/update	         0 rebuilds/update	 1080318 B/op	     216 allocs/op
BenchmarkWindowUpdate/LF/200KB/heading_start-16            	     500	    629357 ns/op	         9.000 adapted/update	    282800 p50-ns/update	    749300 p95-ns/update	         0 rebuilds/update	 1048875 B/op	     152 allocs/op
BenchmarkWindowUpdate/LF/200KB/table_cell-16               	     500	    587897 ns/op	        27.00 adapted/update	    327600 p50-ns/update	    788300 p95-ns/update	         0 rebuilds/update	 1110094 B/op	     382 allocs/op
BenchmarkWindowUpdate/LF/200KB/paragraph_pipe-16           	     500	    532004 ns/op	        13.00 adapted/update	    280100 p50-ns/update	    670300 p95-ns/update	         0 rebuilds/update	 1076074 B/op	     216 allocs/op
BenchmarkWindowUpdate/LF/200KB/table_enter-16              	     500	    486700 ns/op	        28.00 adapted/update	    322100 p50-ns/update	    758800 p95-ns/update	         0 rebuilds/update	 1134976 B/op	     391 allocs/op
BenchmarkWindowUpdate/LF/200KB/conservative_fence-16       	     500	   2114942 ns/op	        10.00 adapted/update	   1990700 p50-ns/update	   2823100 p95-ns/update	         1.000 rebuilds/update	 5615246 B/op	    7109 allocs/op
BenchmarkWindowUpdate/LF/1024KB/paragraph_middle-16        	     500	    903961 ns/op	         2.000 adapted/update	    715400 p50-ns/update	   1605800 p95-ns/update	         0 rebuilds/update	 4728058 B/op	      15 allocs/op
BenchmarkWindowUpdate/LF/1024KB/paragraph_first-16         	     500	   1908221 ns/op	        13.00 adapted/update	   1684000 p50-ns/update	   2935900 p95-ns/update	         0 rebuilds/update	 5158884 B/op	     217 allocs/op
BenchmarkWindowUpdate/LF/1024KB/code_typing-16             	     500	   1937017 ns/op	         9.000 adapted/update	   1730300 p50-ns/update	   2888700 p95-ns/update	         0 rebuilds/update	 5129321 B/op	     164 allocs/op
BenchmarkWindowUpdate/LF/1024KB/code_newline-16            	     500	   1968985 ns/op	         9.000 adapted/update	   1778800 p50-ns/update	   3008600 p95-ns/update	         0 rebuilds/update	 5129518 B/op	     165 allocs/op
BenchmarkWindowUpdate/LF/1024KB/paragraph_enter-16         	     500	   1853770 ns/op	        13.00 adapted/update	   1564400 p50-ns/update	   2740200 p95-ns/update	         0 rebuilds/update	 5152910 B/op	     218 allocs/op
BenchmarkWindowUpdate/LF/1024KB/list_typing-16             	     500	   1054423 ns/op	         4.000 adapted/update	    880000 p50-ns/update	   1864000 p95-ns/update	         0 rebuilds/update	 4728833 B/op	      17 allocs/op
BenchmarkWindowUpdate/LF/1024KB/list_enter-16              	     500	   2214482 ns/op	        13.00 adapted/update	   2019800 p50-ns/update	   3339600 p95-ns/update	         0 rebuilds/update	 5152092 B/op	     212 allocs/op
BenchmarkWindowUpdate/LF/1024KB/list_marker-16             	     500	   1994694 ns/op	        12.00 adapted/update	   1768800 p50-ns/update	   3053400 p95-ns/update	         0 rebuilds/update	 5148111 B/op	     195 allocs/op
BenchmarkWindowUpdate/LF/1024KB/quote_start-16             	     500	   1837926 ns/op	        15.00 adapted/update	   1581200 p50-ns/update	   2763400 p95-ns/update	         0 rebuilds/update	 5156575 B/op	     216 allocs/op
BenchmarkWindowUpdate/LF/1024KB/heading_start-16           	     500	   1764379 ns/op	         9.000 adapted/update	   1502300 p50-ns/update	   2630300 p95-ns/update	         0 rebuilds/update	 5128497 B/op	     152 allocs/op
BenchmarkWindowUpdate/LF/1024KB/table_cell-16              	     500	   1885625 ns/op	        27.00 adapted/update	   1652700 p50-ns/update	   2664500 p95-ns/update	         0 rebuilds/update	 5188677 B/op	     381 allocs/op
BenchmarkWindowUpdate/LF/1024KB/paragraph_pipe-16          	     500	   1827168 ns/op	        13.00 adapted/update	   1549900 p50-ns/update	   2724700 p95-ns/update	         0 rebuilds/update	 5153960 B/op	     216 allocs/op
BenchmarkWindowUpdate/LF/1024KB/table_enter-16             	     500	   1980686 ns/op	        28.00 adapted/update	   1847400 p50-ns/update	   2932700 p95-ns/update	         0 rebuilds/update	 5211812 B/op	     390 allocs/op
BenchmarkWindowUpdate/LF/1024KB/conservative_fence-16      	     500	  11481010 ns/op	        10.00 adapted/update	  11199400 p50-ns/update	  14725500 p95-ns/update	         1.000 rebuilds/update	32884547 B/op	   35930 allocs/op
BenchmarkWindowUpdate/CRLF/200KB/paragraph_middle-16       	     500	    195059 ns/op	         2.000 adapted/update	     71300 p50-ns/update	    385100 p95-ns/update	         0 rebuilds/update	  935152 B/op	      15 allocs/op
BenchmarkWindowUpdate/CRLF/200KB/paragraph_first-16        	     500	    428973 ns/op	        13.00 adapted/update	    322400 p50-ns/update	    777100 p95-ns/update	         0 rebuilds/update	 1085256 B/op	     219 allocs/op
BenchmarkWindowUpdate/CRLF/200KB/code_typing-16            	     500	    440223 ns/op	         9.000 adapted/update	    298800 p50-ns/update	    736600 p95-ns/update	         0 rebuilds/update	 1064971 B/op	     165 allocs/op
BenchmarkWindowUpdate/CRLF/200KB/code_newline-16           	     500	    469576 ns/op	         9.000 adapted/update	    315200 p50-ns/update	    734400 p95-ns/update	         0 rebuilds/update	 1066256 B/op	     166 allocs/op
BenchmarkWindowUpdate/CRLF/200KB/paragraph_enter-16        	     500	    395829 ns/op	        13.00 adapted/update	    316000 p50-ns/update	    771400 p95-ns/update	         0 rebuilds/update	 1085452 B/op	     220 allocs/op
BenchmarkWindowUpdate/CRLF/200KB/list_typing-16            	     500	    303237 ns/op	         4.000 adapted/update	     64700 p50-ns/update	    517900 p95-ns/update	         0 rebuilds/update	  935936 B/op	      17 allocs/op
BenchmarkWindowUpdate/CRLF/200KB/list_enter-16             	     500	    515614 ns/op	        13.00 adapted/update	    313500 p50-ns/update	    777500 p95-ns/update	         0 rebuilds/update	 1082785 B/op	     214 allocs/op
BenchmarkWindowUpdate/CRLF/200KB/list_marker-16            	     500	    505509 ns/op	        12.00 adapted/update	    324200 p50-ns/update	    783800 p95-ns/update	         0 rebuilds/update	 1078116 B/op	     197 allocs/op
BenchmarkWindowUpdate/CRLF/200KB/quote_start-16            	     500	    433649 ns/op	        15.00 adapted/update	    336300 p50-ns/update	    799500 p95-ns/update	         0 rebuilds/update	 1088381 B/op	     220 allocs/op
BenchmarkWindowUpdate/CRLF/200KB/heading_start-16          	     500	    438098 ns/op	         9.000 adapted/update	    317900 p50-ns/update	    731600 p95-ns/update	         0 rebuilds/update	 1064205 B/op	     153 allocs/op
BenchmarkWindowUpdate/CRLF/200KB/table_cell-16             	     500	    500151 ns/op	        27.00 adapted/update	    342600 p50-ns/update	    790400 p95-ns/update	         0 rebuilds/update	 1118387 B/op	     382 allocs/op
BenchmarkWindowUpdate/CRLF/200KB/paragraph_pipe-16         	     500	    471196 ns/op	        13.00 adapted/update	    334400 p50-ns/update	    778800 p95-ns/update	         0 rebuilds/update	 1086094 B/op	     219 allocs/op
BenchmarkWindowUpdate/CRLF/200KB/table_enter-16            	     500	    475034 ns/op	        28.00 adapted/update	    334400 p50-ns/update	    807600 p95-ns/update	         0 rebuilds/update	 1143018 B/op	     393 allocs/op
BenchmarkWindowUpdate/CRLF/200KB/conservative_fence-16     	     500	   2034284 ns/op	        10.00 adapted/update	   1973300 p50-ns/update	   2771700 p95-ns/update	         1.000 rebuilds/update	 5624881 B/op	    7119 allocs/op
BenchmarkWindowUpdate/CRLF/1024KB/paragraph_middle-16      	     500	    917841 ns/op	         2.000 adapted/update	    761200 p50-ns/update	   1653300 p95-ns/update	         0 rebuilds/update	 4769009 B/op	      15 allocs/op
BenchmarkWindowUpdate/CRLF/1024KB/paragraph_first-16       	     500	   2026653 ns/op	        13.00 adapted/update	   1825300 p50-ns/update	   2890000 p95-ns/update	         0 rebuilds/update	 5195242 B/op	     218 allocs/op
BenchmarkWindowUpdate/CRLF/1024KB/code_typing-16           	     500	   1962884 ns/op	         9.000 adapted/update	   1761100 p50-ns/update	   3030300 p95-ns/update	         0 rebuilds/update	 5175339 B/op	     165 allocs/op
BenchmarkWindowUpdate/CRLF/1024KB/code_newline-16          	     500	   2041198 ns/op	         9.000 adapted/update	   1898800 p50-ns/update	   2884300 p95-ns/update	         0 rebuilds/update	 5179593 B/op	     166 allocs/op
BenchmarkWindowUpdate/CRLF/1024KB/paragraph_enter-16       	     500	   2104147 ns/op	        13.00 adapted/update	   2011800 p50-ns/update	   3034300 p95-ns/update	         0 rebuilds/update	 5195001 B/op	     220 allocs/op
BenchmarkWindowUpdate/CRLF/1024KB/list_typing-16           	     500	    971538 ns/op	         4.000 adapted/update	    793800 p50-ns/update	   1729800 p95-ns/update	         0 rebuilds/update	 4769793 B/op	      17 allocs/op
BenchmarkWindowUpdate/CRLF/1024KB/list_enter-16            	     500	   1999901 ns/op	        13.00 adapted/update	   1713400 p50-ns/update	   3404400 p95-ns/update	         0 rebuilds/update	 5192344 B/op	     214 allocs/op
BenchmarkWindowUpdate/CRLF/1024KB/list_marker-16           	     500	   1992388 ns/op	        12.00 adapted/update	   1806800 p50-ns/update	   3021400 p95-ns/update	         0 rebuilds/update	 5187963 B/op	     197 allocs/op
BenchmarkWindowUpdate/CRLF/1024KB/quote_start-16           	     500	   1807637 ns/op	        15.00 adapted/update	   1582400 p50-ns/update	   2657200 p95-ns/update	         0 rebuilds/update	 5203216 B/op	     221 allocs/op
BenchmarkWindowUpdate/CRLF/1024KB/heading_start-16         	     500	   1827084 ns/op	         9.000 adapted/update	   1581800 p50-ns/update	   2719100 p95-ns/update	         0 rebuilds/update	 5174869 B/op	     153 allocs/op
BenchmarkWindowUpdate/CRLF/1024KB/table_cell-16            	     500	   1913781 ns/op	        27.00 adapted/update	   1669800 p50-ns/update	   2944300 p95-ns/update	         0 rebuilds/update	 5227751 B/op	     381 allocs/op
BenchmarkWindowUpdate/CRLF/1024KB/paragraph_pipe-16        	     500	   1825134 ns/op	        13.00 adapted/update	   1632100 p50-ns/update	   2595100 p95-ns/update	         0 rebuilds/update	 5198326 B/op	     219 allocs/op
BenchmarkWindowUpdate/CRLF/1024KB/table_enter-16           	     500	   1981324 ns/op	        28.00 adapted/update	   1789900 p50-ns/update	   3058800 p95-ns/update	         0 rebuilds/update	 5252438 B/op	     393 allocs/op
BenchmarkWindowUpdate/CRLF/1024KB/conservative_fence-16    	     500	  11076305 ns/op	        10.00 adapted/update	  10801100 p50-ns/update	  13709700 p95-ns/update	         1.000 rebuilds/update	32925632 B/op	   35940 allocs/op
PASS
ok  	aurink/internal/core/markdown	45.492s
```

## Verification

```text
> gofmt -l internal/core/markdown
(no output; exit 0)
> go vet ./internal/core/markdown
(no output; exit 0)
> CGO_ENABLED=0 go test -count=1 -cover ./internal/core/markdown ./tests/arch
ok  	aurink/internal/core/markdown	162.439s	coverage: 97.6% of statements
ok  	aurink/tests/arch	0.646s	coverage: [no statements]
```

The complete suite includes contracttest.TestAST, all three 10,000-step seeds, the corpus matrix, snapshot immutability and all prior regressions. Focused concurrent snapshot readers also pass with -race. No contract concern was found. Other operating systems and hardware were not measured.

The temporary zz_accept_test.go and stray coverage / coverage.out files are absent.

## BE-1.2b long blank runs

Root cause: `maxWindowTops` counted coverage-only blank rows, so a run of about 30 blank lines forced a full parse on every edit. The lookahead and the limit now count nonblank subtrees; the 24 KiB limit and the anchor checks are unchanged.

Owner's rerun, 300 consecutive edits without undo, including events, no full rebuilds [实测]: worst p95 1.71 ms at 200 KB and 3.50 ms at 1 MB. Before: p50 about 15 ms and 76 ms. Single samples reach 28 ms at 200 KB, on a machine also running other builds; the cause of these is not established. Long differential run passed (236 s). Three mutations of the change all fail the tests.
