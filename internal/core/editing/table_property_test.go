package editing

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestRandomTableCommandsPreserveSurroundings(t *testing.T) {
	rng := rand.New(rand.NewSource(16001))
	ids := []contract.CommandID{contract.CmdTableRowAbove, contract.CmdTableRowBelow, contract.CmdTableDeleteRow,
		contract.CmdTableColLeft, contract.CmdTableColRight, contract.CmdTableDeleteCol, contract.CmdTableAlign,
		contract.CmdTableFormat, contract.CmdTableNextCell, contract.CmdTablePrevCell}
	values := []string{"a", "中文", `x\|y`, "longer", "é", "文x"}
	for iteration := 0; iteration < 500; iteration++ {
		nl := "\n"
		if rng.Intn(2) == 0 {
			nl = "\r\n"
		}
		cols, bodies := 2+rng.Intn(4), 1+rng.Intn(5)
		leading, trailing := rng.Intn(2) == 0, rng.Intn(2) == 0
		line := func(cells []string) string {
			s := strings.Join(cells, " | ")
			if leading {
				s = "| " + s
			}
			if trailing {
				s += " |"
			}
			return s
		}
		header, delimiter := make([]string, cols), make([]string, cols)
		for i := range header {
			header[i], delimiter[i] = values[rng.Intn(len(values))], "---"
		}
		lines := []string{line(header), line(delimiter)}
		for range bodies {
			cells := make([]string, 1+rng.Intn(cols))
			for i := range cells {
				cells[i] = values[rng.Intn(len(values))]
			}
			lines = append(lines, line(cells))
		}
		prefix, suffix := "BEFORE"+nl+nl, nl+nl+"AFTER"
		src := prefix + strings.Join(lines, nl) + suffix
		initial := contextFor(t, src, contract.Caret(len(prefix)+strings.Index(lines[0], header[0])), nl)
		table, ok := findTable(initial)
		if !ok {
			t.Fatalf("iteration %d: input is not a table: %q", iteration, src)
		}
		row := rng.Intn(len(table.rows))
		col := rng.Intn(min(cols, len(table.rows[row].cells)))
		initial.Selection = contract.Caret(table.rows[row].cells[col].content.Start)
		id := ids[rng.Intn(len(ids))]
		cmd := contract.EditCommand{ID: id, Align: contract.Align(rng.Intn(4))}
		tx, handled, err := New().Run(initial, cmd)
		if err != nil {
			t.Fatalf("iteration %d %s: %v", iteration, id, err)
		}
		if !handled {
			continue
		}
		out, _ := apply(t, initial, tx)
		if !strings.HasPrefix(out, prefix) || !strings.HasSuffix(out, suffix) {
			t.Fatalf("iteration %d %s changed surrounding bytes: %q => %q", iteration, id, src, out)
		}
		next := contextFor(t, out, contract.Caret(len(prefix)), nl)
		var updated *contract.Block
		updatedRows := 0
		for i := 0; i < next.AST.Len(); i++ {
			if next.AST.Block(i).Kind == contract.BlockTable {
				updated = next.AST.Block(i)
				updatedRows = tableRows(next.AST, i)
				break
			}
		}
		if updated == nil {
			t.Fatalf("iteration %d %s destroyed table: %q => %q", iteration, id, src, out)
		}
		wantRows, wantCols := bodies+1, cols
		switch id {
		case contract.CmdTableRowAbove, contract.CmdTableRowBelow:
			wantRows++
		case contract.CmdTableDeleteRow:
			wantRows--
		case contract.CmdTableColLeft, contract.CmdTableColRight:
			wantCols++
		case contract.CmdTableDeleteCol:
			wantCols--
		case contract.CmdTableNextCell:
			if row == len(table.rows)-1 && col == cols-1 {
				wantRows++
			}
		}
		if updatedRows != wantRows || len(updated.Align) != wantCols {
			t.Fatalf("iteration %d %s: got %d rows/%d cols, want %d/%d: %q => %q", iteration, id, updatedRows, len(updated.Align), wantRows, wantCols, src, out)
		}
	}
}

func BenchmarkTableCommand1000Rows(b *testing.B) {
	var s strings.Builder
	s.WriteString("| name | count |\n| --- | --- |\n")
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&s, "| row-%d | %d |\n", i, i)
	}
	src := s.String()
	ctx := contextFor(b, src, contract.Caret(strings.Index(src, "row-500")), "\n")
	ed := New()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		tx, ok, err := ed.Run(ctx, contract.EditCommand{ID: contract.CmdTableColRight})
		if !ok || err != nil {
			b.Fatalf("command failed: %v", err)
		}
		benchmarkTransaction = tx
	}
}

func TestTableCellPasteAndFormatEdges(t *testing.T) {
	for _, tc := range []struct {
		before, after string
		intent        contract.Intent
	}{
		{"a | b\n--- | ---\nx¦y | z", "a | b\n--- | ---\nx\\|¦y | z", contract.Intent{Kind: contract.IntentInsertText, Text: "|"}},
		{"a | b\n--- | ---\nx\\¦y | z", "a | b\n--- | ---\nx\\|¦y | z", contract.Intent{Kind: contract.IntentInsertText, Text: "|"}},
		{"| a | b |\n|---|---|\n| ¦label¦ | x |", "| a | b |\n|---|---|\n| [label](https://example.org)¦ | x |", contract.Intent{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: "https://example.org"}}},
	} {
		src, sel := marked(tc.before)
		ctx := contextFor(t, src, sel, "\n")
		tx, ok := New().HandleIntent(ctx, tc.intent)
		if !ok {
			t.Fatalf("intent not handled: %q", tc.before)
		}
		out, selection := apply(t, ctx, tx)
		if got := show(out, selection); got != tc.after {
			t.Errorf("got %q, want %q", got, tc.after)
		}
	}
	if got := displayWidth(`中\|x`); got != 4 {
		t.Errorf("display width = %d, want 4", got)
	}
	for _, tc := range []struct {
		src     string
		inTable bool
	}{
		{"outside¦\n\n| a | b |\n|---|---|\n| c | d |", false},
		{"outside\n\n| a | b |\n|---|---|\n| ¦c | d |", true},
	} {
		src, sel := marked(tc.src)
		if got := New().Formatting(contextFor(t, src, sel, "\n")).InTable; got != tc.inTable {
			t.Errorf("InTable on %q = %v", tc.src, got)
		}
	}
	src, sel := marked("| a | b |\n| --¦- | --- |\n| c | d |")
	if _, ok, err := New().Run(contextFor(t, src, sel, "\n"), contract.EditCommand{ID: contract.CmdFormatCode}); ok || err != nil {
		t.Fatalf("formatting must not turn a delimiter into text: ok=%v err=%v", ok, err)
	}
}
