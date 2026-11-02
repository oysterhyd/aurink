package workspace

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"unicode"

	"aurink/internal/contract"
)

// Options configures the initial filter and watch limit.
type Options struct {
	MaxEntries int
	ShowAll    bool
}

// Workspace owns one root and its lazily loaded directory lists.
// Except for Ignored, its methods are called on the dispatcher thread.
type Workspace struct {
	watcher    contract.FileWatcher
	trasher    contract.Trasher
	dispatcher contract.Dispatcher
	opts       Options
	root       string
	showAll    bool
	cache      map[string][]contract.Entry
	loading    map[string]bool
	watches    map[string]contract.Watch
	degraded   bool
	generation uint64
	nextGen    uint64
	setting    uint64
	pending    []watchResult
	jobs       []func()
	busy       bool
	listeners  []*listener
	queued     []contract.WorkspaceEvent
	emitting   bool
}

type listener struct{ fn func(contract.WorkspaceEvent) }

type watchResult struct {
	gen   uint64
	batch []contract.WatchEvent
	err   error
}

var _ contract.Workspace = (*Workspace)(nil)

// New creates a workspace. Dispatcher must queue callbacks on the main thread.
func New(watcher contract.FileWatcher, trasher contract.Trasher, dispatcher contract.Dispatcher, opts Options) *Workspace {
	return &Workspace{watcher: watcher, trasher: trasher, dispatcher: dispatcher, opts: opts,
		showAll: opts.ShowAll, cache: make(map[string][]contract.Entry), loading: make(map[string]bool), watches: make(map[string]contract.Watch)}
}

// Root returns the current root, or an empty string before one is opened.
func (w *Workspace) Root() string { return w.root }

// SetRoot validates and watches a new root before replacing the old one.
func (w *Workspace) SetRoot(path string, done func(error)) {
	w.enqueue(func() {
		w.nextGen++
		gen := w.nextGen
		w.setting = gen
		old := cloneWatches(w.watches)
		w.background(func() func() {
			root, err := rootPath(path)
			var watch contract.Watch
			degraded := false
			if err == nil {
				watch, err = w.startWatch(root, true, gen)
				if contract.CodeOf(err) == contract.CodeTooLarge {
					degraded = true
					watch, err = w.startWatch(root, false, gen)
				}
			}
			if err != nil {
				var ce *contract.Error
				if !errors.As(err, &ce) {
					err = ioFailure("workspace.SetRoot", root, err)
				}
			}
			if err == nil {
				closeWatches(old)
			}
			return func() {
				w.setting = 0
				pending := w.pending
				w.pending = nil
				if err == nil {
					w.root, w.generation, w.degraded = root, gen, degraded
					w.cache = make(map[string][]contract.Entry)
					w.loading = make(map[string]bool)
					w.watches = make(map[string]contract.Watch)
					if watch != nil {
						w.watches[root] = watch
					}
					w.emit(contract.WorkspaceEvent{Kind: contract.WsRootChanged, Path: root, IsDir: true})
					if degraded {
						w.emit(contract.WorkspaceEvent{Kind: contract.WsDegraded, Path: root, IsDir: true})
					}
					for _, result := range pending {
						w.watchResult(result)
					}
				}
				callDone(done, err)
			}
		})
	})
}

func rootPath(path string) (string, error) {
	if path == "" {
		return "", failure(contract.CodeInvalid, "workspace.SetRoot", path, nil)
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return "", failure(contract.CodeInvalid, "workspace.SetRoot", path, err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", ioFailure("workspace.SetRoot", root, err)
	}
	if !info.IsDir() {
		return "", failure(contract.CodeInvalid, "workspace.SetRoot", root, nil)
	}
	return filepath.Clean(root), nil
}

// List returns a copy of a cached listing, or starts loading it.
func (w *Workspace) List(dir string) ([]contract.Entry, bool) {
	dir = filepath.Clean(dir)
	if !within(w.root, dir) {
		return nil, false
	}
	entries, loaded := w.cache[dir]
	if !loaded {
		w.load(dir)
		return nil, false
	}
	var result []contract.Entry
	for _, entry := range entries {
		if w.showAll || entry.IsDir || visibleFile(entry.Name) {
			result = append(result, entry)
		}
	}
	return result, true
}

// Refresh reloads a directory on a worker.
func (w *Workspace) Refresh(dir string) {
	dir = filepath.Clean(dir)
	if within(w.root, dir) {
		w.load(dir)
	}
}

func (w *Workspace) load(dir string) {
	if w.loading[dir] {
		return
	}
	w.loading[dir] = true
	requestedGen := w.generation
	w.enqueue(func() {
		if requestedGen != w.generation {
			w.background(func() func() { return func() {} })
			return
		}
		gen := w.generation
		root := w.root
		needsWatch := w.degraded && w.watches[dir] == nil
		w.background(func() func() {
			var watch contract.Watch
			if needsWatch {
				var err error
				watch, err = w.startWatch(dir, false, gen)
				if err != nil {
					slog.Error("watching loaded directory", "path", dir, "error", err)
				}
			}
			entries, err := readDirectory(dir, root)
			if err != nil && watch != nil {
				watch.Close()
				watch = nil
			}
			return func() {
				delete(w.loading, dir)
				if gen != w.generation {
					if watch != nil {
						go watch.Close()
					}
					return
				}
				if err != nil {
					slog.Error("listing workspace directory", "path", dir, "error", err)
					return
				}
				if watch != nil {
					w.watches[dir] = watch
				}
				w.cache[dir] = entries
				w.emit(contract.WorkspaceEvent{Kind: contract.WsListed, Path: dir, IsDir: true})
			}
		})
	})
}

// SetShowAll changes the visible file types and re-emits loaded listings.
func (w *Workspace) SetShowAll(all bool) {
	if w.showAll == all {
		return
	}
	w.showAll = all
	gen := w.generation
	dirs := w.loadedDirs()
	w.dispatcher.Post(func() {
		if w.generation != gen {
			return
		}
		for _, dir := range dirs {
			if _, loaded := w.cache[dir]; loaded {
				w.emit(contract.WorkspaceEvent{Kind: contract.WsListed, Path: dir, IsDir: true})
			}
		}
	})
}

// Ignored is the shared, thread-safe rule for the tree and search.
func (w *Workspace) Ignored(rel string, isDir bool) bool {
	return ignored(rel)
}

func ignored(rel string) bool {
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part != "" && part != "." && part != ".." && (strings.HasPrefix(part, ".") || part == "node_modules") {
			return true
		}
	}
	return false
}

