package editing

import (
	"fmt"
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/document"
)

func TestTyporaNestedQuoteAndTableEnter(t *testing.T) {
	enter := contract.Intent{Kind: contract.IntentNewline}
	for _, tc := range []struct{ name, before, after string }{
		{"nested quote", "> > a¦b", "> > a\n> >\n> > ¦b"},
		{"leave inner quote", "> > a\n> >\n> > ¦", "> > a\n>\n> ¦"},
		{"list in quote", "> - a¦", "> - a\n> - ¦"},
		{"list continuation in quote", "> - a\n>   b¦", "> - a\n>   b\n> - ¦"},
		{"quote in list", "- a\n  > b¦", "- a\n  > b\n  >\n  > ¦"},
		{"three cells", "| a |b| c |¦", "| a | b | c |\n| --- | --- | --- |\n| ¦ |  |  |"},
		{"escaped pipe", "|a\\|b| c |¦", "| a\\|b | c |\n| --- | --- |\n| ¦ |  |"},
		{"quote table", "> |a|b|¦", "> | a | b |\n> | --- | --- |\n> | ¦ |  |"},
		{"one cell", "|a|¦", "|a|\n\n¦"},
		{"missing leading pipe", "a|b|¦", "a|b|\n\n¦"},
		{"missing trailing pipe", "|a|b¦", "|a|b\n\n¦"},
		{"leading spaces", "  |a|b|¦", "  |a|b|\n\n¦"},
		{"trailing spaces", "|a|b|  ¦", "|a|b|  \n\n¦"},
		{"escaped final pipe", "|a|b\\|¦", "|a|b\\|\n\n¦"},
		{"fence is raw", "```\n|a|b|¦\n```", "```\n|a|b|\n¦\n```"},
		{"math in quote", "> $$¦\n\nnext", "> $$\n> ¦\n> $$\n\nnext"},
	} {
		for _, nl := range []string{"\n", "\r\n"} {
			t.Run(fmt.Sprintf("%s/%q", tc.name, nl), func(t *testing.T) {
				src, sel := marked(strings.ReplaceAll(tc.before, "\n", nl))
				ctx := contextFor(t, src, sel, nl)
				tx, ok := New().HandleIntent(ctx, enter)
				if !ok {
					t.Fatal("not handled")
				}
				out, gotSel := apply(t, ctx, tx)
				if got, want := show(out, gotSel), strings.ReplaceAll(tc.after, "\n", nl); got != want {
					for i := 0; i < ctx.AST.Len(); i++ {
						t.Logf("block %d: %+v", i, ctx.AST.Block(i))
					}
					t.Fatalf("got %q, want %q", got, want)
				}
			})
		}
	}
}

func TestTyporaFenceFollowUpScope(t *testing.T) {
	for _, tc := range []struct{ name, before, after string }{
		{"four tildes", "~~~~¦\n\ntext", "~~~~¦\n~~~~\n\ntext"},
		{"inside quote", "> ```¦\n> text\n\noutside", "> ```¦\n> ```\n> text\n\noutside"},
		{"inside list", "- ~~~¦\n  text\n\noutside", "- ~~~¦\n  ~~~\n  text\n\noutside"},
		{"outside quote only", "> ```¦\n\noutside", ""},
		{"only blank text", "```¦\n\n ", ""},
		{"info string", "```go¦\n\ntext", ""},
		{"already closed", "```¦\nbody\n```\n\ntext", ""},
		{"not an opener", "x```¦\n\ntext", ""},
	} {
		for _, nl := range []string{"\n", "\r\n"} {
			t.Run(fmt.Sprintf("%s/%q", tc.name, nl), func(t *testing.T) {
				src, sel := marked(strings.ReplaceAll(tc.before, "\n", nl))
				ctx := contextFor(t, src, sel, nl)
				tx, ok := New().FollowUp(ctx)
				if tc.after == "" {
					if ok {
						t.Fatalf("unexpected follow-up: %+v", tx)
					}
					return
				}
				if !ok {
					for i := 0; i < ctx.AST.Len(); i++ {
						t.Logf("block %d: %+v", i, ctx.AST.Block(i))
					}
					t.Fatal("no follow-up")
				}
				out, gotSel := apply(t, ctx, tx)
				if got, want := show(out, gotSel), strings.ReplaceAll(tc.after, "\n", nl); got != want {
					t.Fatalf("got %q, want %q", got, want)
				}
			})
		}
	}
}

func TestTyporaFenceBackspacePreservesNonemptyOrValidFence(t *testing.T) {
	back := contract.Intent{Kind: contract.IntentDeleteBackward}
	for _, tc := range []struct{ name, before, after string }{
		{"empty body", "```¦\n\n```\n\ntext", "``¦\n\n\ntext"},
		{"four becomes three", "````¦\n````\n\ntext", "```¦\n````\n\ntext"},
		{"info remains", "```go¦\n```\n\ntext", "```g¦\n```\n\ntext"},
		{"nonempty body", "```¦\nbody\n```\n\ntext", "``¦\nbody\n```\n\ntext"},
		{"quote closer", "> ~~~¦\n> ~~~\n>\n> text", "> ~~¦\n>\n> text"},
	} {
		for _, nl := range []string{"\n", "\r\n"} {
			t.Run(fmt.Sprintf("%s/%q", tc.name, nl), func(t *testing.T) {
				src, sel := marked(strings.ReplaceAll(tc.before, "\n", nl))
				ctx := contextFor(t, src, sel, nl)
				tx, ok := New().HandleIntent(ctx, back)
				if !ok {
					t.Fatal("not handled")
				}
				out, gotSel := apply(t, ctx, tx)
				if got, want := show(out, gotSel), strings.ReplaceAll(tc.after, "\n", nl); got != want {
					t.Fatalf("got %q, want %q", got, want)
				}
			})
		}
	}
}

