package workspace

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
)

type listingNotice struct {
	event   contract.WorkspaceEvent
	entries []contract.Entry
	loaded  bool
}

func recordListings(w *Workspace) *[]listingNotice {
	var notices []listingNotice
	w.Subscribe(func(event contract.WorkspaceEvent) {
		notice := listingNotice{event: event}
		if event.Kind == contract.WsListed {
			notice.entries, notice.loaded = w.List(event.Path)
		}
		notices = append(notices, notice)
	})
	return &notices
}

func checkFiles(t *testing.T, entries []contract.Entry, loaded, showAll bool) {
	t.Helper()
	if !loaded {
		t.Fatal("directory is not loaded")
	}
	var files []string
	for _, entry := range entries {
		if !entry.IsDir {
			files = append(files, entry.Name)
		}
	}
	want := []string{"read.md"}
	if showAll {
		want = []string{"extra.txt", "read.md"}
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %q, want %q", files, want)
	}
}

func checkListingNotices(t *testing.T, notices []listingNotice, dirs []string, showAll bool) {
	t.Helper()
	if len(notices) != len(dirs) {
		t.Fatalf("got %d notifications, want %d: %+v", len(notices), len(dirs), notices)
	}
	want := make(map[string]bool, len(dirs))
	for _, dir := range dirs {
		want[dir] = true
	}
	for _, notice := range notices {
		if notice.event.Kind != contract.WsListed || !notice.event.IsDir || !want[notice.event.Path] {
			t.Fatalf("unexpected listing notification: %+v", notice.event)
		}
		delete(want, notice.event.Path)
		checkFiles(t, notice.entries, notice.loaded, showAll)
	}
}

func makeListingDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"read.md", "extra.txt", ".hidden.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSetShowAllLoadedAndLazyDirectories(t *testing.T) {
	for _, tt := range []struct {
		name    string
		initial bool
	}{{"initially filtered", false}, {"initially all", true}} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			child := filepath.Join(root, "child")
			grandchild := filepath.Join(child, "grandchild")
			later := filepath.Join(root, "later")
			dirs := []string{root, child, grandchild}
			for _, dir := range append(append([]string(nil), dirs...), later) {
				makeListingDir(t, dir)
			}
			f := &fixture{t: t, q: newQueue()}
			w := New(&contracttest.SnapshotWatcher{}, &contracttest.RecordingTrasher{}, f.q, Options{ShowAll: tt.initial})
			f.spaces = append(f.spaces, w)
			t.Cleanup(func() { w.Close(nil); f.settle() })
			notices := recordListings(w)
			w.SetRoot(root, func(err error) {
				if err != nil {
					t.Fatal(err)
				}
			})
			f.settle()
			for _, dir := range dirs {
				w.List(dir)
				f.settle()
				entries, loaded := w.List(dir)
				checkFiles(t, entries, loaded, tt.initial)
			}
			*notices = nil
			for _, step := range []struct {
				name    string
				value   bool
				changed bool
			}{{"same initial value", tt.initial, false}, {"toggle", !tt.initial, true}, {"same new value", !tt.initial, false}} {
				if !t.Run(step.name, func(t *testing.T) {
					w.SetShowAll(step.value)
					f.settle()
					var notified []string
					if step.changed {
						notified = dirs
					}
					checkListingNotices(t, *notices, notified, step.value)
					*notices = nil
				}) {
					return
				}
			}
			if _, loaded := w.List(later); loaded {
				t.Fatal("lazy directory was already loaded")
			}
			f.settle()
			checkListingNotices(t, *notices, []string{later}, !tt.initial)
			*notices = nil
			w.SetShowAll(tt.initial)
			f.settle()
			checkListingNotices(t, *notices, append(dirs, later), tt.initial)
		})
	}
}

func TestSetShowAllBeforeSetRoot(t *testing.T) {
	for _, tt := range []struct {
		name    string
		initial bool
	}{{"enable", false}, {"disable", true}} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			makeListingDir(t, root)
			f := &fixture{t: t, q: newQueue()}
			w := New(&contracttest.SnapshotWatcher{}, &contracttest.RecordingTrasher{}, f.q, Options{ShowAll: tt.initial})
			f.spaces = append(f.spaces, w)
			t.Cleanup(func() { w.Close(nil); f.settle() })
			notices := recordListings(w)
			w.SetShowAll(!tt.initial)
			f.settle()
			if len(*notices) != 0 {
				t.Fatalf("notifications before SetRoot: %+v", *notices)
			}
			w.SetRoot(root, func(err error) {
				if err != nil {
					t.Fatal(err)
				}
			})
			f.settle()
			*notices = nil
			w.List(root)
			f.settle()
			checkListingNotices(t, *notices, []string{root}, !tt.initial)
		})
	}
}

func TestSetShowAllDuringLoad(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	makeListingDir(t, child)
	f := &fixture{t: t, q: newQueue()}
	watcher := &blockedAfterWatcher{FileWatcher: &contracttest.SnapshotWatcher{}, path: child, entered: make(chan struct{}), release: make(chan struct{})}
	released := false
	defer func() {
		if !released {
			close(watcher.release)
		}
	}()
	w := New(watcher, &contracttest.RecordingTrasher{}, f.q, Options{MaxEntries: 1})
	f.spaces = append(f.spaces, w)
	t.Cleanup(func() { w.Close(nil); f.settle() })
	notices := recordListings(w)
	w.SetRoot(root, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	})
	f.settle()
	*notices = nil
	w.List(child)
	select {
	case <-watcher.entered:
	case <-time.After(20 * time.Second):
		t.Fatal("directory load did not start")
	}
	w.SetShowAll(true)
	close(watcher.release)
	released = true
	f.settle()
	checkListingNotices(t, *notices, []string{child}, true)
}
