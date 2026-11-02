package editing

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"aurink/internal/contract"
)

type regression struct {
	name, before, after string
	in                  contract.Intent
	cmd                 contract.EditCommand
	unhandled, noEdits  bool
}

func checkRegressions(t *testing.T, cases []regression) {
	t.Helper()
	for _, c := range cases {
		for _, nl := range []string{"\n", "\r\n"} {
			t.Run(fmt.Sprintf("%s/%q", c.name, nl), func(t *testing.T) {
				src, sel := marked(strings.ReplaceAll(c.before, "\n", nl))
				ctx := contextFor(t, src, sel, nl)
				var tx contract.Transaction
				var ok bool
				var err error
				if c.in.Kind != 0 {
					tx, ok = New().HandleIntent(ctx, c.in)
				} else {
					tx, ok, err = New().Run(ctx, c.cmd)
				}
				if err != nil || ok == c.unhandled {
					t.Fatalf("handled=%v err=%v; tx=%+v", ok, err, tx)
				}
				if (c.noEdits || c.unhandled) && len(tx.Edits) != 0 {
					t.Fatalf("expected no edits: %+v", tx)
				}
				out, selection := apply(t, ctx, tx)
				want := strings.ReplaceAll(c.after, "\n", nl)
				if got := show(out, selection); got != want {
					t.Fatalf("%q => %q, want %q; edits %+v", c.before, got, want, tx.Edits)
				}
			})
		}
	}
}

func TestOwnerIntentGoldens(t *testing.T) {
	enter := contract.Intent{Kind: contract.IntentNewline}
	indent := contract.Intent{Kind: contract.IntentIndent}
	insert := func(s string) contract.Intent { return contract.Intent{Kind: contract.IntentInsertText, Text: s} }
	checkRegressions(t, []regression{
		{name: "table Enter", before: "|a|b|\n|-|-|\n|c¦|d|\n", after: "|a|b|\n|-|-|\n|c|d|\n| ¦ |  |\n", in: enter},
		{name: "table hard break", before: "|a|b|\n|-|-|\n|c¦|d|\n", after: "|a|b|\n|-|-|\n|c<br>¦|d|\n", in: contract.Intent{Kind: contract.IntentLineBreak}},
		{name: "HTML Enter", before: "<div>¦\n</div>\n", after: "<div>\n¦\n</div>\n", in: enter},
		{name: "front matter Enter", before: "---\nt: 1¦\n---\n", after: "---\nt: 1\n¦\n---\n", in: enter},
		{name: "soft break before", before: "a\n¦b", after: "a\n\n¦b", in: enter},
		{name: "soft break after", before: "a¦\nb", after: "a\n\n¦b", in: enter},
		{name: "blank gap", before: "a¦\n\nb", after: "a\n\n¦\n\nb", in: enter},
		{name: "pair before letter", before: "(¦abc", after: "((¦abc", in: insert("(")},
		{name: "bracket before letter", before: "a¦b", after: "a[¦b", in: insert("[")},
		{name: "pair before space", before: "a¦ b", after: "a(¦) b", in: insert("(")},
		{name: "first item indent", before: "- a¦", after: "- a¦", in: indent, noEdits: true},
		{name: "CJK word", before: "中文 词语¦", after: "中文 ¦", in: contract.Intent{Kind: contract.IntentDeleteBackward, Gran: contract.GranWord}},
	})
}

func TestOwnerCommandGoldens(t *testing.T) {
	strong := contract.EditCommand{ID: contract.CmdFormatStrong}
	up := contract.EditCommand{ID: contract.CmdBlockMoveUp}
	down := contract.EditCommand{ID: contract.CmdBlockMoveDown}
	cases := []regression{
		{name: "strong blocks", before: "¦a\n\nb¦", after: "**¦a**\n\n**b¦**", cmd: strong},
		{name: "strong soft break", before: "¦a\nb¦", after: "**¦a\nb¦**", cmd: strong},
		{name: "strong blocks off", before: "¦**a**\n\n**b**¦", after: "¦a\n\nb¦", cmd: strong},
		{name: "heading gap", before: "¦a\n\nb¦", after: "## ¦a\n\n## b¦", cmd: contract.EditCommand{ID: contract.CmdBlockHeading, Level: 2}},
		{name: "move item down", before: "- a¦\n- b\n", after: "- b\n- a¦\n", cmd: down},
		{name: "move item up", before: "- a\n- b¦\n", after: "- b¦\n- a\n", cmd: up},
		{name: "move ordered down", before: "1. a¦\n2. b\n", after: "2. b\n1. a¦\n", cmd: down},
		{name: "last sibling", before: "- a\n- b¦\n", after: "- a\n- b¦\n", cmd: down, unhandled: true},
		{name: "move no final EOL", before: "- a\n- b¦", after: "- b¦\n- a", cmd: up},
	}
	for _, f := range []struct {
		id   contract.CommandID
		mark string
	}{
		{contract.CmdFormatStrong, "**"}, {contract.CmdFormatEmphasis, "*"}, {contract.CmdFormatStrike, "~~"}, {contract.CmdFormatCode, "`"},
	} {
		cases = append(cases,
			regression{name: string(f.id) + " whitespace", before: "¦ a ¦", after: " " + f.mark + "¦a¦" + f.mark + " ", cmd: contract.EditCommand{ID: f.id}},
			regression{name: string(f.id) + " only whitespace", before: "¦ \t ¦", after: "¦ \t ¦", cmd: contract.EditCommand{ID: f.id}, noEdits: true})
	}
	for _, id := range []contract.CommandID{contract.CmdFormatStrong, contract.CmdFormatEmphasis, contract.CmdFormatStrike, contract.CmdFormatCode, contract.CmdFormatUnderline, contract.CmdFormatLink} {
		cases = append(cases, regression{name: string(id) + " in code", before: "```\n¦a¦\n```", after: "```\n¦a¦\n```", cmd: contract.EditCommand{ID: id}, unhandled: true})
	}
	for _, b := range []struct {
		id            contract.CommandID
		first, second string
	}{
		{contract.CmdBlockBulletList, "- ", "- "}, {contract.CmdBlockOrderedList, "1. ", "2. "},
		{contract.CmdBlockTaskList, "- [ ] ", "- [ ] "}, {contract.CmdBlockQuote, "> ", "> "},
	} {
		cases = append(cases, regression{name: string(b.id) + " gap", before: "¦a\n\nb¦", after: b.first + "¦a\n\n" + b.second + "b¦", cmd: contract.EditCommand{ID: b.id}})
	}
	checkRegressions(t, cases)
}

