package astspike

import (
	"reflect"
	"strings"
	"testing"

	"aurink.local/spikes/contract-rc/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

func TestFrontMatter(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		enabled   bool
		want      bool
	}{
		{"LF", "---\ntitle: 中\n---\n# 正文 &amp;\n", true, true},
		{"CRLF", "---\r\ntitle: 中\r\n...\r\n> a\r\n> b\r\n", true, true},
		{"mixed", "---\r\na: b\n---\r正文", true, true},
		{"empty", "---\n---\n正文", true, true},
		{"EOF-close", "---\nx: y\n---", true, true},
		{"unclosed", "---\ntitle: 中\n", true, false},
		{"not-first", "\n---\nx: y\n---\n", true, false},
		{"disabled", "---\nx: y\n---\n正文", false, false},
		{"indent", " ---\nx: y\n---\n", true, false},
		{"trailing-space", "--- \nx: y\n---\n", true, false},
		{"opener-only", "---", true, false},
		{"BOM-is-store-responsibility", "\ufeff---\nx: y\n---\n", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := parse(tc.src, parser.Default, tc.enabled)
			if err != nil {
				t.Fatal(err)
			}
			if (s.bodyAt > 0) != tc.want {
				t.Fatalf("frontmatter at=%d want=%t", s.bodyAt, tc.want)
			}
			if !tc.want {
				return
			}
			b := s.blocks[1]
			if b.Kind != contract.BlockFrontMatter || b.Range.Start != 0 || b.Range.End != s.bodyAt || s.doc.Source() != tc.src[s.bodyAt:] {
				t.Fatalf("bad frontmatter block: %+v", b)
			}
			body, err := parse(tc.src[s.bodyAt:], parser.Default, false)
			if err != nil {
				t.Fatal(err)
			}
			for i := 2; i < len(s.blocks); i++ {
				got, want := s.blocks[i], body.blocks[i-1]
				if got.Range.Start != want.Range.Start+s.bodyAt || got.Range.End != want.Range.End+s.bodyAt {
					t.Fatal("body range was not shifted")
				}
				if s.stripped(i, true) != s.doc.Lines(i-1) {
					t.Fatal("shifted prefix mapping differs")
				}
				a, err := s.inlines(i)
				if err != nil {
					t.Fatal(err)
				}
				for _, n := range a {
					if n.Range.Start < s.bodyAt || n.Range.End > len(tc.src) {
						t.Fatal("inline was not shifted")
					}
				}
			}
		})
	}
}

