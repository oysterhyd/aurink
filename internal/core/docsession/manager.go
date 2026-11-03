package docsession

import (
	"crypto/rand"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"aurink/internal/contract"
	"aurink/internal/core/settings"
)

// Timer is a cancellable debounce timer, as implemented by time.Timer.
type Timer interface {
	// Stop cancels the callback if it has not fired yet.
	Stop() bool
}

// Options supplies settings and time sources. Nil fields use the application
// defaults and real time. AfterFunc callbacks may run on any goroutine.
type Options struct {
	// Settings is copied at construction; nil uses settings.Defaults.
	Settings *contract.Settings
	// Clock supplies SavedAt timestamps.
	Clock func() time.Time
	// AfterFunc schedules one callback after the given delay.
	AfterFunc func(time.Duration, func()) Timer
	// RecoveryDelay defaults to half the autosave delay when nonpositive.
	RecoveryDelay time.Duration
	// Watcher monitors the directory of each open file. Nil disables watching.
	Watcher contract.FileWatcher
}

// Manager implements contract.Sessions. All methods and model mutations belong
// to the main thread. File operations run serially on background goroutines.
type Manager struct {
	files      contract.FileStore
	factory    contract.DocumentFactory
	dispatcher contract.Dispatcher
	recovery   string
	options    Options
	settings   contract.Settings
	sessions   []*session
	paths      map[string]*session
	opening    map[string][]func(contract.Session, error)
	watches    map[string]*directoryWatch
	events     listeners[contract.SessionsEvent]
	jobs       []func()
	busy       bool
	idle       []func()
}

var _ contract.Sessions = (*Manager)(nil)

// New creates a manager without doing I/O. recoveryDir is private to this
// application's sessions; Restore scans it only when explicitly requested.
// An empty recoveryDir disables recovery copies.
// files, factory and dispatcher must be non-nil. The dispatcher must queue work
// for the main thread, rather than execute it on the posting goroutine.
func New(files contract.FileStore, factory contract.DocumentFactory, dispatcher contract.Dispatcher, recoveryDir string, opts Options) *Manager {
	if opts.Clock == nil {
		opts.Clock = time.Now
	}
	if opts.AfterFunc == nil {
		opts.AfterFunc = func(d time.Duration, fn func()) Timer { return time.AfterFunc(d, fn) }
	}
	m := &Manager{files: files, factory: factory, dispatcher: dispatcher, recovery: recoveryDir, options: opts,
		settings: settings.Defaults(), paths: make(map[string]*session), opening: make(map[string][]func(contract.Session, error)),
		watches: make(map[string]*directoryWatch)}
	if opts.Settings != nil {
		m.settings = *opts.Settings
	}
	return m
}

// SetSettings applies a settings snapshot and restarts edited sessions' timers.
// It can be connected directly to SettingsStore.Subscribe.
func (m *Manager) SetSettings(value contract.Settings) {
	m.settings = value
	for _, s := range m.sessions {
		s.schedule()
	}
}

func normalized(path string) (string, string, error) {
	if path == "" {
		return "", "", contract.E(contract.CodeInvalid, "docsession.path", nil)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", "", contract.E(contract.CodeInvalid, "docsession.path", err)
	}
	key := filepath.Clean(abs)
	if runtime.GOOS == "windows" {
		key = strings.ToLower(key)
	}
	return abs, key, nil
}

// Open reads a file asynchronously, sharing concurrent opens of the same path.
func (m *Manager) Open(path string, done func(contract.Session, error)) {
	path, key, err := normalized(path)
	if err != nil {
		m.dispatcher.Post(func() { callOpen(done, nil, err) })
		return
	}
	if callbacks, ok := m.opening[key]; ok {
		m.opening[key] = append(callbacks, done)
		return
	}
	m.opening[key] = []func(contract.Session, error){done}
	m.enqueue(func() {
		if s := m.paths[key]; s != nil {
			m.background(func() func() { return func() { m.opened(key, s, nil) } })
			return
		}
		opts := m.readOptions()
		m.background(func() func() {
			content, err := m.files.Read(path, opts)
			return func() {
				var s *session
				if err == nil {
					s, err = m.create(contract.DocID(rand.Text()), path, filepath.Base(path), content.Text, content.Meta, content.Stamp)
					if err == nil {
						m.events.emit(contract.SessionsEvent{Kind: contract.SessionOpened, ID: s.ID()})
					}
				}
				m.opened(key, s, err)
			}
		})
	})
}

