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
}

var _ contract.Recent = (*Recent)(nil)

// NewRecent reads the previous lists and saves later changes in the background.
func NewRecent(path string, dispatcher contract.Dispatcher) *Recent {
	r := &Recent{path: path, dispatcher: dispatcher}
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
		return
	}
	data := *r.pending
	r.pending = nil
	r.busy = true
	go func() {
		err := writeRecent(r.path, data)
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
