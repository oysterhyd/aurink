package editing

import (
	"strings"

	"aurink/internal/contract"
	"aurink/internal/core/markdown"
)

func tableRows(ast contract.AST, index int) int {
	rows := 0
	for i := range ast.Children(index) {
		if ast.Block(i).Kind == contract.BlockTableRow {
			rows++
		}
	}
	return rows
}

// coveredTable reports whether one edit replaces all of a table: select all,
// or a selection around the table, then delete or type. Such an edit removes
// the table, which the table guards let through.
func coveredTable(table contract.Range, edits []contract.TextEdit) bool {
	for _, e := range edits {
		if e.Range.Start <= table.Start && table.End <= e.Range.End {
			return true
		}
	}
	return false
}

func preservesTables(ctx contract.EditContext, edits []contract.TextEdit) bool {
	if ctx.AST == nil || len(edits) == 0 {
		return true
	}
	var tables []int
	for i := 0; i < ctx.AST.Len(); i++ {
		b := ctx.AST.Block(i)
		if b.Kind == contract.BlockTable {
			tables = append(tables, i)
		}
	}
	if len(tables) == 0 {
		return true
	}
	var out strings.Builder
	src, at := ctx.Text.String(), 0
	for _, e := range edits {
		out.WriteString(src[at:e.Range.Start])
		out.WriteString(e.Text)
		at = e.Range.End
	}
	out.WriteString(src[at:])
	p, err := markdown.New(out.String(), ctx.Text.Version()+1, ctx.Prefs.Exts)
	if err != nil {
		return false
	}
	next := ctx
	next.AST = p.Snapshot()
	for _, i := range tables {
		if coveredTable(ctx.AST.Block(i).Range, edits) {
			continue // deleted whole, as the user chose
		}
		at := mapped(contract.Caret(ctx.AST.Block(i).Range.Start), edits).Primary().Head
		b := ancestor(next, at, contract.BlockTable)
		if b == nil {
			return false
		}
		j, _ := next.AST.Index(b.ID)
		if tableRows(ctx.AST, i) != tableRows(next.AST, j) {
			return false
		}
	}
	return true
}
