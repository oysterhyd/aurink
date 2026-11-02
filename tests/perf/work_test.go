package perf

import (
	"testing"

	"aurink/internal/contract"
	"aurink/tests/corpus"
	"github.com/egoist/mygo/ui"
)

// TestTypingWorkStaysLocal guards keystroke cost by counting work instead of
// timing it (07 §7), so a busy CI machine cannot fail it and a slow change
// cannot hide behind a loose wall-clock limit. Measured on 2026-11-02: typing
// walks no rows, lays out 1-2 rows and allocates about 760 times per key at
// 200 KB and at 1 MB alike; Enter walks the rows once and allocates about 98k
// (200 KB) and 484k (1 MB) times. The bounds give 2x headroom.
func TestTypingWorkStaysLocal(t *testing.T) {
	perKey := map[int]float64{}
	for _, c := range []struct {
		size        int
		enterAllocs float64
	}{{200 << 10, 200_000}, {1 << 20, 1_000_000}} {
		src := corpus.Generate(c.size, 1)
		e, tt := openEditor(t, src)
		e.Model().SetSelection(contract.Caret(len(src) / 2))
		tt.Frame()
		walks := e.Stats.RowRebuilds
		e.Stats.MaxFrameShapes = 0
		allocs := testing.AllocsPerRun(200, func() { tt.Type("x"); tt.Frame() })
		if n := e.Stats.RowRebuilds - walks; n != 0 {
			t.Errorf("%d KB: typing walked all rows %d times", c.size>>10, n)
		}
		if e.Stats.MaxFrameShapes > 4 {
			t.Errorf("%d KB: a keystroke laid out %d rows", c.size>>10, e.Stats.MaxFrameShapes)
		}
		if allocs > 1500 {
			t.Errorf("%d KB: %.0f allocations per keystroke, want at most 1500", c.size>>10, allocs)
		}
		perKey[c.size] = allocs
		e.Stats.MaxFrameShapes = 0
		enter := testing.AllocsPerRun(10, func() { tt.Key(0, ui.KeyEnter); tt.Frame() })
		if e.Stats.MaxFrameShapes > 6 {
			t.Errorf("%d KB: Enter laid out %d rows", c.size>>10, e.Stats.MaxFrameShapes)
		}
		if enter > c.enterAllocs {
			t.Errorf("%d KB: %.0f allocations per Enter, want at most %.0f", c.size>>10, enter, c.enterAllocs)
		}
	}
	// Typing must not cost more in a bigger document.
	if small, big := perKey[200<<10], perKey[1<<20]; big > small*1.5 {
		t.Errorf("allocations per keystroke grow with the document: %.0f at 200 KB, %.0f at 1 MB", small, big)
	}
}