// Subscribe registers a listener. The returned function unregisters it.
func (w *Workspace) Subscribe(fn func(contract.WorkspaceEvent)) func() {
	l := &listener{fn: fn}
	w.listeners = append(w.listeners, l)
	return func() {
		if i := slices.Index(w.listeners, l); i >= 0 {
			w.listeners = slices.Delete(w.listeners, i, i+1)
		}
		l.fn = nil
	}
}

func (w *Workspace) emit(event contract.WorkspaceEvent) {
	w.queued = append(w.queued, event)
	if w.emitting {
		return
	}
	w.emitting = true
	defer func() { w.emitting = false }()
	for len(w.queued) != 0 {
		e := w.queued[0]
		w.queued = w.queued[1:]
		for _, listener := range slices.Clone(w.listeners) {
			if listener.fn != nil {
				listener.fn(e)
			}
		}
	}
}

func (w *Workspace) enqueue(job func()) {
	w.jobs = append(w.jobs, job)
	w.startNext()
}

func (w *Workspace) startNext() {
	if w.busy || len(w.jobs) == 0 {
		return
	}
	job := w.jobs[0]
	w.jobs[0] = nil
	w.jobs = w.jobs[1:]
	w.busy = true
	job()
}

func (w *Workspace) background(work func() func()) {
	go func() {
		complete := work()
		w.dispatcher.Post(func() {
			defer func() { w.busy = false; w.startNext() }()
			complete()
		})
	}()
}

// Close releases the watches on a worker; done is posted to the dispatcher.
func (w *Workspace) Close(done func()) {
	w.enqueue(func() {
		old := cloneWatches(w.watches)
		w.background(func() func() {
			closeWatches(old)
			return func() {
				w.generation++
				w.root = ""
				w.cache = make(map[string][]contract.Entry)
				w.watches = make(map[string]contract.Watch)
				if done != nil {
					done()
				}
			}
		})
	})
}

func callDone(done func(error), err error) {
	if done != nil {
		done(err)
	}
}

func (w *Workspace) loadedDirs() []string {
	dirs := make([]string, 0, len(w.cache))
	for dir := range w.cache {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	return dirs
}

func readDirectory(dir, root string) ([]contract.Entry, error) {
	items, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var entries []contract.Entry
	for _, item := range items {
		path := filepath.Join(dir, item.Name())
		rel, err := filepath.Rel(root, path)
		if err != nil || ignored(filepath.ToSlash(rel)) {
			continue
		}
		info, err := item.Info()
		if err != nil {
			continue
		}
		entries = append(entries, contract.Entry{Path: path, Name: item.Name(), IsDir: item.IsDir(), Size: info.Size(), ModTime: info.ModTime()})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return naturalLess(entries[i].Name, entries[j].Name)
	})
	return entries, nil
}

func visibleFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown", ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp":
		return true
	}
	return false
}

func naturalLess(a, b string) bool {
	x, y := []rune(strings.ToLower(a)), []rune(strings.ToLower(b))
	i, j := 0, 0
	for i < len(x) && j < len(y) {
		if x[i] >= '0' && x[i] <= '9' && y[j] >= '0' && y[j] <= '9' {
			ii, jj := i, j
			for ii < len(x) && x[ii] >= '0' && x[ii] <= '9' {
				ii++
			}
			for jj < len(y) && y[jj] >= '0' && y[jj] <= '9' {
				jj++
			}
			ix, jy := i, j
			for ix < ii && x[ix] == '0' {
				ix++
			}
			for jy < jj && y[jy] == '0' {
				jy++
			}
			if ii-ix != jj-jy {
				return ii-ix < jj-jy
			}
			if sx, sy := string(x[ix:ii]), string(y[jy:jj]); sx != sy {
				return sx < sy
			}
			i, j = ii, jj
			continue
		}
		if unicode.ToLower(x[i]) != unicode.ToLower(y[j]) {
			return x[i] < y[j]
		}
		i++
		j++
	}
	if i < len(x) || j < len(y) {
		return i == len(x)
	}
	if len(x) != len(y) {
		return len(x) < len(y)
	}
	return a < b
}

func validName(name string) bool {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return false
	}
	if runtime.GOOS != "windows" {
		return true
	}
	if strings.ContainsAny(name, `<>:"|?*`) || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return false
	}
	for _, char := range name {
		if char < 32 {
			return false
		}
	}
	base, _, _ := strings.Cut(name, ".")
	base = strings.ToUpper(strings.TrimRight(base, " "))
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	return !(len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9')
}

func within(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
