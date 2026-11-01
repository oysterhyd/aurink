package docsession

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/core/document"
	"aurink/internal/core/fsstore"
	"aurink/internal/core/settings"
)

func TestPathAliasesAndSaveAsOwnership(t *testing.T) {
	h := newHarness(t)
	m := h.manager()
	path := filepath.Join(h.dir, "a.md")
	put(t, path, "a")
	s := h.open(m, path)
	alias := filepath.Join(h.dir, "unused", "..", "a.md")
	if runtime.GOOS == "windows" {
		alias = strings.ToUpper(alias)
	}
	if other := h.open(m, alias); other.ID() != s.ID() {
		t.Fatal("path alias opened twice")
	}
	appendText(t, s, "b")
	put(t, path, "external modification")
	wantCode(t, h.finish(func(done func(error)) { s.SaveAs(alias, done) }), contract.CodeConflict)
	if contents(t, path) != "external modification" {
		t.Fatal("SaveAs alias bypassed the expected stamp")
	}
	u := m.New()
	appendText(t, u, "untitled")
	wantCode(t, h.finish(func(done func(error)) { u.SaveAs(path, done) }), contract.CodeExists)
	if !u.State().Edited || u.State().Path != "" {
		t.Fatal("failed SaveAs rebound the session")
	}
	newPath := filepath.Join(h.dir, "new.md")
	wantCode(t, h.finish(func(done func(error)) { s.SaveAs(newPath, done) }), "")
	if _, ok := m.FindByPath(path); ok {
		t.Fatal("SaveAs kept the old path registered")
	}
	if contents(t, newPath) != "ab" || s.State().Conflict {
		t.Fatal("SaveAs failed to resolve conflict at a new path")
	}
	if got, ok := m.Get("missing"); ok || got != nil {
		t.Fatal("unknown document found")
	}
	if got, ok := m.FindByPath(""); ok || got != nil {
		t.Fatal("empty path found")
	}
}

func TestEncodingRoundTripsThroughSession(t *testing.T) {
	for _, encoding := range []string{"utf-8", "utf-16le", "utf-16be"} {
		for _, bom := range []bool{false, true} {
			if encoding != "utf-8" && !bom {
				continue
			}
			t.Run(encoding+map[bool]string{false: "plain", true: "BOM"}[bom], func(t *testing.T) {
				h := newHarness(t)
				path := filepath.Join(h.dir, "encoded.md")
				meta := contract.DocMeta{Encoding: encoding, BOM: bom, Dominant: "\r\n", EOL: contract.EOLMixed}
				if _, err := fsstore.New().Write(path, "中\r\nline\n尾", meta, nil); err != nil {
					t.Fatal(err)
				}
				before := []byte(contents(t, path))
				s := h.open(h.manager(), path)
				wantCode(t, h.finish(s.Save), "")
				if !bytes.Equal([]byte(contents(t, path)), before) {
					t.Fatal("an unedited save changed bytes")
				}
			})
		}
	}
}

func TestReadOnlyAndClosedOperations(t *testing.T) {
	h := newHarness(t)
	m := h.manager()
	u := m.New()
	wantCode(t, h.finish(u.Save), contract.CodeInvalid)
	wantCode(t, h.finish(u.Revert), contract.CodeInvalid)
	wantCode(t, h.finish(func(done func(error)) { u.SaveAs("", done) }), contract.CodeInvalid)
	path := filepath.Join(h.dir, "a.md")
	put(t, path, "a")
	s := h.open(m, path)
	appendText(t, s, "b")
	s.Model().SetReadOnly(true)
	wantCode(t, h.finish(s.Save), contract.CodeReadOnly)
	wantCode(t, h.finish(s.Revert), contract.CodeReadOnly)
	if !s.State().ReadOnly || !s.State().Edited {
		t.Fatal("read-only state lost")
	}
	s.Model().SetReadOnly(false)
	s.LoadTheirs(func(err error) { wantCode(t, err, contract.CodeCanceled) })
	s.Close(true)
	h.settle()
	wantCode(t, h.finish(s.Save), contract.CodeCanceled)
	called := false
	m.Open("", func(got contract.Session, err error) {
		called = true
		wantCode(t, err, contract.CodeInvalid)
	})
	if called {
		t.Fatal("invalid Open called back without dispatch")
	}
	h.settle()
	if !called {
		t.Fatal("missing invalid Open callback")
	}
}

