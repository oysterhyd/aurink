// Package qa drives the whole interface over the real core (QA-1.2). No scenario
// waits on time: fake timers fire by hand and background work is pumped until done.
package qa

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	aurinkapp "aurink/internal/app"
	"aurink/internal/contract"
	"aurink/internal/contracttest"
	"aurink/internal/core/commands"
	"aurink/internal/core/docsession"
	"aurink/internal/core/document"
	"aurink/internal/core/editing"
	"aurink/internal/core/fsstore"
	"aurink/internal/core/markdown"
	"aurink/internal/core/projection"
	"aurink/internal/core/search"
	"aurink/internal/core/settings"
	"aurink/internal/core/workspace"
	"aurink/internal/views/editorapi"
	"aurink/internal/views/palette"
	"aurink/internal/views/shell"
	"aurink/internal/views/theme"
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
	export     string
	folder     string
	trashOK    bool
	choice     shell.Choice
	asked      []string
	saves      int
	mins, maxs int
	closes     int
}

func (p *platform) OpenFile(done func(string))   { done(p.open) }
func (p *platform) OpenFolder(done func(string)) { done(p.folder) }
func (p *platform) ConfirmTrash(name string, done func(bool)) {
	p.asked = append(p.asked, name)
	done(p.trashOK)
}
func (p *platform) ExportPath(_ string, done func(string)) { done(p.export) }
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

// qaExts are the extensions the app parses with, as internal/app has them.
const qaExts = contract.ExtDefault

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
	recent   *workspace.Recent
	files    []string
	ws       *workspace.Workspace
	trash    *contracttest.RecordingTrasher
	reg      *commands.Registry
	s        *shell.Shell
	tt       *ui.Tester
	// holdRoot makes the next boot keep Workspace.SetRoot's answer in heldRoot
	// until the test calls it, and skip the automatic restore.
	holdRoot bool
	heldRoot func()
	// lang is the language the shell last set.
	lang   string
	assets *workspace.Assets
	render *fakeRenderer
}

// heldWorkspace keeps SetRoot's answer until the test lets it through.
type heldWorkspace struct {
	*workspace.Workspace
	a *app
}

func (w heldWorkspace) SetRoot(path string, done func(error)) {
	w.Workspace.SetRoot(path, func(err error) { w.a.heldRoot = func() { done(err) } })
}

func newApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	a := &app{t: t, dir: dir, data: filepath.Join(dir, "data"), q: make(pump, 4096), p: &platform{}, render: &fakeRenderer{available: true}}
	if err := os.MkdirAll(a.data, 0o755); err != nil {
		t.Fatal(err)
	}
	a.boot()
	// Wait for background saves before removing the temporary folder.
	t.Cleanup(a.idle)
	return a
}

// idle waits until session, recent and settings saves finish.
func (a *app) idle() {
	a.t.Helper()
	if a.s == nil {
		return
	}
	// Settings write on their own goroutine; the folder goes after the test.
	_ = a.store.Flush()
	done := false
	a.s.SaveSession(false, func(error) { done = true })
	a.pumpUntil("the session saved", &done)
	done = false
	a.sessions.Idle(func() { done = true })
	a.pumpUntil("the document sessions idle", &done)
	done = false
	a.recent.Idle(func() { done = true })
	a.pumpUntil("the recent list idle", &done)
	_ = a.store.Flush()
}

// pumpUntil runs what is posted until *done.
func (a *app) pumpUntil(what string, done *bool) {
	a.t.Helper()
	for !*done {
		select {
		case f := <-a.q:
			f()
		case <-time.After(30 * time.Second):
			a.t.Fatalf("never: %s", what)
		}
	}
}

// restore brings back the last session, as the app does after starting.
func (a *app) restore() {
	a.t.Helper()
	a.settle()
	done := false
	a.s.RestoreSession(func() { done = true })
	a.pumpUntil("the session restored", &done)
	a.settle()
}

