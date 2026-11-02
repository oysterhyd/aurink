package qa

import (
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

const findDoc = "alpha beta alpha\n\nAlpha again, alpha.\n"

func (a *app) wantStatus(s string) {
	a.t.Helper()
	a.tt.Frame()
	if !a.tt.HasText(s) {
		a.t.Fatalf("the find bar does not say %q", s)
	}
}

func (a *app) wantSelected(start, end int) {
	a.t.Helper()
	if got := a.s.ActiveSession().Model().Selection().Primary(); min(got.Anchor, got.Head) != start || max(got.Anchor, got.Head) != end {
		a.t.Fatalf("selected %+v, want %d–%d", got, start, end)
	}
}

func (a *app) key(m ui.Modifiers, k ui.Key) {
	a.tt.Key(m, k)
	a.tt.Frame()
}

func (a *app) findOpen() bool {
	a.tt.Frame() // a key handled while building shows on the next frame
	_, ok := a.tt.Find(locales.KitMatchCase())
	return ok
}

// TestFindInLiveView: the query selects the match at or after the caret as it
// is typed; Enter and Shift+Enter go round; the options search again; Escape
// closes and clears (FE-2.5).
func TestFindInLiveView(t *testing.T) {
	a := newApp(t)
	a.open("f.md", findDoc)
	a.caret(3)
	a.run(contract.CmdEditFind)
	if !a.findOpen() {
		t.Fatal("no find bar")
	}
	a.typed("alpha")
	// From the caret at 3 the first match at or after it is the second.
	a.wantStatus("2 / 4")
	second := strings.Index(findDoc[1:], "alpha") + 1
	a.wantSelected(second, second+5)
	a.key(0, ui.KeyEnter)
	a.wantStatus("3 / 4")
	a.key(0, ui.KeyEnter)
	a.key(0, ui.KeyEnter)
	a.wantStatus("1 / 4") // round the end
	a.wantSelected(0, 5)
	a.key(ui.Shift, ui.KeyEnter)
	a.wantStatus("4 / 4")

	a.click(locales.KitMatchCase())
	if a.tt.HasText("/ 4") {
		t.Fatal("matching case did not search again")
	}
	a.wantStatus("/ 3")
	a.click(locales.KitMatchCase())
	a.click(locales.KitRegex())
	a.typed("(") // the query is now "alpha("
	a.wantStatus(locales.KitBadRegex())

	a.key(0, ui.KeyEscape)
	if a.findOpen() {
		t.Fatal("Escape left the bar open")
	}
	// Primary+F again seeds the query from the selection.
	a.s.ActiveSession().Model().SetSelection(contract.Selection{Ranges: []contract.SelRange{{Anchor: 6, Head: 10}}})
	a.run(contract.CmdEditFind)
	a.wantStatus("1 / 1") // "beta"
}

// TestReplaceInLiveView: Replace takes the current match and goes on; Replace
// All is one step of undo; a regular expression expands $1 (FE-2.5).
func TestReplaceInLiveView(t *testing.T) {
	a := newApp(t)
	a.open("r.md", findDoc)
	a.caret(0)
	a.run(contract.CmdEditReplace)
	a.typed("alpha")
	a.wantStatus("1 / 4")
	a.click(locales.KitReplaceWith())
	a.typed("omega")
	a.click(locales.KitReplace())
	a.want(a.text(), strings.Replace(findDoc, "alpha", "omega", 1))
	a.wantStatus("1 / 3")
	a.click(locales.KitReplaceAll())
	a.want(a.text(), "omega beta omega\n\nomega again, omega.\n")
	if !a.tt.HasText(locales.KitNoResults()) {
		t.Fatal("matches remain after Replace All")
	}
	m := a.s.ActiveSession().Model()
	m.Undo()
	a.want(a.text(), strings.Replace(findDoc, "alpha", "omega", 1))

	// $1 from a regular expression.
	a.click(locales.KitRegex())
	a.caret(0)
	a.run(contract.CmdEditFind) // focuses the query, selected
	a.typed(`(\w+)ha`)
	a.click(locales.KitReplaceWith())
	a.key(ui.Cmd, ui.KeyA)
	a.typed("<$1>")
	a.click(locales.KitReplaceAll())
	a.want(a.text(), "omega beta <alp>\n\n<Alp> again, <alp>.\n")
}

// TestReadOnlyHasNoReplace: a read-only document finds but does not replace.
func TestReadOnlyHasNoReplace(t *testing.T) {
	a := newApp(t)
	a.open("ro.md", findDoc)
	a.s.ActiveSession().Model().SetReadOnly(true)
	a.run(contract.CmdEditReplace)
	a.typed("alpha")
	a.wantStatus("/ 4")
	if _, ok := a.tt.Find(locales.KitReplaceAll()); ok {
		t.Fatal("Replace All offered in a read-only document")
	}
}

// TestFindInSourceView: the source view's bar is MyGo's finder behind the same
// look; its Replace All is one step of the document's undo (D16).
func TestFindInSourceView(t *testing.T) {
	a := newApp(t)
	a.open("s.md", findDoc)
	a.run(contract.CmdViewSource)
	a.caret(0)
	a.run(contract.CmdEditReplace)
	if !a.findOpen() {
		t.Fatal("no find bar in the source view")
	}
	a.typed("alpha")
	a.wantStatus("/ 4")
	a.click(locales.KitReplaceWith())
	a.typed("omega")
	a.click(locales.KitReplaceAll())
	a.until("the source writes back", func() bool { return !strings.Contains(a.text(), "lpha") })
	a.s.ActiveSession().Model().Undo()
	a.want(a.text(), findDoc)
}

// TestReplaceAllAtScale: 30,000 matches replace as one step without stalling.
func TestReplaceAllAtScale(t *testing.T) {
	a := newApp(t)
	// Paragraphs: one block of 30,000 lines is slow to lay out after any edit,
	// which is the large-document work of Phase 4, not the finder's.
	doc := strings.Repeat("a cat and a dog\n\n", 30000)
	a.open("big.md", doc)
	a.caret(0)
	a.run(contract.CmdEditReplace)
	a.typed("cat")
	a.until("the matches", func() bool { return a.tt.HasText("1 / 30000") })
	a.click(locales.KitReplaceWith())
	a.typed("cow")
	start := time.Now()
	a.click(locales.KitReplaceAll())
	took := time.Since(start)
	a.want(a.text(), strings.Repeat("a cow and a dog\n\n", 30000))
	if took > time.Second {
		t.Fatalf("Replace All of 30,000 took %v", took)
	}
	t.Logf("Replace All of 30,000 matches: %v", took)
	a.s.ActiveSession().Model().Undo()
	a.want(a.text(), doc)
}