func TestAutosaveDebouncesPerSessionAndQueuesDuringSave(t *testing.T) {
	h := newHarness(t)
	m := h.manager()
	aPath, bPath := filepath.Join(h.dir, "a.md"), filepath.Join(h.dir, "b.md")
	put(t, aPath, "a")
	put(t, bPath, "b")
	a, b := h.open(m, aPath), h.open(m, bPath)
	appendText(t, a, "1")
	h.advance(50 * time.Millisecond)
	appendText(t, b, "2")
	h.advance(50 * time.Millisecond)
	if contents(t, aPath) != "a1" || contents(t, bPath) != "b" {
		t.Fatal("session debounce timers interfered")
	}
	h.advance(50 * time.Millisecond)
	if contents(t, bPath) != "b2" {
		t.Fatal("second session did not autosave")
	}
	appendText(t, a, "3")
	h.files.failPath, h.files.entered, h.files.release = aPath, make(chan struct{}, 4), make(chan struct{})
	a.Save(nil)
	<-h.files.entered
	appendText(t, a, "4")
	h.clock.advance(100 * time.Millisecond)
	h.queue.flush()
	if a.(*session).saves != 2 {
		t.Fatal("autosave was not queued behind the active save")
	}
	close(h.files.release)
	h.settle()
	if contents(t, aPath) != "a134" || a.State().Edited || a.State().Saving {
		t.Fatal("queued autosave did not use its snapshot and the new base stamp")
	}
	h.files.mu.Lock()
	defer h.files.mu.Unlock()
	counts := map[string]int{}
	for _, path := range h.files.writes {
		counts[path]++
	}
	if counts[aPath] != 3 || counts[bPath] != 1 {
		t.Fatal("unexpected document write counts", counts)
	}
}

func TestRecoveryRawBytesAndDeletedSource(t *testing.T) {
	h := newHarness(t)
	h.settings.Files.Autosave = false
	m := h.manager()
	u := m.New()
	appendText(t, u, "\xff\x00\r\n")
	path := filepath.Join(h.dir, "gone.md")
	put(t, path, "original")
	s := h.open(m, path)
	appendText(t, s, " edited")
	h.advance(50 * time.Millisecond)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	got := h.restore(h.manager())
	if len(got) != 2 {
		t.Fatal("recovered session count", len(got))
	}
	for _, r := range got {
		if r.State().Path == "" {
			if r.Model().Text().String() != "\xff\x00\r\n" || !r.State().Edited {
				t.Fatal("recovery did not preserve raw bytes")
			}
		} else if !r.State().Deleted || !r.State().Conflict || r.Model().Text().String() != "original edited" {
			t.Fatal("missing source not marked deleted and conflicted", r.State())
		}
	}
}

func TestReentrantEditsAndCallbackPanics(t *testing.T) {
	h := newHarness(t)
	m := h.manager()
	s := m.New()
	once := false
	s.OnState(func(st contract.DocState) {
		if !once && st.Edited {
			once = true
			appendText(t, s, "b")
			appendText(t, s, "c")
		}
	})
	appendText(t, s, "a")
	if s.Model().Text().String() != "abc" || s.(*session).hash != fingerprint("abc") {
		t.Fatal("reentrant events corrupted the fingerprint")
	}
	path := filepath.Join(h.dir, "a.md")
	wantCode(t, h.finish(func(done func(error)) { s.SaveAs(path, done) }), "")
	if s.State().Edited {
		t.Fatal("reentrant text did not become clean")
	}
	s.Save(func(error) { panic("expected completion panic") })
	h.settle()
	m.Open(path, func(contract.Session, error) { panic("expected open panic") })
	h.settle()
	s.Close(true)
	m.Restore(func([]contract.Session, error) { panic("expected restore panic") })
	h.settle()
}

type rawFailureStore struct{ contract.FileStore }

func (rawFailureStore) Read(string, contract.ReadOptions) (contract.FileContent, error) {
	return contract.FileContent{}, errors.New("read failure")
}

