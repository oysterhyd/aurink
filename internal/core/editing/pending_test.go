package editing

import (
	"fmt"
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/projection"
)

func TestPendingMarkerIntents(t *testing.T) {
	var markers []string
	for level := 1; level <= 6; level++ {
		markers = append(markers, strings.Repeat("#", level))
	}
	markers = append(markers, "+", "-", "*", "1.", "1)", "123.", "987654321)", ">")
	for _, marker := range markers {
		for _, nl := range []string{"\n", "\r\n"} {
			t.Run(fmt.Sprintf("%q/%q", marker, nl), func(t *testing.T) {
				for _, tc := range []struct {
					name   string
					intent contract.Intent
					want   string
				}{
					{"enter", contract.Intent{Kind: contract.IntentNewline}, marker + nl + nl},
					{"backspace", contract.Intent{Kind: contract.IntentDeleteBackward}, marker[:len(marker)-1]},
					{"space", contract.Intent{Kind: contract.IntentInsertText, Text: " "}, marker + " "},
				} {
					t.Run(tc.name, func(t *testing.T) {
						ctx := contextFor(t, marker, contract.Caret(len(marker)), nl)
						tx, ok := New().HandleIntent(ctx, tc.intent)
						if !ok {
							t.Fatal("intent not handled")
						}
						out, selection := apply(t, ctx, tx)
						if out != tc.want || selection.Primary().Head != len(out) {
							t.Fatalf("got %q, selection %+v; want %q", out, selection, tc.want)
						}
						if tc.name == "space" {
							next := contextFor(t, out, selection, nl)
							if follow, ok := New().FollowUp(next); ok {
								t.Fatalf("space triggered another edit: %+v", follow)
							}
							leaf := next.AST.LeafAt(0)
							p := projection.New().Project(next.AST, next.Text, leaf, contract.Reveal{Caret: len(out)})
							if p.Kind == contract.BlockParagraph {
								t.Fatalf("space did not complete the marker: %+v", p)
							}
						}
					})
				}
			})
		}
	}
}

func TestPendingMarkerNestedIntents(t *testing.T) {
	for _, tc := range []struct {
		before, after string
		intent        contract.IntentKind
	}{
		{"> +¦", "> +\n> ¦", contract.IntentNewline},
		{"> >¦", "> >\n> ¦", contract.IntentNewline},
		{"> #¦", "> #\n> ¦", contract.IntentNewline},
		{"- a\n+¦", "- a\n+\n\n¦", contract.IntentNewline},
		{"- a\n\n  +¦", "- a\n\n  +\n- ¦", contract.IntentNewline},
		{"> - +¦", "> - +\n> - ¦", contract.IntentNewline},
		{"> 123)¦", "> 123¦", contract.IntentDeleteBackward},
		{"  ##¦", "  #¦", contract.IntentDeleteBackward},
		{"123¦)", "12¦)", contract.IntentDeleteBackward},
	} {
		for _, nl := range []string{"\n", "\r\n"} {
			t.Run(fmt.Sprintf("%q/%v/%q", tc.before, tc.intent, nl), func(t *testing.T) {
				src, sel := marked(strings.ReplaceAll(tc.before, "\n", nl))
				ctx := contextFor(t, src, sel, nl)
				tx, ok := New().HandleIntent(ctx, contract.Intent{Kind: tc.intent})
				if !ok {
					t.Fatal("intent not handled")
				}
				out, after := apply(t, ctx, tx)
				if got, want := show(out, after), strings.ReplaceAll(tc.after, "\n", nl); got != want {
					t.Fatalf("got %q, want %q, edits %+v", got, want, tx.Edits)
				}
			})
		}
	}
}
