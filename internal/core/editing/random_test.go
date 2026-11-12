package editing

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/internal/core/markdown"
	"aurink/internal/core/text"
	"aurink/tests/corpus"
)

func runeOffset(s string, off int) int {
	for off > 0 && off < len(s) && !utf8.RuneStart(s[off]) {
		off--
	}
	if off > 0 && off < len(s) && s[off-1] == '\r' && s[off] == '\n' {
		off--
	}
	return off
}

func verifyTransaction(t *testing.T, ctx contract.EditContext, tx contract.Transaction) contract.EditContext {
	t.Helper()
	src, sel := apply(t, ctx, tx)
	next, changes, err := ctx.Text.(*text.Text).Apply(tx.Edits)
	if err != nil || next.String() != src {
		t.Fatalf("text.Apply differs from string model: %v", err)
	}
	if !utf8.ValidString(src) {
		t.Fatalf("transaction split UTF-8: %+v", tx)
	}
	// The inverse uses new coordinates and must restore every original byte.
	inverse := make([]contract.TextEdit, len(changes))
	for i, c := range changes {
		inverse[i] = edit(c.NewStart, c.NewEnd, ctx.Text.Slice(contract.Range{Start: c.OldStart, End: c.OldEnd}))
	}
	restored, _, err := next.Apply(inverse)
	if err != nil || restored.String() != ctx.Text.String() {
		t.Fatalf("inverse failed: %v", err)
	}
	p, err := markdown.New(src, next.Version(), ctx.Prefs.Exts)
	if err != nil {
		t.Fatal(err)
	}
	ast := p.Snapshot()
	if ast.Block(0).Range.End != len(src) || ast.Version() != next.Version() {
		t.Fatal("invalid parsed root or version")
	}
	end := 0
	for i := range ast.Leaves() {
		b := ast.Block(i)
		if b.Lead.Start != end || b.Lead.End != b.Range.Start || b.Range.End != b.Trailer.Start || b.Trailer.End > len(src) {
			t.Fatalf("unparsable coverage at %+v", b)
		}
		end = b.Trailer.End
	}
	if end != len(src) {
		t.Fatal("parsed leaves do not cover the document")
	}
	ctx.Text, ctx.AST, ctx.Selection = next, ast, sel
	return ctx
}

