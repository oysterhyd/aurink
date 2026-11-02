package watch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"aurink/internal/contract"
	"github.com/egoist/mygo"
)

var desktopAvailable = runtime.GOOS != "linux" || os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""

func TestMain(m *testing.M) {
	if !desktopAvailable {
		os.Exit(m.Run())
	}
	result := make(chan int, 1)
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.WhenReady(func() { go func() { result <- m.Run(); mygo.App.Quit() }() })
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(<-result)
}

func requireDesktop(t *testing.T) {
	t.Helper()
	if !desktopAvailable {
		t.Skip("MyGo's native watcher requires a desktop session")
	}
}

type notice struct {
	batch []contract.WatchEvent
	err   error
}

func waitFor(t *testing.T, notices <-chan notice, match func(contract.WatchEvent) bool) []contract.WatchEvent {
	t.Helper()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	var seen []contract.WatchEvent
	for {
		select {
		case n := <-notices:
			if n.err != nil {
				t.Fatalf("watch ended: %v", n.err)
			}
			seen = append(seen, n.batch...)
			for _, e := range n.batch {
				if match(e) {
					return seen
				}
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for event; saw %+v", seen)
		}
	}
}

func TestWatchChangesAndClose(t *testing.T) {
	requireDesktop(t)
	dir := t.TempDir()
	notices := make(chan notice, 64)
	w, err := New().Watch(dir, contract.WatchOptions{Recursive: true, Ignore: func(rel string, isDir bool) bool {
		return rel == "ignored" || strings.HasPrefix(rel, "ignored/")
	}}, func(batch []contract.WatchEvent, err error) { notices <- notice{batch, err} })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	file := filepath.Join(dir, "one.md")
	if err := os.WriteFile(file, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, notices, func(e contract.WatchEvent) bool { return e.Op == contract.WatchCreate && e.Path == file })
	if err := os.WriteFile(file, []byte("a longer write"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, notices, func(e contract.WatchEvent) bool { return e.Op == contract.WatchWrite && e.Path == file })
	renamed := filepath.Join(dir, "two.md")
	if err := os.Rename(file, renamed); err != nil {
		t.Fatal(err)
	}
	var removed, created bool
	waitFor(t, notices, func(e contract.WatchEvent) bool {
		removed = removed || e.Op == contract.WatchRemove && e.Path == file
		created = created || e.Op == contract.WatchCreate && e.Path == renamed
		return e.Op == contract.WatchRename && e.OldPath == file && e.Path == renamed || removed && created
	})
	if err := os.Remove(renamed); err != nil {
		t.Fatal(err)
	}
	waitFor(t, notices, func(e contract.WatchEvent) bool { return e.Op == contract.WatchRemove && e.Path == renamed })
	ignored := filepath.Join(dir, "ignored")
	if err := os.Mkdir(ignored, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ignored, "hidden.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "marker.md")
	if err := os.WriteFile(marker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	seen := waitFor(t, notices, func(e contract.WatchEvent) bool { return e.Op == contract.WatchCreate && e.Path == marker })
	for _, e := range seen {
		if strings.HasPrefix(e.Path, ignored) {
			t.Fatalf("ignored event: %+v", e)
		}
	}

	barrier := make(chan notice, 8)
	other, err := New().Watch(dir, contract.WatchOptions{}, func(batch []contract.WatchEvent, err error) { barrier <- notice{batch, err} })
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	w.Close()
	after := filepath.Join(dir, "after-close.md")
	if err := os.WriteFile(after, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, barrier, func(e contract.WatchEvent) bool { return e.Op == contract.WatchCreate && e.Path == after })
	select {
	case n := <-notices:
		t.Fatalf("callback after Close: %+v", n)
	default:
	}
}

func TestCloseWaitsForCallback(t *testing.T) {
	requireDesktop(t)
	dir := t.TempDir()
	entered, release, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	w, err := New().Watch(dir, contract.WatchOptions{}, func(_ []contract.WatchEvent, _ error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(15 * time.Second):
		t.Fatal("callback did not start")
	}
	go func() { w.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned during callback")
	default:
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(15 * time.Second):
		t.Fatal("Close did not finish")
	}
}

func TestWatchLimitAndRemovedRoot(t *testing.T) {
	requireDesktop(t)
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := New().Watch(dir, contract.WatchOptions{MaxEntries: 1}, func([]contract.WatchEvent, error) {}); contract.CodeOf(err) != contract.CodeTooLarge {
		t.Fatalf("limit error = %v", err)
	}
	root := filepath.Join(t.TempDir(), "watched")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	notices := make(chan notice, 8)
	w, err := New().Watch(root, contract.WatchOptions{}, func(batch []contract.WatchEvent, err error) { notices <- notice{batch, err} })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	var removed bool
	for {
		select {
		case n := <-notices:
			for _, e := range n.batch {
				removed = removed || e.Op == contract.WatchRemove && e.Path == root
			}
			if n.err != nil {
				if !removed || contract.CodeOf(n.err) != contract.CodeNotFound {
					t.Fatalf("removed=%v, error=%v", removed, n.err)
				}
				return
			}
		case <-timer.C:
			t.Fatal("root removal did not end watch")
		}
	}
}

func TestWatchPreservesSymlinkPrefix(t *testing.T) {
	requireDesktop(t)
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Skipf("cannot create directory symlink: %v", err)
	}
	notices := make(chan notice, 8)
	w, err := New().Watch(alias, contract.WatchOptions{}, func(batch []contract.WatchEvent, err error) { notices <- notice{batch, err} })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := os.WriteFile(filepath.Join(real, "new.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, notices, func(e contract.WatchEvent) bool {
		return e.Op == contract.WatchCreate && e.Path == filepath.Join(alias, "new.md")
	})
}

func TestWatchPreservesRelativePrefix(t *testing.T) {
	requireDesktop(t)
	dir := t.TempDir()
	t.Chdir(filepath.Dir(dir))
	rel := filepath.Base(dir)
	notices := make(chan notice, 8)
	w, err := New().Watch(rel, contract.WatchOptions{}, func(batch []contract.WatchEvent, err error) { notices <- notice{batch, err} })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := os.WriteFile(filepath.Join(dir, "new.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, notices, func(e contract.WatchEvent) bool {
		return e.Op == contract.WatchCreate && e.Path == filepath.Join(rel, "new.md")
	})
}

func TestWatchWindowsJunction(t *testing.T) {
	requireDesktop(t)
	if runtime.GOOS != "windows" {
		t.Skip("Windows junctions are platform specific")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(dir, "junction")
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", alias, real).CombinedOutput(); err != nil {
		t.Skipf("cannot create junction: %v; %s", err, output)
	}
	notices := make(chan notice, 8)
	w, err := New().Watch(alias, contract.WatchOptions{}, func(batch []contract.WatchEvent, err error) { notices <- notice{batch, err} })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if err := os.WriteFile(filepath.Join(real, "new.md"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	waitFor(t, notices, func(e contract.WatchEvent) bool {
		return e.Op == contract.WatchCreate && e.Path == filepath.Join(alias, "new.md")
	})
}
