package qa

import (
	"testing"

	"aurink/internal/contract"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// The selection's format bar (02 §5.11, 08 batch 7).

func (a *app) selectRange(from, to int) {
	a.s.ActiveSession().Model().SetSelection(contract.Selection{Ranges: []contract.SelRange{{Anchor: from, Head: to}}})
	a.animate()
}

// choose opens the menu of the button labeled button and chooses item.
func (a *app) choose(button, item string) {
	a.t.Helper()
	a.click(button)
	if err := a.tt.ChooseMenuItem(item); err != nil {
		a.t.Fatalf("%v: %v", err, a.tt.Menu())
	}
	a.settle()
}

func (a *app) formatBar() bool { return a.shows(locales.LiveeditFormatBar()) }

func TestFormatBarFormatsTheSelection(t *testing.T) {
	a := newApp(t)
	a.open("f.md", "hello world\n")
	if a.formatBar() {
		t.Fatal("the format bar shows without a selection")
	}
	a.selectRange(6, 11)
	if !a.formatBar() {
		t.Fatalf("no format bar over a selection: %v", a.tt.Texts())
	}
	a.click(locales.LiveeditBold())
	a.want(a.text(), "hello **world**\n")
	// The selection is still the word, and Bold is lit.
	if !a.formatBar() || a.s.ActiveLive().Formatting().Strong != contract.On {
		t.Fatal("the bar did not stay over the bolded word")
	}
	a.click(locales.LiveeditBold())
	a.want(a.text(), "hello world\n")
	// A press on the bar between its buttons leaves the focus in the document.
	r, _ := a.tt.Find(locales.LiveeditFormatBar())
	a.tt.ClickAt(r.X+2, r.Y+r.H/2)
	a.settle()
	if !a.formatBar() || !a.s.ActiveLive().Focused() {
		t.Fatal("a press on the bar took the focus")
	}
	if qaExts&contract.ExtHighlight != 0 {
		a.click(locales.LiveeditHighlight())
		a.want(a.text(), "hello ==world==\n")
		a.click(locales.LiveeditHighlight())
	} else if a.shows(locales.LiveeditHighlight()) {
		t.Fatal("Highlight shows with its syntax off")
	}
	a.click(locales.LiveeditInlineCode())
	a.want(a.text(), "hello `world`\n")
	if !a.s.ActiveLive().Focused() {
		t.Fatal("the editor lost the focus to the bar")
	}
}

func TestFormatBarTransform(t *testing.T) {
	a := newApp(t)
	a.open("t.md", "hello world\n")
	a.selectRange(0, 5)
	if !a.shows(locales.LiveeditBody()) {
		t.Fatal("Transform does not name the paragraph")
	}
	a.choose(locales.LiveeditTransform(), locales.LiveeditHeadingN("2"))
	a.want(a.text(), "## hello world\n")
	if !a.shows(locales.LiveeditHeadingN("2")) {
		t.Fatal("Transform does not name the heading")
	}
	a.choose(locales.LiveeditTransform(), locales.LiveeditQuote())
	if got := a.text(); got != "> ## hello world\n" && got != "> hello world\n" {
		t.Fatalf("quote made %q", got)
	}
}

func TestFormatBarLinkField(t *testing.T) {
	a := newApp(t)
	a.open("l.md", "see the docs\n")
	a.selectRange(8, 12)
	a.click(locales.LiveeditLink())
	if !a.shows(locales.LiveeditLinkAddress()) {
		t.Fatal("Link did not turn the bar into an address field")
	}
	a.typed("https://example.com")
	a.key(0, ui.KeyEnter)
	a.settle()
	a.want(a.text(), "see the [docs](https://example.com)\n")
	if a.shows(locales.LiveeditLinkAddress()) || !a.s.ActiveLive().Focused() {
		t.Fatal("Enter did not close the field and go back to the document")
	}
	// Escape closes the field and changes nothing.
	a.selectRange(0, 3)
	a.click(locales.LiveeditLink())
	a.typed("x")
	a.key(0, ui.KeyEscape)
	a.settle()
	a.want(a.text(), "see the [docs](https://example.com)\n")
	if a.shows(locales.LiveeditLinkAddress()) {
		t.Fatal("Escape did not close the field")
	}
	// Moving the selection, or the focus going elsewhere, closes it too.
	a.click(locales.LiveeditLink())
	a.selectRange(0, 2)
	if a.shows(locales.LiveeditLinkAddress()) {
		t.Fatal("the field stayed open over another selection")
	}
	a.click(locales.LiveeditLink())
	a.click(locales.ShellToggleSidebar())
	if a.shows(locales.LiveeditLinkAddress()) {
		t.Fatal("the field stayed open without the focus")
	}
}

// No bar while composing, in the source view, in reading mode, or while the
// pointer still drags out the selection.
func TestFormatBarStaysAway(t *testing.T) {
	a := newApp(t)
	a.open("n.md", "hello world\n")
	a.selectRange(0, 5)
	a.tt.Compose("ni", 2)
	a.tt.Frame()
	if a.formatBar() {
		t.Fatal("the bar shows while composing")
	}
	a.tt.Compose("", 0)
	a.selectRange(0, 5)
	a.run(contract.CmdViewSource)
	a.selectRange(0, 5)
	if a.formatBar() {
		t.Fatal("the bar shows in the source view")
	}
	a.run(contract.CmdViewSource)
	// In reading mode text can be selected, for copying, but not formatted.
	a.run(contract.CmdViewRead)
	a.caret(0)
	x, y := a.s.ActiveLive().RowText(0).X, a.s.ActiveLive().RowText(0).Y
	a.tt.Press(x+1, y+8)
	a.tt.Move(x+60, y+8)
	a.tt.Release(x+60, y+8)
	a.animate()
	if sel := a.s.ActiveSession().Model().Selection().Primary(); sel.Anchor == sel.Head {
		t.Fatalf("no selection in reading mode: %v", sel)
	}
	if a.formatBar() {
		t.Fatal("the bar shows in reading mode")
	}
	a.run(contract.CmdViewRead)

	x, y = a.s.ActiveLive().RowText(0).X, a.s.ActiveLive().RowText(0).Y
	a.caret(0)
	a.animate()
	a.tt.Press(x+1, y+8)
	a.tt.Move(x+60, y+8)
	a.tt.Frame()
	if a.formatBar() {
		t.Fatal("the bar shows while dragging")
	}
	a.tt.Release(x+60, y+8)
	a.animate()
	if !a.formatBar() {
		t.Fatalf("no bar after the drag: %v focused %v", a.s.ActiveSession().Model().Selection(), a.s.ActiveLive().Focused())
	}
}
