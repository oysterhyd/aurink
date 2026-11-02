package projection

import (
	"strings"
	"testing"

	"aurink/internal/contract"
)

func segmentAt(t *testing.T, p contract.BlockProjection, start, end int) contract.Segment {
	t.Helper()
	for _, s := range p.Segments {
		if s.Src == (contract.Range{Start: start, End: end}) {
			return s
		}
	}
	t.Fatalf("no segment [%d,%d): %+v", start, end, p.Segments)
	return contract.Segment{}
}

func checkCoverage(t *testing.T, src string, block *contract.Block, p contract.BlockProjection) {
	t.Helper()
	at := block.Lead.Start
	for _, s := range p.Segments {
		if s.Src.Start != at || s.Src.End < at {
			t.Fatalf("segment coverage at %d: %+v", at, p.Segments)
		}
		if (s.Kind == contract.SegText || s.Kind == contract.SegMarker) && s.Display != src[s.Src.Start:s.Src.End] {
			t.Fatalf("source and display differ: %+v", s)
		}
		at = s.Src.End
	}
	if at != block.Trailer.End {
		t.Fatalf("coverage ended at %d, want %d", at, block.Trailer.End)
	}
}

func TestHTMLTagPairs(t *testing.T) {
	for _, tc := range []struct {
		tag   string
		style contract.Style
	}{
		{"b", contract.StyleStrong}, {"i", contract.StyleEmphasis}, {"u", contract.StyleUnderline},
		{"s", contract.StyleStrike}, {"kbd", contract.StyleKbd}, {"mark", contract.StyleHighlight},
		{"sub", contract.StyleSub}, {"sup", contract.StyleSup}, {"KbD", contract.StyleKbd},
	} {
		t.Run(tc.tag, func(t *testing.T) {
			open, close := "<"+tc.tag+">", "</"+strings.ToUpper(tc.tag)+">"
			src := "p " + open + "中文" + close + " q\n"
			tx, ast := parse(t, src)
			leaf := ast.LeafAt(0)
			start, content := 2, 2+len(open)
			end := content + len("中文")
			for _, tcReveal := range []struct {
				name string
				r    contract.Reveal
				show bool
			}{
				{"rest", contract.Reveal{Caret: -1}, false},
				{"before", contract.Reveal{Caret: start - 1}, false},
				{"open boundary", contract.Reveal{Caret: start}, true},
				{"inside", contract.Reveal{Caret: content}, true},
				{"close boundary", contract.Reveal{Caret: end + len(close)}, true},
				{"after", contract.Reveal{Caret: end + len(close) + 1}, false},
				{"selection", contract.Reveal{Caret: -1, Selection: contract.Range{Start: content, End: end}}, true},
				{"selection touching", contract.Reveal{Caret: -1, Selection: contract.Range{Start: start - 1, End: start}}, false},
				{"always", contract.Reveal{Mode: contract.RevealAlways, Caret: -1}, true},
				{"none", contract.Reveal{Mode: contract.RevealNone, Caret: start}, false},
				{"block", contract.Reveal{Mode: contract.RevealBlock, Caret: 0}, true},
			} {
				t.Run(tcReveal.name, func(t *testing.T) {
					p := New().Project(ast, tx, leaf, tcReveal.r)
					checkCoverage(t, src, ast.Block(leaf), p)
					want := "p 中文 q"
					kind := contract.SegHidden
					if tcReveal.show {
						want, kind = strings.TrimSuffix(src, "\n"), contract.SegMarker
					}
					if got := New().DisplayText(&p); got != want {
						t.Fatalf("display %q, want %q", got, want)
					}
					for _, r := range []contract.Range{{Start: start, End: content}, {Start: end, End: end + len(close)}} {
						s := segmentAt(t, p, r.Start, r.End)
						if s.Kind != kind || tcReveal.show && s.Style != contract.StyleHTML|contract.StyleMarker {
							t.Fatalf("tag: %+v, want kind %v", s, kind)
						}
					}
					if s := segmentAt(t, p, content, end); s.Kind != contract.SegText || s.Style&tc.style == 0 {
						t.Fatalf("content: %+v, want style %v", s, tc.style)
					}
				})
			}
		})
	}
}

