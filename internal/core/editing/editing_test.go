package editing

import (
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
	"aurink/internal/core/markdown"
	"aurink/internal/core/text"
)

func contextFor(t testing.TB, src string, sel contract.Selection, dominant string) contract.EditContext {
	t.Helper()
	p, err := markdown.New(src, 1, contract.ExtDefault)
	if err != nil {
		t.Fatal(err)
	}
	return contract.EditContext{Text: text.New(src, 1), AST: p.Snapshot(), Selection: sel,
		Meta:  contract.DocMeta{Dominant: dominant},
		Prefs: contract.EditPrefs{BulletMarker: '-', StrongDelim: "**", EmphasisDelim: "*", FenceDelim: "```", AutoPair: true, TabSize: 2, Exts: contract.ExtDefault}}
}

func TestContract(t *testing.T) {
	contracttest.TestEditing(t, contracttest.EditingFixture{Editing: New(), Context: func(src string, sel contract.Selection, dominant string) contract.EditContext {
		return contextFor(t, src, sel, dominant)
	}})
}

func marked(s string) (string, contract.Selection) {
	first := strings.Index(s, "¦")
	s = strings.Replace(s, "¦", "", 1)
	last := strings.Index(s, "¦")
	if last < 0 {
		return s, contract.Caret(first)
	}
	s = strings.Replace(s, "¦", "", 1)
	return s, contract.Selection{Ranges: []contract.SelRange{{Anchor: first, Head: last}}}
}

func apply(t testing.TB, ctx contract.EditContext, tx contract.Transaction) (string, contract.Selection) {
	t.Helper()
	src := ctx.Text.String()
	var out strings.Builder
	at := 0
	for _, e := range tx.Edits {
		if e.Range.Start < at || e.Range.End < e.Range.Start || e.Range.End > len(src) {
			t.Fatalf("illegal transaction on %q: %+v", src, tx)
		}
		out.WriteString(src[at:e.Range.Start])
		out.WriteString(e.Text)
		at = e.Range.End
	}
	out.WriteString(src[at:])
	next := out.String()
	// Each untouched span must retain its exact original bytes.
	oldAt, newAt := 0, 0
	for _, e := range tx.Edits {
		n := e.Range.Start - oldAt
		if next[newAt:newAt+n] != src[oldAt:e.Range.Start] {
			t.Fatal("untouched bytes changed")
		}
		oldAt, newAt = e.Range.End, newAt+n+len(e.Text)
	}
	if next[newAt:] != src[oldAt:] {
		t.Fatal("untouched suffix changed")
	}
	sel := mapped(ctx.Selection, tx.Edits)
	if tx.Selection != nil {
		sel = *tx.Selection
	}
	r := sel.Primary().Range()
	if r.Start < 0 || r.End > len(next) {
		t.Fatalf("illegal resulting selection %+v on %q: %+v", sel, next, tx)
	}
	return next, sel
}

func show(s string, sel contract.Selection) string {
	r := sel.Primary().Range()
	if r.Empty() {
		return s[:r.Start] + "¦" + s[r.Start:]
	}
	return s[:r.Start] + "¦" + s[r.Start:r.End] + "¦" + s[r.End:]
}
