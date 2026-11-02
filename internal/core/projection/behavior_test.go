package projection

import (
	"math/rand"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
	"unsafe"

	"aurink/internal/contract"
	"aurink/tests/corpus"
)

func TestUTF16AndAffinity(t *testing.T) {
	pr := New()
	p := literalProjection("a😀中e\u0301")
	for _, tc := range []struct{ src, up, down int }{
		{-1, 0, 0}, {0, 0, 0}, {1, 1, 1}, {2, 1, 3}, {4, 1, 3}, {5, 3, 3}, {6, 3, 4}, {8, 4, 4}, {9, 4, 6}, {10, 4, 6}, {11, 6, 6}, {100, 6, 6},
	} {
		if got := pr.ToDisplay(&p, tc.src, contract.Upstream); got != tc.up {
			t.Errorf("ToDisplay(%d, up)=%d, want %d", tc.src, got, tc.up)
		}
		if got := pr.ToDisplay(&p, tc.src, contract.Downstream); got != tc.down {
			t.Errorf("ToDisplay(%d, down)=%d, want %d", tc.src, got, tc.down)
		}
	}
	for _, tc := range []struct{ d, up, down int }{{-5, 0, 0}, {0, 0, 0}, {1, 1, 1}, {2, 1, 5}, {3, 5, 5}, {4, 8, 8}, {5, 8, 11}, {6, 11, 11}, {99, 11, 11}} {
		if got := pr.ToSource(&p, tc.d, contract.Upstream); got != tc.up {
			t.Errorf("ToSource(%d, up)=%d, want %d", tc.d, got, tc.up)
		}
		if got := pr.ToSource(&p, tc.d, contract.Downstream); got != tc.down {
			t.Errorf("ToSource(%d, down)=%d, want %d", tc.d, got, tc.down)
		}
	}
	tx, a := parse(t, "> **a**\r\n")
	p = pr.Project(a, tx, a.LeafAt(0), contract.Reveal{Mode: contract.RevealNone, Caret: -1})
	for _, tc := range []struct{ d, up, down int }{{0, 2, 4}, {1, 5, 7}} {
		if got := pr.ToSource(&p, tc.d, contract.Upstream); got != tc.up {
			t.Errorf("hidden up %d: %d", tc.d, got)
		}
		if got := pr.ToSource(&p, tc.d, contract.Downstream); got != tc.down {
			t.Errorf("hidden down %d: %d", tc.d, got)
		}
	}
	for _, src := range []string{"&NotEqualTilde;", "![图](a.png)", "a\r\n> b"} {
		tx, a := parse(t, src)
		p := pr.Project(a, tx, a.LeafAt(0), contract.Reveal{Mode: contract.RevealNone, Caret: -1})
		for _, s := range p.Segments {
			if s.Kind != contract.SegObject && s.Kind != contract.SegReplaced {
				continue
			}
			for at := s.Src.Start + 1; at < s.Src.End; at++ {
				for _, aff := range []contract.Affinity{contract.Upstream, contract.Downstream} {
					got := pr.Snap(&p, at, aff)
					if got > s.Src.Start && got < s.Src.End {
						t.Fatalf("atomic segment %+v split at %d", s, got)
					}
				}
			}
		}
	}
	if got := pr.ToSource(&contract.BlockProjection{}, 1, contract.Downstream); got != 0 {
		t.Fatal(got)
	}
}