func TestRandomTransactions(t *testing.T) {
	entries, err := corpus.Index()
	if err != nil {
		t.Fatal(err)
	}
	commands := []contract.CommandID{
		contract.CmdFormatStrong, contract.CmdFormatEmphasis, contract.CmdFormatStrike, contract.CmdFormatCode,
		contract.CmdFormatUnderline, contract.CmdFormatLink, contract.CmdFormatClear,
		contract.CmdBlockHeading1, contract.CmdBlockHeading6, contract.CmdBlockParagraph, contract.CmdBlockQuote,
		contract.CmdBlockBulletList, contract.CmdBlockOrderedList, contract.CmdBlockTaskList, contract.CmdBlockToggleTask,
		contract.CmdBlockIndent, contract.CmdBlockOutdent, contract.CmdBlockCode, contract.CmdBlockHR,
		contract.CmdBlockMoveUp, contract.CmdBlockMoveDown,
		contract.CmdTableNextCell, contract.CmdTableDeleteRow,
	}
	intents := []contract.Intent{
		{Kind: contract.IntentInsertText, Text: "文"}, {Kind: contract.IntentInsertText, Text: "("},
		{Kind: contract.IntentInsertText, Text: "*"}, {Kind: contract.IntentNewline}, {Kind: contract.IntentLineBreak},
		{Kind: contract.IntentInsertText, Text: "`"}, {Kind: contract.IntentInsertText, Text: ")"},
		{Kind: contract.IntentDeleteBackward}, {Kind: contract.IntentDeleteForward},
		{Kind: contract.IntentDeleteBackward, Gran: contract.GranWord}, {Kind: contract.IntentDeleteForward, Gran: contract.GranLine},
		{Kind: contract.IntentDeleteForward, Gran: contract.GranWord}, {Kind: contract.IntentDeleteBackward, Gran: contract.GranLine},
		{Kind: contract.IntentIndent}, {Kind: contract.IntentOutdent},
		{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Markdown: "**中**\n- 文"}},
	}
	steps := 6
	if os.Getenv("AURINK_LONG_TESTS") != "" {
		steps = 120
	}
	count, roundTrips, tableChecks := 0, 0, 0
	for _, seed := range []int64{1331, 42, 982451653} {
		rng := rand.New(rand.NewSource(seed))
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Path, ".md") {
				continue
			}
			t.Run(fmt.Sprintf("%d/%s", seed, entry.Path), func(t *testing.T) {
				data, err := corpus.Read(entry)
				if err != nil {
					t.Fatal(err)
				}
				ctx := contextFor(t, string(data), contract.Caret(0), "\n")
				if strings.Count(string(data), "\r\n") > strings.Count(string(data), "\n")/2 {
					ctx.Meta.Dominant = "\r\n"
				}
				original := ctx
				for step := 0; step < steps; step++ {
					probe := original
					a0 := runeOffset(string(data), rng.Intn(len(data)+1))
					b0 := a0
					if rng.Intn(4) == 0 {
						b0 = runeOffset(string(data), min(len(data), a0+rng.Intn(17)))
					}
					probe.Selection = contract.Selection{Ranges: []contract.SelRange{{Anchor: a0, Head: b0}}}
					if checkIndentRoundTrip(t, probe) {
						roundTrips++
					}
					s := ctx.Text.String()
					a := runeOffset(s, rng.Intn(len(s)+1))
					b := a
					if rng.Intn(4) == 0 {
						b = runeOffset(s, min(len(s), a+rng.Intn(17)))
					}
					ctx.Selection = contract.Selection{Ranges: []contract.SelRange{{Anchor: a, Head: b}}}
					var tx contract.Transaction
					var ok bool
					var intent contract.Intent
					var command contract.EditCommand
					if rng.Intn(2) == 0 {
						intent = intents[rng.Intn(len(intents))]
						tx, ok = New().HandleIntent(ctx, intent)
					} else {
						command = contract.EditCommand{ID: commands[rng.Intn(len(commands))], Text: "go", URL: "https://example.com/a"}
						tx, ok, err = New().Run(ctx, command)
						if err != nil {
							t.Fatal(err)
						}
					}
					if ok {
						next := verifyTransaction(t, ctx, tx)
						row := ancestor(ctx, a, contract.BlockTableRow)
						plainTyping := intent.Kind == contract.IntentInsertText && row != nil && b <= row.Range.End && !strings.ContainsAny(intent.Text, "\r\n")
						if !plainTyping && command.ID != contract.CmdBlockMoveUp && command.ID != contract.CmdBlockMoveDown {
							tableChecks += checkTableRows(t, ctx, next, tx, intent, command)
						}
						ctx = next
						count++
						if follow, ok := New().FollowUp(ctx); ok {
							next := verifyTransaction(t, ctx, follow)
							if next.Text.String() == ctx.Text.String() {
								t.Fatalf("identity FollowUp: %+v", follow)
							}
							ctx = next
							count++
						}
					}
					New().Formatting(ctx)
				}
			})
		}
	}
	if count < 500 {
		t.Fatalf("only %d transactions exercised", count)
	}
	if roundTrips < 100 || tableChecks < 100 {
		t.Fatalf("insufficient invariant checks: round trips=%d tables=%d", roundTrips, tableChecks)
	}
	t.Logf("validated %d transactions, %d Indent/Outdent round trips and %d table row counts across corpus documents and three seeds", count, roundTrips, tableChecks)
}

