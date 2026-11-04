package editing

import (
	"strings"

	"aurink/internal/contract"
)

// resizeTableLine retains the source of each surviving cell, including its
// padding, and changes only the cells at the right edge of the row.
func resizeTableLine(ctx contract.EditContext, row tableLine, oldCols, cols, target int, delimiter bool) (string, int) {
	src := ctx.Text.String()
	keep := len(row.cells)
	if cols < oldCols {
		keep = min(keep, cols)
	} else if cols > oldCols {
		keep = min(keep, oldCols)
	}
	want := keep
	if cols > oldCols {
		want = cols
	}
	if target >= want {
		want = target + 1
	}
	if keep == len(row.cells) && want == keep {
		at := -1
		if target >= 0 {
			at = row.cells[target].content.Start - row.line.start
		}
		return src[row.line.start:row.line.end], at
	}

	var out strings.Builder
	out.WriteString(src[row.line.start:row.cells[keep-1].end])
	at := -1
	if target >= 0 && target < keep {
		at = row.cells[target].content.Start - row.line.start
	}
	for i := keep; i < want; i++ {
		out.WriteByte('|')
		if i == target {
			at = out.Len() + 1
		}
		if delimiter {
			out.WriteString(" --- ")
		} else {
			out.WriteString("  ")
		}
	}
	// Preserve the original trailing pipe and any bytes after it. A one-column
	// header needs a pipe, as does a newly appended final empty cell.
	if !row.trailing && (!row.leading && want == 1 || want > keep && !delimiter) {
		out.WriteByte('|')
	}
	out.WriteString(src[row.cells[len(row.cells)-1].end:row.line.end])
	return out.String(), at
}

func resizeTable(ctx contract.EditContext, t sourceTable, rows, cols int) contract.Transaction {
	src := ctx.Text.String()
	oldCols := len(t.rows[0].cells)
	targetRow, targetCol := min(t.row, rows), min(t.col, cols-1)
	removed := targetRow != t.row || targetCol != t.col
	start := t.rows[0].line.start
	end := t.delimiter.line.end
	if len(t.rows) > 1 {
		end = t.rows[len(t.rows)-1].line.end
	}
	var out strings.Builder
	var selection contract.Selection
	writeRow := func(row tableLine, rowIndex int, delimiter bool) {
		target := -1
		if rowIndex == targetRow && !delimiter {
			target = targetCol
		}
		line, at := resizeTableLine(ctx, row, oldCols, cols, target, delimiter)
		if target >= 0 {
			base := start + out.Len()
			if removed {
				selection = contract.Caret(base + at)
			} else {
				r := ctx.Selection.Primary()
				selection = contract.Selection{Ranges: []contract.SelRange{{
					Anchor: base + r.Anchor - row.line.start,
					Head:   base + r.Head - row.line.start,
				}}}
			}
		}
		out.WriteString(line)
	}
	writeRow(t.rows[0], 0, false)
	out.WriteString(src[t.rows[0].line.end:t.delimiter.line.start])
	writeRow(t.delimiter, -1, true)
	previous := t.delimiter
	for i := 1; i < len(t.rows) && i <= rows; i++ {
		row := t.rows[i]
		out.WriteString(src[previous.line.end:row.line.start])
		writeRow(row, i, false)
		previous = row
	}
	prefix := src[previous.line.start:previous.start]
	for i := len(t.rows) - 1; i < rows; i++ {
		out.WriteString(eol(ctx))
		out.WriteString(prefix)
		out.WriteString(emptyTableRow(t, cols))
	}
	return transaction([]contract.TextEdit{edit(start, end, out.String())}, selection, contract.OriginCommand, "table")
}

func deleteTable(ctx contract.EditContext, t sourceTable, origin contract.Origin) contract.Transaction {
	end := t.delimiter.line.end
	if len(t.rows) > 1 {
		end = t.rows[len(t.rows)-1].line.end
	}
	start := t.rows[0].start
	return transaction([]contract.TextEdit{edit(start, end, "")}, contract.Caret(start), origin, "table")
}

func backspaceEmptyTableCell(ctx contract.EditContext, t sourceTable) contract.Transaction {
	row, col := t.row, t.col-1
	if col < 0 {
		row, col = row-1, len(t.rows[0].cells)-1
	}
	if row < 0 {
		for _, line := range t.rows {
			for _, cell := range line.cells {
				if !cell.content.Empty() {
					return transaction(nil, ctx.Selection, contract.OriginUser, "table")
				}
			}
		}
		return deleteTable(ctx, t, contract.OriginUser)
	}
	if col >= len(t.rows[row].cells) {
		tx := materializeCell(ctx, t, row, col)
		tx.Origin = contract.OriginUser
		return tx
	}
	return transaction(nil, contract.Caret(t.rows[row].cells[col].content.End), contract.OriginUser, "table")
}
