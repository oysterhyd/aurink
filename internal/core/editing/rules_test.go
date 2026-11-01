package editing

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestFollowUpGoldens(t *testing.T) {
	rules := map[string][]golden{
		"heading":              {{"# ¦", "# ¦"}, {"## ¦\n\n中文", "## ¦\n\n中文"}, {"- 父\n  - ### ¦", "- 父\n  - ### ¦"}},
		"bullet":               {{"- ¦", "- ¦"}, {"* ¦\n\n中文", "* ¦\n\n中文"}, {"- 父\n  + ¦", "- 父\n  + ¦"}},
		"ordered":              {{"1. ¦", "1. ¦"}, {"3) ¦\n\n中文", "3) ¦\n\n中文"}, {"- 父\n  1. ¦", "- 父\n  1. ¦"}},
		"task":                 {{"[ ] ¦", "- [ ] ¦"}, {"[x] ¦\n\n中文", "- [x] ¦\n\n中文"}, {"> [X] ¦", "> - [X] ¦"}},
		"quote":                {{"> ¦", "> ¦"}, {"> > ¦\n\n中文", "> > ¦\n\n中文"}, {"- 父\n  > ¦", "- 父\n  > ¦"}},
		"strong":               {{"**中文**¦", "**中文**¦"}, {"__中文__¦ tail", "__中文__¦ tail"}, {"- 父\n  - **子**¦", "- 父\n  - **子**¦"}},
		"emphasis":             {{"*中文*¦", "*中文*¦"}, {"_中文_¦ tail", "_中文_¦ tail"}, {"- 父\n  - *子*¦", "- 父\n  - *子*¦"}},
		"strike":               {{"~~中文~~¦", "~~中文~~¦"}, {"> ~~中文~~¦", "> ~~中文~~¦"}, {"- 父\n  - ~~子~~¦", "- 父\n  - ~~子~~¦"}},
		"code":                 {{"`中文`¦", "`中文`¦"}, {"``中文``¦ tail", "``中文``¦ tail"}, {"- 父\n  - `子`¦", "- 父\n  - `子`¦"}},
		"other closed inlines": {{"[x](url)¦", "[x](url)¦"}, {"<u>x</u>¦", "<u>x</u>¦"}, {"==x==¦", "==x==¦"}, {"**x**¦", "**x**¦"}},
		"closed objects":       {{"![x](url)¦", "![x](url)¦"}, {"$x$¦", "$x$¦"}, {"&amp;¦", "&amp;¦"}, {"\\*¦", "\\*¦"}, {":smile:¦", ":smile:¦"}},
	}
	for rule, cases := range rules {
		for n, c := range cases {
			for _, nl := range []string{"\n", "\r\n"} {
				t.Run(fmt.Sprintf("%s/%d/%q", rule, n, nl), func(t *testing.T) {
					guard := goldenGuard(nl)
					src, sel := marked(guard + strings.ReplaceAll(c.before, "\n", nl))
					ctx := contextFor(t, src, sel, nl)
					tx, ok := New().FollowUp(ctx)
					if c.before == c.after {
						if ok || len(tx.Edits) != 0 {
							t.Fatalf("identity rule: %+v", tx)
						}
						return
					}
					if !ok || tx.Origin != contract.OriginInputRule || tx.Coalesce != "" || tx.Label != "editing_undo_input_rule" {
						t.Fatalf("ok=%v, tx=%+v", ok, tx)
					}
					out, selection := apply(t, ctx, tx)
					if out == src {
						t.Fatal("rule returned an identity transaction")
					}
					assertGuard(t, tx, guard, out)
					if got, want := show(out, selection), guard+strings.ReplaceAll(c.after, "\n", nl); got != want {
						t.Fatalf("got %q, want %q", got, want)
					}
					again, _ := New().FollowUp(ctx)
					if !reflect.DeepEqual(tx, again) {
						t.Fatal("impure follow-up")
					}
				})
			}
		}
	}
	for _, input := range []string{"plain ¦", "#¦", "-¦", "```\n[ ] ¦\n```", "<div>\n[ ] ¦\n</div>", "---\n[ ] ¦\n---", "- [ ] ¦", "¦[ ] ¦", "[ ] ¦tail", "\\*text*¦"} {
		src, sel := marked(input)
		if tx, ok := New().FollowUp(contextFor(t, src, sel, "\n")); ok {
			t.Fatalf("unexpected rule for %q: %+v", input, tx)
		}
	}
}

