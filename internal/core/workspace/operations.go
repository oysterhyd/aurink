package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"aurink/internal/contract"
)

type operationResult struct {
	path   string
	events []contract.WorkspaceEvent
	err    error
}

func (w *Workspace) mutate(work func(string) operationResult, done func(operationResult)) {
	w.enqueue(func() {
		root, dirs, watches, degraded, gen := w.root, w.loadedDirs(), cloneWatches(w.watches), w.degraded, w.generation
		w.background(func() func() {
			result := work(root)
			update := w.reconcile(root, dirs, watches, degraded, gen, result.events, false)
			return func() {
				w.applyUpdate(update)
				for _, event := range result.events {
					w.emit(event)
				}
				done(result)
			}
		})
	})
}

// Create makes a file or directory without replacing an existing entry.
func (w *Workspace) Create(dir, name string, isDir bool, done func(string, error)) {
	w.mutate(func(root string) operationResult {
		const op = "workspace.Create"
		dir = filepath.Clean(dir)
		if !within(root, dir) || !validName(name) {
			return operationResult{err: failure(contract.CodeInvalid, op, dir, nil)}
		}
		path := filepath.Join(dir, name)
		var err error
		if isDir {
			err = os.Mkdir(path, 0o755)
		} else {
			var f *os.File
			f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o666)
			if err == nil {
				err = f.Close()
			}
		}
		if err != nil {
			return operationResult{err: ioFailure(op, path, err)}
		}
		return operationResult{path: path, events: []contract.WorkspaceEvent{{Kind: contract.WsCreated, Path: path, IsDir: isDir}}}
	}, func(result operationResult) {
		if done != nil {
			done(result.path, result.err)
		}
	})
}

// Rename changes one name in its current directory.
func (w *Workspace) Rename(path, newName string, done func(string, error)) {
	w.mutate(func(root string) operationResult {
		const op = "workspace.Rename"
		path = filepath.Clean(path)
		if !within(root, path) || samePath(root, path) || !validName(newName) {
			return operationResult{err: failure(contract.CodeInvalid, op, path, nil)}
		}
		info, err := os.Lstat(path)
		if err != nil {
			return operationResult{err: ioFailure(op, path, err)}
		}
		target := filepath.Join(filepath.Dir(path), newName)
		if path == target {
			return operationResult{path: path}
		}
		if err := checkTarget(path, target, info, op); err != nil {
			return operationResult{err: err}
		}
		if err := os.Rename(path, target); err != nil {
			return operationResult{err: ioFailure(op, path, err)}
		}
		return operationResult{path: target, events: []contract.WorkspaceEvent{{Kind: contract.WsRenamed, Path: target, OldPath: path, IsDir: info.IsDir()}}}
	}, func(result operationResult) {
		if done != nil {
			done(result.path, result.err)
		}
	})
}

// Move checks every source and destination before moving any of them.
func (w *Workspace) Move(paths []string, dstDir string, done func(error)) {
	paths = append([]string(nil), paths...)
	w.mutate(func(root string) operationResult {
		const op = "workspace.Move"
		dstDir = filepath.Clean(dstDir)
		if !within(root, dstDir) {
			return operationResult{err: failure(contract.CodeInvalid, op, dstDir, nil)}
		}
		dstInfo, err := os.Stat(dstDir)
		if err != nil {
			return operationResult{err: ioFailure(op, dstDir, err)}
		}
		if !dstInfo.IsDir() {
			return operationResult{err: failure(contract.CodeInvalid, op, dstDir, nil)}
		}
		infos := make([]os.FileInfo, len(paths))
		targets := make([]string, len(paths))
		for i, path := range paths {
			path = filepath.Clean(path)
			paths[i] = path
			if !within(root, path) || samePath(root, path) {
				return operationResult{err: failure(contract.CodeInvalid, op, path, nil)}
			}
			infos[i], err = os.Lstat(path)
			if err != nil {
				return operationResult{err: ioFailure(op, path, err)}
			}
			if infos[i].IsDir() && within(path, dstDir) {
				return operationResult{err: failure(contract.CodeInvalid, op, path, nil)}
			}
			targets[i] = filepath.Join(dstDir, filepath.Base(path))
			for j := 0; j < i; j++ {
				if samePath(paths[j], path) || samePath(targets[j], targets[i]) ||
					infos[j].IsDir() && within(paths[j], path) || infos[i].IsDir() && within(path, paths[j]) {
					return operationResult{err: failure(contract.CodeInvalid, op, path, nil)}
				}
			}
			if err := checkTarget(path, targets[i], infos[i], op); err != nil {
				return operationResult{err: err}
			}
		}
		var result operationResult
		for i, path := range paths {
			if err := os.Rename(path, targets[i]); err != nil {
				result.err = ioFailure(op, path, err)
				break
			}
			result.events = append(result.events, contract.WorkspaceEvent{Kind: contract.WsRenamed, Path: targets[i], OldPath: path, IsDir: infos[i].IsDir()})
		}
		return result
	}, func(result operationResult) { callDone(done, result.err) })
}

// Trash moves each item to the configured system trash.
func (w *Workspace) Trash(paths []string, done func(error)) {
	paths = append([]string(nil), paths...)
	w.mutate(func(root string) operationResult {
		const op = "workspace.Trash"
		var result operationResult
		for _, path := range paths {
			path = filepath.Clean(path)
			if !within(root, path) || samePath(root, path) {
				result.err = failure(contract.CodeInvalid, op, path, nil)
				break
			}
			info, err := os.Lstat(path)
			if err != nil {
				result.err = ioFailure(op, path, err)
				break
			}
			if err := w.trasher.Trash(path); err != nil {
				var ce *contract.Error
				if errors.As(err, &ce) {
					result.err = ce
				} else {
					result.err = ioFailure(op, path, err)
				}
				break
			}
			result.events = append(result.events, contract.WorkspaceEvent{Kind: contract.WsRemoved, Path: path, IsDir: info.IsDir()})
		}
		return result
	}, func(result operationResult) { callDone(done, result.err) })
}

func checkTarget(source, target string, sourceInfo os.FileInfo, op string) error {
	info, err := os.Lstat(target)
	if err == nil {
		// A case-only rename on Windows names the same entry.
		if runtime.GOOS == "windows" && source != target && samePath(source, target) && os.SameFile(sourceInfo, info) {
			return nil
		}
		return failure(contract.CodeExists, op, target, nil)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return ioFailure(op, target, err)
	}
	return nil
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
