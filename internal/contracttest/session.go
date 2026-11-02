package contracttest

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"aurink/internal/contract"
)

// SessionFixture builds session managers over one recovery directory.
type SessionFixture struct {
	// New returns a manager, as a fresh start of the process would. Autosave is on.
	New func() contract.Sessions
	// Settle runs background work and posted callbacks until nothing is left.
	Settle func()
	// Advance moves the fake clock and fires the timers that became due.
	Advance func(d time.Duration)
	// Autosave is how long after an edit an automatic save happens.
	Autosave time.Duration
	// Dir is an empty directory for the test's documents.
	Dir string
}

// TestSessions checks BE-2.4 and docs/03 §4 for session managers.
func TestSessions(t *testing.T, newFixture func(t *testing.T) SessionFixture) {
	write := func(t *testing.T, path, text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	read := func(t *testing.T, path string) string {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	open := func(t *testing.T, f SessionFixture, m contract.Sessions, path string) (contract.Session, error) {
		t.Helper()
		var got contract.Session
		var gotErr error
		called := 0
		m.Open(path, func(s contract.Session, err error) { got, gotErr = s, err; called++ })
		f.Settle()
		if called != 1 {
			t.Fatalf("Open called back %d times", called)
		}
		return got, gotErr
	}
	mustOpen := func(t *testing.T, f SessionFixture, m contract.Sessions, path string) contract.Session {
		t.Helper()
		s, err := open(t, f, m, path)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	finish := func(t *testing.T, f SessionFixture, run func(done func(error))) error {
		t.Helper()
		var got error
		called := 0
		run(func(err error) { got = err; called++ })
		f.Settle()
		if called != 1 {
			t.Fatalf("done called %d times", called)
		}
		return got
	}
	type_ := func(t *testing.T, s contract.Session, text string) {
		t.Helper()
		end := s.Model().Text().Len()
		tx := insertAt(end, text)
		if _, err := s.Model().Apply(tx); err != nil {
			t.Fatal(err)
		}
	}
	restore := func(t *testing.T, f SessionFixture, m contract.Sessions) []contract.Session {
		t.Helper()
		var got []contract.Session
		called := 0
		m.Restore(func(s []contract.Session, err error) {
			if err != nil {
				t.Error(err)
			}
			got = s
			called++
		})
		f.Settle()
		if called != 1 {
			t.Fatalf("Restore called back %d times", called)
		}
		return got
	}

	t.Run("open once per path", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		path := filepath.Join(f.Dir, "a.md")
		write(t, path, "正文\r\n")
		a := mustOpen(t, f, m, path)
		b := mustOpen(t, f, m, path)
		if a.ID() != b.ID() || len(m.All()) != 1 {
			t.Fatalf("two sessions: %v %v, %d in all", a.ID(), b.ID(), len(m.All()))
		}
		if found, ok := m.FindByPath(path); !ok || found.ID() != a.ID() {
			t.Error("FindByPath")
		}
		if found, ok := m.Get(a.ID()); !ok || found.ID() != a.ID() {
			t.Error("Get")
		}
		if a.Model().Text().String() != "正文\r\n" || a.State().Edited || a.State().Name != "a.md" {
			t.Errorf("text %q, state %+v", a.Model().Text().String(), a.State())
		}
	})
	t.Run("two opens in flight make one session", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		path := filepath.Join(f.Dir, "a.md")
		write(t, path, "x")
		var got []contract.Session
		for range 2 {
			m.Open(path, func(s contract.Session, err error) {
				if err != nil {
					t.Error(err)
				}
				got = append(got, s)
			})
		}
		f.Settle()
		if len(got) != 2 || got[0].ID() != got[1].ID() || len(m.All()) != 1 {
			t.Fatalf("%d callbacks, %d sessions", len(got), len(m.All()))
		}
	})
	t.Run("open errors", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		_, err := open(t, f, m, filepath.Join(f.Dir, "missing.md"))
		wantCode(t, err, contract.CodeNotFound)
		if len(m.All()) != 0 {
			t.Error("a failed open left a session")
		}
	})
	t.Run("edited and saved", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		path := filepath.Join(f.Dir, "a.md")
		write(t, path, "a")
		s := mustOpen(t, f, m, path)
		var states []contract.DocState
		defer s.OnState(func(st contract.DocState) { states = append(states, st) })()
		type_(t, s, "b")
		if !s.State().Edited || len(states) == 0 || !states[len(states)-1].Edited {
			t.Fatalf("state %+v, %d notifications", s.State(), len(states))
		}
		if err := finish(t, f, s.Save); err != nil {
			t.Fatal(err)
		}
		if read(t, path) != "ab" || s.State().Edited || s.State().SavedAt.IsZero() {
			t.Errorf("file %q, state %+v", read(t, path), s.State())
		}
		// Undoing back to the saved text is not an edit either way.
		s.Model().Undo()
		if !s.State().Edited {
			t.Error("the text differs from the file, but the state is clean")
		}
		s.Model().Redo()
		if s.State().Edited {
			t.Error("the text is the saved one, but the state is edited")
		}
	})
	t.Run("an edit during a save stays edited", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		path := filepath.Join(f.Dir, "a.md")
		write(t, path, "a")
		s := mustOpen(t, f, m, path)
		type_(t, s, "b")
		saved := 0
		s.Save(func(err error) {
			if err != nil {
				t.Error(err)
			}
			saved++
		})
		type_(t, s, "c")
		f.Settle()
		if saved != 1 || read(t, path) != "ab" || !s.State().Edited {
			t.Errorf("saves %d, file %q, state %+v", saved, read(t, path), s.State())
		}
	})
	t.Run("autosave", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		path := filepath.Join(f.Dir, "a.md")
		write(t, path, "a")
		s := mustOpen(t, f, m, path)
		type_(t, s, "b")
		f.Advance(f.Autosave - time.Millisecond)
		f.Settle()
		if read(t, path) != "a" {
			t.Fatal("saved before the delay")
		}
		type_(t, s, "c")
		f.Advance(f.Autosave - time.Millisecond)
		f.Settle()
		if read(t, path) != "a" {
			t.Fatal("an edit did not restart the delay")
		}
		f.Advance(time.Millisecond)
		f.Settle()
		if read(t, path) != "abc" || s.State().Edited {
			t.Errorf("file %q, state %+v", read(t, path), s.State())
		}
	})
	t.Run("autosave skips untitled and read-only documents", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		n := m.New()
		type_(t, n, "x")
		f.Advance(10 * f.Autosave)
		f.Settle()
		if !n.State().Edited || n.State().Path != "" {
			t.Errorf("untitled %+v", n.State())
		}
		path := filepath.Join(f.Dir, "a.md")
		write(t, path, "a")
		s := mustOpen(t, f, m, path)
		type_(t, s, "b")
		s.Model().SetReadOnly(true)
		f.Advance(10 * f.Autosave)
		f.Settle()
		if read(t, path) != "a" {
			t.Error("a read-only document was saved")
		}
	})
	t.Run("conflict", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		path := filepath.Join(f.Dir, "a.md")
		write(t, path, "a")
		s := mustOpen(t, f, m, path)
		type_(t, s, "mine")
		write(t, path, "their longer text")
		err := finish(t, f, s.Save)
		wantCode(t, err, contract.CodeConflict)
		if read(t, path) != "their longer text" || !s.State().Conflict || !s.State().Edited {
			t.Fatalf("file %q, state %+v", read(t, path), s.State())
		}
		// Autosave stops while in conflict.
		type_(t, s, "more")
		f.Advance(10 * f.Autosave)
		f.Settle()
		if read(t, path) != "their longer text" {
			t.Fatal("autosave wrote over a conflict")
		}
		if err := finish(t, f, s.KeepMine); err != nil {
			t.Fatal(err)
		}
		if read(t, path) != "aminemore" || s.State().Conflict || s.State().Edited {
			t.Fatalf("file %q, state %+v", read(t, path), s.State())
		}
	})
	t.Run("load theirs is one undoable step", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		path := filepath.Join(f.Dir, "a.md")
		write(t, path, "a")
		s := mustOpen(t, f, m, path)
		type_(t, s, "mine")
		write(t, path, "their longer text")
		wantCode(t, finish(t, f, s.Save), contract.CodeConflict)
		if err := finish(t, f, s.LoadTheirs); err != nil {
			t.Fatal(err)
		}
		if s.Model().Text().String() != "their longer text" || s.State().Conflict || s.State().Edited {
			t.Fatalf("text %q, state %+v", s.Model().Text().String(), s.State())
		}
		s.Model().Undo()
		if s.Model().Text().String() != "amine" || !s.State().Edited {
			t.Errorf("text %q, state %+v", s.Model().Text().String(), s.State())
		}
	})
	t.Run("save as", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		s := m.New()
		if s.State().Path != "" || s.State().Name == "" {
			t.Fatalf("untitled %+v", s.State())
		}
		type_(t, s, "x")
		path := filepath.Join(f.Dir, "new.md")
		if err := finish(t, f, func(done func(error)) { s.SaveAs(path, done) }); err != nil {
			t.Fatal(err)
		}
		if read(t, path) != "x" || s.State().Path != path || s.State().Name != "new.md" || s.State().Edited {
			t.Errorf("file %q, state %+v", read(t, path), s.State())
		}
		if found, ok := m.FindByPath(path); !ok || found.ID() != s.ID() {
			t.Error("the path is not registered")
		}
	})
	t.Run("revert", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		path := filepath.Join(f.Dir, "a.md")
		write(t, path, "a")
		s := mustOpen(t, f, m, path)
		type_(t, s, "b")
		if err := finish(t, f, s.Revert); err != nil {
			t.Fatal(err)
		}
		if s.Model().Text().String() != "a" || s.State().Edited {
			t.Errorf("text %q, state %+v", s.Model().Text().String(), s.State())
		}
	})
	t.Run("close", func(t *testing.T) {
		f := newFixture(t)
		m := f.New()
		var events []contract.SessionsEvent
		defer m.Subscribe(func(e contract.SessionsEvent) { events = append(events, e) })()
		n := m.New()
		type_(t, n, "x")
		if n.Close(false) != contract.CloseNeedsPrompt || len(m.All()) != 1 {
			t.Fatal("an unsaved untitled document closed without asking")
		}
		if n.Close(true) != contract.CloseDone || len(m.All()) != 0 {
			t.Fatal("a forced close did not close")
		}
		if len(events) != 2 || events[0].Kind != contract.SessionOpened || events[1].Kind != contract.SessionClosed {
			t.Errorf("events %+v", events)
		}
		clean := m.New()
		if clean.Close(false) != contract.CloseDone {
			t.Error("a clean document asked")
		}
	})
	t.Run("recovery after a crash", func(t *testing.T) {
		f := newFixture(t)
		first := f.New()
		path := filepath.Join(f.Dir, "a.md")
		write(t, path, "a")
		s := mustOpen(t, f, first, path)
		type_(t, s, "b")
		u := first.New()
		type_(t, u, "未命名内容")
		// The recovery copy is written after a pause, not on every keystroke.
		f.Advance(f.Autosave / 2)
		f.Settle()
		// No Close: the process dies here. A new manager starts over the same directory.
		second := f.New()
		got := restore(t, f, second)
		texts := map[string]contract.DocState{}
		for _, r := range got {
			texts[r.Model().Text().String()] = r.State()
		}
		if st, ok := texts["ab"]; !ok || st.Path != path || !st.Edited {
			t.Errorf("named document not restored: %+v", texts)
		}
		if st, ok := texts["未命名内容"]; !ok || st.Path != "" || !st.Edited {
			t.Errorf("untitled document not restored: %+v", texts)
		}
		if read(t, path) != "a" {
			t.Error("restoring wrote to the document")
		}
	})
	t.Run("nothing to recover after a clean finish", func(t *testing.T) {
		f := newFixture(t)
		first := f.New()
		path := filepath.Join(f.Dir, "a.md")
		write(t, path, "a")
		s := mustOpen(t, f, first, path)
		type_(t, s, "b")
		f.Advance(f.Autosave / 2)
		f.Settle()
		if err := finish(t, f, s.Save); err != nil {
			t.Fatal(err)
		}
		u := first.New()
		type_(t, u, "x")
		f.Advance(f.Autosave / 2)
		f.Settle()
		if u.Close(true) != contract.CloseDone {
			t.Fatal("close")
		}
		f.Settle() // Close removes the copy in the background.
		if got := restore(t, f, f.New()); len(got) != 0 {
			t.Errorf("%d sessions restored", len(got))
		}
	})
}
