package markdown

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/tests/corpus"
)

type windowEdit struct {
	name string
	at   int
	text string
}

func corpusEdits(t testing.TB, src, eol string) []windowEdit {
	t.Helper()
	find := func(s string) int {
		at := strings.Index(src, s)
		if at < 0 {
			t.Fatalf("missing corpus target %q", s)
		}
		return at
	}
	para := find("English")
	item := find("- 项 ")
	return []windowEdit{
		{"paragraph_middle", para + 3, "a"},
		{"paragraph_first", find("段落"), "a"},
		{"code_typing", find("func f") + 2, "a"},
		{"code_newline", find("func f") + 2, eol},
		{"paragraph_enter", para + 3, eol},
		{"list_typing", item + len("- 项"), "a"},
		{"list_enter", item + len("- 项"), eol},
		{"list_marker", item, "x"},
		{"quote_start", find("> 引用"), "x"},
		{"heading_start", find(eol+"#") + len(eol), "x"},
		{"table_cell", find("*x*") + 2, "a"},
		{"paragraph_pipe", para + 3, "|"},
		{"table_enter", find("| 甲 | b |"), eol},
	}
}

// Each realistic edit, including its inverse, adapts at most 32 blocks at
// either size. This counts lookahead and unsuccessful window attempts too.
func TestCorpusWindowWork(t *testing.T) {
	work := map[string]int{}
	for _, eol := range []string{"\n", "\r\n"} {
		for _, size := range []int{200 << 10, 1 << 20} {
			src := corpus.Generate(size, 1331)
			if eol != "\n" {
				src = strings.ReplaceAll(src, "\n", eol)
			}
			for _, c := range corpusEdits(t, src, eol) {
				t.Run(fmt.Sprintf("%d/%d/%s", len(eol), size>>10, c.name), func(t *testing.T) {
					p := mustNew(t, src, 1, contract.ExtFrontMatter)
					for direction, ed := range []contract.TextEdit{
						{Range: contract.Range{Start: c.at, End: c.at}, Text: c.text},
						{Range: contract.Range{Start: c.at, End: c.at + len(c.text)}},
					} {
						old := p.Snapshot()
						u, err := p.Apply([]contract.TextEdit{ed})
						if err != nil {
							t.Fatal(err)
						}
						if u.Stats.AdaptedBlocks > 32 || u.Stats.FullRebuilds != 0 {
							t.Fatalf("work: %+v", u.Stats)
						}
						key := fmt.Sprintf("%d/%s/%d", len(eol), c.name, direction)
						if size == 200<<10 {
							work[key] = u.Stats.AdaptedBlocks
						} else if work[key] != u.Stats.AdaptedBlocks {
							t.Fatalf("work grew: %d -> %d", work[key], u.Stats.AdaptedBlocks)
						}
						full := mustNew(t, u.Snapshot.text.String(), u.Snapshot.Version(), old.exts).Snapshot()
						if err := equivalent(u.Snapshot, full); err != nil {
							t.Fatal(err)
						}
						checkDirty(t, old, u)
						t.Logf("adapted=%d local=%d", u.Stats.AdaptedBlocks, u.Stats.LocalParses)
					}
				})
			}
		}
	}
}

func TestConservativePaths(t *testing.T) {
	body := corpus.Generate(200<<10, 1331)
	cases := []struct {
		name, src string
		ed        contract.TextEdit
		reason    string
	}{
		{"swallow", body, contract.TextEdit{Text: "````\n"}, "boundary"},
		{"release", "````\n" + body, contract.TextEdit{Range: contract.Range{End: 5}}, "limit"},
		{"front", "---\ntitle: old\n---\n" + body, contract.TextEdit{Range: contract.Range{Start: 11, End: 14}, Text: "new"}, "front"},
		{"caret", "[^x]:[^x]\n\r[\n^x]: 注*", contract.TextEdit{Range: contract.Range{Start: 16, End: 22}, Text: "\t"}, "reference"},
		{"large_paste", body, contract.TextEdit{Range: contract.Range{Start: 100, End: 100}, Text: "\n" + strings.Repeat("x", maxWindowBytes)}, "limit"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := mustNew(t, c.src, 1, contract.ExtFrontMatter)
			old := p.Snapshot()
			u, err := p.Apply([]contract.TextEdit{c.ed})
			if err != nil {
				t.Fatal(err)
			}
			reasons := map[string]int{"boundary": u.Stats.BoundaryFallbacks, "limit": u.Stats.WindowLimitFallbacks, "front": u.Stats.FrontMatterFallbacks, "reference": u.Stats.ReferenceFallbacks}
			if u.Stats.FullRebuilds != 1 || reasons[c.reason] != 1 {
				t.Fatalf("fallback: %+v", u.Stats)
			}
			if err := equivalent(u.Snapshot, mustNew(t, u.Snapshot.text.String(), 2, old.exts).Snapshot()); err != nil {
				t.Fatal(err)
			}
			checkDirty(t, old, u)
		})
	}
}

