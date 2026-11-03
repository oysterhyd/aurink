package qa

import (
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// The slash menu (02 §5.3, 08 batch 7).

// slashDoc opens a document whose last line is an empty paragraph, with the
// caret on it.
func (a *app) slashDoc() {
	a.t.Helper()
	a.open("s.md", "# Notes\n\ntext\n\n")
	a.caret(len(a.text()))
	a.animate()
}

func (a *app) slashOpen() bool {
	a.tt.Frame()
	return a.s.ActiveLive().SlashOpen()
}

func (a *app) slashShows(label string) bool {
	a.tt.Advance(300 * 1e6)
	a.tt.Frame()
	_, ok := a.tt.Find(label)
	return ok
}

func TestSlashOpensOnAnEmptyParagraph(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	if !a.slashOpen() || !a.slashShows(locales.LiveeditSlashBullets()) || !a.slashShows(locales.LiveeditSlashBasic()) {
		t.Fatal("no menu after / on an empty paragraph")
	}
	// The preview shows the choice's description and syntax.
	if !a.slashShows(locales.LiveeditSlashH1Desc()) {
		t.Fatal("no preview of Heading 1")
	}
}

func TestSlashStaysShutElsewhere(t *testing.T) {
	for _, c := range []struct{ name, doc string }{
		{"after text", "# Notes\n\ntext"},
		{"in a list item", "- one\n- "},
		{"in a heading", "# "},
		{"in a quote", "> "},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := newApp(t)
			a.open("s.md", c.doc)
			a.caret(len(a.text()))
			a.animate()
			a.typed("/")
			if a.slashOpen() {
				t.Fatalf("the menu opened in %q", a.text())
			}
		})
	}
}

func TestSlashFiltersAndInserts(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.typed("h2")
	if !a.slashOpen() || a.slashShows(locales.LiveeditSlashBullets()) {
		t.Fatal("the filter did not narrow the menu to Heading 2")
	}
	a.key(0, ui.KeyEnter)
	a.want(a.text(), "# Notes\n\ntext\n\n## ")
	a.wantCaret(len(a.text()))
	if a.slashOpen() {
		t.Fatal("the menu stayed")
	}
	// One undo brings back what was typed.
	a.key(ui.Cmd, ui.KeyZ)
	a.want(a.text(), "# Notes\n\ntext\n\n/h2")
}

func TestSlashFindsByEnglishWords(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.typed("todo")
	a.key(0, ui.KeyEnter)
	a.want(a.text(), "# Notes\n\ntext\n\n- [ ] ")
}

func TestSlashArrowsChoose(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.key(0, ui.KeyDown)
	a.key(0, ui.KeyDown)
	if !a.slashShows(locales.LiveeditSlashH3Desc()) {
		t.Fatal("two downs do not preview Heading 3")
	}
	a.key(0, ui.KeyUp)
	a.key(0, ui.KeyEnter)
	a.want(a.text(), "# Notes\n\ntext\n\n## ")
	// Up from the first goes round to the last: Mermaid.
	a.open("t.md", "\n")
	a.caret(1)
	a.typed("/")
	a.key(0, ui.KeyUp)
	a.key(0, ui.KeyEnter)
	if !strings.Contains(a.text(), "```mermaid") {
		t.Fatalf("up from the first did not choose Mermaid: %q", a.text())
	}
}

func TestSlashEscapeKeepsTheText(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.typed("ta")
	a.key(0, ui.KeyEscape)
	if a.slashOpen() {
		t.Fatal("Esc did not close the menu")
	}
	a.want(a.text(), "# Notes\n\ntext\n\n/ta")
	a.typed("b")
	if a.slashOpen() {
		t.Fatal("typing on brought the menu back")
	}
	a.key(0, ui.KeyEnter) // the document's Enter again
	a.want(a.text(), "# Notes\n\ntext\n\n/tab\n\n")
}

func TestSlashClosesWhenTheSlashGoes(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.key(0, ui.KeyBackspace)
	if a.slashOpen() {
		t.Fatal("the menu outlived its /")
	}
	a.want(a.text(), "# Notes\n\ntext\n\n")
}

func TestSlashNoMatch(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.typed("zzz")
	if !a.slashShows(locales.LiveeditSlashEmpty()) {
		t.Fatal("no word that nothing matches")
	}
	a.key(0, ui.KeyEnter)
	a.want(a.text(), "# Notes\n\ntext\n\n/zzz\n\n")
}

func TestSlashCodeAsksTheLanguage(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.typed("code")
	a.key(0, ui.KeyEnter)
	if !a.slashOpen() || !a.slashShows(locales.LiveeditSlashPlain()) {
		t.Fatal("no language step")
	}
	a.want(a.text(), "# Notes\n\ntext\n\n/")
	a.typed("pyt")
	a.key(0, ui.KeyEnter)
	if !strings.HasPrefix(a.text()[len("# Notes\n\ntext\n\n"):], "```python\n") {
		t.Fatalf("got %q", a.text())
	}
	// The insertion is one undo step; the step before it cleared the filter.
	a.key(ui.Cmd, ui.KeyZ)
	a.want(a.text(), "# Notes\n\ntext\n\n/pyt")
}

func TestSlashTableAsksTheSize(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.typed("table")
	a.key(0, ui.KeyEnter)
	if !a.slashShows(locales.LiveeditSlashSizeN("3", "3")) {
		t.Fatal("no size step at 3 × 3")
	}
	a.key(0, ui.KeyRight)
	a.key(0, ui.KeyDown)
	if !a.slashShows(locales.LiveeditSlashSizeN("4", "4")) {
		t.Fatal("the arrows did not grow it to 4 × 4")
	}
	a.key(0, ui.KeyEnter)
	body := a.text()[len("# Notes\n\ntext\n\n"):]
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) != 5 || strings.Count(lines[0], "|") != 5 {
		t.Fatalf("not a 4 by 4 table: %q", body)
	}
}

