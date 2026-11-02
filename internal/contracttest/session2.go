package contracttest

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"aurink/internal/contract"
)

// SessionV2Fixture builds session managers that watch through Watcher.
type SessionV2Fixture struct {
	SessionFixture
	Watcher *SnapshotWatcher
}

func readFile(t *testing.T, path string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(b), true
}

type s2 struct {
	t    *testing.T
	f    SessionV2Fixture
	m    contract.Sessions
	path string
	s    contract.Session
}

func newS2(t *testing.T, newFixture func(t *testing.T) SessionV2Fixture) *s2 {
	f := newFixture(t)
	h := &s2{t: t, f: f, m: f.New(), path: filepath.Join(f.Dir, "a.md")}
	write(t, h.path, "base")
	h.s = h.open(h.path)
	return h
}

func (h *s2) open(path string) contract.Session {
	h.t.Helper()
	var got contract.Session
	var gotErr error
	h.m.Open(path, func(s contract.Session, err error) { got, gotErr = s, err })
	h.f.Settle()
	if gotErr != nil || got == nil {
		h.t.Fatalf("open %s: %v", path, gotErr)
	}
	return got
}

func (h *s2) text() string { return h.s.Model().Text().String() }

func (h *s2) typeText(s string) {
	h.t.Helper()
	if _, err := h.s.Model().Apply(insertAt(h.s.Model().Text().Len(), s)); err != nil {
		h.t.Fatal(err)
	}
}

// external lets the watcher see the disk and settles.
func (h *s2) external() {
	h.f.Watcher.Flush()
	h.f.Settle()
}

func (h *s2) finish(run func(done func(error))) error {
	h.t.Helper()
	var got error
	called := 0
	run(func(err error) { got = err; called++ })
	h.f.Settle()
	if called != 1 {
		h.t.Fatalf("done called %d times", called)
	}
	return got
}

// autosaveIsPaused checks that a long pause writes nothing to path.
func (h *s2) autosaveIsPaused(path string) {
	h.t.Helper()
	before, existed := readFile(h.t, path)
	h.typeText("!")
	h.f.Advance(10 * h.f.Autosave)
	h.f.Settle()
	if after, exists := readFile(h.t, path); after != before || exists != existed {
		h.t.Fatalf("autosave wrote %q (existed %v, exists %v)", after, existed, exists)
	}
}

func (h *s2) want(path string, edited, conflict, deleted bool) {
	h.t.Helper()
	st := h.s.State()
	if st.Path != path || st.Edited != edited || st.Conflict != conflict || st.Deleted != deleted {
		h.t.Fatalf("state %+v; want path %s, edited %v, conflict %v, deleted %v", st, path, edited, conflict, deleted)
	}
}

const (
	mine   = "base+mine"
	theirs = "their longer text"
)

