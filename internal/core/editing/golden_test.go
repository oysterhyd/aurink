package editing

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"aurink/internal/contract"
)

type golden struct {
	before, after string
}

func intentGoldens(t *testing.T, name string, in contract.Intent, cases ...golden) {
	t.Helper()
	if len(cases) < 3 {
		t.Fatal("each rule needs at least three examples")
	}
	for n, c := range cases {
		for _, nl := range []string{"\n", "\r\n"} {
			t.Run(fmt.Sprintf("%s/%d/%q", name, n, nl), func(t *testing.T) {
				guard := goldenGuard(nl)
				src, sel := marked(guard + strings.ReplaceAll(c.before, "\n", nl))
				ctx := contextFor(t, src, sel, nl)
				tx, ok := New().HandleIntent(ctx, in)
				if !ok {
					t.Fatal("not handled")
				}
				out, selection := apply(t, ctx, tx)
				assertGuard(t, tx, guard, out)
				want := guard + strings.ReplaceAll(c.after, "\n", nl)
				if got := show(out, selection); got != want {
					t.Fatalf("%q => %q, want %q; edits %+v", c.before, got, want, tx.Edits)
				}
				again, _ := New().HandleIntent(ctx, in)
				if !reflect.DeepEqual(tx, again) {
					t.Fatal("impure intent")
				}
				origin := contract.OriginUser
				if in.Kind == contract.IntentPaste {
					origin = contract.OriginPaste
				}
				if tx.Origin != origin || !strings.HasPrefix(tx.Label, "editing_undo_") {
					t.Fatalf("wrong metadata: %+v", tx)
				}
				if in.Kind != contract.IntentInsertText && tx.Coalesce != "" {
					t.Fatal("intent coalesces across a history boundary")
				}
			})
		}
	}
}

func commandGoldens(t *testing.T, cmd contract.EditCommand, cases ...golden) {
	t.Helper()
	if len(cases) < 3 {
		t.Fatal("each command needs at least three examples")
	}
	for n, c := range cases {
		for _, nl := range []string{"\n", "\r\n"} {
			t.Run(fmt.Sprintf("%s/%d/%q", cmd.ID, n, nl), func(t *testing.T) {
				guard := goldenGuard(nl)
				src, sel := marked(guard + strings.ReplaceAll(c.before, "\n", nl))
				ctx := contextFor(t, src, sel, nl)
				tx, ok, err := New().Run(ctx, cmd)
				if !ok || err != nil {
					t.Fatalf("ok=%v err=%v", ok, err)
				}
				out, selection := apply(t, ctx, tx)
				assertGuard(t, tx, guard, out)
				want := guard + strings.ReplaceAll(c.after, "\n", nl)
				if got := show(out, selection); got != want {
					t.Fatalf("%q => %q, want %q; edits %+v", c.before, got, want, tx.Edits)
				}
				if tx.Origin != contract.OriginCommand || tx.Coalesce != "" || !strings.HasPrefix(tx.Label, "editing_undo_") {
					t.Fatalf("wrong command metadata: %+v", tx)
				}
				again, ok, err := New().Run(ctx, cmd)
				if !ok || err != nil || !reflect.DeepEqual(tx, again) {
					t.Fatal("impure command")
				}
			})
		}
	}
}

func goldenGuard(nl string) string {
	return strings.ReplaceAll("# 守卫  \n\n|a|b|\n|:---|---:|\n|x|y|\n\n3.  keep\n9.  KEEP\n\n---\n\n", "\n", nl)
}

func assertGuard(t *testing.T, tx contract.Transaction, guard, out string) {
	t.Helper()
	for _, e := range tx.Edits {
		if e.Range.Start < len(guard) {
			t.Fatalf("edit touches unrelated table, numbers or whitespace: %+v", e)
		}
	}
	if !strings.HasPrefix(out, guard) {
		t.Fatal("unrelated table, numbers or whitespace changed")
	}
}

