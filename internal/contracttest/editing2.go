package contracttest

import (
	"regexp"
	"strings"
	"testing"

	"aurink/internal/contract"
)

// caretMark marks the caret in table cases, where "|" is text.
const caretMark = "‸"

func markedWith(s string) (string, contract.Selection) {
	first := strings.Index(s, caretMark)
	src := strings.Replace(s, caretMark, "", 1)
	second := strings.Index(src, caretMark)
	if second < 0 {
		return src, contract.Caret(first)
	}
	src = strings.Replace(src, caretMark, "", 1)
	return src, contract.Selection{Ranges: []contract.SelRange{{Anchor: first, Head: second}}}
}

func showWith(s string, r contract.SelRange) string {
	if r.Empty() {
		return s[:r.Head] + caretMark + s[r.Head:]
	}
	a := r.Range()
	return s[:a.Start] + caretMark + s[a.Start:a.End] + caretMark + s[a.End:]
}

// TestEditingV2 checks BE-1.6: tables (docs/02 §5.8), paste and insert
// commands. Cases are run with LF and CRLF, and table cases with text around
// the table that must keep its bytes.
func TestEditingV2(t *testing.T, f EditingFixture) {
	const table = "| a | b |\n| --- | :-: |\n| 1 | 2 |"
	type outcome struct {
		after string // "" when the command must not apply
	}
	// do runs a command or an intent on a case written with ‸.
	do := func(t *testing.T, before, dominant string, cmd *contract.EditCommand, in *contract.Intent) (string, bool) {
		t.Helper()
		src, sel := markedWith(strings.ReplaceAll(before, "\n", dominant))
		ctx := f.Context(src, sel, dominant)
		var tx contract.Transaction
		var ok bool
		if cmd != nil {
			var err error
			tx, ok, err = f.Editing.Run(ctx, *cmd)
			if err != nil {
				t.Fatal(err)
			}
		} else {
			tx, ok = f.Editing.HandleIntent(ctx, *in)
		}
		if !ok {
			return "", false
		}
		out, caret := applied(t, src, sel, tx)
		return showWith(out, caret), true
	}
	check := func(t *testing.T, name, before, after string, cmd *contract.EditCommand, in *contract.Intent, around bool) {
		for _, dominant := range []string{"\n", "\r\n"} {
			n := name
			if dominant != "\n" {
				n += " CRLF"
			}
			t.Run(n, func(t *testing.T) {
				b, a := before, after
				if around {
					b, a = "x\n\n"+b+"\n\ny", "x\n\n"+a+"\n\ny"
				}
				got, ok := do(t, b, dominant, cmd, in)
				want := strings.ReplaceAll(a, "\n", dominant)
				switch {
				case after == "" && ok:
					t.Fatalf("applied, giving %q", got)
				case after != "" && !ok:
					t.Fatal("not applied")
				case after != "" && got != want:
					t.Fatalf("%q:\n got %q\nwant %q", b, got, want)
				}
			})
		}
	}
	at := func(text, inCell string) string {
		// text with the caret placed before the first occurrence of inCell.
		i := strings.Index(text, inCell)
		return text[:i] + caretMark + text[i:]
	}
	cmd := func(id contract.CommandID) *contract.EditCommand { return &contract.EditCommand{ID: id} }
	align := func(a contract.Align) *contract.EditCommand {
		return &contract.EditCommand{ID: contract.CmdTableAlign, Align: a}
	}
	intent := func(k contract.IntentKind, text string) *contract.Intent {
		return &contract.Intent{Kind: k, Text: text}
	}

	for _, tt := range []struct {
		name, before, after string
		cmd                 *contract.EditCommand
		in                  *contract.Intent
	}{
		// Rows. New cells are two spaces; the caret goes to the same column.
		{"row below a body row", at(table, "1"), table + "\n| ‸ |  |", cmd(contract.CmdTableRowBelow), nil},
		{"row below the header", at(table, "b |"), "| a | b |\n| --- | :-: |\n|  | ‸ |\n| 1 | 2 |", cmd(contract.CmdTableRowBelow), nil},
		{"row above a body row", at(table, "2"), "| a | b |\n| --- | :-: |\n|  | ‸ |\n| 1 | 2 |", cmd(contract.CmdTableRowAbove), nil},
		{"no row above the header", at(table, "a"), "", cmd(contract.CmdTableRowAbove), nil},
		{"delete a body row", at(table, "1"), "| ‸a | b |\n| --- | :-: |", cmd(contract.CmdTableDeleteRow), nil},
		{"delete a middle row", at(table+"\n| 3 | 4 |", "1"), "| a | b |\n| --- | :-: |\n| ‸3 | 4 |", cmd(contract.CmdTableDeleteRow), nil},
		{"the header cannot be deleted as a row", at(table, "a"), "", cmd(contract.CmdTableDeleteRow), nil},
		// Columns. Only the cells of the column change.
		{"column right", at(table, "a"), "| a | ‸ | b |\n| --- | --- | :-: |\n| 1 |  | 2 |", cmd(contract.CmdTableColRight), nil},
		{"column left", at(table, "a"), "| ‸ | a | b |\n| --- | --- | :-: |\n|  | 1 | 2 |", cmd(contract.CmdTableColLeft), nil},
		{"delete a column", at(table, "b |"), "| ‸a |\n| --- |\n| 1 |", cmd(contract.CmdTableDeleteCol), nil},
		{"delete the first column", at(table, "a"), "| ‸b |\n| :-: |\n| 2 |", cmd(contract.CmdTableDeleteCol), nil},
		{"the last column cannot be deleted", at("| a |\n| --- |\n| 1 |", "1"), "", cmd(contract.CmdTableDeleteCol), nil},
		// Alignment changes only the delimiter cell and keeps its dashes.
		{"align center", at(table, "a"), "| ‸a | b |\n| :---: | :-: |\n| 1 | 2 |", align(contract.AlignCenter), nil},
		{"align left", at(table, "2"), "| a | b |\n| --- | :- |\n| 1 | ‸2 |", align(contract.AlignLeft), nil},
		{"align right", at(table, "2"), "| a | b |\n| --- | -: |\n| 1 | ‸2 |", align(contract.AlignRight), nil},
		{"align none", at(table, "2"), "| a | b |\n| --- | - |\n| 1 | ‸2 |", align(contract.AlignNone), nil},
		// Cell navigation selects the content of the next cell.
		{"next cell", at(table, "a"), "| a | ‸b‸ |\n| --- | :-: |\n| 1 | 2 |", cmd(contract.CmdTableNextCell), nil},
		{"next cell skips the delimiter row", at(table, "b |"), "| a | b |\n| --- | :-: |\n| ‸1‸ | 2 |", cmd(contract.CmdTableNextCell), nil},
		{"next cell after the last adds a row", at(table, "2"), table + "\n| ‸ |  |", cmd(contract.CmdTableNextCell), nil},
		{"next cell into an empty cell", at("| a |  |\n| --- | --- |", "a"), "| a | ‸ |\n| --- | --- |", cmd(contract.CmdTableNextCell), nil},
		{"previous cell", at(table, "1"), "| a | ‸b‸ |\n| --- | :-: |\n| 1 | 2 |", cmd(contract.CmdTablePrevCell), nil},
		{"no cell before the first", at(table, "a"), "", cmd(contract.CmdTablePrevCell), nil},
		// Keys in a cell.
		{"Tab moves to the next cell", at(table, "a"), "| a | ‸b‸ |\n| --- | :-: |\n| 1 | 2 |", nil, intent(contract.IntentIndent, "")},
		{"Shift+Tab moves back", at(table, "1"), "| a | ‸b‸ |\n| --- | :-: |\n| 1 | 2 |", nil, intent(contract.IntentOutdent, "")},
		{"Enter adds a row below", at(table, "1"), table + "\n| ‸ |  |", nil, intent(contract.IntentNewline, "")},
		{"Shift+Enter is a <br>", "| a | b |\n| --- | :-: |\n| 1‸ | 2 |", "| a | b |\n| --- | :-: |\n| 1<br>‸ | 2 |", nil, intent(contract.IntentLineBreak, "")},
		{"a typed pipe is escaped", "| a | b |\n| --- | :-: |\n| 1‸ | 2 |", "| a | b |\n| --- | :-: |\n| 1\\|‸ | 2 |", nil, intent(contract.IntentInsertText, "|")},
		{"paste into a cell stays in the cell", "| a | b |\n| --- | :-: |\n| 1‸ | 2 |", "| a | b |\n| --- | :-: |\n| 1x<br>y\\|z‸ | 2 |", nil,
			&contract.Intent{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: "x\ny|z"}}},
		// Resize and delete, as Typora's table toolbar (2026-11-04). New cells
		// are two spaces, new delimiter cells ---; the caret stays in its cell.
		{"resize grows", at(table, "a"), "| ‸a | b |  |\n| --- | :-: | --- |\n| 1 | 2 |  |\n|  |  |  |",
			&contract.EditCommand{ID: contract.CmdTableResize, Rows: 2, Cols: 3}, nil},
		{"resize shrinks", at(table+"\n| 3 | 4 |", "a"), "| ‸a |\n| --- |\n| 1 |",
			&contract.EditCommand{ID: contract.CmdTableResize, Rows: 1, Cols: 1}, nil},
		{"resize to the header alone", at(table, "b |"), "| a | ‸b |\n| --- | :-: |",
			&contract.EditCommand{ID: contract.CmdTableResize, Rows: 0, Cols: 2}, nil},
		{"resize to the same size", at(table, "a"), "", &contract.EditCommand{ID: contract.CmdTableResize, Rows: 1, Cols: 2}, nil},
		{"resize to no column", at(table, "a"), "", &contract.EditCommand{ID: contract.CmdTableResize, Rows: 1, Cols: 0}, nil},
		{"delete the table", at(table, "2"), "‸", cmd(contract.CmdTableDelete), nil},
		// Backspace in an empty cell goes to the end of the previous cell, over
		// the delimiter row; in the first cell of a table with no text at all
		// it deletes the table (Typora, 2026-11-04).
		{"backspace in an empty cell goes back a cell", "| a | ‸ |\n| --- | --- |\n| 1 | 2 |", "| a‸ |  |\n| --- | --- |\n| 1 | 2 |",
			nil, intent(contract.IntentDeleteBackward, "")},
		{"backspace in an empty first body cell goes to the header", "| a | b |\n| --- | --- |\n| ‸ | 2 |", "| a | b‸ |\n| --- | --- |\n|  | 2 |",
			nil, intent(contract.IntentDeleteBackward, "")},
		{"backspace through an empty table deletes it", "| ‸ |  |\n| --- | --- |\n|  |  |", "‸",
			nil, intent(contract.IntentDeleteBackward, "")},
		// An empty task item is a task (TestProjection "an empty task"): Enter
		// on it leaves the list, as on an empty item, and typing fills it.
		{"Enter on an empty task leaves the list", "- [ ] a\n- [ ] ‸", "- [ ] a\n\n‸", nil, intent(contract.IntentNewline, "")},
		{"Enter on a lone empty task leaves the list", "- [ ] ‸", "‸", nil, intent(contract.IntentNewline, "")},
		{"typing in an empty task", "- [ ] ‸", "- [ ] b‸", nil, intent(contract.IntentInsertText, "b")},
		// Format reflows the table only: widths by display width (CJK counts 2),
		// at least 3, padding by alignment.
		{"format", at("| a | 中文 |\n|---|:-:|\n| 123456 | x |", "a"),
			"| ‸a      | 中文 |\n| ------ | :--: |\n| 123456 |  x   |", cmd(contract.CmdTableFormat), nil},
	} {
		check(t, tt.name, tt.before, tt.after, tt.cmd, tt.in, true)
	}

	// Commands that make or find tables and other blocks.
	insertTable := &contract.EditCommand{ID: contract.CmdTableInsert, Rows: 2, Cols: 3}
	newTable := "| ‸ |  |  |\n| --- | --- | --- |\n|  |  |  |\n|  |  |  |"
	for _, tt := range []struct {
		name, before, after string
		cmd                 *contract.EditCommand
		in                  *contract.Intent
	}{
		{"insert a table on an empty line", "x\n\n‸\n\ny", "x\n\n" + newTable + "\n\ny", insertTable, nil},
		{"insert a table after a paragraph", "ab‸\n\ny", "ab\n\n" + newTable + "\n\ny", insertTable, nil},
		{"table commands outside a table", "ab‸", "", cmd(contract.CmdTableRowBelow), nil},
		{"insert a footnote", "a‸ b", "a[^1] b\n\n[^1]: ‸", cmd(contract.CmdInsertFootnote), nil},
		{"insert a footnote keeps the final newline", "a‸\n", "a[^1]\n\n[^1]: ‸\n", cmd(contract.CmdInsertFootnote), nil},
		{"footnote labels count up", "a[^1]‸\n\n[^1]: x", "a[^1][^2]\n\n[^1]: x\n\n[^2]: ‸", cmd(contract.CmdInsertFootnote), nil},
		{"insert images", "x‸", "x![a](assets/a.png)\n![b c](b%20c.png)‸",
			&contract.EditCommand{ID: contract.CmdInsertImages, Paths: []string{"assets/a.png", "b%20c.png"}}, nil},
		// Paste.
		{"a URL over a selection makes a link", "‸word‸", "[word](https://a.b/c)‸", nil,
			&contract.Intent{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: "https://a.b/c"}}},
		{"a URL at the caret is text", "x‸", "xhttps://a.b/c‸", nil,
			&contract.Intent{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: "https://a.b/c"}}},
		{"Markdown is preferred", "x‸", "x**a**‸", nil,
			&contract.Intent{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: "a", Markdown: "**a**"}}},
		{"paste as plain text", "x‸", "xa‸", nil,
			&contract.Intent{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: "a", Markdown: "**a**", AsPlain: true}}},
		{"code blocks take plain text", "```\n‸\n```", "```\n<b>‸\n```", nil,
			&contract.Intent{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: "<b>", Markdown: "**x**"}}},
		{"a URL over a selection in code is text", "```\n‸word‸\n```", "```\nhttps://a.b‸\n```", nil,
			&contract.Intent{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: "https://a.b"}}},
		{"pasted lines take the document's line ending", "x‸", "xa\nb‸", nil,
			&contract.Intent{Kind: contract.IntentPaste, Paste: &contract.PastePayload{Plain: "a\r\nb"}}},
	} {
		check(t, tt.name, tt.before, tt.after, tt.cmd, tt.in, false)
	}
}