func TestHTMLNestingAndSiblings(t *testing.T) {
	src := "p <b>bold <i>both</i> [中文](/url) *md*</b> q\n"
	tx, ast := parse(t, src)
	leaf := ast.LeafAt(0)
	p := New().Project(ast, tx, leaf, contract.Reveal{Caret: -1})
	checkCoverage(t, src, ast.Block(leaf), p)
	if got := New().DisplayText(&p); got != "p bold both 中文 md q" {
		t.Fatalf("display %q", got)
	}
	for _, tc := range []struct {
		text  string
		style contract.Style
		link  string
	}{
		{"bold ", contract.StyleStrong, ""},
		{"both", contract.StyleStrong | contract.StyleEmphasis, ""},
		{"中文", contract.StyleStrong | contract.StyleLink, "/url"},
		{"md", contract.StyleStrong | contract.StyleEmphasis, ""},
	} {
		start := strings.Index(src, tc.text)
		s := segmentAt(t, p, start, start+len(tc.text))
		if s.Style&tc.style != tc.style || s.Link != tc.link {
			t.Errorf("%q: %+v, want style %v and link %q", tc.text, s, tc.style, tc.link)
		}
	}

	for _, src := range []string{"p <b>*x</b>* q\n", "p [<b>x](/url)</b> q\n"} {
		t.Run(src, func(t *testing.T) {
			tx, ast := parse(t, src)
			p := New().Project(ast, tx, ast.LeafAt(0), contract.Reveal{Caret: -1})
			if got := New().DisplayText(&p); got != "p <b>x</b> q" {
				t.Fatalf("tags in different sibling lists paired: %q", got)
			}
		})
	}
}

func TestHTMLRepeatedAndCrossingPairs(t *testing.T) {
	for _, tc := range []struct {
		src    string
		styles map[string]contract.Style
	}{
		{"p <b>甲<b>乙</b>丙</b>丁\n", map[string]contract.Style{"甲": contract.StyleStrong, "乙": contract.StyleStrong, "丙": contract.StyleStrong, "丁": 0}},
		{"p <b>甲<i>乙</b>丙</i>丁\n", map[string]contract.Style{"甲": contract.StyleStrong, "乙": contract.StyleStrong | contract.StyleEmphasis, "丙": contract.StyleEmphasis, "丁": 0}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			tx, ast := parse(t, tc.src)
			leaf := ast.LeafAt(0)
			p := New().Project(ast, tx, leaf, contract.Reveal{Caret: -1})
			checkCoverage(t, tc.src, ast.Block(leaf), p)
			if got := New().DisplayText(&p); got != "p 甲乙丙丁" {
				t.Fatalf("display %q", got)
			}
			for text, style := range tc.styles {
				at := strings.Index(tc.src, text)
				if s := segmentAt(t, p, at, at+len(text)); s.Style != style {
					t.Errorf("%s: style %v, want %v", text, s.Style, style)
				}
			}
		})
	}
}

func TestHTMLUnrecognizedAndUnpaired(t *testing.T) {
	for _, raw := range []string{
		"<b>x", "x</b>", "<b>x</i>", "<b class=\"x\">x</b>", "<b>x</b >",
		"<span>x</span>", "<!-- comment -->", "<br >", "<br  />", "<br class=\"x\">", "<b/>x</b>",
	} {
		t.Run(raw, func(t *testing.T) {
			src := "p " + raw + " q\n"
			tx, ast := parse(t, src)
			leaf := ast.LeafAt(0)
			for _, r := range []contract.Reveal{{Caret: -1}, {Mode: contract.RevealAlways, Caret: -1}} {
				p := New().Project(ast, tx, leaf, r)
				checkCoverage(t, src, ast.Block(leaf), p)
				if got := New().DisplayText(&p); got != strings.TrimSuffix(src, "\n") {
					t.Errorf("display %q, want source", got)
				}
			}
		})
	}
	src := "<div>\n<b>x</b>\n</div>\n"
	tx, ast := parse(t, src)
	leaf := ast.LeafAt(0)
	p := New().Project(ast, tx, leaf, contract.Reveal{Caret: -1})
	if ast.Block(leaf).Kind != contract.BlockHTML || New().DisplayText(&p) != strings.TrimSuffix(src, "\n") {
		t.Fatalf("HTML block changed: %+v", p)
	}
}

