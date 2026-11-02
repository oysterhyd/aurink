package watch

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"aurink/internal/contract"
	"github.com/egoist/mygo"
)

// Watcher starts MyGo file watches after the application is ready.
type Watcher struct{}

var _ contract.FileWatcher = Watcher{}

// New returns a file watcher. Watch must be called off the main thread.
func New() Watcher { return Watcher{} }

// Watch waits until the watch has enrolled the requested path.
func (Watcher) Watch(path string, opts contract.WatchOptions, fn func([]contract.WatchEvent, error)) (contract.Watch, error) {
	nativePath, err := watchPath(path)
	if err != nil {
		return nil, failure(path, err)
	}
	w, err := mygo.WatchFiles(context.Background(), nativePath, mygo.FileWatchOptions{
		Recursive: opts.Recursive, Ignore: opts.Ignore, MaxEntries: opts.MaxEntries,
	})
	if err != nil {
		return nil, failure(path, err)
	}
	s := &watch{native: w, path: path, fn: fn, done: make(chan struct{})}
	go s.run()
	return s, nil
}

type watch struct {
	native *mygo.FileWatcher
	path   string
	fn     func([]contract.WatchEvent, error)
	mu     sync.Mutex
	closed bool
	done   chan struct{}
}

func (w *watch) run() {
	defer close(w.done)
	gone := false
	for {
		batch, err := w.native.Next(context.Background())
		w.mu.Lock()
		if w.closed {
			w.mu.Unlock()
			return
		}
		if err != nil {
			if errors.Is(err, io.EOF) && gone {
				err = contract.E(contract.CodeNotFound, "watch", err)
			} else {
				err = failure(w.path, err)
			}
			w.fn(nil, err)
			w.mu.Unlock()
			return
		}
		out := make([]contract.WatchEvent, 0, len(batch))
		for _, event := range batch {
			var op contract.WatchOp
			switch event.Op {
			case mygo.FileCreate:
				op = contract.WatchCreate
			case mygo.FileWrite:
				op = contract.WatchWrite
			case mygo.FileRemove:
				op = contract.WatchRemove
				gone = gone || event.Path == "."
			case mygo.FileRename:
				op = contract.WatchRename
			case mygo.FileHint:
				op = contract.WatchOverflow
			default:
				continue
			}
			e := contract.WatchEvent{Op: op, Path: join(w.path, event.Path), IsDir: event.IsDir}
			if event.OldPath != "" {
				e.OldPath = join(w.path, event.OldPath)
			}
			out = append(out, e)
		}
		if len(out) != 0 {
			w.fn(out, nil)
		}
		w.mu.Unlock()
	}
}

func join(root, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel))
}

// Close waits for any callback in progress and releases the native watch.
func (w *watch) Close() {
	w.mu.Lock()
	if !w.closed {
		w.closed = true
		_ = w.native.Close()
	}
	w.mu.Unlock()
	<-w.done
	<-w.native.Done()
}

func failure(path string, err error) *contract.Error {
	code := contract.CodeIO
	switch {
	case errors.Is(err, mygo.ErrWatchLimit):
		code = contract.CodeTooLarge
	case errors.Is(err, os.ErrNotExist):
		code = contract.CodeNotFound
	}
	return &contract.Error{Code: code, Op: "watch", Path: path, Err: err}
}
