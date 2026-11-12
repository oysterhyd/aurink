package editing

import (
	"fmt"
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/document"
)

func TestTyporaKeyTransactionsAndUndo(t *testing.T) {
	cmd := func(id contract.CommandID) contract.EditCommand { return contract.EditCommand{ID: id} }
	key := func(kind contract.IntentKind) contract.Intent { return contract.Intent{Kind: kind} }
	cases := []struct {
		name, before, after string
		in                  contract.Intent
		cmd                 contract.EditCommand
		sourceOnly          bool
	}{
		{"heading up", "## abc¦", "# abc¦", contract.Intent{}, cmd(contract.CmdBlockHeadingUp), false},
		{"heading down from six", "###### abc¦", "abc¦", contract.Intent{}, cmd(contract.CmdBlockHeadingDown), false},
		{"same heading removes closing marks", "## abc¦ ##", "abc¦", contract.Intent{}, contract.EditCommand{ID: contract.CmdBlockHeading, Level: 2}, false},
		{"loose list", "¦ab\n\ncd¦", "- ab\n\n- cd", contract.Intent{}, cmd(contract.CmdBlockBulletList), true},
		{"quote paragraphs", "¦ab\n\ncd¦", "> ab\n>\n> cd", contract.Intent{}, cmd(contract.CmdBlockQuote), true},
		{"unquote paragraphs", "> ¦ab\n>\n> cd¦", "ab\n\ncd", contract.Intent{}, cmd(contract.CmdBlockQuote), true},
		{"extend partial quote", "> ¦ab\n>\n\ncd¦", "> ab\n>\n>\n> cd", contract.Intent{}, cmd(contract.CmdBlockQuote), true},
		{"word inside", "ab c¦d ef", "ab **c¦d** ef", contract.Intent{}, cmd(contract.CmdFormatStrong), false},
		{"word end before space", "ab¦ cd", "**ab¦** cd", contract.Intent{}, cmd(contract.CmdFormatStrong), false},
		{"word inside link", "[a¦bc](url)", "[**a¦bc**](url)", contract.Intent{}, cmd(contract.CmdFormatStrong), false},
		{"start of word", "ab ¦cd", "ab **¦**cd", contract.Intent{}, cmd(contract.CmdFormatStrong), false},
		{"unicode link", "中文¦ 字", "[中文¦]() 字", contract.Intent{}, cmd(contract.CmdFormatLink), false},
		{"code after paragraph", "abc¦\n\nnext", "abc\n\n```\n¦\n```\n\nnext", contract.Intent{}, cmd(contract.CmdBlockCode), false},
		{"math after paragraph", "abc¦\n\nnext", "abc\n$$\n¦\n$$\n\nnext", contract.Intent{}, cmd(contract.CmdBlockMath), false},
		{"backspace second item", "- one\n- ¦two", "- one\n\n  ¦two", key(contract.IntentDeleteBackward), contract.EditCommand{}, false},
		{"backspace ordered item", "10. one\n11. ¦two", "10. one\n\n    ¦two", key(contract.IntentDeleteBackward), contract.EditCommand{}, false},
		{"backspace nested item", "- one\n  - ¦two", "- one\n  \n  ¦two", key(contract.IntentDeleteBackward), contract.EditCommand{}, false},
		{"backspace paragraph", "abc\n\n¦def", "abc¦def", key(contract.IntentDeleteBackward), contract.EditCommand{}, false},
		{"backspace closing marks", "**abc**¦", "**abc*¦", key(contract.IntentDeleteBackward), contract.EditCommand{}, false},
		{"delete next heading", "abc¦\n\n# def", "abc¦def", key(contract.IntentDeleteForward), contract.EditCommand{}, false},
		{"delete next item", "- abc¦\n- def", "- abc¦def", key(contract.IntentDeleteForward), contract.EditCommand{}, false},
		{"enter heading start", "## ¦abc", "\n\n## ¦abc", key(contract.IntentNewline), contract.EditCommand{}, false},
		{"enter heading middle", "## a¦bc", "## a\n\n## ¦bc", key(contract.IntentNewline), contract.EditCommand{}, false},
		{"enter nested empty", "- a\n  - b\n  - ¦", "- a\n  - b\n  \n  ¦", key(contract.IntentNewline), contract.EditCommand{}, false},
		{"enter last table row", "| a | b |\n| --- | --- |\n| c | d¦ |", "| a | b |\n| --- | --- |\n| c | d |\n\n¦", key(contract.IntentNewline), contract.EditCommand{}, false},
		{"ctrl enter table row", "| a | b |\n| --- | --- |\n| c | d¦ |", "| a | b |\n| --- | --- |\n| c | d |\n| ¦ |  |", contract.Intent{}, contract.EditCommand{ID: contract.CmdTableRowBelow, Rows: -1}, false},
		{"line break in list", "- abc¦", "- abc\n  ¦", key(contract.IntentLineBreak), contract.EditCommand{}, false},
		{"line break in paragraph", "abc¦", "abc\n¦", key(contract.IntentLineBreak), contract.EditCommand{}, false},
		{"tab paragraph", "ab¦c", "ab\t¦c", key(contract.IntentIndent), contract.EditCommand{}, false},
		{"tab punctuation", "a!¦?", "a!\t¦?", key(contract.IntentIndent), contract.EditCommand{}, false},
		{"tab quote paragraph", "> abc¦", "> abc\t¦", key(contract.IntentIndent), contract.EditCommand{}, false},
		{"tab code", "```\nab¦c\n```", "```\nab\t¦c\n```", key(contract.IntentIndent), contract.EditCommand{}, false},
		{"tab first item", "- abc¦", "- abc\t¦", key(contract.IntentIndent), contract.EditCommand{}, false},
		{"outdent top item", "- abc¦", "abc¦", key(contract.IntentOutdent), contract.EditCommand{}, false},
		{"pair quote", "¦", "\"¦\"", contract.Intent{Kind: contract.IntentInsertText, Text: "\""}, contract.EditCommand{}, false},
		{"backtick literal", "¦", "`¦", contract.Intent{Kind: contract.IntentInsertText, Text: "`"}, contract.EditCommand{}, false},
	}
	for _, tc := range cases {
		for _, nl := range []string{"\n", "\r\n"} {
			t.Run(fmt.Sprintf("%s/%q", tc.name, nl), func(t *testing.T) {
				src, selection := marked(strings.ReplaceAll(tc.before, "\n", nl))
				model := (document.Factory{}).New("typora", src, contract.DocMeta{Dominant: nl}, contract.ExtDefault)
				model.SetSelection(selection)
				ctx := contract.EditContext{Text: model.Text(), AST: model.AST(), Selection: model.Selection(), Meta: model.Meta(), Prefs: contract.EditPrefs{AutoPair: true, Exts: contract.ExtDefault}}
				var tx contract.Transaction
				var ok bool
				if tc.cmd.ID != "" {
					var err error
					tx, ok, err = New().Run(ctx, tc.cmd)
					if err != nil {
						t.Fatal(err)
					}
				} else {
					tx, ok = New().HandleIntent(ctx, tc.in)
				}
				if !ok {
					t.Fatal("not handled")
				}
				if _, err := model.Apply(tx); err != nil {
					t.Fatal(err)
				}
				got := model.Text().String()
				if !tc.sourceOnly {
					got = show(got, model.Selection())
				}
				if want := strings.ReplaceAll(tc.after, "\n", nl); got != want {
					t.Fatalf("got %q, want %q", got, want)
				}
				if !model.Undo() || model.Text().String() != src || model.Selection().Primary() != selection.Primary() || model.CanUndo() {
					t.Fatal("one undo did not restore source and selection")
				}
				if !model.Redo() || model.Text().String() == src {
					t.Fatal("redo did not restore the change")
				}
			})
		}
	}
}

