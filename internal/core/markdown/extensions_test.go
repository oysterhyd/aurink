package markdown

import (
	"strings"
	"testing"

	"aurink/internal/contract"
)

func TestExtensionEdges(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		want      map[contract.InlineKind][]string
		absent    []contract.InlineKind
	}{
		{"nested and CJK", "==外 ^字^ 内== ^x==y==^", map[contract.InlineKind][]string{contract.InlineHighlight: {"==外 ^字^ 内==", "==y=="}, contract.InlineSup: {"^字^", "^x==y==^"}}, nil},
		{"unmatched inner opener", "==a ^unfinished==", map[contract.InlineKind][]string{contract.InlineHighlight: {"==a ^unfinished=="}}, []contract.InlineKind{contract.InlineSup}},
		{"sub and strike", "H~中~O ~~删~~ ~a b~", map[contract.InlineKind][]string{contract.InlineSub: {"~中~"}, contract.InlineStrike: {"~~删~~"}}, nil},
		{"unmatched double tilde before single", "x~~y~", nil, []contract.InlineKind{contract.InlineSub}},
		{"unmatched double tilde after single", "x~y~~", nil, []contract.InlineKind{contract.InlineSub}},
		{"unmatched triple tilde before single", "x ~~~a~", nil, []contract.InlineKind{contract.InlineSub}},
		{"unmatched double tilde", "x~~y", nil, []contract.InlineKind{contract.InlineSub}},
		{"spaces", "== a== ==a == ^a b^ ~a b~", nil, []contract.InlineKind{contract.InlineHighlight, contract.InlineSup, contract.InlineSub}},
		{"split delimiter", "=\n=字==", nil, []contract.InlineKind{contract.InlineHighlight}},
		{"highlight spans break", "==中\n文==", map[contract.InlineKind][]string{contract.InlineHighlight: {"==中\n文=="}}, nil},
		{"shortcodes near words and digits", "a:smile:2 10:30:00 :+1:", map[contract.InlineKind][]string{contract.InlineEmoji: {":smile:", ":+1:"}}, nil},
		{"wiki alias and heading", "[[页面#标题|别名|后缀]]", map[contract.InlineKind][]string{contract.InlineWikiLink: {"[[页面#标题|别名|后缀]]"}}, nil},
		{"wiki with reference definition", "[[page]]\n\n[page]: /target\n", map[contract.InlineKind][]string{contract.InlineWikiLink: {"[[page]]"}}, nil},
		{"wiki wraps formatting", "[[page|**别名**]]", map[contract.InlineKind][]string{contract.InlineWikiLink: {"[[page|**别名**]]"}, contract.InlineStrong: {"**别名**"}}, nil},
		{"opaque nodes", "`==a== :smile:` <https://example.com/:smile:> [label](https://example.com/:smile:) \\==x== $:smile:$", nil, []contract.InlineKind{contract.InlineHighlight, contract.InlineEmoji}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := mustNew(t, tc.src, 1, contract.ExtDefault).Snapshot()
			got := map[contract.InlineKind][]string{}
			for leaf := range a.Leaves() {
				for _, n := range a.Inlines(leaf) {
					got[n.Kind] = append(got[n.Kind], tc.src[n.Range.Start:n.Range.End])
				}
			}
			for kind, want := range tc.want {
				for _, item := range want {
					found := false
					for _, actual := range got[kind] {
						found = found || actual == item
					}
					if !found {
						t.Errorf("kind %d missing %q: %q\n%s", kind, item, got[kind], differentialDump(a))
					}
				}
			}
			for _, kind := range tc.absent {
				if len(got[kind]) != 0 {
					t.Errorf("unexpected kind %d: %q", kind, got[kind])
				}
			}
			if tc.name == "wiki alias and heading" {
				for _, n := range a.Inlines(a.LeafAt(0)) {
					if n.Kind == contract.InlineWikiLink && (n.Dest != "页面#标题" || tc.src[n.Open.End:n.Close.Start] != "别名|后缀") {
						t.Errorf("wiki link %+v", n)
					}
				}
			}
		})
	}
}

func TestTOCWithReferenceDefinition(t *testing.T) {
	src := "[toc]: /target\n\n [TOC] \n\n> [TOC]\n"
	a := mustNew(t, src, 1, contract.ExtTOC).Snapshot()
	var found int
	for leaf := range a.Leaves() {
		if a.Block(leaf).Kind == contract.BlockTOC {
			found++
			if a.Block(leaf).Parent != 0 {
				t.Fatal("nested TOC")
			}
		}
	}
	if found != 1 {
		t.Fatalf("got %d TOC blocks", found)
	}
}

