package markdown_test

import (
	"fmt"
	"slices"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/document"
	"aurink/internal/core/markdown"
)

// Measure the same accumulating workload through transactions and subscribers.
func BenchmarkHoldEnterEvents(b *testing.B) {
	for _, size := range []int{200 << 10, 1 << 20} {
		for _, c := range markdown.EnterWorkloads(b, size) {
			b.Run(fmt.Sprintf("%dKB/%s", size>>10, c.Name), func(b *testing.B) {
				samples := make([]int64, 0, 300*b.N)
				events, dirty := 0, 0
				b.ReportAllocs()
				b.ResetTimer()
				for run := 0; run < b.N; run++ {
					b.StopTimer()
					m := (document.Factory{}).New("enter", c.Source, contract.DocMeta{}, contract.ExtFrontMatter)
					if m == nil {
						b.Fatal("document creation failed")
					}
					m.SetSelection(contract.Caret(c.At))
					m.Subscribe(func(e contract.DocEvent) {
						events++
						dirty += len(e.Dirty)
					})
					at := c.At
					b.StartTimer()
					for step := 0; step < 300; step++ {
						start := markdown.EnterClock()
						_, err := m.Apply(contract.Transaction{
							Edits:  []contract.TextEdit{{Range: contract.Range{Start: at, End: at}, Text: c.Text}},
							Origin: contract.OriginUser, Coalesce: "typing", Label: "editing_undo_typing",
						})
						samples = append(samples, markdown.EnterClock()-start)
						if err != nil {
							b.Fatal(err)
						}
						at += len(c.Text)
					}
					b.StopTimer()
					if m.Text().Len() != len(c.Source)+300*len(c.Text) {
						b.Fatal("edits did not accumulate")
					}
					b.StartTimer()
				}
				b.StopTimer()
				if events != len(samples) || dirty == 0 {
					b.Fatal("missing text events", events, dirty)
				}
				slices.Sort(samples)
				b.ReportMetric(300, "edits/run")
				b.ReportMetric(float64(events)/float64(len(samples)), "events/edit")
				b.ReportMetric(float64(samples[len(samples)/2]), "p50-ns/edit")
				b.ReportMetric(float64(samples[len(samples)*95/100]), "p95-ns/edit")
				b.ReportMetric(float64(samples[len(samples)-1]), "max-ns/edit")
			})
		}
	}
}
