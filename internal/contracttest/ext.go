package contracttest

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"aurink/internal/contract"
)

// extKinds are the inline kinds only extensions produce.
var extKinds = map[contract.InlineKind]contract.Extensions{
	contract.InlineHighlight: contract.ExtHighlight,
	contract.InlineSup:       contract.ExtSupSub,
	contract.InlineSub:       contract.ExtTildeSub,
	contract.InlineEmoji:     contract.ExtEmoji,
	contract.InlineWikiLink:  contract.ExtWikiLink,
}

var allExts = []contract.Extensions{
	contract.ExtFrontMatter, contract.ExtHighlight, contract.ExtSupSub, contract.ExtTildeSub,
	contract.ExtEmoji, contract.ExtWikiLink, contract.ExtTOC, contract.ExtDollarMath,
}

// extSamples exercise each extension, with the near misses that must stay text.
var extSamples = []string{
	"a ==高亮== b == 不是 == c ==x\n",
	"==a **b** c== `==码==` \\==x==\n",
	"x^2^ y^a b^ z^^ \\^n^\n",
	"H~2~O ~~删~~ ~a b~ ~~\n",
	":smile: :heart: :not_an_emoji: 10:30:00 `:smile:`\n",
	"[[页面]] [[页面|别名]] [[页面#标题]] [[]] [[a\nb]]\n",
	"[TOC]\n\n[toc]\n\n[TOC] 后面有字\n\n> [TOC]\n",
	"价格 $5 和 $6，公式 $x^2$ 与 $$y$$\n",
	"---\ntitle: 标题\ntags: [a, b]\ndate: 2026-10-31\n---\n\n# h ==x== ^2^ :smile: [[p]]\n",
	"| ==a== | ^b^ |\n|---|---|\n| :smile: | [[p]] |\n",
}

// inlinesOf returns the inlines of every leaf of src, with the source each
// covers, as "kind:source" strings.
func inlinesOf(a contract.AST, src string) []contract.Inline {
	var out []contract.Inline
	for leaf := range a.Leaves() {
		out = append(out, a.Inlines(leaf)...)
	}
	return out
}

func findInline(a contract.AST, src string, k contract.InlineKind, covering string) (contract.Inline, bool) {
	for _, n := range inlinesOf(a, src) {
		if n.Kind == k && src[n.Range.Start:n.Range.End] == covering {
			return n, true
		}
	}
	return contract.Inline{}, false
}

func countKind(a contract.AST, src string, k contract.InlineKind) int {
	n := 0
	for _, x := range inlinesOf(a, src) {
		if x.Kind == k {
			n++
		}
	}
	return n
}