func TestSlashImagePutsTheCaretInTheDescription(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.typed("image")
	a.key(0, ui.KeyEnter)
	a.want(a.text(), "# Notes\n\ntext\n\n![]()")
	a.wantCaret(len("# Notes\n\ntext\n\n!["))
}

func TestSlashClickInserts(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.slashShows(locales.LiveeditQuote())
	if err := a.tt.Click(locales.LiveeditQuote()); err != nil {
		t.Fatal(err)
	}
	a.tt.Frame()
	a.want(a.text(), "# Notes\n\ntext\n\n> ")
	if !a.s.ActiveLive().Focused() {
		t.Fatal("the click took the focus from the document")
	}
}

func TestSlashWaitsForTheInputMethod(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.slashShows(locales.LiveeditSlashBullets())
	a.tt.Compose("biao", 4)
	if a.slashShows(locales.LiveeditSlashBullets()) {
		t.Fatal("the panel stayed over the composition")
	}
	a.tt.Compose("", 0)
	a.typed("标题")
	if !a.slashOpen() {
		t.Fatal("the menu closed after the composition")
	}
}

// The pointer chooses where it moves; resting, it leaves the keys' choice.
func TestSlashPointerChooses(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.slashShows(locales.LiveeditSlashTasks())
	r, ok := a.tt.Find(locales.LiveeditSlashTasks())
	if !ok {
		t.Fatal("no Task list row")
	}
	a.tt.Move(r.X+r.W/2, r.Y+r.H/2)
	a.tt.Frame()
	if !a.slashShows(locales.LiveeditSlashTasksDesc()) {
		t.Fatal("the pointer did not choose Task list")
	}
	a.key(0, ui.KeyDown)
	a.tt.Frame()
	for range 3 {
		if !a.slashShows(locales.LiveeditSlashQuoteDesc()) || a.slashShows(locales.LiveeditSlashTasksDesc()) {
			t.Fatal("the resting pointer took the choice back from the keys")
		}
	}
	a.key(0, ui.KeyDown)
	a.key(0, ui.KeyEnter) // Code block, which asks the language
	a.want(a.text(), "# Notes\n\ntext\n\n/")
	if !a.slashShows(locales.LiveeditSlashPlain()) {
		t.Fatal("Enter did not take the keys' choice, Code block")
	}
}

func TestSlashSizeGridClick(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.typed("table")
	a.key(0, ui.KeyEnter)
	a.slashShows(locales.LiveeditSlashSize())
	if err := a.tt.Click(locales.LiveeditSlashSizeN("2", "4")); err != nil {
		t.Fatal(err)
	}
	a.tt.Frame()
	body := a.text()[len("# Notes\n\ntext\n\n"):]
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	if len(lines) != 3 || strings.Count(lines[0], "|") != 5 {
		t.Fatalf("not a 2 by 4 table: %q", body)
	}
	if !a.s.ActiveLive().Focused() {
		t.Fatal("the click took the focus from the document")
	}
}

// A line break in what is typed after the "/", as from a paste, closes the
// menu.
func TestSlashClosesOnALineBreak(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.typed("h")
	a.s.ActiveLive().ReplaceText(nil, "a\nb")
	if a.slashOpen() {
		t.Fatalf("the menu stayed over %q", a.text())
	}
}

// A change before the "/" from elsewhere (a reload) moves it: the menu goes
// rather than read the wrong bytes.
func TestSlashClosesWhenTheTextMoves(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	a.typed("h")
	m := a.s.ActiveSession().Model()
	if _, err := m.Apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: 0, End: 0}, Text: "X"}}, Origin: contract.OriginReload}); err != nil {
		t.Fatal(err)
	}
	if a.slashOpen() {
		t.Fatal("the menu outlived the text it was on")
	}
}

// A command that also writes after the "/" (a footnote's definition at the
// end) lands where it should once the "/" and the filter are gone.
func TestSlashFootnoteInTheMiddle(t *testing.T) {
	a := newApp(t)
	a.open("f.md", "# Notes\n\n\n\nafter\n")
	a.caret(len("# Notes\n\n"))
	a.animate()
	a.typed("/")
	a.typed("footn")
	a.key(0, ui.KeyEnter)
	got := a.text()
	if strings.Contains(got, "/footn") || !strings.Contains(got, "\nafter\n") || !strings.Contains(got, "[^1]:") {
		t.Fatalf("got %q", got)
	}
	a.key(ui.Cmd, ui.KeyZ)
	a.want(a.text(), "# Notes\n\n/footn\n\nafter\n")
}

// The panel grows in from the caret; with motion reduced it is just there.
func TestSlashReducedMotion(t *testing.T) {
	a := newApp(t)
	a.slashDoc()
	a.typed("/")
	if !a.tt.FramePending() {
		t.Fatal("the panel did not animate in")
	}
	a.key(0, ui.KeyEscape)
	a.key(0, ui.KeyBackspace)
	if err := a.store.Update(func(v *contract.Settings) { v.Appearance.ReduceMotion = "on" }); err != nil {
		t.Fatal(err)
	}
	a.animate()
	a.typed("/")
	a.tt.Frame()
	if !a.slashOpen() || a.tt.FramePending() {
		t.Fatal("with motion reduced the panel animated in")
	}
	a.key(0, ui.KeyDown)
	a.tt.Frame()
	if a.tt.FramePending() {
		t.Fatal("with motion reduced the pill slid")
	}
}
