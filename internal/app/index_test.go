package app

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"aurink/internal/contracttest"
	"aurink/internal/core/workspace"
)

type chanDispatcher chan func()

func (d chanDispatcher) Post(fn func()) { d <- fn }

// TestFileIndexListsMarkdownByTheTreesRules: quick open gets the folder's
// Markdown files, without what the tree ignores, and follows a new file.
func TestFileIndexListsMarkdownByTheTreesRules(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"a.md", "sub/b.markdown", "sub/c.MD", "pic.png", "node_modules/x.md", ".git/y.md", ".hidden.md"} {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d := make(chanDispatcher, 64)
	ws := workspace.New(&contracttest.SnapshotWatcher{}, &contracttest.RecordingTrasher{}, d, workspace.Options{})
	ix := newFileIndex(ws, d)
	var pump func(cond func() bool)
	pump = func(cond func() bool) {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for !cond() {
			select {
			case f := <-d:
				f()
			case <-deadline:
				t.Fatalf("index: %v", ix.Files())
			}
		}
	}
	ws.SetRoot(root, func(err error) {
		if err != nil {
			t.Error(err)
		}
	})
	want := []string{filepath.Join(root, "a.md"), filepath.Join(root, "sub", "b.markdown"), filepath.Join(root, "sub", "c.MD")}
	pump(func() bool { return slices.Equal(ix.Files(), want) })

	created := false
	ws.Create(root, "new.md", false, func(string, error) { created = true })
	pump(func() bool { return created && slices.Contains(ix.Files(), filepath.Join(root, "new.md")) })
}
