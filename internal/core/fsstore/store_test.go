package fsstore

import (
	"bytes"
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"aurink/internal/contract"
)

func put(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func fileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func wantError(t *testing.T, err error, code contract.Code, op, path string) {
	t.Helper()
	e, ok := err.(*contract.Error)
	if !ok || e.Code != code || e.Op != "fsstore."+op || e.Path != path {
		t.Fatalf("error = %#v, want %s, fsstore.%s, %q", err, code, op, path)
	}
}

func wantBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	if got := fileBytes(t, path); !bytes.Equal(got, want) {
		t.Fatalf("bytes differ at %s: got %x, want %x", path, got, want)
	}
}

func wantOnlyFile(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		t.Fatalf("unexpected files after save: %v", entries)
	}
}

func TestRead(t *testing.T) {
	for _, tt := range []struct {
		name, data, text, encoding, dominant string
		bom                                  bool
		eol                                  contract.EOLStyle
	}{
		{"empty", "", "", "utf-8", "\n", false, contract.EOLLF},
		{"lf", "一\n二\n", "一\n二\n", "utf-8", "\n", false, contract.EOLLF},
		{"crlf", "a\r\nb\r\n", "a\r\nb\r\n", "utf-8", "\r\n", false, contract.EOLCRLF},
		{"mixed lf", "a\nb\r\nc\n", "a\nb\r\nc\n", "utf-8", "\n", false, contract.EOLMixed},
		{"mixed crlf", "a\r\nb\nc\r\n", "a\r\nb\nc\r\n", "utf-8", "\r\n", false, contract.EOLMixed},
		{"mixed tie", "a\nb\r\n", "a\nb\r\n", "utf-8", "\n", false, contract.EOLMixed},
		{"bare cr", "a\rb\r", "a\rb\r", "utf-8", "\n", false, contract.EOLLF},
		{"no final newline", "a\nb ", "a\nb ", "utf-8", "\n", false, contract.EOLLF},
		{"bom", "\xef\xbb\xbf# 标题\n", "# 标题\n", "utf-8", "\n", true, contract.EOLLF},
		{"bom only", "\xef\xbb\xbf", "", "utf-8", "\n", true, contract.EOLLF},
		{"interior bom", "\xef\xbb\xbf\xef\xbb\xbf", "\ufeff", "utf-8", "\n", true, contract.EOLLF},
		{"utf16le", "\xff\xfea\x00\r\x00\n\x00", "a\r\n", "utf-16le", "\r\n", true, contract.EOLCRLF},
		{"utf16be", "\xfe\xff\x00a\x00\n", "a\n", "utf-16be", "\n", true, contract.EOLLF},
		{"surrogate le", "\xff\xfe\x3d\xd8\x00\xde", "😀", "utf-16le", "\n", true, contract.EOLLF},
		{"surrogate be", "\xfe\xff\xd8\x3d\xde\x00", "😀", "utf-16be", "\n", true, contract.EOLLF},
		{"replacement rune", "\xff\xfe\xfd\xff", "�", "utf-16le", "\n", true, contract.EOLLF},
		{"utf16 empty", "\xff\xfe", "", "utf-16le", "\n", true, contract.EOLLF},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "doc.md")
			put(t, path, []byte(tt.data))
			s := New()
			got, err := s.Read(path, contract.ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			meta := contract.DocMeta{Encoding: tt.encoding, BOM: tt.bom, EOL: tt.eol, Dominant: tt.dominant}
			if got.Path != path || got.Text != tt.text || got.Meta != meta || got.Stamp.Size != int64(len(tt.data)) {
				t.Fatalf("Read = %+v, want text %q, meta %+v", got, tt.text, meta)
			}
			stamp, err := s.Write(path, got.Text, got.Meta, &got.Stamp)
			if err != nil {
				t.Fatal(err)
			}
			wantBytes(t, path, []byte(tt.data))
			now, err := s.Stat(path)
			if err != nil || !stamp.ModTime.Equal(now.ModTime) || stamp.Size != now.Size {
				t.Fatalf("Write stamp %+v, Stat %+v, %v", stamp, now, err)
			}
		})
	}
}

