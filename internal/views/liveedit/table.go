package liveedit

import (
	"strconv"

	"aurink/internal/contract"
	"aurink/internal/views/effects"
	"aurink/internal/views/kit"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// inTable reports whether the caret is in a table cell.
func (e *Editor) inTable() bool {
	r := e.caretRow
	return r >= 0 && r < len(e.rows) && e.rows[r].kind == contract.BlockTableCell
}

// tableEdit runs a table command and keeps the focus in the document.
func (e *Editor) tableEdit(cmd contract.EditCommand) {
	// A command that does not apply here does nothing (ok false), as a shortcut.
	_ = e.RunEdit(cmd)
	e.focusReq = true
	e.invalidate()
}

// tableShape is the table holding the caret: its body rows (header not
// counted), its columns, the caret's column and that column's alignment.
func (e *Editor) tableShape() (rows, cols, col int, align contract.Align, ok bool) {
	if !e.inTable() {
		return 0, 0, 0, 0, false
	}
	b := e.block(e.caretRow)
	tr := e.ast.Block(b.Parent)
	if tr == nil || tr.Parent < 0 {
		return 0, 0, 0, 0, false
	}
	tbl := e.ast.Block(tr.Parent)
	for i := tr.Parent + 1; i < e.ast.Len(); i++ {
		x := e.ast.Block(i)
		if x.Range.Start >= tbl.Range.End {
			break
		}
		if x.Kind == contract.BlockTableRow && x.Parent == tr.Parent {
			rows++
		}
	}
	col, cols = e.rows[e.caretRow].col, e.rows[e.caretRow].cols
	if col < len(tbl.Align) {
		align = tbl.Align[col]
	}
	return rows - 1, cols, col, align, true
}

// TableBar shows Typora's table toolbar over the table holding the caret: on
// the left the size grid and the alignment of the caret's column, on the
// right a menu of the row and column commands and Delete Table.
func (e *Editor) TableBar(c *ui.Context) {
	// The More menu takes the focus while it shows; it marks itself built in
	// each frame it shows, and the toolbar stays for it.
	menu := e.tableMenuOpen
	e.tableMenuOpen = false
	if !e.focused && !menu || e.readOnly() || !e.inTable() {
		e.tableGrid = false
		return
	}
	first, last, ok := e.tableRows(e.caretRow)
	rows, cols, _, align, shaped := e.tableShape()
	if !ok || !shaped {
		return
	}
	top := float32(e.rowTop(first)-e.scroll) + tableGap
	bottom := float32(e.rowTop(last)-e.scroll) + e.rows[last].h - tableGap
	if bottom < 40 || top > e.viewH {
		return
	}
	// Above the table, kept in view while the table is.
	y := max(top-32, 4)
	left, right := e.contentX(), e.viewW-e.contentX()-e.width
	ui.Row(c.Key("tableLeft")).Absolute().Left(left - 4).Top(y).Height(28).Gap(2).AlignItems(ui.Center).
		Label(locales.LiveeditTableBar()).Children(func() {
		if e.tableTool(c.Key("size"), kit.IconGrid, locales.LiveeditTableSize(), e.tableGrid).Clicked() {
			e.tableGrid = !e.tableGrid
			e.gridRows, e.gridCols = rows+1, cols
		}
		for _, a := range []struct {
			key   string
			icon  *ui.SVG
			label string
			align contract.Align
		}{
			{"left", kit.IconAlignLeft, locales.LiveeditAlignLeft(), contract.AlignLeft},
			{"center", kit.IconAlignCenter, locales.LiveeditAlignCenter(), contract.AlignCenter},
			{"right", kit.IconAlignRight, locales.LiveeditAlignRight(), contract.AlignRight},
		} {
			// As in Typora, the lit alignment clicked again goes back to none.
			if e.tableTool(c.Key(a.key), a.icon, a.label, align == a.align).Clicked() {
				to := a.align
				if align == a.align {
					to = contract.AlignNone
				}
				e.tableEdit(contract.EditCommand{ID: contract.CmdTableAlign, Align: to})
			}
		}
	})
	ui.Row(c.Key("tableRight")).Absolute().Right(right - 4).Top(y).Height(28).Gap(2).AlignItems(ui.Center).Children(func() {
		e.tableTool(c.Key("more"), kit.IconMore, locales.LiveeditTableMore(), menu).PopoverMenuButton(func(m *ui.Menu) {
			e.tableMenuOpen = true
			e.tableMenu(m)
		})
		if e.tableTool(c.Key("delete"), kit.IconTrash, locales.LiveeditDeleteTable(), false).Clicked() {
			e.tableEdit(contract.EditCommand{ID: contract.CmdTableDelete})
		}
	})
	if e.tableGrid {
		e.tableSizeGrid(c, left-4, y+30)
	}
}

// tableTool is an icon button of the table toolbar; it leaves the focus in the
// document, so the caret stays in its cell.
func (e *Editor) tableTool(c *ui.Context, icon *ui.SVG, label string, on bool) ui.Element {
	t := e.t
	b := ui.Row(c).Size(28, 28).Justify(ui.Center).AlignItems(ui.Center).Radius(6).
		Role(ui.RoleButton).Label(label).Tooltip(label).KeepFocus()
	color := t.Muted
	switch {
	case on:
		b.Background(t.AccentSoft)
		color = t.Accent
	case b.Hovered():
		b.Background(t.Hover)
		color = t.Text
	}
	b.Children(func() { ui.Icon(c, icon).Size(16, 16).TextColor(color) })
	return b
}

// tableSizeGrid is the size grid under the size button: the table's size lit,
// the size under the pointer lit as it moves, a click resizes to it.
func (e *Editor) tableSizeGrid(c *ui.Context, x, y float32) {
	t := e.t
	const cell, gap = 18, 3
	maxR, maxC := min(max(8, e.gridRows+2), 16), min(max(8, e.gridCols+2), 12)
	panel := ui.Column(c.Key("tableGrid")).Absolute().Left(x).Top(y).Padding(10).Gap(8).Radius(10).
		Border(1, t.Border).Shadow(0, 12, 32, 0, t.ShadowColor).Material(effects.Glass(t, effects.Panel)).
		Label(locales.LiveeditTableSize())
	panel.Children(func() {
		ui.Text(c, locales.LiveeditSlashSizeN(strconv.Itoa(e.gridRows), strconv.Itoa(e.gridCols))).
			FontSize(theme.UIBadge).TextColor(t.Muted)
		ui.Column(c).Gap(gap).Children(func() {
			for r := 1; r <= maxR; r++ {
				ui.Row(c.Key(r)).Gap(gap).Children(func() {
					for k := 1; k <= maxC; k++ {
						b := ui.Box(c.Key(k)).Size(cell, cell).Radius(3).KeepFocus().Role(ui.RoleButton).
							Label(locales.LiveeditSlashSizeN(strconv.Itoa(r), strconv.Itoa(k)))
						if r <= e.gridRows && k <= e.gridCols {
							b.Background(t.AccentSoft).Border(1, t.Accent.Alpha(.6))
						} else {
							b.Background(t.Surface).Border(1, t.Border)
						}
						// The header row counts: a table has at least it.
						if b.Hovered() && (e.gridRows != r || e.gridCols != k) {
							e.gridRows, e.gridCols = r, k
							e.invalidate()
						}
						if b.Clicked() {
							e.tableGrid = false
							e.tableEdit(contract.EditCommand{ID: contract.CmdTableResize, Rows: r - 1, Cols: k})
						}
					}
				})
			}
		})
	})
}

// tableMenu fills the context menu of a table cell (02 §5.8): insert, delete,
// align.
func (e *Editor) tableMenu(m *ui.Menu) {
	if e.readOnly() || !e.inTable() {
		return
	}
	run := func(id contract.CommandID) { e.tableEdit(contract.EditCommand{ID: id}) }
	for _, it := range []struct {
		label func() string
		id    contract.CommandID
	}{
		{locales.LiveeditRowAbove, contract.CmdTableRowAbove},
		{locales.LiveeditRowBelow, contract.CmdTableRowBelow},
		{locales.LiveeditColLeft, contract.CmdTableColLeft},
		{locales.LiveeditColRight, contract.CmdTableColRight},
	} {
		if m.Item(it.label()).Chosen() {
			run(it.id)
		}
	}
	m.Separator()
	if m.Item(locales.LiveeditDeleteRow()).Chosen() {
		run(contract.CmdTableDeleteRow)
	}
	if m.Item(locales.LiveeditDeleteCol()).Chosen() {
		run(contract.CmdTableDeleteCol)
	}
	m.Separator()
	for _, it := range []struct {
		label func() string
		align contract.Align
	}{
		{locales.LiveeditAlignLeft, contract.AlignLeft},
		{locales.LiveeditAlignCenter, contract.AlignCenter},
		{locales.LiveeditAlignRight, contract.AlignRight},
	} {
		if m.Item(it.label()).Chosen() {
			e.tableEdit(contract.EditCommand{ID: contract.CmdTableAlign, Align: it.align})
		}
	}
	m.Separator()
	if m.Item(locales.LiveeditFormatTable()).Chosen() {
		run(contract.CmdTableFormat)
	}
}