func checkIndentRoundTrip(t *testing.T, ctx contract.EditContext) bool {
	t.Helper()
	if table, inTable := findTable(ctx); inTable {
		indent, ok := New().HandleIntent(ctx, contract.Intent{Kind: contract.IntentIndent})
		if !ok {
			return false
		}
		next := verifyTransaction(t, ctx, indent)
		checkTableRows(t, ctx, next, indent, contract.Intent{Kind: contract.IntentIndent}, contract.EditCommand{})
		// Tab at the final cell adds a row; Shift+Tab navigates but does not undo it.
		if table.row == len(table.rows)-1 && table.col == len(table.rows[0].cells)-1 || len(indent.Edits) != 0 {
			return true
		}
		outdent, ok := New().HandleIntent(next, contract.Intent{Kind: contract.IntentOutdent})
		if !ok || verifyTransaction(t, next, outdent).Text.String() != ctx.Text.String() {
			t.Fatal("table Tab/Shift+Tab changed text")
		}
		return true
	}
	indent, ok := New().HandleIntent(ctx, contract.Intent{Kind: contract.IntentIndent})
	if !ok {
		return false
	}
	nested := verifyTransaction(t, ctx, indent)
	// A literal Tab inserts text; Shift+Tab is a structural outdent and is
	// no longer its inverse. verifyTransaction already checked the edit's inverse.
	if len(indent.Edits) == 1 && indent.Edits[0].Range.Empty() && indent.Edits[0].Text == "\t" {
		return true
	}
	if item := ancestor(nested, nested.Selection.Primary().Head, contract.BlockItem); item != nil {
		list := nested.AST.Block(item.Parent)
		if nested.AST.Block(list.Parent).Kind != contract.BlockItem {
			// Shift+Tab on a top-level item now turns it into a paragraph.
			return true
		}
	}
	outdent, ok := New().HandleIntent(nested, contract.Intent{Kind: contract.IntentOutdent})
	if !ok {
		return false
	}
	restored := verifyTransaction(t, nested, outdent)
	before, after := ctx.Text.String(), restored.Text.String()
	r := ctx.Selection.Primary().Range()
	if item := ancestor(ctx, r.Start, contract.BlockItem); item != nil && !rawAt(ctx, r.Start) && ctx.AST.Block(item.Parent).Ordered {
		first := lineAt(ctx, item.Range.Start)
		if r.Empty() || lineAt(ctx, r.Start).start == first.start {
			before = withoutOrderedNumber(ctx, first.start)
			after = withoutOrderedNumber(restored, first.start)
		}
	}
	if after != before {
		at := ctx.Selection.Primary().Range().Start
		a, b := max(0, at-50), min(ctx.Text.Len(), at+100)
		t.Fatalf("Indent/Outdent changed bytes near %q; selection %+v; indent %+v; outdent %+v", ctx.Text.String()[a:b], ctx.Selection, indent.Edits, outdent.Edits)
	}
	return true
}

func withoutOrderedNumber(ctx contract.EditContext, at int) string {
	l := lineAt(ctx, at)
	if l.list < 0 || l.delim != '.' && l.delim != ')' {
		return ctx.Text.String()
	}
	a := l.parts[l.list].a
	b := a
	for b < len(l.s) && l.s[b] >= '0' && l.s[b] <= '9' {
		b++
	}
	s := ctx.Text.String()
	return s[:l.start+a] + s[l.start+b:]
}

func checkTableRows(t *testing.T, before, after contract.EditContext, tx contract.Transaction, intent contract.Intent, command contract.EditCommand) int {
	t.Helper()
	active, inTable := findTable(before)
	checked := 0
	for i := 0; i < before.AST.Len(); i++ {
		b := before.AST.Block(i)
		if b.Kind != contract.BlockTable {
			continue
		}
		rows := 0
		for j := range before.AST.Children(i) {
			if before.AST.Block(j).Kind == contract.BlockTableRow {
				rows++
			}
		}
		at := mapped(contract.Caret(b.Range.Start), tx.Edits).Primary().Head
		table := ancestor(after, at, contract.BlockTable)
		if table == nil {
			a, z := max(0, b.Range.Start-70), min(before.Text.Len(), b.Range.End+30)
			t.Fatalf("intent destroyed a table near %q: at %d, intent %+v command %+v selection %+v edits %+v", before.Text.String()[a:z], at, intent, command, before.Selection, tx.Edits)
		}
		index, _ := after.AST.Index(table.ID)
		nextRows := 0
		for j := range after.AST.Children(index) {
			if after.AST.Block(j).Kind == contract.BlockTableRow {
				nextRows++
			}
		}
		delta := 0
		if inTable && active.block.ID == b.ID {
			last := active.row == len(active.rows)-1 && active.col == len(active.rows[0].cells)-1
			if last && (intent.Kind == contract.IntentIndent || command.ID == contract.CmdTableNextCell) {
				delta = 1
			} else if command.ID == contract.CmdTableDeleteRow && active.row > 0 {
				delta = -1
			}
		}
		if rows+delta != nextRows {
			t.Fatalf("table rows %d => %d, want change %+d, intent %+v command %+v edits %+v", rows, nextRows, delta, intent, command, tx.Edits)
		}
		checked++
	}
	return checked
}
