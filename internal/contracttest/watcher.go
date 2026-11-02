package contracttest

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"aurink/internal/contract"
)

// SnapshotWatcher is a contract.FileWatcher for tests (D1). It watches real
// directories but reports only when Flush compares them with what it saw last,
// so tests need no timers or sleeps. Renames come as a removal and a creation,
// as some platforms report them.
type SnapshotWatcher struct {
	mu      sync.Mutex
	watches []*snapshotWatch
	// Calls counts Watch calls, including failed ones.
	Calls int
}

type fileSig struct {
	isDir bool
	size  int64
	mod   time.Time
}

type snapshotWatch struct {
	owner  *SnapshotWatcher
	path   string
	opts   contract.WatchOptions
	fn     func([]contract.WatchEvent, error)
	seen   map[string]fileSig
	closed bool
}

// Watch starts a watch. It fails with CodeNotFound for a missing path and
// CodeTooLarge when the tree has more entries than opts.MaxEntries.
func (w *SnapshotWatcher) Watch(path string, opts contract.WatchOptions, fn func([]contract.WatchEvent, error)) (contract.Watch, error) {
	w.mu.Lock()
	w.Calls++
	w.mu.Unlock()
	s := &snapshotWatch{owner: w, path: filepath.Clean(path), opts: opts, fn: fn}
	seen, err := s.scan()
	if err != nil {
		return nil, err
	}
	s.seen = seen
	w.mu.Lock()
	w.watches = append(w.watches, s)
	w.mu.Unlock()
	return s, nil
}

// Paths returns the paths of the open watches, sorted.
func (w *SnapshotWatcher) Paths() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []string
	for _, s := range w.watches {
		if !s.closed {
			out = append(out, s.path)
		}
	}
	sort.Strings(out)
	return out
}

// Flush reports what changed on disk since the last look, on the calling
// goroutine, one batch per watch that saw changes.
func (w *SnapshotWatcher) Flush() {
	for _, s := range w.open() {
		now, err := s.scan()
		if err != nil {
			s.end(err)
			continue
		}
		if batch := diff(s.seen, now); len(batch) != 0 {
			s.seen = now
			s.fn(batch, nil)
		}
	}
}

// Overflow reports to every open watch that events may have been lost.
func (w *SnapshotWatcher) Overflow() {
	for _, s := range w.open() {
		if now, err := s.scan(); err == nil {
			s.seen = now
		}
		s.fn([]contract.WatchEvent{{Op: contract.WatchOverflow, Path: s.path, IsDir: true}}, nil)
	}
}

// Fail ends every open watch of path with an error of the given code.
func (w *SnapshotWatcher) Fail(path string, code contract.Code) {
	for _, s := range w.open() {
		if s.path == filepath.Clean(path) {
			s.end(contract.E(code, "watch", nil))
		}
	}
}

func (w *SnapshotWatcher) open() []*snapshotWatch {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []*snapshotWatch
	for _, s := range w.watches {
		if !s.closed {
			out = append(out, s)
		}
	}
	return out
}

func (s *snapshotWatch) Close() {
	s.owner.mu.Lock()
	defer s.owner.mu.Unlock()
	s.closed = true
}

func (s *snapshotWatch) end(err error) {
	s.Close()
	s.fn(nil, err)
}

func (s *snapshotWatch) scan() (map[string]fileSig, error) {
	info, err := os.Stat(s.path)
	if err != nil {
		return nil, contract.E(contract.CodeNotFound, "watch", err)
	}
	seen := map[string]fileSig{}
	if !info.IsDir() {
		seen[s.path] = fileSig{size: info.Size(), mod: info.ModTime()}
		return seen, nil
	}
	err = filepath.WalkDir(s.path, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == s.path {
			return err
		}
		rel, _ := filepath.Rel(s.path, p)
		rel = filepath.ToSlash(rel)
		if s.opts.Ignore != nil && s.opts.Ignore(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		i, err := d.Info()
		if err != nil {
			return nil
		}
		seen[p] = fileSig{isDir: d.IsDir(), size: i.Size(), mod: i.ModTime()}
		if d.IsDir() && !s.opts.Recursive {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, contract.E(contract.CodeIO, "watch", err)
	}
	if s.opts.MaxEntries > 0 && len(seen) > s.opts.MaxEntries {
		return nil, contract.E(contract.CodeTooLarge, "watch", nil)
	}
	return seen, nil
}

func diff(old, now map[string]fileSig) []contract.WatchEvent {
	var out []contract.WatchEvent
	for p, sig := range old {
		if _, ok := now[p]; !ok {
			out = append(out, contract.WatchEvent{Op: contract.WatchRemove, Path: p, IsDir: sig.isDir})
		}
	}
	for p, sig := range now {
		was, ok := old[p]
		switch {
		case !ok:
			out = append(out, contract.WatchEvent{Op: contract.WatchCreate, Path: p, IsDir: sig.isDir})
		case !sig.isDir && (was.size != sig.size || !was.mod.Equal(sig.mod)):
			out = append(out, contract.WatchEvent{Op: contract.WatchWrite, Path: p})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Op < out[j].Op
	})
	return out
}
