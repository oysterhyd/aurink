package shell

import (
	"path/filepath"
	"strings"

	"aurink/internal/contract"
	"aurink/internal/views/kit"
	"aurink/internal/views/motion"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// files is the sidebar's file tree over the workspace (FE-2.3).
type files struct {
	tree ui.OutlineState[string]
	// dirs records which listed paths are folders.
	dirs map[string]bool
	// kids caches each loaded folder's children until the workspace changes:
	// rebuilding 10,000 paths every frame took 28 ms.
	kids map[string][]string
	// renaming is the path whose name is being edited inline, "" for none.
	renaming, name, clash string
	off                   func()
	// Built counts the rows built in the last frame, for tests.
	Built int
	// lit is the folder lit under a drag in the last frame, for tests.
	lit string
}

func (s *Shell) watchWorkspace() {
	ws := s.o.Workspace
	if ws == nil {
		return
	}
	s.files.dirs = map[string]bool{}
	s.files.kids = map[string][]string{}
	s.files.off = ws.Subscribe(func(ev contract.WorkspaceEvent) {
		clear(s.files.kids)
		if ev.Kind == contract.WsRenamed {
			// The tabs of the file, or of those in the folder, follow it (D5, D6).
			s.o.Sessions.Moved(ev.OldPath, ev.Path)
			if s.files.renaming == ev.OldPath {
				s.files.renaming = ""
			}
		}
		s.invalidate()
	})
}

// OpenFolder makes path the workspace's root and shows the files panel.
func (s *Shell) OpenFolder(path string) {
	if s.o.Workspace == nil || path == "" {
		return
	}
	s.o.Workspace.SetRoot(path, func(err error) {
		if err != nil {
			s.fail(err)
			return
		}
		s.sidebar, s.panel = true, panelFiles
		if s.o.Recent != nil {
			s.o.Recent.Add(path, true)
		}
		s.invalidate()
		s.sessionChanged()
	})
}

// children lists a folder's entries for the outline, nil for a file. A folder
// not loaded yet has none until its WsListed event.
func (s *Shell) children(path string) []string {
	if !s.files.dirs[path] {
		return nil
	}
	if kids, ok := s.files.kids[path]; ok {
		return kids
	}
	entries, loaded := s.o.Workspace.List(path)
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		s.files.dirs[e.Path] = e.IsDir
		out = append(out, e.Path)
	}
	if loaded {
		s.files.kids[path] = out
	}
	return out
}

func (s *Shell) filesPanel(c *ui.Context, t *theme.Tokens) {
	ws := s.o.Workspace
	root := ""
	if ws != nil {
		root = ws.Root()
	}
	if root == "" {
		ui.Column(c).Padding(12, 8).Gap(10).Children(func() {
			ui.Text(c, locales.ShellNoFolder()).FontSize(theme.UIText).TextColor(t.Muted).Padding(0, 8)
			if ws != nil && kit.Button(c.Key("openFolder"), t, locales.ShellOpenFolder(), "", false).Clicked() {
				s.run(contract.CmdFileOpenFolder)
			}
		})
		s.openDocuments(c, t)
		return
	}
	s.files.dirs[root] = true
	s.files.lit = ""
	head := ui.Text(c, filepath.Base(root)).FontSize(theme.UIBadge).TextColor(t.Muted).Padding(8, 16, 4).SingleLine()
	s.dropOn(head, t, root)
	s.files.Built = 0
	// The tree marks the open file itself (in the sheet color, as the baseline),
	// not with the list's own choice in the system accent.
	tree := ui.Outline(c.Key("tree"), &s.files.tree, s.children(root), s.children, func(path string) {
		s.fileRow(c.Key(path), t, path)
	}).Grow(1).MinHeight(0).Padding(0, 8, 8)
	if n := s.files.tree.Open.Len(); n != s.sess.expanded {
		s.sess.expanded = n
		s.sessionChanged()
	}
	// Below the rows is the folder itself.
	if p, ok := ui.Drop[dragPath](tree); ok && s.canMove(string(p), root) {
		s.move(string(p), root)
	}
}

// dragPath is a file or folder dragged in the tree.
type dragPath string

// canMove reports whether path can go into dir: not into itself, its own
// folders, or where it already is.
func (s *Shell) canMove(path, dir string) bool {
	sep := string(filepath.Separator)
	return path != dir && filepath.Dir(path) != dir && !strings.HasPrefix(dir+sep, path+sep)
}

