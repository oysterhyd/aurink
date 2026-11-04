package contracttest

import (
	"strings"
	"testing"

	"aurink/internal/contract"
)

// TestEditingTypora checks the keys against Typora's behaviour, measured on
// Typora 1.0 by the owner (2026-11-04, the user's real-machine check): what
// each key leaves in the source. Cases are written with ‸ for the caret and
// run with LF and CRLF, as they are, with no text put around them.
func TestEditingTypora(t *testing.T, f EditingFixture) {
	run := func(t *testing.T, before, dominant string, in contract.Intent, followUp bool) (string, bool) {
		t.Helper()
		src, sel := markedWith(strings.ReplaceAll(before, "\n", dominant))
		ctx := f.Context(src, sel, dominant)
		var tx contract.Transaction
		var ok bool
		if followUp {
			tx, ok = f.Editing.FollowUp(ctx)
		} else {
			tx, ok = f.Editing.HandleIntent(ctx, in)
		}
		if !ok {
			return "", false
		}
		out, caret := applied(t, src, sel, tx)
		return showWith(out, caret), true
	}
	check := func(t *testing.T, name, before, after string, in contract.Intent, followUp bool) {
		for _, dominant := range []string{"\n", "\r\n"} {
			n := name
			if dominant != "\n" {
				n += " CRLF"
			}
			t.Run(n, func(t *testing.T) {
				got, ok := run(t, before, dominant, in, followUp)
				want := strings.ReplaceAll(after, "\n", dominant)
				switch {
				case after == "" && ok:
					t.Fatalf("applied, giving %q", got)
				case after != "" && !ok:
					t.Fatal("not applied")
				case after != "" && got != want:
					t.Fatalf("%q:\n got %q\nwant %q", before, got, want)
				}
			})
		}
	}
	enter := contract.Intent{Kind: contract.IntentNewline}
	back := contract.Intent{Kind: contract.IntentDeleteBackward, Gran: contract.GranChar}
	del := contract.Intent{Kind: contract.IntentDeleteForward, Gran: contract.GranChar}

	// Empty paragraphs (BE-BLANK). Typora shows a run of n empty lines
	// between blocks as (n-1)/2 empty paragraphs, rounded down: "a\n\n\n\nb"
	// has one, on its middle line, and that is where the caret is after Enter
	// at the end of a. Enter on an empty paragraph opens another below it;
	// Backspace in one deletes it, the caret going to the end of what is
	// before; Delete in one deletes it, the caret going to the start of what
	// is after.
	for _, tt := range []struct {
		name, before, after string
		in                  contract.Intent
	}{
		{"Enter at the end of a paragraph before another", "a‸\n\nb", "a\n\n‸\n\nb", enter},
		{"Enter on an empty paragraph opens another", "a\n\n‸\n\nb", "a\n\n\n\n‸\n\nb", enter},
		{"Enter on an empty paragraph at the end", "a\n\n‸", "a\n\n\n\n‸", enter},
		{"Backspace deletes an empty paragraph", "a\n\n‸\n\nb", "a‸\n\nb", back},
		{"Backspace deletes an empty paragraph at the end", "a\n\n‸", "a‸", back},
		{"Backspace goes to the empty paragraph before", "a\n\n\n\n‸\n\nb", "a\n\n‸\n\nb", back},
		{"Delete deletes an empty paragraph", "a\n\n‸\n\nb", "a\n\n‸b", del},
	} {
		check(t, tt.name, tt.before, tt.after, tt.in, false)
	}

	// Enter in a quote opens a new paragraph in it, as Typora writes it
	// ("> ab\n>\n> z"); Enter on that empty paragraph leaves the quote
	// (BE-ENTER).
	for _, tt := range []struct {
		name, before, after string
		in                  contract.Intent
	}{
		{"Enter in a quote opens a paragraph in it", "> a‸", "> a\n>\n> ‸", enter},
		{"Enter in the middle of a quoted line", "> a‸b", "> a\n>\n> ‸b", enter},
		{"Enter on an empty quoted paragraph leaves the quote", "> a\n>\n> ‸", "> a\n\n‸", enter},
		{"Enter in a quote before more text", "> a‸\n\nz", "> a\n>\n> ‸\n\nz", enter},
	} {
		check(t, tt.name, tt.before, tt.after, tt.in, false)
	}

	// Enter completes a block typed on one line, as in Typora (BE-ENTER):
	// "$$" opens a math block with its closing line; a row of pipes becomes
	// a table with its delimiter row and an empty body row, the caret in its
	// first cell; a fence opener already closed puts the caret inside.
	for _, tt := range []struct {
		name, before, after string
		in                  contract.Intent
	}{
		{"Enter after $$ opens a math block", "a\n\n$$‸", "a\n\n$$\n‸\n$$", enter},
		{"Enter after $$ before more text", "$$‸\n\nz", "$$\n‸\n$$\n\nz", enter},
		{"Enter after a row of pipes makes a table", "|a|b|‸", "| a | b |\n| --- | --- |\n| ‸ |  |", enter},
		{"Enter after a padded row of pipes", "| a | b |‸\n\nz", "| a | b |\n| --- | --- |\n| ‸ |  |\n\nz", enter},
		{"Enter in a closed empty fence", "```js‸\n```\n\nz", "```js\n‸\n```\n\nz", enter},
	} {
		check(t, tt.name, tt.before, tt.after, tt.in, false)
	}

	// Typing the third backtick of a fence on a line of its own, with text
	// after it that the open fence would take into its code, closes the fence
	// at once, so the rest of the document stays as it was while the opener
	// is typed (Typora makes no fence before Enter). At the end of the
	// document there is nothing to take and nothing is added. Backspace over
	// the opener of such an empty fence takes its closing line too.
	for _, tt := range []struct {
		name, before, after string
	}{
		{"the third backtick closes the fence before text", "a\n\n```‸\n\nz", "a\n\n```‸\n```\n\nz"},
		{"a tilde fence too", "~~~‸\n\nz", "~~~‸\n~~~\n\nz"},
		{"nothing to close at the end", "a\n\n```‸", ""},
		{"nothing when a fence already closes it", "```‸\nb\n```", ""},
	} {
		check(t, tt.name, tt.before, tt.after, contract.Intent{}, true)
	}
	check(t, "Backspace over the opener of an empty fence takes its closer", "a\n\n```‸\n```\n\nz", "a\n\n``‸\n\nz", back, false)
}
