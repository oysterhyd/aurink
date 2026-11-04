package ui_test

import (
	"strings"
	"testing"
	"time"
)

// TestTypingMovesNothing: as in Typora, typing a block's marks on a line
// moves neither the caret nor the text below until Enter. A heading, a rule,
// a fence, HTML or indented code shows as a draft paragraph while its line is
// typed (draft.go); an empty item, quote or footnote line is as tall as one
// with text; a list ends with a paragraph's spacing. Measured on Typora 1.0
// (2026-11-04).
func TestTypingMovesNothing(t *testing.T) {
	for _, seq := range []string{"# ab", "## ab", "- ab", "1. ab", "+ ab", "> ab", "- [ ] ab", "---", "***",
		"    ab", "<div>", "[^1]: ab", "[a]: http://x.y", "中文", "$x$", "|a|b|"} {
		h := openRail(t, "first\n\n\n\nlast\n")
		h.caret(7)
		settle := func() {
			for range 12 {
				h.tt.Advance(20 * time.Millisecond)
				h.tt.Frame()
			}
		}
		last := func() float32 {
			for r := range h.e.RowCount() {
				if strings.TrimSpace(h.e.Display(r)) == "last" {
					return h.e.RowText(r).Y
				}
			}
			t.Fatalf("%q: the text below is gone", seq)
			return 0
		}
		// From the first character that is not a space: a line of spaces is
		// still an empty line.
		runes := []rune(seq)
		k := len(seq) - len(strings.TrimLeft(seq, " "))
		h.typed(string(runes[:k+1]))
		runes = runes[k+1:]
		settle()
		_, y0, h0 := h.e.CaretDrawn()
		l0 := last()
		for _, ch := range runes {
			h.typed(string(ch))
			settle()
			if _, y, hh := h.e.CaretDrawn(); y != y0 || hh != h0 {
				t.Errorf("%q: after %q the caret went from y %.1f h %.1f to y %.1f h %.1f", seq, string(ch), y0, h0, y, hh)
			}
			if l := last(); l != l0 {
				t.Errorf("%q: after %q the text below moved from %.1f to %.1f", seq, string(ch), l0, l)
			}
		}
	}
}
