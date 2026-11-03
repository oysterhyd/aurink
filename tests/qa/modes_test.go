package qa

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// Reading, focus and typewriter modes, the zoom and the type from settings
// (FE-1.7 part 1, 02 §5.10, D17).

func readingDoc() string {
	var b strings.Builder
	b.WriteString("# Reading Title\n\n")
	for i := range 200 {
		fmt.Fprintf(&b, "Paragraph %d with a few words to read.\n\n", i)
	}
	return b.String()
}

// animate lets the window's animations finish.
func (a *app) animate() {
	for range 10 {
		a.tt.Advance(100 * time.Millisecond)
	}
	a.tt.Frame()
}

// darkest returns the darkest red level in the box of a text.
func (a *app) darkest(text string) int {
	a.t.Helper()
	r, ok := a.tt.Find(text)
	if !ok {
		a.t.Fatalf("no %q", text)
	}
	img := a.tt.Image()
	m := 255
	for x := int(r.X); x < int(r.X+r.W); x++ {
		for y := int(r.Y); y < int(r.Y+r.H); y++ {
			m = min(m, int(img.RGBAAt(x, y).R))
		}
	}
	return m
}

// enabled reports whether a command is on.
func (a *app) enabled(id contract.CommandID) bool {
	for _, c := range a.reg.All() {
		if c.ID == id {
			return c.Enabled == nil || c.Enabled()
		}
	}
	a.t.Fatalf("no command %s", id)
	return false
}

func TestReadingModeEntersAndLeaves(t *testing.T) {
	a := newApp(t)
	a.open("read.md", readingDoc())
	a.key(0, ui.KeyF7)
	a.animate()
	if !a.s.Reading() || a.s.ActiveLive().Mode() != editorapi.ModeRead {
		t.Fatalf("F7: reading %v, mode %v", a.s.Reading(), a.s.ActiveLive().Mode())
	}
	if a.s.ActiveSession().State().ReadOnly {
		t.Fatal("reading mode made the file read-only (D17)")
	}
	// The window's chrome is gone: the tab, the view menu, the sidebar.
	if a.shows("read.md") || a.shows(locales.ShellViewLive()) || a.shows(locales.ShellPanelFiles()) || a.s.SheetLeft() != 0 {
		t.Fatal("the title bar or the sidebar still shows in reading mode")
	}
	if !a.shows(locales.ShellExitReading()) {
		t.Fatal("no exit button")
	}
	before := a.text()
	a.typed("x")
	a.want(a.text(), before)
	// Escape leaves, to the live view, with the chrome back.
	a.key(0, ui.KeyEscape)
	a.animate()
	if a.s.Reading() || a.s.ActiveLive().Mode() != editorapi.ModeLive || a.s.ActiveMode() != editorapi.ModeLive {
		t.Fatalf("Escape: reading %v, mode %v", a.s.Reading(), a.s.ActiveLive().Mode())
	}
	if !a.shows("read.md") || a.shows(locales.ShellExitReading()) {
		t.Fatal("the title bar did not come back")
	}
	// The button leaves too.
	a.run(contract.CmdViewRead)
	a.animate()
	a.click(locales.ShellExitReading())
	if a.s.Reading() {
		t.Fatal("the exit button left reading mode on")
	}
}

func TestReadingModeShowsTheTitleAfterTheHeading(t *testing.T) {
	a := newApp(t)
	a.open("read.md", readingDoc())
	a.run(contract.CmdViewRead)
	a.animate()
	if _, past := a.s.ActiveLive().TitleAbove(); past || a.shows("Reading Title") {
		t.Fatal("the title shows before the heading has scrolled away")
	}
	if f := a.s.ActiveLive().ScrollFraction(); f != 0 {
		t.Fatalf("progress %v at the top", f)
	}
	bar := a.tt.Image().RGBAAt(4, 0)
	a.tt.Scroll(640, 400, 0, 2000)
	a.animate()
	// The progress bar has grown past the left edge.
	if a.tt.Image().RGBAAt(4, 0) == bar {
		t.Fatal("the progress bar did not grow")
	}
	if _, past := a.s.ActiveLive().TitleAbove(); !past || !a.shows("Reading Title") {
		t.Fatal("the title does not show once the heading has scrolled away")
	}
	if f := a.s.ActiveLive().ScrollFraction(); f <= 0 || f >= 1 {
		t.Fatalf("progress %v in the middle", f)
	}
}

