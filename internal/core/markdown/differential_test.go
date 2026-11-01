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

func TestDifferentialLongBlankRuns(t *testing.T) {
	cases := []struct {
		name, marked, insert string
	}{
		{"paragraph", "active words@tail\n\n", "\n\n"},
		{"fence", "```go\nactive words@tail\n```\n\n", "\n\n"},
		{"list_exit", "- active words@tail\n- next item\n\n", "\n\n"},
		{"list_inside", "- active words@tail\n- next item\n\n", "\n  \n  "},
		{"quote_exit", "> active words@tail\n> next line\n\n", "\n\n"},
		{"quote_inside", "> active words@tail\n> next line\n\n", "\n> \n> "},
		{"html_comment", "<!--\nactive words@tail\n-->\n\n", "\n\n"},
		{"html_script", "<script>\nactive words@tail\n</script>\n\n", "\n\n"},
		{"html_div", "<div>\nactive words@tail\n</div>\n\n", "\n\n"},
		{"start", "@active words\n\n", "\n\n"},
		{"end", "active words\n\n@", "\n\n"},
		{"empty", "@", "\n\n"},
	}
	for _, eol := range []string{"\n", "\r\n", "\r"} {
		for _, c := range cases {
			t.Run(fmt.Sprintf("EOL%d-%q/%s", len(eol), eol, c.name), func(t *testing.T) {
				marked := c.marked
				if c.name != "start" && c.name != "empty" {
					marked = strings.Repeat("before words\n\n", 6) + marked
				}
				if c.name != "end" && c.name != "empty" {
					marked += strings.Repeat("after words\n\n", 8)
				}
				marked = strings.ReplaceAll(marked, "\n", eol)
				at := strings.IndexByte(marked, '@')
				src := marked[:at] + marked[at+1:]
				insert := strings.ReplaceAll(c.insert, "\n", eol)
				p := mustNew(t, src, 1, 0)
				for step := 0; step < longSteps(80, 300); step++ {
					old := p.Snapshot()
					u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: at, End: at}, Text: insert}})
					if err != nil {
						t.Fatal(err)
					}
					src = src[:at] + insert + src[at:]
					at += len(insert)
					if u.Stats.FullRebuilds != 0 {
						t.Fatalf("step %d: %+v", step, u.Stats)
					}
					if u.Snapshot.text.String() != src {
						t.Fatal("source differs")
					}
					if err := equivalent(u.Snapshot, mustNew(t, src, u.Snapshot.Version(), 0).Snapshot()); err != nil {
						t.Fatalf("step %d: %v", step, err)
					}
					checkCoverage(t, u.Snapshot)
					checkDirty(t, old, u)
				}
			})
		}
	}
}

func TestDifferentialExistingBlankRun(t *testing.T) {
	for _, line := range []string{"\n", " \t\r\n", "> \n", "  \n"} {
		for _, atLine := range []int{0, 1, 250, 498, 499} {
			t.Run(fmt.Sprintf("%q/%d", line, atLine), func(t *testing.T) {
				prefix := "before words\n\n"
				src := prefix + strings.Repeat(line, 500) + "after words\n\nlast words\n"
				p := mustNew(t, src, 1, 0)
				at := len(prefix) + atLine*len(line)
				for _, insert := range []string{"text", "\n\n", "<!--\n", "-->\n", "[r]: /u\n", "use [r]\n"} {
					old := p.Snapshot()
					u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: at, End: at}, Text: insert}})
					if err != nil {
						t.Fatal(err)
					}
					src = src[:at] + insert + src[at:]
					at += len(insert)
					if err := equivalent(u.Snapshot, mustNew(t, src, u.Snapshot.Version(), 0).Snapshot()); err != nil {
						t.Fatalf("insert %q: %v", insert, err)
					}
					checkCoverage(t, u.Snapshot)
					checkDirty(t, old, u)
				}
			})
		}
	}
}

func TestDifferentialBlankRunContainerState(t *testing.T) {
	for _, container := range []string{"- item words", "[^n]: note words", "    code words"} {
		for _, insert := range []string{"\n", "    continued words\n"} {
			t.Run(container+"/"+insert, func(t *testing.T) {
				prefix := "before words\n\n" + container + "\n\n"
				src := prefix + strings.Repeat("\n", 500) + "after words\n\nlast words\n"
				at := len(prefix) + 250
				if insert == "\n" {
					at = len("before")
				}
				p := mustNew(t, src, 1, 0)
				old := p.Snapshot()
				u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: at, End: at}, Text: insert}})
				if err != nil {
					t.Fatal(err)
				}
				if u.Stats.FullRebuilds != 0 {
					t.Fatalf("fallback: %+v", u.Stats)
				}
				if err := equivalent(u.Snapshot, mustNew(t, src[:at]+insert+src[at:], 2, 0).Snapshot()); err != nil {
					t.Fatal(err)
				}
				checkCoverage(t, u.Snapshot)
				checkDirty(t, old, u)
			})
		}
	}
}
