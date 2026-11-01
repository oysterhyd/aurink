package editing

import (
	"fmt"
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestFormatReviewGoldens(t *testing.T) {
	var cases []regression
	for _, f := range []struct {
		id          contract.CommandID
		open, close string
		kind        contract.InlineKind
	}{
		{contract.CmdFormatStrong, "**", "**", contract.InlineStrong},
		{contract.CmdFormatEmphasis, "*", "*", contract.InlineEmphasis},
		{contract.CmdFormatStrike, "~~", "~~", contract.InlineStrike},
		{contract.CmdFormatCode, "`", "`", contract.InlineCode},
		{contract.CmdFormatUnderline, "<u>", "</u>", contract.InlineHTML},
		{contract.CmdFormatLink, "[", "](url)", contract.InlineLink},
	} {
		cmd := contract.EditCommand{ID: f.id, URL: "url"}
		name := string(f.id)
		cases = append(cases,
			regression{name: name + " blocks", before: "¦a\n\nb¦", after: f.open + "¦a" + f.close + "\n\n" + f.open + "b¦" + f.close, cmd: cmd},
			regression{name: name + " soft break", before: "¦a\nb¦", after: f.open + "¦a\nb¦" + f.close, cmd: cmd},
			regression{name: name + " nested blocks", before: "> - ¦a\n> - b¦", after: "> - " + f.open + "¦a" + f.close + "\n> - " + f.open + "b¦" + f.close, cmd: cmd},
			regression{name: name + " table cells", before: "|a|b|\n|-|-|\n|¦c|d¦|", after: "|a|b|\n|-|-|\n|" + f.open + "¦c" + f.close + "|" + f.open + "d¦" + f.close + "|", cmd: cmd},
			regression{name: name + " Unicode space", before: "¦\u3000a\t¦", after: "\u3000" + f.open + "¦a¦" + f.close + "\t", cmd: cmd},
		)
		cmd.URL = ""
		cases = append(cases, regression{name: name + " blocks off", before: "¦" + f.open + "a" + f.close + "\n\n" + f.open + "b" + f.close + "¦", after: "¦a\n\nb¦", cmd: cmd})
		for _, nl := range []string{"\n", "\r\n"} {
			t.Run(name+fmt.Sprintf("/nodes/%q", nl), func(t *testing.T) {
				src, sel := marked("¦a" + nl + nl + "b¦")
				ctx := contextFor(t, src, sel, nl)
				tx, ok, err := New().Run(ctx, contract.EditCommand{ID: f.id, URL: "url"})
				if !ok || err != nil {
					t.Fatal(ok, err)
				}
				next := verifyTransaction(t, ctx, tx)
				count := 0
				for i := range next.AST.Leaves() {
					for _, n := range next.AST.Inlines(i) {
						if n.Kind == f.kind {
							count++
						}
					}
				}
				want := 2
				if f.id == contract.CmdFormatUnderline {
					want = 4
				}
				if count != want {
					t.Fatalf("format did not parse: nodes=%d, want %d; %q", count, want, next.Text.String())
				}
			})
		}
	}
	checkRegressions(t, cases)
}

func TestMixedFormatMinimalEdits(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		src, sel := marked("¦a" + nl + nl + "__b__¦")
		ctx := contextFor(t, src, sel, nl)
		tx, ok, err := New().Run(ctx, contract.EditCommand{ID: contract.CmdFormatStrong})
		if !ok || err != nil {
			t.Fatal(ok, err)
		}
		out, selection := apply(t, ctx, tx)
		if got, want := show(out, selection), "**¦a**"+nl+nl+"__b¦__"; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		for _, e := range tx.Edits {
			if e.Range.Start > 1 {
				t.Fatalf("rewrote an already formatted block: %+v", tx.Edits)
			}
		}
	}
}

