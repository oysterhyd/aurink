package docsession

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
)

// gatedStore places the external write on either side of the checked write.
type gatedStore struct {
	contract.FileStore
	path    string
	gate    string
	entered chan struct{}
	release chan struct{}
}

type gatedReadStore struct {
	contract.FileStore
	path    string
	entered chan struct{}
	release chan struct{}
}

func (s *gatedReadStore) Read(path string, opts contract.ReadOptions) (contract.FileContent, error) {
	if path == s.path {
		close(s.entered)
		<-s.release
	}
	return s.FileStore.Read(path, opts)
}

func (s *gatedStore) Write(path, text string, meta contract.DocMeta, expect *contract.FileStamp) (contract.FileStamp, error) {
	if path != s.path {
		return s.FileStore.Write(path, text, meta, expect)
	}
	if expect == nil {
		return contract.FileStamp{}, contract.E(contract.CodeInternal, "test.missingExpect", nil)
	}
	if s.gate == "before" {
		close(s.entered)
		<-s.release
	}
	stamp, err := s.FileStore.Write(path, text, meta, expect)
	if s.gate == "after" {
		close(s.entered)
		<-s.release
	}
	return stamp, err
}

func awaitWrite(t *testing.T, h *harness, entered <-chan struct{}) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-entered:
			return
		case <-h.queue.ready:
			h.queue.flush()
		case <-deadline.C:
			t.Fatal("timed out waiting for the document write")
		}
	}
}

func TestAutosaveExternalWriteInterleavings(t *testing.T) {
	for _, tc := range []struct {
		name       string
		gate       string
		delayEvent bool
	}{
		{"external before checked write", "before", false},
		{"external after checked write", "after", false},
		{"external after checked write, delayed event", "after", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			watcher := &contracttest.SnapshotWatcher{}
			h.watcher = watcher
			path := filepath.Join(h.dir, "race.md")
			put(t, path, "base")
			gate := &gatedStore{FileStore: h.files.FileStore, path: path, gate: tc.gate,
				entered: make(chan struct{}), release: make(chan struct{})}
			var release sync.Once
			unblock := func() { release.Do(func() { close(gate.release) }) }
			defer unblock()
			h.files.FileStore = gate
			m := h.manager()
			s := h.open(m, path)
			appendText(t, s, "+mine")
			h.clock.advance(100 * time.Millisecond)
			awaitWrite(t, h, gate.entered)
			put(t, path, "external version")
			if !tc.delayEvent {
				watcher.Flush()
				h.queue.flush()
			}
			unblock()
			h.settle()
			if tc.delayEvent {
				watcher.Flush()
				h.settle()
			}
			if disk := contents(t, path); disk != "external version" {
				t.Fatalf("external text lost: disk %q", disk)
			}
			if !tc.delayEvent {
				if !s.State().Conflict || !s.State().Edited || s.Model().Text().String() != "base+mine" {
					t.Fatalf("expected conflict: state %+v, model %q", s.State(), s.Model().Text().String())
				}
				return
			}
			if s.State().Conflict || s.State().Edited || s.Model().Text().String() != "external version" {
				t.Fatalf("expected reload: state %+v, model %q", s.State(), s.Model().Text().String())
			}
			var found bool
			for _, recovered := range h.restore(h.manager()) {
				found = found || recovered.Model().Text().String() == "base+mine"
			}
			if !found {
				t.Fatalf("local text lost: state %+v, model %q", s.State(), s.Model().Text().String())
			}
		})
	}
}

func TestWatchFailureRechecksDirectory(t *testing.T) {
	h := newHarness(t)
	watcher := &contracttest.SnapshotWatcher{}
	h.watcher = watcher
	path := filepath.Join(h.dir, "gone.md")
	put(t, path, "base")
	s := h.open(h.manager(), path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	watcher.Fail(h.dir, contract.CodeNotFound)
	h.settle()
	if !s.State().Deleted || s.Model().Text().String() != "base" {
		t.Fatalf("watch failure did not recheck the file: %+v", s.State())
	}
}

func TestRecheckRestartsEndedWatch(t *testing.T) {
	h := newHarness(t)
	watcher := &contracttest.SnapshotWatcher{}
	h.watcher = watcher
	path := filepath.Join(h.dir, "a.md")
	put(t, path, "base")
	m := h.manager()
	s := h.open(m, path)
	watcher.Fail(h.dir, contract.CodeIO)
	h.settle()
	m.Recheck()
	h.settle()
	if paths := watcher.Paths(); len(paths) != 1 || paths[0] != h.dir {
		t.Fatalf("watch not restarted: %v", paths)
	}
	put(t, path, "external text")
	watcher.Flush()
	h.settle()
	if s.Model().Text().String() != "external text" {
		t.Fatal("restarted watch missed the change")
	}
}

func TestRecheckDoesNotOverwriteConcurrentEdit(t *testing.T) {
	h := newHarness(t)
	path := filepath.Join(h.dir, "a.md")
	put(t, path, "base")
	m := h.manager()
	s := h.open(m, path)
	gate := &gatedReadStore{FileStore: h.files.FileStore, path: path,
		entered: make(chan struct{}), release: make(chan struct{})}
	var release sync.Once
	unblock := func() { release.Do(func() { close(gate.release) }) }
	defer unblock()
	h.files.FileStore = gate
	put(t, path, "external text")
	m.Recheck()
	awaitWrite(t, h, gate.entered)
	appendText(t, s, "+mine")
	unblock()
	h.settle()
	if !s.State().Conflict || s.Model().Text().String() != "base+mine" || contents(t, path) != "external text" {
		t.Fatalf("concurrent edit was not preserved: %+v, %q", s.State(), s.Model().Text().String())
	}
}

func TestWatcherPostsReadOnlyReload(t *testing.T) {
	h := newHarness(t)
	watcher := &contracttest.SnapshotWatcher{}
	h.watcher = watcher
	path := filepath.Join(h.dir, "a.md")
	put(t, path, "base")
	s := h.open(h.manager(), path)
	s.Model().SetSelection(contract.Caret(3))
	s.Model().SetReadOnly(true)
	put(t, path, "longer external text")
	done := make(chan struct{})
	go func() { watcher.Flush(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("watch callback timed out")
	}
	if s.Model().Text().String() != "base" {
		t.Fatal("watch callback modified the model on its own goroutine")
	}
	h.settle()
	if s.Model().Text().String() != "longer external text" || !s.State().ReadOnly || s.Model().Selection().Primary().Head != 3 {
		t.Fatalf("reload lost the read-only flag or caret: %+v", s.State())
	}
}
