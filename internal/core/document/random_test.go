package document

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"aurink/internal/contract"
)

type oracleStep struct {
	before, after       string
	beforeSel, afterSel contract.Selection
}

var fragments = []string{"a", "中", "🐈", " ", "\n", "\n\n", "\r\n", "\t", "**", "*", "`", "```\n", "> ", "- ", "1. ", "# ", "| a |\n|---|\n", "[r]: /u\n", "[^x]: y\n", "[r]", "---\n", "$$\n", "<div>\n", "\\*", "&amp;"}

func randomEdits(rng *rand.Rand, source string) []contract.TextEdit {
	count := 1 + rng.Intn(3)
	points := make([]int, 2*count)
	for i := range points {
		points[i] = rng.Intn(len(source) + 1)
		for points[i] > 0 && points[i] < len(source) && !utf8.RuneStart(source[points[i]]) {
			points[i]--
		}
	}
	sort.Ints(points)
	edits := make([]contract.TextEdit, count)
	for i := range edits {
		edits[i] = contract.TextEdit{Range: contract.Range{Start: points[2*i], End: points[2*i+1]}}
		if edits[i].Range.Empty() || rng.Intn(3) != 0 {
			edits[i].Text = fragments[rng.Intn(len(fragments))]
		}
	}
	return edits
}

func oracleApply(source string, sel contract.Selection, edits []contract.TextEdit) (string, contract.Selection) {
	sel = cloneSelection(sel)
	// Build the output prefix up to each original endpoint. Keeping original
	// coordinates distinguishes adjacent replacements from boundary insertions.
	endpoint := func(off int, right bool) int {
		var prefix strings.Builder
		consumed := 0
		for _, e := range edits {
			if off < e.Range.Start || off == e.Range.Start && !right {
				return prefix.Len() + off - consumed
			}
			prefix.WriteString(source[consumed:e.Range.Start])
			if off < e.Range.End {
				if right {
					prefix.WriteString(e.Text)
				}
				return prefix.Len()
			}
			prefix.WriteString(e.Text)
			consumed = e.Range.End
		}
		return prefix.Len() + off - consumed
	}
	for j, r := range sel.Ranges {
		a, h := endpoint(r.Anchor, r.Anchor <= r.Head), endpoint(r.Head, r.Head <= r.Anchor)
		if (r.Anchor < r.Head && a > h) || (r.Anchor > r.Head && a < h) {
			a = max(a, h)
			h = a
		}
		sel.Ranges[j] = contract.SelRange{Anchor: a, Head: h}
	}
	// Apply from the right so every range still addresses the original string.
	for i := len(edits) - 1; i >= 0; i-- {
		e := edits[i]
		source = source[:e.Range.Start] + e.Text + source[e.Range.End:]
	}
	return source, sel
}

func TestRandomHistory(t *testing.T) {
	steps := 750
	if os.Getenv("AURINK_LONG_TESTS") != "" {
		steps = 10000
	}
	for _, seed := range []int64{1331, 20261101, 73} {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))
			source := "# 标题\n\n正文 **粗** [r]\n\n> 引用\n\n- 项\n\n```\n码\n```\n\n[r]: /u\n"
			m := newModel(t, source)
			sel := contract.Caret(0)
			var undo, redo []oracleStep
			undos, redos := 0, 0
			for step := 0; step < steps; step++ {
				version := m.Version()
				beforeText, beforeSel := source, sel
				var applied []contract.TextEdit
				switch n := rng.Intn(100); {
				case n < 20 && len(undo) > 0:
					s := undo[len(undo)-1]
					undo = undo[:len(undo)-1]
					redo = append(redo, s)
					source, sel = s.before, s.beforeSel
					if !m.Undo() {
						t.Fatal("undo stopped", step)
					}
					undos++
				case n < 40 && len(redo) > 0:
					s := redo[len(redo)-1]
					redo = redo[:len(redo)-1]
					undo = append(undo, s)
					source, sel = s.after, s.afterSel
					if !m.Redo() {
						t.Fatal("redo stopped", step)
					}
					redos++
				default:
					if rng.Intn(3) == 0 {
						sel = contract.Selection{Ranges: []contract.SelRange{{Anchor: rng.Intn(len(source) + 1), Head: rng.Intn(len(source) + 1)}, {Anchor: rng.Intn(len(source) + 1), Head: rng.Intn(len(source) + 1)}}}
						m.SetSelection(sel)
					}
					edits := randomEdits(rng, source)
					beforeSel, applied = sel, edits
					next, afterSel := oracleApply(source, sel, edits)
					tx := contract.Transaction{Edits: edits, Origin: contract.OriginCommand}
					if rng.Intn(4) == 0 {
						afterSel = contract.Caret(rng.Intn(len(next) + 1))
						tx.Selection = &afterSel
					}
					undo = append(undo, oracleStep{source, next, cloneSelection(sel), cloneSelection(afterSel)})
					if len(undo) > maxHistorySteps {
						undo = undo[1:]
					}
					redo = nil
					apply(t, m, tx)
					source, sel = next, afterSel
				}
				if m.Text().String() != source || !sameSelection(m.Selection(), sel) || m.Version() != version+1 || m.AST().Version() != m.Version() || m.CanUndo() != (len(undo) > 0) || m.CanRedo() != (len(redo) > 0) {
					t.Fatalf("seed %d step %d: text=%q want=%q selection=%+v want=%+v version=%d/%d before=%q selection=%+v edits=%+v", seed, step, m.Text().String(), source, m.Selection(), sel, m.Version(), version+1, beforeText, beforeSel, applied)
				}
				assertBudget(t, &m.history)
				if step%31 == 0 {
					assertAST(t, m)
				}
			}
			assertAST(t, m)
			t.Logf("seed=%d steps=%d undos=%d redos=%d", seed, steps, undos, redos)
		})
	}
}
