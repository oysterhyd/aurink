package shell

import (
	"path/filepath"
	"strconv"
	"strings"

	"aurink/internal/contract"
	"aurink/internal/views/editorapi"
	"aurink/internal/views/palette"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// Commands returns the commands M1 implements (06 INT-1.1): menus and shortcuts come
// from these alone. Title holds a key of Title.
func (s *Shell) Commands() []contract.Command {
	hasDoc := func() bool { return s.active != nil }
	cmds := []contract.Command{
		{ID: contract.CmdFileNew, Title: "new", Run: func() error { s.o.Sessions.New(); return nil }},
		{ID: contract.CmdFileOpen, Title: "open", Run: func() error {
			s.o.Platform.OpenFile(func(path string) {
				if path != "" {
					s.Open(path)
				}
			})
			return nil
		}},
		{ID: contract.CmdFileSave, Title: "save", Enabled: hasDoc, Run: func() error { s.save(s.active, nil); return nil }},
		{ID: contract.CmdFileSaveAs, Title: "saveAs", Enabled: hasDoc, Run: func() error {
			s.saveAs(s.active, func(err error) {
				if err != nil {
					s.fail(err)
				}
			})
			return nil
		}},
		{ID: contract.CmdFileClose, Title: "close", Enabled: hasDoc, Run: func() error { s.closeDoc(s.active); return nil }},
		{ID: contract.CmdViewToggleSidebar, Title: "sidebar", Checked: func() bool { return s.sidebar }, Run: func() error {
			s.sidebar = !s.sidebar
			s.invalidate()
			return nil
		}},
		{ID: contract.CmdViewLive, Title: "live", Enabled: hasDoc, Checked: func() bool { return s.active != nil && s.active.mode == editorapi.ModeLive },
			Run: func() error { s.setMode(s.active, editorapi.ModeLive); return nil }},
		{ID: contract.CmdViewSource, Title: "source", Enabled: hasDoc, Checked: func() bool { return s.active != nil && s.active.mode == editorapi.ModeSource },
			Run: func() error {
				// The shortcut toggles, as there is no key for going back otherwise.
				if s.active.mode == editorapi.ModeSource {
					s.setMode(s.active, editorapi.ModeLive)
				} else {
					s.setMode(s.active, editorapi.ModeSource)
				}
				return nil
			}},
		{ID: contract.CmdViewThemeLight, Title: "light", Checked: func() bool { return !s.dark }, Run: func() error {
			if s.dark {
				s.ToggleTheme(ui.Point{})
			}
			return nil
		}},
		{ID: contract.CmdViewThemeDark, Title: "dark", Checked: func() bool { return s.dark }, Run: func() error {
			if !s.dark {
				s.ToggleTheme(ui.Point{})
			}
			return nil
		}},
		{ID: contract.CmdFileOpenFolder, Title: "openFolder", Enabled: func() bool { return s.o.Workspace != nil }, Run: func() error {
			s.o.Platform.OpenFolder(s.OpenFolder)
			return nil
		}},
		{ID: contract.CmdFileRename, Title: "rename", Enabled: s.canRenameActive, Run: func() error { s.renameActive(); return nil }},
		{ID: contract.CmdViewFiles, Title: "files", Run: func() error { s.showPanel(panelFiles); return nil }},
		{ID: contract.CmdViewOutline, Title: "outline", Run: func() error { s.showPanel(panelOutline); return nil }},
		{ID: contract.CmdViewSearch, Title: "search", Run: func() error { s.showPanel(panelSearch); return nil }},
		{ID: contract.CmdNavSearchWorkspace, Title: "searchWorkspace", Run: func() error { s.searchWorkspace(); return nil }},
		{ID: contract.CmdNavQuickOpen, Title: "quickOpen", Run: func() error { s.openPalette(palette.ModeFiles); return nil }},
		{ID: contract.CmdNavPalette, Title: "palette", Run: func() error { s.openPalette(palette.ModeCommands); return nil }},
		{ID: contract.CmdNavGoToHeading, Title: "goToHeading", Enabled: hasDoc, Run: func() error { s.openPalette(palette.ModeHeadings); return nil }},
		{ID: contract.CmdFileReopenClosed, Title: "reopenClosed", Enabled: func() bool { return len(s.closed) > 0 },
			Run: func() error { s.reopenClosed(); return nil }},
		{ID: contract.CmdNavNextTab, Title: "nextTab", Enabled: func() bool { return len(s.docs) > 1 },
			Run: func() error { s.step(1); return nil }},
		{ID: contract.CmdNavPrevTab, Title: "prevTab", Enabled: func() bool { return len(s.docs) > 1 },
			Run: func() error { s.step(-1); return nil }},
	}
	for i, id := range []contract.CommandID{contract.CmdNavGoToTab1, contract.CmdNavGoToTab2, contract.CmdNavGoToTab3,
		contract.CmdNavGoToTab4, contract.CmdNavGoToTab5, contract.CmdNavGoToTab6, contract.CmdNavGoToTab7,
		contract.CmdNavGoToTab8, contract.CmdNavGoToTab9} {
		cmds = append(cmds, contract.Command{ID: id, Title: "goToTab" + strconv.Itoa(i+1),
			Enabled: func() bool { return i < len(s.docs) }, Run: func() error { s.activateAt(i); return nil }})
	}
	if s.o.ExportHTML != nil {
		cmds = append(cmds, contract.Command{ID: contract.CmdFileExportHTML, Title: "exportHTML", Enabled: hasDoc,
			Run: func() error { s.exportHTML(s.active); return nil }})
	}
	return append(cmds, s.editCommands()...)
}

// exportHTML asks where and writes the document there as an HTML page. Its
// images are embedded when the page goes to another folder, where their
// relative paths would not reach them.
func (s *Shell) exportHTML(d *doc) {
	if d.source != nil {
		d.source.Sync()
	}
	st := d.s.State()
	name := strings.TrimSuffix(st.Name, filepath.Ext(st.Name))
	md := d.s.Model().Text().String()
	s.o.Platform.ExportPath(name, func(path string) {
		if path == "" {
			return
		}
		if filepath.Ext(path) == "" {
			path += ".html"
		}
		opts := contract.ExportOptions{Format: contract.ExportHTML, Title: name}
		if st.Path != "" {
			opts.BaseDir = filepath.Dir(st.Path)
			opts.EmbedImages = filepath.Dir(path) != opts.BaseDir
		}
		s.o.ExportHTML(md, path, opts, func(err error) {
			if err != nil {
				s.fail(err)
				return
			}
			s.Notify(contract.Notice{Level: contract.NoticeInfo, Message: locales.ShellExported(), Detail: filepath.Base(path)})
		})
	})
}

// editCommands run in the live editor of the active tab; source mode has its own
// keys, so they are off there.
func (s *Shell) editCommands() []contract.Command {
	live := func() bool { return s.active != nil && s.active.mode == editorapi.ModeLive }
	edits := []struct {
		title string
		cmd   contract.EditCommand
	}{
		{"strong", contract.EditCommand{ID: contract.CmdFormatStrong}},
		{"emphasis", contract.EditCommand{ID: contract.CmdFormatEmphasis}},
		{"underline", contract.EditCommand{ID: contract.CmdFormatUnderline}},
		{"strike", contract.EditCommand{ID: contract.CmdFormatStrike}},
		{"inlineCode", contract.EditCommand{ID: contract.CmdFormatCode}},
		{"link", contract.EditCommand{ID: contract.CmdFormatLink}},
		{"paragraph", contract.EditCommand{ID: contract.CmdBlockParagraph}},
		{"heading1", contract.EditCommand{ID: contract.CmdBlockHeading1}},
		{"heading2", contract.EditCommand{ID: contract.CmdBlockHeading2}},
		{"heading3", contract.EditCommand{ID: contract.CmdBlockHeading3}},
		{"heading4", contract.EditCommand{ID: contract.CmdBlockHeading4}},
		{"heading5", contract.EditCommand{ID: contract.CmdBlockHeading5}},
		{"heading6", contract.EditCommand{ID: contract.CmdBlockHeading6}},
		{"quote", contract.EditCommand{ID: contract.CmdBlockQuote}},
		{"bulletList", contract.EditCommand{ID: contract.CmdBlockBulletList}},
		{"orderedList", contract.EditCommand{ID: contract.CmdBlockOrderedList}},
		{"taskList", contract.EditCommand{ID: contract.CmdBlockTaskList}},
		{"toggleTask", contract.EditCommand{ID: contract.CmdBlockToggleTask}},
		{"codeBlock", contract.EditCommand{ID: contract.CmdBlockCode}},
		{"hr", contract.EditCommand{ID: contract.CmdBlockHR}},
		{"indent", contract.EditCommand{ID: contract.CmdBlockIndent}},
		{"outdent", contract.EditCommand{ID: contract.CmdBlockOutdent}},
		{"moveUp", contract.EditCommand{ID: contract.CmdBlockMoveUp}},
		{"moveDown", contract.EditCommand{ID: contract.CmdBlockMoveDown}},
		// Rows counts body rows: with the header the table is 3 by 3.
		{"tableInsert", contract.EditCommand{ID: contract.CmdTableInsert, Rows: 2, Cols: 3}},
		{"rowAbove", contract.EditCommand{ID: contract.CmdTableRowAbove}},
		{"rowBelow", contract.EditCommand{ID: contract.CmdTableRowBelow}},
		{"colLeft", contract.EditCommand{ID: contract.CmdTableColLeft}},
		{"colRight", contract.EditCommand{ID: contract.CmdTableColRight}},
		{"deleteRow", contract.EditCommand{ID: contract.CmdTableDeleteRow}},
		{"deleteCol", contract.EditCommand{ID: contract.CmdTableDeleteCol}},
		{"tableFormat", contract.EditCommand{ID: contract.CmdTableFormat}},
	}
	hasDoc := func() bool { return s.active != nil }
	var out []contract.Command
	for _, e := range edits {
		out = append(out, contract.Command{ID: e.cmd.ID, Title: e.title, Enabled: live, Run: func() error {
			err := s.active.live.RunEdit(e.cmd)
			s.focusNext = true
			s.invalidate()
			return err
		}})
	}
	return append(out,
		contract.Command{ID: contract.CmdViewFocus, Title: "focus", Enabled: live,
			Checked: func() bool { return live() && s.active.live.FocusMode() },
			Run:     func() error { s.active.live.SetFocusMode(!s.active.live.FocusMode()); s.invalidate(); return nil }},
		contract.Command{ID: contract.CmdViewTypewriter, Title: "typewriter", Enabled: live,
			Checked: func() bool { return live() && s.active.live.Typewriter() },
			Run:     func() error { s.active.live.SetTypewriter(!s.active.live.Typewriter()); s.invalidate(); return nil }},
		contract.Command{ID: contract.CmdEditFind, Title: "find", Enabled: hasDoc, Run: func() error { s.find(false, 0); return nil }},
		contract.Command{ID: contract.CmdEditReplace, Title: "replace", Enabled: hasDoc, Run: func() error { s.find(true, 0); return nil }},
		contract.Command{ID: contract.CmdEditFindNext, Title: "findNext", Enabled: hasDoc, Run: func() error { s.find(false, 1); return nil }},
		contract.Command{ID: contract.CmdEditFindPrev, Title: "findPrev", Enabled: hasDoc, Run: func() error { s.find(false, -1); return nil }},
		contract.Command{ID: contract.CmdEditCopyMarkdown, Title: "copyMarkdown", Enabled: live,
			Run: func() error { s.active.live.CopyMarkdown(); return nil }},
		contract.Command{ID: contract.CmdEditCopyHTML, Title: "copyHTML", Enabled: live,
			Run: func() error { s.active.live.CopyHTML(); return nil }},
		contract.Command{ID: contract.CmdEditPastePlain, Title: "pastePlain", Enabled: live,
			Run: func() error { s.active.live.PastePlain(); return nil }},
	)
}

var editTitles = map[string]func() string{
	"strong": locales.ShellStrong, "emphasis": locales.ShellEmphasis, "underline": locales.ShellUnderline,
	"strike": locales.ShellStrike, "inlineCode": locales.ShellInlineCode, "link": locales.ShellLink,
	"paragraph": locales.ShellParagraph, "quote": locales.ShellQuote, "bulletList": locales.ShellBulletList,
	"orderedList": locales.ShellOrderedList, "taskList": locales.ShellTaskList, "toggleTask": locales.ShellToggleTask,
	"codeBlock": locales.ShellCodeBlock, "hr": locales.ShellHr, "indent": locales.ShellIndent,
	"outdent": locales.ShellOutdent, "moveUp": locales.ShellMoveUp, "moveDown": locales.ShellMoveDown,
	"tableInsert": locales.ShellTableInsert, "rowAbove": locales.ShellRowAbove, "rowBelow": locales.ShellRowBelow,
	"colLeft": locales.ShellColLeft, "colRight": locales.ShellColRight, "deleteRow": locales.ShellDeleteRow,
	"deleteCol": locales.ShellDeleteCol, "tableFormat": locales.ShellTableFormat, "focus": locales.ShellFocus,
	"typewriter": locales.ShellTypewriter, "quickOpen": locales.ShellQuickOpen, "palette": locales.ShellCommandPalette,
	"goToHeading": locales.ShellGoToHeading, "openFolder": locales.ShellOpenFolder, "files": locales.ShellPanelFiles,
	"outline": locales.ShellPanelOutline, "search": locales.ShellPanelSearch,
	"searchWorkspace": locales.ShellSearchInFolder, "rename": locales.ShellRename,
	"copyMarkdown": locales.ShellCopyMarkdown, "copyHTML": locales.ShellCopyHtml, "pastePlain": locales.ShellPastePlain,
	"find": locales.ShellFind, "replace": locales.ShellReplace, "findNext": locales.ShellFindNext,
	"findPrev": locales.ShellFindPrev,
}

// showPanel opens the sidebar on a panel.
func (s *Shell) showPanel(p int) {
	s.sidebar, s.panel = true, p
	s.invalidate()
}

// Title says a command's title in the app's language.
func Title(key string) string {
	switch key {
	case "new":
		return locales.ShellNew()
	case "open":
		return locales.ShellOpen()
	case "save":
		return locales.ShellSave()
	case "saveAs":
		return locales.ShellSaveAs()
	case "close":
		return locales.ShellClose()
	case "sidebar":
		return locales.ShellToggleSidebar()
	case "live":
		return locales.ShellViewLive()
	case "source":
		return locales.ShellViewSource()
	case "light":
		return locales.ShellThemeLight()
	case "dark":
		return locales.ShellThemeDark()
	case "reopenClosed":
		return locales.ShellReopenClosed()
	case "nextTab":
		return locales.ShellNextTab()
	case "prevTab":
		return locales.ShellPrevTab()
	case "exportHTML":
		return locales.ShellExportHtml()
	}
	if f, ok := editTitles[key]; ok {
		return f()
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(key, "heading")); err == nil && strings.HasPrefix(key, "heading") {
		return locales.ShellHeading(n)
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(key, "goToTab")); err == nil && strings.HasPrefix(key, "goToTab") {
		return locales.ShellGoToTab(n)
	}
	return key
}

// bindCommands gives every registered command its shortcuts.
func (s *Shell) bindCommands(c *ui.Context) {
	for _, cmd := range s.o.Commands.All() {
		for _, ch := range s.o.Commands.KeysFor(cmd.ID) {
			key, ok := keyOf(ch.Key)
			if !ok {
				continue
			}
			id := cmd.ID
			c.OnShortcut(modsOf(ch.Mods), key, func() { s.run(id) })
		}
	}
}

func modsOf(m contract.Mods) ui.Modifiers {
	var out ui.Modifiers
	if m&contract.ModPrimary != 0 {
		out |= ui.Cmd
	}
	if m&contract.ModShift != 0 {
		out |= ui.Shift
	}
	if m&contract.ModAlt != 0 {
		out |= ui.Alt
	}
	return out
}

var namedKeys = map[string]ui.Key{
	"A": ui.KeyA, "B": ui.KeyB, "C": ui.KeyC, "D": ui.KeyD, "E": ui.KeyE, "F": ui.KeyF, "G": ui.KeyG,
	"H": ui.KeyH, "I": ui.KeyI, "J": ui.KeyJ, "K": ui.KeyK, "L": ui.KeyL, "M": ui.KeyM, "N": ui.KeyN,
	"O": ui.KeyO, "P": ui.KeyP, "Q": ui.KeyQ, "R": ui.KeyR, "S": ui.KeyS, "T": ui.KeyT, "U": ui.KeyU,
	"V": ui.KeyV, "W": ui.KeyW, "X": ui.KeyX, "Y": ui.KeyY, "Z": ui.KeyZ, "/": ui.KeySlash,
	"Tab": ui.KeyTab, "`": ui.KeyBackquote, "[": ui.KeyBracketLeft, "]": ui.KeyBracketRight,
}

func init() {
	for i := range 10 {
		namedKeys[strconv.Itoa(i)] = ui.Key0 + ui.Key(i)
	}
	for i, k := range []ui.Key{ui.KeyF1, ui.KeyF2, ui.KeyF3, ui.KeyF4, ui.KeyF5, ui.KeyF6,
		ui.KeyF7, ui.KeyF8, ui.KeyF9, ui.KeyF10, ui.KeyF11, ui.KeyF12} {
		namedKeys["F"+strconv.Itoa(i+1)] = k
	}
}

func keyOf(name string) (ui.Key, bool) {
	k, ok := namedKeys[name]
	return k, ok
}

// find opens the find bar of the active tab, or steps through its matches
// (dir ±1); the source view's is MyGo's own.
func (s *Shell) find(replace bool, dir int) {
	d := s.active
	switch {
	case d.mode == editorapi.ModeSource && dir != 0:
		d.source.FindStep(dir)
	case d.mode == editorapi.ModeSource:
		d.source.Find(replace)
	case dir != 0:
		s.findStep(d, dir)
	default:
		s.openFind(d, replace)
	}
	s.invalidate()
}