func TestForeignErrorIsWrapped(t *testing.T) {
	h := newHarness(t)
	m := New(rawFailureStore{h.files}, h.factory, h.queue, h.recovery, Options{AfterFunc: h.clock.after})
	h.managers = append(h.managers, m)
	called := false
	m.Open(filepath.Join(h.dir, "a.md"), func(s contract.Session, err error) {
		called = true
		wantCode(t, err, contract.CodeIO)
	})
	h.settle()
	if !called {
		t.Fatal("missing callback")
	}
}

type changedTargetStore struct{ contract.FileStore }

func (s changedTargetStore) Write(path, text string, meta contract.DocMeta, expect *contract.FileStamp) (contract.FileStamp, error) {
	if strings.HasSuffix(path, ".md") {
		if err := os.WriteFile(path, []byte("another writer changed the target"), 0o600); err != nil {
			return contract.FileStamp{}, err
		}
	}
	return s.FileStore.Write(path, text, meta, expect)
}

func TestSaveAsChecksExistingTargetStamp(t *testing.T) {
	h := newHarness(t)
	m := New(changedTargetStore{h.files}, h.factory, h.queue, h.recovery, Options{AfterFunc: h.clock.after})
	h.managers = append(h.managers, m)
	s := m.New()
	appendText(t, s, "mine")
	path := filepath.Join(h.dir, "target.md")
	put(t, path, "target")
	wantCode(t, h.finish(func(done func(error)) { s.SaveAs(path, done) }), contract.CodeConflict)
	if contents(t, path) != "another writer changed the target" || !s.State().Edited || s.State().Path != "" {
		t.Fatal("SaveAs overwrote a changed target or changed its binding")
	}
}

type regressionDispatcher chan func()

func (d regressionDispatcher) Post(fn func()) { d <- fn }

type regressionTimer struct{ fn func() }

func (t *regressionTimer) Stop() bool {
	t.fn = nil
	return true
}

// A restored copy is not written by autosave until the user edits it.
func TestRestoredCopyWaitsForAnEditBeforeAutosave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	if err := os.WriteFile(path, []byte("base"), 0o644); err != nil {
		t.Fatal(err)
	}
	var timers []*regressionTimer
	start := func(auto bool) (*Manager, func()) {
		d := make(regressionDispatcher, 256)
		value := settings.Defaults()
		value.Files.Autosave = auto
		m := New(fsstore.New(), document.Factory{}, d, filepath.Join(dir, "recovery"), Options{Settings: &value,
			AfterFunc: func(_ time.Duration, fn func()) Timer {
				timer := &regressionTimer{fn: fn}
				timers = append(timers, timer)
				return timer
			}})
		return m, func() {
			for {
				for m.busy || len(d) > 0 {
					(<-d)()
				}
				due := timers
				timers = nil
				fired := false
				for _, timer := range due {
					if timer.fn != nil {
						timer.fn()
						fired = true
					}
				}
				if !fired && len(d) == 0 && !m.busy {
					return
				}
			}
		}
	}
	m, settle := start(false)
	var s contract.Session
	m.Open(path, func(x contract.Session, err error) {
		if err != nil {
			t.Fatal(err)
		}
		s = x
	})
	settle()
	edit := func(s contract.Session, at int, text string) {
		t.Helper()
		if _, err := s.Model().Apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: at, End: at}, Text: text}}, Origin: contract.OriginUser}); err != nil {
			t.Fatal(err)
		}
	}
	edit(s, 4, " unsaved")
	settle()

	m, settle = start(true)
	var restored []contract.Session
	m.Restore(func(x []contract.Session, err error) {
		if err != nil {
			t.Fatal(err)
		}
		restored = x
	})
	settle()
	if len(restored) != 1 || !restored[0].State().Edited {
		t.Fatalf("restored %d sessions", len(restored))
	}
	if got, _ := os.ReadFile(path); string(got) != "base" {
		t.Fatalf("restoring wrote %q", got)
	}
	edit(restored[0], 0, "x")
	settle()
	if got, _ := os.ReadFile(path); string(got) != "xbase unsaved" {
		t.Fatalf("after an edit the file is %q", got)
	}
}