// Reading mode remembers each tab's view: a source tab reads in the live view
// and goes back to source; Ctrl+/ in reading mode leaves it.
func TestReadingModeKeepsEachTabsView(t *testing.T) {
	a := newApp(t)
	a.open("one.md", "# One\n\ntext\n")
	a.open("two.md", "# Two\n\ntext\n")
	a.run(contract.CmdViewSource)
	a.run(contract.CmdViewRead)
	if a.s.ActiveMode() != editorapi.ModeLive || a.s.ActiveLive().Mode() != editorapi.ModeRead {
		t.Fatal("a source tab does not read in the live view")
	}
	// A tab activated in reading mode reads too.
	a.run(contract.CmdNavNextTab)
	if a.s.ActiveSession().State().Name != "one.md" || a.s.ActiveLive().Mode() != editorapi.ModeRead {
		t.Fatalf("%s: mode %v", a.s.ActiveSession().State().Name, a.s.ActiveLive().Mode())
	}
	a.run(contract.CmdViewRead)
	if a.s.ActiveLive().Mode() != editorapi.ModeLive {
		t.Fatal("one.md stayed in reading mode")
	}
	a.run(contract.CmdNavNextTab)
	if a.s.ActiveMode() != editorapi.ModeSource {
		t.Fatal("two.md did not go back to source")
	}
	// Choosing a view leaves reading mode.
	a.run(contract.CmdViewRead)
	a.run(contract.CmdViewSource)
	if a.s.Reading() {
		t.Fatal("Ctrl+/ left reading mode on")
	}
	for _, id := range []contract.CommandID{contract.CmdViewFocus, contract.CmdViewTypewriter, contract.CmdFormatStrong} {
		a.run(contract.CmdViewLive)
		a.run(contract.CmdViewRead)
		if a.enabled(id) {
			t.Fatalf("%s is on in reading mode", id)
		}
		a.run(contract.CmdViewRead)
	}
}

func TestReadingModeEndsWithTheLastTab(t *testing.T) {
	a := newApp(t)
	a.open("one.md", "text\n")
	a.run(contract.CmdViewRead)
	a.run(contract.CmdFileClose)
	a.tt.Frame()
	if a.s.Reading() {
		t.Fatal("reading mode outlived the last tab")
	}
}

