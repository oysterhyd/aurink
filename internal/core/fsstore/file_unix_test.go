//go:build !windows

package fsstore

import (
	"os"
	"path/filepath"
	"testing"

	"aurink/internal/contract"
)

func TestPermissionsPreserved(t *testing.T) {
	for _, mode := range []os.FileMode{0o600, 0o640, 0o751} {
		t.Run(mode.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "doc.md")
			put(t, path, []byte("old"))
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := New().Write(path, "new", contract.DocMeta{}, nil); err != nil {
				t.Fatal(err)
			}
			after, err := os.Stat(path)
			if err != nil || after.Mode().Perm() != mode {
				t.Fatalf("mode = %v, want %v; %v", after, mode, err)
			}
			if os.SameFile(before, after) {
				t.Fatal("ordinary file was not replaced")
			}
			wantBytes(t, path, []byte("new"))
		})
	}
}

func TestNewFilePermissions(t *testing.T) {
	dir := t.TempDir()
	reference, path := filepath.Join(dir, "reference.md"), filepath.Join(dir, "new.md")
	if err := os.WriteFile(reference, []byte("new"), 0o666); err != nil {
		t.Fatal(err)
	}
	if _, err := New().Write(path, "new", contract.DocMeta{}, nil); err != nil {
		t.Fatal(err)
	}
	want, err := os.Stat(reference)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.Stat(path)
	if err != nil || got.Mode().Perm() != want.Mode().Perm() {
		t.Fatalf("new file permissions = %v, want %v; %v", got, want, err)
	}
}

func TestDirectoryWithoutTempPermission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	put(t, path, []byte("original longer content"))
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Error(err)
		}
	})
	probe, err := os.CreateTemp(dir, "probe-*")
	if err == nil {
		name := probe.Name()
		if err := probe.Close(); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(name); err != nil {
			t.Fatal(err)
		}
		t.Skip("current user can create files despite directory permissions")
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := New().Write(path, "new", contract.DocMeta{}, nil); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("fallback did not preserve file identity: %v", err)
	}
	wantBytes(t, path, []byte("new"))
	wantOnlyFile(t, path)
}

func TestReadPermission(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	put(t, path, []byte("text"))
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0o644); err != nil {
			t.Error(err)
		}
	})
	if f, err := os.Open(path); err == nil {
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		t.Skip("current user can read files despite file permissions")
	}
	_, err := New().Read(path, contract.ReadOptions{})
	wantError(t, err, contract.CodePermission, "Read", path)
}

func TestSymlink(t *testing.T) {
	dir := t.TempDir()
	target, link := filepath.Join(dir, "target.md"), filepath.Join(dir, "link.md")
	put(t, target, []byte("original"))
	if err := os.Symlink("target.md", link); err != nil {
		t.Fatal(err)
	}
	s := New()
	got, err := s.Read(link, contract.ReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write(link, "replacement", got.Meta, &got.Stamp); err != nil {
		t.Fatal(err)
	}
	if dest, err := os.Readlink(link); err != nil || dest != "target.md" {
		t.Fatalf("symlink changed: %q, %v", dest, err)
	}
	wantBytes(t, target, []byte("replacement"))
	wantBytes(t, link, []byte("replacement"))
}
