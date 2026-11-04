package workspace

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"aurink/internal/contract"
)

type recentData struct {
	Files   []string `json:"files"`
	Folders []string `json:"folders"`
}

// Recent holds the most recently opened files and folders on the main thread.
type Recent struct {
	path       string
	dispatcher contract.Dispatcher
	data       recentData
	busy       bool
	pending    *recentData
	idle       []func()
	write      func(string, recentData) error
}

var _ contract.Recent = (*Recent)(nil)

// NewRecent reads the previous lists and saves later changes in the background.
func NewRecent(path string, dispatcher contract.Dispatcher) *Recent {
	r := &Recent{path: path, dispatcher: dispatcher, write: writeRecent}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &r.data); err != nil {
			slog.Error("reading recent paths", "path", path, "error", err)
			r.data = recentData{}
		}
	}
	r.data.Files = normalizeRecent(r.data.Files)
	r.data.Folders = normalizeRecent(r.data.Folders)
	return r
}

// Files returns a copy of the recent files, newest first.
func (r *Recent) Files() []string { return slices.Clone(r.data.Files) }

// Folders returns a copy of the recent folders, newest first.
func (r *Recent) Folders() []string { return slices.Clone(r.data.Folders) }

// Add records one path and schedules an atomic write.
func (r *Recent) Add(path string, isDir bool) {
	if path == "" {
		return
	}
	list := &r.data.Files
	if isDir {
		list = &r.data.Folders
	}
	*list = normalizeRecent(append([]string{path}, (*list)...))
	r.save()
}

// Clear removes both lists and schedules an atomic write.
func (r *Recent) Clear() {
	r.data = recentData{}
	r.save()
}

// Idle posts done to the dispatcher after the running write and all pending
// writes finish, including their background work and completions. Writes added
// before the queue drains may also delay done. Call Idle on the dispatcher
// thread; done is always posted to a later dispatcher turn.
func (r *Recent) Idle(done func()) {
	if !r.busy && r.pending == nil {
		r.postIdle(done)
		return
	}
	r.idle = append(r.idle, done)
}

func (r *Recent) postIdle(done func()) {
	r.dispatcher.Post(func() {
		if done != nil {
			defer func() {
				if p := recover(); p != nil {
					slog.Error("recent callback panicked", "panic", p)
				}
			}()
			done()
		}
	})
}

func normalizeRecent(paths []string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, path := range paths {
		if path == "" {
			continue
		}
		key := filepath.Clean(path)
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			seen[key] = true
			out = append(out, path)
			if len(out) == contract.RecentMax {
				break
			}
		}
	}
	return out
}

func (r *Recent) save() {
	snapshot := recentData{Files: slices.Clone(r.data.Files), Folders: slices.Clone(r.data.Folders)}
	r.pending = &snapshot
	if r.busy {
		return
	}
	r.writeNext()
}

func (r *Recent) writeNext() {
	if r.pending == nil {
		idle := r.idle
		r.idle = nil
		for _, done := range idle {
			r.postIdle(done)
		}
		return
	}
	data := *r.pending
	r.pending = nil
	r.busy = true
	go func() {
		err := r.write(r.path, data)
		r.dispatcher.Post(func() {
			if err != nil {
				slog.Error("saving recent paths", "path", r.path, "error", err)
			}
			r.busy = false
			r.writeNext()
		})
	}()
}

func writeRecent(path string, data recentData) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".recent-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return replaceFile(f.Name(), path)
}
