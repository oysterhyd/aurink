package docsession

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
	"aurink/internal/core/document"
	"aurink/internal/core/fsstore"
	"aurink/internal/core/settings"
)

type fakeTimer struct {
	clock *fakeClock
	at    time.Time
	fn    func()
}

func (t *fakeTimer) Stop() bool {
	_, ok := t.clock.timers[t]
	delete(t.clock.timers, t)
	return ok
}

type fakeClock struct {
	now    time.Time
	timers map[*fakeTimer]bool
}

func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), timers: make(map[*fakeTimer]bool)}
}

func (c *fakeClock) after(d time.Duration, fn func()) Timer {
	t := &fakeTimer{clock: c, at: c.now.Add(d), fn: fn}
	c.timers[t] = true
	return t
}

func (c *fakeClock) advance(d time.Duration) {
	c.now = c.now.Add(d)
	for {
		var next *fakeTimer
		for t := range c.timers {
			if !t.at.After(c.now) && (next == nil || t.at.Before(next.at)) {
				next = t
			}
		}
		if next == nil {
			return
		}
		delete(c.timers, next)
		next.fn()
	}
}

type queued struct {
	mu    sync.Mutex
	fns   []func()
	ready chan struct{}
}

func (q *queued) Post(fn func()) {
	q.mu.Lock()
	q.fns = append(q.fns, fn)
	q.mu.Unlock()
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

func (q *queued) flush() {
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

type harness struct {
	t        *testing.T
	dir      string
	recovery string
	clock    *fakeClock
	queue    *queued
	files    *probeStore
	settings contract.Settings
	managers []*Manager
	factory  contract.DocumentFactory
}

func goroutineID() uint64 {
	var data [64]byte
	n := runtime.Stack(data[:], false)
	id, _ := strconv.ParseUint(strings.Fields(string(data[:n]))[1], 10, 64)
	return id
}

type probeStore struct {
	contract.FileStore
	t        *testing.T
	main     uint64
	mu       sync.Mutex
	writes   []string
	failPath string
	failCode contract.Code
	entered  chan struct{}
	release  chan struct{}
}

func (s *probeStore) guard() {
	if goroutineID() == s.main {
		s.t.Error("blocking FileStore operation on the main goroutine")
	}
}

func (s *probeStore) Read(path string, opts contract.ReadOptions) (contract.FileContent, error) {
	s.guard()
	return s.FileStore.Read(path, opts)
}

func (s *probeStore) Stat(path string) (contract.FileStamp, error) {
	s.guard()
	return s.FileStore.Stat(path)
}

func (s *probeStore) Write(path, text string, meta contract.DocMeta, expect *contract.FileStamp) (contract.FileStamp, error) {
	s.guard()
	s.mu.Lock()
	s.writes = append(s.writes, path)
	fail := path == s.failPath
	code, entered, release := s.failCode, s.entered, s.release
	s.mu.Unlock()
	if entered != nil && path == s.failPath {
		entered <- struct{}{}
		<-release
	}
	if fail && code != "" {
		return contract.FileStamp{}, contract.E(code, "injected.Write", nil)
	}
	return s.FileStore.Write(path, text, meta, expect)
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{t: t, dir: t.TempDir(), clock: newClock(), queue: &queued{ready: make(chan struct{}, 1)}, settings: settings.Defaults()}
	h.recovery = filepath.Join(h.dir, "recovery")
	h.settings.Files.AutosaveDelayMs = 100
	h.factory = document.Factory{Clock: func() time.Time { return h.clock.now }}
	h.files = &probeStore{FileStore: fsstore.New(), t: t, main: goroutineID()}
	t.Cleanup(h.settle)
	return h
}

func (h *harness) manager() *Manager {
	m := New(h.files, h.factory, h.queue, h.recovery, Options{Settings: &h.settings,
		Clock: func() time.Time { return h.clock.now }, AfterFunc: h.clock.after})
	h.managers = append(h.managers, m)
	return m
}

func (h *harness) settle() {
	h.t.Helper()
	for {
		h.queue.flush()
		busy := false
		for _, m := range h.managers {
			busy = busy || m.busy || len(m.jobs) != 0
		}
		if !busy {
			return
		}
		<-h.queue.ready
	}
}

func (h *harness) advance(d time.Duration) { h.clock.advance(d); h.settle() }

func put(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func contents(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (h *harness) open(m *Manager, path string) contract.Session {
	h.t.Helper()
	var got contract.Session
	m.Open(path, func(s contract.Session, err error) {
		if err != nil {
			h.t.Fatal(err)
		}
		got = s
	})
	h.settle()
	if got == nil {
		h.t.Fatal("missing open callback")
	}
	return got
}

func edit(t *testing.T, s contract.Session, start, end int, text string) {
	t.Helper()
	_, err := s.Model().Apply(contract.Transaction{Origin: contract.OriginUser, Edits: []contract.TextEdit{{Range: contract.Range{Start: start, End: end}, Text: text}}})
	if err != nil {
		t.Fatal(err)
	}
}

func appendText(t *testing.T, s contract.Session, text string) {
	edit(t, s, s.Model().Text().Len(), s.Model().Text().Len(), text)
}

func (h *harness) finish(run func(func(error))) error {
	h.t.Helper()
	called := 0
	var got error
	run(func(err error) {
		if goroutineID() != h.files.main {
			h.t.Error("completion on a worker goroutine")
		}
		called++
		got = err
	})
	if called != 0 {
		h.t.Error("completion ran before dispatch")
	}
	h.settle()
	if called != 1 {
		h.t.Fatalf("completion count %d", called)
	}
	return got
}

func (h *harness) restore(m *Manager) []contract.Session {
	h.t.Helper()
	var got []contract.Session
	m.Restore(func(s []contract.Session, err error) {
		if err != nil {
			h.t.Fatal(err)
		}
		got = s
	})
	h.settle()
	return got
}

func wantCode(t *testing.T, err error, code contract.Code) {
	t.Helper()
	if contract.CodeOf(err) != code {
		t.Fatalf("error %v, want code %q", err, code)
	}
	if err != nil {
		if _, ok := err.(*contract.Error); !ok {
			t.Fatalf("untyped error %T", err)
		}
	}
}

func TestSessions(t *testing.T) {
	contracttest.TestSessions(t, func(t *testing.T) contracttest.SessionFixture {
		h := newHarness(t)
		return contracttest.SessionFixture{New: func() contract.Sessions { return h.manager() }, Settle: h.settle,
			Advance: h.clock.advance, Autosave: 100 * time.Millisecond, Dir: h.dir}
	})
}

func TestWriteFailureRetainsRecovery(t *testing.T) {
	h := newHarness(t)
	m := h.manager()
	path := filepath.Join(h.dir, "a.md")
	put(t, path, "a")
	s := h.open(m, path)
	appendText(t, s, "b")
	h.advance(50 * time.Millisecond)
	copyPath := s.(*session).copyPath()
	before := contents(t, copyPath)
	h.files.failPath, h.files.failCode = path, contract.CodeIO
	wantCode(t, h.finish(s.Save), contract.CodeIO)
	if !s.State().Edited || s.State().Err == nil || s.State().Err.Code != contract.CodeIO || contents(t, path) != "a" || contents(t, copyPath) != before {
		t.Fatalf("failed save lost state or copy: %+v", s.State())
	}
	h.files.failCode = ""
	wantCode(t, h.finish(s.Save), "")
	if _, err := os.Stat(copyPath); !os.IsNotExist(err) {
		t.Fatal("successful save retained the recovery copy", err)
	}
}

func TestQueuedSavesAndClose(t *testing.T) {
	for _, closeDuringSave := range []bool{false, true} {
		t.Run(fmt.Sprint(closeDuringSave), func(t *testing.T) {
			h := newHarness(t)
			m := h.manager()
			path := filepath.Join(h.dir, "a.md")
			put(t, path, "a")
			s := h.open(m, path)
			appendText(t, s, "b")
			h.advance(50 * time.Millisecond)
			h.files.failPath, h.files.entered, h.files.release = path, make(chan struct{}, 2), make(chan struct{})
			var errs []error
			s.Save(func(err error) { errs = append(errs, err) })
			<-h.files.entered
			appendText(t, s, "c")
			s.Save(func(err error) { errs = append(errs, err) })
			if !s.State().Saving {
				t.Fatal("queued saves not marked saving")
			}
			if closeDuringSave {
				if s.Close(false) != contract.CloseNeedsPrompt || s.Close(true) != contract.CloseDone || s.Close(true) != contract.CloseDone {
					t.Fatal("close decisions")
				}
			}
			close(h.files.release)
			h.settle()
			if len(errs) != 2 {
				t.Fatalf("callbacks %d", len(errs))
			}
			want := "abc"
			code := contract.Code("")
			if closeDuringSave {
				want, code = "ab", contract.CodeCanceled
				if len(m.All()) != 0 || len(h.restore(h.manager())) != 0 {
					t.Fatal("closed session returned")
				}
			}
			for _, err := range errs {
				wantCode(t, err, code)
			}
			if contents(t, path) != want || s.State().Saving {
				t.Fatalf("file %q state %+v", contents(t, path), s.State())
			}
		})
	}
}

func TestRecoveryConflictMetadataAndCorruption(t *testing.T) {
	h := newHarness(t)
	h.settings.Files.Autosave = false
	m := h.manager()
	path := filepath.Join(h.dir, "utf16.md")
	meta := contract.DocMeta{Encoding: "utf-16be", BOM: true, Dominant: "\r\n", EOL: contract.EOLCRLF}
	if _, err := fsstore.New().Write(path, "中\r\n", meta, nil); err != nil {
		t.Fatal(err)
	}
	s := h.open(m, path)
	appendText(t, s, "文")
	h.advance(50 * time.Millisecond)
	copyPath := s.(*session).copyPath()
	copyBytes := contents(t, copyPath)
	put(t, path, "external longer content")
	put(t, filepath.Join(h.recovery, "broken.json"), "{broken")
	put(t, filepath.Join(h.recovery, "invalid.json"), `{"Format":7}`)
	put(t, filepath.Join(h.recovery, "ignored.tmp"), "unfinished")
	if err := os.Mkdir(filepath.Join(h.recovery, "dir.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	prior := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prior) })
	second := h.manager()
	got := h.restore(second)
	if len(got) != 1 || !got[0].State().Conflict || !got[0].State().Edited || got[0].Model().Meta() != meta || got[0].Model().Text().String() != "中\r\n文" {
		t.Fatalf("restored %+v", got)
	}
	if !strings.Contains(logs.String(), "corrupt recovery copy skipped") || contents(t, filepath.Join(h.recovery, "broken.json")) != "{broken" || contents(t, copyPath) != copyBytes {
		t.Fatal("corruption was not logged and retained")
	}
	if again := h.restore(second); len(again) != 1 || again[0].ID() != got[0].ID() {
		t.Fatal("restore duplicated sessions")
	}
	if contents(t, path) != "external longer content" {
		t.Fatal("restore wrote to source")
	}
}

func TestNotificationsReenterAndSurvivePanic(t *testing.T) {
	h := newHarness(t)
	m := h.manager()
	s := m.New()
	var order []int
	off := s.OnState(func(contract.DocState) { order = append(order, 1); s.Close(true) })
	s.OnState(func(contract.DocState) { panic("expected") })
	s.OnState(func(contract.DocState) { order = append(order, 2) })
	appendText(t, s, "a")
	off()
	off()
	if fmt.Sprint(order) != "[1 2]" || len(m.All()) != 0 {
		t.Fatalf("order %v sessions %d", order, len(m.All()))
	}
	other := m.New()
	m.Subscribe(func(e contract.SessionsEvent) {
		if e.Kind == contract.SessionOpened {
			other.Close(true)
		}
	})
	m.Subscribe(func(e contract.SessionsEvent) { order = append(order, int(e.Kind)+10) })
	m.New()
	if fmt.Sprint(order) != "[1 2 11 12]" {
		t.Fatal("collection events reordered", order)
	}
	h.settle()
}