func TestRawCommandReviewGoldens(t *testing.T) {
	ids := []contract.CommandID{
		contract.CmdFormatStrong, contract.CmdFormatEmphasis, contract.CmdFormatStrike, contract.CmdFormatCode,
		contract.CmdFormatUnderline, contract.CmdFormatHighlight, contract.CmdFormatLink, contract.CmdFormatClear, contract.CmdInsertLink,
		contract.CmdBlockHeading, contract.CmdBlockParagraph, contract.CmdBlockQuote, contract.CmdBlockBulletList,
		contract.CmdBlockOrderedList, contract.CmdBlockTaskList, contract.CmdBlockToggleTask, contract.CmdBlockHR,
	}
	var cases []regression
	for _, id := range ids {
		for _, raw := range []string{"```\n¦a¦\n```", "    ¦a¦", "<div>\n¦a¦\n</div>", "---\nt: ¦a¦\n---"} {
			cases = append(cases, regression{name: string(id) + "/" + raw, before: raw, after: raw, cmd: contract.EditCommand{ID: id, Level: 2, URL: "url"}, unhandled: true})
		}
		for _, raw := range []string{"```\na\n```", "<div>\na\n</div>"} {
			before := "¦p\n\n" + raw + "¦"
			cases = append(cases, regression{name: string(id) + " crossing " + raw, before: before, after: before, cmd: contract.EditCommand{ID: id, Level: 2}, unhandled: true})
		}
		if strings.HasPrefix(string(id), "block.") {
			before := "|a|b|\n|-|-|\n|¦c¦|d|"
			cases = append(cases, regression{name: string(id) + " table", before: before, after: before, cmd: contract.EditCommand{ID: id, Level: 2}, unhandled: true})
		}
	}
	for _, id := range []contract.CommandID{contract.CmdBlockCode, contract.CmdBlockIndent, contract.CmdBlockOutdent} {
		for _, raw := range []string{"<div>\n¦a¦\n</div>", "---\nt: ¦a¦\n---", "|a|b|\n|-|-|\n|¦c¦|d|"} {
			cases = append(cases, regression{name: string(id) + "/" + raw, before: raw, after: raw, cmd: contract.EditCommand{ID: id}, unhandled: true})
		}
	}
	checkRegressions(t, cases)
}

func TestBlockReviewGoldens(t *testing.T) {
	checkRegressions(t, []regression{
		{name: "mixed bullet", before: "- ¦a\n\nb¦", after: "- ¦a\n\n- b¦", cmd: contract.EditCommand{ID: contract.CmdBlockBulletList}},
		{name: "mixed ordered", before: "3) ¦a\n\nb¦", after: "3) ¦a\n\n2. b¦", cmd: contract.EditCommand{ID: contract.CmdBlockOrderedList}},
		{name: "mixed task", before: "- [x] ¦a\n\nb¦", after: "- [x] ¦a\n\n- [ ] b¦", cmd: contract.EditCommand{ID: contract.CmdBlockTaskList}},
		{name: "mixed quote", before: "> ¦a\n\nb¦", after: "> ¦a\n\n> b¦", cmd: contract.EditCommand{ID: contract.CmdBlockQuote}},
		{name: "quote code soft break", before: "> ¦a\n> b¦", after: "> ```\n> ¦a\n> b¦\n> ```", cmd: contract.EditCommand{ID: contract.CmdBlockCode}},
		{name: "item code soft break", before: "- ¦a\n  b¦", after: "- ```\n  ¦a\n  b¦\n  ```", cmd: contract.EditCommand{ID: contract.CmdBlockCode}},
		{name: "code crossing fence", before: "¦a\n\n```\nb\n```¦", after: "¦a\n\n```\nb\n```¦", cmd: contract.EditCommand{ID: contract.CmdBlockCode}, unhandled: true},
		{name: "move nested subtree", before: "> - a\n>   - child\n> - b¦\n>   - other", after: "> - b¦\n>   - other\n> - a\n>   - child", cmd: contract.EditCommand{ID: contract.CmdBlockMoveUp}},
		{name: "move nested siblings", before: "- parent\n  - a¦\n  - b\n- tail", after: "- parent\n  - b\n  - a¦\n- tail", cmd: contract.EditCommand{ID: contract.CmdBlockMoveDown}},
		{name: "last nested sibling", before: "- parent\n  - a¦\n- tail", after: "- parent\n  - a¦\n- tail", cmd: contract.EditCommand{ID: contract.CmdBlockMoveDown}, unhandled: true},
		{name: "move compact quote prefix", before: " >- ¦a\n >- b", after: " >- b\n >- ¦a", cmd: contract.EditCommand{ID: contract.CmdBlockMoveDown}},
	})
}

