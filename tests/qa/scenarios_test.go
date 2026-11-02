// Package qa drives the whole interface over the real core (QA-1.2). No scenario
// waits on time: fake timers fire by hand and background work is pumped until done.
package qa

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/core/commands"
	"aurink/internal/core/docsession"
	"aurink/internal/core/document"
	"aurink/internal/core/editing"
	"aurink/internal/core/fsstore"
	"aurink/internal/core/projection"
	"aurink/internal/core/settings"
	"aurink/internal/views/editorapi"
	"aurink/internal/views/shell"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type pump chan func()

func (p pump) Post(f func()) { p <- f }

type timer struct {
	at      time.Duration
	fn      func()
	stopped bool
}

func (t *timer) Stop() bool {
	was := !t.stopped
	t.stopped = true
	return was
}

type platform struct {
	open, save string
	choice     shell.Choice
	asked      []string
	saves      int
	mins, maxs int
	closes     int
}

func (p *platform) OpenFile(done func(string)) { done(p.open) }
func (p *platform) SavePath(_ string, done func(string)) {
	p.saves++
	done(p.save)
}
func (p *platform) ConfirmClose(name string, done func(shell.Choice)) {
	p.asked = append(p.asked, name)
	done(p.choice)
}
func (p *platform) Minimize()         { p.mins++ }
func (p *platform) ToggleMaximize()   { p.maxs++ }
func (p *platform) IsMaximized() bool { return false }
func (p *platform) Close()            { p.closes++ }

type app struct {
	t        *testing.T
	dir      string
	data     string
	q        pump
	now      time.Duration
	timers   []*timer
	p        *platform
	store    *settings.Store
	sessions *docsession.Manager
	reg      *commands.Registry
	s        *shell.Shell
	tt       *ui.Tester
}

func newApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	a := &app{t: t, dir: dir, data: filepath.Join(dir, "data"), q: make(pump, 4096), p: &platform{}}
	if err := os.MkdirAll(a.data, 0o755); err != nil {
		t.Fatal(err)
	}
	a.boot()
	return a
}

// boot starts the app over the data folder, as a new process does.
func (a *app) boot() {
	t := a.t
	files := fsstore.New()
	store, err := settings.New(filepath.Join(a.data, "settings.json"), files, a.q, nil)
	if err != nil {
		t.Fatal(err)
	}
	a.store = store
	base := store.Get()
	base.Editor.Exts = contract.ExtFrontMatter
	base.Files.AutosaveDelayMs = 1000
	a.timers = nil
	a.sessions = docsession.New(files, document.Factory{}, a.q, filepath.Join(a.data, "recovery"), docsession.Options{Settings: &base,
		AfterFunc: func(d time.Duration, fn func()) docsession.Timer {
			tm := &timer{at: a.now + d, fn: fn}
			a.timers = append(a.timers, tm)
			return tm
		}})
	a.reg = commands.New()
	a.reg.SetDefaults(commands.DefaultBindings())
	a.s = shell.New(shell.Options{Sessions: a.sessions, Settings: store, Projector: projection.New(), Editing: editing.New(),
		Commands: a.reg, Dispatcher: a.q, Platform: a.p, WindowButtons: true, Exts: contract.ExtFrontMatter})
	if err := a.reg.Register(a.s.Commands()...); err != nil {
		t.Fatal(err)
	}
	a.tt = ui.NewTester(a.s.View, 1280, 800)
	a.tt.Frame()
}

// settle runs what is posted until the sessions are idle: Restore is queued behind
// everything before it, so its answer means they are done.
func (a *app) settle() {
	a.t.Helper()
	// Twice: what the first round runs can queue more work.
	for range 2 {
		for len(a.q) > 0 {
			(<-a.q)()
		}
		done := false
		a.sessions.Restore(func([]contract.Session, error) { done = true })
		for !done {
			select {
			case f := <-a.q:
				f()
			case <-time.After(30 * time.Second):
				a.t.Fatal("the sessions never settled")
			}
		}
	}
	a.tt.Frame()
}

// advance moves the fake clock and fires what came due.
func (a *app) advance(d time.Duration) {
	a.now += d
	for _, tm := range a.timers {
		if !tm.stopped && tm.at <= a.now {
			tm.stopped = true
			tm.fn()
		}
	}
	a.settle()
}

func (a *app) file(name, text string) string {
	a.t.Helper()
	p := filepath.Join(a.dir, name)
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		a.t.Fatal(err)
	}
	return p
}

