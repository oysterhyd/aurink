package editing

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/width"

	"aurink/internal/contract"
)

type tableCell struct {
	start, end int
	content    contract.Range
}

type tableLine struct {
	line              sourceLine
	start             int
	cells             []tableCell
	leading, trailing bool
}

type sourceTable struct {
	block     *contract.Block
	rows      []tableLine
	delimiter tableLine
	row, col  int
}

// Missing cells in a ragged row are implicit. Column insertion materializes
// only the missing cells needed to reach the inserted column; deletion leaves
// other missing cells implicit. Existing outer pipes are retained; outer pipes
// are added where an empty first or final cell needs one to remain explicit.
func parseTableLine(ctx contract.EditContext, l sourceLine, start int) tableLine {
	s := ctx.Text.String()
	t := tableLine{line: l, start: start}
	end := l.end
	if start > end {
		return t
	}
	p := start
	if p < end && s[p] == '|' {
		t.leading = true
		p++
	}
	last := end
	for last > p && space(s[last-1]) {
		last--
	}
	t.trailing = last > p && s[last-1] == '|' && !escaped(s, last-1)
	limit := end
	if t.trailing {
		limit = last - 1
	}
	for {
		q := p
		for q < limit && (s[q] != '|' || escaped(s, q)) {
			q++
		}
		a, b := p, q
		for a < b && space(s[a]) {
			a++
		}
		for b > a && space(s[b-1]) {
			b--
		}
		if a == b && p < q {
			a, b = p+(q-p)/2, p+(q-p)/2
		}
		t.cells = append(t.cells, tableCell{start: p, end: q, content: contract.Range{Start: a, End: b}})
		if q == limit {
			break
		}
		p = q + 1
	}
	return t
}

func findTable(ctx contract.EditContext) (sourceTable, bool) {
	r := ctx.Selection.Primary().Range()
	b := ancestor(ctx, r.Start, contract.BlockTable)
	if b == nil || ctx.AST == nil {
		return sourceTable{}, false
	}
	i, ok := ctx.AST.Index(b.ID)
	if !ok {
		return sourceTable{}, false
	}
	t := sourceTable{block: b, row: -1, col: -1}
	for j := range ctx.AST.Children(i) {
		row := ctx.AST.Block(j)
		if row.Kind != contract.BlockTableRow {
			continue
		}
		l := lineAt(ctx, row.Range.Start)
		t.rows = append(t.rows, parseTableLine(ctx, l, row.Range.Start))
	}
	if len(t.rows) == 0 {
		return sourceTable{}, false
	}
	l := lineAt(ctx, t.rows[0].line.next)
	t.delimiter = parseTableLine(ctx, l, l.start+l.content)
	for i, row := range t.rows {
		for j, c := range row.cells {
			if r.Start >= c.start && r.End <= c.end && j < len(t.rows[0].cells) {
				t.row, t.col = i, j
				return t, true
			}
		}
	}
	return sourceTable{}, false
}