func TestInputRuleHistory(t *testing.T) {
	for _, nl := range []string{"\n", "\r\n"} {
		ctx := contextFor(t, "中文"+nl+nl+"[ ]", contract.Caret(len("中文"+nl+nl+"[ ]")), nl)
		typing, ok := New().HandleIntent(ctx, contract.Intent{Kind: contract.IntentInsertText, Text: " "})
		if !ok || typing.Origin != contract.OriginUser || typing.Coalesce != "typing" || typing.Label != "editing_undo_typing" {
			t.Fatalf("typing: %+v", typing)
		}
		raw := verifyTransaction(t, ctx, typing)
		rule, ok := New().FollowUp(raw)
		if !ok || rule.Origin != contract.OriginInputRule || rule.Coalesce != "" {
			t.Fatalf("rule: %+v", rule)
		}
		formatted := verifyTransaction(t, raw, rule)
		if formatted.Text.String() != "中文"+nl+nl+"- [ ] " {
			t.Fatal("task rule did not run")
		}
		// Undo only the rule, retaining the space that triggered it.
		e := rule.Edits[0]
		inverse := transaction([]contract.TextEdit{edit(e.Range.Start, e.Range.Start+len(e.Text), "")}, raw.Selection, contract.OriginUndo, "input_rule")
		restored := verifyTransaction(t, formatted, inverse)
		if restored.Text.String() != raw.Text.String() || !reflect.DeepEqual(restored.Selection, raw.Selection) {
			t.Fatal("undo of input rule did not preserve the raw typing")
		}
	}
}

func TestFormattingGoldens(t *testing.T) {
	for _, tc := range []struct {
		before string
		check  func(contract.FormatState) bool
	}{
		{"**¦中文¦**", func(f contract.FormatState) bool { return f.Strong == contract.On }},
		{"¦**中** 文¦", func(f contract.FormatState) bool { return f.Strong == contract.Mixed }},
		{"**中** ¦文¦", func(f contract.FormatState) bool { return f.Strong == contract.Off }},
		{"*¦中文¦*", func(f contract.FormatState) bool { return f.Emphasis == contract.On }},
		{"¦*中* 文¦", func(f contract.FormatState) bool { return f.Emphasis == contract.Mixed }},
		{"*中* ¦文¦", func(f contract.FormatState) bool { return f.Emphasis == contract.Off }},
		{"~~¦中文¦~~", func(f contract.FormatState) bool { return f.Strike == contract.On }},
		{"¦~~中~~ 文¦", func(f contract.FormatState) bool { return f.Strike == contract.Mixed }},
		{"~~中~~ ¦文¦", func(f contract.FormatState) bool { return f.Strike == contract.Off }},
		{"`¦中文¦`", func(f contract.FormatState) bool { return f.Code == contract.On }},
		{"¦`中` 文¦", func(f contract.FormatState) bool { return f.Code == contract.Mixed }},
		{"`中` ¦文¦", func(f contract.FormatState) bool { return f.Code == contract.Off }},
		{"[¦中文¦](url)", func(f contract.FormatState) bool { return f.Link == contract.On && f.LinkDest == "url" }},
		{"¦[中](a) [文](b)¦", func(f contract.FormatState) bool { return f.Link == contract.Mixed && f.LinkDest == "" }},
		{"[中](url) ¦文¦", func(f contract.FormatState) bool { return f.Link == contract.Off && f.LinkDest == "" }},
		{"- 父\n  - **¦子¦**", func(f contract.FormatState) bool { return f.Strong == contract.On && f.List == contract.ListBullet }},
		{"1. ¦中¦", func(f contract.FormatState) bool { return f.List == contract.ListOrdered }},
		{"- [x] ¦中¦", func(f contract.FormatState) bool { return f.List == contract.ListTask }},
		{"| 中 | 文 |\n|---|---|\n| ¦x¦ | y |", func(f contract.FormatState) bool { return f.InTable && !f.InCode }},
		{"```\n¦中文¦\n```", func(f contract.FormatState) bool { return f.InCode && f.Block == contract.BlockCode }},
		{"### **中¦文**", func(f contract.FormatState) bool {
			return f.Strong == contract.On && f.Level == 3 && f.Block == contract.BlockHeading
		}},
	} {
		for _, nl := range []string{"\n", "\r\n"} {
			t.Run(fmt.Sprintf("%s/%q", tc.before, nl), func(t *testing.T) {
				src, sel := marked(strings.ReplaceAll(tc.before, "\n", nl))
				ctx := contextFor(t, src, sel, nl)
				f := New().Formatting(ctx)
				if !tc.check(f) {
					t.Fatalf("%q: %+v", tc.before, f)
				}
				if again := New().Formatting(ctx); f != again {
					t.Fatal("impure formatting")
				}
			})
		}
	}
}