func TestWindowGrowth(t *testing.T) {
	src := "# before\n\n" + strings.Repeat("paragraph words\n\n", 12) + "-->\n\n" + strings.Repeat("after words\n\n", 50)
	p := mustNew(t, src, 1, 0)
	u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: 10, End: 10}, Text: "<!--\n"}})
	if err != nil {
		t.Fatal(err)
	}
	if u.Stats.FullRebuilds != 0 || u.Stats.LocalParses < 2 {
		t.Fatalf("growth: %+v", u.Stats)
	}
	if err := equivalent(u.Snapshot, mustNew(t, u.Snapshot.text.String(), 2, 0).Snapshot()); err != nil {
		t.Fatal(err)
	}
}

func TestWindowKindsAndReferenceContext(t *testing.T) {
	bodies := []string{
		"```go\ncode words\n```\n\n",
		"- item words\n- other item\n\n",
		"> quote words [r]\n> next line\n\n",
		"| a | b |\n|---|---|\n| [r] | words |\n\n",
		"<div>\nhtml words\n</div>\n\n",
		"$$\nmath words\n$$\n\n",
		"[^n]: footnote words [r]\n    continuation\n\n",
		"    code words\n\n",
		"before words\n\t| a |\n|---|\n| b |\n\n",
	}
	for _, eol := range []string{"\n", "\r\n"} {
		for n, body := range bodies {
			t.Run(fmt.Sprintf("%d/%d", len(eol), n), func(t *testing.T) {
				src := "[r]: /first\n\n" + strings.Repeat("before words\n\n", 10) + body + strings.Repeat("after words\n\n", 10) + "[r]: /duplicate\n\nuse [r] [^n]\n"
				if eol != "\n" {
					src = strings.ReplaceAll(src, "\n", eol)
				}
				// Edit the selected kind, with both definitions outside its window.
				prefix := strings.ReplaceAll("[r]: /first\n\n"+strings.Repeat("before words\n\n", 10), "\n", eol)
				at := len(prefix) + strings.Index(strings.ReplaceAll(body, "\n", eol), "words")
				p := mustNew(t, src, 1, 0)
				old := p.Snapshot()
				u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: at, End: at}, Text: eol}})
				if err != nil {
					t.Fatal(err)
				}
				if u.Stats.FullRebuilds != 0 {
					t.Fatalf("not local: %+v", u.Stats)
				}
				if err := equivalent(u.Snapshot, mustNew(t, u.Snapshot.text.String(), 2, 0).Snapshot()); err != nil {
					t.Fatal(err)
				}
				checkDirty(t, old, u)
			})
		}
	}
}

func TestCorpusRandomWork(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(fmt.Sprintf("EOL%d", len(eol)), func(t *testing.T) {
			src := corpus.Generate(200<<10, 1331)
			if eol != "\n" {
				src = strings.ReplaceAll(src, "\n", eol)
			}
			p := mustNew(t, src, 1, contract.ExtFrontMatter)
			rng := rand.New(rand.NewSource(20261101))
			var total Stats
			for step := 0; step < longSteps(25, 500); step++ {
				at := rng.Intn(len(src))
				for at > 0 && !utf8.RuneStart(src[at]) {
					at--
				}
				if at > 0 && src[at] == '\n' && src[at-1] == '\r' {
					at--
				}
				end := at
				insert := "a"
				switch rng.Intn(10) {
				case 0, 1:
					insert = eol
				case 2:
					insert = "中"
				case 3:
					insert = " "
				case 4:
					_, width := utf8.DecodeRuneInString(src[at:])
					end = at + width
					insert = ""
				}
				original := src[at:end]
				for _, ed := range []contract.TextEdit{
					{Range: contract.Range{Start: at, End: end}, Text: insert},
					{Range: contract.Range{Start: at, End: at + len(insert)}, Text: original},
				} {
					old := p.Snapshot()
					u, err := p.Apply([]contract.TextEdit{ed})
					if err != nil {
						t.Fatal(err)
					}
					addStats(&total, u.Stats)
					full := mustNew(t, u.Snapshot.text.String(), u.Snapshot.Version(), old.exts).Snapshot()
					if err := equivalent(u.Snapshot, full); err != nil {
						t.Fatalf("step=%d edit=%+v: %v", step, ed, err)
					}
					checkDirty(t, old, u)
				}
			}
			if total.FullRebuilds != total.ReferenceFallbacks+total.FrontMatterFallbacks+total.BoundaryFallbacks+total.WindowLimitFallbacks {
				t.Fatal("uncounted rebuild", total)
			}
			t.Logf("1000 updates on generated corpus (500 edits and inverses): %+v", total)
		})
	}
}
