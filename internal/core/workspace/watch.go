package workspace

import (
	"log/slog"
	"path/filepath"
	"sort"

	"aurink/internal/contract"
)

func (w *Workspace) startWatch(path string, recursive bool, gen uint64) (contract.Watch, error) {
	limit := w.opts.MaxEntries
	if !recursive {
		// A loaded directory may itself exceed both the recursive limit and
		// the backend default. It must remain watched after degradation.
		limit = int(^uint(0) >> 1)
	}
	return w.watcher.Watch(path, contract.WatchOptions{Recursive: recursive, MaxEntries: limit,
		Ignore: func(rel string, isDir bool) bool { return ignored(rel) }}, func(batch []contract.WatchEvent, err error) {
		result := watchResult{gen: gen, batch: append([]contract.WatchEvent(nil), batch...), err: err}
		w.dispatcher.Post(func() { w.watchResult(result) })
	})
}

func (w *Workspace) watchResult(result watchResult) {
	if result.gen == w.setting {
		w.pending = append(w.pending, result)
		return
	}
	if result.gen != w.generation || w.root == "" {
		return
	}
	if result.err != nil {
		if contract.CodeOf(result.err) == contract.CodeTooLarge && !w.degraded {
			w.degrade(result.gen)
		} else if contract.CodeOf(result.err) != contract.CodeNotFound {
			slog.Error("workspace watch ended", "error", result.err)
		}
		return
	}
	var events []contract.WorkspaceEvent
	overflow := false
	for _, change := range result.batch {
		if change.Op == contract.WatchOverflow {
			overflow = true
			continue
		}
		if !within(w.root, change.Path) {
			continue
		}
		rel, err := filepath.Rel(w.root, change.Path)
		if err != nil || ignored(rel) {
			continue
		}
		var kind contract.WorkspaceEventKind
		switch change.Op {
		case contract.WatchCreate:
			kind = contract.WsCreated
		case contract.WatchWrite:
			kind = contract.WsModified
		case contract.WatchRemove:
			kind = contract.WsRemoved
		case contract.WatchRename:
			kind = contract.WsRenamed
		default:
			continue
		}
		events = append(events, contract.WorkspaceEvent{Kind: kind, Path: filepath.Clean(change.Path), OldPath: change.OldPath, IsDir: change.IsDir})
	}
	if len(events) == 0 && !overflow {
		return
	}
	w.enqueue(func() {
		if result.gen != w.generation {
			w.background(func() func() { return func() {} })
			return
		}
		root, dirs, watches, degraded := w.root, w.loadedDirs(), cloneWatches(w.watches), w.degraded
		w.background(func() func() {
			update := w.reconcile(root, dirs, watches, degraded, result.gen, events, overflow)
			return func() {
				w.applyUpdate(update)
				if overflow {
					w.emitListings(update.entries)
				}
				for _, event := range events {
					w.emit(event)
				}
			}
		})
	})
}

func (w *Workspace) degrade(gen uint64) {
	w.enqueue(func() {
		if gen != w.generation || w.degraded {
			w.background(func() func() { return func() {} })
			return
		}
		w.degraded = true
		root, dirs, old := w.root, w.loadedDirs(), cloneWatches(w.watches)
		loaded := append([]string(nil), dirs...)
		w.emit(contract.WorkspaceEvent{Kind: contract.WsDegraded, Path: root, IsDir: true})
		w.background(func() func() {
			closeWatches(old)
			if !contains(dirs, root) {
				dirs = append(dirs, root)
			}
			watches := make(map[string]contract.Watch)
			for _, dir := range dirs {
				watch, err := w.startWatch(dir, false, gen)
				if err != nil {
					slog.Error("watching directory after degradation", "path", dir, "error", err)
					continue
				}
				watches[dir] = watch
			}
			update := w.reconcile(root, loaded, nil, false, gen, nil, true)
			return func() {
				w.watches = watches
				w.applyUpdate(update)
				w.emitListings(update.entries)
			}
		})
	})
}

type cacheUpdate struct {
	removeDirs    []string
	entries       map[string][]contract.Entry
	removeWatches []string
	addWatches    map[string]contract.Watch
}

// reconcile reads affected loaded directories after a change. Loaded subtrees
// move with renamed folders; removed subtrees and their watches are discarded.
func (w *Workspace) reconcile(root string, dirs []string, watches map[string]contract.Watch, degraded bool, gen uint64, events []contract.WorkspaceEvent, overflow bool) cacheUpdate {
	u := cacheUpdate{entries: make(map[string][]contract.Entry), addWatches: make(map[string]contract.Watch)}
	read := make(map[string]bool)
	for _, dir := range dirs {
		mapped, removed := mappedDir(dir, events)
		if removed || mapped != dir {
			u.removeDirs = append(u.removeDirs, dir)
		}
		if !removed && (mapped != dir || overflow || affectedParent(dir, events)) {
			read[mapped] = true
		}
	}
	if degraded {
		for dir, watch := range watches {
			mapped, removed := mappedDir(dir, events)
			if !removed && mapped == dir {
				continue
			}
			watch.Close()
			u.removeWatches = append(u.removeWatches, dir)
			if !removed {
				if next, err := w.startWatch(mapped, false, gen); err == nil {
					u.addWatches[mapped] = next
				} else {
					slog.Error("watching moved directory", "path", mapped, "error", err)
				}
			}
		}
	}
	for dir := range read {
		if entries, err := readDirectory(dir, root); err == nil {
			u.entries[dir] = entries
		} else {
			u.removeDirs = append(u.removeDirs, dir)
		}
	}
	return u
}

func mappedDir(dir string, events []contract.WorkspaceEvent) (string, bool) {
	for _, e := range events {
		if !e.IsDir {
			continue
		}
		switch e.Kind {
		case contract.WsRemoved:
			if within(e.Path, dir) {
				return "", true
			}
		case contract.WsRenamed:
			if within(e.OldPath, dir) {
				rel, _ := filepath.Rel(e.OldPath, dir)
				dir = filepath.Join(e.Path, rel)
			}
		}
	}
	return dir, false
}

func affectedParent(dir string, events []contract.WorkspaceEvent) bool {
	for _, e := range events {
		if filepath.Dir(e.Path) == dir || e.OldPath != "" && filepath.Dir(e.OldPath) == dir {
			return true
		}
	}
	return false
}

func (w *Workspace) applyUpdate(u cacheUpdate) {
	for _, dir := range u.removeDirs {
		delete(w.cache, dir)
	}
	for dir, entries := range u.entries {
		w.cache[dir] = entries
	}
	for _, dir := range u.removeWatches {
		delete(w.watches, dir)
	}
	for dir, watch := range u.addWatches {
		w.watches[dir] = watch
	}
}

func (w *Workspace) emitListings(entries map[string][]contract.Entry) {
	dirs := make([]string, 0, len(entries))
	for dir := range entries {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		w.emit(contract.WorkspaceEvent{Kind: contract.WsListed, Path: dir, IsDir: true})
	}
}

func cloneWatches(in map[string]contract.Watch) map[string]contract.Watch {
	out := make(map[string]contract.Watch, len(in))
	for dir, watch := range in {
		out[dir] = watch
	}
	return out
}

func closeWatches(watches map[string]contract.Watch) {
	for _, watch := range watches {
		watch.Close()
	}
}

func contains(paths []string, path string) bool {
	i := sort.SearchStrings(paths, path)
	return i < len(paths) && paths[i] == path
}