func TestInvalidText(t *testing.T) {
	for _, tt := range []struct {
		name, data string
		allow      bool
		code       contract.Code
	}{
		{"nul", "a\x00b", false, contract.CodeBinary},
		{"allowed nul", "a\x00b", true, ""},
		{"binary ratio", "\xff\x80\xfeabc", false, contract.CodeBinary},
		{"allowed invalid binary", "\xff\x80\xfeabc", true, contract.CodeEncoding},
		{"invalid utf8", "hello\xffworld", false, contract.CodeEncoding},
		{"invalid bom utf8", "\xef\xbb\xbfhello\xffworld", false, contract.CodeEncoding},
		{"odd utf16", "\xff\xfea", false, contract.CodeEncoding},
		{"unpaired high", "\xff\xfe\x00\xd8", false, contract.CodeEncoding},
		{"unpaired low", "\xfe\xff\xdc\x00", false, contract.CodeEncoding},
		{"bad pair", "\xff\xfe\x00\xd8a\x00", false, contract.CodeEncoding},
		{"utf16 nul", "\xff\xfe\x00\x00", false, contract.CodeBinary},
		{"allowed utf16 nul", "\xff\xfe\x00\x00", true, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "doc.md")
			put(t, path, []byte(tt.data))
			got, err := New().Read(path, contract.ReadOptions{AllowBinary: tt.allow})
			if tt.code != "" {
				wantError(t, err, tt.code, "Read", path)
			} else if err != nil || !strings.ContainsRune(got.Text, 0) {
				t.Fatalf("Read = %+v, %v", got, err)
			}
		})
	}
}

func TestSizeLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	put(t, path, []byte("12345"))
	s := New()
	for _, size := range []int64{5, 6, math.MaxInt64} {
		if _, err := s.Read(path, contract.ReadOptions{MaxSize: size}); err != nil {
			t.Fatal(err)
		}
	}
	_, err := s.Read(path, contract.ReadOptions{MaxSize: 4})
	wantError(t, err, contract.CodeTooLarge, "Read", path)
	_, err = s.Read(path, contract.ReadOptions{MaxSize: -1})
	wantError(t, err, contract.CodeInvalid, "Read", path)
	if err := os.Truncate(path, defaultMaxSize+1); err != nil {
		t.Fatal(err)
	}
	_, err = s.Read(path, contract.ReadOptions{})
	wantError(t, err, contract.CodeTooLarge, "Read", path)
}

func TestCorpusFidelity(t *testing.T) {
	// tests/arch disallows core tests importing aurink/tests/corpus.
	root := filepath.Join("..", "..", "..", "testdata", "corpus")
	dest := t.TempDir()
	s := New()
	count := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		count++
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		t.Run(filepath.ToSlash(rel), func(t *testing.T) {
			original := fileBytes(t, path)
			copyPath := filepath.Join(dest, rel)
			if err := os.MkdirAll(filepath.Dir(copyPath), 0o755); err != nil {
				t.Fatal(err)
			}
			put(t, copyPath, original)
			got, err := s.Read(copyPath, contract.ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Write(copyPath, got.Text, got.Meta, &got.Stamp); err != nil {
				t.Fatal(err)
			}
			wantBytes(t, copyPath, original)
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("empty corpus")
	}
	t.Logf("byte identity: %d/%d corpus files", count, count)
}

func TestWriteEncodings(t *testing.T) {
	for _, tt := range []struct {
		encoding, want string
		bom            bool
	}{
		{"", "a😀\r\n", false},
		{"utf-8", "\xef\xbb\xbfa😀\r\n", true},
		{"utf-16le", "a\x00\x3d\xd8\x00\xde\r\x00\n\x00", false},
		{"utf-16be", "\x00a\xd8\x3d\xde\x00\x00\r\x00\n", false},
		{"utf-16le", "\xff\xfea\x00\x3d\xd8\x00\xde\r\x00\n\x00", true},
		{"utf-16be", "\xfe\xff\x00a\xd8\x3d\xde\x00\x00\r\x00\n", true},
	} {
		t.Run(tt.encoding+tt.want[:1], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "new.md")
			var s Store
			// EOL describes the text; it must not cause normalization on write.
			meta := contract.DocMeta{Encoding: tt.encoding, BOM: tt.bom, EOL: contract.EOLLF, Dominant: "\n"}
			if _, err := s.Write(path, "a😀\r\n", meta, nil); err != nil {
				t.Fatal(err)
			}
			wantBytes(t, path, []byte(tt.want))
			wantOnlyFile(t, path)
		})
	}
}

