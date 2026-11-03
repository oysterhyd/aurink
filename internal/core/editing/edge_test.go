package editing

import (
	"reflect"
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestAdditionalIntentGoldens(t *testing.T) {
	intentGoldens(t, "square pair", contract.Intent{Kind: contract.IntentInsertText, Text: "["}, golden{"中¦", "中[¦]"}, golden{"- a\n  - ¦b¦", "- a\n  - [¦b¦]"}, golden{"> ¦文", "> [¦文"})
	intentGoldens(t, "backtick pair", contract.Intent{Kind: contract.IntentInsertText, Text: "`"}, golden{"中¦", "中`¦`"}, golden{"- a\n  - ¦b¦", "- a\n  - `¦b¦`"}, golden{"> ¦文", "> `¦文"})
	intentGoldens(t, "pair backspace", contract.Intent{Kind: contract.IntentDeleteBackward}, golden{"(¦)", "¦"}, golden{"[¦]", "¦"}, golden{"`¦`", "¦"})
	intentGoldens(t, "literal code typing", contract.Intent{Kind: contract.IntentInsertText, Text: "("}, golden{"```\n中¦\n```", "```\n中(¦\n```"}, golden{"- ```\n  ¦\n  ```", "- ```\n  (¦\n  ```"}, golden{"    中¦", "    中(¦"})
	intentGoldens(t, "plain tab", contract.Intent{Kind: contract.IntentIndent}, golden{"中¦文", "中  ¦文"}, golden{"¦", "  ¦"}, golden{"a\n¦b", "a\n  ¦b"})
	intentGoldens(t, "multiline indent", contract.Intent{Kind: contract.IntentIndent}, golden{"¦中文\nword¦\ntail", "  ¦中文\n  word¦\ntail"}, golden{"¦- 父\n  - 子¦", "¦- 父\n  - 子¦"}, golden{"> ¦中\n> 文¦", ">   ¦中\n>   文¦"})
	intentGoldens(t, "indent subtree", contract.Intent{Kind: contract.IntentIndent}, golden{"- 先\n- 父¦\n  - 子\n- 后", "- 先\n  - 父¦\n    - 子\n- 后"}, golden{"- 先\n- [ ] 父¦\n  子", "- 先\n  - [ ] 父¦\n    子"}, golden{"- 先\n- 父¦\n  - 子\n    - 孙", "- 先\n  - 父¦\n    - 子\n      - 孙"})
	intentGoldens(t, "line selection end", contract.Intent{Kind: contract.IntentIndent}, golden{"¦- 中\n¦tail", "¦- 中\n¦tail"}, golden{"¦a\n¦b", "  ¦a\n¦b"}, golden{"> ¦中\n¦tail", ">   ¦中\n¦tail"})
	intentGoldens(t, "entity backspace", contract.Intent{Kind: contract.IntentDeleteBackward}, golden{"中&amp;¦", "中¦"}, golden{"\\*¦", "¦"}, golden{"![中](url)¦", "¦"})
	intentGoldens(t, "entity forward", contract.Intent{Kind: contract.IntentDeleteForward}, golden{"¦&amp;中", "¦中"}, golden{"¦\\*中", "¦中"}, golden{"¦![中](url)", "¦"})
	intentGoldens(t, "setext backspace", contract.Intent{Kind: contract.IntentDeleteBackward}, golden{"¦中文\n===", "¦中文"}, golden{"¦中文\n---\n", "¦中文\n"}, golden{"前文\n\n¦中文\n===", "前文\n\n¦中文"})
	intentGoldens(t, "code closer Enter", contract.Intent{Kind: contract.IntentNewline}, golden{"```\n中\n```¦", "```\n中\n```\n\n¦"}, golden{"~~~~\n文\n~~~~¦", "~~~~\n文\n~~~~\n\n¦"}, golden{"- ```\n  中\n  ```¦", "- ```\n  中\n  ```\n\n¦"})
	intentGoldens(t, "list continuation Enter", contract.Intent{Kind: contract.IntentNewline}, golden{"- a\n  中文¦", "- a\n  中文\n- ¦"}, golden{"3) a\n   中文¦", "3) a\n   中文\n4) ¦"}, golden{"- a\n  - b\n    中文¦", "- a\n  - b\n    中文\n  - ¦"})
	intentGoldens(t, "plain paste", contract.Intent{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: "中\r\n文", Markdown: "ignored", AsPlain: true}}, golden{"a¦b", "a中\n文¦b"}, golden{"- a\n  - ¦b¦", "- a\n  - 中\n文¦"}, golden{"> ¦", "> 中\n文¦"})
}

