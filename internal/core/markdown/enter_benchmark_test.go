package markdown

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/tests/corpus"
)

// EnterWorkload and EnterClock are shared with the external document benchmark.
type EnterWorkload struct {
	Name, Source, Text string
	At                 int
}

func EnterClock() int64 { return benchmarkNow() }

func EnterWorkloads(t testing.TB, size int) []EnterWorkload {
	t.Helper()
	src := corpus.Generate(size, 1331)
	find := func(target string) int {
		at := strings.Index(src[len(src)/2:], target)
		if at < 0 {
			t.Fatalf("missing Enter target %q", target)
		}
		return len(src)/2 + at
	}
	para := find("English") + 3
	code := find("func f") + 3
	item := find("- 项 ") + len("- 项 ")
	gap := src[:para] + "\n\n" + strings.Repeat("\n", 500) + src[para:]
	return []EnterWorkload{
		{"paragraph", src, "\n\n", para},
		{"fence", src, "\n\n", code},
		{"list_item", src, "\n\n", item},
		{"list_continuation", src, "\n  \n  ", item},
		{"typing_500_blanks", gap, "a", para + 252},
	}
}

// Every run accumulates 300 edits, advancing the caret without undo or reset.
func BenchmarkHoldEnter(b *testing.B) {
	for _, size := range []int{200 << 10, 1 << 20} {
		for _, c := range EnterWorkloads(b, size) {
			b.Run(fmt.Sprintf("%dKB/%s", size>>10, c.Name), func(b *testing.B) {
				samples := make([]int64, 0, 300*b.N)
				var total Stats
				b.ReportAllocs()
				b.ResetTimer()
				for run := 0; run < b.N; run++ {
					b.StopTimer()
					p := mustNew(b, c.Source, 1, contract.ExtFrontMatter)
					at := c.At
					b.StartTimer()
					for step := 0; step < 300; step++ {
						start := benchmarkNow()
						u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: at, End: at}, Text: c.Text}})
						samples = append(samples, benchmarkNow()-start)
						if err != nil {
							b.Fatal(err)
						}
						addStats(&total, u.Stats)
						at += len(c.Text)
					}
				}
				b.StopTimer()
				slices.Sort(samples)
				count := float64(len(samples))
				b.ReportMetric(300, "edits/run")
				b.ReportMetric(float64(samples[len(samples)/2]), "p50-ns/edit")
				b.ReportMetric(float64(samples[len(samples)*95/100]), "p95-ns/edit")
				b.ReportMetric(float64(samples[len(samples)-1]), "max-ns/edit")
				b.ReportMetric(float64(total.FullRebuilds)/count, "rebuilds/edit")
				b.ReportMetric(float64(total.WindowLimitFallbacks)/count, "limits/edit")
				b.ReportMetric(float64(total.BoundaryFallbacks)/count, "boundaries/edit")
				b.ReportMetric(float64(total.AdaptedBlocks)/count, "adapted/edit")
			})
		}
	}
}