func TestOrderedIndentAST(t *testing.T) {
	for _, c := range []struct {
		name, before, after string
		nestedItems, items  int
	}{
		{"ordered", "1. a\n2. b¦", "1. a\n   1. b¦", 1, 2},
		{"wide ordered", "10. a\n11. b¦", "10. a\n    1. b¦", 1, 2},
		{"parenthesis", "1) a\n2) b¦", "1) a\n   1) b¦", 1, 2},
		{"already one", "1. a\n1. b¦", "1. a\n   1. b¦", 1, 2},
		{"blank line", "1. a\n\n2. b¦", "1. a\n\n   2. b¦", 1, 2},
		{"existing nested list", "1. a\n   1. x\n2. b¦", "1. a\n   1. x\n   2. b¦", 2, 3},
		{"task bullets", "- [ ] a\n- [ ] b¦", "- [ ] a\n  - [ ] b¦", 1, 2},
		{"ordered task", "10. [x] a\n11. [ ] b¦", "10. [x] a\n    1. [ ] b¦", 1, 2},
		{"children", "10. a\n11. b¦\n    - child", "10. a\n    1. b¦\n        - child", 1, 3},
		{"later sibling", "1. a\n2. b¦\n9. c", "1. a\n   1. b¦\n9. c", 1, 3},
	} {
		for _, nl := range []string{"\n", "\r\n"} {
			for _, command := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%q/command=%v", c.name, nl, command), func(t *testing.T) {
					src, sel := marked(strings.ReplaceAll(c.before, "\n", nl))
					ctx := contextFor(t, src, sel, nl)
					var tx contract.Transaction
					var ok bool
					if command {
						var err error
						tx, ok, err = New().Run(ctx, contract.EditCommand{ID: contract.CmdBlockIndent})
						if err != nil {
							t.Fatal(err)
						}
					} else {
						tx, ok = New().HandleIntent(ctx, contract.Intent{Kind: contract.IntentIndent})
					}
					if !ok {
						t.Fatal("not handled")
					}
					nested := verifyTransaction(t, ctx, tx)
					if got, want := show(nested.Text.String(), nested.Selection), strings.ReplaceAll(c.after, "\n", nl); got != want {
						t.Fatalf("got %q, want %q; edits %+v", got, want, tx.Edits)
					}
					item := ancestor(nested, nested.Selection.Primary().Head, contract.BlockItem)
					if item == nil {
						t.Fatal("no nested item")
					}
					list := nested.AST.Block(item.Parent)
					if list.Kind != contract.BlockList || nested.AST.Block(list.Parent).Kind != contract.BlockItem {
						t.Fatalf("indent did not produce nested list: %+v", list)
					}
					nestedItems, items := 0, 0
					for i := 0; i < nested.AST.Len(); i++ {
						b := nested.AST.Block(i)
						if b.Kind == contract.BlockItem {
							items++
							if b.Parent == item.Parent {
								nestedItems++
							}
						}
					}
					if nestedItems != c.nestedItems || items != c.items {
						t.Fatalf("nested items=%d, total items=%d; want %d, %d", nestedItems, items, c.nestedItems, c.items)
					}
				})
			}
		}
	}
}