func TestSignatureFields(t *testing.T) {
	base := contract.BlockProjection{Kind: contract.BlockParagraph, Segments: []contract.Segment{{Kind: contract.SegText, Display: "字", Object: &contract.ObjectRef{Kind: contract.ObjImage}}}, Containers: []contract.ContainerDecor{{Kind: contract.BlockItem}}}
	changes := []struct {
		name   string
		change func(*contract.BlockProjection)
	}{
		{"kind", func(p *contract.BlockProjection) { p.Kind++ }},
		{"level", func(p *contract.BlockProjection) { p.Level++ }},
		{"lang", func(p *contract.BlockProjection) { p.Lang = "go" }},
		{"revealed", func(p *contract.BlockProjection) { p.Revealed = true }},
		{"container-kind", func(p *contract.BlockProjection) { p.Containers[0].Kind++ }},
		{"depth", func(p *contract.BlockProjection) { p.Containers[0].Depth++ }},
		{"ordered", func(p *contract.BlockProjection) { p.Containers[0].Ordered = true }},
		{"number", func(p *contract.BlockProjection) { p.Containers[0].Number++ }},
		{"marker", func(p *contract.BlockProjection) { p.Containers[0].Marker = '+' }},
		{"task", func(p *contract.BlockProjection) { p.Containers[0].Task = contract.TaskDone }},
		{"first", func(p *contract.BlockProjection) { p.Containers[0].First = true }},
		{"label", func(p *contract.BlockProjection) { p.Containers[0].Label = "n" }},
		{"containers", func(p *contract.BlockProjection) { p.Containers = nil }},
		{"segment-kind", func(p *contract.BlockProjection) { p.Segments[0].Kind++ }},
		{"display", func(p *contract.BlockProjection) { p.Segments[0].Display = "学" }},
		{"style", func(p *contract.BlockProjection) { p.Segments[0].Style = contract.StyleKbd }},
		{"segments", func(p *contract.BlockProjection) { p.Segments = append(p.Segments, contract.Segment{}) }},
		{"object-nil", func(p *contract.BlockProjection) { p.Segments[0].Object = nil }},
		{"object-kind", func(p *contract.BlockProjection) { p.Segments[0].Object.Kind++ }},
		{"source", func(p *contract.BlockProjection) { p.Segments[0].Object.Source = "a.png" }},
		{"alt", func(p *contract.BlockProjection) { p.Segments[0].Object.Alt = "a" }},
		{"title", func(p *contract.BlockProjection) { p.Segments[0].Object.Title = "t" }},
		{"object-lang", func(p *contract.BlockProjection) { p.Segments[0].Object.Lang = "mermaid" }},
		{"display-object", func(p *contract.BlockProjection) { p.Segments[0].Object.Display = true }},
		{"checked", func(p *contract.BlockProjection) { p.Segments[0].Object.Checked = true }},
	}
	for _, tc := range changes {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			p.Segments = slices.Clone(base.Segments)
			p.Containers = slices.Clone(base.Containers)
			o := *base.Segments[0].Object
			p.Segments[0].Object = &o
			tc.change(&p)
			if signature(&base) == signature(&p) {
				t.Fatal("layout change did not change Sig")
			}
		})
	}
	p := base
	p.Block = 99
	p.Index = 10
	p.Segments = slices.Clone(base.Segments)
	p.Containers = slices.Clone(base.Containers)
	p.Segments[0].Src = contract.Range{Start: 100, End: 200}
	p.Segments[0].Link = "/another"
	p.Containers[0].Block = 88
	p.Containers[0].TaskSrc = contract.Range{Start: 50, End: 53}
	if signature(&base) != signature(&p) {
		t.Fatal("identity or mapping changed Sig")
	}
	pr := New()
	var sig uint64
	for _, src := range []string{"&amp;", "&#x26;"} {
		tx, a := parse(t, src)
		p := pr.Project(a, tx, a.LeafAt(0), contract.Reveal{Caret: -1})
		if sig != 0 && p.Sig != sig {
			t.Fatal("equivalent entities changed layout signature")
		}
		sig = p.Sig
		if got := pr.ToSource(&p, 1, contract.Downstream); got != len(src) {
			t.Fatal("Sig used as source mapping")
		}
	}
}

func TestDigestNumbers(t *testing.T) {
	for _, initial := range []digest{0, 1, 14695981039346656037, ^digest(0)} {
		for _, n := range []uint64{0, 1, 256, ^uint64(0)} {
			want, remaining := initial, n
			for range 8 {
				want = (want ^ digest(byte(remaining))) * 1099511628211
				remaining >>= 8
			}
			got := initial
			got.number(n)
			if got != want {
				t.Fatalf("digest(%x, %x) = %x, want %x", initial, n, got, want)
			}
		}
	}
}

func TestDefinitionsOwnershipAndPurity(t *testing.T) {
	pr := New()
	for _, dest := range []string{"one", "two"} {
		src := "[^n]: label\n\n[x][ref] ![图](a.png \"t\")\n\n[ref]: " + dest + "\n"
		tx, a := parse(t, src)
		leaf := a.LeafAt(strings.Index(src, "[x]"))
		p := pr.Project(a, tx, leaf, contract.Reveal{Caret: -1})
		found := false
		check := func(s string) {
			if s == "" {
				return
			}
			at := uintptr(unsafe.Pointer(unsafe.StringData(s)))
			start := uintptr(unsafe.Pointer(unsafe.StringData(tx.String())))
			if at >= start && at < start+uintptr(len(src)) {
				t.Fatalf("cached string %q points into source", s)
			}
		}
		check(p.Lang)
		for _, s := range p.Segments {
			check(s.Display)
			check(s.Link)
			if s.Style&contract.StyleLink != 0 {
				found = true
				if s.Link != dest {
					t.Fatalf("stale reference: %q", s.Link)
				}
			}
			if s.Object != nil {
				check(s.Object.Source)
				check(s.Object.Alt)
				check(s.Object.Title)
			}
		}
		if !found {
			t.Fatal("missing link")
		}
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				for range 10 {
					got := pr.Project(a, tx, leaf, contract.Reveal{Caret: -1})
					if !reflect.DeepEqual(p, got) {
						t.Error("concurrent projection changed output")
					}
				}
			})
		}
		wg.Wait()
		if got := pr.Project(a, tx, 0, contract.Reveal{}); len(got.Segments) != 0 {
			t.Fatal("projected a container")
		}
		if got := pr.Project(a, tx, -1, contract.Reveal{}); len(got.Segments) != 0 {
			t.Fatal("projected an invalid leaf")
		}
	}
}

