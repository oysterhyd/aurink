package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
)

type queue struct {
	mu    sync.Mutex
	fns   []func()
	ready chan struct{}
}

func newQueue() *queue { return &queue{ready: make(chan struct{}, 1)} }

func (q *queue) Post(fn func()) {
	q.mu.Lock()
	q.fns = append(q.fns, fn)
	q.mu.Unlock()
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

func (q *queue) flush() {
	for {
		q.mu.Lock()
		if len(q.fns) == 0 {
			q.mu.Unlock()
			return
		}
		fn := q.fns[0]
		q.fns = q.fns[1:]
		q.mu.Unlock()
		fn()
	}
}

type fixture struct {
	t       *testing.T
	q       *queue
	spaces  []*Workspace
	recents []*Recent
}

func (f *fixture) settle() {
	f.t.Helper()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for {
		f.q.flush()
		busy := false
		for _, w := range f.spaces {
			busy = busy || w.busy || len(w.jobs) != 0
		}
		for _, r := range f.recents {
			busy = busy || r.busy || r.pending != nil
		}
		if !busy {
			return
		}
		select {
		case <-f.q.ready:
		case <-deadline.C:
			f.t.Fatal("background work did not settle")
		}
	}
}

func TestWorkspaceContract(t *testing.T) {
	contracttest.TestWorkspace(t, func(t *testing.T) contracttest.WorkspaceFixture {
		f := &fixture{t: t, q: newQueue()}
		watcher, trash := &contracttest.SnapshotWatcher{}, &contracttest.RecordingTrasher{}
		t.Cleanup(func() {
			for _, w := range f.spaces {
				w.Close(nil)
			}
			f.settle()
		})
		return contracttest.WorkspaceFixture{Dir: t.TempDir(), Watcher: watcher, Trash: trash, Settle: f.settle,
			New: func(maxEntries int) contract.Workspace {
				w := New(watcher, trash, f.q, Options{MaxEntries: maxEntries})
				f.spaces = append(f.spaces, w)
				return w
			}}
	})
}

func TestAssetsContract(t *testing.T) {
	contracttest.TestAssets(t, func(t *testing.T) contracttest.AssetsFixture {
		return contracttest.AssetsFixture{Dir: t.TempDir(), New: func(dir string, copy bool) contract.Assets {
			return NewAssets(dir, copy)
		}}
	})
}

func TestRecentContract(t *testing.T) {
	contracttest.TestRecent(t, func(t *testing.T) contracttest.RecentFixture {
		f := &fixture{t: t, q: newQueue()}
		dir := t.TempDir()
		return contracttest.RecentFixture{Dir: dir, Settle: f.settle, New: func() contract.Recent {
			r := NewRecent(filepath.Join(dir, "recent.json"), f.q)
			f.recents = append(f.recents, r)
			return r
		}}
	})
}

func TestMovePreflightAndDegradedWatch(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"src", "dst", "loaded"} {
		if err := os.Mkdir(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"src/a.md", "src/b.md", "dst/b.md"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(file)), []byte(file), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f := &fixture{t: t, q: newQueue()}
	watcher := &contracttest.SnapshotWatcher{}
	w := New(watcher, &contracttest.RecordingTrasher{}, f.q, Options{})
	f.spaces = append(f.spaces, w)
	t.Cleanup(func() { w.Close(nil); f.settle() })
	var events []contract.WorkspaceEvent
	w.Subscribe(func(e contract.WorkspaceEvent) { events = append(events, e) })
	w.SetRoot(root, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
	f.settle()
	w.List(filepath.Join(root, "loaded"))
	f.settle()
	var moveErr error
	w.Move([]string{filepath.Join(root, "src", "a.md"), filepath.Join(root, "src", "b.md")}, filepath.Join(root, "dst"), func(err error) { moveErr = err })
	f.settle()
	if contract.CodeOf(moveErr) != contract.CodeExists {
		t.Fatalf("move error = %v", moveErr)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "a.md")); err != nil {
		t.Fatalf("first source moved on clash: %v", err)
	}
	watcher.Fail(root, contract.CodeTooLarge)
	f.settle()
	w.watchResult(watchResult{gen: w.generation, err: contract.E(contract.CodeTooLarge, "watch", nil)})
	f.settle()
	var degraded int
	for _, e := range events {
		if e.Kind == contract.WsDegraded {
			degraded++
		}
	}
	if degraded != 1 {
		t.Fatalf("got %d degradation events", degraded)
	}
	if got, want := watcher.Paths(), []string{root, filepath.Join(root, "loaded")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("watches %q, want %q", got, want)
	}
}

type blockedWatcher struct {
	contract.FileWatcher
	path    string
	entered chan struct{}
	release chan struct{}
}

type rejectFallbackWatcher struct {
	contract.FileWatcher
	path string
}

func (w rejectFallbackWatcher) Watch(path string, opts contract.WatchOptions, fn func([]contract.WatchEvent, error)) (contract.Watch, error) {
	if path == w.path && !opts.Recursive {
		return nil, contract.E(contract.CodeIO, "watch", nil)
	}
	return w.FileWatcher.Watch(path, opts, fn)
}

func TestFailedFallbackKeepsPreviousRoot(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	for _, name := range []string{"a.md", "b.md"} {
		if err := os.WriteFile(filepath.Join(second, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f := &fixture{t: t, q: newQueue()}
	snapshot := &contracttest.SnapshotWatcher{}
	w := New(rejectFallbackWatcher{snapshot, second}, &contracttest.RecordingTrasher{}, f.q, Options{MaxEntries: 1})
	f.spaces = append(f.spaces, w)
	t.Cleanup(func() { w.Close(nil); f.settle() })
	w.SetRoot(first, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
	f.settle()
	var result error
	w.SetRoot(second, func(err error) { result = err })
	f.settle()
	if contract.CodeOf(result) != contract.CodeIO || w.Root() != first {
		t.Fatalf("root=%q, error=%v", w.Root(), result)
	}
	if got, want := snapshot.Paths(), []string{first}; !reflect.DeepEqual(got, want) {
		t.Fatalf("watches = %q, want %q", got, want)
	}
}

func (w *blockedWatcher) Watch(path string, opts contract.WatchOptions, fn func([]contract.WatchEvent, error)) (contract.Watch, error) {
	if path == w.path {
		close(w.entered)
		<-w.release
	}
	return w.FileWatcher.Watch(path, opts, fn)
}

func TestRootChangeDiscardsQueuedOldListing(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	oldDir := filepath.Join(first, "old")
	if err := os.Mkdir(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, q: newQueue()}
	watcher := &blockedWatcher{FileWatcher: &contracttest.SnapshotWatcher{}, path: second, entered: make(chan struct{}), release: make(chan struct{})}
	w := New(watcher, &contracttest.RecordingTrasher{}, f.q, Options{})
	f.spaces = append(f.spaces, w)
	t.Cleanup(func() { w.Close(nil); f.settle() })
	var events []contract.WorkspaceEvent
	w.Subscribe(func(event contract.WorkspaceEvent) { events = append(events, event) })
	w.SetRoot(first, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
	f.settle()
	w.SetRoot(second, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
	select {
	case <-watcher.entered:
	case <-time.After(20 * time.Second):
		t.Fatal("second watch did not start")
	}
	w.List(oldDir)
	close(watcher.release)
	f.settle()
	for _, event := range events {
		if event.Kind == contract.WsListed && event.Path == oldDir {
			t.Fatalf("listed old root after change: %+v", event)
		}
	}
	if _, loaded := w.cache[oldDir]; loaded {
		t.Fatal("old directory remained cached")
	}
}

func TestDegradedRenameMovesLoadedWatches(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "dir", "sub")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "a.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, q: newQueue()}
	watcher := &contracttest.SnapshotWatcher{}
	w := New(watcher, &contracttest.RecordingTrasher{}, f.q, Options{MaxEntries: 2})
	f.spaces = append(f.spaces, w)
	t.Cleanup(func() { w.Close(nil); f.settle() })
	w.SetRoot(root, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
	f.settle()
	for _, dir := range []string{root, filepath.Join(root, "dir"), child} {
		w.List(dir)
		f.settle()
	}
	var renameErr error
	w.Rename(filepath.Join(root, "dir"), "moved", func(_ string, err error) { renameErr = err })
	f.settle()
	if renameErr != nil {
		t.Fatal(renameErr)
	}
	moved := filepath.Join(root, "moved")
	movedChild := filepath.Join(moved, "sub")
	if got, want := watcher.Paths(), []string{root, moved, movedChild}; !reflect.DeepEqual(got, want) {
		t.Fatalf("watches after rename = %q, want %q", got, want)
	}
	if err := os.WriteFile(filepath.Join(movedChild, "new.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	watcher.Flush()
	f.settle()
	entries, loaded := w.List(movedChild)
	if !loaded || len(entries) != 2 || entries[1].Name != "new.md" {
		t.Fatalf("moved listing = %+v, loaded=%v", entries, loaded)
	}
}

func TestNaturalOrderWithLeadingZeros(t *testing.T) {
	if !naturalLess("x0001", "x1z") || naturalLess("x1z", "x0001") {
		t.Fatal("numeric equality must compare the remaining suffix")
	}
}

type smallDefaultWatcher struct{ contract.FileWatcher }

func (w smallDefaultWatcher) Watch(path string, opts contract.WatchOptions, fn func([]contract.WatchEvent, error)) (contract.Watch, error) {
	if opts.MaxEntries == 0 {
		opts.MaxEntries = 2
	}
	return w.FileWatcher.Watch(path, opts, fn)
}

func TestDegradedFlatDirectoryAndOverflowNotifications(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.md", "b.md", "c.md"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	f := &fixture{t: t, q: newQueue()}
	watcher := &contracttest.SnapshotWatcher{}
	w := New(smallDefaultWatcher{watcher}, &contracttest.RecordingTrasher{}, f.q, Options{})
	f.spaces = append(f.spaces, w)
	t.Cleanup(func() { w.Close(nil); f.settle() })
	var events []contract.WorkspaceEvent
	w.Subscribe(func(event contract.WorkspaceEvent) { events = append(events, event) })
	w.SetRoot(root, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
	f.settle()
	for _, dir := range []string{root, sub} {
		w.List(dir)
		f.settle()
	}
	if got, want := watcher.Paths(), []string{root, sub}; !reflect.DeepEqual(got, want) {
		t.Fatalf("watches = %q, want %q", got, want)
	}
	newPath := filepath.Join(root, "new.md")
	if err := os.WriteFile(newPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	watcher.Flush()
	f.settle()
	entries, _ := w.List(root)
	if len(entries) != 5 {
		t.Fatalf("listing after create = %+v", entries)
	}
	events = nil
	watcher.Overflow()
	f.settle()
	listed := make(map[string]bool)
	for _, event := range events {
		if event.Kind == contract.WsListed {
			listed[event.Path] = true
		}
	}
	if !listed[root] || !listed[sub] {
		t.Fatalf("overflow listings = %+v", events)
	}
}

type blockedAfterWatcher struct {
	contract.FileWatcher
	path    string
	entered chan struct{}
	release chan struct{}
}

func (w *blockedAfterWatcher) Watch(path string, opts contract.WatchOptions, fn func([]contract.WatchEvent, error)) (contract.Watch, error) {
	watch, err := w.FileWatcher.Watch(path, opts, fn)
	if path == w.path && err == nil {
		close(w.entered)
		<-w.release
	}
	return watch, err
}

func TestDegradedListingStartsWatchBeforeReading(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, q: newQueue()}
	watcher := &blockedAfterWatcher{FileWatcher: &contracttest.SnapshotWatcher{}, path: sub, entered: make(chan struct{}), release: make(chan struct{})}
	w := New(watcher, &contracttest.RecordingTrasher{}, f.q, Options{MaxEntries: 1})
	f.spaces = append(f.spaces, w)
	t.Cleanup(func() { w.Close(nil); f.settle() })
	w.SetRoot(root, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
	f.settle()
	w.List(sub)
	select {
	case <-watcher.entered:
	case <-time.After(20 * time.Second):
		t.Fatal("directory watch did not start")
	}
	if err := os.WriteFile(filepath.Join(sub, "during.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	close(watcher.release)
	f.settle()
	entries, loaded := w.List(sub)
	if !loaded || len(entries) != 1 || entries[0].Name != "during.md" {
		t.Fatalf("listing = %+v, loaded=%v", entries, loaded)
	}
}