func tableCommand(ctx contract.EditContext, cmd contract.EditCommand) (contract.Transaction, bool, error) {
	if cmd.ID == contract.CmdTableInsert {
		return insertTable(ctx, cmd)
	}
	t, ok := findTable(ctx)
	if !ok || len(t.rows[0].cells) == 0 || len(t.delimiter.cells) < len(t.rows[0].cells) {
		return contract.Transaction{}, false, nil
	}
	cols := len(t.rows[0].cells)
	switch cmd.ID {
	case contract.CmdTableResize:
		if cmd.Rows < 0 || cmd.Cols < 1 || cmd.Rows == len(t.rows)-1 && cmd.Cols == cols {
			return contract.Transaction{}, false, nil
		}
		return resizeTable(ctx, t, cmd.Rows, cmd.Cols), true, nil
	case contract.CmdTableDelete:
		return deleteTable(ctx, t, contract.OriginCommand), true, nil
	case contract.CmdTableRowAbove, contract.CmdTableRowBelow:
		if cmd.ID == contract.CmdTableRowAbove && t.row == 0 {
			return contract.Transaction{}, false, nil
		}
		tx := addTableRow(ctx, t, cmd.ID == contract.CmdTableRowAbove, t.col)
		return tx, true, nil
	case contract.CmdTableDeleteRow:
		if t.row == 0 {
			return contract.Transaction{}, false, nil
		}
		return deleteTableRow(ctx, t), true, nil
	case contract.CmdTableColLeft, contract.CmdTableColRight, contract.CmdTableDeleteCol:
		if cmd.ID == contract.CmdTableDeleteCol && cols == 1 {
			return contract.Transaction{}, false, nil
		}
		return changeTableColumn(ctx, t, cmd.ID), true, nil
	case contract.CmdTableAlign:
		if cmd.Align > contract.AlignRight {
			return contract.Transaction{}, false, contract.E(contract.CodeInvalid, "editing.Run", nil)
		}
		cell := t.delimiter.cells[t.col].content
		dashes := strings.Trim(ctx.Text.Slice(cell), ":")
		if cmd.Align == contract.AlignLeft || cmd.Align == contract.AlignCenter {
			dashes = ":" + dashes
		}
		if cmd.Align == contract.AlignRight || cmd.Align == contract.AlignCenter {
			dashes += ":"
		}
		edits := []contract.TextEdit{edit(cell.Start, cell.End, dashes)}
		return transaction(edits, mapped(ctx.Selection, edits), contract.OriginCommand, "table"), true, nil
	case contract.CmdTableNextCell, contract.CmdTablePrevCell:
		row, col := t.row, t.col
		if cmd.ID == contract.CmdTableNextCell {
			col++
			if col == cols {
				row, col = row+1, 0
			}
			if row == len(t.rows) {
				return addTableRow(ctx, t, false, 0), true, nil
			}
		} else {
			col--
			if col < 0 {
				row, col = row-1, cols-1
			}
			if row < 0 {
				return contract.Transaction{}, false, nil
			}
		}
		if col >= len(t.rows[row].cells) {
			return materializeCell(ctx, t, row, col), true, nil
		}
		c := t.rows[row].cells[col].content
		return transaction(nil, selected(ctx, c.Start, c.End), contract.OriginCommand, "table"), true, nil
	case contract.CmdTableFormat:
		return formatTable(ctx, t)
	}
	return contract.Transaction{}, false, nil
}

func emptyTableRow(t sourceTable, cols int) string {
	parts := make([]string, cols)
	for i := range parts {
		parts[i] = "  "
	}
	// Outer pipes make both end cells unambiguously present when empty.
	return "|" + strings.Join(parts, "|") + "|"
}

func addTableRow(ctx contract.EditContext, t sourceTable, above bool, col int) contract.Transaction {
	row := t.rows[t.row]
	prefix := ctx.Text.String()[row.line.start:row.start]
	s := prefix + emptyTableRow(t, len(t.rows[0].cells))
	cellOffset := len(prefix) + 2 + 3*col
	at, text, caret := row.line.start, s+eol(ctx), 0
	if !above {
		if t.row == 0 {
			at = t.delimiter.line.end
		} else {
			at = row.line.end
		}
		text = eol(ctx) + s
		caret = at + len(eol(ctx)) + cellOffset
	} else {
		caret = at + cellOffset
	}
	return transaction([]contract.TextEdit{edit(at, at, text)}, contract.Caret(caret), contract.OriginCommand, "table")
}

func deleteTableRow(ctx contract.EditContext, t sourceTable) contract.Transaction {
	row := t.rows[t.row]
	var e contract.TextEdit
	target := t.rows[0]
	if t.row+1 < len(t.rows) {
		e = edit(row.line.start, row.line.next, "")
		target = t.rows[t.row+1]
	} else {
		previous := t.delimiter.line
		if t.row > 1 {
			previous = t.rows[t.row-1].line
			target = t.rows[t.row-1]
		}
		e = edit(previous.end, row.line.end, "")
	}
	col := min(t.col, len(target.cells)-1)
	sel := contract.Caret(target.cells[col].content.Start)
	edits := []contract.TextEdit{e}
	return transaction(edits, mapped(sel, edits), contract.OriginCommand, "table")
}

