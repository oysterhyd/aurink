package qa

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// Focus mode dims every block but the caret's to 0.28 (02 §5.10): a task's
// box, a quote and code keep their own colours, and dim with the rest.
func TestFocusModeDimsEveryOtherBlock(t *testing.T) {
	a := newApp(t)
	a.open("focus.md", "# Focus\n\ntext\n\n- [x] done\n\n> quote\n\n```go\nfunc main() {}\n```\n\n| a | b |\n|---|---|\n| c | d |\n")
	a.caret(len("# Focus\n\nte"))
	a.animate()
	l := a.s.ActiveLive()
	// The darkest level of any channel in a row's band, across the sheet. A
	// table's band is its first cell's height.
	ink := func(r int) int {
		img := a.tt.Image()
		y0 := int(l.RowText(r).Y) + 46
		y1 := y0 + int(max(l.RowHeight(r), l.RowHeight(r-1))) - 4
		m := 255
		for x := int(a.s.SheetLeft()); x < img.Bounds().Dx(); x++ {
			for y := y0; y < y1; y++ {
				c := img.RGBAAt(x, y)
				m = min(m, int(c.R), int(c.G), int(c.B))
			}
		}
		return m
	}
	rows := []int{2, 3, 4, l.RowCount() - 1}
	before := make([]int, len(rows))
	for i, r := range rows {
		before[i] = ink(r)
	}
	a.key(0, ui.KeyF8)
	a.animate()
	if !l.FocusMode() {
		t.Fatal("F8 did not turn focus mode on")
	}
	for i, r := range rows {
		// 0.28 of the ink over a light sheet: the darkest level rises a lot.
		if got := ink(r); got < before[i]+90 {
			t.Errorf("row %d: darkest %d before, %d in focus mode", r, before[i], got)
		}
	}
	if got := ink(1); got > 80 {
		t.Errorf("the caret's row dimmed: darkest %d", got)
	}
}
