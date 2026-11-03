package contracttest

import (
	"slices"
	"strings"
	"testing"

	"aurink/internal/contract"
)

// TestProjectionExtensions checks how BE-1.7 shows extension syntax, math,
// diagrams, the TOC and Front Matter (docs/04 §4.3, 08 §6). Objects follow
// images: shown as an object until the caret, a selection or RevealAlways
// reveals the node, and always an object under RevealNone.
func TestProjectionExtensions(t *testing.T, f ProjectionFixture) {
	pr := f.Projector
	mermaid := []string{"mermaid"}
	project := func(src string, exts contract.Extensions, caret int, mode contract.RevealMode) contract.BlockProjection {
		text, ast := f.Parse(src, exts)
		leaf := ast.LeafAt(max(caret, 0))
		return pr.Project(ast, text, leaf, contract.Reveal{Mode: mode, Caret: caret, Exts: exts, ObjectLangs: mermaid})
	}
	at := func(src, s string) int { return strings.Index(src, s) }
	objects := func(p *contract.BlockProjection) []*contract.ObjectRef {
		var out []*contract.ObjectRef
		for _, s := range p.Segments {
			if s.Kind == contract.SegObject {
				out = append(out, s.Object)
			}
		}
		return out
	}
	styleOf := func(p *contract.BlockProjection, display string) (contract.Style, string) {
		for _, s := range p.Segments {
			if s.Display == display {
				return s.Style, s.Link
			}
		}
		return 0, ""
	}

	t.Run("invariants with every extension on", func(t *testing.T) {
		docs := append(append([]string(nil), extSamples...), projectionCorpus(t)...)
		docs = append(docs, "```mermaid\ngraph TD\n  a-->b\n```\n", "$$\nx\n$$\n\n$y$\n")
		for _, src := range docs {
			text, ast := f.Parse(src, contract.ExtDefault)
			for leaf := range ast.Leaves() {
				b := ast.Block(leaf)
				for _, mode := range []contract.RevealMode{contract.RevealInline, contract.RevealBlock, contract.RevealAlways, contract.RevealNone} {
					for _, caret := range []int{-1, b.Range.Start, (b.Range.Start + b.Range.End) / 2, b.Range.End} {
						p := pr.Project(ast, text, leaf, contract.Reveal{Mode: mode, Caret: caret, Exts: contract.ExtDefault, ObjectLangs: mermaid})
						checkProjection(t, pr, src, b, &p, mode)
					}
				}
				walkCaret(t, pr, src, ast, text, leaf)
			}
		}
	})

	for _, tt := range []struct {
		name          string
		src           string
		exts          contract.Extensions
		hidden, shown string // display with the caret away, and inside at `inside`
		inside        string
		style         contract.Style
		styled        string // the display text that carries style
		link          string
	}{
		{"highlight", "a ==b== c\n", contract.ExtHighlight, "a b c", "a ==b== c", "b", contract.StyleHighlight, "b", ""},
		{"superscript", "x^2^ y\n", contract.ExtSupSub, "x2 y", "x^2^ y", "2", contract.StyleSup, "2", ""},
		{"subscript", "H~2~O\n", contract.ExtTildeSub, "H2O", "H~2~O", "2", contract.StyleSub, "2", ""},
		{"emoji", "a :smile: b\n", contract.ExtEmoji, "a 😄 b", "a :smile: b", "smile", 0, "", ""},
		{"wiki link", "看 [[页面]] 吧\n", contract.ExtWikiLink, "看 页面 吧", "看 [[页面]] 吧", "页面", contract.StyleWikiLink, "页面", "页面"},
		{"wiki link with an alias", "看 [[页面|别名]] 吧\n", contract.ExtWikiLink, "看 别名 吧", "看 [[页面|别名]] 吧", "别名", contract.StyleWikiLink, "别名", "页面"},
		{"inline math", "a $x^2$ b\n", contract.ExtDollarMath, "a ￼ b", "a $x^2$ b", "x^2", 0, "", ""},
		{"TOC", "[TOC]\n", contract.ExtTOC, "￼", "[TOC]", "TOC", 0, "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			p := project(tt.src, tt.exts, -1, contract.RevealInline)
			if got := pr.DisplayText(&p); got != tt.hidden {
				t.Errorf("caret away: %q, want %q", got, tt.hidden)
			}
			if tt.styled != "" {
				if st, link := styleOf(&p, tt.styled); st&tt.style == 0 || link != tt.link {
					t.Errorf("%q has style %b and link %q", tt.styled, st, link)
				}
			}
			q := project(tt.src, tt.exts, at(tt.src, tt.inside), contract.RevealInline)
			if got := pr.DisplayText(&q); got != tt.shown {
				t.Errorf("caret inside: %q, want %q", got, tt.shown)
			}
			if p.Sig == q.Sig {
				t.Error("the two displays have the same Sig")
			}
			off := project(tt.src, 0, -1, contract.RevealInline)
			if tt.exts != contract.ExtTOC && tt.exts != contract.ExtDollarMath {
				if got := pr.DisplayText(&off); got == tt.hidden {
					t.Errorf("with the extension off the display is still %q", got)
				}
			}
		})
	}

	t.Run("inline math object", func(t *testing.T) {
		p := project("a $x^2$ b\n", contract.ExtDollarMath, -1, contract.RevealInline)
		if o := objects(&p); len(o) != 1 || o[0].Kind != contract.ObjMath || o[0].Source != "x^2" || o[0].Display {
			t.Errorf("objects %+v", o)
		}
	})
	t.Run("block math is one object", func(t *testing.T) {
		src := "$$\nx + y\n$$\n"
		p := project(src, 0, -1, contract.RevealInline)
		if o := objects(&p); len(o) != 1 || o[0].Kind != contract.ObjMath || o[0].Source != "x + y" || !o[0].Display {
			t.Errorf("objects %+v", o)
		}
		if got := pr.DisplayText(&p); got != "￼" {
			t.Errorf("display %q", got)
		}
		q := project(src, 0, at(src, "y"), contract.RevealInline)
		if o := objects(&q); len(o) != 0 || !strings.Contains(pr.DisplayText(&q), "x + y") {
			t.Errorf("caret inside: objects %+v, display %q", o, pr.DisplayText(&q))
		}
		r := project(src, 0, at(src, "y"), contract.RevealNone)
		if o := objects(&r); len(o) != 1 {
			t.Errorf("reading mode shows %d objects", len(o))
		}
	})
	t.Run("a diagram is one object when its language asks for it", func(t *testing.T) {
		src := "```mermaid\ngraph TD\n  a-->b\n```\n"
		p := project(src, 0, -1, contract.RevealInline)
		if o := objects(&p); len(o) != 1 || o[0].Kind != contract.ObjDiagram || o[0].Lang != "mermaid" || o[0].Source != "graph TD\n  a-->b" || !o[0].Display {
			t.Errorf("objects %+v", o)
		}
		text, ast := f.Parse(src, 0)
		plain := pr.Project(ast, text, 0+ast.LeafAt(0), contract.Reveal{Mode: contract.RevealInline, Caret: -1})
		if o := objects(&plain); len(o) != 0 {
			t.Errorf("without ObjectLangs: objects %+v", o)
		}
		q := project(src, 0, at(src, "a-->"), contract.RevealInline)
		if o := objects(&q); len(o) != 0 {
			t.Errorf("caret inside: objects %+v", o)
		}
	})
	t.Run("front matter header", func(t *testing.T) {
		src := "---\ntitle: Aurink 产品说明\ntags: [markdown, editor]\ndate: 2026-10-31\n---\n\n# h\n"
		p := project(src, contract.ExtFrontMatter, -1, contract.RevealInline)
		o := objects(&p)
		if len(o) != 1 || o[0].Kind != contract.ObjFrontMatter || o[0].Meta == nil {
			t.Fatalf("objects %+v", o)
		}
		if m := o[0].Meta; m.Kicker != "Aurink 产品说明" || !slices.Equal(m.Tags, []string{"markdown", "editor"}) || m.Date != "2026-10-31" {
			t.Errorf("meta %+v", m)
		}
		block := "---\nkicker: K\nkeywords:\n  - a\n  - \"b c\"\n---\n"
		p = project(block, contract.ExtFrontMatter, -1, contract.RevealInline)
		if o := objects(&p); len(o) != 1 || o[0].Meta == nil || o[0].Meta.Kicker != "K" || !slices.Equal(o[0].Meta.Tags, []string{"a", "b c"}) {
			t.Errorf("block list: objects %+v", o)
		}
		q := project(src, contract.ExtFrontMatter, at(src, "tags"), contract.RevealInline)
		if o := objects(&q); len(o) != 0 {
			t.Errorf("caret inside: objects %+v", o)
		}
		bad := "---\na: [unclosed\n  : x\n---\n"
		r := project(bad, contract.ExtFrontMatter, -1, contract.RevealInline)
		if o := objects(&r); len(o) != 0 {
			t.Errorf("YAML outside the subset: objects %+v", o)
		}
	})
}
