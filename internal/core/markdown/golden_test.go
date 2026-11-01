package markdown

import (
	"reflect"
	"strings"
	"testing"

	"aurink/internal/contract"
)

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
		s := mustNew(t, tc.src, 1, 0).Snapshot()
		found := false
		for i := range s.Leaves() {
			b := s.Block(i)
			if b.Kind != tc.kind {
				continue
			}
			found = true
			if s.text.Slice(b.Range) != tc.full || s.text.Slice(b.Content) != tc.content || len(b.Markers) != tc.markers || len(b.Prefixes) != tc.prefixes {
				t.Fatalf("source=%q block=%+v", tc.src, b)
			}
		}
		if !found {
			t.Fatal("missing kind")
		}
	}
	for _, src := range []string{"> ~~~go\n> a\n> ~~~\n", "> 标题\n> ---\n", "> $$\n> x\n> $$\n"} {
		s := mustNew(t, src, 1, 0).Snapshot()
		for i := range s.Leaves() {
			for _, r := range s.Block(i).Markers {
				if strings.Contains(s.text.Slice(r), ">") {
					t.Fatal("container in marker")
				}
			}
		}
	}
	s := mustNew(t, "$$ $$\n$$", 1, 0).Snapshot()
	if s.Block(1).Range.End != 5 || s.Block(2).Range.Start != 6 {
		t.Fatal("empty math swallowed next opener")
	}
}

