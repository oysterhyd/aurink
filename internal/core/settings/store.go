package settings

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"sync"

	"aurink/internal/contract"
)

// Store implements contract.SettingsStore. Except for Flush, its public methods
// must be called on the main thread. The constructor loads before the first frame.
type Store struct {
	path       string
	files      contract.FileStore
	dispatcher contract.Dispatcher
	onError    func(error)
	value      contract.Settings
	subs       []*subscriber

	mu        sync.Mutex
	finished  *sync.Cond
	pending   *write
	running   bool
	submitted uint64
	completed uint64
	writeErr  error
}

type subscriber struct{ fn func(contract.Settings) }
type write struct {
	sequence uint64
	text     string
}

var _ contract.SettingsStore = (*Store)(nil)

// New synchronously loads path using files. Call it during startup, before the
// first frame. Missing or corrupt files use Defaults; corrupt files are renamed
// beside the original. Other load or backup failures return a *contract.Error.
// Only a successful Update schedules a write, so newer files remain untouched.
// onError may be nil; write failures are logged and posted to it via dispatcher.
func New(path string, files contract.FileStore, dispatcher contract.Dispatcher, onError func(error)) (*Store, error) {
	return newStore(path, files, dispatcher, onError, migrations)
}

func newStore(path string, files contract.FileStore, dispatcher contract.Dispatcher, onError func(error), steps []migration) (*Store, error) {
	if path == "" || files == nil || dispatcher == nil {
		return nil, failure(contract.CodeInvalid, "settings.New", path, nil)
	}
	s := &Store{path: path, files: files, dispatcher: dispatcher, onError: onError, value: Defaults()}
	s.finished = sync.NewCond(&s.mu)
	if err := s.load(steps); err != nil {
		return nil, err
	}
	return s, nil
}

// Get returns an independent snapshot, including a copy of the keymap.
func (s *Store) Get() contract.Settings { return clone(s.value) }

// Update runs fn synchronously on a copy, normalizes it, and notifies subscribers
// synchronously in subscription order. A nil or panicking fn leaves state intact.
// Numeric limits are hue 0..359, font 8..72, line height 1..3, width 320..2000,
// tab size 1..8, and autosave delay 100..60000 ms; invalid choices use defaults.
// Persistence runs in the background; later pending snapshots replace earlier ones.
func (s *Store) Update(fn func(*contract.Settings)) (err error) {
	if fn == nil {
		return failure(contract.CodeInvalid, "settings.Update", s.path, nil)
	}
	v := s.Get()
	if err := edit(fn, &v); err != nil {
		return failure(contract.CodeInternal, "settings.Update", s.path, err)
	}
	normalize(&v)
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return failure(contract.CodeInvalid, "settings.Update", s.path, err)
	}
	s.value = clone(v)
	// Queue before notifications so a reentrant Update cannot be overwritten.
	s.enqueue(string(data) + "\n")
	for _, sub := range slices.Clone(s.subs) {
		if sub.fn != nil {
			notify(sub.fn, clone(v))
		}
	}
	return nil
}

// Subscribe registers fn without an initial notification. Its idempotent off
// function must also run on the main thread. Subscribers may call store methods.
func (s *Store) Subscribe(fn func(contract.Settings)) (off func()) {
	sub := &subscriber{fn: fn}
	s.subs = append(s.subs, sub)
	return func() {
		sub.fn = nil
		s.subs = slices.DeleteFunc(s.subs, func(item *subscriber) bool { return item == sub })
	}
}

// Flush waits for writes scheduled before this call and returns the most recent
// completed write's error. It does not run posted error handlers; the app's main
// loop does that. No main-thread work is needed for Flush to complete.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	target := s.submitted
	for s.completed < target {
		s.finished.Wait()
	}
	return s.writeErr
}

func (s *Store) enqueue(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.submitted++
	s.pending = &write{sequence: s.submitted, text: text}
	if !s.running {
		s.running = true
		go s.run()
	}
}

func (s *Store) run() {
	for {
		s.mu.Lock()
		job := s.pending
		s.pending = nil
		if job == nil {
			s.running = false
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		err := s.persist(job.text)
		if err != nil {
			s.report(err)
		}
		s.mu.Lock()
		s.completed, s.writeErr = job.sequence, err
		s.finished.Broadcast()
		s.mu.Unlock()
	}
}

func (s *Store) persist(text string) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = failure(contract.CodeInternal, "settings.Write", s.path, fmt.Errorf("panic: %v", p))
			slog.Error("settings writer panicked", "component", "settings", "error", err, "stack", string(debug.Stack()))
		}
	}()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return failure(contract.CodeIO, "settings.Write", s.path, err)
	}
	_, err = s.files.Write(s.path, text, contract.DocMeta{Encoding: "utf-8", Dominant: "\n"}, nil)
	if err != nil {
		return failure(contract.CodeOf(err), "settings.Write", s.path, err)
	}
	return nil
}

func (s *Store) report(err error) {
	defer logPanic("error dispatcher")
	slog.Error("settings write failed", "component", "settings", "error", err)
	if s.onError != nil {
		s.dispatcher.Post(func() {
			defer logPanic("error handler")
			s.onError(err)
		})
	}
}

func edit(fn func(*contract.Settings), v *contract.Settings) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("update panicked: %v", p)
			slog.Error("settings update panicked", "component", "settings", "error", err, "stack", string(debug.Stack()))
		}
	}()
	fn(v)
	return nil
}

func notify(fn func(contract.Settings), v contract.Settings) {
	defer logPanic("subscriber")
	fn(v)
}

func logPanic(source string) {
	if p := recover(); p != nil {
		slog.Error("settings callback panicked", "component", "settings", "source", source, "panic", p, "stack", string(debug.Stack()))
	}
}

func clone(v contract.Settings) contract.Settings {
	v.Keymap = slices.Clone(v.Keymap)
	return v
}

func failure(code contract.Code, op, path string, err error) *contract.Error {
	return &contract.Error{Code: code, Op: op, Path: path, Err: err}
}