// dropOn makes e take a drag into dir, lit while one that can go there is over
// it. It reports whether it is lit.
func (s *Shell) dropOn(e ui.Element, t *theme.Tokens, dir string) bool {
	if p, ok := ui.Drop[dragPath](e); ok && s.canMove(string(p), dir) {
		s.move(string(p), dir)
	}
	p, ok := ui.DragOver[dragPath](e)
	if !ok || !s.canMove(string(p), dir) {
		return false
	}
	e.Background(t.AccentSoft).Border(1, t.AccentRing)
	s.files.lit = dir
	return true
}

// move moves a file or folder into dir; the notice undoes it by moving it back
// (D7). Open tabs follow on the WsRenamed events.
func (s *Shell) move(path, dir string) {
	from, name := filepath.Dir(path), filepath.Base(path)
	s.o.Workspace.Move([]string{path}, dir, func(err error) {
		if err != nil {
			s.fail(err)
			return
		}
		s.files.dirs[dir] = true
		s.files.tree.Open.Add(dir)
		moved := filepath.Join(dir, name)
		s.Notify(contract.Notice{Level: contract.NoticeInfo, Message: locales.ShellMoved(name, filepath.Base(dir)),
			Action: locales.ShellUndo(), Run: func() {
				s.o.Workspace.Move([]string{moved}, from, func(err error) {
					if err != nil {
						s.fail(err)
					}
				})
			}})
		s.invalidate()
	})
}

type filePill struct{}

func (s *Shell) fileRow(c *ui.Context, t *theme.Tokens, path string) {
	s.files.Built++
	dir := s.files.dirs[path]
	name := filepath.Base(path)
	d := s.docAt(path)
	row := ui.Row(c).Height(28).FillWidth().Padding(0, 8).Gap(8).AlignItems(ui.Center).Radius(theme.RadiusRow).Label(name)
	if s.files.renaming != path {
		row.Drag(dragPath(path))
	}
	lit := dir && s.dropOn(row, t, path)
	switch {
	case lit:
	case row.Dragging():
		row.Opacity(.5)
	case d != nil && d == s.active:
		// The pill below shows it.
	case row.Hovered():
		row.Background(t.Hover)
	}
	row.Children(func() {
		// One pill marks the open document, sliding from row to row (02 §4.4
		// rule 2).
		if d != nil && d == s.active && !lit {
			ui.Box(c.Key("pill")).Absolute().Fill().Radius(theme.RadiusRow).Background(t.SurfaceHover).
				PassThrough().Transition(s.matched(&motion.Smooth, filePill{}))
		}
		if s.files.renaming == path {
			s.renameField(c, t, path)
			return
		}
		ui.Text(c, name).FontSize(theme.UIText).SingleLine().Grow(1)
		if d != nil && d.s.State().Edited {
			ui.Box(c).Size(6, 6).Radius(3).Background(t.Accent)
		}
	})
	if row.DoubleClicked() {
		s.startRename(path)
	} else if row.Clicked() {
		if dir {
			if s.files.tree.Open.Has(path) {
				s.files.tree.Open.Remove(path)
			} else {
				s.files.tree.Open.Add(path)
			}
		} else {
			s.Open(path)
		}
	}
	row.PopoverMenu(func(m *ui.Menu) {
		parent := path
		if !dir {
			parent = filepath.Dir(path)
		}
		if m.Item(locales.ShellNewFile()).Chosen() {
			s.createIn(parent, false)
		}
		if m.Item(locales.ShellNewFolder()).Chosen() {
			s.createIn(parent, true)
		}
		m.Separator()
		if m.Item(locales.ShellRename()).Chosen() {
			s.startRename(path)
		}
		if m.Item(locales.ShellTrash()).Chosen() {
			s.trash(path)
		}
	})
}

func (s *Shell) renameField(c *ui.Context, t *theme.Tokens, path string) {
	ui.Column(c).Grow(1).Children(func() {
		in := ui.TextInputBase(c.Key("rename"), &s.files.name).FontSize(theme.UIText).Label(locales.ShellRename()).Focus()
		in.OnTextKey(func(k ui.TextKey) bool {
			switch {
			case k.Key == ui.KeyEnter && k.Mods == 0:
				s.commitRename(path)
			case k.Key == ui.KeyEscape:
				s.files.renaming = ""
				s.invalidate()
			default:
				return false
			}
			return true
		})
		if in.Changed() {
			s.files.clash = ""
		}
		if s.files.clash != "" {
			ui.Text(c, s.files.clash).FontSize(theme.UIBadge).TextColor(t.Danger)
		}
	})
}