func TestTyporaTableEnterSelectsCellBelow(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		for _, tc := range []struct{ before, after string }{
			{"| a | ¦b |\n| --- | --- |\n| c | d |", "| a | b |\n| --- | --- |\n| c | ¦d¦ |"},
			{"| a | b |\n| --- | --- |\n| c¦ | d |\n| e | f |", "| a | b |\n| --- | --- |\n| c | d |\n| ¦e¦ | f |"},
		} {
			src, selection := marked(strings.ReplaceAll(tc.before, "\n", nl))
			ctx := contextFor(t, src, selection, nl)
			tx, ok := New().HandleIntent(ctx, contract.Intent{Kind: contract.IntentNewline})
			if !ok || len(tx.Edits) != 0 {
				t.Fatalf("table navigation changed source: %+v", tx)
			}
			out, result := apply(t, ctx, tx)
			if got, want := show(out, result), strings.ReplaceAll(tc.after, "\n", nl); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		}
	}
}

func TestTyporaHeadingUpStopsAtOne(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		src := "# one" + nl + nl + "next"
		ctx := contextFor(t, src, contract.Caret(len("# one")), nl)
		tx, ok, err := New().Run(ctx, contract.EditCommand{ID: contract.CmdBlockHeadingUp})
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			got, _ := apply(t, ctx, tx)
			if got != src {
				t.Fatalf("H1 changed to %q", got)
			}
		}
	}
}

func TestTyporaHeadingCommandsRespectRawBlocks(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		src, selection := marked(strings.ReplaceAll("¦paragraph\n\n```\ncode\n```¦", "\n", nl))
		ctx := contextFor(t, src, selection, nl)
		for _, id := range []contract.CommandID{contract.CmdBlockHeadingUp, contract.CmdBlockHeadingDown} {
			if tx, ok, err := New().Run(ctx, contract.EditCommand{ID: id}); err != nil || ok && len(tx.Edits) != 0 {
				t.Fatalf("%s changed a selection spanning a code block: %+v, %v", id, tx, err)
			}
		}
	}
}

func TestTyporaInsertedBlocksParseSeparately(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		for _, tc := range []struct {
			id   contract.CommandID
			kind contract.BlockKind
		}{{contract.CmdBlockCode, contract.BlockCode}, {contract.CmdBlockMath, contract.BlockMath}} {
			src := "before" + nl + nl + "after"
			ctx := contextFor(t, src, contract.Caret(3), nl)
			tx, ok, err := New().Run(ctx, contract.EditCommand{ID: tc.id})
			if !ok || err != nil {
				t.Fatalf("%s: %v", tc.id, err)
			}
			next := verifyTransaction(t, ctx, tx)
			if next.AST.Block(next.AST.LeafAt(0)).Kind != contract.BlockParagraph || next.AST.Block(next.AST.LeafAt(next.Selection.Primary().Head)).Kind != tc.kind {
				t.Fatalf("%s did not leave a paragraph followed by the new block: %q", tc.id, next.Text.String())
			}
		}
	}
}