func TestIntentReviewGoldens(t *testing.T) {
	var cases []regression
	for _, raw := range []struct{ before, after string }{
		{"```\n- ¦a\n```", "```\n-¦a\n```"},
		{"<div>\n- ¦a\n</div>", "<div>\n-¦a\n</div>"},
		{"---\n- ¦a\n---", "---\n-¦a\n---"},
	} {
		cases = append(cases, regression{name: "raw backspace " + raw.before, before: raw.before, after: raw.after, in: contract.Intent{Kind: contract.IntentDeleteBackward}})
	}
	for _, raw := range []string{"```\na¦\n```", "<div>\na¦\n</div>", "---\nt: a¦\n---"} {
		cases = append(cases, regression{name: "raw hard break " + raw, before: raw, after: strings.Replace(raw, "¦", "\n¦", 1), in: contract.Intent{Kind: contract.IntentLineBreak}})
		cases = append(cases, regression{name: "raw pair " + raw, before: raw, after: strings.Replace(raw, "¦", "(¦", 1), in: contract.Intent{Kind: contract.IntentInsertText, Text: "("}})
	}
	cases = append(cases,
		regression{name: "pair before digit", before: "¦1", after: "(¦1", in: contract.Intent{Kind: contract.IntentInsertText, Text: "("}},
		regression{name: "pair before closer", before: "¦]", after: "(¦)]", in: contract.Intent{Kind: contract.IntentInsertText, Text: "("}},
		regression{name: "pair before punctuation", before: "¦!", after: "(¦!", in: contract.Intent{Kind: contract.IntentInsertText, Text: "("}},
		regression{name: "pair selection blocks", before: "¦a\n\nb¦", after: "*¦a*\n\n*b¦*", in: contract.Intent{Kind: contract.IntentInsertText, Text: "*"}},
		regression{name: "code pair blocks", before: "¦a\n\nb¦", after: "`¦a`\n\n`b¦`", in: contract.Intent{Kind: contract.IntentInsertText, Text: "`"}},
		regression{name: "indent ordered task", before: "10. [x] a\n11. [ ] b¦", after: "10. [x] a\n    1. [ ] b¦", in: contract.Intent{Kind: contract.IntentIndent}},
		regression{name: "indent child quote", before: "- a\n- b¦\n  > c", after: "- a\n  - b¦\n    > c", in: contract.Intent{Kind: contract.IntentIndent}},
		regression{name: "outdent child quote", before: "- a\n  - b¦\n    > c", after: "- a\n- b¦\n  > c", in: contract.Intent{Kind: contract.IntentOutdent}},
		regression{name: "nested raw newline", before: "> <div>¦\n> </div>", after: "> <div>\n> ¦\n> </div>", in: contract.Intent{Kind: contract.IntentNewline}},
		regression{name: "nested code newline", before: "- ```\n  a¦\n  ```", after: "- ```\n  a\n  ¦\n  ```", in: contract.Intent{Kind: contract.IntentNewline}},
		regression{name: "nested raw hard break", before: "> <div>¦\n> </div>", after: "> <div>\n> ¦\n> </div>", in: contract.Intent{Kind: contract.IntentLineBreak}},
		regression{name: "inline code pair", before: "`a¦ b`", after: "`a(¦ b`", in: contract.Intent{Kind: contract.IntentInsertText, Text: "("}},
	)
	checkRegressions(t, cases)
}

func TestInlineCodeReviewGoldens(t *testing.T) {
	var cases []regression
	for _, id := range []contract.CommandID{contract.CmdFormatStrong, contract.CmdFormatEmphasis, contract.CmdFormatStrike, contract.CmdFormatUnderline, contract.CmdFormatLink} {
		cases = append(cases, regression{name: string(id), before: "`¦a¦`", after: "`¦a¦`", cmd: contract.EditCommand{ID: id, URL: "url"}, unhandled: true})
	}
	checkRegressions(t, cases)
}

func TestTableIntentReviewGoldens(t *testing.T) {
	var cases []regression
	for _, in := range []contract.Intent{
		{Kind: contract.IntentNewline}, {Kind: contract.IntentLineBreak}, {Kind: contract.IntentIndent}, {Kind: contract.IntentOutdent},
		{Kind: contract.IntentDeleteBackward}, {Kind: contract.IntentDeleteForward},
		{Kind: contract.IntentDeleteBackward, Gran: contract.GranWord}, {Kind: contract.IntentDeleteForward, Gran: contract.GranLine},
		{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: "a\nb"}},
	} {
		before := "|a|b|\n|-|-|\n|c\n¦|d|e|\n"
		cases = append(cases, regression{name: fmt.Sprintf("table boundary/%d/%d", in.Kind, in.Gran), before: before, after: before, in: in, noEdits: true})
	}
	cases = append(cases,
		regression{name: "table plain cell deletion", before: "|a|b|\n|-|-|\n|cd¦|e|", after: "|a|b|\n|-|-|\n|c¦|e|", in: contract.Intent{Kind: contract.IntentDeleteBackward}},
		regression{name: "table single line paste", before: "|a|b|\n|-|-|\n|c¦|d|", after: "|a|b|\n|-|-|\n|cx¦|d|", in: contract.Intent{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: "x"}}},
		regression{name: "table fence boundary", before: "```\nx\n```¦\n\n|a|b|\n|-|-|\n|c|d|", after: "```\nx\n```¦\n\n|a|b|\n|-|-|\n|c|d|", in: contract.Intent{Kind: contract.IntentDeleteBackward, Gran: contract.GranLine}, noEdits: true},
	)
	checkRegressions(t, cases)
}
