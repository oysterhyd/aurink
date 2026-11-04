package contracttest

import (
	"strings"
	"testing"

	"aurink/internal/contract"
)

// TestEditingTypora2 checks commands and keys at block edges against Typora
// 1.0, as the owner measured it (2026-11-04: the same keys in Typora and in
// Aurink, tools/parity). Cases use ‸ for the caret, two of them for a
// selection, and run with LF and CRLF. A case whose after is "=" must leave
// the source as it was (or not apply). A case marked src checks the source
// only, not where the caret ends.
func TestEditingTypora2(t *testing.T, f EditingFixture) {
	type tcase struct {
		name, before, after string
		in                  *contract.Intent
		cmd                 *contract.EditCommand
		src                 bool
	}
	cmd := func(id contract.CommandID) *contract.EditCommand { return &contract.EditCommand{ID: id} }
	key := func(k contract.IntentKind) *contract.Intent {
		return &contract.Intent{Kind: k, Gran: contract.GranChar}
	}
	enter, back, del := key(contract.IntentNewline), key(contract.IntentDeleteBackward), key(contract.IntentDeleteForward)
	brk, tab, untab := key(contract.IntentLineBreak), key(contract.IntentIndent), key(contract.IntentOutdent)
	up, down := cmd(contract.CmdBlockHeadingUp), cmd(contract.CmdBlockHeadingDown)

	cases := []tcase{
		// Heading up and down (Ctrl+= / Ctrl+-): a paragraph is below H6.
		{name: "heading up", before: "## ab‸", after: "# ab‸", cmd: up},
		{name: "heading up from a paragraph", before: "ab‸", after: "###### ab‸", cmd: up},
		{name: "heading up stops at H1", before: "# ab‸", after: "=", cmd: up},
		{name: "heading down", before: "## ab‸", after: "### ab‸", cmd: down},
		{name: "heading down from H6", before: "###### ab‸", after: "ab‸", cmd: down},
		{name: "heading down stops at a paragraph", before: "ab‸", after: "=", cmd: down},
		{name: "the same heading level again", before: "## ab‸", after: "ab‸", cmd: &contract.EditCommand{ID: contract.CmdBlockHeading, Level: 2}},

		// List and quote commands toggle and switch.
		{name: "bullet list off", before: "- ab‸", after: "ab‸", cmd: cmd(contract.CmdBlockBulletList)},
		{name: "ordered list off", before: "1. ab‸", after: "ab‸", cmd: cmd(contract.CmdBlockOrderedList)},
		{name: "bullet to ordered", before: "- ab‸", after: "1. ab‸", cmd: cmd(contract.CmdBlockOrderedList)},
		{name: "ordered to bullet", before: "1. ab‸", after: "- ab‸", cmd: cmd(contract.CmdBlockBulletList)},
		{name: "two paragraphs to one list", before: "‸ab\n\ncd‸", after: "- ab\n\n- cd", cmd: cmd(contract.CmdBlockBulletList), src: true},
		{name: "two paragraphs to one quote", before: "‸ab\n\ncd‸", after: "> ab\n>\n> cd", cmd: cmd(contract.CmdBlockQuote), src: true},

		// Inline formats with no selection take the word at the caret.
		{name: "strong in a word", before: "a‸b cd", after: "**ab** cd", cmd: cmd(contract.CmdFormatStrong), src: true},
		{name: "strong at a word's end keeps the caret inside", before: "ab‸", after: "**ab‸**", cmd: cmd(contract.CmdFormatStrong)},
		{name: "emphasis in a word", before: "a‸b cd", after: "*ab* cd", cmd: cmd(contract.CmdFormatEmphasis), src: true},
		{name: "code in a word", before: "a‸b cd", after: "`ab` cd", cmd: cmd(contract.CmdFormatCode), src: true},
		{name: "link in a word", before: "a‸b cd", after: "[ab]() cd", cmd: cmd(contract.CmdFormatLink), src: true},
		{name: "strong at a word's start is an empty pair", before: "ab ‸cd", after: "ab **‸**cd", cmd: cmd(contract.CmdFormatStrong)},

		// Code and math blocks after a paragraph, the caret inside.
		{name: "code block after a paragraph", before: "ab‸", after: "ab\n\n```\n‸\n```", cmd: cmd(contract.CmdBlockCode)},
		{name: "math block after a paragraph", before: "ab‸", after: "ab\n$$\n‸\n$$", cmd: cmd(contract.CmdBlockMath)},

		// Backspace at the start of a block.
		{name: "Backspace merges paragraphs", before: "ab\n\n‸cd", after: "ab‸cd", in: back},
		{name: "Backspace merges into a heading", before: "# ab\n\n‸cd", after: "# ab‸cd", in: back},
		{name: "Backspace deletes a rule before", before: "ab\n\n---\n\n‸cd", after: "ab\n\n‸cd", in: back},
		{name: "Backspace joins the code before", before: "ab\n\n```\nx\n```\n\n‸cd", after: "ab\n\n```\nx‸cd\n```", in: back},
		{name: "Backspace on the second item", before: "- ab\n- ‸cd", after: "- ab\n\n  ‸cd", in: back},
		{name: "Backspace on the second ordered item", before: "1. ab\n2. ‸cd", after: "1. ab\n\n   ‸cd", in: back},
		{name: "Backspace on a nested item", before: "- a\n  - ‸b", after: "- a\n  \n  ‸b", in: back},
		{name: "Backspace on an empty item", before: "- ab\n- ‸", after: "- ab\n\n  ‸", in: back},
		{name: "Backspace merges paragraphs in a quote", before: "> ab\n>\n> ‸cd", after: "> ab‸cd", in: back},
		{name: "Backspace at the start of code", before: "```\n‸ab\n```", after: "‸ab", in: back},
		{name: "Backspace after a closing mark", before: "**ab**‸", after: "**ab*‸", in: back},

		// Delete at the end of a block.
		{name: "Delete merges paragraphs", before: "ab‸\n\ncd", after: "ab‸cd", in: del},
		{name: "Delete merges items", before: "- ab‸\n- cd", after: "- ab‸cd", in: del},
		{name: "Delete takes a heading's text", before: "ab‸\n\n# cd", after: "ab‸cd", in: del},

		// Enter.
		{name: "Enter at a heading's start", before: "# ‸ab", after: "\n\n# ‸ab", in: enter},
		{name: "Enter in a heading", before: "# a‸b", after: "# a\n\n# ‸b", in: enter},
		{name: "Enter on an empty nested item", before: "- a\n  - b\n  - ‸", after: "- a\n  - b\n  \n  ‸", in: enter},
		{name: "Shift+Enter", before: "ab‸", after: "ab\n‸", in: brk},
		{name: "Shift+Enter in an item", before: "- ab‸", after: "- ab\n  ‸", in: brk},

		// Tab and Shift+Tab.
		{name: "Tab in a paragraph", before: "ab‸", after: "ab\t‸", in: tab},
		{name: "Tab in code", before: "```\nab‸\n```", after: "```\nab\t‸\n```", in: tab},
		{name: "Tab on the first item", before: "- ab‸", after: "- ab\t‸", in: tab},
		{name: "Shift+Tab on a top-level item", before: "- ab‸", after: "ab‸", in: untab},
	}
	for _, tt := range cases {
		for _, dominant := range []string{"\n", "\r\n"} {
			name := tt.name
			if dominant != "\n" {
				name += " CRLF"
			}
			t.Run(name, func(t *testing.T) {
				src, sel := markedWith(strings.ReplaceAll(tt.before, "\n", dominant))
				ctx := f.Context(src, sel, dominant)
				var tx contract.Transaction
				var ok bool
				if tt.cmd != nil {
					var err error
					tx, ok, err = f.Editing.Run(ctx, *tt.cmd)
					if err != nil {
						t.Fatalf("error %v", err)
					}
				} else {
					tx, ok = f.Editing.HandleIntent(ctx, *tt.in)
				}
				before := strings.ReplaceAll(tt.before, "\n", dominant)
				if tt.after == "=" {
					if ok {
						out, caret := applied(t, src, sel, tx)
						if out != src {
							t.Fatalf("%q changed to %q", before, showWith(out, caret))
						}
					}
					return
				}
				if !ok {
					t.Fatal("not applied")
				}
				out, caret := applied(t, src, sel, tx)
				want := strings.ReplaceAll(tt.after, "\n", dominant)
				got := showWith(out, caret)
				if tt.src {
					got = out
				}
				if got != want {
					t.Fatalf("%q:\n got %q\nwant %q", before, got, want)
				}
			})
		}
	}
}
