package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type recentWriteGate struct {
	started chan recentData
	release chan struct{}
}

func gatedRecent(t *testing.T) (*Recent, *queue, *recentWriteGate) {
	t.Helper()
	q := newQueue()
	r := NewRecent(filepath.Join(t.TempDir(), "recent.json"), q)
	gate := &recentWriteGate{started: make(chan recentData, 4), release: make(chan struct{}, 4)}
	write := r.write
	r.write = func(path string, data recentData) error {
		gate.started <- data
		<-gate.release
		return write(path, data)
	}
	t.Cleanup(func() {
		close(gate.release)
		(&fixture{t: t, q: q, recents: []*Recent{r}}).settle()
	})
	return r, q, gate
}

func waitRecentWrite(t *testing.T, q *queue, started <-chan recentData) recentData {
	t.Helper()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for {
		q.flush()
		select {
		case data := <-started:
			return data
		case <-q.ready:
		case <-deadline.C:
			t.Fatal("recent write did not start")
		}
	}
}

func waitRecentIdle(t *testing.T, q *queue, done func() bool) {
	t.Helper()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for !done() {
		q.flush()
		if done() {
			return
		}
		select {
		case <-q.ready:
		case <-deadline.C:
			t.Fatal("recent did not become idle")
		}
	}
}

func TestRecentIdleWhenEmpty(t *testing.T) {
	q := newQueue()
	r := NewRecent(filepath.Join(t.TempDir(), "recent.json"), q)
	var calls []int
	r.Idle(func() { calls = append(calls, 1) })
	r.Idle(nil)
	r.Idle(func() { calls = append(calls, 2) })
	if len(calls) != 0 {
		t.Fatal("idle callback ran synchronously")
	}
	q.flush()
	if !reflect.DeepEqual(calls, []int{1, 2}) {
		t.Fatalf("idle callbacks = %v, want [1 2]", calls)
	}
}

func TestRecentIdleWaitsForRunningWrite(t *testing.T) {
	r, q, gate := gatedRecent(t)
	r.Add("first.md", false)
	waitRecentWrite(t, q, gate.started)
	calls := 0
	r.Idle(func() { calls++ })
	q.flush()
	if calls != 0 {
		t.Fatal("idle callback ran while the write was blocked")
	}
	gate.release <- struct{}{}
	waitRecentIdle(t, q, func() bool { return calls != 0 })
	if calls != 1 {
		t.Fatalf("idle callback called %d times", calls)
	}
	b, err := os.ReadFile(r.path)
	if err != nil {
		t.Fatal(err)
	}
	var data recentData
	if err := json.Unmarshal(b, &data); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(data.Files, []string{"first.md"}) {
		t.Fatalf("saved files = %v", data.Files)
	}
}

func TestRecentIdleWaitsForPendingWrites(t *testing.T) {
	r, q, gate := gatedRecent(t)
	r.Add("first.md", false)
	waitRecentWrite(t, q, gate.started)
	r.Add("second.md", false)
	var calls []int
	r.Idle(func() { calls = append(calls, 1) })
	r.Idle(func() { calls = append(calls, 2) })
	// This replaces the pending snapshot after Idle has been called.
	r.Add("third.md", false)
	gate.release <- struct{}{}
	second := waitRecentWrite(t, q, gate.started)
	q.flush()
	if len(calls) != 0 {
		t.Fatalf("idle callbacks ran while the pending write was blocked: %v", calls)
	}
	if want := []string{"third.md", "second.md", "first.md"}; !reflect.DeepEqual(second.Files, want) {
		t.Fatalf("pending files = %v, want %v", second.Files, want)
	}
	// A new write queued while the second write runs must extend the wait too.
	r.Add("folder", true)
	gate.release <- struct{}{}
	third := waitRecentWrite(t, q, gate.started)
	q.flush()
	if len(calls) != 0 {
		t.Fatalf("idle callbacks ran while a later write was blocked: %v", calls)
	}
	if !reflect.DeepEqual(third.Folders, []string{"folder"}) {
		t.Fatalf("later folders = %v", third.Folders)
	}
	gate.release <- struct{}{}
	waitRecentIdle(t, q, func() bool { return len(calls) == 2 })
	if !reflect.DeepEqual(calls, []int{1, 2}) {
		t.Fatalf("idle callbacks = %v, want [1 2]", calls)
	}
	b, err := os.ReadFile(r.path)
	if err != nil {
		t.Fatal(err)
	}
	var saved recentData
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved, third) {
		t.Fatalf("saved = %+v, want %+v", saved, third)
	}
	r.Add("later.md", false)
	waitRecentWrite(t, q, gate.started)
	gate.release <- struct{}{}
	done := false
	r.Idle(func() { done = true })
	waitRecentIdle(t, q, func() bool { return done })
	if !reflect.DeepEqual(calls, []int{1, 2}) {
		t.Fatalf("idle callbacks were repeated: %v", calls)
	}
}

func TestRecentIdleAfterWriteError(t *testing.T) {
	q := newQueue()
	r := NewRecent(filepath.Join(t.TempDir(), "recent.json"), q)
	r.write = func(string, recentData) error { return errors.New("write failed") }
	r.Add("file.md", false)
	done := false
	r.Idle(func() { done = true })
	waitRecentIdle(t, q, func() bool { return done })
}

func TestRecentIdleRecoversCallbackPanic(t *testing.T) {
	q := newQueue()
	r := NewRecent(filepath.Join(t.TempDir(), "recent.json"), q)
	done := false
	r.Idle(func() { panic("test callback") })
	r.Idle(func() { done = true })
	q.flush()
	if !done {
		t.Fatal("a panicking callback prevented the next idle callback")
	}
}
