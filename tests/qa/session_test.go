package qa

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// TestRestartRestoresTheSession is INT-2.1's flow: open a folder, open files,
// edit, autosave, and after the process ends without closing (killed), the
// next start has the folder, its open folder, the tabs in order, the active
// one, the edit on disk, and each tab's caret and scroll (D9, D10).
func TestRestartRestoresTheSession(t *testing.T) {
	a := newApp(t)
	root := a.folder("c.md", "docs/a.md")
	docs := filepath.Join(root, "docs")
	a.s.ExpandFolder(docs)
	var b strings.Builder
	for i := range 60 {
		fmt.Fprintf(&b, "## Part %d\n\n%s\n\n", i, strings.Repeat("Words of the part. ", 20))
	}
	long := filepath.Join(root, "long.md")
	if err := os.WriteFile(long, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{filepath.Join(docs, "a.md"), filepath.Join(root, "c.md"), long} {
		a.s.Open(p)
		a.settle()
	}
	// Scrolled down the long file, with the caret there.
	at := strings.Index(b.String(), "## Part 40")
	a.s.GoTo(at)
	a.tt.Advance(time.Second)
	a.tt.Frame()
	a.caret(at + 3)
	row, off := a.s.ActiveLive().ViewAnchor()
	if row == 0 {
		t.Fatal("the long file did not scroll")
	}
	// An edit in c.md, autosaved; c.md active.
	a.run(contract.CmdNavGoToTab2)
	a.caret(len("# c.md"))
	a.typed("!")
	a.advance(2 * time.Second)
	a.want(a.disk(filepath.Join(root, "c.md")), "# c.md!\n")

	a.idle()
	a.boot() // No close: a killed process.

	if got := a.ws.Root(); got != root {
		t.Fatalf("root %q, want %q", got, root)
	}
	if !slices.Contains(a.s.SessionState().Expanded, docs) {
		t.Fatalf("docs not open in the tree: %v", a.s.SessionState().Expanded)
	}
	a.want(strings.Join(a.order(), ","), "a.md,c.md,long.md")
	a.want(a.s.ActiveSession().State().Name, "c.md")
	a.want(a.text(), "# c.md!\n")
	a.run(contract.CmdNavGoToTab3)
	a.tt.Frame()
	if sel := a.s.ActiveSession().Model().Selection().Primary(); sel.Head != at+3 {
		t.Fatalf("caret at %d, want %d", sel.Head, at+3)
	}
	if r, o := a.s.ActiveLive().ViewAnchor(); r != row || o != off {
		t.Fatalf("scroll at row %d +%v, want %d +%v", r, o, row, off)
	}
}

// TestCleanExitRestoresTheSession: closing the window saves the session before
// the tabs close one by one, so all of them come back.
func TestCleanExitRestoresTheSession(t *testing.T) {
	a := newApp(t)
	a.open("a.md", "a\n")
	a.open("b.md", "b\n")
	saved := false
	a.s.SaveSession(true, func(error) { saved = true })
	a.pumpUntil("saved", &saved)
	closed := false
	a.s.CloseAll(func(ok bool) { closed = ok })
	a.settle()
	if !closed {
		t.Fatal("not closed")
	}
	a.boot()
	a.want(strings.Join(a.order(), ","), "a.md,b.md")
	a.want(a.s.ActiveSession().State().Name, "b.md")
}

// TestRestoreSkipsMissingFiles: a file gone since is left out, and one notice
// says how many.
func TestRestoreSkipsMissingFiles(t *testing.T) {
	a := newApp(t)
	a.open("a.md", "a\n")
	gone := a.open("b.md", "b\n")
	a.idle()
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	a.boot()
	a.want(strings.Join(a.order(), ","), "a.md")
	a.want(strings.Join(a.s.Notices(), ","), locales.ShellSessionMissing(1))
}

// restoreOff turns session restore off, written to disk before a restart reads it.
func (a *app) restoreOff() {
	a.t.Helper()
	if err := a.store.Update(func(v *contract.Settings) { v.Files.RestoreSession = false }); err != nil {
		a.t.Fatal(err)
	}
	if err := a.store.Flush(); err != nil {
		a.t.Fatal(err)
	}
}

// TestRestoreSessionOff starts empty when the setting is off.
func TestRestoreSessionOff(t *testing.T) {
	a := newApp(t)
	a.open("a.md", "a\n")
	a.idle()
	a.restoreOff()
	a.boot()
	a.tabs(0)
}

// TestDamagedSessionStartsEmpty: a damaged session.json is set aside and the
// app starts empty, saying so.
func TestDamagedSessionStartsEmpty(t *testing.T) {
	a := newApp(t)
	a.idle() // The first start saves its empty session.
	if err := os.WriteFile(filepath.Join(a.data, "session.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.boot()
	a.tabs(0)
	a.want(strings.Join(a.s.Notices(), ","), locales.ShellSessionUnreadable())
	// And it saves again from then on.
	a.open("a.md", "a\n")
	a.idle()
	a.boot()
	a.tabs(1)
}

// TestExportHTML writes the page where the dialog says, with the theme, the
// title and highlighted code, and says it did.
func TestExportHTML(t *testing.T) {
	a := newApp(t)
	a.open("note.md", "# Title\n\nSome *words*.\n\n```go\nfunc f() {}\n```\n")
	a.p.export = filepath.Join(a.dir, "out", "note.html")
	if err := os.MkdirAll(filepath.Dir(a.p.export), 0o755); err != nil {
		t.Fatal(err)
	}
	a.run(contract.CmdFileExportHTML)
	a.until("the page is written", func() bool { _, err := os.Stat(a.p.export); return err == nil })
	page := a.disk(a.p.export)
	for _, want := range []string{"<!DOCTYPE html>", "<title>note</title>", "<h1", "<em>words</em>", `class="tok-keyword"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("the page lacks %s:\n%s", want, page)
		}
	}
	a.until("the notice", func() bool { return slices.Contains(a.s.Notices(), locales.ShellExported()) })
}

// TestRecentOnTheWelcomePage: opened files and folders show on the welcome
// page, after a restart too, and a click opens one.
func TestRecentOnTheWelcomePage(t *testing.T) {
	a := newApp(t)
	p := a.open("recent-one.md", "x\n")
	a.folder("in-folder.md")
	a.p.choice = 0
	a.s.CloseAll(func(bool) {})
	a.settle()
	a.idle()
	a.restoreOff()
	a.boot()
	a.tabs(0)
	for _, want := range []string{locales.ShellRecent(), "recent-one.md", "proj"} {
		if !a.tt.HasText(want) {
			t.Fatalf("the welcome page lacks %q", want)
		}
	}
	a.click("recent-one.md")
	a.tabs(1)
	a.want(a.s.ActiveSession().State().Path, p)
}

// TestNewDefaultShortcuts: F2 renames the active file in the tree, F3 and
// Shift+F3 step through matches (02 §7; macOS has ⌘G and ⇧⌘G).
func TestNewDefaultShortcuts(t *testing.T) {
	a := newApp(t)
	root := a.folder("c.md")
	a.s.Open(filepath.Join(root, "c.md"))
	a.settle()
	a.key(0, ui.KeyF2)
	a.tt.Advance(time.Second)
	a.tt.Frame()
	if _, ok := a.tt.Find(locales.ShellRename()); !ok {
		t.Fatal("F2 did not start a rename")
	}
	a.key(0, ui.KeyEscape)

	a.open("f.md", findDoc)
	a.caret(0)
	a.run(contract.CmdEditFind)
	a.typed("alpha")
	a.wantStatus("1 / 4")
	next, prev := func() { a.key(0, ui.KeyF3) }, func() { a.key(ui.Shift, ui.KeyF3) }
	if runtime.GOOS == "darwin" {
		next, prev = func() { a.key(ui.Cmd, ui.KeyG) }, func() { a.key(ui.Cmd|ui.Shift, ui.KeyG) }
	}
	next()
	a.wantStatus("2 / 4")
	prev()
	a.wantStatus("1 / 4")
}

// TestRecoveredTabKeepsItsPlace: after a crash, the unsaved document comes
// back from its recovery copy before the session opens the others; the tabs
// still take their saved order, and the edit is still there.
func TestRecoveredTabKeepsItsPlace(t *testing.T) {
	a := newApp(t)
	a.open("a.md", "a\n")
	a.open("b.md", "b\n")
	a.open("c.md", "c\n")
	a.caret(1)
	a.typed("!") // unsaved: no autosave yet
	a.advance(600 * time.Millisecond)
	a.run(contract.CmdNavGoToTab1)
	a.idle()
	a.boot() // killed
	a.want(strings.Join(a.order(), ","), "a.md,b.md,c.md")
	a.want(a.s.ActiveSession().State().Name, "a.md")
	a.run(contract.CmdNavGoToTab3)
	a.want(a.text(), "c!\n")
}
