package liveedit

import (
	"aurink/internal/contract"
	"aurink/internal/views/effects"
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

// TableBar shows the glass toolbar over the table holding the caret (02 §5.8):
// a row below, a column right, and Format, which alone reflows the table.
func (e *Editor) TableBar(c *ui.Context) {
	if !e.focused || e.readOnly() || !e.inTable() {
		return
	}
	first, last, ok := e.tableRows(e.caretRow)
	if !ok {
		return
	}
	t := e.t
	top := float32(e.rowTop(first)-e.scroll) + tableGap
	bottom := float32(e.rowTop(last)-e.scroll) + e.rows[last].h - tableGap
	if bottom < 40 || top > e.viewH {
		return
	}
	// Above the table, kept in view while the table is.
	y := max(top-38, 6)
	bar := ui.Row(c).Absolute().Top(y).Right(e.viewW-e.contentX()-e.width).Height(30).Padding(0, 4).Gap(2).
		AlignItems(ui.Center).Radius(9).Border(1, t.Border).Shadow(0, 6, 18, 0, t.ShadowColor).
		Material(effects.Glass(t, effects.Chip)).Label(locales.LiveeditTableBar())
	bar.Children(func() {
		e.barButton(c.Key("row"), "+ "+locales.LiveeditRow(), locales.LiveeditRowBelow(), contract.CmdTableRowBelow)
		e.barButton(c.Key("col"), "+ "+locales.LiveeditColumn(), locales.LiveeditColRight(), contract.CmdTableColRight)
		ui.Box(c).Size(1, 16).Background(t.Border)
		e.barButton(c.Key("format"), locales.LiveeditFormatTable(), locales.LiveeditFormatTable(), contract.CmdTableFormat)
	})
}

func (e *Editor) barButton(c *ui.Context, text, label string, id contract.CommandID) {
	t := e.t
	b := ui.Row(c).Height(24).Padding(0, 8).Radius(6).AlignItems(ui.Center).Role(ui.RoleButton).Label(label).KeepFocus()
	if b.Hovered() {
		b.Background(t.Hover)
	}
	b.Children(func() {
		ui.Text(c, text).FontSize(theme.UIBadge + 1).TextColor(t.Text)
	})
	if b.Clicked() {
		e.tableEdit(contract.EditCommand{ID: id})
	}
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