func TestAdditionalCommandGoldens(t *testing.T) {
	commandGoldens(t, contract.EditCommand{ID: contract.CmdFormatStrong}, golden{"¦**中文** plain¦", "**¦中文 plain¦**"}, golden{"¦**中**\n\n**文**¦", "¦中\n\n文¦"}, golden{"¦__中文__ plain¦", "**¦中文 plain¦**"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdFormatCode}, golden{"¦a`中¦", "``¦a`中¦``"}, golden{"¦`中文`¦", "¦中文¦"}, golden{"¦ 中文 ¦", " `¦中文¦` "})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdFormatClear}, golden{"***¦中文¦***", "¦中文¦"}, golden{"¦<u>中文</u>¦", "¦中文¦"}, golden{"**[¦中¦](url)**", "¦中¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdFormatLink}, golden{"[¦中文¦](old)", "¦中文¦"}, golden{"a ¦", "a [¦]()"}, golden{"- 父\n  - [子¦](old)", "- 父\n  - 子¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdInsertLink, Text: "中[文]", URL: "a b(c)"}, golden{"¦", "[¦中\\[文\\]¦](a%20b\\(c\\))"}, golden{"- a\n  - ¦", "- a\n  - [¦中\\[文\\]¦](a%20b\\(c\\))"}, golden{"> ¦word¦", "> [¦word¦](a%20b\\(c\\))"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockParagraph}, golden{"中¦文\n===", "中¦文\n"}, golden{"## 中¦文 ##", "中¦文"}, golden{"- 父\n  - ## 子¦ ##", "- 父\n  - 子¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockCode}, golden{"    中¦文", "中¦文"}, golden{"\t中¦文", "中¦文"}, golden{"    中¦文\n    word", "中¦文\nword"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockCode}, golden{"# 中¦文", "```\n中¦文\n```"}, golden{"## 中¦文 ##", "```\n中¦文\n```"}, golden{"- 父\n  - # 子¦", "- 父\n  - ```\n    子¦\n    ```"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockBulletList}, golden{"0. 中¦", "- 中¦"}, golden{"0) 中¦", "- 中¦"}, golden{"- 父\n  0. 子¦", "- 父\n  - 子¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockOrderedList}, golden{"0. 中¦", "中¦"}, golden{"0) 中¦", "中¦"}, golden{"- 父\n  0. 子¦", "- 父\n  子¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockToggleTask}, golden{"- [ ] 中\n  文¦", "- [x] 中\n  文¦"}, golden{"- [x] 中\n  ¦文¦", "- [ ] 中\n  ¦文¦"}, golden{"- [ ] ¦中\n  文¦", "- [x] ¦中\n  文¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockHeading3}, golden{"#  中¦文 ##", "###  中¦文 ##"}, golden{"##\t中¦文", "###\t中¦文"}, golden{"- 父\n  - #  子¦", "- 父\n  - ###  子¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockBulletList}, golden{"3.  中¦", "-  中¦"}, golden{"3)\t中¦", "-\t中¦"}, golden{"- 父\n  9)  子¦", "- 父\n  -  子¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdFormatLink, URL: "new"}, golden{"[¦中文¦](old \"title\")", "[¦中文¦](new \"title\")"}, golden{"[¦中文¦](<old> 'title')", "[¦中文¦](<new> 'title')"}, golden{"- 父\n  - [子¦](a(b)  \"t\")", "- 父\n  - [子¦](new  \"t\")"})
}

