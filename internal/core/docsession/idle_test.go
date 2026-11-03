package docsession

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func blockIdleWrite(t *testing.T, h *harness, path string) func() {
	t.Helper()
	h.files.failPath, h.files.entered, h.files.release = path, make(chan struct{}, 1), make(chan struct{})
	released := false
	release := func() {
		if !released {
			close(h.files.release)
			released = true
		}
	}
	t.Cleanup(release)
	return release
}

func waitIdle(t *testing.T, h *harness, done func() bool) {
	t.Helper()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for !done() {
		h.queue.flush()
		if done() {
			return
		}
		select {
		case <-h.queue.ready:
		case <-deadline.C:
			t.Fatal("timed out waiting for idle")
		}
	}
}

func TestIdleWaitsForSave(t *testing.T) {
	h := newHarness(t)
	m := h.manager()
	s := m.New()
	appendText(t, s, "saved text")
	path := filepath.Join(h.dir, "saved.md")
	release := blockIdleWrite(t, h, path)

	saved, idle := false, false
	s.SaveAs(path, func(err error) {
		if err != nil {
			t.Errorf("save: %v", err)
		}
		saved = true
	})
	awaitWrite(t, h, h.files.entered)
	m.Idle(func() {
		if goroutineID() != h.files.main {
			t.Error("idle callback ran off the dispatcher thread")
		}
		if !saved {
			t.Error("idle callback ran before the save completion")
		}
		idle = true
	})
	h.queue.flush()
	if idle || saved {
		t.Fatal("idle or save completed while the write was blocked")
	}
	release()
	waitIdle(t, h, func() bool { return idle })
	if got := contents(t, path); got != "saved text" {
		t.Fatalf("saved text %q", got)
	}
}

func TestIdleWaitsForRecoveryWrite(t *testing.T) {
	h := newHarness(t)
	m := h.manager()
	s := m.New()
	appendText(t, s, "recovery text")
	path := s.(*session).copyPath()
	release := blockIdleWrite(t, h, path)

	h.clock.advance(50 * time.Millisecond)
	h.queue.flush()
	awaitWrite(t, h, h.files.entered)
	idle := false
	m.Idle(func() {
		if goroutineID() != h.files.main {
			t.Error("idle callback ran off the dispatcher thread")
		}
		idle = true
	})
	h.queue.flush()
	if idle {
		t.Fatal("idle completed while the recovery write was blocked")
	}
	release()
	waitIdle(t, h, func() bool { return idle })
	var copy recoveryCopy
	if err := json.Unmarshal([]byte(contents(t, path)), &copy); err != nil {
		t.Fatal(err)
	}
	if string(copy.Text) != "recovery text" {
		t.Fatalf("recovery text %q", copy.Text)
	}
}

func TestIdleWithNothingQueued(t *testing.T) {
	h := newHarness(t)
	m := h.manager()
	var calls []int
	m.Idle(func() {
		if goroutineID() != h.files.main {
			t.Error("idle callback ran off the dispatcher thread")
		}
		calls = append(calls, 1)
	})
	m.Idle(func() { calls = append(calls, 2) })
	if len(calls) != 0 {
		t.Fatal("idle callback ran synchronously")
	}
	h.queue.flush()
	if !reflect.DeepEqual(calls, []int{1, 2}) {
		t.Fatalf("idle callbacks were not posted in order: %v", calls)
	}
}

func TestIdleCallsInOrder(t *testing.T) {
	h := newHarness(t)
	m := h.manager()
	s := m.New()
	appendText(t, s, "text")
	path := filepath.Join(h.dir, "saved.md")
	release := blockIdleWrite(t, h, path)
	s.SaveAs(path, nil)
	awaitWrite(t, h, h.files.entered)
	var calls []int
	m.Idle(func() { calls = append(calls, 1) })
	m.Idle(func() { calls = append(calls, 2) })
	h.queue.flush()
	if len(calls) != 0 {
		t.Fatalf("idle callbacks ran while the write was blocked: %v", calls)
	}
	release()
	waitIdle(t, h, func() bool { return len(calls) == 2 })
	if !reflect.DeepEqual(calls, []int{1, 2}) {
		t.Fatalf("idle callbacks %v", calls)
	}
}

func TestIdleWaitsForQueuedSave(t *testing.T) {
	h := newHarness(t)
	m := h.manager()
	first, second := m.New(), m.New()
	appendText(t, first, "first")
	appendText(t, second, "second")
	path := filepath.Join(h.dir, "second.md")
	release := blockIdleWrite(t, h, path)
	completed := 0
	onSave := func(err error) {
		if err != nil {
			t.Errorf("save: %v", err)
		}
		completed++
	}
	first.SaveAs(filepath.Join(h.dir, "first.md"), onSave)
	second.SaveAs(path, onSave)
	idle := false
	m.Idle(func() { idle = true })
	awaitWrite(t, h, h.files.entered)
	h.queue.flush()
	if idle || completed != 1 {
		t.Fatalf("while queued save is blocked: idle=%v, completed=%d", idle, completed)
	}
	release()
	waitIdle(t, h, func() bool { return idle })
	if completed != 2 || contents(t, path) != "second" {
		t.Fatalf("idle before queued save completed: completions=%d", completed)
	}
}

func TestIdleWaitsForRecoveryQueuedBySaveCompletion(t *testing.T) {
	h := newHarness(t)
	m := h.manager()
	s := m.New()
	appendText(t, s, "first")
	path := s.(*session).copyPath()
	release := blockIdleWrite(t, h, path)
	saved, idle := false, false
	s.SaveAs(filepath.Join(h.dir, "saved.md"), func(err error) {
		if err != nil {
			t.Errorf("save: %v", err)
		}
		saved = true
	})
	// The save completion queues a recovery write for the newer edit.
	appendText(t, s, " second")
	m.Idle(func() { idle = true })
	awaitWrite(t, h, h.files.entered)
	h.queue.flush()
	if idle || !saved {
		t.Fatalf("while completion's recovery write is blocked: idle=%v, saved=%v", idle, saved)
	}
	release()
	waitIdle(t, h, func() bool { return idle })
	var copy recoveryCopy
	if err := json.Unmarshal([]byte(contents(t, path)), &copy); err != nil {
		t.Fatal(err)
	}
	if string(copy.Text) != "first second" {
		t.Fatalf("recovery text %q", copy.Text)
	}
}

func TestIdleDoesNotWaitForTimer(t *testing.T) {
	h := newHarness(t)
	m := h.manager()
	s := m.New()
	appendText(t, s, "later")
	idle := false
	m.Idle(func() { idle = true })
	h.queue.flush()
	if !idle || len(h.clock.timers) == 0 {
		t.Fatal("idle did not finish while the recovery timer was pending")
	}
	path := s.(*session).copyPath()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("recovery copy before timer fired: %v", err)
	}
	h.advance(50 * time.Millisecond)
	var copy recoveryCopy
	if err := json.Unmarshal([]byte(contents(t, path)), &copy); err != nil {
		t.Fatal(err)
	}
	if string(copy.Text) != "later" {
		t.Fatalf("recovery text %q", copy.Text)
	}
}