func displayStops(s string) []int {
	var state graphemeState
	var stops []int
	u := 0
	for _, r := range s {
		if state.push(r) {
			stops = append(stops, u)
		}
		u++
		if r > 0xFFFF {
			u++
		}
	}
	return append(stops, u)
}

func walk(t *testing.T, src string) {
	t.Helper()
	pr := New()
	tx, a := parse(t, src)
	for leaf := range a.Leaves() {
		n := a.Block(leaf)
		project := func(caret int) contract.BlockProjection {
			return pr.Project(a, tx, leaf, contract.Reveal{Caret: caret})
		}
		p := project(-1)
		rest := displayStops(pr.DisplayText(&p))
		var visible []contract.Range
		for i := 0; i+1 < len(rest); i++ {
			visible = append(visible, contract.Range{Start: pr.ToSource(&p, rest[i], contract.Downstream), End: pr.ToSource(&p, rest[i+1], contract.Upstream)})
		}
		for _, forward := range []bool{true, false} {
			at, edge, aff := n.Range.Start, n.Range.End, contract.Downstream
			if !forward {
				at, edge, aff = n.Range.End, n.Range.Start, contract.Upstream
			}
			for steps := 0; at != edge; steps++ {
				if steps > 4*(n.Range.Len()+2) {
					t.Fatalf("%q: walk did not terminate", src)
				}
				p := project(at)
				d := pr.ToDisplay(&p, at, aff)
				if got := pr.ToSource(&p, d, aff); got != pr.Snap(&p, at, aff) {
					t.Fatalf("%q: round trip at %d", src, at)
				}
				stops := displayStops(pr.DisplayText(&p))
				i, _ := slices.BinarySearch(stops, d)
				if forward {
					i++
				} else {
					i--
				}
				if i < 0 || i >= len(stops) {
					if pr.ToDisplay(&p, edge, aff) != d {
						t.Fatalf("%q: walk ended before the visible edge at %d", src, at)
					}
					break
				}
				next := pr.ToSource(&p, stops[i], aff)
				if forward && next <= at || !forward && next >= at {
					t.Fatalf("%q: walk stalled %d -> %d", src, at, next)
				}
				crossed := 0
				for _, span := range visible {
					if span.Start >= min(at, next) && span.End <= max(at, next) && !span.Empty() {
						crossed++
					}
				}
				if crossed > 1 {
					t.Fatalf("%q: walk skipped %d visible graphemes: %d -> %d", src, crossed-1, at, next)
				}
				at = next
			}
		}
	}
}

func TestMultilineContainerWalk(t *testing.T) {
	for _, tc := range fixtures {
		walk(t, tc.source)
	}
	for _, src := range []string{
		"> - **甲\r\n>   *👩‍👩‍👧‍👧*\r\n>   乙** end\r\n",
		"> a [多\n> 行](a.md) &amp;\n> `代\n> 码`\n",
		"> 3. first\n>\n>    second **粗**\n>\n> 4. third\n",
		"- [ ] e\u0301 **🇨🇳**\n  \\* &NotEqualTilde;\n",
	} {
		walk(t, src)
	}
	for _, seed := range []int64{1, 17, 2301} {
		walk(t, corpus.Generate(512, seed))
	}
}

func TestRandomProjection(t *testing.T) {
	steps := 40
	if os.Getenv("AURINK_LONG_TESTS") != "" {
		steps = 10000
	}
	parts := []string{"甲", "😀", "e\u0301", "**粗**", "*e*", "[a](b)", "\\*", "&amp;", "\n> ", "\r\n", "`a`", "  \n", "![图](a.png)"}
	for _, seed := range []int64{1, 17, 2301} {
		r := rand.New(rand.NewSource(seed))
		for range steps {
			var src strings.Builder
			for n := 1 + r.Intn(8); n > 0; n-- {
				src.WriteString(parts[r.Intn(len(parts))])
			}
			s := src.String()
			if !utf8.ValidString(s) {
				t.Fatal("invalid fixture")
			}
			walk(t, s)
		}
	}
}