func TestFrontMatterEdits(t *testing.T) {
	e, err := newEngine("---\r\nx: y\r\n---\r\n# body\n", parser.Default, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, edit := range []contract.TextEdit{
		{Range: contract.Range{Start: 8, End: 8}, Text: "中文"},
		{Range: contract.Range{Start: 0, End: 3}, Text: "+++"},
		{Range: contract.Range{Start: 0, End: 3}, Text: "---"},
		{Range: contract.Range{Start: 0, End: 0}, Text: "\n"},
		{Range: contract.Range{Start: 0, End: 1}, Text: ""},
	} {
		if err := e.edit(edit); err != nil {
			t.Fatal(err)
		}
		full, err := parse(e.state.source, parser.Default, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := equivalent(e.state, full); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEscapeEntityGolden(t *testing.T) {
	for _, tc := range []struct {
		src             string
		kinds           []contract.InlineKind
		spellings, text []string
	}{
		{`中 \* &amp; &#x1F431; &NotEqualTilde;`, []contract.InlineKind{contract.InlineEscape, contract.InlineEntity, contract.InlineEntity, contract.InlineEntity}, []string{`\*`, `&amp;`, `&#x1F431;`, `&NotEqualTilde;`}, []string{"*", "&", "🐱", "≂̸"}},
		{`\\* \q &fake; &#0; &notit; &amp`, []contract.InlineKind{contract.InlineEscape, contract.InlineEntity}, []string{`\\`, `&#0;`}, []string{`\`, "�"}},
		{"`\\* &amp;` <b a='&amp;'> &amp; [x](a&amp;b) <https://a/\\*>", []contract.InlineKind{contract.InlineEntity}, []string{`&amp;`}, []string{"&"}},
		{"| a\\|b | `a\\|b` |\n| --- | --- |\n", []contract.InlineKind{contract.InlineEscape}, []string{`\|`}, []string{"|"}},
		{"**\\* &amp;**\r\n> \\!", []contract.InlineKind{contract.InlineEscape, contract.InlineEntity, contract.InlineEscape}, []string{`\*`, `&amp;`, `\!`}, []string{"*", "&", "!"}},
	} {
		s, err := parse(tc.src, parser.Default, false)
		if err != nil {
			t.Fatal(err)
		}
		var kinds []contract.InlineKind
		var spellings, texts []string
		for i := 1; i < len(s.blocks); i++ {
			in, err := s.inlines(i)
			if err != nil {
				t.Fatal(err)
			}
			for j, n := range in {
				if n.Next <= j || n.Next > len(in) {
					t.Fatalf("invalid flat inline topology: %+v", n)
				}
				if n.Kind != contract.InlineEscape && n.Kind != contract.InlineEntity {
					continue
				}
				kinds = append(kinds, n.Kind)
				spellings = append(spellings, tc.src[n.Range.Start:n.Range.End])
				texts = append(texts, n.Text)
				if n.Kind == contract.InlineEscape && tc.src[n.Open.Start:n.Open.End] != `\` {
					t.Fatal("escape backslash range is wrong")
				}
			}
		}
		if !reflect.DeepEqual(kinds, tc.kinds) || !reflect.DeepEqual(spellings, tc.spellings) || !reflect.DeepEqual(texts, tc.text) {
			t.Errorf("src=%q\nkinds=%v spellings=%q texts=%q", tc.src, kinds, spellings, texts)
		}
	}
}

func TestShardsAndGlobalReferences(t *testing.T) {
	parts := []string{"# 题\n\n", "[text][r] &amp;\n\n", "> - 中\n>   文\n\n", "```go\na\n```\n\n"}
	definitions := "[r]: /old \"title\"\n[r]: /ignored\n"
	source := strings.Join(parts, "") + definitions
	full := parser.Parse(source)
	var naive, global strings.Builder
	for _, part := range parts {
		naive.WriteString(parser.Parse(part).HTML())
		global.WriteString(parser.Parse(part + "\n\n" + definitions).HTML())
	}
	if naive.String() == full.HTML() || global.String() != full.HTML() {
		t.Fatal("global definition dependency experiment failed")
	}
	updated := strings.ReplaceAll(definitions, "/old", "/new")
	if parser.Parse(parts[1]+"\n\n"+updated).HTML() == parser.Parse(parts[1]+"\n\n"+definitions).HTML() {
		t.Fatal("definition edit must invalidate the referencing shard")
	}
	// Whitespace/case normalization, multiline destination/title and first
	// definition precedence must be shared in document order, not regex lines.
	multiline := "[R]:\n  /first\n  \"multi title\"\n[r]: /second\n"
	use := "[label][ r ]\n\n"
	if parser.Parse(use+multiline).HTML() != parser.Parse(use+"\n\n"+multiline).HTML() || !strings.Contains(parser.Parse(use+multiline).HTML(), `href="/first"`) {
		t.Fatal("multiline/normalized reference context was lost")
	}
	if !strings.Contains(parser.Parse(use+"[r]: /second\n").HTML(), `href="/second"`) {
		t.Fatal("removing the first definition must promote the duplicate")
	}
	// Definitions inside containers enter the SAME document-wide namespace.
	if !strings.Contains(parser.Parse("[r]\n\n> [r]: /nested\n").HTML(), `href="/nested"`) {
		t.Fatal("nested definitions must be included in global context")
	}
	// Safe boundaries come from a whole parse, not splitting on blank lines.
	unsafeParts := []string{"```go\na\n\n", "b\n```\n"}
	if parser.Parse(strings.Join(unsafeParts, "")).HTML() == parser.Parse(unsafeParts[0]).HTML()+parser.Parse(unsafeParts[1]).HTML() {
		t.Fatal("expected naive blank-line splitting to change fenced-code structure")
	}
	// Appending definitions is also UNSAFE for an unclosed shard: the fence
	// swallows the synthetic definitions. A public reference-context API or
	// a safe/closed-shard restriction is necessary.
	unclosed := "[r]\n\n```\nx\n"
	if parser.Parse(unclosed+"\n\n"+definitions).HTML() == parser.Parse(definitions+"\n\n"+unclosed).HTML() {
		t.Fatal("expected definition placement to matter for open fences")
	}
	t.Log("4 closed shards: independent references FAIL; shared raw definitions PASS; definition edit invalidates users; blank-line split and open-fence definition suffix FAIL")
}
