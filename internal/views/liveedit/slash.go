package liveedit

import (
	"strconv"
	"strings"

	"aurink/internal/contract"
	"aurink/internal/views/effects"
	"aurink/internal/views/motion"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// The slash menu (02 §5.3, 08 §batch 7): "/" typed at the start of an empty
// top-level paragraph opens a two-column panel of the blocks to insert. What
// is typed after the "/" filters it; ↑↓ choose, Enter inserts, Esc closes and
// keeps the text. Code blocks go on to a language, tables to a size.

const (
	slashW     = 500
	slashH     = 316
	slashListW = 214
	slashRowH  = 32
	// slashQueryMax closes the menu once the filter is longer than any label.
	slashQueryMax = 24
	// tableMax bounds the size grid, header row included.
	tableMaxRows, tableMaxCols = 8, 8
)

type slashStep uint8

const (
	slashItems slashStep = iota
	slashLanguage
	slashSize
)

type slashMenu struct {
	open bool
	at   int // the "/"
	step slashStep
	sel  int
	// scroll asks the list to bring the choice into view.
	scroll     bool
	rows, cols int // the size step's table, header row included
	list       ui.ListState
	// The pointer chooses only when it moves: resting on a row, it leaves the
	// keys' choice alone.
	px, py float32
	moved  bool
}

type slashItem struct {
	group  string
	glyph  string
	label  string
	desc   string
	syntax string
	keys   string // English words it is found by, whatever the language
	run    func(e *Editor)
	next   slashStep
}

func (e *Editor) slashItems() []slashItem {
	cmd := func(id contract.CommandID) func(*Editor) {
		return func(e *Editor) { e.slashCommand(contract.EditCommand{ID: id}) }
	}
	basic, media, adv := locales.LiveeditSlashBasic(), locales.LiveeditSlashMedia(), locales.LiveeditSlashAdvanced()
	render := locales.LiveeditSlashRenderDesc()
	title, item := locales.LiveeditSlashSampleTitle(), locales.LiveeditSlashSampleItem()
	out := []slashItem{
		{basic, "H1", locales.LiveeditHeadingN("1"), locales.LiveeditSlashH1Desc(), "# " + title, "h1 heading title", cmd(contract.CmdBlockHeading1), 0},
		{basic, "H2", locales.LiveeditHeadingN("2"), locales.LiveeditSlashH2Desc(), "## " + title, "h2 heading", cmd(contract.CmdBlockHeading2), 0},
		{basic, "H3", locales.LiveeditHeadingN("3"), locales.LiveeditSlashH3Desc(), "### " + title, "h3 heading", cmd(contract.CmdBlockHeading3), 0},
		{basic, "•", locales.LiveeditSlashBullets(), locales.LiveeditSlashBulletsDesc(), "- " + item, "bulleted list ul unordered", cmd(contract.CmdBlockBulletList), 0},
		{basic, "1.", locales.LiveeditSlashNumbers(), locales.LiveeditSlashNumbersDesc(), "1. " + item, "numbered list ol ordered", cmd(contract.CmdBlockOrderedList), 0},
		{basic, "[ ]", locales.LiveeditSlashTasks(), locales.LiveeditSlashTasksDesc(), "- [ ] " + locales.LiveeditSlashSampleTodo(), "task list todo checkbox", cmd(contract.CmdBlockTaskList), 0},
		{basic, ">", locales.LiveeditQuote(), locales.LiveeditSlashQuoteDesc(), "> " + locales.LiveeditSlashSampleQuote(), "quote blockquote", cmd(contract.CmdBlockQuote), 0},
		{media, "{}", locales.LiveeditSlashCode(), locales.LiveeditSlashCodeDesc(), "```go\n\n```", "code block fence", nil, slashLanguage},
		{media, "▦", locales.LiveeditSlashTable(), locales.LiveeditSlashTableDesc(), tableSyntax(2, 2), "table grid", nil, slashSize},
		{media, "▣", locales.LiveeditSlashImage(), locales.LiveeditSlashImageDesc(), "![" + locales.LiveeditSlashSampleCaption() + "](assets/image.png)", "image picture", (*Editor).slashImage, 0},
		{media, "—", locales.LiveeditSlashDivider(), locales.LiveeditSlashDividerDesc(), "---", "divider hr rule line", cmd(contract.CmdBlockHR), 0},
	}
	if e.opts.Exts&contract.ExtTOC != 0 {
		out = append(out, slashItem{media, "≡", locales.LiveeditSlashToc(), locales.LiveeditSlashTocDesc(), "[TOC]", "toc contents", cmd(contract.CmdBlockTOC), 0})
	}
	out = append(out,
		slashItem{media, "ⁿ", locales.LiveeditSlashFootnote(), locales.LiveeditSlashFootnoteDesc(), "[^1]: " + locales.LiveeditSlashSampleNote(), "footnote note", cmd(contract.CmdInsertFootnote), 0},
		slashItem{adv, "∑", locales.LiveeditSlashMath(), render, "$$\nE = mc^2\n$$", "math formula tex latex equation", cmd(contract.CmdBlockMath), 0},
		slashItem{adv, "◇", "Mermaid", render, "```mermaid\nflowchart LR\n```", "mermaid diagram chart flowchart", cmd(contract.CmdBlockMermaid), 0},
	)
	return out
}

// slashLanguages are the code step's languages, "" for plain text.
var slashLanguages = []string{"", "go", "python", "javascript", "typescript", "rust", "c", "cpp", "java", "csharp",
	"json", "yaml", "toml", "bash", "powershell", "sql", "html", "css", "markdown", "diff"}

// slashTyped opens the menu when s, just typed, is a "/" that makes an empty
// top-level paragraph.
func (e *Editor) slashTyped(s string) {

	if s != "/" || e.readOnly() {
		return
	}
	sel := e.model.Selection().Primary()
	if sel.Anchor != sel.Head {
		return
	}
	ast, text := e.model.AST(), e.model.Text()
	at := sel.Head - 1
	if at < 0 {
		return
	}
	b := ast.Block(ast.LeafAt(at))
	top := b.Parent < 0 || ast.Block(b.Parent).Kind == contract.BlockRoot
	if b.Kind != contract.BlockParagraph || !top || text.Slice(b.Content) != "/" {
		return
	}
	e.slash = slashMenu{open: true, at: at}
	e.invalidate()
}

// slashQuery returns what was typed after the "/", and false when the menu
// should close: the "/" is gone, the caret left it, or a line was ended.
func (e *Editor) slashQuery() (string, bool) {
	sm := &e.slash
	if !sm.open || e.readOnly() || !e.focused {
		return "", false
	}
	sel := e.model.Selection().Primary()
	text := e.model.Text()
	if sel.Anchor != sel.Head || sel.Head <= sm.at || sm.at+1 > text.Len() || sel.Head-sm.at-1 > slashQueryMax*4 {
		return "", false
	}
	if text.Slice(contract.Range{Start: sm.at, End: sm.at + 1}) != "/" {
		return "", false
	}
	q := text.Slice(contract.Range{Start: sm.at + 1, End: sel.Head})
	if strings.ContainsAny(q, "\r\n") || len([]rune(q)) > slashQueryMax {
		return "", false
	}
	return q, true
}

// slashMatches filters the items, or the languages, by the query.
func (e *Editor) slashMatches(q string) (items []slashItem, langs []string) {
	q = strings.ToLower(strings.TrimSpace(q))
	if e.slash.step == slashLanguage {
		for _, l := range slashLanguages {
			name := l
			if l == "" {
				name = strings.ToLower(locales.LiveeditSlashPlain()) + " text plain"
			}
			if q == "" || strings.Contains(name, q) {
				langs = append(langs, l)
			}
		}
		return nil, langs
	}
	for _, it := range e.slashItems() {
		if q == "" || strings.Contains(strings.ToLower(it.label), q) || strings.Contains(strings.ToLower(it.glyph), q) ||
			strings.Contains(it.keys, q) {
			items = append(items, it)
		}
	}
	return items, nil
}

func (e *Editor) slashCount(q string) int {
	items, langs := e.slashMatches(q)
	if e.slash.step == slashLanguage {
		return len(langs)
	}
	return len(items)
}

// SlashOpen reports whether the slash menu is up, for tests.
func (e *Editor) SlashOpen() bool {
	_, ok := e.slashQuery()
	return ok
}

func (e *Editor) closeSlash() {
	if e.slash.open {
		e.slash.open = false
		e.invalidate()
	}
}

// slashKey takes the menu's keys while it is up.
func (e *Editor) slashKey(ev ui.InputEvent) bool {
	q, ok := e.slashQuery()
	if !ok {
		e.slash.open = false
		return false
	}
	sm := &e.slash
	if ev.Mods&^ui.Shift != 0 {
		return false
	}
	if sm.step == slashSize {
		switch ev.Key {
		case ui.KeyUp:
			sm.rows = max(2, sm.rows-1)
		case ui.KeyDown:
			sm.rows = min(tableMaxRows, sm.rows+1)
		case ui.KeyLeft:
			sm.cols = max(1, sm.cols-1)
		case ui.KeyRight:
			sm.cols = min(tableMaxCols, sm.cols+1)
		case ui.KeyEnter:
			e.slashTable(sm.rows, sm.cols)
		case ui.KeyEscape:
			e.closeSlash()
		default:
			return false
		}
		e.invalidate()
		return true
	}
	n := e.slashCount(q)
	switch ev.Key {
	case ui.KeyDown:
		if n > 0 {
			sm.sel = (sm.sel + 1) % n
			sm.scroll = true
		}
	case ui.KeyUp:
		if n > 0 {
			sm.sel = (sm.sel - 1 + n) % n
			sm.scroll = true
		}
	case ui.KeyEnter, ui.KeyTab:
		if n == 0 {
			// Nothing to insert: Enter is the document's again.
			e.closeSlash()
			return false
		}
		e.slashChoose(q, min(sm.sel, n-1))
	case ui.KeyEscape:
		e.closeSlash()
	default:
		return false
	}
	e.invalidate()
	return true
}

// slashChoose inserts the i-th match, or goes on to its next step.
func (e *Editor) slashChoose(q string, i int) {
	sm := &e.slash
	items, langs := e.slashMatches(q)
	if sm.step == slashLanguage {
		e.slashCommand(contract.EditCommand{ID: contract.CmdBlockCode, Text: langs[i]})
		return
	}
	it := items[i]
	if it.next == 0 {
		it.run(e)
		return
	}
	// The next step starts with an empty filter: the query goes, the "/"
	// stays, in the step the insertion joins.
	e.slashClear(sm.at + 1)
	sm.step, sm.sel, sm.scroll = it.next, 0, true
	sm.rows, sm.cols = 3, 3
	sm.list = ui.ListState{}
}

// slashClear deletes from `from` to the caret: the query, when a step
// starts with an empty filter.
func (e *Editor) slashClear(from int) {
	head := e.model.Selection().Primary().Head
	c := contract.Caret(from)
	e.apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: from, End: head}}},
		Selection: &c, Origin: contract.OriginUser, Label: "editing_undo_typing"})
}