// boot starts the app over the data folder, as a new process does.
func (a *app) boot() {
	t := a.t
	a.idle() // The last process's saves are done before the next one reads.
	files := fsstore.New()
	store, err := settings.New(filepath.Join(a.data, "settings.json"), files, a.q, nil)
	if err != nil {
		t.Fatal(err)
	}
	a.store = store
	base := store.Get()
	base.Editor.Exts = qaExts
	base.Files.AutosaveDelayMs = 1000
	a.timers = nil
	a.sessions = docsession.New(files, document.Factory{}, a.q, filepath.Join(a.data, "recovery"), docsession.Options{Settings: &base,
		AfterFunc: func(d time.Duration, fn func()) docsession.Timer {
			tm := &timer{at: a.now + d, fn: fn}
			a.timers = append(a.timers, tm)
			return tm
		}})
	a.trash = &contracttest.RecordingTrasher{}
	a.ws = workspace.New(&contracttest.SnapshotWatcher{}, a.trash, a.q, workspace.Options{})
	var ws contract.Workspace = a.ws
	if a.holdRoot {
		ws = heldWorkspace{a.ws, a}
	}
	a.assets = workspace.NewAssets(base.Files.ImageDir, base.Files.CopyImages)
	store.Subscribe(aurinkapp.FollowSettings(a.ws, a.assets, a.sessions, qaExts))
	a.reg = commands.New()
	a.reg.SetDefaults(commands.DefaultBindings())
	a.recent = workspace.NewRecent(filepath.Join(a.data, "recent.json"), a.q)
	a.s = shell.New(shell.Options{Finder: search.NewFinder(), Sessions: a.sessions, Settings: store, Projector: projection.New(), Editing: editing.New(),
		Commands: a.reg, Dispatcher: a.q, Platform: a.p, WindowButtons: true, Exts: qaExts, ObjectLangs: []string{"mermaid"}, Renderer: a.render, Fuzzy: search.NewFuzzy(),
		Files: func() []string { return a.files }, Workspace: ws, Search: search.NewWorkspaceSearch(),
		SessionStore: docsession.NewSessionStore(filepath.Join(a.data, "session.json"), files),
		Recent:       a.recent,
		ExportHTML:   aurinkapp.ExportHTML(files, a.q, qaExts),
		SetLanguage:  func(tag string) { a.lang = tag; a.tt.SetLocale(tag) }, Version: "9.9.9", Licenses: []string{"MyGo — MIT"}})
	if err := a.reg.Register(a.s.Commands()...); err != nil {
		t.Fatal(err)
	}
	a.tt = ui.NewTester(a.s.View, 1280, 800)
	a.tt.Frame()
	if !a.holdRoot {
		a.restore() // As the app does once its recovered copies are back.
	}
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

// order lists the tabs' names, left to right.
func (a *app) order() []string {
	var out []string
	for _, x := range a.s.Sessions() {
		out = append(out, x.State().Name)
	}
	return out
}

// until settles until cond holds: the workspace lists and changes files on its
// own goroutines and posts back.
func (a *app) until(what string, cond func() bool) {
	a.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		a.settle()
		a.tt.Advance(100 * time.Millisecond) // the sidebar slides open
		a.tt.Frame()
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			a.t.Fatalf("never: %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// folder makes files under a.dir/proj and opens it in the files panel.
func (a *app) folder(files ...string) string {
	a.t.Helper()
	root := filepath.Join(a.dir, "proj")
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if strings.HasSuffix(f, "/") {
			if err := os.MkdirAll(p, 0o755); err != nil {
				a.t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			a.t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("# "+filepath.Base(f)+"\n"), 0o644); err != nil {
			a.t.Fatal(err)
		}
	}
	a.p.folder = root
	a.run(contract.CmdFileOpenFolder)
	a.until("the folder lists", func() bool { _, ok := a.ws.List(root); return ok && a.tt.HasText(filepath.Base(files[0])) })
	return root
}

// rightClickRow right-clicks the tree's row i (folders first, then files, by
// name): a file's name also shows on its tab, which Find meets first.
func (a *app) rightClickRow(i int) {
	a.t.Helper()
	r, ok := a.tt.Find("proj")
	if !ok {
		a.t.Fatal("no tree")
	}
	// Measured: rows begin 36 DIPs in and 36 apart (28 high plus the outline's spacing).
	a.tt.RightClickAt(r.X+60, r.Y+r.H+float32(i)*36+18)
	a.tt.Frame()
}

// row is the middle of the tree's row i, as rightClickRow finds it.
func (a *app) row(i int) (x, y float32) {
	a.t.Helper()
	r, ok := a.tt.Find("proj")
	if !ok {
		a.t.Fatal("no tree")
	}
	return r.X + 60, r.Y + r.H + float32(i)*36 + 18
}

// drag drags the tree's row from onto row to, a few DIPs at a time.
func (a *app) drag(from, to int) {
	a.t.Helper()
	x0, y0 := a.row(from)
	x1, y1 := a.row(to)
	a.tt.Press(x0, y0)
	for k := 1; k <= 8; k++ {
		a.tt.Move(x0+(x1-x0)*float32(k)/8, y0+(y1-y0)*float32(k)/8)
		a.tt.Frame()
	}
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

// wantCaret checks the active document's caret.
func (a *app) wantCaret(off int) {
	a.t.Helper()
	if got := a.s.ActiveSession().Model().Selection().Primary(); got.Anchor != off || got.Head != off {
		a.t.Fatalf("the caret is at %+v, want %d", got, off)
	}
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
	{"primary+B bolds the selection", func(a *app) {
		a.open("a.md", "hello\n")
		a.tt.Key(ui.Cmd, ui.KeyA)
		a.tt.Key(ui.Cmd, ui.KeyB)
		a.settle()
		a.want(a.text(), "**hello**\n")
	}},
	{"primary+2 makes a heading and primary+0 undoes it", func(a *app) {
		a.open("a.md", "hello\n")
		a.tt.Key(ui.Cmd, ui.Key2)
		a.settle()
		a.want(a.text(), "## hello\n")
		a.tt.Key(ui.Cmd, ui.Key0)
		a.settle()
		a.want(a.text(), "hello\n")
	}},
	{"list and quote shortcuts change the block", func(a *app) {
		a.open("a.md", "hello\n")
		a.tt.Key(ui.Cmd|ui.Shift, ui.Key8)
		a.settle()
		a.want(a.text(), "- hello\n")
		a.tt.Key(ui.Cmd|ui.Shift, ui.Key8)
		a.tt.Key(ui.Cmd|ui.Shift, ui.KeyQ)
		a.settle()
		a.want(a.text(), "> hello\n")
	}},
	{"primary+T inserts a table and the table commands edit it", func(a *app) {
		a.open("a.md", "")
		a.tt.Key(ui.Cmd, ui.KeyT)
		a.settle()
		doc, _ := markdown.New(a.text(), 1, contract.ExtDefault)
		if !strings.Contains(a.text(), "|") || tableRows(doc.Snapshot()) != 3 {
			a.t.Fatalf("no 3-row table after primary+T: %q", a.text())
		}
		a.run(contract.CmdTableRowBelow)
		doc, _ = markdown.New(a.text(), 1, contract.ExtDefault)
		if tableRows(doc.Snapshot()) != 4 {
			a.t.Fatalf("row below did not add a row: %q", a.text())
		}
		a.run(contract.CmdTableDeleteRow)
		doc, _ = markdown.New(a.text(), 1, contract.ExtDefault)
		if tableRows(doc.Snapshot()) != 3 {
			a.t.Fatalf("delete row did not remove a row: %q", a.text())
		}
	}},
	{"primary+P opens a file by typing its name", func(a *app) {
		a.open("alpha.md", "a\n")
		a.open("beta.md", "b\n")
		a.tt.Key(ui.Cmd, ui.KeyP)
		a.settle()
		if !a.s.Palette().IsOpen() || a.s.Palette().Mode() != palette.ModeFiles {
			a.t.Fatal("primary+P did not open the palette on files")
		}
		a.typed("alp")
		a.want(fmt.Sprint(a.s.Palette().Rows()), "[alpha.md]")
		a.tt.Key(0, ui.KeyEnter)
		a.settle()
		if a.s.Palette().IsOpen() {
			a.t.Fatal("Enter left the palette open")
		}
		a.want(a.s.ActiveSession().State().Name, "alpha.md")
		a.typed("Z")
		a.settle()
		a.want(a.text(), "Za\n")
	}},
	{"the arrows move the selection and wrap", func(a *app) {
		a.open("a.md", "x\n")
		a.open("b.md", "x\n")
		a.open("c.md", "x\n")
		a.run(contract.CmdNavQuickOpen)
		p := a.s.Palette()
		if p.Selected() != 0 {
			a.t.Fatalf("selected %d at first", p.Selected())
		}
		a.tt.Key(0, ui.KeyUp)
		a.tt.Frame()
		if p.Selected() != len(p.Rows())-1 {
			a.t.Fatalf("Up from the first row selected %d of %d", p.Selected(), len(p.Rows()))
		}
		a.tt.Key(0, ui.KeyDown)
		a.tt.Key(0, ui.KeyDown)
		a.tt.Frame()
		want := p.Rows()[1]
		a.tt.Key(0, ui.KeyEnter)
		a.settle()
		a.want(a.s.ActiveSession().State().Name, want)
	}},
	{"primary+shift+P runs a command by name", func(a *app) {
		a.open("a.md", "hello\n")
		a.tt.Key(ui.Cmd, ui.KeyA)
		a.tt.Key(ui.Cmd|ui.Shift, ui.KeyP)
		a.settle()
		a.want(a.s.Palette().Query(), ">")
		a.typed(locales.ShellStrong())
		a.tt.Key(0, ui.KeyEnter)
		a.settle()
		a.want(a.text(), "**hello**\n")
	}},
	{"the command list has only commands that can run, with their keys", func(a *app) {
		a.run(contract.CmdNavPalette)
		rows := a.s.Palette().Rows()
		for _, r := range rows {
			if r == locales.ShellSave() || r == locales.ShellStrong() {
				a.t.Fatalf("with no document the palette lists %q", r)
			}
			if r == locales.ShellCommandPalette() {
				a.t.Fatal("the palette lists itself")
			}
		}
		if !slices.Contains(rows, locales.ShellNew()) {
			a.t.Fatalf("no %q in %v", locales.ShellNew(), rows)
		}
	}},
	{"Escape closes the palette and typing goes back to the document", func(a *app) {
		a.open("a.md", "hello\n")
		a.run(contract.CmdNavPalette)
		a.typed("zz")
		a.tt.Key(0, ui.KeyEscape)
		a.settle()
		if a.s.Palette().IsOpen() {
			a.t.Fatal("Escape left the palette open")
		}
		a.typed("x")
		a.settle()
		if !strings.Contains(a.text(), "x") || strings.Contains(a.text(), "zz") {
			a.t.Fatalf("typing after Escape: %q", a.text())
		}
	}},
	{"@ goes to a heading", func(a *app) {
		src := "# Intro\n\ntext\n\n## Details\n\nmore\n"
		a.open("a.md", src)
		a.run(contract.CmdNavQuickOpen)
		a.typed("@det")
		a.want(fmt.Sprint(a.s.Palette().Rows()), "[Details]")
		a.tt.Key(0, ui.KeyEnter)
		a.settle()
		a.caret(strings.Index(src, "## Details"))
	}},
	{"@ in source mode goes to the heading too", func(a *app) {
		src := "# Intro\n\ntext\n\n## Details\n"
		a.open("a.md", src)
		a.run(contract.CmdViewSource)
		a.run(contract.CmdNavGoToHeading)
		a.typed("Det")
		a.tt.Key(0, ui.KeyEnter)
		a.settle()
		a.run(contract.CmdViewSource) // back to live, which takes the source view's caret
		a.caret(strings.Index(src, "## Details"))
	}},
	{"clicking the omnibox opens the palette", func(a *app) {
		a.click(locales.ShellOmnibox())
		if !a.s.Palette().IsOpen() || a.s.Palette().Mode() != palette.ModeFiles {
			a.t.Fatal("the omnibox did not open the palette on files")
		}
	}},
	{"Chinese names match and open", func(a *app) {
		a.open("笔记.md", "x\n")
		a.open("路线图.md", "y\n")
		a.run(contract.CmdNavQuickOpen)
		a.typed("路线")
		a.want(fmt.Sprint(a.s.Palette().Rows()), "[路线图.md]")
		a.tt.Key(0, ui.KeyEnter)
		a.settle()
		a.want(a.s.ActiveSession().State().Name, "路线图.md")
	}},
	{"opening a folder shows its files, and a click opens one", func(a *app) {
		root := a.folder("b.md", "a.md", "notes/")
		if !a.s.SidebarOpen() {
			a.t.Fatal("the sidebar stayed closed")
		}
		for _, name := range []string{"proj", "notes", "a.md", "b.md"} {
			if !a.tt.HasText(name) {
				a.t.Fatalf("no %q in the tree", name)
			}
		}
		a.click("a.md")
		a.tabs(1)
		a.want(a.s.ActiveSession().State().Path, filepath.Join(root, "a.md"))
	}},
	{"a folder opens to show what is in it", func(a *app) {
		a.folder("top.md", "notes/inner.md")
		if a.tt.HasText("inner.md") {
			a.t.Fatal("a closed folder shows its files")
		}
		a.click("notes")
		a.until("the folder opens", func() bool { return a.tt.HasText("inner.md") })
		a.click("notes")
		a.until("the folder closes", func() bool { return !a.tt.HasText("inner.md") })
	}},
	{"new file from the menu makes it, opens it and edits its name", func(a *app) {
		root := a.folder("a.md")
		if err := a.tt.RightClick("a.md"); err != nil {
			a.t.Fatal(err)
		}
		a.tt.Frame()
		a.click(locales.ShellNewFile())
		untitled := filepath.Join(root, locales.ShellUntitled()+".md")
		a.until("the new file opens", func() bool { _, err := os.Stat(untitled); return err == nil && len(a.s.Sessions()) == 1 })
		a.tt.Key(ui.Cmd, ui.KeyA)
		a.typed("plan.md")
		a.tt.Key(0, ui.KeyEnter)
		a.until("the file is renamed", func() bool { _, err := os.Stat(filepath.Join(root, "plan.md")); return err == nil })
		a.until("its tab follows", func() bool { return a.s.ActiveSession().State().Name == "plan.md" })
	}},
	{"a rename onto an existing name says so and keeps editing", func(a *app) {
		root := a.folder("a.md", "b.md")
		a.tt.RightClick("a.md")
		a.tt.Frame()
		a.click(locales.ShellRename())
		a.tt.Key(ui.Cmd, ui.KeyA)
		a.typed("b.md")
		a.tt.Key(0, ui.KeyEnter)
		a.until("the clash shows", func() bool { return a.tt.HasText(locales.ShellNameExists()) })
		a.want(a.disk(filepath.Join(root, "b.md")), "# b.md\n")
		a.tt.Key(0, ui.KeyEscape)
		a.settle()
		if a.tt.HasText(locales.ShellNameExists()) {
			a.t.Fatal("Escape left the rename open")
		}
	}},
	{"renaming an open file in the tree moves its tab with it", func(a *app) {
		root := a.folder("a.md")
		a.click("a.md")
		a.typed("x")
		a.rightClickRow(0)
		a.click(locales.ShellRename())
		a.tt.Key(ui.Cmd, ui.KeyA)
		a.typed("renamed.md")
		a.tt.Key(0, ui.KeyEnter)
		a.until("the tab follows", func() bool { return a.s.ActiveSession().State().Path == filepath.Join(root, "renamed.md") })
		st := a.s.ActiveSession().State()
		if st.Deleted || st.Conflict {
			a.t.Fatalf("an in-app rename marked the tab: %+v", st)
		}
	}},
	{"trashing a file with unsaved changes asks first", func(a *app) {
		root := a.folder("a.md", "b.md")
		a.click("a.md")
		a.typed("unsaved")
		a.rightClickRow(0)
		a.click(locales.ShellTrash())
		a.settle()
		a.want(fmt.Sprint(a.p.asked), "[a.md]")
		if _, err := os.Stat(filepath.Join(root, "a.md")); err != nil {
			a.t.Fatal("trashed though the answer was no")
		}
		a.p.trashOK = true
		a.rightClickRow(0)
		a.click(locales.ShellTrash())
		a.until("it goes", func() bool { _, err := os.Stat(filepath.Join(root, "a.md")); return os.IsNotExist(err) })
		a.until("its tab knows", func() bool { return a.s.ActiveSession().State().Deleted })
		if !strings.Contains(a.text(), "unsaved") {
			a.t.Fatal("the tab lost its text")
		}
		// A file without changes goes without asking.
		a.rightClickRow(0)
		a.click(locales.ShellTrash())
		a.until("b goes", func() bool { _, err := os.Stat(filepath.Join(root, "b.md")); return os.IsNotExist(err) })
		a.want(fmt.Sprint(a.p.asked), "[a.md a.md]")
	}},
	{"dragging a file onto a folder moves it, its tab follows, and undo moves it back", func(a *app) {
		root := a.folder("a.md", "sub/x.md")
		a.s.Open(filepath.Join(root, "a.md"))
		a.settle()
		a.drag(1, 0)
		// The folder lights while a file that can go there is over it.
		lit := a.s.FileDropLit()
		x, y := a.row(0)
		a.tt.Release(x, y)
		a.until("a.md is in sub", func() bool { _, err := os.Stat(filepath.Join(root, "sub", "a.md")); return err == nil })
		if lit != filepath.Join(root, "sub") {
			a.t.Fatalf("lit %q while dragging", lit)
		}
		a.until("its tab follows", func() bool { return a.s.ActiveSession().State().Path == filepath.Join(root, "sub", "a.md") })
		a.click(locales.ShellUndo())
		a.until("a.md is back", func() bool { _, err := os.Stat(filepath.Join(root, "a.md")); return err == nil })
		a.until("its tab follows back", func() bool { return a.s.ActiveSession().State().Path == filepath.Join(root, "a.md") })
	}},
	{"a folder dropped into itself or a file onto its own folder stays", func(a *app) {
		root := a.folder("sub/", "sub/x.md", "a.md")
		a.s.ExpandFolder(filepath.Join(root, "sub"))
		a.until("sub lists", func() bool { return a.tt.HasText("x.md") })
		// Rows: sub, x.md (in sub), a.md. x.md onto sub is where it is.
		a.drag(1, 0)
		if lit := a.s.FileDropLit(); lit != "" {
			a.t.Fatalf("lit %q for a file over its own folder", lit)
		}
		x, y := a.row(0)
		a.tt.Release(x, y)
		a.settle()
		if _, err := os.Stat(filepath.Join(root, "sub", "x.md")); err != nil {
			a.t.Fatal(err)
		}
		if a.tt.HasText(locales.ShellUndo()) {
			a.t.Fatal("a move that did nothing offered undo")
		}
	}},
	{"a double click on a file edits its name", func(a *app) {
		root := a.folder("a.md")
		x, y := a.row(0)
		a.tt.ClickAt(x, y)
		a.tt.ClickAt(x, y)
		a.settle()
		a.tt.Key(ui.Cmd, ui.KeyA)
		a.typed("b.md")
		a.tt.Key(0, ui.KeyEnter)
		a.until("renamed", func() bool { _, err := os.Stat(filepath.Join(root, "b.md")); return err == nil })
	}},
	{"the rename command edits the active file's name in the tree", func(a *app) {
		root := a.folder("c.md", "sub/a.md")
		a.s.Open(filepath.Join(root, "sub", "a.md"))
		a.settle()
		a.run(contract.CmdFileRename)
		a.tt.Advance(time.Second)
		a.tt.Frame()
		a.tt.Key(ui.Cmd, ui.KeyA)
		a.typed("b.md")
		a.tt.Key(0, ui.KeyEnter)
		a.until("renamed", func() bool { _, err := os.Stat(filepath.Join(root, "sub", "b.md")); return err == nil })
		a.until("the tab follows", func() bool { return a.s.ActiveSession().State().Name == "b.md" })
	}},
	{"the outline lists the headings and a click goes to one", func(a *app) {
		src := "# One\n\ntext\n\n## Two\n\n### Three\n"
		a.open("a.md", src)
		a.run(contract.CmdViewOutline)
		a.tt.Advance(time.Second)
		a.tt.Frame()
		for _, h := range []string{"One", "Two", "Three"} {
			if !a.tt.HasText(h) {
				a.t.Fatalf("no %q in the outline", h)
			}
		}
		a.click("Three")
		a.wantCaret(strings.Index(src, "### Three"))
	}},
	{"the outline follows edits", func(a *app) {
		a.open("a.md", "# One\n")
		a.run(contract.CmdViewOutline)
		a.tt.Advance(time.Second)
		a.tt.Key(ui.Cmd, ui.KeyEnd)
		a.tt.Key(0, ui.KeyEnter)
		a.typed("## Added")
		a.settle()
		if !a.tt.HasText("Added") {
			a.t.Fatal("the outline did not pick up a new heading")
		}
	}},
	{"search streams the folder's matches and a click goes to one", func(a *app) {
		root := a.folder("a.md", "sub/b.md", "c.md")
		write := func(name, text string) {
			if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(text), 0o644); err != nil {
				a.t.Fatal(err)
			}
		}
		write("a.md", "alpha\nneedle one\n")
		write("sub/b.md", "x needle two needle\n")
		write("c.md", "nothing\n")
		a.run(contract.CmdNavSearchWorkspace)
		a.typed("needle")
		a.until("the matches come", func() bool { return a.tt.HasText(locales.ShellSearchCount("3", "2")) })
		a.click("x needle two needle")
		a.until("b.md opens", func() bool { return a.s.ActiveSession().State().Name == "b.md" })
		a.wantCaret(2)
		// The other file's match goes to its line.
		a.click("needle one")
		a.until("a.md opens", func() bool { return a.s.ActiveSession().State().Name == "a.md" })
		a.wantCaret(len("alpha\n"))
		a.tabs(2)
	}},
	{"search options narrow the matches and a bad pattern says so", func(a *app) {
		root := a.folder("a.md")
		if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("Needle needles\n"), 0o644); err != nil {
			a.t.Fatal(err)
		}
		a.run(contract.CmdNavSearchWorkspace)
		a.typed("needle")
		a.until("both", func() bool { return a.tt.HasText(locales.ShellSearchCount("2", "1")) })
		a.click(locales.ShellSearchCase())
		a.until("the lowercase one", func() bool { return a.tt.HasText(locales.ShellSearchCount("1", "1")) })
		a.click(locales.ShellSearchWord())
		a.until("none", func() bool { return a.tt.HasText(locales.ShellSearchNone()) })
		a.click(locales.ShellSearchWord())
		a.click(locales.ShellSearchRegex())
		a.typed("(")
		a.until("the error", func() bool { return a.tt.HasText(locales.ShellSearchBadRegex()) })
	}},
	{"a file's header folds its matches", func(a *app) {
		root := a.folder("a.md")
		if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("needle\nneedle again\n"), 0o644); err != nil {
			a.t.Fatal(err)
		}
		a.run(contract.CmdNavSearchWorkspace)
		a.typed("needle")
		a.until("the matches", func() bool { return a.tt.HasText("needle again") })
		a.click("a.md")
		if a.tt.HasText(" again") {
			a.t.Fatal("the folded file still shows its matches")
		}
		a.click("a.md")
		if !a.tt.HasText(" again") {
			a.t.Fatal("the file did not unfold")
		}
	}},
	{"format shortcuts do nothing in source mode", func(a *app) {
		a.open("a.md", "hello\n")
		a.run(contract.CmdViewSource)
		a.tt.Key(ui.Cmd, ui.KeyB)
		a.tt.Key(ui.Cmd, ui.Key2)
		a.settle()
		a.want(a.text(), "hello\n")
		a.notices(0)
	}},
	{"F8 and F9 toggle focus and typewriter", func(a *app) {
		a.open("a.md", "hello\n")
		checked := func(id contract.CommandID) bool {
			c, _ := a.reg.Lookup(id)
			return c.Checked()
		}
		a.tt.Key(0, ui.KeyF8)
		a.tt.Key(0, ui.KeyF9)
		a.settle()
		if !checked(contract.CmdViewFocus) || !checked(contract.CmdViewTypewriter) {
			a.t.Fatal("F8/F9 did not turn the modes on")
		}
		a.tt.Key(0, ui.KeyF8)
		a.settle()
		if checked(contract.CmdViewFocus) {
			a.t.Fatal("F8 did not turn focus mode off")
		}
	}},
	{"every registered command has a title in both languages", func(a *app) {
		for _, c := range a.reg.All() {
			if shell.Title(c.Title) == c.Title {
				a.t.Errorf("%s has no title for key %q", c.ID, c.Title)
			}
		}
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
	{"idle with a folder watched, each panel and the find bar open, no frames (07 §7)", func(a *app) {
		root := a.folder("a.md", "sub/b.md")
		a.s.Open(filepath.Join(root, "a.md"))
		a.settle()
		a.run(contract.CmdEditFind)
		a.typed("a")
		for _, id := range []contract.CommandID{contract.CmdViewFiles, contract.CmdViewOutline, contract.CmdViewSearch} {
			a.run(id)
			a.tt.Advance(time.Minute)
			a.tt.Frame()
			if a.tt.FramePending() {
				a.t.Fatalf("frames while idle after %s", id)
			}
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
			fresh := document.Factory{}.New("fresh", m.Text().String(), m.Meta(), qaExts).AST()
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
	{"dragging a tab past its neighbor reorders the tabs", func(a *app) {
		for _, n := range []string{"a.md", "b.md", "c.md"} {
			a.open(n, n+"\n")
		}
		from, ok := a.tt.Find("a.md")
		to, ok2 := a.tt.Find("b.md")
		if !ok || !ok2 {
			a.t.Fatal("tabs not found")
		}
		y := from.Y + from.H/2
		a.tt.Press(from.X+from.W/2, y)
		for x := from.X + from.W/2; x <= to.X+to.W*0.8; x += 4 {
			a.tt.Move(x, y)
			a.tt.Frame()
		}
		a.tt.Release(to.X+to.W*0.8, y)
		a.settle()
		a.want(strings.Join(a.order(), " "), "b.md a.md c.md")
		a.want(a.s.ActiveSession().State().Name, "c.md")
	}},
	{"dragging a tab back and forth keeps the order", func(a *app) {
		for _, n := range []string{"a.md", "b.md"} {
			a.open(n, n+"\n")
		}
		from, _ := a.tt.Find("a.md")
		y := from.Y + from.H/2
		a.tt.Press(from.X+from.W/2, y)
		for _, dx := range []float32{8, 16, 8, 0, -8, 0} {
			a.tt.Move(from.X+from.W/2+dx, y)
			a.tt.Frame()
		}
		a.tt.Release(from.X+from.W/2, y)
		a.settle()
		a.want(strings.Join(a.order(), " "), "a.md b.md")
	}},
	{"a closed tab reopens", func(a *app) {
		a.open("a.md", "a\n")
		a.open("b.md", "b\n")
		a.run(contract.CmdFileClose)
		a.tabs(1)
		a.tt.Key(ui.Cmd|ui.Shift, ui.KeyT)
		a.settle()
		a.tabs(2)
		a.want(a.s.ActiveSession().State().Name, "b.md")
		a.want(strings.Join(a.order(), " "), "a.md b.md")
	}},
	{"next and previous tab wrap around", func(a *app) {
		for _, n := range []string{"a.md", "b.md", "c.md"} {
			a.open(n, n+"\n")
		}
		a.tt.Key(ui.Cmd, ui.KeyTab)
		a.settle()
		a.want(a.s.ActiveSession().State().Name, "a.md")
		a.run(contract.CmdNavPrevTab)
		a.want(a.s.ActiveSession().State().Name, "c.md")
		a.run(contract.CmdNavGoToTab2)
		a.want(a.s.ActiveSession().State().Name, "b.md")
	}},
	{"many tabs scroll, and the active one shows", func(a *app) {
		var names []string
		for i := range 24 {
			names = append(names, fmt.Sprintf("a rather long document name %02d.md", i))
			a.open(names[i], "x\n")
		}
		visible := func(name string) {
			a.t.Helper()
			r, ok := a.tt.Find(name)
			if !ok || r.X < 0 || r.X+r.W > 1280 {
				a.t.Fatalf("%s is at %+v (found %v)", name, r, ok)
			}
		}
		visible(names[23])
		if r, ok := a.tt.Find(locales.ShellNew()); !ok || r.X+r.W > 1280 {
			a.t.Fatalf("the new button is at %+v", r)
		}
		a.run(contract.CmdNavGoToTab1)
		a.tt.Frame()
		visible(names[0])
	}},
}

// tableRows counts the rows of the first table, header included.
func tableRows(ast contract.AST) int {
	n := 0
	for i := range ast.Len() {
		if ast.Block(i).Kind == contract.BlockTableRow {
			n++
		}
	}
	return n
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

// TestPaletteAtScale types into quick open over 10,000 files, and into a
// palette over 500 commands (05 FE-2.4): each keystroke must answer in one
// frame's work, which the palette bounds by building only the rows in view.
// Measured on 2026-11-02: 73 ms per keystroke over 10,000 files while it built
// 200 rows, 10 ms with the virtual list.
func TestPaletteAtScale(t *testing.T) {
	a := newApp(t)
	for i := range 10000 {
		a.files = append(a.files, filepath.Join(a.dir, fmt.Sprintf("dir%03d", i%300), fmt.Sprintf("note-%05d.md", i)))
	}
	a.run(contract.CmdNavQuickOpen)
	typing(t, a.tt, a.s.Palette(), "note0042")

	var cmds []palette.Item
	for i := range 500 {
		cmds = append(cmds, palette.Item{Title: fmt.Sprintf("Command number %03d", i), Keys: "Ctrl K"})
	}
	p := palette.New(search.NewFuzzy(), fixedSource{cmds})
	tokens := theme.For(theme.Options{Hue: theme.DefaultHue})
	tt := ui.NewTester(func(c *ui.Context) {
		p.View(c, tokens, palette.Labels{Count: func(n int) string { return fmt.Sprint(n) }}, false)
	}, 1280, 800)
	p.Open(palette.ModeCommands)
	tt.Frame()
	typing(t, tt, p, "num04")
}

func typing(t *testing.T, tt *ui.Tester, p *palette.Palette, text string) {
	t.Helper()
	var worst time.Duration
	for _, r := range text {
		t0 := time.Now()
		tt.Type(string(r))
		tt.Frame()
		worst = max(worst, time.Since(t0))
		if n := len(p.Rows()); n > palette.MaxRows {
			t.Fatalf("%d rows listed", n)
		}
		// 460 DIPs of panel show about ten 40-DIP rows; the list builds a few beyond.
		if p.Built > 30 {
			t.Fatalf("%d rows built in a frame", p.Built)
		}
	}
	rows := p.Rows()
	if len(rows) == 0 {
		t.Fatalf("%q matched nothing", text)
	}
	t.Logf("%q: slowest keystroke %v, %d rows, %d built, first %q", text, worst, len(rows), p.Built, rows[0])
}

type fixedSource struct{ cmds []palette.Item }

func (f fixedSource) Files() []palette.Item    { return nil }
func (f fixedSource) Commands() []palette.Item { return f.cmds }
func (f fixedSource) Headings() []palette.Item { return nil }

// TestFileTreeAtScale opens a folder of 10,000 files (05 FE-2.3): the tree
// builds the rows in view only.
func TestFileTreeAtScale(t *testing.T) {
	a := newApp(t)
	names := make([]string, 10000)
	for i := range names {
		names[i] = fmt.Sprintf("note-%05d.md", i)
	}
	a.folder(names...)
	t0 := time.Now()
	a.tt.Frame()
	n := a.s.TreeRowsBuilt()
	t.Logf("frame %v, %d rows built", time.Since(t0), n)
	if n == 0 || n > 60 {
		t.Fatalf("%d tree rows built for 10,000 files", n)
	}
}

// TestSearchAtScale searches a folder of 10,000 files that all match: the
// panel stops at its cap and builds the rows in view only.
func TestSearchAtScale(t *testing.T) {
	a := newApp(t)
	names := make([]string, 10000)
	for i := range names {
		names[i] = fmt.Sprintf("note-%05d.md", i)
	}
	a.folder(names...)
	a.run(contract.CmdNavSearchWorkspace)
	t0 := time.Now()
	a.typed("note")
	a.until("the search stops at its cap", func() bool { return a.tt.HasText(locales.ShellSearchCapped("2000")) })
	t.Logf("capped after %v", time.Since(t0))
	t0 = time.Now()
	a.tt.Frame()
	n := a.s.SearchRowsBuilt()
	t.Logf("frame %v, %d rows built", time.Since(t0), n)
	if n == 0 || n > 60 {
		t.Fatalf("%d search rows built", n)
	}
	total := 0
	for _, f := range a.s.SearchResults() {
		total += len(f.Matches)
	}
	if total != 2000 {
		t.Fatalf("%d matches kept, want the cap", total)
	}
}
