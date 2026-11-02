package ui_test

import (
	"strings"
	"testing"

	"aurink/internal/contract"
)

// TestFootnoteReferenceIsASuperscript: a reference with a definition shows as
// its label raised, its source hidden until the caret is in it; hovering it
// previews the note; without the definition it is source again (BE-1.6b, 02
// §5.16).
func TestFootnoteReferenceIsASuperscript(t *testing.T) {
	const src = "Text[^n] more words.\n\n[^n]: The note body.\n"
	h := open(t, src)
	h.caret(len(src) - 1)
	h.tt.Frame()
	if d := h.e.Display(0); strings.Contains(d, "[^n]") || !strings.Contains(d, "Text") {
		t.Fatalf("a defined reference shows its source: %q", d)
	}
	// Hover along the row until the reference answers.
	at := h.rowShowing(strings.TrimSpace(h.e.Display(0)))
	found := false
	for x := at.X; x < at.X+200 && !found; x += 2 {
		h.tt.Move(x, at.Y+10)
		h.tt.Frame()
		found = h.e.NoteHint() == "n"
	}
	if !found {
		t.Fatal("hovering the superscript does not preview its note")
	}
	h.tt.Move(at.X+600, at.Y+300)
	h.tt.Frame()

	// The caret in it shows the source.
	h.caret(strings.Index(src, "^n") + 1)
	h.tt.Frame()
	if d := h.e.Display(0); !strings.Contains(d, "[^n]") {
		t.Fatalf("the caret in the reference does not show it: %q", d)
	}

	// Without its definition it is ordinary source, even with the caret away.
	h.caret(0)
	def := strings.Index(src, "\n\n[^n]")
	if _, err := h.m.Apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: def, End: len(src)}, Text: "\n"}},
		Origin: contract.OriginUser}); err != nil {
		t.Fatal(err)
	}
	h.caret(len(h.m.Text().String()) - 1)
	h.tt.Frame()
	if d := h.e.Display(0); !strings.Contains(d, "[^n]") {
		t.Fatalf("a reference without a definition stays an object: %q", d)
	}
}

// TestInlineHTMLTakesEffect: the listed tags style their text and hide until
// the caret is in them; <br> breaks the line (BE-1.6b).
func TestInlineHTMLTakesEffect(t *testing.T) {
	const src = "Press <kbd>Ctrl</kbd> and <mark>this</mark>, x<sup>2</sup> <b>bold</b> <span>kept</span>.\n\nAfter\n"
	h := open(t, src)
	h.caret(len(src) - 1)
	h.tt.Frame()
	d := h.e.Display(0)
	for _, tag := range []string{"<kbd>", "</kbd>", "<mark>", "<sup>", "</b>"} {
		if strings.Contains(d, tag) {
			t.Fatalf("%s shows with the caret away: %q", tag, d)
		}
	}
	if !strings.Contains(d, "Ctrl") || !strings.Contains(d, "<span>kept</span>") {
		t.Fatalf("the text or an unlisted tag went: %q", d)
	}
	h.caret(strings.Index(src, "Ctrl") + 1)
	h.tt.Frame()
	if d := h.e.Display(0); !strings.Contains(d, "<kbd>Ctrl</kbd>") || strings.Contains(d, "<mark>") {
		t.Fatalf("the caret in <kbd> reveals %q", d)
	}

	const br = "one<br>two\n\nAfter\n"
	h = open(t, br)
	h.caret(len(br) - 1)
	h.tt.Frame()
	if d := h.e.Display(0); strings.Contains(d, "<br>") || !strings.Contains(d, "one") || !strings.Contains(d, "two") {
		t.Fatalf("<br> shows as %q", d)
	}
}
