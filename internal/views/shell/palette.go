package shell

import (
	"path/filepath"
	"strings"

	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"aurink/internal/views/kit"
	"aurink/internal/views/palette"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// paletteSource lists what the palette offers, from the shell's state.
type paletteSource struct{ s *Shell }

// Files are the open tabs, the closed ones, the workspace's files and the
// recent files, each once.
func (p paletteSource) Files() []palette.Item {
	s := p.s
	seen := map[string]bool{}
	var out []palette.Item
	add := func(path string) {
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		out = append(out, palette.Item{Title: filepath.Base(path), Detail: filepath.Dir(path), Run: func() { s.Open(path) }})
	}
	for _, d := range s.docs {
		add(d.s.State().Path)
	}
	for i := len(s.closed) - 1; i >= 0; i-- {
		add(s.closed[i])
	}
	if s.o.Files != nil {
		for _, path := range s.o.Files() {
			add(path)
		}
	}
	if s.o.Recent != nil {
		for _, path := range s.o.Recent.Files() {
			add(path)
		}
	}
	return out
}

// paletteOwn are the commands that open the palette, which it does not list.
var paletteOwn = map[contract.CommandID]bool{
	contract.CmdNavPalette: true, contract.CmdNavQuickOpen: true, contract.CmdNavGoToHeading: true,
}

// Commands are the registered commands that can run now.
func (p paletteSource) Commands() []palette.Item {
	s := p.s
	var out []palette.Item
	for _, c := range s.o.Commands.All() {
		if paletteOwn[c.ID] || c.Run == nil || (c.Enabled != nil && !c.Enabled()) {
			continue
		}
		id := c.ID
		out = append(out, palette.Item{Title: Title(c.Title), Icon: commandIcon(id), Keys: s.keyLabel(id), Run: func() { s.run(id) }})
	}
	return out
}

func commandIcon(id contract.CommandID) *ui.SVG {
	s := string(id)
	switch id {
	case contract.CmdFileNew:
		return kit.IconPlus
	case contract.CmdFileOpenFolder, contract.CmdViewFiles:
		return kit.IconFolder
	case contract.CmdFileClose:
		return kit.IconClose
	case contract.CmdFileExportHTML, contract.CmdFileExportPDF, contract.CmdFilePrint:
		return kit.IconExport
	case contract.CmdViewToggleSidebar:
		return kit.IconSidebar
	case contract.CmdViewSource, contract.CmdFormatCode, contract.CmdBlockCode:
		return kit.IconCode
	case contract.CmdViewLive, contract.CmdViewRead, contract.CmdViewFocus, contract.CmdViewTypewriter:
		return kit.IconEye
	case contract.CmdViewThemeLight, contract.CmdViewThemeDark:
		return kit.IconContrast
	case contract.CmdAppSettings:
		return kit.IconSliders
	case contract.CmdAppAbout, contract.CmdAppCheckUpdates:
		return kit.IconInfo
	case contract.CmdViewOutline:
		return kit.IconHeading
	case contract.CmdFormatLink:
		return kit.IconLink
	case contract.CmdBlockTaskList, contract.CmdBlockToggleTask:
		return kit.IconCheck
	case contract.CmdBlockMoveUp, contract.CmdEditFindPrev:
		return kit.IconArrowUp
	case contract.CmdBlockMoveDown, contract.CmdEditFindNext:
		return kit.IconArrowDown
	}
	switch {
	case strings.HasPrefix(s, "file."):
		return kit.IconFile
	case strings.HasPrefix(s, "table."):
		return kit.IconGrid
	case strings.Contains(s, "search") || strings.Contains(s, "Search") || strings.HasPrefix(s, "edit.find") || id == contract.CmdEditReplace:
		return kit.IconSearch
	case strings.HasPrefix(s, "block.heading"):
		return kit.IconHeading
	case strings.HasPrefix(s, "format.") || strings.HasPrefix(s, "block.") || strings.HasPrefix(s, "edit."):
		return kit.IconEdit
	}
	return kit.IconCommand
}

// Headings are the active document's headings, in order.
func (p paletteSource) Headings() []palette.Item {
	s := p.s
	d := s.active
	if d == nil {
		return nil
	}
	m := d.s.Model()
	text, ast := m.Text(), m.AST()
	var out []palette.Item
	for i := range ast.Len() {
		b := ast.Block(i)
		if b.Kind != contract.BlockHeading {
			continue
		}
		title := strings.TrimSpace(text.Slice(b.Content))
		if title == "" {
			continue
		}
		off := b.Range.Start
		out = append(out, palette.Item{Title: title, Detail: "H" + string(rune('0'+b.Level)), Indent: b.Level - 1,
			Run: func() { s.goToOffset(d, off) }})
	}
	return out
}

// goToOffset puts the caret at a source offset and scrolls it to the top.
func (s *Shell) goToOffset(d *doc, off int) {
	if d != s.active {
		return
	}
	if d.mode == editorapi.ModeSource {
		d.s.Model().SetSelection(contract.Caret(off))
		d.source.Enter(off)
	} else {
		d.live.RevealOffset(off, editorapi.AlignTop)
	}
	s.focusNext = true
	s.invalidate()
}

func (s *Shell) openPalette(m palette.Mode) {
	s.pal.Open(m)
	s.invalidate()
}

// Palette is the command palette, for tests.
func (s *Shell) Palette() *palette.Palette { return s.pal }

func (s *Shell) paletteView(c *ui.Context, t *theme.Tokens) {
	if !s.live {
		return
	}
	was := s.pal.IsOpen()
	s.pal.View(c, t, palette.Labels{
		Files: locales.ShellPaletteFiles(), Commands: locales.ShellPaletteCommands(), Headings: locales.ShellPaletteHeadings(),
		Placeholder: locales.ShellPalettePlaceholder(), Footer: locales.ShellPaletteFooter(),
		Count: locales.ShellPaletteCount, Empty: locales.ShellPaletteEmpty(),
	}, s.live && !s.reduceMotion(c))
	// Escape and a click outside leave the focus nowhere: give it to the document.
	if was && !s.pal.IsOpen() {
		s.focusNext = true
	}
}

// GoTo puts the caret at a source offset of the active document and scrolls it
// to the top, as a heading picked in the palette does.
func (s *Shell) GoTo(off int) {
	if s.active != nil {
		s.goToOffset(s.active, off)
	}
}
