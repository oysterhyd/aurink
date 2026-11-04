package markdown

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"aurink/internal/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

type example struct {
	Markdown string `json:"markdown"`
	Example  int    `json:"example"`
	Section  string `json:"section"`
}

func TestParserCorpusMappingsAndCoverage(t *testing.T) {
	var leaves, lines, padded int
	for _, c := range parserCorpus(t) {
		a, err := parseAdapted(c.src, c.ext, false)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for i := 1; i < len(a.blocks); i++ {
			b := a.blocks[i]
			if !isLeaf(b.Kind) {
				continue
			}
			leaves++
			for k, l := range b.Lines {
				want, _ := a.doc.Line(i, k)
				got := strings.Repeat(" ", int(l.Padding)) + c.src[l.Source.Start:l.Source.End]
				if got != want {
					t.Fatalf("%s block=%d line=%d: %q != %q", c.name, i, k, got, want)
				}
				lines++
				if l.Padding > 0 {
					padded++
				}
			}
			if got, want := a.stripped(i, true), a.doc.Lines(i); got != want {
				t.Fatalf("%s block=%d: %q != %q", c.name, i, got, want)
			}
		}
		// The product always uses parser.Default, independently of each spec
		// fixture's syntax switches used for the mapping oracle above.
		s := mustNew(t, c.src, 1, 0).Snapshot()
		checkCoverage(t, s)
		d := parser.Parse(c.src)
		for j := 1; j < d.Len(); j++ {
			p := d.Block(j)
			if p.Kind != parser.Paragraph && p.Kind != parser.Heading && p.Kind != parser.TableCell {
				continue
			}
			found := -1
			for i := range s.entries {
				b := s.Block(i)
				if b.Kind == contract.BlockKind(p.Kind) && b.Range.Start == p.Start {
					found = i
					break
				}
			}
			if found < 0 {
				t.Fatalf("%s missing parser leaf %d", c.name, j)
			}
			var want, got strings.Builder
			for _, n := range d.Inlines(j) {
				want.WriteString(n.Text)
			}
			for _, n := range s.Inlines(found) {
				got.WriteString(n.Text)
			}
			if want.String() != got.String() {
				t.Fatalf("%s block=%d inline text %q != %q", c.name, j, got.String(), want.String())
			}
		}
		for i := 1; i < s.Len(); i++ {
			b := s.Block(i)
			p := s.Block(b.Parent)
			if b.Parent >= i || b.Next <= i || b.Next > p.Next || b.Range.Start < p.Range.Start {
				t.Fatalf("%s block %d topology %+v", c.name, i, b)
			}
		}
	}
	if padded == 0 {
		t.Fatal("partial-tab witness missing")
	}
	t.Logf("1339 samples: %d leaves, %d lines, %d padded lines; mappings and coverage pass", leaves, lines, padded)
}

type sample struct {
	name, src string
	ext       parser.Extensions
}

func parserCorpus(t testing.TB) []sample {
	t.Helper()
	dir, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/egoist/mygo").Output()
	if err != nil {
		t.Fatal(err)
	}
	var samples []sample
	for _, file := range []string{"commonmark.json", "gfm.json"} {
		data, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(dir)), "ui", "markdown", "parser", "testdata", file))
		if err != nil {
			t.Fatal(err)
		}
		var examples []example
		if err := json.Unmarshal(data, &examples); err != nil {
			t.Fatal(err)
		}
		ext := parser.Extensions(0)
		if file == "gfm.json" {
			ext = parser.GFM
		}
		for _, e := range examples {
			samples = append(samples, sample{fmt.Sprintf("%s/%d/%s", file, e.Example, e.Section), e.Markdown, ext})
		}
	}
	for name, src := range extraSamples() {
		samples = append(samples, sample{name, src, parser.Default})
	}
	return samples
}

func extraSamples() map[string]string {
	return map[string]string{
		"CJK":          "# 中文 🐈 **标题**\n\n> 引用繁體\n> 第二行 é\n\n- 日文かな\n  中文 *词* &amp; \\*\n",
		"CRLF":         "## 中文 ##\r\n\r\n> - 一\r\n>   二\r\n\r\n```go\r\n代码\r\n```\r\n",
		"mixed-EOL":    "甲\r\n乙\n丙\r丁\r\n\n> 一\r\n> 二\n> 三\r\n",
		"deep-quote":   strings.Repeat("> ", 96) + "深层\n" + strings.Repeat("> ", 96) + "续行\n",
		"depth-limit":  strings.Repeat("> ", 128) + "限制\n",
		"deep-list":    nestedList(32),
		"mixed-nested": "> 1. 外\n>    - 中\n>      > 内\n>      > 续行\n>\n>    末\n",
		"lazy":         "> - 内容\n懒续行\n>   后续\n",
		"tabs":         ">\t\tfoo\n>\t\tbar\n\n-\t\t字\n\t续行\n\n    \tcode\n",
		"math":         "$$\nx + y\n$$\n\n\\[\n甲 + β\n\\]\n\n> $$\n> x\n> $$\n",
		"footnote":     "[^注]: 脚注中文\n    二行\n\n使用[^注]。\n",
		"fences":       "> ~~~go\n> a\n> ~~~\n\n```\n\n```\n\n```open",
		"setext":       "> 文字\n> ----\n\n- 第一行\n  第二行\n  ======\n",
		"table-pipes":  "| a\\|b | `x\\|y` |\n| --- | --- |\n| &amp; | \\* |\n| |\n",
		"references":   "[a]: /甲\n\"标题\"\n\n[a] &NotEqualTilde; &#x1f431;\n",
		"empty":        "",
		"no-final-EOL": "无尾换行\\* &amp;",
	}
}

func nestedList(depth int) string {
	var b strings.Builder
	for i := 0; i < depth; i++ {
		fmt.Fprintf(&b, "%s- 层%d\n", strings.Repeat("  ", i), i)
	}
	b.WriteString(strings.Repeat("  ", depth) + "续行\n")
	return b.String()
}

// stripped reconstructs content from Range/Prefixes/Markers only, independently
// of BlockLine.Source. Line endings normalize to LF as parser.Lines does;
// marker-only lines disappear. Padding is added only in the CR bridge mode.
func (s *adapted) stripped(i int, bridge bool) string {
	b := s.blocks[i]
	deletions := slices.Concat(b.Prefixes, b.Markers)
	slices.SortFunc(deletions, func(a, b contract.Range) int { return a.Start - b.Start })
	var out strings.Builder
	k := 0
	for start := b.Range.Start; start <= b.Range.End; {
		end, next := lineEnd(s.source, start)
		end = min(end, b.Range.End)
		if k < len(b.Lines) && b.Lines[k].Source.Start <= end {
			if bridge {
				out.WriteString(strings.Repeat(" ", int(b.Lines[k].Padding)))
			}
			pos := start
			for _, r := range deletions {
				if r.End <= start || r.Start > end {
					continue
				}
				if pos < r.Start {
					out.WriteString(s.source[pos:min(r.Start, end)])
				}
				pos = max(pos, min(r.End, end))
			}
			out.WriteString(s.source[pos:end])
			out.WriteByte('\n')
			k++
		}
		if next <= start || next > b.Range.End {
			break
		}
		start = next
	}
	return out.String()
}
