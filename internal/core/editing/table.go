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
	if ctx.AST.Len() > 64 && !canCrossTableBoundary(ctx, edits) {
		ok, decided := preservesTablesLocal(ctx, edits)
		if decided && ok {
			return true
		}
	}
	return preservesTablesFull(ctx, edits)
}

func canCrossTableBoundary(ctx contract.EditContext, edits []contract.TextEdit) bool {
	if edits[0].Range.Start < 512 {
		return true
	}
	if fm, ok := ctx.AST.FrontMatter(); ok && edits[0].Range.Start <= fm.Range.End+4 {
		return true
	}
	hasConstruct := func(s string) bool {
		return strings.ContainsAny(s, "`~$<") || strings.Contains(s, "-->") ||
			strings.Contains(s, "?>") || strings.Contains(s, "]]>") || strings.Contains(s, "]:")
	}
	for _, e := range edits {
		if hasConstruct(e.Text) {
			return true
		}
		a := ctx.Text.LineStart(ctx.Text.LineOf(e.Range.Start))
		b := ctx.Text.LineStart(ctx.Text.LineOf(e.Range.End) + 1)
		if hasConstruct(ctx.Text.Slice(contract.Range{Start: a, End: b})) {
			return true
		}
	}
	return false
}

func preservesTablesLocal(ctx contract.EditContext, edits []contract.TextEdit) (ok, decided bool) {
	topAt := func(off int) int {
		p := ctx.AST.Path(max(0, min(off, ctx.Text.Len())))
		if len(p) < 2 {
			return 1
		}
		return p[1]
	}
	leadOf := func(top int) int {
		if top <= 1 {
			return 0
		}
		l := ctx.AST.LeafAt(ctx.AST.Block(top).Range.Start)
		return max(0, ctx.AST.Block(l).Lead.Start)
	}
	loTop := topAt(edits[0].Range.Start)
	for range 3 {
		at := leadOf(loTop)
		if at == 0 {
			loTop = 1
			break
		}
		loTop = topAt(at - 1)
	}
	hiTop := topAt(edits[len(edits)-1].Range.End)
	for range 4 {
		if hiTop >= ctx.AST.Len() {
			break
		}
		hiTop = ctx.AST.Block(hiTop).Next
	}
	var tables []int
	for i := loTop; i < hiTop && i < ctx.AST.Len(); i++ {
		if ctx.AST.Block(i).Kind == contract.BlockTable {
			tables = append(tables, i)
		}
	}
	if len(tables) == 0 {
		return true, true
	}
	winLo := leadOf(loTop)
	winHi := ctx.Text.Len()
	if hiTop < ctx.AST.Len() {
		winHi = leadOf(hiTop)
	}
	if winHi-winLo > 32<<10 || edits[0].Range.Start < winLo || edits[len(edits)-1].Range.End > winHi {
		return false, false
	}
	var out strings.Builder
	at := winLo
	var localEdits []contract.TextEdit
	for _, e := range edits {
		out.WriteString(ctx.Text.Slice(contract.Range{Start: at, End: e.Range.Start}))
		out.WriteString(e.Text)
		at = e.Range.End
		localEdits = append(localEdits, contract.TextEdit{
			Range: contract.Range{Start: e.Range.Start - winLo, End: e.Range.End - winLo},
			Text:  e.Text,
		})
	}
	out.WriteString(ctx.Text.Slice(contract.Range{Start: at, End: winHi}))
	exts := ctx.Prefs.Exts
	if winLo > 0 {
		exts &^= contract.ExtFrontMatter
	}
	p, err := markdown.New(out.String(), 1, exts)
	if err != nil {
		return false, false
	}
	next := ctx
	next.AST = p.Snapshot()
	for _, i := range tables {
		tb := ctx.AST.Block(i)
		if coveredTable(tb.Range, edits) {
			continue
		}
		mappedAt := mapped(contract.Caret(tb.Range.Start-winLo), localEdits).Primary().Head
		b := ancestor(next, mappedAt, contract.BlockTable)
		if b == nil {
			return false, true
		}
		j, _ := next.AST.Index(b.ID)
		if tableRows(ctx.AST, i) != tableRows(next.AST, j) {
			return false, true
		}
	}
	return true, true
}

func preservesTablesFull(ctx contract.EditContext, edits []contract.TextEdit) bool {
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