func TestHTMLBreakAndSoftLine(t *testing.T) {
	for _, tag := range []string{"<br>", "<br/>", "<br />", "<BR>", "<Br/>", "<bR />"} {
		t.Run(tag, func(t *testing.T) {
			src := "p " + tag + " q\n"
			tx, ast := parse(t, src)
			leaf := ast.LeafAt(0)
			for _, tc := range []struct {
				r    contract.Reveal
				show bool
			}{
				{contract.Reveal{Caret: -1}, false}, {contract.Reveal{Caret: 2}, true},
				{contract.Reveal{Caret: 2 + len(tag)}, true}, {contract.Reveal{Mode: contract.RevealAlways}, true},
				{contract.Reveal{Mode: contract.RevealNone, Caret: 2}, false},
			} {
				p := New().Project(ast, tx, leaf, tc.r)
				checkCoverage(t, src, ast.Block(leaf), p)
				s := segmentAt(t, p, 2, 2+len(tag))
				if tc.show && (s.Kind != contract.SegText || s.Display != tag || s.Style&(contract.StyleHTML|contract.StyleMarker) != contract.StyleHTML|contract.StyleMarker) ||
					!tc.show && (s.Kind != contract.SegReplaced || s.Display != "\n") {
					t.Errorf("reveal %+v: %+v", tc.r, s)
				}
			}
		})
	}
	for _, src := range []string{"p <kbd>中\n文</kbd> q\n", "> p <kbd>中\n> 文</kbd> q\n"} {
		t.Run(src, func(t *testing.T) {
			tx, ast := parse(t, src)
			leaf := ast.LeafAt(0)
			p := New().Project(ast, tx, leaf, contract.Reveal{Caret: -1})
			checkCoverage(t, src, ast.Block(leaf), p)
			if got := New().DisplayText(&p); got != "p 中\n文 q" {
				t.Fatalf("display %q", got)
			}
			for _, text := range []string{"中", "文", "\n"} {
				at := strings.Index(src, text)
				found := false
				for _, s := range p.Segments {
					if s.Src.Start <= at && s.Src.End >= at+len(text) && s.Style&contract.StyleKbd != 0 {
						found = true
					}
				}
				if !found {
					t.Errorf("%q lacks kbd style: %+v", text, p.Segments)
				}
			}
		})
	}
	src := "p <b>a<br>b</b> q\n"
	tx, ast := parse(t, src)
	p := New().Project(ast, tx, ast.LeafAt(0), contract.Reveal{Caret: -1})
	if s := segmentAt(t, p, strings.Index(src, "<br>"), strings.Index(src, "<br>")+4); s.Kind != contract.SegReplaced || s.Style != contract.StyleStrong {
		t.Errorf("styled break: %+v", s)
	}
}

type noFootnotes struct{ contract.AST }

func (noFootnotes) Footnotes() map[string]int { return nil }

func TestFootnoteReferences(t *testing.T) {
	for _, tc := range []struct{ ref, def string }{
		{"note", "note"}, {"NoTe", "note"}, {"Straße", "STRASSE"},
	} {
		t.Run(tc.ref, func(t *testing.T) {
			src := "p [^" + tc.ref + "] q\n\n[^" + tc.def + "]: note\n"
			tx, ast := parse(t, src)
			leaf := ast.LeafAt(0)
			end := 5 + len(tc.ref)
			for _, r := range []contract.Reveal{{Caret: -1}, {Mode: contract.RevealNone, Caret: 2}} {
				p := New().Project(ast, tx, leaf, r)
				checkCoverage(t, src, ast.Block(leaf), p)
				s := segmentAt(t, p, 2, end)
				if s.Kind != contract.SegObject || s.Display != "\ufffc" || s.Style != contract.StyleFootnoteRef || s.Object == nil || s.Object.Kind != contract.ObjFootnoteRef || s.Object.Source != tc.ref {
					t.Fatalf("footnote: %+v", s)
				}
			}
			for _, r := range []contract.Reveal{
				{Caret: 2}, {Caret: end}, {Caret: -1, Selection: contract.Range{Start: 3, End: 4}},
				{Mode: contract.RevealAlways, Caret: -1}, {Mode: contract.RevealBlock, Caret: 0},
			} {
				p := New().Project(ast, tx, leaf, r)
				s := segmentAt(t, p, 2, end)
				if s.Kind != contract.SegText || s.Display != src[2:end] || s.Style != contract.StyleFootnoteRef|contract.StyleMarker || s.Object != nil {
					t.Fatalf("revealed footnote: %+v", s)
				}
			}
			p := New().Project(noFootnotes{ast}, tx, leaf, contract.Reveal{Caret: -1})
			if s := segmentAt(t, p, 2, end); s.Kind != contract.SegText || s.Object != nil || s.Style != contract.StyleFootnoteRef|contract.StyleMarker {
				t.Fatalf("reference absent from definition table: %+v", s)
			}
		})
	}
	src := "p [^missing] q\n"
	tx, ast := parse(t, src)
	p := New().Project(ast, tx, ast.LeafAt(0), contract.Reveal{Caret: -1})
	if got := New().DisplayText(&p); got != strings.TrimSuffix(src, "\n") {
		t.Fatalf("undefined reference: %q", got)
	}
	for _, s := range p.Segments {
		if s.Kind == contract.SegObject {
			t.Fatalf("undefined reference became object: %+v", s)
		}
	}
	src = "p **a[^n]** q\n\n[^n]: note\n"
	tx, ast = parse(t, src)
	p = New().Project(ast, tx, ast.LeafAt(0), contract.Reveal{Caret: -1})
	at := strings.Index(src, "[^n]")
	if s := segmentAt(t, p, at, at+4); s.Kind != contract.SegObject || s.Style != contract.StyleStrong|contract.StyleFootnoteRef {
		t.Fatalf("styled footnote: %+v", s)
	}
}