func TestTyporaKeysAreSingleUndoSteps(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
		in                  contract.Intent
	}{
		{"empty enter", "a\n\n¦\n\nb", "a\n\n\n\n¦\n\nb", contract.Intent{Kind: contract.IntentNewline}},
		{"empty backspace", "a\n\n¦\n\nb", "a¦\n\nb", contract.Intent{Kind: contract.IntentDeleteBackward}},
		{"empty delete", "a\n\n¦\n\nb", "a\n\n¦b", contract.Intent{Kind: contract.IntentDeleteForward}},
		{"table enter", "|a|b|¦", "| a | b |\n| --- | --- |\n| ¦ |  |", contract.Intent{Kind: contract.IntentNewline}},
		{"fence typing", "a\n\n``¦\n\nz", "a\n\n```¦\n```\n\nz", contract.Intent{Kind: contract.IntentInsertText, Text: "`"}},
		{"extend paired fence", "a\n\n```¦\n```\n\nz", "a\n\n````¦\n````\n\nz", contract.Intent{Kind: contract.IntentInsertText, Text: "`"}},
		{"tilde typing", "> ~~¦\n> z", "> ~~~¦\n> ~~~\n> z", contract.Intent{Kind: contract.IntentInsertText, Text: "~"}},
		{"fence backspace", "```¦\n```\n\nz", "``¦\n\nz", contract.Intent{Kind: contract.IntentDeleteBackward}},
	} {
		for _, nl := range []string{"\n", "\r\n"} {
			t.Run(fmt.Sprintf("%s/%q", tc.name, nl), func(t *testing.T) {
				src, sel := marked(strings.ReplaceAll(tc.before, "\n", nl))
				m := (document.Factory{}).New("test", src, contract.DocMeta{Dominant: nl}, contract.ExtDefault)
				m.SetSelection(sel)
				ctx := contract.EditContext{Text: m.Text(), AST: m.AST(), Selection: m.Selection(), Meta: m.Meta(), Prefs: contract.EditPrefs{Exts: contract.ExtDefault, AutoPair: true}}
				tx, ok := New().HandleIntent(ctx, tc.in)
				if !ok || tx.Coalesce != "" {
					t.Fatalf("transaction %+v, ok=%v", tx, ok)
				}
				if _, err := m.Apply(tx); err != nil {
					t.Fatal(err)
				}
				if got, want := show(m.Text().String(), m.Selection()), strings.ReplaceAll(tc.after, "\n", nl); got != want {
					t.Fatalf("got %q, want %q", got, want)
				}
				next := ctx
				next.Text, next.AST, next.Selection = m.Text(), m.AST(), m.Selection()
				if tc.in.Kind == contract.IntentInsertText {
					if follow, ok := New().FollowUp(next); ok {
						t.Fatalf("extra follow-up would be another undo step: %+v", follow)
					}
				}
				if !m.Undo() || m.Text().String() != src || m.Selection().Primary() != sel.Primary() || m.CanUndo() {
					t.Fatalf("undo did not restore %q: %q, %+v", src, m.Text().String(), m.Selection())
				}
				if !m.Redo() || show(m.Text().String(), m.Selection()) != strings.ReplaceAll(tc.after, "\n", nl) {
					t.Fatal("redo did not restore the edit")
				}
			})
		}
	}
}

func TestTyporaTypingIntoOpenedParagraph(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		src := "a" + nl + nl + "b"
		m := (document.Factory{}).New("test", src, contract.DocMeta{Dominant: nl}, contract.ExtDefault)
		m.SetSelection(contract.Caret(1))
		ctx := func() contract.EditContext {
			return contract.EditContext{Text: m.Text(), AST: m.AST(), Selection: m.Selection(), Meta: m.Meta(), Prefs: contract.EditPrefs{Exts: contract.ExtDefault}}
		}
		for _, in := range []contract.Intent{{Kind: contract.IntentNewline}, {Kind: contract.IntentInsertText, Text: "x"}} {
			tx, ok := New().HandleIntent(ctx(), in)
			if !ok {
				t.Fatal("not handled")
			}
			if _, err := m.Apply(tx); err != nil {
				t.Fatal(err)
			}
		}
		want := "a" + nl + nl + "x" + nl + nl + "b"
		if got := m.Text().String(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		b := m.AST().Block(m.AST().LeafAt(1 + 2*len(nl)))
		if b.Kind != contract.BlockParagraph || m.Text().Slice(b.Range) != "x" {
			t.Fatalf("typed text has wrong leaf: %+v", b)
		}
		if !m.Undo() || !m.Undo() || m.Text().String() != src {
			t.Fatal("two undos did not restore the original source")
		}
	}
}
