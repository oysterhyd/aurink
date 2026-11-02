package contracttest

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"aurink/internal/contract"
)

func TestSnapshotWatcher(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.md"), "a")
	write(t, filepath.Join(dir, "sub", "b.md"), "b")
	write(t, filepath.Join(dir, "skip", "c.md"), "c")
	var w SnapshotWatcher
	var got []contract.WatchEvent
	var ended error
	watch, err := w.Watch(dir, contract.WatchOptions{Recursive: true, Ignore: func(rel string, isDir bool) bool { return rel == "skip" }},
		func(batch []contract.WatchEvent, err error) { got = append(got, batch...); ended = err })
	if err != nil {
		t.Fatal(err)
	}
	w.Flush()
	if len(got) != 0 {
		t.Fatalf("events with no change: %+v", got)
	}
	write(t, filepath.Join(dir, "a.md"), "longer")
	write(t, filepath.Join(dir, "sub", "new.md"), "n")
	write(t, filepath.Join(dir, "skip", "d.md"), "d")
	if err := os.Remove(filepath.Join(dir, "sub", "b.md")); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	want := []contract.WatchEvent{
		{Op: contract.WatchWrite, Path: filepath.Join(dir, "a.md")},
		{Op: contract.WatchRemove, Path: filepath.Join(dir, "sub", "b.md")},
		{Op: contract.WatchCreate, Path: filepath.Join(dir, "sub", "new.md")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if _, err := w.Watch(dir, contract.WatchOptions{Recursive: true, MaxEntries: 2}, nil); contract.CodeOf(err) != contract.CodeTooLarge {
		t.Fatalf("limit: %v", err)
	}
	if _, err := w.Watch(filepath.Join(dir, "none"), contract.WatchOptions{}, nil); contract.CodeOf(err) != contract.CodeNotFound {
		t.Fatalf("missing: %v", err)
	}
	if !reflect.DeepEqual(w.Paths(), []string{filepath.Clean(dir)}) {
		t.Fatalf("paths %q", w.Paths())
	}
	w.Fail(dir, contract.CodeNotFound)
	if contract.CodeOf(ended) != contract.CodeNotFound || len(w.Paths()) != 0 {
		t.Fatalf("Fail: %v, %q", ended, w.Paths())
	}
	watch.Close()
}
