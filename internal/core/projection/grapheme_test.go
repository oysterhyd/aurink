package projection

import (
	"bufio"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"aurink/internal/contract"
)

func literalProjection(s string) contract.BlockProjection {
	return contract.BlockProjection{Segments: []contract.Segment{
		{Kind: contract.SegHidden},
		{Kind: contract.SegText, Src: contract.Range{End: len(s)}, Display: s},
		{Kind: contract.SegHidden, Src: contract.Range{Start: len(s), End: len(s)}},
	}}
}

func TestUnicodeGraphemeConformance(t *testing.T) {
	f, err := os.Open("testdata/GraphemeBreakTest.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	count := 0
	pr := New()
	for scanner.Scan() {
		line, _, _ := strings.Cut(scanner.Text(), "#")
		if strings.TrimSpace(line) == "" {
			continue
		}
		var source strings.Builder
		var boundaries []int
		for _, tok := range strings.Fields(line) {
			if tok == "÷" {
				boundaries = append(boundaries, source.Len())
				continue
			}
			if tok == "×" {
				continue
			}
			r, err := strconv.ParseUint(tok, 16, 32)
			if err != nil {
				t.Fatal(err)
			}
			source.WriteRune(rune(r))
		}
		s := source.String()
		p := literalProjection(s)
		var state graphemeState
		var got []int
		for at, r := range s {
			if state.push(r) {
				got = append(got, at)
			}
		}
		got = append(got, len(s))
		if !slices.Equal(got, boundaries) {
			t.Fatalf("%s: boundaries %v, want %v", line, got, boundaries)
		}
		for off := 0; off <= len(s); off++ {
			i, exact := slices.BinarySearch(boundaries, off)
			up, down := boundaries[i], boundaries[i]
			if !exact {
				up = boundaries[i-1]
			}
			for _, tc := range []struct {
				aff  contract.Affinity
				want int
			}{{contract.Upstream, up}, {contract.Downstream, down}} {
				if got := pr.Snap(&p, off, tc.aff); got != tc.want {
					t.Fatalf("%s at %d affinity %d: Snap %d, want %d", line, off, tc.aff, got, tc.want)
				}
			}
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count < 700 {
		t.Fatalf("only %d Unicode cases", count)
	}
	t.Logf("%d Unicode 17.0 extended grapheme cases", count)
}

func TestGraphemesAcrossSegments(t *testing.T) {
	for _, src := range []string{"a**\u0301**z", "*a*\u0301z", "a&#x301;z", "👩**\u200d👩**z", "🇨**🇳**z", "क्**क**z", "e\u0301 👨‍👩‍👧‍👦 🇺🇳 👍🏽 1️⃣"} {
		text, ast := parse(t, src)
		p := New().Project(ast, text, ast.LeafAt(0), contract.Reveal{Mode: contract.RevealNone, Caret: -1})
		display := New().DisplayText(&p)
		var state graphemeState
		allowed := map[int]bool{}
		u := 0
		for _, r := range display {
			if state.push(r) {
				allowed[u] = true
			}
			u++
			if r > 0xFFFF {
				u++
			}
		}
		allowed[u] = true
		for off := 0; off <= len(src); off++ {
			for _, aff := range []contract.Affinity{contract.Upstream, contract.Downstream} {
				d := New().ToDisplay(&p, off, aff)
				if !allowed[d] {
					t.Fatalf("%q at %d maps inside grapheme at %d", src, off, d)
				}
				s := New().Snap(&p, off, aff)
				if s < len(src) && !utf8.RuneStart(src[s]) {
					t.Fatalf("Snap splits rune: %q at %d", src, s)
				}
			}
		}
	}
}

func TestInvalidUTF8Mapping(t *testing.T) {
	p := literalProjection("a\xff\x80b")
	for off := 0; off <= 4; off++ {
		for _, aff := range []contract.Affinity{contract.Upstream, contract.Downstream} {
			if got := New().Snap(&p, off, aff); got != off {
				t.Errorf("Snap(%d, %d) = %d", off, aff, got)
			}
		}
	}
}