// TestExtensions checks the extension syntaxes of BE-1.7 (docs/04 §4.2.10,
// 08 D1–D6) on the AST.
func TestExtensions(t *testing.T, f ASTFixture) {
	docs := append(append([]string(nil), extSamples...), astCorpus(t)...)
	t.Run("invariants with every extension on", func(t *testing.T) {
		for _, src := range docs {
			a := f.Parse(src, contract.ExtDefault)
			checkAST(t, src, a)
			checkNoHiddenText(t, src, a)
		}
	})
	t.Run("an extension that is off adds nothing", func(t *testing.T) {
		for _, src := range docs {
			plain := dumpAST(f.Parse(src, 0))
			for _, e := range allExts {
				if e == contract.ExtFrontMatter {
					continue
				}
				a := f.Parse(src, e)
				for k, needs := range extKinds {
					if needs != e && countKind(a, src, k) > 0 {
						t.Errorf("only %d on, yet %d nodes of kind %d in %q", e, countKind(a, src, k), k, src)
					}
				}
			}
			a := f.Parse(src, 0)
			for k := range extKinds {
				if n := countKind(a, src, k); n > 0 {
					t.Errorf("no extension on, yet %d nodes of kind %d in %q", n, k, src)
				}
			}
			if hasLeaf(a, contract.BlockTOC) {
				t.Errorf("no extension on, yet a TOC in %q", src)
			}
			if dumpAST(a) != plain {
				t.Fatal("parsing is not deterministic")
			}
		}
	})
	t.Run("incremental equals full with every extension on", func(t *testing.T) {
		for _, seed := range []int64{4, 20261102} {
			randomExtEdits(t, f, seed, 1500, func(a contract.AST, src string) {
				if got, want := dumpAST(a), dumpAST(f.Parse(src, contract.ExtDefault)); got != want {
					t.Fatalf("seed %d, source %q:\nincremental %s\nfull %s", seed, src, got, want)
				}
			})
		}
	})

	type want struct {
		kind        contract.InlineKind
		covering    string // the node's Range
		open, close string // its delimiters' source
		text, dest  string
	}
	for _, tt := range []struct {
		name string
		exts contract.Extensions
		src  string
		want []want
		none []string // sources that must not be an extension node
	}{
		{"highlight", contract.ExtHighlight, "a ==高亮== b\n", []want{{contract.InlineHighlight, "==高亮==", "==", "==", "", ""}}, nil},
		{"highlight wraps other inlines", contract.ExtHighlight, "==a **b** c==\n", []want{{contract.InlineHighlight, "==a **b** c==", "==", "==", "", ""}}, nil},
		{"highlight needs no space inside its delimiters", contract.ExtHighlight, "a == b == c\n", nil, []string{"== b =="}},
		{"no highlight in code or after an escape", contract.ExtHighlight, "`==a==` \\==b==\n", nil, []string{"==a==", "==b==", "\\==b=="}},
		{"superscript", contract.ExtSupSub, "x^2^\n", []want{{contract.InlineSup, "^2^", "^", "^", "", ""}}, nil},
		{"superscript has no spaces", contract.ExtSupSub, "x^a b^\n", nil, []string{"^a b^"}},
		{"subscript", contract.ExtTildeSub, "H~2~O\n", []want{{contract.InlineSub, "~2~", "~", "~", "", ""}}, nil},
		{"double tilde is still a strike", contract.ExtTildeSub, "~~删~~\n", []want{{contract.InlineStrike, "~~删~~", "~~", "~~", "", ""}}, nil},
		{"without the subscript, one tilde is a strike", 0, "~a~\n", []want{{contract.InlineStrike, "~a~", "~", "~", "", ""}}, nil},
		{"emoji", contract.ExtEmoji, "a :smile: b\n", []want{{contract.InlineEmoji, ":smile:", "", "", "😄", ""}}, nil},
		{"unknown shortcodes stay text", contract.ExtEmoji, ":not_an_emoji: 10:30:00\n", nil, []string{":not_an_emoji:", ":30:"}},
		{"wiki link", contract.ExtWikiLink, "[[页面]]\n", []want{{contract.InlineWikiLink, "[[页面]]", "[[", "]]", "", "页面"}}, nil},
		{"wiki link with an alias", contract.ExtWikiLink, "[[页面|别名]]\n", []want{{contract.InlineWikiLink, "[[页面|别名]]", "[[页面|", "]]", "", "页面"}}, nil},
		{"wiki link to a heading", contract.ExtWikiLink, "[[页面#标题]]\n", []want{{contract.InlineWikiLink, "[[页面#标题]]", "[[", "]]", "", "页面#标题"}}, nil},
		{"empty wiki link is text", contract.ExtWikiLink, "[[]]\n", nil, []string{"[[]]"}},
		{"inline math", contract.ExtDollarMath, "公式 $x^2$ 完\n", []want{{contract.InlineMath, "$x^2$", "$", "$", "x^2", ""}}, nil},
		{"dollar amounts are text", contract.ExtDollarMath, "价格 $5 和 $6\n", nil, []string{"$5 和 $"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			src := tt.src
			a := f.Parse(src, tt.exts)
			for _, w := range tt.want {
				n, ok := findInline(a, src, w.kind, w.covering)
				if !ok {
					t.Fatalf("no node of kind %d over %q in %s", w.kind, w.covering, dumpInlines(a, src))
				}
				if got := src[n.Open.Start:n.Open.End]; got != w.open {
					t.Errorf("open %q, want %q", got, w.open)
				}
				if got := src[n.Close.Start:n.Close.End]; got != w.close {
					t.Errorf("close %q, want %q", got, w.close)
				}
				if w.text != "" && n.Text != w.text {
					t.Errorf("text %q, want %q", n.Text, w.text)
				}
				if w.dest != "" && n.Dest != w.dest {
					t.Errorf("dest %q, want %q", n.Dest, w.dest)
				}
			}
			for _, s := range tt.none {
				for _, n := range inlinesOf(a, src) {
					if _, ext := extKinds[n.Kind]; (ext || n.Kind == contract.InlineMath) && src[n.Range.Start:n.Range.End] == s {
						t.Errorf("%q became a node of kind %d", s, n.Kind)
					}
				}
			}
		})
	}

	t.Run("TOC is a paragraph that is only [TOC]", func(t *testing.T) {
		src := "[TOC]\n\n[toc]\n\n[TOC] 后面有字\n\n> [TOC]\n"
		a := f.Parse(src, contract.ExtTOC)
		var tocs []string
		for leaf := range a.Leaves() {
			if b := a.Block(leaf); b.Kind == contract.BlockTOC {
				tocs = append(tocs, src[b.Range.Start:b.Range.End])
			}
		}
		if got := strings.Join(tocs, ","); got != "[TOC],[toc]" {
			t.Errorf("TOC blocks %q", got)
		}
		if hasLeaf(f.Parse(src, 0), contract.BlockTOC) {
			t.Error("a TOC with the extension off")
		}
	})
}

func dumpInlines(a contract.AST, src string) string {
	var b strings.Builder
	for _, n := range inlinesOf(a, src) {
		fmt.Fprintf(&b, "\n  kind %d %q", n.Kind, src[n.Range.Start:n.Range.End])
	}
	return b.String()
}

// extFragments are typed into documents with every extension on.
var extFragments = []string{"==", "^", "~", ":smile:", ":", "[[", "]]", "|", "[TOC]\n", "$", "$$", "---\n", "a: b\n"}