// renameActive edits the active document's name in the file tree.
func (s *Shell) renameActive() {
	if s.active == nil || s.active.s.State().Path == "" {
		return
	}
	path := s.active.s.State().Path
	s.showPanel(panelFiles)
	for dir := filepath.Dir(path); s.o.Workspace != nil && strings.HasPrefix(dir, s.o.Workspace.Root()); dir = filepath.Dir(dir) {
		s.files.dirs[dir] = true
		s.files.tree.Open.Add(dir)
		if dir == filepath.Dir(dir) {
			break
		}
	}
	s.startRename(path)
}

// StartRename edits a file's name in its row.
func (s *Shell) startRename(path string) {
	s.files.renaming, s.files.name, s.files.clash = path, filepath.Base(path), ""
	s.invalidate()
}

func (s *Shell) commitRename(path string) {
	name := strings.TrimSpace(s.files.name)
	if name == "" || name == filepath.Base(path) {
		s.files.renaming = ""
		s.invalidate()
		return
	}
	s.o.Workspace.Rename(path, name, func(_ string, err error) {
		switch {
		case err == nil:
			s.files.renaming = ""
		case contract.CodeOf(err) == contract.CodeExists:
			s.files.clash = locales.ShellNameExists()
		case contract.CodeOf(err) == contract.CodeInvalid:
			s.files.clash = locales.ShellNameInvalid()
		default:
			s.files.renaming = ""
			s.fail(err)
		}
		s.invalidate()
	})
}

// createIn makes an untitled file or folder in dir and edits its name.
func (s *Shell) createIn(dir string, isDir bool) {
	base, ext := locales.ShellUntitled(), ".md"
	if isDir {
		ext = ""
	}
	taken := map[string]bool{}
	entries, _ := s.o.Workspace.List(dir)
	for _, e := range entries {
		taken[strings.ToLower(e.Name)] = true
	}
	name := base + ext
	for i := 1; taken[strings.ToLower(name)]; i++ {
		name = base + "-" + itoa(i) + ext
	}
	s.files.dirs[dir] = true
	s.files.tree.Open.Add(dir)
	s.o.Workspace.Create(dir, name, isDir, func(path string, err error) {
		if err != nil {
			s.fail(err)
			return
		}
		s.files.dirs[path] = isDir
		if !isDir {
			s.Open(path)
		}
		s.startRename(path)
	})
}

// trash moves a file or folder to the trash, asking first when an open tab in it
// has unsaved changes (D8).
func (s *Shell) trash(path string) {
	unsaved := false
	for _, d := range s.docs {
		p := d.s.State().Path
		if d.s.State().Edited && (p == path || strings.HasPrefix(p, path+string(filepath.Separator))) {
			unsaved = true
		}
	}
	do := func() {
		s.o.Workspace.Trash([]string{path}, func(err error) {
			if err != nil {
				s.fail(err)
				return
			}
			// The open tabs of what went learn it is gone (D8): they keep their text.
			s.o.Sessions.Recheck()
			s.Notify(contract.Notice{Level: contract.NoticeInfo, Message: locales.ShellTrashed(filepath.Base(path))})
		})
	}
	if !unsaved {
		do()
		return
	}
	s.o.Platform.ConfirmTrash(filepath.Base(path), func(ok bool) {
		if ok {
			do()
		}
	})
}

func (s *Shell) docAt(path string) *doc {
	for _, d := range s.docs {
		if d.s.State().Path == path {
			return d
		}
	}
	return nil
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}

// TreeRowsBuilt is how many file tree rows the last frame built, for tests.
func (s *Shell) TreeRowsBuilt() int { return s.files.Built }

// ExpandFolder shows a folder's files in the tree.
func (s *Shell) ExpandFolder(path string) {
	s.files.dirs[path] = true
	s.files.tree.Open.Add(path)
	s.invalidate()
}

// canRenameActive reports whether the active document is in the open folder,
// where the tree can edit its name.
func (s *Shell) canRenameActive() bool {
	if s.active == nil || s.o.Workspace == nil || s.o.Workspace.Root() == "" {
		return false
	}
	rel, err := filepath.Rel(s.o.Workspace.Root(), s.active.s.State().Path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

// FileDropLit is the folder the tree lit under a drag in the last frame, "" for none.
func (s *Shell) FileDropLit() string { return s.files.lit }

// docOf returns the tab of a session.
func (s *Shell) docOf(x contract.Session) *doc {
	for _, d := range s.docs {
		if d.s.ID() == x.ID() {
			return d
		}
	}
	return nil
}
