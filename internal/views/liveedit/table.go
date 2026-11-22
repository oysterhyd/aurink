package liveedit

import (
	"slices"
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
		Role(ui.RoleToolbar).Label(locales.LiveeditTableBar()).Children(func() {
		if e.tableTool(c.Key("size"), kit.IconGrid, locales.LiveeditTableSize(), e.tableGrid, true).Clicked() {
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
			if e.tableTool(c.Key(a.key), a.icon, a.label, align == a.align, true).Clicked() {
				to := a.align
				if align == a.align {
					to = contract.AlignNone
				}
				e.tableEdit(contract.EditCommand{ID: contract.CmdTableAlign, Align: to})
			}
		}
	})
	ui.Row(c.Key("tableRight")).Absolute().Right(right - 4).Top(y).Height(28).Gap(2).AlignItems(ui.Center).
		Role(ui.RoleToolbar).Label(locales.LiveeditTableBar()).Children(func() {
		e.tableTool(c.Key("more"), kit.IconMore, locales.LiveeditTableMore(), menu, false).PopoverMenuButton(func(m *ui.Menu) {
			e.tableMenuOpen = true
			e.tableMenu(m)
		})
		if e.tableTool(c.Key("delete"), kit.IconTrash, locales.LiveeditDeleteTable(), false, false).Clicked() {
			e.tableEdit(contract.EditCommand{ID: contract.CmdTableDelete})
		}
	})
	if e.tableGrid {
		e.tableSizeGrid(c, left-4, y+30)
	}
}