func TestTypographyFollowsSettingsZoomAndReading(t *testing.T) {
	a := newApp(t)
	a.open("type.md", "# Type\n\ntext\n")
	ty := a.s.ActiveLive().Typography()
	if ty.Body != 16 || ty.LineHeight != 1.75 || ty.Column != 720 || ty.Serif {
		t.Fatalf("default type %+v", ty)
	}
	if err := a.store.Update(func(v *contract.Settings) {
		v.Appearance.FontSize, v.Appearance.LineHeight, v.Appearance.ContentWidth = 18, 1.6, 640
	}); err != nil {
		t.Fatal(err)
	}
	a.settle()
	if ty = a.s.ActiveLive().Typography(); ty.Body != 18 || ty.LineHeight != 1.6 || ty.Column != 640 {
		t.Fatalf("type from settings %+v", ty)
	}
	near := func(got, want float32) bool { return got > want-0.01 && got < want+0.01 }
	a.key(ui.Cmd|ui.Shift, ui.KeyEqual)
	if ty = a.s.ActiveLive().Typography(); !near(ty.Body, 18*1.1) || !near(ty.Column, 640*1.1) {
		t.Fatalf("zoomed in %+v", ty)
	}
	a.key(ui.Cmd|ui.Shift, ui.KeyMinus)
	a.key(ui.Cmd|ui.Shift, ui.KeyMinus)
	if ty = a.s.ActiveLive().Typography(); !near(ty.Body, 18*0.9) {
		t.Fatalf("zoomed out %+v", ty)
	}
	for range 20 {
		a.run(contract.CmdViewZoomOut)
	}
	if z := a.s.Zoom(); !near(z, 0.5) {
		t.Fatalf("zoom %v below its floor", z)
	}
	// Ctrl+Shift+0 is actual size, not the paragraph that Ctrl+0 is.
	before := a.text()
	a.key(ui.Cmd|ui.Shift, ui.Key0)
	if a.s.Zoom() != 1 || a.s.ActiveLive().Typography().Body != 18 {
		t.Fatalf("actual size: zoom %v", a.s.Zoom())
	}
	a.want(a.text(), before)
	// Reading: serif, one larger, line height at least 1.9, column at most 680.
	a.run(contract.CmdViewRead)
	if ty = a.s.ActiveLive().Typography(); !ty.Serif || ty.Body != 19 || ty.LineHeight != 1.9 || ty.Column != 640 {
		t.Fatalf("reading type %+v", ty)
	}
	if err := a.store.Update(func(v *contract.Settings) { v.Appearance.ContentWidth = 900 }); err != nil {
		t.Fatal(err)
	}
	a.settle()
	if ty = a.s.ActiveLive().Typography(); ty.Column != 680 {
		t.Fatalf("reading column %v", ty.Column)
	}
	a.run(contract.CmdViewRead)
	if ty = a.s.ActiveLive().Typography(); ty.Serif || ty.Column != 900 {
		t.Fatalf("type after reading %+v", ty)
	}
}

// Focus mode folds the sidebar away and gives it back after (02 §5.10).
func TestFocusModeFoldsTheSidebar(t *testing.T) {
	a := newApp(t)
	a.open("focus.md", "# Focus\n\ntext\n")
	if !a.s.SidebarOpen() {
		a.run(contract.CmdViewToggleSidebar)
	}
	tab := a.darkest("focus.md")
	a.key(0, ui.KeyF8)
	a.animate()
	// The title bar fades to 0.55: its darkest ink gets lighter.
	if faded := a.darkest("focus.md"); faded < tab+60 {
		t.Fatalf("the tab's ink went from %d to %d in focus mode", tab, faded)
	}
	if !a.s.ActiveLive().FocusMode() || a.s.SheetLeft() != 0 || a.shows(locales.ShellPanelFiles()) {
		t.Fatalf("F8: focus %v, sheet at %v", a.s.ActiveLive().FocusMode(), a.s.SheetLeft())
	}
	if !a.s.SidebarOpen() {
		t.Fatal("focus mode closed the sidebar for good")
	}
	a.key(0, ui.KeyF8)
	a.animate()
	if a.s.ActiveLive().FocusMode() || a.s.SheetLeft() == 0 || !a.shows(locales.ShellPanelFiles()) {
		t.Fatal("the sidebar did not come back after focus mode")
	}
	a.key(0, ui.KeyF9)
	if !a.s.ActiveLive().Typewriter() {
		t.Fatal("F9 did not turn typewriter mode on")
	}
}

// The view menu has the writing modes and the zoom, as Typora's does.
func TestViewMenuHasTheModes(t *testing.T) {
	a := newApp(t)
	a.open("menu.md", "# Menu\n\ntext\n")
	a.click(locales.ShellViewLive())
	a.click(locales.ShellZoomIn())
	if a.s.Zoom() <= 1 {
		t.Fatal("the menu did not zoom in")
	}
	a.click(locales.ShellViewLive())
	a.click(locales.ShellFocus())
	if !a.s.ActiveLive().FocusMode() {
		t.Fatal("the menu did not turn focus mode on")
	}
	a.click(locales.ShellViewLive())
	a.click(locales.ShellReading())
	if !a.s.Reading() {
		t.Fatal("the menu did not enter reading mode")
	}
}
