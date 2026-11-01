package mock

import (
	"bytes"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"aurink/internal/contract"
)

const defaultMaxSize = 64 << 20

// FileStore is an in-memory contract.FileStore. Errors holds the error to
// return for a path; a test sets it to inject a failure.
type FileStore struct {
	mu     sync.Mutex
	files  map[string]memFile
	clock  time.Time
	Errors map[string]error
}

type memFile struct {
	data  []byte
	stamp contract.FileStamp
}

// NewFileStore returns an empty store.
func NewFileStore() *FileStore {
	return &FileStore{
		files:  map[string]memFile{},
		clock:  time.Unix(1_700_000_000, 0),
		Errors: map[string]error{},
	}
}

// Put writes bytes as an external program would.
func (s *FileStore) Put(path string, data []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.put(path, bytes.Clone(data))
}

// Bytes returns the bytes of a file.
func (s *FileStore) Bytes(path string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.files[path]
	return bytes.Clone(f.data), ok
}

func (s *FileStore) put(path string, data []byte) contract.FileStamp {
	s.clock = s.clock.Add(time.Second)
	stamp := contract.FileStamp{ModTime: s.clock, Size: int64(len(data))}
	s.files[path] = memFile{data, stamp}
	return stamp
}

func (s *FileStore) Read(path string, opts contract.ReadOptions) (contract.FileContent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.lookup("mock.FileStore.Read", path)
	if err != nil {
		return contract.FileContent{}, err
	}
	limit := opts.MaxSize
	if limit == 0 {
		limit = defaultMaxSize
	}
	if int64(len(f.data)) > limit {
		return contract.FileContent{}, &contract.Error{Code: contract.CodeTooLarge, Op: "mock.FileStore.Read", Path: path}
	}
	text, meta := decode(f.data)
	if !opts.AllowBinary && meta.Encoding == "utf-8" && bytes.IndexByte(f.data, 0) >= 0 {
		return contract.FileContent{}, &contract.Error{Code: contract.CodeBinary, Op: "mock.FileStore.Read", Path: path}
	}
	return contract.FileContent{Path: path, Text: text, Meta: meta, Stamp: f.stamp}, nil
}

func (s *FileStore) Write(path, text string, meta contract.DocMeta, expect *contract.FileStamp) (contract.FileStamp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Errors[path]; err != nil {
		return contract.FileStamp{}, err
	}
	if expect != nil {
		f, ok := s.files[path]
		if !ok || !f.stamp.ModTime.Equal(expect.ModTime) || f.stamp.Size != expect.Size {
			return contract.FileStamp{}, &contract.Error{Code: contract.CodeConflict, Op: "mock.FileStore.Write", Path: path}
		}
	}
	return s.put(path, encode(text, meta)), nil
}

func (s *FileStore) Stat(path string) (contract.FileStamp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.lookup("mock.FileStore.Stat", path)
	return f.stamp, err
}

func (s *FileStore) lookup(op, path string) (memFile, error) {
	if err := s.Errors[path]; err != nil {
		return memFile{}, err
	}
	f, ok := s.files[path]
	if !ok {
		return memFile{}, &contract.Error{Code: contract.CodeNotFound, Op: op, Path: path}
	}
	return f, nil
}

func decode(data []byte) (string, contract.DocMeta) {
	meta := contract.DocMeta{Encoding: "utf-8"}
	var text string
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xFE}):
		meta.BOM, meta.Encoding = true, "utf-16le"
		text = fromUTF16(data[2:], false)
	case bytes.HasPrefix(data, []byte{0xFE, 0xFF}):
		meta.BOM, meta.Encoding = true, "utf-16be"
		text = fromUTF16(data[2:], true)
	case bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}):
		meta.BOM = true
		text = string(data[3:])
	default:
		text = string(data)
	}
	crlf := strings.Count(text, "\r\n")
	lf := strings.Count(text, "\n") - crlf
	switch {
	case crlf > 0 && lf > 0:
		meta.EOL = contract.EOLMixed
	case crlf > 0:
		meta.EOL = contract.EOLCRLF
	}
	meta.Dominant = "\n"
	if crlf > lf {
		meta.Dominant = "\r\n"
	}
	return text, meta
}

func encode(text string, meta contract.DocMeta) []byte {
	var out []byte
	switch meta.Encoding {
	case "utf-16le", "utf-16be":
		be := meta.Encoding == "utf-16be"
		if meta.BOM {
			out = putUnit(out, 0xFEFF, be)
		}
		for _, u := range utf16.Encode([]rune(text)) {
			out = putUnit(out, u, be)
		}
	default:
		if meta.BOM {
			out = append(out, 0xEF, 0xBB, 0xBF)
		}
		out = append(out, text...)
	}
	return out
}

func putUnit(out []byte, u uint16, be bool) []byte {
	if be {
		return append(out, byte(u>>8), byte(u))
	}
	return append(out, byte(u), byte(u>>8))
}

func fromUTF16(data []byte, be bool) string {
	units := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		if be {
			units = append(units, uint16(data[i])<<8|uint16(data[i+1]))
		} else {
			units = append(units, uint16(data[i+1])<<8|uint16(data[i]))
		}
	}
	return string(utf16.Decode(units))
}
