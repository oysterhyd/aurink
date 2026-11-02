package workspace_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"aurink/internal/adapters/watch"
	"aurink/internal/contract"
	"aurink/internal/core/workspace"
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

type dispatcher struct {
	mu    sync.Mutex
	fns   []func()
	ready chan struct{}
}

func (d *dispatcher) Post(fn func()) {
	d.mu.Lock()
	d.fns = append(d.fns, fn)
	d.mu.Unlock()
	select {
	case d.ready <- struct{}{}:
	default:
	}
}

func (d *dispatcher) flush() {
	for {
		d.mu.Lock()
		if len(d.fns) == 0 {
			d.mu.Unlock()
			return
		}
		fn := d.fns[0]
		d.fns = d.fns[1:]
		d.mu.Unlock()
		fn()
	}
}

func (d *dispatcher) until(t *testing.T, condition func() bool) {
	t.Helper()
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	for {
		d.flush()
		if condition() {
			return
		}
		select {
		case <-d.ready:
		case <-timer.C:
			t.Fatal("timed out waiting for workspace")
		}
	}
}

func TestExternalChanges(t *testing.T) {
	if !desktopAvailable {
		t.Skip("MyGo's native watcher requires a desktop session")
	}
	root := t.TempDir()
	d := &dispatcher{ready: make(chan struct{}, 1)}
	w := workspace.New(watch.New(), nil, d, workspace.Options{})
	t.Cleanup(func() {
		closed := false
		w.Close(func() { closed = true })
		d.until(t, func() bool { return closed })
	})
	var events []contract.WorkspaceEvent
	w.Subscribe(func(e contract.WorkspaceEvent) { events = append(events, e) })
	rootDone := false
	var rootErr error
	w.SetRoot(root, func(err error) { rootErr = err; rootDone = true })
	d.until(t, func() bool { return rootDone })
	if rootErr != nil {
		t.Fatal(rootErr)
	}
	w.List(root)
	d.until(t, func() bool { _, loaded := w.List(root); return loaded })
	hasEvent := func(kind contract.WorkspaceEventKind, path string) bool {
		for _, e := range events {
			if e.Kind == kind && e.Path == path {
				return true
			}
		}
		return false
	}
	hasName := func(name string) bool {
		entries, loaded := w.List(root)
		if !loaded {
			return false
		}
		for _, e := range entries {
			if e.Name == name {
				return true
			}
		}
		return false
	}
	one := filepath.Join(root, "one.md")
	if err := os.WriteFile(one, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	d.until(t, func() bool { return hasEvent(contract.WsCreated, one) && hasName("one.md") })
	two := filepath.Join(root, "two.md")
	if err := os.Rename(one, two); err != nil {
		t.Fatal(err)
	}
	d.until(t, func() bool {
		renamed := false
		for _, e := range events {
			renamed = renamed || e.Kind == contract.WsRenamed && e.OldPath == one && e.Path == two
		}
		return (renamed || hasEvent(contract.WsRemoved, one) && hasEvent(contract.WsCreated, two)) && !hasName("one.md") && hasName("two.md")
	})
	if err := os.Remove(two); err != nil {
		t.Fatal(err)
	}
	d.until(t, func() bool { return hasEvent(contract.WsRemoved, two) && !hasName("two.md") })
}
