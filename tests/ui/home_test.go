package ui_test

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestHomeLandsWhereTheTextStarts: Home and Primary+Home go to where the text
// starts, after a heading's "# " or an item's "- ", as in Typora, not onto
// the marks the caret there would show; Backspace there then acts on the
// block (a heading becomes a paragraph).
func TestHomeLandsWhereTheTextStarts(t *testing.T) {
	for _, c := range []struct {
		src  string
		want int
	}{{"# ab", 2}, {"- ab", 2}, {"> ab", 2}, {"1. ab", 3}, {"ab", 0}} {
		h := openRail(t, c.src)
		h.caret(len(c.src))
		h.key(ui.KeyHome)
		if got := h.m.Selection().Primary().Head; got != c.want {
			t.Errorf("%q: Home went to %d, want %d", c.src, got, c.want)
		}
		h.caret(len(c.src))
		h.mod(ui.Cmd, ui.KeyHome)
		if got := h.m.Selection().Primary().Head; got != c.want {
			t.Errorf("%q: Primary+Home went to %d, want %d", c.src, got, c.want)
		}
	}
	h := openRail(t, "# ab")
	h.caret(4)
	h.mod(ui.Cmd, ui.KeyHome)
	h.key(ui.KeyBackspace)
	h.wantSrc("ab")
}