// slashCommand replaces the "/" and its query with the block cmd makes, as
// one undo step: the command is worked out on the text without them, then
// applied with their deletion in one transaction.
func (e *Editor) slashCommand(cmd contract.EditCommand) {
	from := e.slash.at
	e.closeSlash()
	head := e.model.Selection().Primary().Head
	c := contract.Caret(from)
	if !e.apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: from, End: head}}},
		Selection: &c, Origin: contract.OriginUser}) {
		return
	}
	tx, ok, err := e.opts.Editing.Run(e.editContext(), cmd)
	e.model.Undo()
	e.model.BreakCoalescing()
	if err != nil || !ok {
		return
	}
	tx.Edits = unshift(tx.Edits, from, head-from)
	e.apply(tx)
	e.model.BreakCoalescing()
}

// unshift maps edits made on a text without [from, from+d) onto the text
// with it: the edit that reaches from takes the bytes in, or a deletion of
// them joins the edits.
func unshift(edits []contract.TextEdit, from, d int) []contract.TextEdit {
	out := make([]contract.TextEdit, 0, len(edits)+1)
	taken := false
	for _, ed := range edits {
		r := ed.Range
		switch {
		case r.End < from:
		case r.Start <= from && !taken:
			r.End += d
			taken = true
		default:
			r.Start, r.End = r.Start+d, r.End+d
		}
		if !taken && r.Start > from {
			out = append(out, contract.TextEdit{Range: contract.Range{Start: from, End: from + d}})
			taken = true
		}
		out = append(out, contract.TextEdit{Range: r, Text: ed.Text})
	}
	if !taken {
		out = append(out, contract.TextEdit{Range: contract.Range{Start: from, End: from + d}})
	}
	return out
}