func TestWikiLinksDoNotCrossLineBreaks(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"LF", "[[page\nsection]]"},
		{"CRLF", "[[page\r\nsection]]"},
		{"CR", "[[page\rsection]]"},
		{"hard break with spaces", "[[page  \nsection]]"},
		{"hard break with escape", "[[page\\\nsection]]"},
		{"block quote", "> [[page\n> section]]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := mustNew(t, tc.src, 1, contract.ExtWikiLink).Snapshot()
			foundBreak := false
			for leaf := range a.Leaves() {
				for _, n := range a.Inlines(leaf) {
					if n.Kind == contract.InlineWikiLink {
						t.Errorf("wiki link spans lines: %+v", n)
					}
					if n.Kind == contract.InlineText && strings.ContainsAny(tc.src[n.Range.Start:n.Range.End], "\r\n") {
						t.Errorf("text node spans a line break: %+v", n)
					}
					foundBreak = foundBreak || n.Kind == contract.InlineSoftBreak || n.Kind == contract.InlineLineBreak
				}
			}
			if !foundBreak {
				t.Fatal("fixture did not produce a line-break node")
			}
		})
	}
}

func TestUnmatchedExtensionsPreserveInlineTree(t *testing.T) {
	for _, tc := range []struct {
		src string
		ext contract.Extensions
	}{
		{"before ==unfinished after", contract.ExtHighlight},
		{"before == bad == after", contract.ExtHighlight},
		{"before ^unfinished after", contract.ExtSupSub},
		{"before ~unfinished after", contract.ExtTildeSub},
		{"before :not_a_shortcode: after 10:30:00", contract.ExtEmoji},
		{"before [[unfinished after", contract.ExtWikiLink},
	} {
		plain := differentialDump(mustNew(t, tc.src, 1, 0).Snapshot())
		extended := differentialDump(mustNew(t, tc.src, 1, tc.ext).Snapshot())
		if plain != extended {
			t.Errorf("unmatched syntax %q changed the AST\nplain %s\nextended %s", tc.src, plain, extended)
		}
	}
}

func TestExtensionHeadingOutline(t *testing.T) {
	a := mustNew(t, "# ==重点== :smile: [[页|别名]] ^2^\n", 1, contract.ExtDefault).Snapshot()
	if outline := a.Outline(); len(outline) != 1 || outline[0].Text != "重点 😄 别名 2" {
		t.Fatalf("outline %+v", outline)
	}
}

func TestIncrementalExtensionClosers(t *testing.T) {
	for _, tc := range []struct {
		name, marked, insert string
		kind                 contract.InlineKind
		block                contract.BlockKind
	}{
		{"highlight", "==中@", "==", contract.InlineHighlight, 0},
		{"sup", "^中@", "^", contract.InlineSup, 0},
		{"sub", "~中@", "~", contract.InlineSub, 0},
		{"emoji", ":smile@", ":", contract.InlineEmoji, 0},
		{"wiki", "[[目标@", "]]", contract.InlineWikiLink, 0},
		{"math", "$x@", "$", contract.InlineMath, 0},
		{"toc", "[TOC@", "]", 0, contract.BlockTOC},
		{"front matter", "---\ntitle: A\n--@\n\nbody", "-", 0, contract.BlockFrontMatter},
	} {
		t.Run(tc.name, func(t *testing.T) {
			marked := tc.marked
			if tc.name != "front matter" {
				marked = "before text\n\n" + marked + "\n\nafter text\n\nlast text\n"
			}
			at := strings.Index(marked, "@")
			src := strings.Replace(marked, "@", "", 1)
			p := mustNew(t, src, 1, contract.ExtDefault)
			for leaf := range p.Snapshot().Leaves() {
				_ = p.Snapshot().Inlines(leaf)
			}
			u, err := p.Apply([]contract.TextEdit{{Range: contract.Range{Start: at, End: at}, Text: tc.insert}})
			if err != nil {
				t.Fatal(err)
			}
			next := src[:at] + tc.insert + src[at:]
			if got, want := differentialDump(u.Snapshot), differentialDump(mustNew(t, next, 1, contract.ExtDefault).Snapshot()); got != want {
				t.Fatalf("incremental differs from full\n%s\n%s", got, want)
			}
			if tc.name != "front matter" && u.Stats.FullRebuilds != 0 {
				t.Fatalf("unexpected full parse: %+v", u.Stats)
			}
			found := false
			for leaf := range u.Snapshot.Leaves() {
				if tc.block != 0 && u.Snapshot.Block(leaf).Kind == tc.block {
					found = true
				}
				for _, n := range u.Snapshot.Inlines(leaf) {
					if tc.kind != 0 && n.Kind == tc.kind {
						found = true
					}
				}
			}
			if !found {
				t.Fatal("completed extension was not recognized")
			}
		})
	}
}
