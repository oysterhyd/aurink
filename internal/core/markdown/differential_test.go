package markdown

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/tests/corpus"
)

// longSteps returns long when AURINK_LONG_TESTS is set: the full random runs of
// docs/06 batch 2 take minutes, too long for every gate.
func longSteps(short, long int) int {
	if os.Getenv("AURINK_LONG_TESTS") != "" {
		return long
	}
	return short
}

func differentialDump(a contract.AST) string {
	var s strings.Builder
	for i := 0; i < a.Len(); i++ {
		b := *a.Block(i)
		b.ID = 0
		fmt.Fprintf(&s, "%+v\n", b)
		if isLeaf(b.Kind) {
			fmt.Fprintf(&s, "%+v\n", a.Inlines(i))
		}
	}
	return s.String()
}

var differentialFragments = []string{"a", "中", " ", "    ", "\n", "\n\n", "\r\n", "\t", "**", "*", "`", "```\n", "~~~", "> ", "- ", "1. ", "# ", "|", "| a |\n|---|\n", "|---|\n", "[r]: /u\n", "[链接]", "[^x]: y\n", "[^x]", "---\n", "===\n", "$$\n", "<div>\n", "</div>", "<!--", "\\", "&amp;", "  - ", "  "}

// Random edits on large realistic documents, compared with a fresh parse.
func TestDifferentialOnLargeDocuments(t *testing.T) {
	total := Stats{}
	for _, seed := range []int64{1, 2, 3, 4} {
		src := corpus.Generate(30<<10, seed)
		if seed%2 == 0 {
			src = strings.ReplaceAll(src, "\n", "\r\n")
		}
		p := mustNew(t, src, 1, contract.ExtFrontMatter)
		rng := rand.New(rand.NewSource(seed))
		for step := 0; step < longSteps(150, 1200); step++ {
			at := rng.Intn(len(src) + 1)
			for at < len(src) && !utf8.RuneStart(src[at]) {
				at--
			}
			end := at
			if rng.Intn(3) == 0 {
				end = min(len(src), at+rng.Intn(20))
				for end < len(src) && !utf8.RuneStart(src[end]) {
					end++
				}
			}
			txt := ""
			if end == at || rng.Intn(2) == 0 {
				txt = differentialFragments[rng.Intn(len(differentialFragments))]
			}
			if at == end && txt == "" {
				continue
			}
			u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: at, End: end}, Text: txt}})
			if err != nil {
				t.Fatal(err)
			}
			src = src[:at] + txt + src[end:]
			total.LocalParses += u.Stats.LocalParses
			f := mustNew(t, src, 1, contract.ExtFrontMatter)
			if a, b := differentialDump(u.Snapshot), differentialDump(f.Snapshot()); a != b {
				t.Fatalf("seed %d step %d edit %d-%d %q differs\nsrc near %q", seed, step, at, end, txt, src[max(0, at-60):min(len(src), at+60)])
			}
		}
	}
	if total.LocalParses == 0 {
		t.Error("the window path never ran")
	}
}