func (e *Editor) slashTable(rows, cols int) {
	e.slashCommand(contract.EditCommand{ID: contract.CmdTableInsert, Rows: rows - 1, Cols: cols})
}

// slashImage inserts an image with the caret in its description, as Typora's
// Insert Image does.
func (e *Editor) slashImage() {
	at := e.slash.at
	e.closeSlash()
	head := e.model.Selection().Primary().Head
	c := contract.Caret(at + 2)
	e.apply(contract.Transaction{Edits: []contract.TextEdit{{Range: contract.Range{Start: at, End: head}, Text: "![]()"}},
		Selection: &c, Origin: contract.OriginUser, Label: "editing_undo_typing"})
	e.model.BreakCoalescing()
}

type slashPill struct{}

// SlashMenu builds the panel under the "/" (02 §5.3): the list on the left,
// grouped, with a sliding pill; the choice's preview on the right. It returns
// the active menu item or grid cell for ActiveDescendant.
func (e *Editor) SlashMenu(c *ui.Context) ui.Element {
	var active ui.Element
	q, ok := e.slashQuery()
	if !ok {
		e.slash.open = false
		return active
	}
	if e.preedit != "" {
		return active // the input method's candidates are where the panel would be
	}
	sm := &e.slash
	x, y, h := e.caretRect()
	top := y + h + 4
	if top+slashH > e.viewH-8 && y-slashH-4 >= 8 {
		top = y - slashH - 4
	}
	left := max(8, min(x-4, e.viewW-slashW-8))
	t := e.t
	panel := ui.Row(c).Absolute().Left(left).Top(top).Size(slashW, slashH).Radius(14).Clip().
		Border(1, t.Border).Shadow(0, 10, 30, 0, t.ShadowColor).Material(effects.Glass(t, effects.Panel)).
		Role(ui.RoleMenu).Label(locales.LiveeditSlashMenu()).Cursor(ui.CursorDefault).KeepFocus()
	if !e.reduceMotion {
		panel.Transition(ui.ElementTransition{Animation: &motion.Snappy,
			Enter: &ui.Motion{Y: -6, Scale: .97, Opacity: 0}, Exit: &ui.Motion{Y: -6, Scale: .97, Opacity: 0}})
	}
	px, py, in := panel.PointerPosition()
	sm.moved = in && (px != sm.px || py != sm.py)
	sm.px, sm.py = px, py
	panel.Children(func() {
		var label, desc, syntax string
		switch sm.step {
		case slashSize:
			active = e.slashGrid(c)
			label = locales.LiveeditSlashSizeN(strconv.Itoa(sm.rows), strconv.Itoa(sm.cols))
			desc = locales.LiveeditSlashSizeDesc()
			syntax = tableSyntax(min(sm.rows, 3), min(sm.cols, 4))
		case slashLanguage:
			_, langs := e.slashMatches(q)
			active = e.slashList(c, q, len(langs))
			if len(langs) > 0 {
				l := langs[min(sm.sel, len(langs)-1)]
				label, desc, syntax = locales.LiveeditSlashLanguage(), locales.LiveeditSlashLanguageDesc(), "```"+l+"\n\n```"
			}
		default:
			items, _ := e.slashMatches(q)
			active = e.slashList(c, q, len(items))
			if len(items) > 0 {
				it := items[min(sm.sel, len(items)-1)]
				label, desc, syntax = it.label, it.desc, it.syntax
			}
		}
		ui.Column(c).Grow(1).FillHeight().MinWidth(0).Padding(14).Gap(10).Background(t.Surface).Children(func() {
			if label == "" {
				return
			}
			ui.Text(c, label).FontSize(theme.UIText).FontWeight(600).TextColor(t.Text)
			ui.Text(c, desc).FontSize(theme.UIStatus).TextColor(t.Muted)
			ui.Spacer(c)
			ui.Text(c, syntax).Font(theme.Mono).FontSize(theme.UIStatus).TextColor(t.Muted).
				Padding(10).Radius(8).Background(t.CodeBg).Border(1, t.Border).FillWidth()
		})
	})
	return active
}