func TestPreferencesAndBoundaries(t *testing.T) {
	for _, tc := range []struct {
		before, after string
		prefs         contract.EditPrefs
		in            contract.Intent
		cmd           contract.EditCommand
	}{
		{"¦中¦", "__¦中¦__", contract.EditPrefs{StrongDelim: "__"}, contract.Intent{}, contract.EditCommand{ID: contract.CmdFormatStrong}},
		{"¦中¦", "_¦中¦_", contract.EditPrefs{EmphasisDelim: "_"}, contract.Intent{}, contract.EditCommand{ID: contract.CmdFormatEmphasis}},
		{"中¦", "~~~go\n中¦\n~~~", contract.EditPrefs{FenceDelim: "~~~"}, contract.Intent{}, contract.EditCommand{ID: contract.CmdBlockCode, Text: "go"}},
		{"x```中¦", "````\nx```中¦\n````", contract.EditPrefs{}, contract.Intent{}, contract.EditCommand{ID: contract.CmdBlockCode}},
		{"¦", "```\n¦\n```", contract.EditPrefs{}, contract.Intent{}, contract.EditCommand{ID: contract.CmdBlockCode}},
		{"中¦", "* 中¦", contract.EditPrefs{BulletMarker: '*'}, contract.Intent{}, contract.EditCommand{ID: contract.CmdBlockBulletList}},
		{"中¦", "- 中¦", contract.EditPrefs{}, contract.Intent{}, contract.EditCommand{ID: contract.CmdBlockBulletList}},
		{"中¦", "中    ¦", contract.EditPrefs{TabSize: 4}, contract.Intent{Kind: contract.IntentIndent}, contract.EditCommand{}},
		{"中¦", "中  ¦", contract.EditPrefs{TabSize: 100}, contract.Intent{Kind: contract.IntentIndent}, contract.EditCommand{}},
		{"中¦", "中  ¦", contract.EditPrefs{}, contract.Intent{Kind: contract.IntentIndent}, contract.EditCommand{}},
		{"中¦", "中(¦", contract.EditPrefs{}, contract.Intent{Kind: contract.IntentInsertText, Text: "("}, contract.EditCommand{}},
		{"\\¦", "\\(¦", contract.EditPrefs{AutoPair: true}, contract.Intent{Kind: contract.IntentInsertText, Text: "("}, contract.EditCommand{}},
		{"\\\\¦", "\\\\(¦)", contract.EditPrefs{AutoPair: true}, contract.Intent{Kind: contract.IntentInsertText, Text: "("}, contract.EditCommand{}},
		{"``¦", "```¦", contract.EditPrefs{AutoPair: true}, contract.Intent{Kind: contract.IntentInsertText, Text: "`"}, contract.EditCommand{}},
		{"(中¦)", "(中)¦", contract.EditPrefs{AutoPair: true}, contract.Intent{Kind: contract.IntentInsertText, Text: ")"}, contract.EditCommand{}},
		{"[中¦]", "[中]¦", contract.EditPrefs{AutoPair: true}, contract.Intent{Kind: contract.IntentInsertText, Text: "]"}, contract.EditCommand{}},
		{"`中¦`", "`中`¦", contract.EditPrefs{AutoPair: true}, contract.Intent{Kind: contract.IntentInsertText, Text: "`"}, contract.EditCommand{}},
	} {
		t.Run(tc.before+string(tc.cmd.ID)+tc.in.Text, func(t *testing.T) {
			src, sel := marked(tc.before)
			ctx := contextFor(t, src, sel, "\n")
			ctx.Prefs = tc.prefs
			var tx contract.Transaction
			var ok bool
			if tc.in.Kind != 0 {
				tx, ok = New().HandleIntent(ctx, tc.in)
			} else {
				var err error
				tx, ok, err = New().Run(ctx, tc.cmd)
				if err != nil {
					t.Fatal(err)
				}
			}
			if !ok {
				t.Fatal("not handled")
			}
			out, selection := apply(t, ctx, tx)
			if got := show(out, selection); got != tc.after {
				t.Fatalf("got %q, want %q; %+v", got, tc.after, tx)
			}
		})
	}
}

func TestInvalidAndUnsupported(t *testing.T) {
	ed := New()
	for _, ctx := range []contract.EditContext{
		{}, contextFor(t, "中", contract.Caret(-1), "\n"), contextFor(t, "中", contract.Caret(4), "\n"),
	} {
		if _, ok := ed.HandleIntent(ctx, contract.Intent{Kind: contract.IntentNewline}); ok {
			t.Fatal("handled invalid context")
		}
		if _, ok := ed.FollowUp(ctx); ok {
			t.Fatal("followed invalid context")
		}
		if _, ok, err := ed.Run(ctx, contract.EditCommand{ID: contract.CmdFormatStrong}); ok || contract.CodeOf(err) != contract.CodeInvalid {
			t.Fatalf("invalid Run: %v %v", ok, err)
		}
		if f := ed.Formatting(ctx); f != (contract.FormatState{}) {
			t.Fatal(f)
		}
	}
	ctx := contextFor(t, "中文", contract.Caret(3), "\n")
	for _, in := range []contract.Intent{{}, {Kind: 255}, {Kind: contract.IntentPaste}, {Kind: contract.IntentInsertText, Text: "\xff"}} {
		if _, ok := ed.HandleIntent(ctx, in); ok {
			t.Fatalf("handled invalid intent %+v", in)
		}
	}
	for _, cmd := range []contract.EditCommand{
		{ID: contract.CmdBlockHeading, Level: 0}, {ID: contract.CmdBlockHeading, Level: 7},
		{ID: contract.CmdBlockCode, Text: "go\nbad"}, {ID: contract.CmdFormatLink, URL: "url\nbad"},
	} {
		if _, ok, err := ed.Run(ctx, cmd); ok || contract.CodeOf(err) != contract.CodeInvalid {
			t.Fatalf("command %+v: ok=%v err=%v", cmd, ok, err)
		}
	}
	for _, id := range []contract.CommandID{
		"x.unknown", "block.heading7", "block.headingx", "format.unknown",
		contract.CmdBlockMoveUp, contract.CmdBlockMoveDown, contract.CmdBlockToggleTask, contract.CmdBlockOutdent,
		contract.CmdTableInsert, contract.CmdTableRowAbove, contract.CmdTableRowBelow, contract.CmdTableColLeft,
		contract.CmdTableColRight, contract.CmdTableDeleteRow, contract.CmdTableDeleteCol, contract.CmdTableAlign,
		contract.CmdTableFormat, contract.CmdTableNextCell, contract.CmdTablePrevCell,
	} {
		if _, ok, err := ed.Run(ctx, contract.EditCommand{ID: id}); ok || err != nil {
			t.Fatalf("command %q: ok=%v err=%v", id, ok, err)
		}
	}
	if tx, ok, err := ed.Run(ctx, contract.EditCommand{ID: contract.CmdFormatHighlight}); !ok || err != nil || len(tx.Edits) == 0 {
		t.Fatalf("highlight command: ok=%v err=%v edits=%v", ok, err, tx.Edits)
	}
	for _, src := range []string{"```\n中文\n```", "|a|b|\n|-|-|\n|中|文|"} {
		ctx := contextFor(t, src, contract.Caret(strings.Index(src, "中")), "\n")
		if _, ok, err := ed.Run(ctx, contract.EditCommand{ID: contract.CmdBlockHeading2}); ok || err != nil {
			t.Fatalf("changed unsupported block: %v %v", ok, err)
		}
	}
}

