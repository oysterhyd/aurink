package contracttest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"aurink/internal/contract"
)

// WorkspaceFixture builds a workspace over real temporary directories.
type WorkspaceFixture struct {
	// New returns a workspace without a root that watches through Watcher and
	// trashes through Trash. maxEntries is its watch limit, 0 for the default.
	New func(maxEntries int) contract.Workspace
	// Settle runs background work and posted callbacks until nothing is left.
	Settle  func()
	Watcher *SnapshotWatcher
	Trash   *RecordingTrasher
	// Dir is an empty directory for the test's files.
	Dir string
}

// RecordingTrasher is a contract.Trasher that deletes and records what it got.
type RecordingTrasher struct{ Paths []string }

// Trash removes path.
func (r *RecordingTrasher) Trash(path string) error {
	if _, err := os.Lstat(path); err != nil {
		return contract.E(contract.CodeNotFound, "trash", err)
	}
	r.Paths = append(r.Paths, path)
	if err := os.RemoveAll(path); err != nil {
		return contract.E(contract.CodeIO, "trash", err)
	}
	return nil
}

type wsHarness struct {
	t      *testing.T
	f      WorkspaceFixture
	ws     contract.Workspace
	events []contract.WorkspaceEvent
}

func newWs(t *testing.T, newFixture func(t *testing.T) WorkspaceFixture, maxEntries int, files ...string) *wsHarness {
	t.Helper()
	f := newFixture(t)
	for _, name := range files {
		p := filepath.Join(f.Dir, filepath.FromSlash(name))
		if strings.HasSuffix(name, "/") {
			mkdir(t, p)
			continue
		}
		mkdir(t, filepath.Dir(p))
		if err := os.WriteFile(p, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := &wsHarness{t: t, f: f, ws: f.New(maxEntries)}
	h.ws.Subscribe(func(e contract.WorkspaceEvent) { h.events = append(h.events, e) })
	if err := h.call(func(done func(error)) { h.ws.SetRoot(f.Dir, done) }); err != nil {
		t.Fatal(err)
	}
	return h
}

func mkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

// call runs an operation and returns its error, checking done comes once and
// only through Settle.
func (h *wsHarness) call(run func(done func(error))) error {
	h.t.Helper()
	var got error
	called := 0
	run(func(err error) { got = err; called++ })
	if called != 0 {
		h.t.Fatal("done ran before the operation returned")
	}
	h.f.Settle()
	if called != 1 {
		h.t.Fatalf("done called %d times", called)
	}
	return got
}

func (h *wsHarness) path(rel string) string { return filepath.Join(h.f.Dir, filepath.FromSlash(rel)) }

// names lists dir, loading it first.
func (h *wsHarness) names(rel string) []string {
	h.t.Helper()
	dir := h.path(rel)
	if _, loaded := h.ws.List(dir); !loaded {
		h.f.Settle()
	}
	entries, loaded := h.ws.List(dir)
	if !loaded {
		h.t.Fatalf("%s is not loaded after Settle", rel)
	}
	var out []string
	for _, e := range entries {
		if e.Path != filepath.Join(dir, e.Name) {
			h.t.Fatalf("entry %q has path %q", e.Name, e.Path)
		}
		name := e.Name
		if e.IsDir {
			name += "/"
		}
		out = append(out, name)
	}
	return out
}

func (h *wsHarness) wantNames(rel string, want ...string) {
	h.t.Helper()
	if got := h.names(rel); !reflect.DeepEqual(got, want) {
		h.t.Fatalf("%s lists %q, want %q", rel, got, want)
	}
}

// external flushes the watcher and settles.
func (h *wsHarness) external() {
	h.f.Watcher.Flush()
	h.f.Settle()
}

func (h *wsHarness) take() []contract.WorkspaceEvent {
	e := h.events
	h.events = nil
	return e
}

// saw reports whether an event of kind for path came.
func saw(events []contract.WorkspaceEvent, kind contract.WorkspaceEventKind, path string) bool {
	for _, e := range events {
		if e.Kind == kind && e.Path == path {
			return true
		}
	}
	return false
}

// TestWorkspace checks BE-2.5 and docs/07 §5 (D1, D5–D7, D15) for workspaces.
func TestWorkspace(t *testing.T, newFixture func(t *testing.T) WorkspaceFixture) {
	t.Run("root", func(t *testing.T) {
		h := newWs(t, newFixture, 0)
		if h.ws.Root() != filepath.Clean(h.f.Dir) {
			t.Fatalf("Root %q, want %q", h.ws.Root(), h.f.Dir)
		}
		if !saw(h.take(), contract.WsRootChanged, filepath.Clean(h.f.Dir)) {
			t.Fatal("no WsRootChanged")
		}
		wantCode(t, h.call(func(done func(error)) { h.ws.SetRoot(h.path("none"), done) }), contract.CodeNotFound)
		if err := os.WriteFile(h.path("file.md"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		wantCode(t, h.call(func(done func(error)) { h.ws.SetRoot(h.path("file.md"), done) }), contract.CodeInvalid)
		if h.ws.Root() != filepath.Clean(h.f.Dir) {
			t.Fatalf("a failed SetRoot changed the root to %q", h.ws.Root())
		}
	})
	t.Run("list sorts folders first, naturally", func(t *testing.T) {
		h := newWs(t, newFixture, 0, "b/", "A/", "note10.md", "note2.md", "Note1.md", "pic.PNG", "z.markdown", "A/x.md")
		h.wantNames("", "A/", "b/", "Note1.md", "note2.md", "note10.md", "pic.PNG", "z.markdown")
		// Directories load lazily, and say so when they have.
		h.take()
		if _, loaded := h.ws.List(h.path("A")); loaded {
			t.Fatal("A was loaded before anyone listed it")
		}
		h.f.Settle()
		if !saw(h.take(), contract.WsListed, h.path("A")) {
			t.Fatal("no WsListed for A")
		}
		h.wantNames("A", "x.md")
	})
	t.Run("filter and ignore", func(t *testing.T) {
		h := newWs(t, newFixture, 0, "a.md", "b.txt", "c.svg", ".hidden.md", ".git/config", "node_modules/x.md", "sub/node_modules/y.md", "sub/d.go")
		h.wantNames("", "sub/", "a.md", "c.svg")
		h.wantNames("sub")
		h.take()
		h.ws.SetShowAll(true)
		h.f.Settle()
		if !saw(h.take(), contract.WsListed, h.path("")) {
			t.Fatal("SetShowAll sent no WsListed for a loaded directory")
		}
		h.wantNames("", "sub/", "a.md", "b.txt", "c.svg")
		h.wantNames("sub", "d.go")
		for _, tt := range []struct {
			rel          string
			isDir, wants bool
		}{{".git", true, true}, {"node_modules", true, true}, {"sub/node_modules", true, true},
			{".hidden.md", false, true}, {"a.md", false, false}, {"sub", true, false}, {"sub/d.go", false, false}} {
			if got := h.ws.Ignored(tt.rel, tt.isDir); got != tt.wants {
				t.Errorf("Ignored(%q) = %v, want %v", tt.rel, got, tt.wants)
			}
		}
	})
	t.Run("create", func(t *testing.T) {
		h := newWs(t, newFixture, 0, "sub/")
		h.wantNames("", "sub/")
		var got string
		err := h.call(func(done func(error)) {
			h.ws.Create(h.path("sub"), "新文档.md", false, func(p string, err error) { got = p; done(err) })
		})
		if err != nil || got != h.path("sub/新文档.md") {
			t.Fatalf("Create gave %q, %v", got, err)
		}
		if _, err := os.Stat(got); err != nil {
			t.Fatal(err)
		}
		if !saw(h.take(), contract.WsCreated, got) {
			t.Fatal("no WsCreated")
		}
		err = h.call(func(done func(error)) {
			h.ws.Create(h.path(""), "dir", true, func(p string, err error) { got = p; done(err) })
		})
		if info, statErr := os.Stat(h.path("dir")); err != nil || statErr != nil || !info.IsDir() {
			t.Fatalf("Create dir: %v, %v", err, statErr)
		}
		h.wantNames("", "dir/", "sub/")
		h.wantNames("sub", "新文档.md")
		wantCode(t, h.call(func(done func(error)) {
			h.ws.Create(h.path("sub"), "新文档.md", false, func(_ string, err error) { done(err) })
		}), contract.CodeExists)
		for _, bad := range []string{"", ".", "..", "a/b", `a\b`} {
			wantCode(t, h.call(func(done func(error)) {
				h.ws.Create(h.path(""), bad, false, func(_ string, err error) { done(err) })
			}), contract.CodeInvalid)
		}
	})
	t.Run("rename", func(t *testing.T) {
		h := newWs(t, newFixture, 0, "a.md", "b.md")
		h.wantNames("", "a.md", "b.md")
		h.take()
		var got string
		err := h.call(func(done func(error)) {
			h.ws.Rename(h.path("a.md"), "c.md", func(p string, err error) { got = p; done(err) })
		})
		if err != nil || got != h.path("c.md") {
			t.Fatalf("Rename gave %q, %v", got, err)
		}
		found := false
		for _, e := range h.take() {
			found = found || e.Kind == contract.WsRenamed && e.Path == got && e.OldPath == h.path("a.md")
		}
		if !found {
			t.Fatal("no WsRenamed with the old path")
		}
		h.wantNames("", "b.md", "c.md")
		wantCode(t, h.call(func(done func(error)) {
			h.ws.Rename(h.path("c.md"), "b.md", func(_ string, err error) { done(err) })
		}), contract.CodeExists)
		if b, _ := os.ReadFile(h.path("b.md")); string(b) != "b.md" {
			t.Fatalf("a refused rename changed b.md to %q", b)
		}
		wantCode(t, h.call(func(done func(error)) {
			h.ws.Rename(h.path("none.md"), "x.md", func(_ string, err error) { done(err) })
		}), contract.CodeNotFound)
	})
	t.Run("move", func(t *testing.T) {
		h := newWs(t, newFixture, 0, "a.md", "b.md", "dst/", "dst/b.md", "dir/c.md")
		h.wantNames("", "dir/", "dst/", "a.md", "b.md")
		h.take()
		if err := h.call(func(done func(error)) { h.ws.Move([]string{h.path("a.md"), h.path("dir")}, h.path("dst"), done) }); err != nil {
			t.Fatal(err)
		}
		events := h.take()
		for _, old := range []string{"a.md", "dir"} {
			found := false
			for _, e := range events {
				found = found || e.Kind == contract.WsRenamed && e.OldPath == h.path(old) && e.Path == h.path("dst/"+old)
			}
			if !found {
				t.Fatalf("no WsRenamed from %s", old)
			}
		}
		h.wantNames("", "dst/", "b.md")
		h.wantNames("dst", "dir/", "a.md", "b.md")
		h.wantNames("dst/dir", "c.md")
		// A clash refuses the whole move.
		wantCode(t, h.call(func(done func(error)) { h.ws.Move([]string{h.path("b.md")}, h.path("dst"), done) }), contract.CodeExists)
		h.wantNames("", "dst/", "b.md")
		// Into itself.
		wantCode(t, h.call(func(done func(error)) { h.ws.Move([]string{h.path("dst")}, h.path("dst/dir"), done) }), contract.CodeInvalid)
		h.wantNames("dst", "dir/", "a.md", "b.md")
	})
	t.Run("trash", func(t *testing.T) {
		h := newWs(t, newFixture, 0, "a.md", "dir/b.md")
		h.wantNames("", "dir/", "a.md")
		h.take()
		if err := h.call(func(done func(error)) { h.ws.Trash([]string{h.path("a.md"), h.path("dir")}, done) }); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(h.f.Trash.Paths, []string{h.path("a.md"), h.path("dir")}) {
			t.Fatalf("trashed %q", h.f.Trash.Paths)
		}
		events := h.take()
		if !saw(events, contract.WsRemoved, h.path("a.md")) || !saw(events, contract.WsRemoved, h.path("dir")) {
			t.Fatalf("events %+v", events)
		}
		h.wantNames("")
		wantCode(t, h.call(func(done func(error)) { h.ws.Trash([]string{h.path("a.md")}, done) }), contract.CodeNotFound)
	})
	t.Run("external changes", func(t *testing.T) {
		h := newWs(t, newFixture, 0, "a.md", "b.md", "sub/c.md")
		h.wantNames("", "sub/", "a.md", "b.md")
		h.wantNames("sub", "c.md")
		h.take()
		write(t, h.path("new.md"), "x")
		write(t, h.path("a.md"), "longer than before")
		if err := os.Remove(h.path("b.md")); err != nil {
			t.Fatal(err)
		}
		write(t, h.path("sub/d.md"), "x")
		write(t, h.path("node_modules/e.md"), "x")
		h.external()
		events := h.take()
		for _, want := range []struct {
			kind contract.WorkspaceEventKind
			rel  string
		}{{contract.WsCreated, "new.md"}, {contract.WsModified, "a.md"}, {contract.WsRemoved, "b.md"}, {contract.WsCreated, "sub/d.md"}} {
			if !saw(events, want.kind, h.path(want.rel)) {
				t.Errorf("no event %d for %s in %+v", want.kind, want.rel, events)
			}
		}
		for _, e := range events {
			if strings.Contains(e.Path, "node_modules") {
				t.Errorf("event for an ignored path: %+v", e)
			}
		}
		h.wantNames("", "sub/", "a.md", "new.md")
		h.wantNames("sub", "c.md", "d.md")
		// A rename may come as a removal and a creation; the list follows either way.
		if err := os.Rename(h.path("new.md"), h.path("renamed.md")); err != nil {
			t.Fatal(err)
		}
		h.external()
		h.wantNames("", "sub/", "a.md", "renamed.md")
	})
	t.Run("overflow refreshes loaded directories", func(t *testing.T) {
		h := newWs(t, newFixture, 0, "a.md")
		h.wantNames("", "a.md")
		write(t, h.path("lost.md"), "x")
		h.f.Watcher.Overflow()
		h.f.Settle()
		h.wantNames("", "a.md", "lost.md")
	})
	// maxEntries limits the recursive watch of the root; the root itself and
	// every directory here stay within it, so per-directory watches fit either way.
	t.Run("degrades past the watch limit", func(t *testing.T) {
		h := newWs(t, newFixture, 3, "a.md", "sub/d.md", "other/e.md", "other/f.md")
		h.wantNames("", "other/", "sub/", "a.md")
		if !saw(h.take(), contract.WsDegraded, filepath.Clean(h.f.Dir)) {
			t.Fatal("no WsDegraded for the root")
		}
		h.wantNames("sub", "d.md")
		// Loaded directories are still watched.
		write(t, h.path("sub/new.md"), "x")
		h.external()
		h.wantNames("sub", "d.md", "new.md")
		for _, p := range h.f.Watcher.Paths() {
			if p != h.path("") && p != h.path("sub") {
				t.Errorf("watching %s, which is not loaded", p)
			}
		}
	})
	t.Run("a new root closes the old watches", func(t *testing.T) {
		h := newWs(t, newFixture, 0, "one/a.md", "two/b.md")
		if err := h.call(func(done func(error)) { h.ws.SetRoot(h.path("two"), done) }); err != nil {
			t.Fatal(err)
		}
		for _, p := range h.f.Watcher.Paths() {
			if !strings.HasPrefix(p, h.path("two")) {
				t.Errorf("still watching %s", p)
			}
		}
		h.take()
		write(t, h.path("one/x.md"), "x")
		h.external()
		for _, e := range h.take() {
			t.Errorf("event from the old root: %+v", e)
		}
	})
}

func write(t *testing.T, path, text string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// AssetsFixture builds an importer.
type AssetsFixture struct {
	// New returns an importer with Settings.Files.ImageDir and CopyImages set.
	New func(imageDir string, copyImages bool) contract.Assets
	Dir string
}

// TestAssets checks docs/03 §5.3 for asset importers.
func TestAssets(t *testing.T, newFixture func(t *testing.T) AssetsFixture) {
	png := []byte("\x89PNG\r\n\x1a\nfake")
	t.Run("image dir templates", func(t *testing.T) {
		for _, tt := range []struct{ dir, want, file string }{
			{"./assets", "assets/shot.png", "assets/shot.png"},
			{"./${filename}.assets", "my%20doc.assets/shot.png", "my doc.assets/shot.png"},
		} {
			f := newFixture(t)
			doc := filepath.Join(f.Dir, "my doc.md")
			got, err := f.New(tt.dir, true).ImportImage(doc, png, "shot")
			if err != nil || got != tt.want {
				t.Fatalf("%s: got %q, %v; want %q", tt.dir, got, err, tt.want)
			}
			if b, err := os.ReadFile(filepath.Join(f.Dir, filepath.FromSlash(tt.file))); err != nil || string(b) != string(png) {
				t.Fatalf("%s: file %q, %v", tt.dir, b, err)
			}
		}
	})
	t.Run("absolute dir", func(t *testing.T) {
		f := newFixture(t)
		doc := filepath.Join(f.Dir, "notes", "a.md")
		got, err := f.New(filepath.Join(f.Dir, "images"), true).ImportImage(doc, png, "")
		if err != nil || got != "../images/image.png" {
			t.Fatalf("got %q, %v", got, err)
		}
	})
	t.Run("names never overwrite", func(t *testing.T) {
		f := newFixture(t)
		a := f.New("./assets", true)
		doc := filepath.Join(f.Dir, "a.md")
		var got []string
		for i := 0; i < 3; i++ {
			p, err := a.ImportImage(doc, png, "x")
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, p)
		}
		if want := []string{"assets/x.png", "assets/x-1.png", "assets/x-2.png"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
	t.Run("import files", func(t *testing.T) {
		f := newFixture(t)
		src := filepath.Join(f.Dir, "src")
		write(t, filepath.Join(src, "a b.jpg"), "jpg")
		write(t, filepath.Join(src, "c.gif"), "gif")
		write(t, filepath.Join(f.Dir, "assets", "c.gif"), "taken")
		doc := filepath.Join(f.Dir, "doc.md")
		got, err := f.New("./assets", true).ImportFiles(doc, []string{filepath.Join(src, "a b.jpg"), filepath.Join(src, "c.gif")})
		if want := []string{"assets/a%20b.jpg", "assets/c-1.gif"}; err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %q, %v; want %q", got, err, want)
		}
		if b, _ := os.ReadFile(filepath.Join(f.Dir, "assets", "c-1.gif")); string(b) != "gif" {
			t.Fatalf("copied %q", b)
		}
		got, err = f.New("./assets", false).ImportFiles(doc, []string{filepath.Join(src, "c.gif")})
		if err != nil || !reflect.DeepEqual(got, []string{"src/c.gif"}) {
			t.Fatalf("without copying: %q, %v", got, err)
		}
	})
	t.Run("untitled document", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.New("./assets", true).ImportImage("", png, "x")
		wantCode(t, err, contract.CodeInvalid)
	})
}

// RecentFixture builds recent lists over one store.
type RecentFixture struct {
	// New returns a list as a fresh start of the process would.
	New func() contract.Recent
	// Settle waits for background writes.
	Settle func()
	Dir    string
}

// TestRecent checks BE-2.5 for recent lists.
func TestRecent(t *testing.T, newFixture func(t *testing.T) RecentFixture) {
	f := newFixture(t)
	r := f.New()
	p := func(name string) string { return filepath.Join(f.Dir, name) }
	r.Add(p("a.md"), false)
	r.Add(p("b.md"), false)
	r.Add(p("dir"), true)
	r.Add(p("a.md"), false)
	if got, want := r.Files(), []string{p("a.md"), p("b.md")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Files %q, want %q", got, want)
	}
	if got, want := r.Folders(), []string{p("dir")}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Folders %q, want %q", got, want)
	}
	for i := 0; i < contract.RecentMax+5; i++ {
		r.Add(p(strings.Repeat("x", i+1)+".md"), false)
	}
	files := r.Files()
	if len(files) != contract.RecentMax || files[0] != p(strings.Repeat("x", contract.RecentMax+5)+".md") {
		t.Fatalf("%d files, first %q", len(files), files[0])
	}
	f.Settle()
	again := f.New()
	if !reflect.DeepEqual(again.Files(), files) || !reflect.DeepEqual(again.Folders(), []string{p("dir")}) {
		t.Fatalf("after a restart: %q, %q", again.Files(), again.Folders())
	}
	again.Clear()
	f.Settle()
	if third := f.New(); len(third.Files()) != 0 || len(third.Folders()) != 0 {
		t.Fatalf("Clear did not persist: %q, %q", third.Files(), third.Folders())
	}
}