// tableTool is an icon button of the table toolbar; it leaves the focus in the
// document, so the caret stays in its cell.
func (e *Editor) tableTool(c *ui.Context, icon *ui.SVG, label string, on, toggle bool) ui.Element {
	t := e.t
	role := ui.RoleButton
	if toggle {
		role = ui.RoleToggleButton
	}
	b := ui.Row(c).Size(28, 28).Justify(ui.Center).AlignItems(ui.Center).Radius(6).
		Role(role).Label(label).Tooltip(label).KeepFocus()
	if toggle {
		b.Checked(on)
	}
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

type tableColCache struct {
	version contract.Version
	width   float32
	widths  []float32
}

const tableResizeHit = 5

func (e *Editor) tableInfo(r int) (tbl *contract.Block, first, last, cols int, ok bool) {
	if r < 0 || r >= len(e.rows) || e.rows[r].cols <= 0 {
		return nil, 0, 0, 0, false
	}
	b := e.block(r)
	if b.Kind != contract.BlockTableCell || b.Parent < 0 {
		return nil, 0, 0, 0, false
	}
	tr := e.ast.Block(b.Parent)
	if tr == nil || tr.Parent < 0 {
		return nil, 0, 0, 0, false
	}
	tbl = e.ast.Block(tr.Parent)
	first, last = e.rowAt(tbl.Range.Start), e.rowAt(max(tbl.Range.Start, tbl.Range.End-1))
	first -= e.rows[first].col
	last -= e.rows[last].col
	cols = e.rows[first].cols
	if cols <= 0 || !(first <= r && r < last+e.rows[last].cols) {
		return nil, 0, 0, 0, false
	}
	return tbl, first, last, cols, true
}

func cellVisualUnits(s string) int {
	n := 0
	for _, r := range s {
		switch {
		case r == '\r' || r == '\n':
		case r < 0x80:
			n++
		default:
			n += 2
		}
	}
	return n
}

func (e *Editor) minTableColWidth(cols int) float32 {
	if cols <= 1 {
		return max(e.width, 1)
	}
	return min(56, e.width/float32(cols)*0.5)
}

// TableColumnWidths returns the display widths of the columns of the table
// containing row r, for tests and layout.
func (e *Editor) TableColumnWidths(r int) []float32 {
	return slices.Clone(e.tableColWidths(r))
}

func (e *Editor) tableColWidths(r int) []float32 {
	tbl, first, last, cols, ok := e.tableInfo(r)
	if !ok || cols <= 1 || e.width <= 0 {
		return []float32{e.width}
	}
	if ratios := e.tableRatios[tbl.ID]; len(ratios) == cols {
		out := make([]float32, cols)
		var sum float32
		for k := 0; k < cols-1; k++ {
			out[k] = ratios[k] * e.width
			sum += out[k]
		}
		out[cols-1] = max(1, e.width-sum)
		return out
	}
	if c, ok := e.tableAuto[tbl.ID]; ok && c.version == e.ast.Version() && c.width == e.width && len(c.widths) == cols {
		return c.widths
	}
	maxUnits := make([]int, cols)
	endRow := min(last+e.rows[last].cols, len(e.rows))
	for row := first; row < endRow; {
		step := max(e.rows[row].cols, 1)
		for k := 0; k < step && row+k < endRow; k++ {
			if col := e.rows[row+k].col; col >= 0 && col < cols {
				u := cellVisualUnits(e.text.Slice(e.block(row + k).Content))
				if u > maxUnits[col] {
					maxUnits[col] = u
				}
			}
		}
		row += step
	}
	minU, maxU := maxUnits[0], maxUnits[0]
	for _, u := range maxUnits[1:] {
		minU = min(minU, u)
		maxU = max(maxU, u)
	}
	out := make([]float32, cols)
	if maxU-minU <= 2 {
		w := e.width / float32(cols)
		var sum float32
		for k := 0; k < cols-1; k++ {
			out[k] = w
			sum += w
		}
		out[cols-1] = max(1, e.width-sum)
	} else {
		minW := e.minTableColWidth(cols)
		weights := make([]float32, cols)
		var totalW float32
		for k, u := range maxUnits {
			// Dampen extreme lengths while giving wide content proportionally more room.
			capped := min(u, 72)
			weights[k] = float32(6 + capped)
			totalW += weights[k]
		}
		var sum float32
		for k := 0; k < cols; k++ {
			out[k] = max(minW, e.width*weights[k]/totalW)
			sum += out[k]
		}
		if sum != e.width && sum > 0 {
			scale := e.width / sum
			var acc float32
			for k := 0; k < cols-1; k++ {
				out[k] *= scale
				acc += out[k]
			}
			out[cols-1] = max(1, e.width-acc)
		}
	}
	if e.tableAuto == nil {
		e.tableAuto = map[contract.BlockID]tableColCache{}
	}
	if prev, had := e.tableAuto[tbl.ID]; had && len(prev.widths) == len(out) && !slices.Equal(prev.widths, out) {
		for row := first; row < endRow; row++ {
			if len(e.rows[row].forms) > 0 {
				e.dropForms(row)
			}
		}
	}
	e.tableAuto[tbl.ID] = tableColCache{version: e.ast.Version(), width: e.width, widths: out}
	return out
}

func (e *Editor) tableColBox(r int) (left, width float32) {
	v := &e.rows[r]
	if v.cols <= 1 {
		return 0, e.width
	}
	ws := e.tableColWidths(r)
	col := min(max(v.col, 0), len(ws)-1)
	for k := 0; k < col; k++ {
		left += ws[k]
	}
	return left, ws[col]
}

func (e *Editor) tableDividerAt(x, y float32) (tblID contract.BlockID, first, last, col int, ok bool) {
	r := e.rowAtPoint(x, y)
	if r < 0 {
		return 0, 0, 0, 0, false
	}
	tbl, first, last, cols, ok := e.tableInfo(r)
	if !ok || cols <= 1 {
		return 0, 0, 0, 0, false
	}
	top, _ := e.tableGaps(first)
	_, bottom := e.tableGaps(last)
	yTop := float32(e.rowTop(first)-e.scroll) + top
	yBot := float32(e.rowTop(last)-e.scroll) + e.rows[last].h - bottom
	if y < yTop || y > yBot {
		return 0, 0, 0, 0, false
	}
	ws := e.tableColWidths(first)
	relX := x - e.contentX()
	var edge float32
	for k := 0; k < cols-1; k++ {
		edge += ws[k]
		if relX >= edge-tableResizeHit && relX <= edge+tableResizeHit {
			return tbl.ID, first, last, k, true
		}
	}
	return 0, 0, 0, 0, false
}

func (e *Editor) dropTableForms(first, last int) {
	end := min(last+e.rows[last].cols, len(e.rows))
	for r := first; r < end; r++ {
		e.dropForms(r)
		if e.rows[r].h > 0 {
			e.touched = append(e.touched, e.rows[r].id)
		}
	}
}

func (e *Editor) dragTableDividerTo(x float32) {
	if e.dragTableID == 0 || e.width <= 0 || e.dragTableFirst < 0 || e.dragTableFirst >= len(e.rows) {
		return
	}
	ws := slices.Clone(e.tableColWidths(e.dragTableFirst))
	cols := len(ws)
	col := e.dragTableCol
	if col < 0 || col+1 >= cols {
		return
	}
	var leftEdge float32
	for k := 0; k < col; k++ {
		leftEdge += ws[k]
	}
	pairW := ws[col] + ws[col+1]
	minW := min(e.minTableColWidth(cols), pairW/2)
	wantLeft := min(max(x-e.contentX()-leftEdge, minW), pairW-minW)
	ws[col] = wantLeft
	ws[col+1] = pairW - wantLeft
	ratios := make([]float32, cols)
	for k, w := range ws {
		ratios[k] = w / e.width
	}
	if e.tableRatios == nil {
		e.tableRatios = map[contract.BlockID][]float32{}
	}
	e.tableRatios[e.dragTableID] = ratios
	e.dropTableForms(e.dragTableFirst, e.dragTableLast)
	e.invalidate()
}
