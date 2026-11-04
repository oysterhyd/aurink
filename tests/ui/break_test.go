package ui_test

import (
	"testing"
)

// TestThematicBreakIsARule: a thematic break is a thin rule across the
// column, as Typora's, not a centred "···".
func TestThematicBreakIsARule(t *testing.T) {
	h := openRail(t, "11\n\n---\n\n111\n")
	h.caret(0)
	h.tt.Frame()
	img := h.tt.Image()
	at := h.e.RowText(1)
	bg := img.At(int(at.X)+300, int(h.e.RowText(0).Y)+30)
	ruled := -1
	for y := int(at.Y); y < int(at.Y+h.e.RowHeight(1)); y++ {
		if !sameColor(img.At(int(at.X)+2, y), bg) {
			ruled = y
			break
		}
	}
	if ruled < 0 {
		t.Fatal("no rule at the column's left edge")
	}
	for _, x := range []int{int(at.X) + 2, int(at.X) + 200, int(at.X) + 400, int(at.X) + 600} {
		if sameColor(img.At(x, ruled), bg) {
			t.Fatalf("the rule breaks at x %d (y %d)", x, ruled)
		}
	}
}