func (m *Manager) opened(key string, s *session, err error) {
	callbacks := m.opening[key]
	delete(m.opening, key)
	for _, fn := range callbacks {
		var result contract.Session
		if s != nil {
			result = s
		}
		callOpen(fn, result, err)
	}
}

// New creates an empty untitled document synchronously.
func (m *Manager) New() contract.Session {
	s, err := m.create(contract.DocID(rand.Text()), "", "Untitled", "", contract.DocMeta{Encoding: "utf-8", Dominant: "\n"}, contract.FileStamp{})
	if err != nil {
		return nil
	}
	m.events.emit(contract.SessionsEvent{Kind: contract.SessionOpened, ID: s.ID()})
	return s
}

func (m *Manager) create(id contract.DocID, path, name, source string, meta contract.DocMeta, stamp contract.FileStamp) (*session, error) {
	model := m.factory.New(id, source, meta, m.settings.Editor.Exts)
	if model == nil {
		return nil, contract.E(contract.CodeInternal, "docsession.create", nil)
	}
	s := &session{manager: m, model: model, stamp: stamp, observed: stamp, saved: source, savedHash: fingerprint(source), previous: model.Text(), hash: fingerprint(source),
		state: contract.DocState{ID: id, Path: path, Name: name, ReadOnly: model.ReadOnly()}}
	s.off = model.Subscribe(s.changed)
	m.sessions = append(m.sessions, s)
	if path != "" {
		_, key, _ := normalized(path)
		m.paths[key] = s
		m.addWatch(path)
	}
	return s, nil
}

// Get finds an open session by identity.
func (m *Manager) Get(id contract.DocID) (contract.Session, bool) {
	for _, s := range m.sessions {
		if s.ID() == id {
			return s, true
		}
	}
	return nil, false
}

// FindByPath finds an open session using normalized path comparison.
func (m *Manager) FindByPath(path string) (contract.Session, bool) {
	_, key, err := normalized(path)
	if err == nil && m.paths[key] != nil {
		return m.paths[key], true
	}
	return nil, false
}

// All returns a copy of the open sessions in opening order.
func (m *Manager) All() []contract.Session {
	all := make([]contract.Session, len(m.sessions))
	for i, s := range m.sessions {
		all[i] = s
	}
	return all
}

// Subscribe registers a synchronous, ordered collection event callback.
func (m *Manager) Subscribe(fn func(contract.SessionsEvent)) func() { return m.events.subscribe(fn) }

func (m *Manager) readOptions() contract.ReadOptions {
	limit := m.settings.Files.LargeFileBytes
	if limit <= 0 {
		limit = 5 << 20
	}
	return contract.ReadOptions{MaxSize: limit}
}

func (m *Manager) delay() time.Duration {
	if m.settings.Files.AutosaveDelayMs <= 0 {
		return 1500 * time.Millisecond
	}
	return time.Duration(m.settings.Files.AutosaveDelayMs) * time.Millisecond
}

// Idle posts done to the dispatcher after the running job and all jobs already
// queued finish, including their background work and completions. Jobs added
// before the queue drains may also delay done. Timers that have not fired are
// not jobs; a timer that fires later can queue more work. Call Idle on the
// dispatcher thread; done is always posted to a later dispatcher turn.
func (m *Manager) Idle(done func()) {
	if !m.busy && len(m.jobs) == 0 {
		m.postIdle(done)
		return
	}
	m.idle = append(m.idle, done)
}

func (m *Manager) postIdle(done func()) {
	m.dispatcher.Post(func() {
		if done != nil {
			defer logPanic()
			done()
		}
	})
}

func (m *Manager) enqueue(start func()) {
	m.jobs = append(m.jobs, start)
	m.startNext()
}

func (m *Manager) startNext() {
	if m.busy {
		return
	}
	if len(m.jobs) == 0 {
		idle := m.idle
		m.idle = nil
		for _, done := range idle {
			m.postIdle(done)
		}
		return
	}
	start := m.jobs[0]
	m.jobs[0] = nil
	m.jobs = m.jobs[1:]
	m.busy = true
	start()
}

func (m *Manager) background(work func() func()) {
	go func() {
		complete := work()
		m.dispatcher.Post(func() {
			defer func() { m.busy = false; m.startNext() }()
			complete()
		})
	}()
}