// randomExtEdits is randomEdits on a document of extension samples, with
// fragments that open and close extension syntax.
func randomExtEdits(t *testing.T, f ASTFixture, seed int64, steps int, check func(contract.AST, string)) {
	t.Helper()
	fragments := append(append([]string(nil), editFragments...), extFragments...)
	src := strings.Join(extSamples, "\n")
	a := f.Parse(src, contract.ExtDefault)
	rng := rand.New(rand.NewSource(seed))
	for range steps {
		edit := randomEditOf(rng, src, fragments)
		if edit.Range.Empty() && edit.Text == "" {
			continue
		}
		a, src = f.Edit(a, src, []contract.TextEdit{edit})
		check(a, src)
	}
}

// TestEditingExtensions checks the commands of BE-1.7 (08 §6): highlight,
// superscript and subscript toggle like strong; math, Mermaid and the TOC are
// blocks; Front Matter goes to the top. Cases use ‸ for the caret and run with
// LF and CRLF.
func TestEditingExtensions(t *testing.T, f EditingFixture) {
	for _, tt := range []struct {
		name, before string
		cmd          contract.EditCommand
		after        string // "" when the command must not apply
	}{
		{"highlight on", "‸a‸", contract.EditCommand{ID: contract.CmdFormatHighlight}, "==‸a‸=="},
		{"highlight off", "==‸a‸==", contract.EditCommand{ID: contract.CmdFormatHighlight}, "‸a‸"},
		{"highlight at a caret inserts a pair", "a ‸b", contract.EditCommand{ID: contract.CmdFormatHighlight}, "a ==‸==b"},
		{"superscript on", "x‸2‸", contract.EditCommand{ID: contract.CmdFormatSup}, "x^‸2‸^"},
		{"superscript off", "x^‸2‸^", contract.EditCommand{ID: contract.CmdFormatSup}, "x‸2‸"},
		{"subscript on", "H‸2‸O", contract.EditCommand{ID: contract.CmdFormatSub}, "H~‸2‸~O"},
		{"subscript off", "H~‸2‸~O", contract.EditCommand{ID: contract.CmdFormatSub}, "H‸2‸O"},
		{"no superscript over a space", "‸a b‸", contract.EditCommand{ID: contract.CmdFormatSup}, ""},
		{"math block on an empty line", "x\n\n‸\n\ny", contract.EditCommand{ID: contract.CmdBlockMath}, "x\n\n$$\n‸\n$$\n\ny"},
		{"math block around a paragraph", "x + y‸", contract.EditCommand{ID: contract.CmdBlockMath}, "$$\nx + y‸\n$$"},
		{"Mermaid block on an empty line", "x\n\n‸\n\ny", contract.EditCommand{ID: contract.CmdBlockMermaid}, "x\n\n```mermaid\n‸\n```\n\ny"},
		{"TOC on an empty line", "x\n\n‸\n\ny", contract.EditCommand{ID: contract.CmdBlockTOC}, "x\n\n[TOC]‸\n\ny"},
		{"TOC after a paragraph", "ab‸\n\ny", contract.EditCommand{ID: contract.CmdBlockTOC}, "ab\n\n[TOC]‸\n\ny"},
		{"front matter at the top", "# a‸\n", contract.EditCommand{ID: contract.CmdInsertFrontMatter}, "---\n‸\n---\n\n# a\n"},
		{"front matter that exists takes the caret", "---\nx: 1\n---\n\n# a‸\n", contract.EditCommand{ID: contract.CmdInsertFrontMatter}, "---\nx: 1‸\n---\n\n# a\n"},
	} {
		for _, dominant := range []string{"\n", "\r\n"} {
			name := tt.name
			if dominant != "\n" {
				name += " CRLF"
			}
			t.Run(name, func(t *testing.T) {
				src, sel := markedWith(strings.ReplaceAll(tt.before, "\n", dominant))
				tx, ok, err := f.Editing.Run(f.Context(src, sel, dominant), tt.cmd)
				if err != nil {
					t.Fatal(err)
				}
				switch {
				case tt.after == "" && ok:
					out, caret := applied(t, src, sel, tx)
					t.Fatalf("applied, giving %q", showWith(out, caret))
				case tt.after == "":
					return
				case !ok:
					t.Fatal("not applied")
				}
				out, caret := applied(t, src, sel, tx)
				if got, want := showWith(out, caret), strings.ReplaceAll(tt.after, "\n", dominant); got != want {
					t.Errorf("got %q, want %q", got, want)
				}
			})
		}
	}
	t.Run("Formatting reports highlight, superscript and subscript", func(t *testing.T) {
		src, sel := markedWith("==‸a‸==")
		if got := f.Editing.Formatting(f.Context(src, sel, "\n")); got.Highlight != contract.On {
			t.Errorf("highlight %+v", got)
		}
		src, sel = markedWith("x^‸2‸^")
		if got := f.Editing.Formatting(f.Context(src, sel, "\n")); got.Sup != contract.On || got.Sub != contract.Off {
			t.Errorf("superscript %+v", got)
		}
		src, sel = markedWith("H~‸2‸~O")
		if got := f.Editing.Formatting(f.Context(src, sel, "\n")); got.Sub != contract.On {
			t.Errorf("subscript %+v", got)
		}
	})
}
