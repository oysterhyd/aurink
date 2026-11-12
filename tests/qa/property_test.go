package qa

import (
	"math/rand/v2"
	"testing"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/internal/core/document"
	"aurink/internal/core/editing"
	"aurink/internal/core/projection"
	"aurink/internal/views/liveedit"
	"github.com/egoist/mygo/ui"
)

var propertyInserts = []string{"a", "字", "😀", "\n", "\n\n", "\r\n", "# ", "- ", "> ", "**", "`", "```\n", "| a |\n", "[x](y)", "==h==", "^s^", "~t~", ":smile:", "[[w]]", "$x$", "[TOC]\n\n", "\t", " "}

// boundary returns a random offset on a rune boundary of s.
func boundary(rng *rand.Rand, s string) int {
	at := rng.IntN(len(s) + 1)
	for at > 0 && at < len(s) && !utf8.RuneStart(s[at]) {
		at--
	}
	return at
}

// 06 §8 and 08 §9: three seeds of 10,000 random edits through the real model
// and editor, with all extensions on and all extensions off. The text always
// equals a string model, every frame survives, and undo walks back through the
// same texts as far as its limit of 1000 steps (05 BE-1.4).
func TestRandomEditsTenThousandSteps(t *testing.T) {
	if testing.Short() {
		t.Skip("long")
	}
	const steps, undoSteps = 10000, 1000
	for _, mode := range []struct {
		name string
		exts contract.Extensions
	}{{"extensions_on", contract.ExtDefault}, {"extensions_off", 0}} {
		t.Run(mode.name, func(t *testing.T) {
			for _, seed := range []uint64{1, 2, 3} {
				start := "---\ntitle: 笔记\n---\n\n# 标题\n\n段落 **粗** ==亮== $x$\n\n- 项\n"
				model := start
				m := document.Factory{}.New("doc", start, contract.DocMeta{Dominant: "\n", Encoding: "utf-8"}, mode.exts)
				e := liveedit.New(liveedit.Options{Model: m, Projector: projection.New(), Editing: editing.New()})
				tt := ui.NewTester(func(c *ui.Context) { e.View(c) }, 1000, 700)
				tt.Frame()
				rng := rand.New(rand.NewPCG(seed, 2026))
				history := make([]string, 0, steps+1)
				history = append(history, model)
				for step := 0; step < steps; {
					a := boundary(rng, model)
					z := a
					// Deleting more once the text is long keeps it at a few kilobytes.
					if len(model) > 4000 || rng.IntN(3) == 0 {
						z = boundary(rng, model[a:][:min(len(model)-a, 400)]) + a
					}
					text := ""
					if rng.IntN(4) != 0 {
						text = propertyInserts[rng.IntN(len(propertyInserts))]
					}
					if a == z && text == "" {
						continue
					}
					edit := contract.TextEdit{Range: contract.Range{Start: a, End: z}, Text: text}
					if _, err := m.Apply(contract.Transaction{Edits: []contract.TextEdit{edit}, Origin: contract.OriginCommand}); err != nil {
						t.Fatalf("seed %d step %d: %v", seed, step, err)
					}
					model = model[:a] + text + model[z:]
					history = append(history, model)
					if got := m.Text().String(); got != model {
						t.Fatalf("seed %d step %d: %q, want %q", seed, step, got, model)
					}
					if step%10 == 0 {
						m.SetSelection(contract.Caret(boundary(rng, model)))
						tt.Frame()
					}
					step++
				}
				for back := 1; back <= undoSteps; back++ {
					if !m.Undo() {
						t.Fatalf("seed %d: undo stopped after %d", seed, back-1)
					}
					if got, want := m.Text().String(), history[len(history)-1-back]; got != want {
						t.Fatalf("seed %d undo %d: %q, want %q", seed, back, got, want)
					}
				}
				if m.Undo() {
					t.Fatalf("seed %d: undo went past the limit of %d", seed, undoSteps)
				}
				tt.Frame()
				e.Close()
			}
		})
	}
}