func changeTableColumn(ctx contract.EditContext, t sourceTable, id contract.CommandID) contract.Transaction {
	index := t.col
	if id == contract.CmdTableColRight {
		index++
	}
	lines := append([]tableLine{t.rows[0], t.delimiter}, t.rows[1:]...)
	var edits []contract.TextEdit
	targetEdit, targetOffset := -1, 0
	var target contract.Selection
	for i, row := range lines {
		selectedRow := i == 0 && t.row == 0 || i == t.row+1 && t.row > 0
		if id == contract.CmdTableDeleteCol {
			if index >= len(row.cells) {
				continue
			}
			if len(row.cells) == 1 {
				// The remaining logical cells were implicit; leave an empty
				// physical cell so the ragged body row remains a table row.
				value := "  "
				if !row.leading {
					value = "|" + value
				}
				if !row.trailing {
					value += "|"
				}
				if selectedRow {
					targetEdit, targetOffset = len(edits), 1
					if !row.leading {
						targetOffset++
					}
				}
				edits = append(edits, edit(row.cells[0].start, row.cells[0].end, value))
				continue
			}
			var a, b int
			if index+1 < len(row.cells) {
				a, b = row.cells[index].start, row.cells[index+1].start
			} else {
				a, b = row.cells[index-1].end, row.cells[index].end
			}
			if selectedRow {
				col := index
				if col+1 >= len(row.cells) {
					col--
				} else {
					col++
				}
				target = contract.Caret(row.cells[col].content.Start)
			}
			replacement := ""
			if i <= 1 && len(t.rows[0].cells) == 2 && !row.leading && !row.trailing {
				// One-column tables still need a pipe in the header and separator.
				replacement = "|"
			}
			edits = append(edits, edit(a, b, replacement))
			continue
		}
		value := "  "
		if i == 1 {
			value = " --- "
		}
		at, text, offset := 0, "", 0
		if index < len(row.cells) {
			at = row.cells[index].start
			text = value + "|"
			offset = 1
			if index == 0 && !row.leading && i != 1 {
				text = "|" + text
				offset++
			}
		} else {
			at = row.cells[len(row.cells)-1].end
			missing := index - len(row.cells)
			text = strings.Repeat("|  ", missing) + "|" + value
			offset = len(text) - len(value) + 1
			if !row.trailing && i != 1 {
				text += "|"
			}
		}
		if selectedRow {
			targetEdit, targetOffset = len(edits), offset
		}
		edits = append(edits, edit(at, at, text))
	}
	if id == contract.CmdTableDeleteCol && targetEdit < 0 {
		return transaction(edits, mapped(target, edits), contract.OriginCommand, "table")
	}
	shift := 0
	for _, e := range edits[:targetEdit] {
		shift += len(e.Text) - e.Range.Len()
	}
	return transaction(edits, contract.Caret(edits[targetEdit].Range.Start+shift+targetOffset), contract.OriginCommand, "table")
}

func materializeCell(ctx contract.EditContext, t sourceTable, rowIndex, col int) contract.Transaction {
	row := t.rows[rowIndex]
	at := row.cells[len(row.cells)-1].end
	s := strings.Repeat("|  ", col-len(row.cells)+1)
	if !row.trailing {
		s += "|"
	}
	caret := at + len(s) - 1
	if !row.trailing {
		caret--
	}
	return transaction([]contract.TextEdit{edit(at, at, s)}, contract.Caret(caret), contract.OriginCommand, "table")
}

func displayWidth(s string) int {
	n := 0
	for i, r := range s {
		if r == '\\' && i+1 < len(s) && strings.ContainsRune(`!"#$%&'()*+,-./:;<=>?@[\]^_`+"`"+`{|}~`, rune(s[i+1])) && !escaped(s, i) {
			continue
		}
		if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
			continue
		}
		if k := width.LookupRune(r).Kind(); k == width.EastAsianWide || k == width.EastAsianFullwidth {
			n += 2
		} else {
			n++
		}
	}
	return n
}