func TestFootnoteAndHTMLCaretMapping(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		start     int
		end       int
		footnote  bool
	}{
		{"footnote", "a[^label]z\n\n[^label]: note\n", 1, 9, true},
		{"kbd", "a<kbd>x</kbd>z\n", 1, 13, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pr := New()
			tx, ast := parse(t, tc.src)
			leaf := ast.LeafAt(0)
			for _, mode := range []contract.RevealMode{contract.RevealInline, contract.RevealAlways, contract.RevealNone} {
				for caret := 0; caret <= ast.Block(leaf).Range.End; caret++ {
					p := pr.Project(ast, tx, leaf, contract.Reveal{Mode: mode, Caret: caret})
					checkCoverage(t, tc.src, ast.Block(leaf), p)
					show := mode == contract.RevealAlways || mode == contract.RevealInline && caret >= tc.start && caret <= tc.end
					if tc.footnote {
						s := segmentAt(t, p, tc.start, tc.end)
						if show && (s.Kind != contract.SegText || s.Style != contract.StyleFootnoteRef|contract.StyleMarker) ||
							!show && (s.Kind != contract.SegObject || s.Object == nil || s.Object.Kind != contract.ObjFootnoteRef) {
							t.Fatalf("mode %v caret %d: %+v", mode, caret, s)
						}
					} else {
						want := contract.SegHidden
						if show {
							want = contract.SegMarker
						}
						for _, r := range []contract.Range{{Start: 1, End: 6}, {Start: 7, End: 13}} {
							if s := segmentAt(t, p, r.Start, r.End); s.Kind != want {
								t.Fatalf("mode %v caret %d: %+v, want kind %v", mode, caret, s, want)
							}
						}
						if s := segmentAt(t, p, 6, 7); s.Style&contract.StyleKbd == 0 {
							t.Fatalf("mode %v caret %d lost kbd style: %+v", mode, caret, s)
						}
					}
					for at := tc.start; at <= tc.end; at++ {
						for _, aff := range []contract.Affinity{contract.Upstream, contract.Downstream} {
							d := pr.ToDisplay(&p, at, aff)
							got := pr.ToSource(&p, d, aff)
							if got != pr.Snap(&p, at, aff) {
								t.Fatalf("mode %v caret %d source %d affinity %v did not round trip", mode, caret, at, aff)
							}
							if show && (d != at || got != at) {
								t.Fatalf("revealed mode %v caret %d source %d mapped to %d then %d", mode, caret, at, d, got)
							}
							if !show && tc.footnote && at > tc.start && at < tc.end {
								want := tc.start
								if aff == contract.Downstream {
									want = tc.end
								}
								if got != want {
									t.Fatalf("object source %d affinity %v snapped to %d, want %d", at, aff, got, want)
								}
							}
							if !show && !tc.footnote {
								var hidden contract.Range
								if at >= 1 && at <= 6 {
									hidden = contract.Range{Start: 1, End: 6}
								} else if at >= 7 && at <= 13 {
									hidden = contract.Range{Start: 7, End: 13}
								}
								if !hidden.Empty() {
									want := hidden.Start
									if aff == contract.Downstream {
										want = hidden.End
									}
									if got != want {
										t.Fatalf("hidden source %d affinity %v snapped to %d, want %d", at, aff, got, want)
									}
								}
							}
						}
					}
				}
			}
			walk(t, tc.src)
		})
	}
}
