package markdown

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/tests/corpus"
)

func BenchmarkWindowUpdate(b *testing.B) {
	for _, eol := range []string{"\n", "\r\n"} {
		for _, size := range []int{200 << 10, 1 << 20} {
			src := corpus.Generate(size, 1331)
			label := "LF"
			if eol != "\n" {
				src = strings.ReplaceAll(src, "\n", eol)
				label = "CRLF"
			}
			cases := corpusEdits(b, src, eol)
			cases = append(cases, windowEdit{"conservative_fence", 0, "````" + eol})
			for _, c := range cases {
				b.Run(fmt.Sprintf("%s/%dKB/%s", label, size>>10, c.name), func(b *testing.B) {
					p := mustNew(b, src, 1, contract.ExtFrontMatter)
					// Reset to the same immutable snapshot outside the timed operation.
					// Each sample measures the specified forward edit, including text.Apply.
					base := p.Snapshot()
					samples := make([]time.Duration, b.N)
					var adapted, rebuilds int
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						p.current, p.next = base, base.highest
						start := benchmarkNow()
						u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: c.at, End: c.at}, Text: c.text}})
						samples[i] = time.Duration(benchmarkNow() - start)
						if err != nil {
							b.Fatal(err)
						}
						adapted += u.Stats.AdaptedBlocks
						rebuilds += u.Stats.FullRebuilds
					}
					b.StopTimer()
					sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
					b.ReportMetric(float64(samples[len(samples)/2].Nanoseconds()), "p50-ns/update")
					b.ReportMetric(float64(samples[min(len(samples)-1, len(samples)*95/100)].Nanoseconds()), "p95-ns/update")
					b.ReportMetric(float64(adapted)/float64(b.N), "adapted/update")
					b.ReportMetric(float64(rebuilds)/float64(b.N), "rebuilds/update")
				})
			}
		}
	}
}