// slashList is the left column: the items under their groups, or the
// languages.
func (e *Editor) slashList(c *ui.Context, q string, n int) ui.Element {
	var active ui.Element
	sm := &e.slash
	t := e.t
	col := ui.Column(c).Width(slashListW).FillHeight().Padding(5)
	col.Children(func() {
		if n == 0 {
			ui.Text(c, locales.LiveeditSlashEmpty()).FontSize(theme.UIText).TextColor(t.Muted).Padding(12, 8)
			return
		}
		sel := min(sm.sel, n-1)
		items, langs := e.slashMatches(q)
		// Rows: a group's title before its first item.
		type row struct {
			head string
			item int
		}
		var rows []row
		selRow := 0
		last := ""
		for i := 0; i < n; i++ {
			if sm.step == slashItems && items[i].group != last {
				last = items[i].group
				rows = append(rows, row{head: last, item: -1})
			}
			if i == sel {
				selRow = len(rows)
			}
			rows = append(rows, row{item: i})
		}
		if sm.step == slashLanguage {
			rows = append([]row{{head: locales.LiveeditSlashLanguage(), item: -1}}, rows...)
			selRow++
		}
		if sm.scroll {
			sm.scroll = false
			sm.list.ScrollIntoView(selRow)
		}
		ui.List(c.Key("slashRows"), &sm.list, len(rows), func(k int) {
			r := rows[k]
			if r.item < 0 {
				ui.Text(c.Key(k), r.head).FontSize(theme.UIBadge).FontWeight(600).TextColor(t.Muted).Padding(7, 8, 3)
				return
			}
			i := r.item
			var glyph, label string
			if sm.step == slashLanguage {
				glyph, label = "</>", langs[i]
				if label == "" {
					label = locales.LiveeditSlashPlain()
				}
			} else {
				glyph, label = items[i].glyph, items[i].label
			}
			b := ui.Row(c.Key(k)).Height(slashRowH).Padding(0, 8).Gap(9).AlignItems(ui.Center).Radius(8).
				Role(ui.RoleMenuItem).Label(label).Checked(i == sel).KeepFocus()
			if i == sel {
				active = b
			}
			b.Children(func() {
				if i == sel {
					p := ui.Box(c.Key("pill")).Absolute().Fill().Radius(8).Background(t.AccentSoft)
					if !e.reduceMotion {
						p.Transition(ui.ElementTransition{Animation: &motion.Snappy, Match: slashPill{}})
					}
				}
				ui.Row(c).Size(22, 22).Radius(6).Justify(ui.Center).AlignItems(ui.Center).Background(t.Surface).Children(func() {
					ui.Text(c, glyph).Font(theme.Mono).FontSize(10).TextColor(t.Muted).SingleLine()
				})
				ui.Text(c, label).FontSize(theme.UIText).TextColor(t.Text).SingleLine()
			})
			if b.Hovered() && sm.sel != i && sm.moved {
				sm.sel = i
				e.invalidate()
			}
			if b.Clicked() {
				sm.sel = i
				e.slashChoose(q, i)
				e.invalidate()
			}
		}).Grow(1).MinHeight(0)
	})
	return active
}

