package app

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const (
	logMaxBytes = 5 << 20 // 5 MB per file
	logMaxFiles = 5       // aurink.log + aurink.log.1 .. aurink.log.4 (5×5 MB total)
)

// RollingLog is a goroutine-safe size-rotating file writer (03 §9, 09 D14).
type RollingLog struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	maxFiles int
	file     *os.File
	size     int64
}

// NewRollingLog opens path (creating parent directories) and rotates up to
// maxFiles total files of maxBytes each.
func NewRollingLog(path string, maxBytes int64, maxFiles int) (*RollingLog, error) {
	if maxBytes <= 0 {
		maxBytes = logMaxBytes
	}
	if maxFiles < 1 {
		maxFiles = logMaxFiles
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	r := &RollingLog{path: path, maxBytes: maxBytes, maxFiles: maxFiles}
	if err := r.open(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *RollingLog) open() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	r.file = f
	r.size = info.Size()
	return nil
}

// Write appends p to the active log file, rotating first if writing p would
// exceed maxBytes on a non-empty file.
func (r *RollingLog) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		if err := r.open(); err != nil {
			return 0, err
		}
	}
	if r.size > 0 && r.size+int64(len(p)) > r.maxBytes {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *RollingLog) rotate() error {
	if r.file != nil {
		_ = r.file.Close()
		r.file = nil
	}
	backups := r.maxFiles - 1
	if backups <= 0 {
		_ = os.Remove(r.path)
		return r.open()
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", r.path, backups))
	for i := backups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", r.path, i)
		dst := fmt.Sprintf("%s.%d", r.path, i+1)
		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(src, dst)
		}
	}
	if _, err := os.Stat(r.path); err == nil {
		_ = os.Rename(r.path, r.path+".1")
	}
	return r.open()
}

// Close closes the active log file.
func (r *RollingLog) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}

type bestEffortStderr struct {
	primary io.Writer
}

func (w bestEffortStderr) Write(p []byte) (int, error) {
	_, _ = os.Stderr.Write(p)
	return w.primary.Write(p)
}

func setupLogging(logPath string) *RollingLog {
	rl, err := NewRollingLog(logPath, logMaxBytes, logMaxFiles)
	if err != nil {
		slog.Warn("rolling file logger unavailable", "path", logPath, "error", err)
		return nil
	}
	handler := slog.NewTextHandler(bestEffortStderr{primary: rl}, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(handler))
	return rl
}