// TestSessionsV2 checks BE-2.7: the conflict matrix of docs/07 §6 batch 4
// (disk events by document states) and D2–D6.
func TestSessionsV2(t *testing.T, newFixture func(t *testing.T) SessionV2Fixture) {
	type state struct {
		name  string
		setup func(h *s2)
	}
	states := []state{
		{"unedited", func(h *s2) {}},
		{"edited", func(h *s2) { h.typeText("+mine") }},
		// The disk event lands while a save is in flight: before or after the
		// write, which the test does not control. Cells check what holds either way.
		{"saving", func(h *s2) { h.typeText("+mine"); h.s.Save(func(error) {}) }},
		{"conflicted", func(h *s2) {
			h.typeText("+mine")
			write(h.t, h.path, "first theirs")
			h.external()
			h.want(h.path, true, true, false)
		}},
		{"read-only", func(h *s2) { h.s.Model().SetReadOnly(true) }},
	}
	other := func(h *s2) string { return filepath.Join(h.f.Dir, "b.md") }
	events := []struct {
		name  string
		apply func(h *s2)
		check map[string]func(h *s2)
	}{
		{"modified", func(h *s2) { write(h.t, h.path, theirs); h.external() }, map[string]func(h *s2){
			"unedited": func(h *s2) {
				h.want(h.path, false, false, false)
				if h.text() != theirs {
					h.t.Fatalf("not reloaded: %q", h.text())
				}
				h.s.Model().Undo()
				if h.text() != "base" {
					h.t.Fatalf("the reload is not one undoable step: %q", h.text())
				}
			},
			"edited": func(h *s2) {
				h.want(h.path, true, true, false)
				if h.text() != mine {
					h.t.Fatalf("text %q", h.text())
				}
				h.autosaveIsPaused(h.path)
			},
			"saving": func(h *s2) {
				disk, _ := readFile(h.t, h.path)
				st := h.s.State()
				switch {
				case st.Conflict && h.text() == mine && disk == theirs:
				case !st.Conflict && !st.Edited && h.text() == theirs && disk == theirs:
					h.s.Model().Undo()
					if h.text() != mine {
						h.t.Fatalf("saved, reloaded, and undo gives %q", h.text())
					}
				default:
					h.t.Fatalf("state %+v, text %q, disk %q", st, h.text(), disk)
				}
			},
			"conflicted": func(h *s2) {
				h.want(h.path, true, true, false)
				if err := h.finish(h.s.LoadTheirs); err != nil {
					h.t.Fatal(err)
				}
				if h.text() != theirs {
					h.t.Fatalf("LoadTheirs gave %q, not the latest disk text", h.text())
				}
			},
			"read-only": func(h *s2) {
				if h.text() != theirs || !h.s.State().ReadOnly || h.s.State().Conflict {
					h.t.Fatalf("text %q, state %+v", h.text(), h.s.State())
				}
			},
		}},
		{"deleted", func(h *s2) {
			if err := os.Remove(h.path); err != nil {
				h.t.Fatal(err)
			}
			h.external()
		}, map[string]func(h *s2){
			"unedited": func(h *s2) {
				h.want(h.path, false, false, true)
				if h.text() != "base" {
					h.t.Fatalf("text %q", h.text())
				}
				h.autosaveIsPaused(h.path)
				if err := h.finish(h.s.Save); err != nil {
					h.t.Fatal(err)
				}
				if disk, _ := readFile(h.t, h.path); disk != "base!" || h.s.State().Deleted {
					h.t.Fatalf("Save gave %q, state %+v", disk, h.s.State())
				}
			},
			"edited": func(h *s2) {
				h.want(h.path, true, false, true)
				h.autosaveIsPaused(h.path)
				// The recovery copy keeps up.
				h.f.Advance(h.f.Autosave / 2)
				h.f.Settle()
				var got []contract.Session
				h.f.New().Restore(func(s []contract.Session, err error) { got = s })
				h.f.Settle()
				if len(got) != 1 || got[0].Model().Text().String() != mine+"!" {
					h.t.Fatalf("restored %d sessions", len(got))
				}
			},
			"saving": func(h *s2) {
				disk, exists := readFile(h.t, h.path)
				st := h.s.State()
				if h.text() != mine || (exists && (disk != mine || st.Deleted)) || (!exists && !st.Deleted) {
					h.t.Fatalf("state %+v, text %q, disk %q (exists %v)", st, h.text(), disk, exists)
				}
			},
			"conflicted": func(h *s2) {
				if !h.s.State().Deleted || h.text() != mine {
					h.t.Fatalf("state %+v, text %q", h.s.State(), h.text())
				}
			},
			"read-only": func(h *s2) {
				if !h.s.State().Deleted || h.text() != "base" {
					h.t.Fatalf("state %+v, text %q", h.s.State(), h.text())
				}
			},
		}},
		{"renamed in place", func(h *s2) {
			if err := os.Rename(h.path, other(h)); err != nil {
				h.t.Fatal(err)
			}
			h.external()
		}, map[string]func(h *s2){
			"unedited": func(h *s2) {
				h.want(other(h), false, false, false)
				if h.text() != "base" || h.s.State().Name != "b.md" {
					h.t.Fatalf("text %q, state %+v", h.text(), h.s.State())
				}
				if s, ok := h.m.FindByPath(other(h)); !ok || s.ID() != h.s.ID() {
					h.t.Fatal("FindByPath does not find the new path")
				}
			},
			"edited": func(h *s2) {
				h.want(other(h), true, false, false)
				if err := h.finish(h.s.Save); err != nil {
					h.t.Fatal(err)
				}
				if disk, _ := readFile(h.t, other(h)); disk != mine {
					h.t.Fatalf("b.md is %q", disk)
				}
				if _, exists := readFile(h.t, h.path); exists {
					h.t.Fatal("a.md came back")
				}
			},
			"saving": func(h *s2) {
				if h.text() != mine {
					h.t.Fatalf("text %q", h.text())
				}
				if h.s.State().Deleted {
					return
				}
				if err := h.finish(h.s.Save); err != nil {
					h.t.Fatal(err)
				}
				if disk, _ := readFile(h.t, h.s.State().Path); disk != mine {
					h.t.Fatalf("%s is %q", h.s.State().Path, disk)
				}
				if _, exists := readFile(h.t, h.path); exists && h.s.State().Path != h.path {
					h.t.Fatal("a.md came back")
				}
			},
			"conflicted": func(h *s2) { h.want(other(h), true, true, false) },
			"read-only":  func(h *s2) { h.want(other(h), false, false, false) },
		}},
		{"deleted and created again", func(h *s2) {
			if err := os.Remove(h.path); err != nil {
				h.t.Fatal(err)
			}
			h.external()
			write(h.t, h.path, theirs)
			h.external()
		}, map[string]func(h *s2){
			"unedited": func(h *s2) {
				h.want(h.path, false, false, false)
				if h.text() != theirs {
					h.t.Fatalf("text %q", h.text())
				}
			},
			"edited": func(h *s2) {
				h.want(h.path, true, true, false)
				if h.text() != mine {
					h.t.Fatalf("text %q", h.text())
				}
			},
			// saving: the deletion and the modification are the cells above, one
			// after the other; together they add nothing the test can pin.
			"conflicted": func(h *s2) { h.want(h.path, true, true, false) },
			"read-only": func(h *s2) {
				if h.text() != theirs || h.s.State().Deleted {
					h.t.Fatalf("text %q, state %+v", h.text(), h.s.State())
				}
			},
		}},
	}
	cells := 0
	for _, ev := range events {
		for _, st := range states {
			check, ok := ev.check[st.name]
			if !ok {
				continue
			}
			cells++
			t.Run(ev.name+" while "+st.name, func(t *testing.T) {
				h := newS2(t, newFixture)
				st.setup(h)
				ev.apply(h)
				check(h)
			})
		}
	}
	if cells < 12 {
		t.Fatalf("the matrix has %d cells, docs/05 asks for at least 12", cells)
	}

	t.Run("own saves are not external changes", func(t *testing.T) {
		h := newS2(t, newFixture)
		h.typeText("+mine")
		if err := h.finish(h.s.Save); err != nil {
			t.Fatal(err)
		}
		v := h.s.Model().Version()
		h.external()
		h.want(h.path, false, false, false)
		if h.s.Model().Version() != v {
			t.Fatal("the document reloaded its own save")
		}
		// An autosave too.
		h.typeText("x")
		h.f.Advance(10 * h.f.Autosave)
		h.f.Settle()
		v = h.s.Model().Version()
		h.external()
		h.want(h.path, false, false, false)
		if h.s.Model().Version() != v {
			t.Fatal("the document reloaded its autosave")
		}
	})
	t.Run("recheck finds what the watcher missed", func(t *testing.T) {
		h := newS2(t, newFixture)
		write(t, h.path, theirs)
		h.m.Recheck()
		h.f.Settle()
		if h.text() != theirs {
			t.Fatalf("text %q", h.text())
		}
		if err := os.Remove(h.path); err != nil {
			t.Fatal(err)
		}
		h.m.Recheck()
		h.f.Settle()
		h.want(h.path, false, false, true)
	})
	t.Run("an overflow rechecks", func(t *testing.T) {
		h := newS2(t, newFixture)
		h.typeText("+mine")
		write(t, h.path, theirs)
		h.f.Watcher.Overflow()
		h.f.Settle()
		h.want(h.path, true, true, false)
	})
	t.Run("moved to another directory outside the app is a deletion", func(t *testing.T) {
		h := newS2(t, newFixture)
		mkdir(t, filepath.Join(h.f.Dir, "sub"))
		if err := os.Rename(h.path, filepath.Join(h.f.Dir, "sub", "a.md")); err != nil {
			t.Fatal(err)
		}
		h.f.Watcher.Flush()
		h.f.Settle()
		h.want(h.path, false, false, true)
	})
	for _, order := range []string{"events first", "Moved first"} {
		t.Run("moved in the app, "+order, func(t *testing.T) {
			f := newFixture(t)
			m := f.New()
			h := &s2{t: t, f: f, m: m}
			write(t, filepath.Join(f.Dir, "dir", "x.md"), "x")
			write(t, filepath.Join(f.Dir, "dir", "deep", "y.md"), "y")
			write(t, filepath.Join(f.Dir, "z.md"), "z")
			x := h.open(filepath.Join(f.Dir, "dir", "x.md"))
			y := h.open(filepath.Join(f.Dir, "dir", "deep", "y.md"))
			z := h.open(filepath.Join(f.Dir, "z.md"))
			mkdir(t, filepath.Join(f.Dir, "dst"))
			if err := os.Rename(filepath.Join(f.Dir, "dir"), filepath.Join(f.Dir, "dst", "dir")); err != nil {
				t.Fatal(err)
			}
			if order == "events first" {
				f.Watcher.Flush()
				f.Settle()
			}
			m.Moved(filepath.Join(f.Dir, "dir"), filepath.Join(f.Dir, "dst", "dir"))
			f.Watcher.Flush()
			f.Settle()
			for _, c := range []struct {
				s    contract.Session
				path string
			}{{x, filepath.Join(f.Dir, "dst", "dir", "x.md")}, {y, filepath.Join(f.Dir, "dst", "dir", "deep", "y.md")}, {z, filepath.Join(f.Dir, "z.md")}} {
				st := c.s.State()
				if st.Path != c.path || st.Deleted || st.Conflict || st.Edited {
					t.Errorf("state %+v, want path %s", st, c.path)
				}
			}
		})
	}
	t.Run("each open file's directory is watched", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		h := &s2{t: t, f: f, m: m}
		for _, p := range []string{"a/x.md", "a/y.md", "b/z.md"} {
			write(t, filepath.Join(f.Dir, filepath.FromSlash(p)), p)
		}
		x := h.open(filepath.Join(f.Dir, "a", "x.md"))
		y := h.open(filepath.Join(f.Dir, "a", "y.md"))
		h.open(filepath.Join(f.Dir, "b", "z.md"))
		want := []string{filepath.Join(f.Dir, "a"), filepath.Join(f.Dir, "b")}
		if got := f.Watcher.Paths(); !reflect.DeepEqual(got, want) {
			t.Fatalf("watching %q, want %q", got, want)
		}
		x.Close(false)
		y.Close(false)
		f.Settle()
		if got := f.Watcher.Paths(); !reflect.DeepEqual(got, want[1:]) {
			t.Fatalf("after closing a/: watching %q", got)
		}
	})
}

