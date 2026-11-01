package corpus

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexMatchesFiles(t *testing.T) {
	entries, err := Index()
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	real := 0
	for _, e := range entries {
		listed[e.Path] = true
		data, err := Read(e)
		if err != nil {
			t.Error(err)
			continue
		}
		sum := sha256.Sum256(data)
		if len(data) != e.Size || !strings.HasPrefix(hex.EncodeToString(sum[:]), e.SHA256) {
			t.Errorf("%s changed: %d bytes, sha256 %x", e.Path, len(data), sum[:8])
		}
		if strings.HasPrefix(e.Path, "real/") {
			real++
		}
	}
	if real < 50 {
		t.Errorf("%d real documents, want at least 50", real)
	}
	root := Root()
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if strings.Contains(rel, "/") && !listed[rel] {
			t.Errorf("%s is not in INDEX.md", rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The edge samples hold the bytes they exist to test.
func TestEdgeSamples(t *testing.T) {
	entries, err := Index()
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, e := range entries {
		if files[e.Path], err = Read(e); err != nil {
			t.Fatal(err)
		}
	}
	checks := map[string]func([]byte) bool{
		"edge/crlf.md": func(b []byte) bool {
			return bytes.Count(b, []byte("\r\n")) == bytes.Count(b, []byte("\n")) && len(b) > 0
		},
		"edge/cr.md":               func(b []byte) bool { return bytes.Contains(b, []byte("\r")) && !bytes.Contains(b, []byte("\n")) },
		"edge/bom-utf8.md":         func(b []byte) bool { return bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) },
		"edge/mixed-eol.md":        func(b []byte) bool { return bytes.Contains(b, []byte("\r\n")) && bytes.Contains(b, []byte("a\n")) },
		"edge/no-final-newline.md": func(b []byte) bool { return len(b) > 0 && !bytes.HasSuffix(b, []byte("\n")) },
		"edge/empty.md":            func(b []byte) bool { return len(b) == 0 },
	}
	for path, ok := range checks {
		if b, found := files[path]; !found || !ok(b) {
			t.Errorf("%s does not hold what it tests: %q", path, b)
		}
	}
}

func TestGenerateIsDeterministic(t *testing.T) {
	for _, size := range []int{1 << 20, 5 << 20} {
		a, b := Generate(size, 7), Generate(size, 7)
		if len(a) < size || a != b {
			t.Errorf("size %d: got %d bytes, equal = %v", size, len(a), a == b)
		}
	}
	if Generate(1<<16, 1) == Generate(1<<16, 2) {
		t.Error("seeds give the same document")
	}
}
