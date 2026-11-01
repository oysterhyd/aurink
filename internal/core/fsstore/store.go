package fsstore

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"aurink/internal/contract"
)

const defaultMaxSize int64 = 64 << 20

// Store reads text files and saves them with their original encoding.
// Its zero value is ready to use. Writes through one Store are serialized.
type Store struct {
	mu  sync.Mutex
	ops fileOps
}

type fileOps struct {
	create  func(dir, pattern string) (*os.File, error)
	write   func(*os.File, []byte) error
	sync    func(*os.File) error
	replace func(target, replacement string) error
}

var _ contract.FileStore = (*Store)(nil)

// New returns a file store. Its blocking methods must run off the UI thread.
func New() *Store { return &Store{} }

// Read decodes a file without changing its newlines or including its BOM.
func (s *Store) Read(path string, opts contract.ReadOptions) (contract.FileContent, error) {
	const op = "fsstore.Read"
	limit := opts.MaxSize
	if limit < 0 {
		return contract.FileContent{}, failure(contract.CodeInvalid, op, path, nil)
	}
	if limit == 0 {
		limit = defaultMaxSize
	}
	f, err := os.Open(path)
	if err != nil {
		return contract.FileContent{}, ioFailure(op, path, err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return contract.FileContent{}, ioFailure(op, path, err)
	}
	if !info.Mode().IsRegular() {
		return contract.FileContent{}, failure(contract.CodeInvalid, op, path, nil)
	}
	if info.Size() > limit {
		return contract.FileContent{}, failure(contract.CodeTooLarge, op, path, nil)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit))
	if err == nil && int64(len(data)) == limit {
		// Check growth beyond the stat size without overflowing limit + 1.
		var extra [1]byte
		var n int
		n, err = f.Read(extra[:])
		if n != 0 {
			return contract.FileContent{}, failure(contract.CodeTooLarge, op, path, nil)
		}
		if err == io.EOF {
			err = nil
		}
	}
	if err != nil {
		return contract.FileContent{}, ioFailure(op, path, err)
	}
	text, meta, code := decode(data, opts.AllowBinary)
	if code != "" {
		return contract.FileContent{}, failure(code, op, path, nil)
	}
	return contract.FileContent{Path: path, Text: text, Meta: meta, Stamp: stampOf(info)}, nil
}

// Stat returns the file's modification time and byte size.
func (s *Store) Stat(path string) (contract.FileStamp, error) {
	info, err := os.Stat(path)
	if err != nil {
		return contract.FileStamp{}, ioFailure("fsstore.Stat", path, err)
	}
	return stampOf(info), nil
}

// Write saves text using meta's encoding and BOM, preserving existing permissions.
// A non-nil expect must match both size and modification time. Hard links and
// files in directories denying temp creation are written in place, as in MyGo;
// that fallback cannot provide the crash atomicity of replacement.
func (s *Store) Write(path, text string, meta contract.DocMeta, expect *contract.FileStamp) (contract.FileStamp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	const op = "fsstore.Write"
	target := path
	if real, err := filepath.EvalSymlinks(path); err == nil {
		target = real
	} else if !errors.Is(err, os.ErrNotExist) {
		return contract.FileStamp{}, ioFailure(op, path, err)
	}
	info, err := os.Stat(target)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return contract.FileStamp{}, ioFailure(op, path, err)
	}
	if expect != nil && (info == nil || info.Size() != expect.Size || !info.ModTime().Equal(expect.ModTime)) {
		return contract.FileStamp{}, failure(contract.CodeConflict, op, path, nil)
	}
	data, err := encode(text, meta)
	if err != nil {
		return contract.FileStamp{}, failure(contract.CodeEncoding, op, path, err)
	}
	if info != nil {
		if !info.Mode().IsRegular() {
			return contract.FileStamp{}, failure(contract.CodeInvalid, op, path, nil)
		}
		// A writable directory must not let replacement bypass a read-only file.
		if info.Mode().Perm()&0o222 == 0 {
			return contract.FileStamp{}, failure(contract.CodePermission, op, path, os.ErrPermission)
		}
		f, err := os.OpenFile(target, os.O_WRONLY, 0)
		if err != nil {
			return contract.FileStamp{}, ioFailure(op, path, err)
		}
		if err := f.Close(); err != nil {
			return contract.FileStamp{}, ioFailure(op, path, err)
		}
	}
	if err := s.writeSafely(target, data, info); err != nil {
		return contract.FileStamp{}, ioFailure(op, path, err)
	}
	info, err = os.Stat(target)
	if err != nil {
		return contract.FileStamp{}, ioFailure(op, path, err)
	}
	return stampOf(info), nil
}

func (s *Store) writeSafely(path string, data []byte, info os.FileInfo) error {
	if info != nil {
		linked, err := multipleLinks(path, info)
		if err != nil {
			return err
		}
		if linked {
			return writeInPlace(path, data)
		}
	}
	ops := s.ops
	if ops.create == nil {
		ops.create = createSibling
	}
	if ops.write == nil {
		ops.write = writeBytes
	}
	if ops.sync == nil {
		ops.sync = (*os.File).Sync
	}
	if ops.replace == nil {
		ops.replace = replaceFile
	}
	f, err := ops.create(filepath.Dir(path), ".aurink-*.tmp")
	if err != nil {
		if info != nil && errors.Is(err, os.ErrPermission) {
			return writeInPlace(path, data)
		}
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	err = ops.write(f, data)
	if err == nil {
		err = preserveMode(f, info)
	}
	if err == nil {
		err = ops.sync(f)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return ops.replace(path, name)
}

func createSibling(dir, _ string) (*os.File, error) {
	// Unlike CreateTemp's 0600, new documents use 0666 filtered by the umask.
	return os.OpenFile(filepath.Join(dir, ".aurink-"+rand.Text()+".tmp"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
}

func writeInPlace(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return err
	}
	err = writeBytes(f, data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func writeBytes(f *os.File, data []byte) error {
	n, err := f.Write(data)
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}
	return err
}

func stampOf(info os.FileInfo) contract.FileStamp {
	return contract.FileStamp{ModTime: info.ModTime(), Size: info.Size()}
}

func failure(code contract.Code, op, path string, err error) *contract.Error {
	return &contract.Error{Code: code, Op: op, Path: path, Err: err}
}

func ioFailure(op, path string, err error) *contract.Error {
	code := contract.CodeIO
	switch {
	case errors.Is(err, os.ErrNotExist):
		code = contract.CodeNotFound
	case errors.Is(err, os.ErrPermission):
		code = contract.CodePermission
	case errors.Is(err, os.ErrExist):
		code = contract.CodeExists
	}
	return failure(code, op, path, err)
}
