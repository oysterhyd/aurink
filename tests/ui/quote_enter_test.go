package ui_test

import (
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestEnterContinuesTheQuote: as in Typora, Enter at the end of a quoted line
// opens another line of the quote, the caret in it; Enter again on that empty
// line leaves the quote.
func TestEnterContinuesTheQuote(t *testing.T) {
	h := openRail(t, "> a")
	h.caret(3)
	h.key(ui.KeyEnter)
	h.wantSrc("> a\n> ")
	h.wantCaret(6)
	h.typed("b")
	h.wantSrc("> a\n> b")
	h.key(ui.KeyEnter)
	h.key(ui.KeyEnter)
	h.typed("c")
	got := h.m.Text().String()
	if got != "> a\n> b\n\nc" {
		t.Fatalf("Enter on an empty quoted line did not leave the quote: %q", got)
	}
	if err := h.e.CheckRows(); err != nil {
		t.Fatal(err)
	}
}

// TestQuoteBarRunsOnToTheNewLine: the bar of a quote runs unbroken onto the
// empty line Enter opens, as Typora's (the line has an empty range at the
// quote's end, which the bar once took for past it).
func TestQuoteBarRunsOnToTheNewLine(t *testing.T) {
	h := openRail(t, "> 11")
	h.caret(4)
	h.key(ui.KeyEnter)
	h.wantSrc("> 11\n> ")
	h.tt.Frame()
	first, second := h.e.RowText(0), h.e.RowText(1)
	if second.Y <= first.Y {
		t.Fatalf("the new line is not below: %v %v", first, second)
	}
	img := h.tt.Image()
	x := int(first.X) - 20 // in the bar, 22 before the text and 4 wide
	bg := img.At(int(first.X)-40, int(first.Y)+5)
	for y := int(first.Y) + 4; y < int(second.Y)+12; y++ {
		if sameColor(img.At(x, y), bg) {
			t.Fatalf("the bar breaks at y %d (lines at %v and %v)", y, first.Y, second.Y)
		}
	}
}

func sameColor(a, b color.Color) bool {
	r1, g1, b1, _ := a.RGBA()
	r2, g2, b2, _ := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2
}
