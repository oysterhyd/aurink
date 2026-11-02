package projection

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"aurink/internal/contract"
)

var updateGolden = flag.Bool("update", false, "update projection golden segment tables")

type fixture struct{ name, source, find, rest string }

var fixtures = []fixture{
	{"paragraph", "plain words\n", "", "plain words"},
	{"cjk", "中文 日本語 한국어\n", "", "中文 日本語 한국어"},
	{"emoji", "😀 👩‍💻 🇨🇳 👍🏽\n", "", "😀 👩‍💻 🇨🇳 👍🏽"},
	{"combining", "cafe\u0301 हिन्दी\n", "", "cafe\u0301 हिन्दी"},
	{"strong", "a **粗体** b\n", "", "a 粗体 b"},
	{"emphasis", "a *字* b\n", "", "a 字 b"},
	{"underscore", "a _word_ b\n", "", "a word b"},
	{"strong-underscore", "__word__\n", "", "word"},
	{"nested", "**a *b* c** d\n", "", "a b c d"},
	{"nested-strike", "~~a **b** c~~\n", "", "a b c"},
	{"strike", "a ~~gone~~ b\n", "", "a gone b"},
	{"single-tilde", "a ~gone~ b\n", "", "a gone b"},
	{"code", "a `x &amp; *y*` b\n", "", "a x &amp; *y* b"},
	{"code-trim", "` a `\n", "", "a"},
	{"code-all-spaces", "`   `\n", "", "   "},
	{"code-backticks", "`` a ` b ``\n", "", "a ` b"},
	{"code-multiline", "` a\nb `\n", "", "a b"},
	{"code-quote", " > `a\n > b`\n", "", "a b"},
	{"code-quote-all-space", "> ` \n>  `\n", "", "  "},
	{"code-table-pipe", "| `a\\|b` |\n|---|\n", "a", "a|b"},
	{"code-quote-trim", "> ` a\n> `\n", "", "a"},
	{"link", "a [label](local.md \"title\") b\n", "", "a label b"},
	{"link-emphasis", "[**粗**](a.md) tail\n", "", "粗 tail"},
	{"reference", "[ref][x]\n\n[x]: /one \"t\"\n", "", "ref"},
	{"autolink", "<https://example.com/a>\n", "", "https://example.com/a"},
	{"gfm-autolink", "www.example.com tail\n", "", "www.example.com tail"},
	{"email", "<a@example.com>\n", "", "a@example.com"},
	{"escapes", "a \\* \\[ \\\\ b\n", "", "a * [ \\ b"},
	{"entities", "&amp; &#38; &#x1F600; &copy;\n", "", "& & 😀 ©"},
	{"unknown-entity", "&unknown; &amp\n", "", "&unknown; &amp"},
	{"heading-one", "# 标题\n", "", "标题"},
	{"heading-two", "## **标题** ##\n", "", "标题"},
	{"heading-six", "###### 六\n", "", "六"},
	{"heading-empty", "###\n", "", ""},
	{"setext", "Title\n=====\n", "", "Title"},
	{"setext-multiline", "first\nsecond\n---\n", "", "first\nsecond"},
	{"soft-lf", "a\nb\n", "", "a\nb"},
	{"soft-trailing-space", "> a \n> b\n", "", "a\nb"},
	{"soft-crlf", "甲\r\n乙\r\n", "", "甲\n乙"},
	{"soft-cr", "a\rb\r", "", "a\nb"},
	{"hard-spaces", "a  \nb\n", "", "a\nb"},
	{"hard-slash", "a\\\nb\n", "", "a\nb"},
	{"hard-crlf", "a  \r\nb\r\n", "", "a\nb"},
	{"quote", " > a **b**\n > c\n", "", "a b\nc"},
	{"quote-lazy", "> a\nlazy\n", "", "a\nlazy"},
	{"quote-nested", "> > a\n> > b\n", "", "a\nb"},
	{"quote-multiline-inline", "> **a\n> b** c\n", "", "a\nb c"},
	{"quote-crlf", "> 甲\r\n> 乙\r\n", "", "甲\n乙"},
	{"list", "- a\n  b\n", "", "a\nb"},
	{"ordered", "7. a\n8. b\n", "b", "b"},
	{"nested-container", "> 3. - **甲**\n>      乙\n", "", "甲\n乙"},
	{"task-open", "- [ ] todo\n", "", "todo"},
	{"task-done", "- [x] done\n", "", "done"},
	{"task-upper", "- [X] done\n", "", "done"},
	{"task-quote", "> - [x] a\n>   b\n", "", "a\nb"},
	{"empty-list", "-\n", "", ""},
	{"empty-quote", ">\n", "", ""},
	{"fence", "```go\nx\ny\n```\n", "", "x\ny"},
	{"fence-crlf", "~~~text\r\n甲\r\n乙\r\n~~~\r\n", "", "甲\n乙"},
	{"fence-quote", "> ```go\n> x\n> ```\n", "", "x"},
	{"fence-unclosed", "```\nx\ny", "", "x\ny"},
	{"fence-empty", "```\n```\n", "", ""},
	{"indented-code", "    a\n\tb\n", "", "a\nb"},
	{"thematic", "***\n", "", ""},
	{"thematic-quote", "> ---\n", "", ""},
	{"image", "![图](a.png \"title\")\n", "", "\ufffc"},
	{"image-alt", "![a **粗** &amp;](../图.png) tail\n", "", "\ufffc tail"},
	{"table", "| **甲** | b |\n|---|---|\n| c | d |\n", "甲", "甲"},
	{"table-body", "| a | b |\n|---|---|\n| c | *丁* |\n", "丁", "丁"},
	{"table-escape", "| a\\|b |\n|---|\n", "a", "a|b"},
	{"html-inline", "a <b>x</b> b\n", "", "a x b"},
	{"html-block", "<div>\n**raw**\n</div>\n", "", "<div>\n**raw**\n</div>"},
	{"footnote-ref", "a[^n]\n\n[^n]: note\n", "a", "a\ufffc"},
	{"footnote-definition", "[^n]: **note**\n    line\n", "note", "**note**\nline"},
	{"frontmatter", "---\ntitle: **raw**\n---\n", "", "---\ntitle: **raw**\n---\n"},
	{"definition", "[x]: /url \"title\"\n", "", "[x]: /url \"title\""},
	{"extensions-off", "==hi== ^up^ :smile: [[wiki]] $x$\n", "", "==hi== ^up^ :smile: [[wiki]] $x$"},
	{"empty", "", "", ""},
	{"blank", "\n\n\n", "", ""},
}