// TestPasteConverter checks the shapes BE-1.6 converts. Real samples are
// golden tests next to the implementation (testdata/paste); these cases pin
// the rules. Lines are compared without trailing spaces, and runs of blank
// lines count as one.
func TestPasteConverter(t *testing.T, c contract.PasteConverter) {
	norm := func(s string) string {
		s = strings.ReplaceAll(s, "\r\n", "\n")
		lines := strings.Split(s, "\n")
		for i, l := range lines {
			lines[i] = strings.TrimRight(l, " \t")
		}
		s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
		return strings.Trim(s, "\n")
	}
	for _, tt := range []struct{ name, html, want string }{
		{"inline", `<p>a <b>b</b> <strong>c</strong> <i>d</i> <em>e</em> <code>f</code> <s>g</s></p>`, "a **b** **c** *d* *e* `f` ~~g~~"},
		{"headings", `<h1>T</h1><h3>S</h3><p>p</p>`, "# T\n\n### S\n\np"},
		{"link and image", `<p><a href="https://x.y/a b">x</a> <img src="https://x.y/i.png" alt="i"></p>`, "[x](https://x.y/a%20b) ![i](https://x.y/i.png)"},
		{"data images are dropped", `<p>a<img src="data:image/png;base64,AAAA">b</p>`, "ab"},
		{"lists", `<ul><li>a</li><li>b<ul><li>c</li></ul></li></ul><ol><li>d</li><li>e</li></ol>`, "- a\n- b\n  - c\n\n1. d\n2. e"},
		{"quote", `<blockquote><p>q</p></blockquote>`, "> q"},
		{"code block", `<pre><code class="language-go">x := 1
y := 2</code></pre>`, "```go\nx := 1\ny := 2\n```"},
		{"table", `<table><thead><tr><th>a</th><th>b</th></tr></thead><tbody><tr><td>1</td><td>x|y</td></tr></tbody></table>`,
			"| a | b |\n| --- | --- |\n| 1 | x\\|y |"},
		{"line break", `<p>a<br>b</p>`, "a\\\nb"},
		{"markdown characters are escaped", `<p>1. not a list *x*</p>`, `1\. not a list \*x\*`},
		{"scripts and styles are dropped", `<style>p{}</style><script>alert(1)</script><p>ok</p>`, "ok"},
		{"Word noise", `<p class=MsoNormal><span style='mso-bidi-font-weight:bold'><b>W</b></span><o:p></o:p></p>`, "**W**"},
		{"fragment markers", "<html><body><!--StartFragment--><p>f</p><!--EndFragment--></body></html>", "f"},
		{"CJK", `<p>中文<b>粗体</b></p>`, "中文**粗体**"},
	} {
		t.Run("html "+tt.name, func(t *testing.T) {
			got, err := c.HTMLToMarkdown(tt.html)
			if err != nil {
				t.Fatal(err)
			}
			if norm(got) != tt.want {
				t.Fatalf("got  %q\nwant %q", norm(got), tt.want)
			}
		})
	}
	// RTF: paragraphs, bold, italic and line breaks (D12).
	for _, tt := range []struct{ name, rtf, want string }{
		{"bold italic", `{\rtf1\ansi{\fonttbl{\f0 Arial;}}\f0 a {\b bold} and {\i italic}\par next\par}`, "a **bold** and *italic*\n\nnext"},
		{"unicode", `{\rtf1\ansi\uc1 \u20013?\u25991?\par}`, "中文"},
		{"line break", `{\rtf1\ansi a\line b\par}`, "a\\\nb"},
	} {
		t.Run("rtf "+tt.name, func(t *testing.T) {
			got, err := c.RTFToMarkdown(tt.rtf)
			if err != nil {
				t.Fatal(err)
			}
			if norm(got) != tt.want {
				t.Fatalf("got  %q\nwant %q", norm(got), tt.want)
			}
		})
	}
}