// SessionStoreFixture builds stores over one directory.
type SessionStoreFixture struct {
	New func() contract.SessionStore
	// File is where the store keeps its state.
	File string
}

// TestSessionStore checks D9 for session stores.
func TestSessionStore(t *testing.T, newFixture func(t *testing.T) SessionStoreFixture) {
	empty := contract.SessionState{Version: contract.SessionStateVersion, Active: -1}
	t.Run("nothing saved", func(t *testing.T) {
		got, err := newFixture(t).New().Load()
		if err != nil || !reflect.DeepEqual(got, empty) {
			t.Fatalf("%+v, %v", got, err)
		}
	})
	t.Run("round trip", func(t *testing.T) {
		f := newFixture(t)
		s := contract.SessionState{Version: contract.SessionStateVersion, Root: filepath.Join("C:", "notes"),
			Expanded: []string{"a", "中文 dir"}, Active: 1, Tabs: []contract.TabState{
				{Path: "x.md", View: contract.ViewState{Anchor: 3, Head: 7, Block: 12, BlockOffset: 4.5}},
				{Path: "y.md"},
			}}
		if err := f.New().Save(s); err != nil {
			t.Fatal(err)
		}
		if got, err := f.New().Load(); err != nil || !reflect.DeepEqual(got, s) {
			t.Fatalf("%+v, %v", got, err)
		}
	})
	for name, data := range map[string]string{"corrupt": "{not json", "newer": `{"Version": 999}`} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			write(t, f.File, data)
			got, err := f.New().Load()
			wantCode(t, err, contract.CodeInvalid)
			if !reflect.DeepEqual(got, empty) {
				t.Fatalf("%+v", got)
			}
			if _, exists := readFile(t, f.File); exists {
				t.Fatal("the bad file was not moved aside")
			}
			entries, _ := os.ReadDir(filepath.Dir(f.File))
			backup := false
			for _, e := range entries {
				if b, _ := os.ReadFile(filepath.Join(filepath.Dir(f.File), e.Name())); string(b) == data {
					backup = true
				}
			}
			if !backup {
				t.Fatal("no backup keeps the bad file's bytes")
			}
		})
	}
}