type goldenResult struct {
	Display    string
	Kind       contract.BlockKind
	Level      int
	Lang       string
	Revealed   bool
	Segments   []contract.Segment
	Containers []contract.ContainerDecor
}

type goldenCase struct {
	Name, Source string
	At           int
	Reveal       contract.Reveal
	Want         goldenResult
}

func goldenOutput(p contract.BlockProjection) goldenResult {
	for i := range p.Containers {
		p.Containers[i].Block = 0
	}
	return goldenResult{New().DisplayText(&p), p.Kind, p.Level, p.Lang, p.Revealed, p.Segments, p.Containers}
}

func TestGolden(t *testing.T) {
	const path = "testdata/projection.jsonl"
	if *updateGolden {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		enc := json.NewEncoder(f)
		enc.SetEscapeHTML(false)
		for _, tc := range fixtures {
			tx, a := parse(t, tc.source)
			at := 0
			if tc.find != "" {
				at = strings.Index(tc.source, tc.find)
			}
			leaf := a.LeafAt(at)
			n := a.Block(leaf)
			contexts := []contract.Reveal{
				{Caret: -1}, {Caret: n.Range.Start}, {Caret: (n.Range.Start + n.Range.End) / 2}, {Caret: n.Range.End},
				{Mode: contract.RevealNone, Caret: -1}, {Mode: contract.RevealAlways, Caret: -1},
				{Mode: contract.RevealBlock, Caret: n.Range.Start}, {Caret: -1, SoftBreak: contract.SoftBreakSpace},
				{Caret: -1, Selection: n.Range},
			}
			for i, r := range contexts {
				p := New().Project(a, tx, leaf, r)
				out := goldenOutput(p)
				if i == 0 && out.Display != tc.rest {
					t.Errorf("%s resting display %q, want %q", tc.name, out.Display, tc.rest)
				}
				if err := enc.Encode(goldenCase{fmt.Sprintf("%s/%d", tc.name, i), tc.source, at, r, out}); err != nil {
					t.Fatal(err)
				}
			}
		}
		return
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	count := 0
	for scanner.Scan() {
		var tc goldenCase
		if err := json.Unmarshal(scanner.Bytes(), &tc); err != nil {
			t.Fatal(err)
		}
		t.Run(tc.Name, func(t *testing.T) {
			tx, a := parse(t, tc.Source)
			got := goldenOutput(New().Project(a, tx, a.LeafAt(tc.At), tc.Reveal))
			if !reflect.DeepEqual(got, tc.Want) {
				data, _ := json.Marshal(got)
				t.Fatalf("projection differs:\n%s\nwant:\n%+v", data, tc.Want)
			}
		})
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count < 150 {
		t.Fatalf("only %d goldens", count)
	}
}