func TestEscapeEntityGolden(t *testing.T) {
	for _, tc := range []struct {
		src             string
		spellings, text []string
	}{
		{`中 \* &amp; &#x1F431; &NotEqualTilde;`, []string{`\*`, `&amp;`, `&#x1F431;`, `&NotEqualTilde;`}, []string{"*", "&", "🐱", "≂̸"}},
		{`\\* \q &fake; &#0; &notit; &amp`, []string{`\\`, `&#0;`}, []string{`\`, "�"}},
		{"`\\* &amp;` <b a='&amp;'> &amp; [x](a&amp;b) <https://a/\\*>", []string{`&amp;`}, []string{"&"}},
		{"| a\\|b | `a\\|b` |\n| --- | --- |\n", []string{`\|`}, []string{"|"}},
		{"**\\* &amp;**\r\n> \\!", []string{`\*`, `&amp;`, `\!`}, []string{"*", "&", "!"}},
	} {
		s := mustNew(t, tc.src, 1, 0).Snapshot()
		var spelling, values []string
		for i := range s.Leaves() {
			for _, n := range s.Inlines(i) {
				if n.Kind == contract.InlineEscape || n.Kind == contract.InlineEntity {
					spelling = append(spelling, s.text.Slice(n.Range))
					values = append(values, n.Text)
					if n.Kind == contract.InlineEscape && s.text.Slice(n.Open) != `\` {
						t.Fatal("bad escape marker")
					}
				}
			}
		}
		if !reflect.DeepEqual(spelling, tc.spellings) || !reflect.DeepEqual(values, tc.text) {
			t.Fatalf("source=%q spellings=%q values=%q", tc.src, spelling, values)
		}
	}
}

func TestInlineDelimitersAndChildren(t *testing.T) {
	src := "**a *b* c** ~~strike~~ ~s~ `` x ` y `` [link [text]](url \"t\") ![alt](img) <https://example.com> \\(x\\)"
	s := mustNew(t, src, 1, 0).Snapshot()
	in := s.Inlines(s.LeafAt(0))
	seen := map[contract.InlineKind]bool{}
	for i, n := range in {
		seen[n.Kind] = true
		if n.Next <= i || n.Next > len(in) {
			t.Fatal("inline topology")
		}
		if !n.Open.Empty() && !n.Close.Empty() {
			for j := i + 1; j < n.Next; j++ {
				if in[j].Range.Start < n.Open.End || in[j].Range.End > n.Close.Start {
					t.Fatalf("child outside delimiters %+v %+v", n, in[j])
				}
			}
		}
		switch n.Kind {
		case contract.InlineStrong:
			if s.text.Slice(n.Open) != "**" || s.text.Slice(n.Close) != "**" {
				t.Fatal(n)
			}
		case contract.InlineCode:
			if s.text.Slice(n.Open) != "``" || s.text.Slice(n.Close) != "``" {
				t.Fatal(n)
			}
		case contract.InlineLink:
			if !n.Auto && s.text.Slice(n.Close) != "](url \"t\")" {
				t.Fatal(n)
			}
		case contract.InlineImage:
			if s.text.Slice(n.Open) != "![" || s.text.Slice(n.Close) != "](img)" {
				t.Fatal(n)
			}
		}
	}
	for _, k := range []contract.InlineKind{contract.InlineStrong, contract.InlineEmphasis, contract.InlineStrike, contract.InlineCode, contract.InlineLink, contract.InlineImage, contract.InlineMath} {
		if !seen[k] {
			t.Fatalf("missing %d", k)
		}
	}
}

func TestFrontMatterAndDisabledExtensions(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{"---\ntitle: 中\n---\n# body\n", true}, {"---\r\ntitle: 中\r\n...\r\nbody", true}, {"---\r\na: b\n---\rbody", true}, {"---\n---\nbody", true}, {"---\nx: y\n---", true},
		{"---\ntitle: 中\n", false}, {"\n---\nx: y\n---\n", false}, {" ---\nx: y\n---\n", false}, {"--- \nx: y\n---\n", false}, {"---", false}, {"\ufeff---\nx: y\n---\n", false},
	} {
		s := mustNew(t, tc.src, 1, contract.ExtFrontMatter).Snapshot()
		if _, ok := s.FrontMatter(); ok != tc.want {
			t.Fatalf("source=%q front=%v", tc.src, ok)
		}
		checkCoverage(t, s)
	}
	p := mustNew(t, "---\r\nx: y\r\n---\r\n# body\n", 1, contract.ExtFrontMatter)
	for _, src := range []string{"---\n---\n\n", "---\n---\n\n\n", "---\n---\n\n\n# h\n"} {
		s := mustNew(t, src, 1, contract.ExtFrontMatter).Snapshot()
		checkCoverage(t, s)
		if s.text.Slice(s.Block(1).Trailer) != "\n" {
			t.Fatal("Front Matter trailer absorbed extra separators")
		}
	}
	for _, e := range []contract.TextEdit{{Range: contract.Range{Start: 8, End: 8}, Text: "中文"}, {Range: contract.Range{End: 3}, Text: "+++"}, {Range: contract.Range{End: 3}, Text: "---"}, {Range: contract.Range{}, Text: "\n"}, {Range: contract.Range{End: 1}, Text: ""}} {
		u, err := p.Apply([]contract.TextEdit{e})
		if err != nil {
			t.Fatal(err)
		}
		if err := equivalent(u.Snapshot, mustNew(t, u.Snapshot.text.String(), u.Snapshot.Version(), contract.ExtFrontMatter).Snapshot()); err != nil {
			t.Fatal(err)
		}
	}
	for _, src := range []string{"==highlight==", "^sup^", ":smile:", "[[wiki]]", "[TOC]", "$math$"} {
		s := mustNew(t, src, 1, ^contract.Extensions(0)).Snapshot()
		in := s.Inlines(s.LeafAt(0))
		if len(in) != 1 || in[0].Kind != contract.InlineText || in[0].Text != src {
			t.Fatalf("extension should be plain text: %q %+v", src, in)
		}
	}
}

func TestBlankGolden(t *testing.T) {
	for _, tc := range []struct {
		src   string
		kinds []contract.BlockKind
	}{
		{"- a\n- b\n", []contract.BlockKind{contract.BlockParagraph, contract.BlockParagraph}},
		{"> a\n>\n> b\n", []contract.BlockKind{contract.BlockParagraph, contract.BlockParagraph}},
		{"- a\n- \n", []contract.BlockKind{contract.BlockParagraph, contract.BlockBlank}},
		{"a\n\n\n\nb\n", []contract.BlockKind{contract.BlockParagraph, contract.BlockBlank, contract.BlockBlank, contract.BlockParagraph}},
		{"a\n\n", []contract.BlockKind{contract.BlockParagraph, contract.BlockBlank}},
		{"a\r\n\r\nb\r\n", []contract.BlockKind{contract.BlockParagraph, contract.BlockParagraph}},
		{"", []contract.BlockKind{contract.BlockBlank}},
	} {
		s := mustNew(t, tc.src, 1, 0).Snapshot()
		var kinds []contract.BlockKind
		for i := range s.Leaves() {
			kinds = append(kinds, s.Block(i).Kind)
		}
		if !reflect.DeepEqual(kinds, tc.kinds) {
			t.Fatalf("source=%q kinds=%v", tc.src, kinds)
		}
		checkCoverage(t, s)
	}
	for _, src := range []string{"> [ref]: /u\n", "| a |\n|---|\n| 1 | 多余 |\n", "``---\n` |\n|---|\n\r\n[*^x]: y\n-\\\t\n\r\n \n[r]di`\n[r]*中* \\# 🐈["} {
		s := mustNew(t, src, 1, 0).Snapshot()
		checkCoverage(t, s)
	}
}

func TestDefinitionTables(t *testing.T) {
	for _, src := range []string{"[1]: 123\n\nparagraph\n", "> [1]: 123\n", "> [r\n> x]: /nested \"title\"\n\n[r x]\n", "> [1\n> 2]: 123\n", "[R]:\n  /first\n  \"multi title\"\n[r]: /second\n\n[label][ r ]\n"} {
		s := mustNew(t, src, 1, 0).Snapshot()
		checkCoverage(t, s)
		defs := s.LinkDefs()
		if len(defs) != 1 {
			for i := range s.entries {
				t.Logf("%+v", s.Block(i))
			}
			t.Fatalf("definitions of %q: %+v", src, defs)
		}
		if strings.Contains(src, "nested") {
			if d, ok := defs["r x"]; !ok || d.Dest != "/nested" || d.Title != "title" {
				t.Fatal(defs)
			}
		}
		if strings.Contains(src, "first") {
			if d := defs["r"]; d.Dest != "/first" || d.Title != "multi title" {
				t.Fatal(defs)
			}
			if d := defs["r"]; s.text.Slice(d.Range) != "[R]:\n  /first\n  \"multi title\"" {
				t.Fatal(d)
			}
		}
		if strings.Contains(src, "123") {
			found := false
			for i := range s.Leaves() {
				b := s.Block(i)
				if b.Kind == contract.BlockLinkDef {
					found = true
					if !strings.Contains(s.text.Slice(b.Range), "123") {
						t.Fatal(b)
					}
				}
			}
			if !found {
				t.Fatal("numeric definition hidden")
			}
		}
	}
	src := "| a |\n|---|\n| b | [r] `x\\|y` |\n\n[r]: /resolved\n"
	s := mustNew(t, src, 1, 0).Snapshot()
	found := false
	for i := range s.Leaves() {
		b := s.Block(i)
		if !strings.Contains(s.text.Slice(b.Range), "[r]") || b.Kind != contract.BlockTableCell {
			continue
		}
		for _, n := range s.Inlines(i) {
			if n.Kind == contract.InlineLink && n.Dest == "/resolved" {
				found = true
			}
			if n.Kind == contract.InlineCode && n.Text != "x|y" {
				t.Fatal(n)
			}
		}
	}
	if !found {
		t.Fatal("overflow cell reference unresolved")
	}
}

func TestGitHubSlugs(t *testing.T) {
	for _, tc := range [][2]string{{"I ♥ unicode", "i--unicode"}, {"😄 unicode emoji", "-unicode-emoji"}, {"A + B = C", "a--b--c"}, {"a_ ‿ ⁀ ⁔ ＿b", "a_-‿-⁀-⁔-＿b"}, {"a² Ⅳ b", "a-ⅳ-b"}, {"İstanbul", "i\u0307stanbul"}, {"中文 标题!", "中文-标题"}} {
		if got := slug(tc[0]); got != tc[1] {
			t.Fatalf("slug(%q)=%q want %q", tc[0], got, tc[1])
		}
	}
	s := mustNew(t, "# echo\n# echo\n# echo 1\n# echo-1\n# echo\n", 1, 0).Snapshot()
	var got []string
	for _, h := range s.Outline() {
		got = append(got, h.Slug)
	}
	if strings.Join(got, " ") != "echo echo-1 echo-1-1 echo-1-2 echo-2" {
		t.Fatal(got)
	}
}
