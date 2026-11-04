package contracttest

import (
	"strings"
	"testing"

	"aurink/internal/contract"
)

// EditingFixture builds contexts for the editing suite.
type EditingFixture struct {
	Editing contract.Editing
	// Context returns a context for src with the given selection and default prefs.
	Context func(src string, sel contract.Selection, dominant string) contract.EditContext
}

// marked splits a case written with "|" for the caret, or two "|" for a selection.
func marked(s string) (string, contract.Selection) {
	first := strings.Index(s, "|")
	src := strings.Replace(s, "|", "", 1)
	second := strings.Index(src, "|")
	if second < 0 {
		return src, contract.Caret(first)
	}
	src = strings.Replace(src, "|", "", 1)
	return src, contract.Selection{Ranges: []contract.SelRange{{Anchor: first, Head: second}}}
}

// applied returns the source after tx, and the caret of tx's selection.
func applied(t *testing.T, src string, sel contract.Selection, tx contract.Transaction) (string, contract.SelRange) {
	t.Helper()
	var b strings.Builder
	at := 0
	for _, e := range tx.Edits {
		if e.Range.Start < at || e.Range.End < e.Range.Start || e.Range.End > len(src) {
			t.Fatalf("source %q: illegal edits %+v", src, tx.Edits)
		}
		b.WriteString(src[at:e.Range.Start])
		b.WriteString(e.Text)
		at = e.Range.End
	}
	b.WriteString(src[at:])
	if tx.Selection != nil {
		return b.String(), tx.Selection.Primary()
	}
	// The rule of docs/04 §4.1.3 for a caret.
	caret := sel.Primary().Head
	shift := 0
	for _, e := range tx.Edits {
		if e.Range.End <= caret {
			shift += len(e.Text) - e.Range.Len()
		}
	}
	return b.String(), contract.SelRange{Anchor: caret + shift, Head: caret + shift}
}

func show(s string, r contract.SelRange) string {
	if r.Empty() {
		return s[:r.Head] + "|" + s[r.Head:]
	}
	a := r.Range()
	return s[:a.Start] + "|" + s[a.Start:a.End] + "|" + s[a.End:]
}