func TestIntentGoldens(t *testing.T) {
	insert := func(s string) contract.Intent { return contract.Intent{Kind: contract.IntentInsertText, Text: s} }
	enter := contract.Intent{Kind: contract.IntentNewline}
	back := contract.Intent{Kind: contract.IntentDeleteBackward}
	forward := contract.Intent{Kind: contract.IntentDeleteForward}
	intentGoldens(t, "typing", insert("中"), golden{"a¦b", "a中¦b"}, golden{"¦ab¦c", "中¦c"}, golden{"- 父\n  - ¦字", "- 父\n  - 中¦字"})
	intentGoldens(t, "paragraph", enter, golden{"a¦b", "a\n\n¦b"}, golden{"中文¦", "中文\n\n¦"}, golden{"a\n\n¦", "a\n\n\n\n¦"})
	intentGoldens(t, "heading", enter, golden{"# a¦", "# a\n\n¦"}, golden{"## 中¦文", "## 中\n\n¦文"}, golden{"### a¦\n\nx", "### a\n\n¦\n\nx"})
	intentGoldens(t, "bullet", enter, golden{"+  中文¦", "+  中文\n+  ¦"}, golden{"- a\n  * b¦", "- a\n  * b\n  * ¦"}, golden{"> - a¦b", "> - a\n> - ¦b"})
	intentGoldens(t, "ordered", enter, golden{"9)  中文¦", "9)  中文\n10)  ¦"}, golden{"1. a\n   3. b¦", "1. a\n   3. b\n   4. ¦"}, golden{"> 8. a¦\n> 2. keep", "> 8. a\n> 9. ¦\n> 2. keep"})
	intentGoldens(t, "task", enter, golden{"- [X] 中文¦", "- [X] 中文\n- [ ] ¦"}, golden{"- 父\n  + [ ] 子¦", "- 父\n  + [ ] 子\n  + [ ] ¦"}, golden{"> 9) [x]  a¦", "> 9) [x]  a\n> 10) [ ]  ¦"})
	intentGoldens(t, "empty item", enter, golden{"- a\n- ¦", "- a\n\n¦"}, golden{"1. 中文\n2. ¦", "1. 中文\n\n¦"}, golden{"- 父\n  - ¦", "- 父\n- ¦"})
	intentGoldens(t, "quote", enter, golden{"> 中文¦", "> 中文\n>\n> ¦"}, golden{"> > a¦", "> > a\n> >\n> > ¦"}, golden{"- a\n  > b¦", "- a\n  > b\n  >\n  > ¦"})
	intentGoldens(t, "empty quote", enter, golden{"> a\n> ¦", "> a\n\n¦"}, golden{"> 中文\n> ¦", "> 中文\n\n¦"}, golden{"> > ¦", "> \n> ¦"})
	intentGoldens(t, "code newline", enter, golden{"```go\n  中文¦\n```", "```go\n  中文\n  ¦\n```"}, golden{"~~~\na¦b\n~~~", "~~~\na\n¦b\n~~~"}, golden{"    中文¦", "    中文\n    ¦"})
	intentGoldens(t, "fence opens", enter, golden{"```go¦", "```go\n¦\n```"}, golden{"~~~~¦", "~~~~\n¦\n~~~~"}, golden{"> ```js¦", "> ```js\n> ¦\n> ```"})
	intentGoldens(t, "nested fence opens", enter, golden{"- ```go¦", "- ```go\n  ¦\n  ```"}, golden{"- a\n  - ```¦", "- a\n  - ```\n    ¦\n    ```"}, golden{"> - ~~~¦", "> - ~~~\n>   ¦\n>   ~~~"})
	intentGoldens(t, "hr enter", enter, golden{"---¦", "---\n\n¦"}, golden{"***¦", "***\n\n¦"}, golden{"中文\n\n___¦", "中文\n\n___\n\n¦"})
	intentGoldens(t, "hard break", contract.Intent{Kind: contract.IntentLineBreak}, golden{"中¦文", "中  \n¦文"}, golden{"- a¦", "- a  \n¦"}, golden{"> a¦", "> a  \n¦"})
	intentGoldens(t, "back grapheme", back, golden{"aé¦", "a¦"}, golden{"a👩‍👩‍👧‍👦¦", "a¦"}, golden{"a🇨🇳¦", "a¦"}, golden{"a👍🏽¦", "a¦"}, golden{"中文¦", "中¦"})
	intentGoldens(t, "back selection", back, golden{"a¦中文¦b", "a¦b"}, golden{"- ¦ab¦", "- ¦"}, golden{"¦a\nb¦c", "¦c"})
	intentGoldens(t, "back item", back, golden{"- ¦中文", "¦中文"}, golden{"- a\n  - ¦b", "- a\n- ¦b"}, golden{"> - ¦中文", "> ¦中文"})
	intentGoldens(t, "back code indent", back, golden{"```\n  ¦中文\n```", "```\n¦中文\n```"}, golden{"```\n    ¦中\n```", "```\n  ¦中\n```"}, golden{"    ¦中文", "  ¦中文"})
	intentGoldens(t, "back quote", back, golden{"> ¦中文", "¦中文"}, golden{"> > ¦中", "> ¦中"}, golden{"- a\n  > ¦b", "- a\n  ¦b"})
	intentGoldens(t, "back heading", back, golden{"## ¦中文", "¦中文"}, golden{"> # ¦中", "> ¦中"}, golden{"- a\n  - # ¦b", "- a\n  - ¦b"})
	intentGoldens(t, "hidden back", back, golden{"**中文**¦", "**中¦**"}, golden{"~~a~~¦", "~~¦~~"}, golden{"[中文](url)¦", "[中¦](url)"})
	intentGoldens(t, "delete forward", forward, golden{"¦中文", "¦文"}, golden{"¦👍🏽a", "¦a"}, golden{"- ¦ab", "- ¦b"})
	intentGoldens(t, "hidden forward", forward, golden{"¦**中文**", "**¦文**"}, golden{"¦~~a~~", "~~¦~~"}, golden{"¦[中文](url)", "[¦文](url)"})
	intentGoldens(t, "word back", contract.Intent{Kind: contract.IntentDeleteBackward, Gran: contract.GranWord}, golden{"foo bar¦", "foo ¦"}, golden{"foo  ¦", "¦"}, golden{"中文¦", "¦"})
	intentGoldens(t, "word forward", contract.Intent{Kind: contract.IntentDeleteForward, Gran: contract.GranWord}, golden{"¦foo bar", "¦ bar"}, golden{"¦  foo bar", "¦ bar"}, golden{"¦中文", "¦"})
	intentGoldens(t, "line back", contract.Intent{Kind: contract.IntentDeleteBackward, Gran: contract.GranLine}, golden{"a\nbc¦d", "a\n¦d"}, golden{"中文¦", "¦"}, golden{"a\n¦b", "a¦b"})
	intentGoldens(t, "line forward", contract.Intent{Kind: contract.IntentDeleteForward, Gran: contract.GranLine}, golden{"a¦bc\nd", "a¦\nd"}, golden{"¦中文", "¦"}, golden{"a¦\nb", "a¦b"})
	intentGoldens(t, "auto pair", insert("("), golden{"中¦文", "中(¦文"}, golden{"- a\n  - ¦b", "- a\n  - (¦b"}, golden{"¦中文¦", "(¦中文¦)"})
	intentGoldens(t, "closer", insert(")"), golden{"(中¦)", "(中)¦"}, golden{"- a\n  - (¦)", "- a\n  - ()¦"}, golden{"x(文¦)z", "x(文)¦z"})
	intentGoldens(t, "selection wrap", insert("*"), golden{"¦中文¦", "*¦中文¦*"}, golden{"- ¦ab¦", "- *¦ab¦*"}, golden{"> ¦中¦", "> *¦中¦*"})
	intentGoldens(t, "indent", contract.Intent{Kind: contract.IntentIndent}, golden{"- a\n- 中¦", "- a\n  - 中¦"}, golden{"> 中¦", ">   中¦"}, golden{"```\n中¦\n```", "```\n  中¦\n```"})
	intentGoldens(t, "outdent", contract.Intent{Kind: contract.IntentOutdent}, golden{"- a\n  - 中¦", "- a\n- 中¦"}, golden{">   中¦", "> 中¦"}, golden{"```\n  中¦\n```", "```\n中¦\n```"})
	intentGoldens(t, "paste markdown", contract.Intent{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: "plain", Markdown: "**中**\n文", HTML: "<b>bad</b>"}}, golden{"a¦b", "a**中**\n文¦b"}, golden{"- a\n  - ¦b", "- a\n  - **中**\n文¦b"}, golden{"¦中文¦", "**中**\n文¦"})
	intentGoldens(t, "paste fallback", contract.Intent{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: "中文", HTML: "<p>ignored</p>", RTF: "ignored"}}, golden{"a¦b", "a中文¦b"}, golden{"- a\n  - ¦b", "- a\n  - 中文¦b"}, golden{"¦ab¦", "中文¦"})
}

