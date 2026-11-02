package search

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"aurink/internal/contract"
)

const defaultMaxFileSize int64 = 5 << 20

// WorkspaceSearch searches files with a pool of CPU-count workers.
type WorkspaceSearch struct{}

var _ contract.WorkspaceSearch = (*WorkspaceSearch)(nil)

// NewWorkspaceSearch returns a searcher safe for concurrent use.
func NewWorkspaceSearch() *WorkspaceSearch { return &WorkspaceSearch{} }

type fileResult struct {
	matches contract.FileMatches
	err     error
}

// Search walks root and calls onBatch on the calling goroutine. Directory
// symlinks and other non-regular entries are not traversed.
func (s *WorkspaceSearch) Search(ctx context.Context, root string, q contract.Query, opts contract.SearchOptions, onBatch func([]contract.FileMatches)) error {
	const op = "search.WorkspaceSearch"
	if err := ctx.Err(); err != nil {
		return contract.E(contract.CodeCanceled, op, err)
	}
	compiled, err := compileQuery(q)
	if err != nil {
		return err
	}
	if opts.MaxFileSize < 0 || opts.PerFile < 0 {
		return contract.E(contract.CodeInvalid, op, nil)
	}
	if opts.MaxFileSize == 0 {
		opts.MaxFileSize = defaultMaxFileSize
	}
	info, err := os.Stat(root)
	if canceled := ctx.Err(); canceled != nil {
		return contract.E(contract.CodeCanceled, op, canceled)
	}
	if err != nil {
		return searchIOError(root, err)
	}
	if !info.IsDir() {
		return &contract.Error{Code: contract.CodeInvalid, Op: op, Path: root}
	}
	if q.Text == "" {
		return nil
	}
	compiled.streamRegexp()

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	workers := runtime.NumCPU()
	jobs := make(chan string, workers)
	results := make(chan fileResult, workers)
	walkErr := make(chan error, 1)
	go func() {
		defer close(jobs)
		walkErr <- walk(workCtx, root, opts.Ignore, jobs)
	}()
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-workCtx.Done():
					return
				case path, ok := <-jobs:
					if !ok {
						return
					}
					text, ok, err := readText(workCtx, path, opts.MaxFileSize)
					if err != nil {
						select {
						case results <- fileResult{err: err}:
						case <-workCtx.Done():
						}
						return
					}
					if workCtx.Err() != nil {
						return
					}
					if !ok {
						continue
					}
					matches := compiled.findContext(workCtx, text, opts.PerFile)
					if len(matches) != 0 {
						result := fileResult{matches: contract.FileMatches{Path: path, Matches: matches}}
						select {
						case results <- result:
						case <-workCtx.Done():
							return
						}
					}
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	for {
		select {
		case <-ctx.Done():
			return contract.E(contract.CodeCanceled, op, ctx.Err())
		case result, ok := <-results:
			if err := ctx.Err(); err != nil {
				return contract.E(contract.CodeCanceled, op, err)
			}
			if !ok {
				if err := <-walkErr; err != nil {
					return err
				}
				return nil
			}
			if result.err != nil {
				return result.err
			}
			batch := []contract.FileMatches{result.matches}
			// Coalesce results already available without delaying the first hit.
		collect:
			for len(batch) < 32 {
				select {
				case result, ok = <-results:
					if !ok {
						break collect
					}
					if result.err != nil {
						return result.err
					}
					batch = append(batch, result.matches)
				default:
					break collect
				}
			}
			if err := ctx.Err(); err != nil {
				return contract.E(contract.CodeCanceled, op, err)
			}
			if onBatch != nil {
				onBatch(batch)
			}
			if !ok {
				if err := ctx.Err(); err != nil {
					return contract.E(contract.CodeCanceled, op, err)
				}
				return <-walkErr
			}
		}
	}
}

func walk(ctx context.Context, root string, ignore func(string, bool) bool, jobs chan<- string) error {
	dirs := []string{root}
	for len(dirs) != 0 {
		if ctx.Err() != nil {
			return nil
		}
		dir := dirs[len(dirs)-1]
		dirs = dirs[:len(dirs)-1]
		entries, err := os.ReadDir(dir)
		if err != nil {
			return searchIOError(dir, err)
		}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return nil
			}
			path := filepath.Join(dir, entry.Name())
			isDir := entry.IsDir()
			if entry.Type()&os.ModeSymlink != 0 {
				// Follow file links, but not directory links or dangling links.
				info, err := os.Stat(path)
				if err != nil || !info.Mode().IsRegular() {
					continue
				}
			}
			if ignore != nil {
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return searchIOError(path, err)
				}
				if ignore(filepath.ToSlash(rel), isDir) {
					continue
				}
			}
			if isDir {
				dirs = append(dirs, path)
			} else if entry.Type().IsRegular() || entry.Type()&os.ModeSymlink != 0 {
				select {
				case jobs <- path:
				case <-ctx.Done():
					return nil
				}
			}
		}
	}
	return nil
}

func readText(ctx context.Context, path string, limit int64) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", false, searchIOError(path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", false, searchIOError(path, err)
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return "", false, nil
	}
	data, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return "", false, searchIOError(path, err)
	}
	if ctx.Err() != nil {
		return "", false, nil
	}
	if int64(len(data)) == limit {
		var extra [1]byte
		n, err := f.Read(extra[:])
		if n > 0 {
			return "", false, nil
		}
		if err != nil && err != io.EOF {
			return "", false, searchIOError(path, err)
		}
	}
	text, ok := decodeText(data)
	return text, ok, nil
}

// decodeText follows fsstore's BOM, surrogate, UTF-8 and NUL rules.
func decodeText(data []byte) (string, bool) {
	var order binary.ByteOrder
	switch {
	case bytes.HasPrefix(data, []byte{0xff, 0xfe}):
		order, data = binary.LittleEndian, data[2:]
	case bytes.HasPrefix(data, []byte{0xfe, 0xff}):
		order, data = binary.BigEndian, data[2:]
	case bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}):
		data = data[3:]
	}
	if order == nil {
		if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
			return "", false
		}
		return string(data), true
	}
	if len(data)%2 != 0 {
		return "", false
	}
	units := make([]uint16, len(data)/2)
	for i := range units {
		units[i] = order.Uint16(data[2*i:])
	}
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u >= 0xd800 && u <= 0xdbff {
			if i+1 == len(units) || units[i+1] < 0xdc00 || units[i+1] > 0xdfff {
				return "", false
			}
			i++
		} else if u >= 0xdc00 && u <= 0xdfff {
			return "", false
		}
	}
	text := string(utf16.Decode(units))
	return text, !strings.ContainsRune(text, 0)
}

func searchIOError(path string, err error) *contract.Error {
	code := contract.CodeIO
	if errors.Is(err, os.ErrNotExist) {
		code = contract.CodeNotFound
	} else if errors.Is(err, os.ErrPermission) {
		code = contract.CodePermission
	}
	return &contract.Error{Code: code, Op: "search.WorkspaceSearch", Path: path, Err: err}
}