func formatTable(ctx contract.EditContext, t sourceTable) (contract.Transaction, bool, error) {
	cols := len(t.rows[0].cells)
	widths := make([]int, cols)
	for i := range widths {
		widths[i] = 3
	}
	for _, row := range t.rows {
		if len(row.cells) > cols {
			return contract.Transaction{}, false, nil
		}
		for i, c := range row.cells {
			widths[i] = max(widths[i], displayWidth(ctx.Text.Slice(c.content)))
		}
	}
	aligns := t.block.Align
	var out strings.Builder
	caret := 0
	writeRow := func(row tableLine, rowIndex int) {
		out.WriteString(ctx.Text.String()[row.line.start:row.start])
		out.WriteByte('|')
		for col := 0; col < cols; col++ {
			value := ""
			if col < len(row.cells) {
				value = ctx.Text.Slice(row.cells[col].content)
			}
			pad := widths[col] - displayWidth(value)
			left := 0
			if col < len(aligns) {
				if aligns[col] == contract.AlignRight {
					left = pad
				} else if aligns[col] == contract.AlignCenter {
					left = pad / 2
				}
			}
			out.WriteString(" " + strings.Repeat(" ", left))
			if rowIndex == t.row && col == t.col {
				old := row.cells[col].content
				caret = out.Len() + min(max(0, ctx.Selection.Primary().Head-old.Start), old.Len())
			}
			out.WriteString(value + strings.Repeat(" ", pad-left) + " |")
		}
	}
	start := t.rows[0].line.start
	writeRow(t.rows[0], 0)
	out.WriteString(ctx.Text.String()[t.rows[0].line.end:t.delimiter.line.start])
	out.WriteString(ctx.Text.String()[t.delimiter.line.start:t.delimiter.start])
	out.WriteByte('|')
	for i, w := range widths {
		left, right := false, false
		if i < len(aligns) {
			left = aligns[i] == contract.AlignLeft || aligns[i] == contract.AlignCenter
			right = aligns[i] == contract.AlignRight || aligns[i] == contract.AlignCenter
		}
		n := w
		if left {
			n--
		}
		if right {
			n--
		}
		out.WriteByte(' ')
		if left {
			out.WriteByte(':')
		}
		out.WriteString(strings.Repeat("-", max(1, n)))
		if right {
			out.WriteByte(':')
		}
		out.WriteString(" |")
	}
	previous := t.delimiter.line
	for i, row := range t.rows[1:] {
		out.WriteString(ctx.Text.String()[previous.end:row.line.start])
		writeRow(row, i+1)
		previous = row.line
	}
	end := previous.end
	return transaction([]contract.TextEdit{edit(start, end, out.String())}, contract.Caret(start+caret), contract.OriginCommand, "table"), true, nil
}

func tableIntent(ctx contract.EditContext, in contract.Intent) (contract.Transaction, bool) {
	t, ok := findTable(ctx)
	if !ok {
		return contract.Transaction{}, false
	}
	var id contract.CommandID
	switch in.Kind {
	case contract.IntentDeleteBackward:
		if in.Gran != contract.GranChar || !ctx.Selection.Primary().Empty() || !t.rows[t.row].cells[t.col].content.Empty() {
			return contract.Transaction{}, false
		}
		return backspaceEmptyTableCell(ctx, t), true
	case contract.IntentIndent:
		id = contract.CmdTableNextCell
	case contract.IntentOutdent:
		id = contract.CmdTablePrevCell
	case contract.IntentNewline:
		id = contract.CmdTableRowBelow
	case contract.IntentLineBreak:
		return replace(ctx.Selection.Primary().Range(), "<br>", contract.OriginUser, "newline"), true
	case contract.IntentInsertText:
		if !utf8.ValidString(in.Text) || !strings.ContainsAny(in.Text, "|\r\n") {
			return contract.Transaction{}, false
		}
		r := ctx.Selection.Primary().Range()
		return replace(r, tableText(in.Text, ctx.Text.String()[:r.Start]), contract.OriginUser, "typing"), true
	case contract.IntentPaste:
		if in.Paste == nil {
			return contract.Transaction{}, false
		}
		s := in.Paste.Plain
		if !in.Paste.AsPlain && in.Paste.Markdown != "" {
			s = in.Paste.Markdown
		}
		r := ctx.Selection.Primary().Range()
		if !in.Paste.AsPlain && !r.Empty() && s == in.Paste.Plain && singleURL(s) {
			s = "[" + ctx.Text.Slice(r) + "](" + linkDestination(s) + ")"
		}
		return replace(r, tableText(s, ctx.Text.String()[:r.Start]), contract.OriginPaste, "paste"), true
	default:
		return contract.Transaction{}, false
	}
	tx, handled, _ := tableCommand(ctx, contract.EditCommand{ID: id})
	if !handled {
		return transaction(nil, ctx.Selection, contract.OriginUser, "table"), true
	}
	tx.Origin = contract.OriginUser
	_ = t
	return tx, true
}

func tableText(s, before string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	var b strings.Builder
	slashes := 0
	for i := len(before) - 1; i >= 0 && before[i] == '\\'; i-- {
		slashes++
	}
	for _, r := range s {
		switch r {
		case '\n':
			b.WriteString("<br>")
			slashes = 0
		case '|':
			if slashes%2 == 0 {
				b.WriteByte('\\')
			}
			b.WriteRune(r)
			slashes = 0
		case '\\':
			b.WriteRune(r)
			slashes++
		default:
			b.WriteRune(r)
			slashes = 0
		}
	}
	return b.String()
}