func TestWriteInvalidEncoding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	put(t, path, []byte("original"))
	for _, tt := range []struct{ text, encoding string }{{"\xff", "utf-8"}, {"ok", "latin1"}, {"\xff", "utf-16be"}} {
		_, err := New().Write(path, tt.text, contract.DocMeta{Encoding: tt.encoding}, nil)
		wantError(t, err, contract.CodeEncoding, "Write", path)
		wantBytes(t, path, []byte("original"))
		wantOnlyFile(t, path)
	}
}

func TestConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	s := New()
	put(t, path, []byte("old"))
	stamp, err := s.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Equal instants compare equal even with a different location.
	stamp.ModTime = stamp.ModTime.In(time.FixedZone("other", 3600))
	if _, err := s.Write(path, "new", contract.DocMeta{}, &stamp); err != nil {
		t.Fatal(err)
	}
	stamp, err = s.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := stamp.ModTime.Add(-time.Hour)
	if err := os.Chtimes(path, changed, changed); err != nil {
		t.Fatal(err)
	}
	_, err = s.Write(path, "mine", contract.DocMeta{}, &stamp)
	wantError(t, err, contract.CodeConflict, "Write", path)
	wantBytes(t, path, []byte("new"))
	put(t, path, []byte("external"))
	_, err = s.Write(path, "mine", contract.DocMeta{}, &stamp)
	wantError(t, err, contract.CodeConflict, "Write", path)
	wantBytes(t, path, []byte("external"))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	_, err = s.Write(path, "mine", contract.DocMeta{}, &stamp)
	wantError(t, err, contract.CodeConflict, "Write", path)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("conflict created a file: %v", err)
	}
	if _, err := s.Write(path, "mine", contract.DocMeta{}, nil); err != nil {
		t.Fatal(err)
	}
	wantBytes(t, path, []byte("mine"))
}

func TestConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	put(t, path, []byte("original"))
	s := New()
	stamp, err := s.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			_, err := s.Write(path, "new", contract.DocMeta{}, &stamp)
			results <- err
		})
	}
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for err := range results {
		if err == nil {
			ok++
		} else {
			wantError(t, err, contract.CodeConflict, "Write", path)
			conflict++
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("successful = %d, conflicts = %d", ok, conflict)
	}
	wantBytes(t, path, []byte("new"))
}

func TestWriteFailures(t *testing.T) {
	for _, stage := range []string{"write", "sync", "replace"} {
		t.Run(stage, func(t *testing.T) {
			// Write resolves symlinks, and the temp dir can be behind one.
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "doc.md")
			original := []byte("original\r\n")
			put(t, path, original)
			before, err := New().Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			s := New()
			injected := errors.New("injected " + stage)
			called := false
			switch stage {
			case "write":
				s.ops.write = func(f *os.File, data []byte) error {
					called = true
					if filepath.Dir(f.Name()) != filepath.Dir(path) {
						t.Fatal("temporary file is not a sibling")
					}
					if _, err := f.Write(data[:len(data)/2]); err != nil {
						t.Fatal(err)
					}
					return injected
				}
			case "sync":
				s.ops.sync = func(f *os.File) error {
					called = true
					wantBytes(t, f.Name(), []byte("replacement"))
					return injected
				}
			case "replace":
				s.ops.replace = func(target, tmp string) error {
					called = true
					if target != path {
						t.Fatalf("replace target = %q", target)
					}
					wantBytes(t, tmp, []byte("replacement"))
					return injected
				}
			}
			_, err = s.Write(path, "replacement", contract.DocMeta{}, &before)
			wantError(t, err, contract.CodeIO, "Write", path)
			if !called || !errors.Is(err, injected) {
				t.Fatalf("injection was not returned: %v", err)
			}
			wantBytes(t, path, original)
			wantOnlyFile(t, path)
			after, err := s.Stat(path)
			if err != nil || !before.ModTime.Equal(after.ModTime) || before.Size != after.Size {
				t.Fatalf("failure changed stamp: before %+v, after %+v, %v", before, after, err)
			}
		})
	}
}

