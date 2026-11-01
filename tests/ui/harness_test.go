// Package ui_test drives the real editor over the real core in a headless window.
package ui_test

import (
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/document"
	"aurink/internal/core/editing"
	"aurink/internal/core/projection"
	"aurink/internal/views/liveedit"
	"github.com/egoist/mygo/ui"
)

type harness struct {
	t  *testing.T
	m  contract.DocumentModel
	e  *liveedit.Editor
	tt *ui.Tester
}

func open(t *testing.T, src string) *harness {
	t.Helper()
	return openSized(t, src, 1000, 700)
}

func openSized(t *testing.T, src string, w, h int) *harness {
	t.Helper()
	m := document.Factory{}.New("doc", src, contract.DocMeta{Dominant: "\n", Encoding: "utf-8"}, 0)
	e := liveedit.New(liveedit.Options{Model: m, Projector: projection.New(), Editing: editing.New()})
	t.Cleanup(e.Close)
	e.Focus()
	tt := ui.NewTester(func(c *ui.Context) { e.View(c) }, w, h)
	tt.Frame()
	return &harness{t, m, e, tt}
}

func (h *harness) caret(off int) {
	h.t.Helper()
	h.m.SetSelection(contract.Caret(off))
	h.tt.Frame()
}

func (h *harness) sel(anchor, head int) {
	h.t.Helper()
	h.m.SetSelection(contract.Selection{Ranges: []contract.SelRange{{Anchor: anchor, Head: head}}})
	h.tt.Frame()
}

func (h *harness) wantSrc(want string) {
	h.t.Helper()
	if got := h.m.Text().String(); got != want {
		h.t.Fatalf("source\n got %q\nwant %q", got, want)
	}
	if err := h.e.CheckRows(); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) wantCaret(want int) {
	h.t.Helper()
	if s := h.m.Selection().Primary(); s.Anchor != want || s.Head != want {
		h.t.Fatalf("selection %d..%d, want caret %d, in %q", s.Anchor, s.Head, want, h.m.Text().String())
	}
}

func (h *harness) wantSel(anchor, head int) {
	h.t.Helper()
	if s := h.m.Selection().Primary(); s.Anchor != anchor || s.Head != head {
		h.t.Fatalf("selection %d..%d, want %d..%d, in %q", s.Anchor, s.Head, anchor, head, h.m.Text().String())
	}
}

func (h *harness) key(k ui.Key) {
	h.t.Helper()
	h.tt.Key(0, k)
	h.tt.Frame()
}

func (h *harness) keys(k ui.Key, n int) {
	h.t.Helper()
	for range n {
		h.key(k)
	}
}

func (h *harness) mod(m ui.Modifiers, k ui.Key) {
	h.t.Helper()
	h.tt.Key(m, k)
	h.tt.Frame()
}

func (h *harness) typed(s string) {
	h.t.Helper()
	h.tt.Type(s)
	h.tt.Frame()
}
