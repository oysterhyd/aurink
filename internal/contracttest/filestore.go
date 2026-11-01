package contracttest

import (
	"bytes"
	"testing"

	"aurink/internal/contract"
)

// FileStoreFixture is a store with a way to touch its files from outside it.
type FileStoreFixture struct {
	Store contract.FileStore
	// Path returns the path of a file in the store's scratch area.
	Path func(name string) string
	// Put writes bytes as an external program would, and must change the file's
	// stamp when its size changes.
	Put func(path string, data []byte)
	// Bytes returns the bytes of a file.
	Bytes func(path string) []byte
}

var fidelitySamples = []struct {
	name, encoding string
	data           []byte
	text           string
	bom            bool
	eol            contract.EOLStyle
}{
	{"empty", "utf-8", nil, "", false, contract.EOLLF},
	{"lf", "utf-8", []byte("一\n二\n"), "一\n二\n", false, contract.EOLLF},
	{"no final newline", "utf-8", []byte("一\n二"), "一\n二", false, contract.EOLLF},
	{"crlf", "utf-8", []byte("a\r\nb\r\n"), "a\r\nb\r\n", false, contract.EOLCRLF},
	{"mixed", "utf-8", []byte("a\nb\r\nc\n"), "a\nb\r\nc\n", false, contract.EOLMixed},
	{"bom", "utf-8", []byte("\xEF\xBB\xBF# 标题\n"), "# 标题\n", true, contract.EOLLF},
	{"utf-16le", "utf-16le", []byte("\xFF\xFEa\x00\n\x00"), "a\n", true, contract.EOLLF},
	{"utf-16be", "utf-16be", []byte("\xFE\xFF\x00a\x00\n"), "a\n", true, contract.EOLLF},
}

// TestFileStore checks docs/04 §4.5 for a store.
func TestFileStore(t *testing.T, newFixture func(t *testing.T) FileStoreFixture) {
	for _, s := range fidelitySamples {
		// Open, do not edit, save: the bytes stay the same (§4.5.2).
		t.Run("fidelity "+s.name, func(t *testing.T) {
			f := newFixture(t)
			path := f.Path("doc.md")
			f.Put(path, s.data)
			got, err := f.Store.Read(path, contract.ReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Text != s.text || got.Meta.BOM != s.bom || got.Meta.EOL != s.eol || got.Meta.Encoding != s.encoding {
				t.Fatalf("read %q, meta %+v", got.Text, got.Meta)
			}
			stamp, err := f.Store.Write(path, got.Text, got.Meta, &got.Stamp)
			if err != nil {
				t.Fatal(err)
			}
			if b := f.Bytes(path); !bytes.Equal(b, s.data) {
				t.Fatalf("wrote %q, want %q", b, s.data)
			}
			if now, err := f.Store.Stat(path); err != nil || !now.ModTime.Equal(stamp.ModTime) || now.Size != stamp.Size {
				t.Fatalf("Stat %+v, %v; Write returned %+v", now, err, stamp)
			}
		})
	}
	t.Run("missing file", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.Store.Read(f.Path("none.md"), contract.ReadOptions{})
		wantCode(t, err, contract.CodeNotFound)
		_, err = f.Store.Stat(f.Path("none.md"))
		wantCode(t, err, contract.CodeNotFound)
	})
	// A stale stamp is a conflict and writes nothing (§4.5.4).
	t.Run("conflict", func(t *testing.T) {
		f := newFixture(t)
		path := f.Path("doc.md")
		f.Put(path, []byte("old"))
		got, err := f.Store.Read(path, contract.ReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		f.Put(path, []byte("external"))
		_, err = f.Store.Write(path, "mine", got.Meta, &got.Stamp)
		wantCode(t, err, contract.CodeConflict)
		if b := f.Bytes(path); string(b) != "external" {
			t.Fatalf("the file became %q", b)
		}
		if _, err := f.Store.Write(path, "mine", got.Meta, nil); err != nil {
			t.Fatal(err)
		}
		if b := f.Bytes(path); string(b) != "mine" {
			t.Fatalf("the file became %q", b)
		}
	})
	t.Run("too large", func(t *testing.T) {
		f := newFixture(t)
		path := f.Path("doc.md")
		f.Put(path, []byte("12345"))
		_, err := f.Store.Read(path, contract.ReadOptions{MaxSize: 4})
		wantCode(t, err, contract.CodeTooLarge)
		if _, err := f.Store.Read(path, contract.ReadOptions{MaxSize: 5}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("binary", func(t *testing.T) {
		f := newFixture(t)
		path := f.Path("doc.bin")
		f.Put(path, []byte("a\x00b"))
		_, err := f.Store.Read(path, contract.ReadOptions{})
		wantCode(t, err, contract.CodeBinary)
		if _, err := f.Store.Read(path, contract.ReadOptions{AllowBinary: true}); err != nil {
			t.Fatal(err)
		}
	})
}

func wantCode(t *testing.T, err error, code contract.Code) {
	t.Helper()
	if _, ok := err.(*contract.Error); !ok {
		t.Fatalf("error %v is not a *contract.Error", err)
	}
	if got := contract.CodeOf(err); got != code {
		t.Fatalf("code %q, want %q: %v", got, code, err)
	}
}