func TestTempCreationFailures(t *testing.T) {
	for _, tt := range []struct {
		name   string
		exists bool
		err    error
		code   contract.Code
	}{
		{"permission fallback", true, os.ErrPermission, ""},
		{"missing permission", false, os.ErrPermission, contract.CodePermission},
		{"io", true, errors.New("disk full"), contract.CodeIO},
		{"exists", true, os.ErrExist, contract.CodeExists},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "doc.md")
			if tt.exists {
				put(t, path, []byte("old"))
			}
			s := New()
			called := false
			s.ops.create = func(dir, pattern string) (*os.File, error) {
				called = true
				return nil, &os.PathError{Op: "create", Path: filepath.Join(dir, pattern), Err: tt.err}
			}
			_, err := s.Write(path, "replacement", contract.DocMeta{}, nil)
			if !called {
				t.Fatal("temp creation was not attempted")
			}
			if tt.code == "" {
				if err != nil {
					t.Fatal(err)
				}
				wantBytes(t, path, []byte("replacement"))
			} else {
				wantError(t, err, tt.code, "Write", path)
				if tt.exists {
					wantBytes(t, path, []byte("old"))
				}
			}
		})
	}
}

func TestHardLink(t *testing.T) {
	dir := t.TempDir()
	path, link := filepath.Join(dir, "doc.md"), filepath.Join(dir, "linked.md")
	put(t, path, []byte("original content"))
	if err := os.Link(path, link); err != nil {
		t.Fatal(err)
	}
	s := New()
	s.ops.create = func(string, string) (*os.File, error) {
		t.Fatal("hard-linked file must be written in place")
		return nil, nil
	}
	if _, err := s.Write(path, "short", contract.DocMeta{}, nil); err != nil {
		t.Fatal(err)
	}
	wantBytes(t, path, []byte("short"))
	wantBytes(t, link, []byte("short"))
	a, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(link)
	if err != nil || !os.SameFile(a, b) {
		t.Fatalf("hard link identity changed: %v", err)
	}
}

func TestReadOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	put(t, path, []byte("original"))
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0o644); err != nil {
			t.Error(err)
		}
	})
	s := New()
	if _, err := s.Read(path, contract.ReadOptions{}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Write(path, "replacement", contract.DocMeta{}, nil)
	wantError(t, err, contract.CodePermission, "Write", path)
	wantBytes(t, path, []byte("original"))
	wantOnlyFile(t, path)
}

func TestPathErrors(t *testing.T) {
	s := New()
	path := filepath.Join(t.TempDir(), "missing", "doc.md")
	_, err := s.Read(path, contract.ReadOptions{})
	wantError(t, err, contract.CodeNotFound, "Read", path)
	_, err = s.Stat(path)
	wantError(t, err, contract.CodeNotFound, "Stat", path)
	_, err = s.Write(path, "text", contract.DocMeta{}, nil)
	wantError(t, err, contract.CodeNotFound, "Write", path)
	dir := t.TempDir()
	_, err = s.Read(dir, contract.ReadOptions{})
	wantError(t, err, contract.CodeInvalid, "Read", dir)
	_, err = s.Write(dir, "text", contract.DocMeta{}, nil)
	wantError(t, err, contract.CodeInvalid, "Write", dir)
}

// Another program writing while the replacement is being prepared is a conflict, and
// its text stays (07 §8).
func TestWriteChecksTheStampAgainBeforeReplacing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	put(t, path, []byte("original"))
	s := New()
	before, err := s.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	s.ops.sync = func(f *os.File) error {
		put(t, path, []byte("theirs, written meanwhile"))
		return f.Sync()
	}
	_, err = s.Write(path, "mine", contract.DocMeta{}, &before)
	wantError(t, err, contract.CodeConflict, "Write", path)
	wantBytes(t, path, []byte("theirs, written meanwhile"))
	wantOnlyFile(t, path)
}
