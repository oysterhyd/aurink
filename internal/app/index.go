package app

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"aurink/internal/contract"
)

// maxIndexed bounds the files quick open lists from a folder.
const maxIndexed = 50000

// fileIndex lists the Markdown files under the workspace's root for quick
// open. It walks the folder off the main thread, by the tree's ignore rules
// (D15), and again when files come, go or move there.
type fileIndex struct {
	ws      contract.Workspace
	post    contract.Dispatcher
	files   []string
	walking bool
	again   bool
}

func newFileIndex(ws contract.Workspace, d contract.Dispatcher) *fileIndex {
	ix := &fileIndex{ws: ws, post: d}
	ws.Subscribe(func(ev contract.WorkspaceEvent) {
		switch ev.Kind {
		case contract.WsRootChanged, contract.WsCreated, contract.WsRemoved, contract.WsRenamed:
			ix.rescan()
		}
	})
	return ix
}

// Files returns the indexed files, sorted; call on the main thread.
func (ix *fileIndex) Files() []string { return ix.files }

func (ix *fileIndex) rescan() {
	if ix.walking {
		ix.again = true
		return
	}
	root := ix.ws.Root()
	if root == "" {
		ix.files = nil
		return
	}
	ix.walking = true
	go func() {
		files := markdownUnder(root, ix.ws.Ignored)
		ix.post.Post(func() {
			ix.walking = false
			if ix.ws.Root() == root {
				ix.files = files
			}
			if ix.again {
				ix.again = false
				ix.rescan()
			}
		})
	}()
}

// markdownUnder lists the .md and .markdown files under root that ignored
// does not exclude, up to maxIndexed.
func markdownUnder(root string, ignored func(rel string, isDir bool) bool) []string {
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if ignored(filepath.ToSlash(rel), d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := strings.ToLower(filepath.Ext(p)); !d.IsDir() && (ext == ".md" || ext == ".markdown") {
			out = append(out, p)
			if len(out) >= maxIndexed {
				return filepath.SkipAll
			}
		}
		return nil
	})
	slices.Sort(out)
	return out
}