// slashGrid is the size step: a grid of cells, the chosen size lit; a cell
// under the pointer chooses its size, a click inserts it.
func (e *Editor) slashGrid(c *ui.Context) ui.Element {
	var active ui.Element
	sm := &e.slash
	t := e.t
	const cell, gap = 20, 4
	ui.Column(c).Width(slashListW).FillHeight().Padding(12).Gap(10).Children(func() {
		ui.Text(c, locales.LiveeditSlashSize()).FontSize(theme.UIBadge).FontWeight(600).TextColor(t.Muted)
		ui.Column(c).Gap(gap).Children(func() {
			for r := 1; r <= tableMaxRows; r++ {
				ui.Row(c.Key(r)).Gap(gap).Children(func() {
					for k := 1; k <= tableMaxCols; k++ {
						b := ui.Box(c.Key(k)).Size(cell, cell).Radius(4).Border(1, t.Border).KeepFocus().
							Role(ui.RoleMenuItem).Label(locales.LiveeditSlashSizeN(strconv.Itoa(r), strconv.Itoa(k))).
							Checked(r <= sm.rows && k <= sm.cols)
						if r == sm.rows && k == sm.cols {
							active = b
						}
						if r <= sm.rows && k <= sm.cols {
							b.Background(t.AccentSoft).Border(1, t.Accent.Alpha(.6))
						} else {
							b.Background(t.Surface)
						}
						if b.Hovered() && sm.moved && (sm.rows != max(r, 2) || sm.cols != k) {
							sm.rows, sm.cols = max(r, 2), k
							e.invalidate()
						}
						if b.Clicked() {
							e.slashTable(max(r, 2), k)
							e.invalidate()
						}
					}
				})
			}
		})
	})
	return active
}

// tableSyntax is the Markdown of an empty table, for the preview.
func tableSyntax(rows, cols int) string {
	var b strings.Builder
	line := func(cell string) {
		b.WriteString("|")
		for range cols {
			b.WriteString(cell + "|")
		}
		b.WriteString("\n")
	}
	line("   ")
	line(" --- ")
	for range rows - 1 {
		line("   ")
	}
	return strings.TrimSuffix(b.String(), "\n")
}
