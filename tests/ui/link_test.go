package ui_test

import (
	"path/filepath"
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/document"
	"aurink/internal/core/editing"
	"aurink/internal/core/projection"
	"aurink/internal/views/liveedit"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

const linkDoc = "See [site](https://example.com \"t\") now.\n\nRead [notes](sub/a%20b.md#top) too.\n\nA [ref][x] here.\n\n[x]: https://ref.example\n"

func openLinks(t *testing.T, opened *[]string) *harness {
	t.Helper()
	m := document.Factory{}.New("doc", linkDoc, contract.DocMeta{Dominant: "\n", Encoding: "utf-8"}, 0)
	e := liveedit.New(liveedit.Options{Model: m, Projector: projection.New(), Editing: editing.New(),
		DocDir:   func() string { return filepath.FromSlash("/docs") },
		KeyLabel: func(contract.CommandID) string { return "Ctrl K" },
		OpenFile: func(p string) { *opened = append(*opened, p) }})
	t.Cleanup(e.Close)
	e.Focus()
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Box(c).Fill().Children(func() {
			e.View(c.Key("live"))
			e.LinkBar(c.Key("link"))
		})
	}, 1000, 700)
	tt.Frame()
	return &harness{t, m, e, tt}
}

// TestLinkPopoverEditsTheDestination shows the popover while the caret is in a
// link; Edit selects the destination, so typing replaces it.
func TestLinkPopoverEditsTheDestination(t *testing.T) {
	var opened []string
	h := openLinks(t, &opened)
	h.caret(1)
	if _, ok := h.tt.Find(locales.LiveeditOpenLink()); ok {
		t.Fatal("a popover outside any link")
	}
	h.caret(strings.Index(linkDoc, "site") + 2)
	if !h.tt.HasText("https://example.com") {
		t.Fatal("no popover in a link")
	}
	if err := h.tt.Click(locales.LiveeditEditLink()); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	start := strings.Index(linkDoc, "https://example.com")
	h.wantSel(start, start+len("https://example.com"))
	h.typed("https://new.example")
	h.wantSrc(strings.Replace(linkDoc, "https://example.com", "https://new.example", 1))
}

// TestPrimaryKInALinkEditsIt: the link command in a link selects its
// destination instead of unlinking.
func TestPrimaryKInALinkEditsIt(t *testing.T) {
	var opened []string
	h := openLinks(t, &opened)
	h.caret(strings.Index(linkDoc, "notes") + 1)
	if err := h.e.RunEdit(contract.EditCommand{ID: contract.CmdFormatLink}); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	start := strings.Index(linkDoc, "sub/a%20b.md#top")
	h.wantSel(start, start+len("sub/a%20b.md#top"))
	h.wantSrc(linkDoc)
}

// TestLinkPopoverOpensAFileNextToTheDocument resolves a relative link against
// the document's folder, without its fragment and with %20 as a space.
func TestLinkPopoverOpensAFileNextToTheDocument(t *testing.T) {
	var opened []string
	h := openLinks(t, &opened)
	h.caret(strings.Index(linkDoc, "notes") + 1)
	if err := h.tt.Click(locales.LiveeditOpenLink()); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	want := filepath.Join(filepath.FromSlash("/docs"), "sub", "a b.md")
	if len(opened) != 1 || opened[0] != want {
		t.Fatalf("opened %q, want %q", opened, want)
	}
}

// TestReferenceLinkHasNoEditInPlace: a reference link's destination lives in
// its definition, so the popover does not offer to edit it here.
func TestReferenceLinkHasNoEditInPlace(t *testing.T) {
	var opened []string
	h := openLinks(t, &opened)
	h.caret(strings.Index(linkDoc, "ref]") + 1)
	if _, ok := h.tt.Find(locales.LiveeditOpenLink()); !ok || !h.tt.HasText("https://ref.example") {
		t.Fatal("no popover for a reference link")
	}
	if _, ok := h.tt.Find(locales.LiveeditEditLink() + "  Ctrl K"); ok {
		t.Fatal("edit offered for a reference link")
	}
	// Primary+K there does not select anything to type over.
	if err := h.e.RunEdit(contract.EditCommand{ID: contract.CmdFormatLink}); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	if s := h.m.Selection().Primary(); s.Anchor != s.Head && h.m.Text().Slice(contract.Range{Start: min(s.Anchor, s.Head), End: max(s.Anchor, s.Head)}) == "x" {
		t.Fatal("selected the reference label as if it were a destination")
	}
}
