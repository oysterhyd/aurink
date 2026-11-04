package ui_test

import (
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