// TestEditing checks docs/04 §4.4 and the rules of docs/03 §4.5.
func TestEditing(t *testing.T, f EditingFixture) {
	ed := f.Editing
	newline := contract.Intent{Kind: contract.IntentNewline}
	back := contract.Intent{Kind: contract.IntentDeleteBackward}
	insert := func(s string) contract.Intent { return contract.Intent{Kind: contract.IntentInsertText, Text: s} }

	for _, tt := range []struct {
		name, before string
		in           contract.Intent
		after        string
	}{
		{"typing", "a|b", insert("中"), "a中|b"},
		{"typing replaces a selection", "|ab|c", insert("x"), "x|c"},
		{"paragraph Enter", "a|", newline, "a\n\n|"},
		{"paragraph Enter in the middle", "a|b", newline, "a\n\n|b"},
		{"hard break: a plain line break, as Typora writes it", "a|", contract.Intent{Kind: contract.IntentLineBreak}, "a\n|"},
		{"heading Enter", "# a|", newline, "# a\n\n|"},
		{"bullet continues", "- a|", newline, "- a\n- |"},
		{"bullet keeps its marker", "* a|", newline, "* a\n* |"},
		{"ordered increments", "1. a|", newline, "1. a\n2. |"},
		{"ordered keeps its delimiter", "3) a|", newline, "3) a\n4) |"},
		{"task continues open", "- [x] a|", newline, "- [x] a\n- [ ] |"},
		{"nested list keeps its indent", "- a\n  - b|", newline, "- a\n  - b\n  - |"},
		{"CJK list item", "- 中文|", newline, "- 中文\n- |"},
		{"empty item leaves the list", "- a\n- |", newline, "- a\n\n|"},
		// A new paragraph in the quote, as Typora writes it (TestEditingTypora).
		{"quote continues", "> a|", newline, "> a\n>\n> |"},
		{"empty quote line leaves the quote", "> a\n>\n> |", newline, "> a\n\n|"},
		{"code keeps its indent", "```\n  a|\n```", newline, "```\n  a\n  |\n```"},
		{"fence opens", "```go|", newline, "```go\n|\n```"},
		{"thematic break", "---|", newline, "---\n\n|"},
		{"backspace is one grapheme", "a👍🏽|", back, "a|"},
		{"backspace removes a selection", "|ab|c", back, "|c"},
		{"backspace at the start of an item", "- |a", back, "|a"},
		{"backspace at the start of a quote", "> |a", back, "|a"},
		{"backspace at the start of a heading", "# |a", back, "|a"},
		{"backspace after a closing mark deletes a mark character, as Typora", "**a**|", back, "**a*|"},
		{"delete forward", "|ab", contract.Intent{Kind: contract.IntentDeleteForward}, "|b"},
		{"word delete", "foo bar|", contract.Intent{Kind: contract.IntentDeleteBackward, Gran: contract.GranWord}, "foo |"},
		{"auto pair", "a|", insert("("), "a(|)"},
		{"typing over the closer", "(a|)", insert(")"), "(a)|"},
		{"wrapping a selection", "|a|", insert("*"), "*|a|*"},
		{"indent a list item", "- a\n- b|", contract.Intent{Kind: contract.IntentIndent}, "- a\n  - b|"},
		{"outdent a list item", "- a\n  - b|", contract.Intent{Kind: contract.IntentOutdent}, "- a\n- b|"},
	} {
		for _, dominant := range []string{"\n", "\r\n"} {
			name := tt.name
			if dominant != "\n" {
				name += " CRLF"
			}
			t.Run(name, func(t *testing.T) {
				before, after := strings.ReplaceAll(tt.before, "\n", dominant), strings.ReplaceAll(tt.after, "\n", dominant)
				src, sel := marked(before)
				ctx := f.Context(src, sel, dominant)
				tx, ok := ed.HandleIntent(ctx, tt.in)
				if !ok {
					t.Fatal("not handled")
				}
				out, caret := applied(t, src, sel, tx)
				if got := show(out, caret); got != after {
					t.Errorf("%q: got %q, want %q", before, got, after)
				}
				again, _ := ed.HandleIntent(f.Context(src, sel, dominant), tt.in)
				if out2, _ := applied(t, src, sel, again); out2 != out {
					t.Errorf("not pure: %q then %q", out, out2)
				}
			})
		}
	}

	t.Run("edits are minimal", func(t *testing.T) {
		src := "# 头\n\n1.  a\n3.  b\n\n|x|y|\n|-|-|\n"
		at := strings.Index(src, "a") + 1
		tx, ok := ed.HandleIntent(f.Context(src, contract.Caret(at), "\n"), newline)
		if !ok {
			t.Fatal("not handled")
		}
		out, _ := applied(t, src, contract.Caret(at), tx)
		if !strings.HasPrefix(out, src[:at]) || !strings.HasSuffix(out, src[at:]) {
			t.Errorf("bytes outside the edit changed: %q", out)
		}
	})

	t.Run("input rules are a separate step", func(t *testing.T) {
		src := "[ ] |"
		text, sel := marked(src)
		tx, ok := ed.FollowUp(f.Context(text, sel, "\n"))
		if !ok {
			t.Fatal("no rule for a task marker")
		}
		if tx.Origin != contract.OriginInputRule {
			t.Errorf("origin %d", tx.Origin)
		}
		out, caret := applied(t, text, sel, tx)
		if got := show(out, caret); got != "- [ ] |" {
			t.Errorf("got %q", got)
		}
		if _, ok := ed.FollowUp(f.Context("plain ", contract.Caret(6), "\n")); ok {
			t.Error("a rule on plain text")
		}
	})

	for _, tt := range []struct {
		name, before string
		cmd          contract.EditCommand
		after        string
	}{
		{"strong on", "|a|", contract.EditCommand{ID: contract.CmdFormatStrong}, "**|a|**"},
		{"strong off", "**|a|**", contract.EditCommand{ID: contract.CmdFormatStrong}, "|a|"},
		{"emphasis on", "|a|", contract.EditCommand{ID: contract.CmdFormatEmphasis}, "*|a|*"},
		{"strike on", "|a|", contract.EditCommand{ID: contract.CmdFormatStrike}, "~~|a|~~"},
		{"code on", "|a|", contract.EditCommand{ID: contract.CmdFormatCode}, "`|a|`"},
		{"strong at a caret inserts a pair", "a |b", contract.EditCommand{ID: contract.CmdFormatStrong}, "a **|**b"},
		{"heading", "a|", contract.EditCommand{ID: contract.CmdBlockHeading, Level: 2}, "## a|"},
		{"heading changes level", "# a|", contract.EditCommand{ID: contract.CmdBlockHeading, Level: 3}, "### a|"},
		{"paragraph", "## a|", contract.EditCommand{ID: contract.CmdBlockParagraph}, "a|"},
		{"quote", "a|", contract.EditCommand{ID: contract.CmdBlockQuote}, "> a|"},
		{"bullet list", "a|", contract.EditCommand{ID: contract.CmdBlockBulletList}, "- a|"},
		{"ordered list", "a|", contract.EditCommand{ID: contract.CmdBlockOrderedList}, "1. a|"},
		{"task list", "a|", contract.EditCommand{ID: contract.CmdBlockTaskList}, "- [ ] a|"},
		{"move down", "a|\n\nb\n", contract.EditCommand{ID: contract.CmdBlockMoveDown}, "b\n\na|\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			src, sel := marked(tt.before)
			tx, ok, err := ed.Run(f.Context(src, sel, "\n"), tt.cmd)
			if err != nil || !ok {
				t.Fatalf("ok %v, error %v", ok, err)
			}
			out, caret := applied(t, src, sel, tx)
			if got := show(out, caret); got != tt.after {
				t.Errorf("%q: got %q, want %q", tt.before, got, tt.after)
			}
		})
	}

	// Toggling a task changes exactly one byte.
	t.Run("toggle task", func(t *testing.T) {
		src := "- [ ] a\n"
		tx, ok, err := ed.Run(f.Context(src, contract.Caret(6), "\n"), contract.EditCommand{ID: contract.CmdBlockToggleTask})
		if err != nil || !ok {
			t.Fatalf("ok %v, error %v", ok, err)
		}
		if len(tx.Edits) != 1 || tx.Edits[0].Range.Len() != 1 || tx.Edits[0].Text != "x" {
			t.Errorf("edits %+v", tx.Edits)
		}
	})

	t.Run("Formatting", func(t *testing.T) {
		src, sel := marked("## **|a|** b")
		got := ed.Formatting(f.Context(src, sel, "\n"))
		if got.Strong != contract.On || got.Block != contract.BlockHeading || got.Level != 2 {
			t.Errorf("state %+v", got)
		}
		src, sel = marked("**a** |b|")
		if got := ed.Formatting(f.Context(src, sel, "\n")); got.Strong != contract.Off {
			t.Errorf("state %+v", got)
		}
		src, sel = marked("|**a** b|")
		if got := ed.Formatting(f.Context(src, sel, "\n")); got.Strong != contract.Mixed {
			t.Errorf("state %+v", got)
		}
		src, sel = marked("- [ ] a|")
		if got := ed.Formatting(f.Context(src, sel, "\n")); got.List != contract.ListTask {
			t.Errorf("state %+v", got)
		}
		src, sel = marked("```\na|\n```")
		if got := ed.Formatting(f.Context(src, sel, "\n")); !got.InCode {
			t.Errorf("state %+v", got)
		}
	})
}
