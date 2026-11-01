package ui_test

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestKeystrokeFrameBudget measures input plus frame on a 200 KB document: p95 below
// 8 ms is the card's budget (05 §6). It reports the first frame too (ADR-001).
func TestKeystrokeFrameBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("measures")
	}
	src := longDoc(2500)
	for len(src) < 200<<10 {
		src += longDoc(100)
	}
	start := time.Now()
	h := open(t, src)
	t.Logf("%d KB, %d rows, first frame with parse %v", len(src)>>10, h.e.RowCount(), time.Since(start))
	h.caret(len(src) / 2)
	const n = 600
	times := make([]time.Duration, 0, n)
	for i := range n {
		t0 := time.Now()
		if i%40 == 39 {
			h.tt.Key(0, ui.KeyEnter)
		} else {
			h.tt.Type("x")
		}
		h.tt.Frame()
		times = append(times, time.Since(t0))
	}
	slices.Sort(times)
	p95 := times[n*95/100]
	t.Logf("p50 %v p95 %v max %v, walks %d", times[n/2], p95, times[n-1], h.e.Stats.RowRebuilds)
	if p95 >= 8*time.Millisecond {
		t.Fatalf("p95 %v", p95)
	}
}