func TestMixedEndingsAndExactEdits(t *testing.T) {
	src := "# 标题\r\n\r\n1.  中\n3.  文\r\n\n|a|b|\n|:--|--:|\n|x|y|\n"
	at := strings.Index(src, "中") + len("中")
	ctx := contextFor(t, src, contract.Caret(at), "\r\n")
	tx, ok := New().HandleIntent(ctx, contract.Intent{Kind: contract.IntentNewline})
	if !ok || !reflect.DeepEqual(tx.Edits, []contract.TextEdit{edit(at, at, "\r\n2.  ")}) {
		t.Fatalf("nonminimal Enter: %+v", tx)
	}
	out, _ := apply(t, ctx, tx)
	if out != src[:at]+"\r\n2.  "+src[at:] {
		t.Fatal("existing endings, numbers or alignment changed")
	}
	for _, nl := range []string{"\n", "\r\n"} {
		src, sel := marked("- [ ] 中¦")
		ctx := contextFor(t, src, sel, nl)
		tx, _, _ := New().Run(ctx, contract.EditCommand{ID: contract.CmdBlockToggleTask})
		if !reflect.DeepEqual(tx.Edits, []contract.TextEdit{edit(3, 4, "x")}) {
			t.Fatalf("task toggle rewrites more than one byte: %+v", tx.Edits)
		}
	}
}

func TestReverseSelection(t *testing.T) {
	ctx := contextFor(t, "中文", contract.Selection{Ranges: []contract.SelRange{{Anchor: 6, Head: 0}}}, "\n")
	tx, ok, err := New().Run(ctx, contract.EditCommand{ID: contract.CmdFormatStrong})
	if !ok || err != nil {
		t.Fatal(ok, err)
	}
	out, sel := apply(t, ctx, tx)
	if out != "**中文**" || sel.Primary() != (contract.SelRange{Anchor: 8, Head: 2}) {
		t.Fatal(out, sel)
	}
}

func TestGraphemeSequences(t *testing.T) {
	for _, s := range []string{"👍🏽", "👩‍👩‍👧‍👦", "🇨🇳", "é", "각", "각", "각ᆨ", "각", "ᄀꥠᅡᆨ", "का", "a\ufe0f", "🏴\U000e0067\U000e0062\U000e007f"} {
		ctx := contextFor(t, "a"+s+"z", contract.Caret(1+len(s)), "\n")
		tx, _ := New().HandleIntent(ctx, contract.Intent{Kind: contract.IntentDeleteBackward})
		out, sel := apply(t, ctx, tx)
		if out != "az" || sel.Primary().Head != 1 {
			t.Fatalf("backspace %q: %q %+v", s, out, sel)
		}
		ctx.Selection = contract.Caret(1)
		tx, _ = New().HandleIntent(ctx, contract.Intent{Kind: contract.IntentDeleteForward})
		out, _ = apply(t, ctx, tx)
		if out != "az" {
			t.Fatalf("delete %q: %q", s, out)
		}
	}
}