func (a *app) disk(p string) string {
	a.t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		a.t.Fatal(err)
	}
	return string(data)
}

func (a *app) open(name, text string) string {
	a.t.Helper()
	p := a.file(name, text)
	a.s.Open(p)
	a.settle()
	return p
}

func (a *app) run(id contract.CommandID) {
	a.t.Helper()
	if err := a.reg.Run(id); err != nil {
		a.t.Fatal(err)
	}
	a.settle()
}

func (a *app) click(label string) {
	a.t.Helper()
	if err := a.tt.Click(label); err != nil {
		a.t.Fatal(err)
	}
	a.settle()
}

func (a *app) typed(s string) {
	a.t.Helper()
	a.tt.Type(s)
	a.tt.Frame()
}

func (a *app) text() string { return a.s.ActiveSession().Model().Text().String() }

func (a *app) want(got, want string) {
	a.t.Helper()
	if got != want {
		a.t.Fatalf("\n got %q\nwant %q", got, want)
	}
}

func (a *app) tabs(want int) {
	a.t.Helper()
	if got := len(a.s.Sessions()); got != want {
		a.t.Fatalf("%d tabs, want %d", got, want)
	}
}

func (a *app) notices(want int) {
	a.t.Helper()
	if got := a.s.Notices(); len(got) != want {
		a.t.Fatalf("notices %q, want %d", got, want)
	}
}

// caret puts the active document's caret at an offset.
func (a *app) caret(off int) {
	a.s.ActiveSession().Model().SetSelection(contract.Caret(off))
	a.tt.Frame()
}

type scenario struct {
	name string
	run  func(a *app)
}

