package qa

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/views/shell"
	"aurink/locales"
)

func (a *app) shows(label string) bool {
	a.tt.Advance(400 * time.Millisecond) // the banner collapses in
	a.tt.Frame()
	_, ok := a.tt.Find(label)
	return ok
}

// conflicted opens a.md, edits it, changes it on disk and saves: the save
// stops in conflict.
func conflicted(t *testing.T) (*app, string) {
	a := newApp(t)
	p := a.open("a.md", "x\n")
	a.caret(1)
	a.typed("y")
	a.file("a.md", "theirs\n")
	a.run(contract.CmdFileSave)
	if !a.s.ActiveSession().State().Conflict {
		t.Fatal("no conflict")
	}
	if !a.shows(locales.ShellBannerConflict()) {
		t.Fatal("no conflict banner")
	}
	return a, p
}

// TestConflictKeepMine writes the document over the file (FE-2.5).
func TestConflictKeepMine(t *testing.T) {
	a, p := conflicted(t)
	a.click(locales.ShellKeepMine())
	a.want(a.disk(p), "xy\n")
	if a.s.ActiveSession().State().Conflict || a.shows(locales.ShellBannerConflict()) {
		t.Fatal("still in conflict")
	}
	a.want(a.s.Status(), locales.ShellSaved())
}

// TestConflictLoadTheirs takes the disk's text as a step of undo (FE-2.5).
func TestConflictLoadTheirs(t *testing.T) {
	a, p := conflicted(t)
	a.click(locales.ShellLoadTheirs())
	a.want(a.text(), "theirs\n")
	a.want(a.disk(p), "theirs\n")
	if a.s.ActiveSession().State().Conflict || a.shows(locales.ShellBannerConflict()) {
		t.Fatal("still in conflict")
	}
	a.s.ActiveSession().Model().Undo()
	a.want(a.text(), "xy\n")
}

// TestConflictCapsule: the capsule says the conflict too (02 §5.5); no notice
// repeats the banner.
func TestConflictCapsule(t *testing.T) {
	a, _ := conflicted(t)
	a.want(a.s.Status(), locales.ShellConflict())
	a.notices(0)
}

// TestBannerKeepsTheReadingPlace: the banner collapsing in does not scroll the
// text away from the line it was at (07 FE-2.5).
func TestBannerKeepsTheReadingPlace(t *testing.T) {
	a := newApp(t)
	var b strings.Builder
	for i := range 40 {
		fmt.Fprintf(&b, "## Part %d\n\n%s\n\n", i, strings.Repeat("Some words of the part. ", 30))
	}
	doc := b.String()
	p := a.open("long.md", doc)
	a.s.GoTo(strings.Index(doc, "## Part 20"))
	a.tt.Advance(time.Second)
	a.tt.Frame()
	live := a.s.ActiveLive()
	part := strings.Index(doc, "## Part 20")
	a.caret(part + strings.Index(doc[part:], "Some words"))
	a.typed("y")
	a.tt.Advance(time.Second)
	a.tt.Frame()
	scroll := live.ScrollOffset()
	heading, _ := live.VisibleHeading()
	if scroll <= 0 {
		t.Fatal("the jump did not scroll")
	}
	a.file(filepath.Base(p), "theirs\n")
	a.run(contract.CmdFileSave)
	for range 10 {
		a.tt.Advance(40 * time.Millisecond)
		a.tt.Frame()
		if got, _ := live.VisibleHeading(); live.ScrollOffset() != scroll || got != heading {
			t.Fatalf("the banner moved the view: scroll %v→%v, heading %v→%v", scroll, live.ScrollOffset(), heading, got)
		}
	}
	if !a.shows(locales.ShellBannerConflict()) {
		t.Fatal("no banner")
	}
}

// deleted opens a.md, edits it and deletes it on disk.
func deleted(t *testing.T) (*app, string) {
	a := newApp(t)
	p := a.open("a.md", "x\n")
	a.caret(1)
	a.typed("y")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	a.sessions.Recheck()
	a.settle()
	if !a.s.ActiveSession().State().Deleted || !a.shows(locales.ShellBannerDeleted()) {
		t.Fatal("no deleted banner")
	}
	a.want(a.s.Status(), locales.ShellDeletedStatus())
	return a, p
}

// TestDeletedSaveAgain creates the file again.
func TestDeletedSaveAgain(t *testing.T) {
	a, p := deleted(t)
	a.click(locales.ShellSaveAgain())
	a.want(a.disk(p), "xy\n")
	if a.shows(locales.ShellBannerDeleted()) {
		t.Fatal("the banner stayed")
	}
}

// TestDeletedCloseTab closes the tab, asking first since it holds unsaved text.
func TestDeletedCloseTab(t *testing.T) {
	a, _ := deleted(t)
	a.p.choice = shell.ChoiceDiscard
	a.click(locales.ShellCloseTab())
	a.tabs(0)
}

// TestReadOnlySaveAs offers a copy.
func TestReadOnlySaveAs(t *testing.T) {
	a := newApp(t)
	a.open("ro.md", "x\n")
	a.s.ActiveSession().Model().SetReadOnly(true)
	a.settle()
	if !a.shows(locales.ShellBannerReadOnly()) {
		t.Fatal("no read-only banner")
	}
	a.p.save = filepath.Join(a.dir, "copy.md")
	a.click(locales.ShellSaveAs())
	a.want(a.disk(a.p.save), "x\n")
}