func TestOrderedOutdent(t *testing.T) {
	cases := []regression{
		{name: "ordered", before: "1. a\n   1. b¦", after: "1. a\n2. b¦"},
		{name: "wide ordered", before: "10. a\n    1. b¦", after: "10. a\n11. b¦"},
		{name: "nested sibling", before: "1. a\n   1. x\n   2. y¦", after: "1. a\n   1. x\n2. y¦"},
		{name: "bullet parent", before: "- a\n  1. b¦", after: "- a\n1. b¦"},
		{name: "parenthesis", before: "10) a\n    1) b¦", after: "10) a\n11) b¦"},
		{name: "different delimiter", before: "10) a\n    1. b¦", after: "10) a\n1. b¦"},
		{name: "bullet child", before: "10. a\n    - b¦", after: "10. a\n- b¦"},
		{name: "later sibling", before: "10. a\n    1. b¦\n20. c", after: "10. a\n11. b¦\n20. c"},
		{name: "ordered task", before: "10. [x] a\n    1. [ ] b¦", after: "10. [x] a\n11. [ ] b¦"},
		{name: "children", before: "10. a\n    1. b¦\n        - child", after: "10. a\n11. b¦\n    - child"},
	}
	for i := range cases {
		cases[i].in = contract.Intent{Kind: contract.IntentOutdent}
	}
	checkRegressions(t, cases)
	for i := range cases {
		cases[i].in = contract.Intent{}
		cases[i].cmd = contract.EditCommand{ID: contract.CmdBlockOutdent}
	}
	checkRegressions(t, cases)
}

func TestOrderedIndentMinimalEdits(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		for _, c := range []struct {
			before, after string
			out           bool
			edits         []contract.TextEdit
		}{
			{"1. a\n2. b¦eta", "1. a\n   1. b¦eta", false, []contract.TextEdit{edit(4+len(nl), 5+len(nl), "   1")}},
			{"10. a\n11. ¦beta¦", "10. a\n    1. ¦beta¦", false, []contract.TextEdit{edit(5+len(nl), 7+len(nl), "    1")}},
			{"1. a\n   1. b¦eta", "1. a\n2. b¦eta", true, []contract.TextEdit{edit(4+len(nl), 7+len(nl), ""), edit(7+len(nl), 8+len(nl), "2")}},
			{"10. a\n    1. ¦beta¦", "10. a\n11. ¦beta¦", true, []contract.TextEdit{edit(5+len(nl), 9+len(nl), ""), edit(9+len(nl), 10+len(nl), "11")}},
		} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%q/reverse=%v", c.before, nl, reverse), func(t *testing.T) {
					src, sel := marked(strings.ReplaceAll(c.before, "\n", nl))
					wantText, wantSel := marked(strings.ReplaceAll(c.after, "\n", nl))
					if reverse {
						sel.Ranges[0].Anchor, sel.Ranges[0].Head = sel.Ranges[0].Head, sel.Ranges[0].Anchor
						wantSel.Ranges[0].Anchor, wantSel.Ranges[0].Head = wantSel.Ranges[0].Head, wantSel.Ranges[0].Anchor
					}
					ctx := contextFor(t, src, sel, nl)
					kind := contract.IntentIndent
					if c.out {
						kind = contract.IntentOutdent
					}
					tx, ok := New().HandleIntent(ctx, contract.Intent{Kind: kind})
					if !ok || !reflect.DeepEqual(tx.Edits, c.edits) {
						t.Fatalf("handled=%v edits=%+v, want %+v", ok, tx.Edits, c.edits)
					}
					next := verifyTransaction(t, ctx, tx)
					if next.Text.String() != wantText || !reflect.DeepEqual(next.Selection, wantSel) {
						t.Fatalf("got %q %+v, want %q %+v", next.Text.String(), next.Selection, wantText, wantSel)
					}
				})
			}
		}
	}
}

func TestOrderedIndentParserCounterexample(t *testing.T) {
	// CommonMark permits only 1 as an ordered marker interrupting a paragraph.
	for _, src := range []string{"1. a\n   2. b", "10. a\n    11. b"} {
		for _, nl := range []string{"\n", "\r\n"} {
			ctx := contextFor(t, strings.ReplaceAll(src, "\n", nl), contract.Caret(0), nl)
			lists, items := 0, 0
			for i := 0; i < ctx.AST.Len(); i++ {
				switch ctx.AST.Block(i).Kind {
				case contract.BlockList:
					lists++
				case contract.BlockItem:
					items++
				}
			}
			if lists != 1 || items != 1 {
				t.Fatalf("parser behavior changed: lists=%d items=%d", lists, items)
			}
			t.Logf("%q parses as one list, one item, with paragraph continuation", ctx.Text.String())
		}
	}
}

func TestMinimalItemMove(t *testing.T) {
	ctx := contextFor(t, "1. a\n2. b\n", contract.Caret(4), "\n")
	tx, ok, err := New().Run(ctx, contract.EditCommand{ID: contract.CmdBlockMoveDown})
	if !ok || err != nil {
		t.Fatal(ok, err)
	}
	want := []contract.TextEdit{edit(0, 4, "2. b"), edit(5, 9, "1. a")}
	if !reflect.DeepEqual(tx.Edits, want) {
		t.Fatalf("edits %+v, want %+v", tx.Edits, want)
	}
}