var scenarios = []scenario{
	{"the first window welcomes", func(a *app) {
		a.tabs(0)
		if _, ok := a.tt.Find(locales.ShellOpen()); !ok {
			a.t.Fatal("no open button")
		}
	}},
	{"the welcome page's new button makes a tab", func(a *app) { a.click(locales.ShellNew()); a.tabs(1) }},
	{"the welcome page's open button opens", func(a *app) {
		a.p.open = a.file("a.md", "x\n")
		a.click(locales.ShellOpen())
		a.tabs(1)
		a.want(a.text(), "x\n")
	}},
	{"primary+N makes a tab", func(a *app) {
		a.tt.Key(ui.Cmd, ui.KeyN)
		a.settle()
		a.tabs(1)
	}},
	{"primary+O opens", func(a *app) {
		a.p.open = a.file("a.md", "x\n")
		a.tt.Key(ui.Cmd, ui.KeyO)
		a.settle()
		a.tabs(1)
	}},
	{"a canceled dialog opens nothing", func(a *app) { a.run(contract.CmdFileOpen); a.tabs(0); a.notices(0) }},
	{"opening an open file shows its tab", func(a *app) {
		a.open("a.md", "a\n")
		a.open("b.md", "b\n")
		a.s.Open(filepath.Join(a.dir, "a.md"))
		a.settle()
		a.tabs(2)
		a.want(a.text(), "a\n")
	}},
	{"a missing file is a notice", func(a *app) {
		a.s.Open(filepath.Join(a.dir, "gone.md"))
		a.settle()
		a.tabs(0)
		a.notices(1)
	}},
	{"a file over 5 MB is refused with a notice", func(a *app) {
		p := a.file("big.md", strings.Repeat("x", 5<<20+1))
		a.s.Open(p)
		a.settle()
		a.tabs(0)
		if got := a.s.Notices(); len(got) != 1 || got[0] != locales.ShellTooLarge() {
			a.t.Fatalf("notices %q", got)
		}
	}},
	{"typing and saving writes the file", func(a *app) {
		p := a.open("a.md", "x\n")
		a.caret(1)
		a.typed("y")
		a.tt.Key(ui.Cmd, ui.KeyS)
		a.settle()
		a.want(a.disk(p), "xy\n")
		if a.s.ActiveSession().State().Edited {
			a.t.Fatal("still edited after saving")
		}
	}},
	{"saving keeps every byte not edited", func(a *app) {
		src := "a\r\nb\n\tc  \r\n\r\n"
		p := a.open("a.md", src)
		a.caret(1)
		a.typed("!")
		a.run(contract.CmdFileSave)
		a.want(a.disk(p), "a!\r\nb\n\tc  \r\n\r\n")
	}},
	{"autosave writes after the delay", func(a *app) {
		p := a.open("a.md", "x\n")
		a.caret(1)
		a.typed("y")
		a.advance(500 * time.Millisecond)
		a.want(a.disk(p), "x\n")
		a.advance(600 * time.Millisecond)
		a.want(a.disk(p), "xy\n")
	}},
	{"saving an untitled document asks where and adds .md", func(a *app) {
		a.run(contract.CmdFileNew)
		a.typed("hello")
		a.p.save = filepath.Join(a.dir, "new")
		a.run(contract.CmdFileSave)
		a.want(a.disk(a.p.save+".md"), "hello")
		a.want(a.s.ActiveSession().State().Name, "new.md")
	}},
	{"canceling save as changes nothing", func(a *app) {
		a.run(contract.CmdFileNew)
		a.typed("hello")
		a.run(contract.CmdFileSaveAs)
		a.notices(0)
		if !a.s.ActiveSession().State().Edited {
			a.t.Fatal("it says saved")
		}
	}},
	{"save as moves the document", func(a *app) {
		p := a.open("a.md", "x\n")
		a.p.save = filepath.Join(a.dir, "b.md")
		a.run(contract.CmdFileSaveAs)
		a.want(a.disk(a.p.save), "x\n")
		a.want(a.disk(p), "x\n")
		a.want(a.s.ActiveSession().State().Name, "b.md")
	}},
	{"a change on disk stops the save and says so", func(a *app) {
		p := a.open("a.md", "x\n")
		a.caret(1)
		a.typed("y")
		a.want(a.disk(a.file("a.md", "theirs\n")), "theirs\n")
		a.run(contract.CmdFileSave)
		a.want(a.disk(p), "theirs\n")
		if !a.s.ActiveSession().State().Conflict {
			a.t.Fatal("no conflict")
		}
		a.want(a.s.Status(), locales.ShellConflict())
	}},
	{"in conflict autosave never writes", func(a *app) {
		p := a.open("a.md", "x\n")
		a.caret(1)
		a.typed("y")
		a.file("a.md", "theirs\n")
		a.run(contract.CmdFileSave)
		a.typed("z")
		a.advance(5 * time.Second)
		a.want(a.disk(p), "theirs\n")
	}},
	{"closing a clean tab asks nothing", func(a *app) {
		a.open("a.md", "x\n")
		a.tt.Key(ui.Cmd, ui.KeyW)
		a.settle()
		a.tabs(0)
		if len(a.p.asked) != 0 {
			a.t.Fatal("it asked")
		}
	}},
	{"canceling the close prompt keeps the tab", func(a *app) {
		a.open("a.md", "x\n")
		a.caret(1)
		a.typed("y")
		a.run(contract.CmdFileClose)
		a.tabs(1)
		a.want(strings.Join(a.p.asked, ","), "a.md")
		a.want(a.text(), "xy\n")
	}},
	{"don't save discards", func(a *app) {
		p := a.open("a.md", "x\n")
		a.caret(1)
		a.typed("y")
		a.p.choice = shell.ChoiceDiscard
		a.run(contract.CmdFileClose)
		a.tabs(0)
		a.want(a.disk(p), "x\n")
	}},
	{"save on close writes and closes", func(a *app) {
		p := a.open("a.md", "x\n")
		a.caret(1)
		a.typed("y")
		a.p.choice = shell.ChoiceSave
		a.run(contract.CmdFileClose)
		a.tabs(0)
		a.want(a.disk(p), "xy\n")
	}},
	{"a failed save on close keeps the tab", func(a *app) {
		a.run(contract.CmdFileNew)
		a.typed("y")
		a.p.choice = shell.ChoiceSave
		a.run(contract.CmdFileClose)
		a.tabs(1)
	}},
	{"tabs keep their own caret and text", func(a *app) {
		a.open("a.md", "aaa\n")
		a.caret(2)
		a.open("b.md", "bbb\n")
		a.caret(1)
		a.click("a.md")
		a.typed("!")
		a.want(a.text(), "aa!a\n")
		a.click("b.md")
		a.want(a.text(), "bbb\n")
	}},
	{"closing the active tab shows its neighbor", func(a *app) {
		a.open("a.md", "a\n")
		a.open("b.md", "b\n")
		a.run(contract.CmdFileClose)
		a.want(a.text(), "a\n")
	}},
	{"the sidebar toggles from its key and lists open documents", func(a *app) {
		a.open("a.md", "a\n")
		a.open("b.md", "b\n")
		a.tt.Key(ui.Cmd|ui.Shift, ui.KeyL)
		a.settle()
		a.tt.Advance(time.Second)
		if !a.s.SidebarOpen() {
			a.t.Fatal("closed")
		}
		if _, ok := a.tt.Find(locales.ShellOpenDocuments()); !ok {
			a.t.Fatal("no list")
		}
		a.run(contract.CmdViewToggleSidebar)
		a.tt.Advance(time.Second)
		if _, ok := a.tt.Find(locales.ShellOpenDocuments()); ok {
			a.t.Fatal("still shown")
		}
	}},
	{"switching to source and back changes no byte", func(a *app) {
		src := "# a\r\n\r\nb\nc\r\n"
		p := a.open("a.md", src)
		for range 6 {
			a.run(contract.CmdViewSource)
		}
		a.want(a.text(), src)
		if a.s.ActiveSession().State().Edited {
			a.t.Fatal("switching edited")
		}
		a.want(a.disk(p), src)
	}},
	{"a source edit saves", func(a *app) {
		p := a.open("a.md", "x\r\ny\n")
		a.run(contract.CmdViewSource)
		a.s.ActiveSource().State().Buffer.Edit(editor.Edit{Start: 2, End: 2, Text: "z"})
		a.run(contract.CmdFileSave)
		a.want(a.disk(p), "x\r\nzy\n")
	}},
	{"a source edit undoes after going back", func(a *app) {
		a.open("a.md", "x\ny\n")
		a.run(contract.CmdViewSource)
		a.s.ActiveSource().State().Buffer.Edit(editor.Edit{Start: 2, End: 2, Text: "z"})
		a.tt.Frame()
		a.run(contract.CmdViewLive)
		a.want(a.text(), "x\nzy\n")
		a.tt.Key(ui.Cmd, ui.KeyZ)
		a.settle()
		a.want(a.text(), "x\ny\n")
	}},
	{"the caret crosses the views", func(a *app) {
		a.open("a.md", "abc\r\ndef\n")
		a.caret(6)
		a.run(contract.CmdViewSource)
		if got := a.s.ActiveSource().State().Cursor(); got != 5 {
			a.t.Fatalf("mirror caret %d", got)
		}
		a.run(contract.CmdViewLive)
		if got := a.s.ActiveSession().Model().Selection().Primary().Head; got != 6 {
			a.t.Fatalf("document caret %d", got)
		}
	}},
	{"the view menu is drawn in the window and switches the view", func(a *app) {
		a.open("a.md", "a\n")
		a.click(locales.ShellViewLive())
		if len(a.tt.Menu()) != 0 {
			a.t.Fatal("the view menu went to the native menu, which clips at 150% scale")
		}
		a.click(locales.ShellViewSource())
		if a.s.ActiveMode() != editorapi.ModeSource {
			a.t.Fatal("the menu did not switch to source")
		}
	}},
	{"the view menu follows the app's dark theme on a light system", func(a *app) {
		a.open("a.md", "a\n")
		a.run(contract.CmdViewThemeDark)
		a.tt.Advance(2 * time.Second)
		a.tt.Frame()
		a.click(locales.ShellViewLive())
		a.tt.Advance(500 * time.Millisecond)
		a.tt.Frame()
		r, ok := a.tt.Find(locales.ShellViewSource())
		if !ok {
			a.t.Fatal("the menu is not open")
		}
		red, _, _, _ := a.tt.Image().At(int(r.X-4), int(r.Y+r.H/2)).RGBA()
		if red>>8 > 80 {
			a.t.Fatalf("the menu's face has red %d in a dark window", red>>8)
		}
	}},
	{"views are per tab", func(a *app) {
		a.open("a.md", "a\n")
		a.run(contract.CmdViewSource)
		a.open("b.md", "b\n")
		if a.s.ActiveMode() != editorapi.ModeLive {
			a.t.Fatal("the new tab is not live")
		}
		a.click("a.md")
		if a.s.ActiveMode() != editorapi.ModeSource {
			a.t.Fatal("the first tab lost its view")
		}
	}},
	{"the theme toggle persists and settles", func(a *app) {
		a.open("a.md", "a\n")
		before := a.s.Dark()
		a.click(locales.ShellToggleTheme())
		a.tt.Advance(time.Second)
		if a.s.Dark() == before {
			a.t.Fatal("no change")
		}
		want := "light"
		if a.s.Dark() {
			want = "dark"
		}
		a.want(a.store.Get().Appearance.Theme, want)
		if a.tt.FramePending() {
			a.t.Fatal("frames after the dissolve")
		}
	}},
	{"the theme dissolves over a source view", func(a *app) {
		a.open("a.md", "a\n")
		a.run(contract.CmdViewSource)
		a.run(contract.CmdViewThemeDark)
		a.tt.Advance(100 * time.Millisecond)
		a.tt.Advance(time.Second)
		a.typed("q")
		a.s.ActiveSource().Sync()
		a.want(a.text(), "qa\n")
	}},
	{"reduced transparency takes the plain path", func(a *app) {
		a.open("a.md", "a\n")
		if err := a.store.Update(func(v *contract.Settings) { v.Appearance.ReduceTransparency = true }); err != nil {
			a.t.Fatal(err)
		}
		a.run(contract.CmdViewThemeDark)
		a.tt.Advance(100 * time.Millisecond)
		a.tt.Advance(time.Second)
		if !a.s.Dark() {
			a.t.Fatal("not dark")
		}
	}},
	{"at most four notices", func(a *app) {
		for range 6 {
			a.s.Notify(contract.Notice{Message: "n"})
		}
		a.tt.Frame()
		a.notices(4)
	}},
	{"a notice leaves by itself, a sticky one stays", func(a *app) {
		a.s.Notify(contract.Notice{Message: "gone"})
		a.s.Notify(contract.Notice{Message: "kept", Sticky: true})
		a.tt.Frame()
		a.tt.Advance(6 * time.Second)
		a.tt.Frame()
		a.want(strings.Join(a.s.Notices(), ","), "kept")
	}},
	{"a notice can be dismissed and updated", func(a *app) {
		h := a.s.Notify(contract.Notice{Message: "a", Sticky: true})
		h.Update(contract.Notice{Message: "b", Sticky: true})
		a.want(strings.Join(a.s.Notices(), ","), "b")
		h.Dismiss()
		a.notices(0)
	}},
	{"after a crash the edit comes back, unwritten", func(a *app) {
		p := a.open("a.md", "x\n")
		a.caret(1)
		a.typed("y")
		a.advance(600 * time.Millisecond)
		a.boot() // No close, as a killed process does not.
		a.settle()
		a.tabs(1)
		a.want(a.text(), "xy\n")
		a.want(a.disk(p), "x\n")
		if !a.s.ActiveSession().State().Edited {
			a.t.Fatal("restored as saved")
		}
		a.advance(10 * time.Second)
		a.want(a.disk(p), "x\n")
	}},
	{"a clean exit leaves nothing to recover", func(a *app) {
		a.open("a.md", "x\n")
		a.caret(1)
		a.typed("y")
		a.advance(600 * time.Millisecond)
		a.p.choice = shell.ChoiceDiscard
		closed := false
		a.s.CloseAll(func(ok bool) { closed = ok })
		a.settle()
		if !closed {
			a.t.Fatal("not closed")
		}
		a.boot()
		a.settle()
		a.tabs(0)
	}},
	{"canceling on exit keeps every tab", func(a *app) {
		a.open("a.md", "x\n")
		a.caret(1)
		a.typed("y")
		answered, closed := false, true
		a.s.CloseAll(func(ok bool) { answered, closed = true, ok })
		a.settle()
		if !answered || closed {
			a.t.Fatalf("answered %v, closed %v", answered, closed)
		}
		a.tabs(1)
	}},
	{"a checkbox saves one byte different", func(a *app) {
		src := "- [ ] task\r\n"
		p := a.open("a.md", src)
		a.caret(len(src))
		at := a.s.ActiveLive().RowText(0)
		a.tt.ClickAt(at.X-17+a.s.SheetLeft(), at.Y+9+float32(46))
		a.tt.Frame()
		a.run(contract.CmdFileSave)
		a.want(a.disk(p), "- [x] task\r\n")
	}},
	{"window buttons reach the window", func(a *app) {
		a.click(locales.ShellMinimize())
		a.click(locales.ShellMaximize())
		if a.p.mins != 1 || a.p.maxs != 1 {
			a.t.Fatalf("%d minimized, %d maximized", a.p.mins, a.p.maxs)
		}
	}},
	{"every title bar button acts", func(a *app) {
		a.click(locales.ShellToggleSidebar())
		if !a.s.SidebarOpen() {
			a.t.Fatal("sidebar")
		}
		a.p.open = a.file("a.md", "x")
		if err := a.tt.Click(locales.ShellOpen()); err != nil {
			a.t.Fatal(err)
		}
		a.settle()
		a.tabs(1)
	}},
	{"an idle window asks for no frames", func(a *app) {
		a.open("a.md", "# a\n\nb\n")
		a.tt.Advance(time.Minute)
		if a.tt.FramePending() {
			a.t.Fatal("frames while idle")
		}
	}},
	{"words count CJK by character", func(a *app) {
		for text, want := range map[string]int{"": 0, "two words": 2, "中文 ok": 3, "don't-stop 1": 2, "# 标题\n": 2} {
			if got := shell.CountWords(text); got != want {
				a.t.Fatalf("%q: %d, want %d", text, got, want)
			}
		}
	}},
	{"the status names the state", func(a *app) {
		a.open("a.md", "x")
		a.want(a.s.Status(), locales.ShellSaved())
		a.caret(1)
		a.typed("y")
		a.want(a.s.Status(), locales.ShellUnsaved())
	}},

	// §7's risks, one regression each.
	{"risk: a snapshot stays as it was while the document changes", func(a *app) {
		a.open("a.md", "# one\n\ntwo\n")
		m := a.s.ActiveSession().Model()
		text, ast := m.Text(), m.AST()
		leaves := ast.LeafCount()
		a.caret(7)
		a.typed("\n\nthree")
		a.want(text.String(), "# one\n\ntwo\n")
		if ast.LeafCount() != leaves || m.AST().LeafCount() == leaves {
			a.t.Fatal("the old tree changed or the new one did not")
		}
	}},
	{"risk: the edited tree equals a fresh parse", func(a *app) {
		a.open("a.md", "# a\n\n- b\n- c\n\n> d\n\n```\ne\n```\n")
		for _, off := range []int{3, 9, 14, 20, 26} {
			a.caret(off)
			a.typed("x\n")
			m := a.s.ActiveSession().Model()
			fresh := document.Factory{}.New("fresh", m.Text().String(), m.Meta(), contract.ExtFrontMatter).AST()
			if err := sameTree(m.AST(), fresh); err != "" {
				a.t.Fatalf("after typing at %d: %s", off, err)
			}
		}
	}},
	{"risk: the caret walks through revealed markers without a stop lost", func(a *app) {
		src := "a **b** c\n"
		a.open("a.md", src)
		a.caret(len(src) - 1)
		seen := map[int]bool{}
		for range len(src) + 2 {
			seen[a.s.ActiveSession().Model().Selection().Primary().Head] = true
			a.tt.Key(0, ui.KeyLeft)
			a.tt.Frame()
		}
		if !seen[0] || len(seen) < 6 {
			a.t.Fatalf("stops %v", seen)
		}
	}},
	{"risk: mixed line endings survive source view edits", func(a *app) {
		p := a.open("a.md", "1\r\n2\n3\r\n4\n")
		a.run(contract.CmdViewSource)
		b := a.s.ActiveSource().State().Buffer
		b.Edit(editor.Edit{Start: 0, End: 0, Text: "A"}, editor.Edit{Start: b.Len() - 2, End: b.Len() - 2, Text: "B"})
		a.run(contract.CmdFileSave)
		a.want(a.disk(p), "A1\r\n2\n3\r\nB4\n")
	}},
	{"risk: a theme change settles without a GPU", func(a *app) {
		a.open("a.md", strings.Repeat("para\n\n", 50))
		a.run(contract.CmdViewThemeDark)
		a.run(contract.CmdViewThemeLight)
		a.tt.Advance(time.Second)
		if a.tt.FramePending() {
			a.t.Fatal("frames after two dissolves")
		}
	}},
}

func sameTree(a, b contract.AST) string {
	if a.Len() != b.Len() {
		return "different block counts"
	}
	for i := range a.Len() {
		x, y := a.Block(i), b.Block(i)
		if x.Kind != y.Kind || x.Range != y.Range || x.Parent != y.Parent {
			return "block differs"
		}
	}
	return ""
}

func TestScenarios(t *testing.T) {
	if len(scenarios) < 40 {
		t.Fatalf("%d scenarios: the card asks for 40", len(scenarios))
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			a := newApp(t)
			sc.run(a)
			a.settle()
			if err := a.s.Check(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
