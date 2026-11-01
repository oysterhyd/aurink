package astspike

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"aurink.local/spikes/contract-rc/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

type example struct {
	Markdown string `json:"markdown"`
	Example  int    `json:"example"`
	Section  string `json:"section"`
}

type sample struct {
	name, src string
	ext       parser.Extensions
}

func corpus(t testing.TB) []sample {
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
		"math":         "$$ x + y $$\n\n\\[\n甲 + β\n\\]\n\n> $$\n> x\n> $$\n",
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

func TestCorpusPrefixes(t *testing.T) {
	var blocks, lines, strictFailures, padded, inlineNodes, escapes, entities int
	var proseBlocks, proseFailures int
	var witnesses []string
	samples := corpus(t)
	for _, c := range samples {
		s, err := parse(c.src, c.ext, false)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		for i := 1; i < len(s.blocks); i++ {
			b := s.blocks[i]
			if b.Range.Start < 0 || b.Range.End > len(c.src) || b.Range.End < b.Range.Start ||
				b.Content.Start < b.Range.Start || b.Content.End > b.Range.End || b.Parent >= i || b.Next <= i {
				t.Fatalf("%s invalid block %d: %+v", c.name, i, b)
			}
			if !isLeaf(b.Kind) {
				continue
			}
			blocks++
			prose := b.Kind == contract.BlockParagraph || b.Kind == contract.BlockHeading
			if prose {
				proseBlocks++
			}
			lines += len(s.lines[i])
			for _, l := range s.lines[i] {
				if l.Padding > 0 {
					padded++
				}
			}
			want := s.doc.Lines(i)
			if got := s.stripped(i, false); got != want {
				strictFailures++
				if prose {
					proseFailures++
				}
				if len(witnesses) < 6 {
					witnesses = append(witnesses, fmt.Sprintf("%s block=%d strict=%q parser=%q", c.name, i, got, want))
				}
			}
			if got := s.stripped(i, true); got != want {
				t.Fatalf("%s block=%d range=%+v prefixes=%+v markers=%+v\nbridge=%q\nparser=%q\nsrc=%q", c.name, i, b.Range, b.Prefixes, b.Markers, got, want, c.src)
			}
			in, err := s.inlines(i)
			if err != nil {
				t.Fatalf("%s block=%d: %v src=%q", c.name, i, err, c.src)
			}
			for _, n := range in {
				inlineNodes++
				if n.Kind == contract.InlineEscape {
					escapes++
				}
				if n.Kind == contract.InlineEntity {
					entities++
				}
			}
		}
	}
	if padded == 0 || strictFailures == 0 || escapes == 0 || entities == 0 {
		t.Fatal("corpus failed to exercise required cases")
	}
	t.Logf("samples=%d leaf_blocks=%d content_lines=%d strict_pass=%d/%d bridge_pass=%d/%d padded_lines=%d inline_nodes=%d escape=%d entity=%d", len(samples), blocks, lines, blocks-strictFailures, blocks, blocks, blocks, padded, inlineNodes, escapes, entities)
	t.Logf("paragraph_heading_strict_pass=%d/%d", proseBlocks-proseFailures, proseBlocks)
	for _, w := range witnesses {
		t.Log(w)
	}
}

func TestSourceEnvelopes(t *testing.T) {
	for _, tc := range []struct {
		src, full, content string
		kind               contract.BlockKind
		markers, prefixes  int
	}{
		{"## 中文 ##\n", "## 中文 ##", "中文", contract.BlockHeading, 2, 0},
		{"> a\n> b\n", "a\n> b", "a\n> b", contract.BlockParagraph, 0, 1},
		{"标题\r\n---\r\n", "标题\r\n---", "标题", contract.BlockHeading, 1, 0},
		{"```go\nx\n```\n", "```go\nx\n```", "x", contract.BlockCode, 2, 0},
		{"$$ x $$\n", "$$ x $$", "x", contract.BlockMath, 2, 0},
	} {
		s, err := parse(tc.src, parser.Default, false)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, b := range s.blocks {
			if b.Kind != tc.kind {
				continue
			}
			found = true
			if s.source[b.Range.Start:b.Range.End] != tc.full || s.source[b.Content.Start:b.Content.End] != tc.content || len(b.Markers) != tc.markers || len(b.Prefixes) != tc.prefixes {
				t.Errorf("src=%q block=%+v", tc.src, b)
			}
		}
		if !found {
			t.Errorf("kind %d missing in %q", tc.kind, tc.src)
		}
	}
}

func TestEmptyMathDoesNotConsumeNextOpener(t *testing.T) {
	s, err := parse("$$ $$\n$$", parser.Default, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.blocks) != 3 || s.blocks[1].Range.End != 5 || s.blocks[2].Range.Start != 6 {
		t.Fatalf("adjacent empty math envelopes overlap: %+v", s.blocks)
	}
}

func TestMarkersExcludeContainerPrefixes(t *testing.T) {
	for _, src := range []string{"> ~~~go\n> a\n> ~~~\n", "> 标题\n> ---\n", "> $$\n> x\n> $$\n"} {
		s, err := parse(src, parser.Default, false)
		if err != nil {
			t.Fatal(err)
		}
		for i, b := range s.blocks {
			if !isLeaf(b.Kind) {
				continue
			}
			for _, r := range b.Markers {
				if strings.Contains(src[r.Start:r.End], ">") {
					t.Fatalf("container prefix leaked into marker: %+v", b)
				}
			}
			if s.stripped(i, true) != s.doc.Lines(i) {
				t.Fatal("marker/prefix partition lost content")
			}
		}
	}
}