func TestCommandGoldens(t *testing.T) {
	commandGoldens(t, contract.EditCommand{ID: contract.CmdFormatStrong}, golden{"¦中文¦", "**¦中文¦**"}, golden{"**¦中文¦**", "¦中文¦"}, golden{"- a\n  - ¦b¦", "- a\n  - **¦b¦**"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdFormatEmphasis}, golden{"¦中文¦", "*¦中文¦*"}, golden{"*¦中文¦*", "¦中文¦"}, golden{"> a ¦", "> a *¦*"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdFormatStrike}, golden{"¦中文¦", "~~¦中文¦~~"}, golden{"~~¦中文¦~~", "¦中文¦"}, golden{"- a\n  - ¦b¦", "- a\n  - ~~¦b¦~~"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdFormatCode}, golden{"¦中文¦", "`¦中文¦`"}, golden{"`¦中文¦`", "¦中文¦"}, golden{"- a\n  - ¦b¦", "- a\n  - `¦b¦`"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdFormatUnderline}, golden{"¦中文¦", "<u>¦中文¦</u>"}, golden{"<u>¦中文¦</u>", "¦中文¦"}, golden{"- a\n  - ¦b¦", "- a\n  - <u>¦b¦</u>"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdFormatLink, URL: "https://example.com"}, golden{"¦中文¦", "[¦中文¦](https://example.com)"}, golden{"- a\n  - ¦b¦", "- a\n  - [¦b¦](https://example.com)"}, golden{"[¦中¦](old)", "[¦中¦](https://example.com)"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdFormatClear}, golden{"**¦中文¦**", "¦中文¦"}, golden{"- a\n  - ~~¦b¦~~", "- a\n  - ¦b¦"}, golden{"[¦中文¦](url)", "¦中文¦"}, golden{"<u>¦中文¦</u>", "¦中文¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockHeading, Level: 3}, golden{"中¦文", "### 中¦文"}, golden{"# 中¦", "### 中¦"}, golden{"- a\n  - 中¦", "- a\n  - ### 中¦"})
	for n := 1; n <= 6; n++ {
		mark := strings.Repeat("#", n) + " "
		commandGoldens(t, contract.EditCommand{ID: contract.CommandID(fmt.Sprintf("block.heading%d", n))}, golden{"a¦", mark + "a¦"}, golden{"## 中¦文", mark + "中¦文"}, golden{"- 父\n  - 子¦", "- 父\n  - " + mark + "子¦"})
	}
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockParagraph}, golden{"## 中¦文", "中¦文"}, golden{"> ## 中¦", "> 中¦"}, golden{"- a\n  - # b¦", "- a\n  - b¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockQuote}, golden{"中¦文", "> 中¦文"}, golden{"> 中¦文", "中¦文"}, golden{"- a\n  - 中¦", "- a\n  > - 中¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockBulletList}, golden{"中¦文", "- 中¦文"}, golden{"- 中¦", "中¦"}, golden{"> 3) 中¦", "> - 中¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockOrderedList}, golden{"中¦文", "1. 中¦文"}, golden{"3) 中¦", "中¦"}, golden{"> - 中¦", "> 1. 中¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockTaskList}, golden{"中¦文", "- [ ] 中¦文"}, golden{"- [x] 中¦", "中¦"}, golden{"- a\n  - b¦", "- a\n  - [ ] b¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockToggleTask}, golden{"- [ ] 中¦", "- [x] 中¦"}, golden{"- [X] 中¦", "- [ ] 中¦"}, golden{"- a\n  - [x] b¦", "- a\n  - [ ] b¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockIndent}, golden{"- a\n- 中¦", "- a\n  - 中¦"}, golden{"> 中¦", ">   中¦"}, golden{"```\n中¦\n```", "```\n  中¦\n```"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockOutdent}, golden{"- a\n  - 中¦", "- a\n- 中¦"}, golden{">   中¦", "> 中¦"}, golden{"```\n  中¦\n```", "```\n中¦\n```"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockCode, Text: "go"}, golden{"中¦文", "```go\n中¦文\n```"}, golden{"¦ab¦", "```go\n¦ab¦\n```"}, golden{"```\n中文¦\n```", "中文¦\n"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockCode}, golden{"- a\n  - 中¦", "- a\n  - ```\n    中¦\n    ```"}, golden{"> 中¦", "> ```\n> 中¦\n> ```"}, golden{"- ```\n  中¦\n  ```", "- 中¦\n"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockHR}, golden{"¦", "---\n\n¦"}, golden{"中¦文", "中\n\n---\n\n¦文"}, golden{"- a\n\n¦", "- a\n\n---\n\n¦"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockMoveDown}, golden{"a¦\n\nb\n", "b\n\na¦\n"}, golden{"中¦\n\n## 文", "## 文\n\n中¦"}, golden{"- a¦\n  - child\n- b", "- b\n- a¦\n  - child"})
	commandGoldens(t, contract.EditCommand{ID: contract.CmdBlockMoveUp}, golden{"a\n\nb¦\n", "b¦\n\na\n"}, golden{"中\n\n## 文¦", "## 文¦\n\n中"}, golden{"- a\n- b¦\n  - child", "- b¦\n  - child\n- a"})
}
