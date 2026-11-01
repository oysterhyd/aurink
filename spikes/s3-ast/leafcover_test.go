package astspike

import (
	"fmt"
	"math/rand"
	"testing"

	"aurink.local/spikes/contract-rc/contract"
	"github.com/egoist/mygo/ui/markdown/parser"
)

// checkCoverage returns the violated rule of §4.2.8, or "".
func checkCoverage(c *coverage) string {
	pos := 0
	for i, l := range c.leaves {
		switch {
		case l.Lead.Start != pos:
			return fmt.Sprintf("leaf %d Lead.Start=%d, want %d", i, l.Lead.Start, pos)
		case l.Lead.End != l.Range.Start:
			return fmt.Sprintf("leaf %d Lead.End=%d Range.Start=%d", i, l.Lead.End, l.Range.Start)
		case l.Range.End != l.Trailer.Start:
			return fmt.Sprintf("leaf %d Range.End=%d Trailer.Start=%d", i, l.Range.End, l.Trailer.Start)
		case l.Trailer.End < l.Trailer.Start || l.Range.End < l.Range.Start || l.Lead.End < l.Lead.Start:
			return fmt.Sprintf("leaf %d has an inverted range: %+v", i, l)
		}
		pos = l.Trailer.End
	}
	if pos != len(c.src) {
		return fmt.Sprintf("coverage ends at %d of %d", pos, len(c.src))
	}
	return ""
}

// swallowed returns text held by a Lead or Trailer.
func swallowed(c *coverage) string {
	for _, l := range c.leaves {
		for _, r := range []contract.Range{l.Lead, l.Trailer} {
			t := c.src[r.Start:r.End]
			if !structuralRe.MatchString(footnoteRe.ReplaceAllString(t, "")) {
				return t
			}
		}
	}
	return ""
}

func TestCoverageExamples(t *testing.T) {
	for _, tt := range []struct {
		name, src string
		kinds     []contract.BlockKind
	}{
		{"second item marker", "- a\n- b\n", []contract.BlockKind{contract.BlockParagraph, contract.BlockParagraph}},
		{"quote separator", "> a\n>\n> b\n", []contract.BlockKind{contract.BlockParagraph, contract.BlockParagraph}},
		{"empty item", "- a\n- \n", []contract.BlockKind{contract.BlockParagraph, kindBlank}},
		{"link definition", "[x]: /u\n\npara\n", []contract.BlockKind{kindLinkDef, contract.BlockParagraph}},
		{"table", "| a | b |\n|---|---|\n| c | d |\n", []contract.BlockKind{contract.BlockTableCell, contract.BlockTableCell, contract.BlockTableCell, contract.BlockTableCell}},
		{"empty", "", []contract.BlockKind{kindBlank}},
		{"extra blanks", "a\n\n\n\nb\n", []contract.BlockKind{contract.BlockParagraph, kindBlank, kindBlank, contract.BlockParagraph}},
		{"ending separator", "a\n\n", []contract.BlockKind{contract.BlockParagraph, kindBlank}},
		{"crlf", "a\r\n\r\nb\r\n", []contract.BlockKind{contract.BlockParagraph, contract.BlockParagraph}},
	} {
		s, err := parse(tt.src, parser.Default, false)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		c := s.cover()
		if msg := checkCoverage(c); msg != "" {
			t.Errorf("%s: %s", tt.name, msg)
		}
		var kinds []contract.BlockKind
		for _, l := range c.leaves {
			kinds = append(kinds, l.Kind)
		}
		if fmt.Sprint(kinds) != fmt.Sprint(tt.kinds) {
			t.Errorf("%s: kinds %v, want %v; leaves %+v", tt.name, kinds, tt.kinds, c.leaves)
		}
	}
}

func TestCoverageCorpus(t *testing.T) {
	samples := corpus(t)
	var leaves, overlaps, swallowedCount int
	var witnesses []string
	for _, x := range samples {
		s, err := parse(x.src, x.ext, false)
		if err != nil {
			t.Fatal(err)
		}
		c := s.cover()
		if msg := checkCoverage(c); msg != "" {
			t.Errorf("%s: %s src=%q", x.name, msg, x.src)
		}
		leaves += len(c.leaves)
		overlaps += c.overlaps
		if text := swallowed(c); text != "" {
			swallowedCount++
			if len(witnesses) < 25 {
				witnesses = append(witnesses, fmt.Sprintf("%s holds %q in src=%q", x.name, text, x.src))
			}
		}
	}
	t.Logf("samples=%d leaves=%d overlaps=%d samples_with_text_in_lead_or_trailer=%d", len(samples), leaves, overlaps, swallowedCount)
	for _, w := range witnesses {
		t.Log(w)
	}
}

func TestCoverageRandomEdits(t *testing.T) {
	samples := corpus(t)
	tokens := []string{"\n", "\n\n", "\r\n", "> ", "- ", "1. ", "| a | b |\n|---|---|\n", "[x]: /u\n", "```\n", "a", "# ", "\t", "    ", "- [ ] ", "---\n"}
	for _, seed := range []int64{1, 2, 3} {
		r := rand.New(rand.NewSource(seed))
		var edits, overlaps, swallowedCount int
		for edits < 10000 {
			x := samples[r.Intn(len(samples))]
			src := x.src
			for range 5 {
				at := r.Intn(len(src) + 1)
				if r.Intn(3) == 0 && at < len(src) {
					end := min(len(src), at+1+r.Intn(8))
					src = src[:at] + src[end:]
				} else {
					src = src[:at] + tokens[r.Intn(len(tokens))] + src[at:]
				}
				edits++
				s, err := parse(src, x.ext, false)
				if err != nil {
					continue // adapter's own Line mapping, covered by the S3 tests
				}
				c := s.cover()
				if msg := checkCoverage(c); msg != "" {
					t.Fatalf("seed %d: %s src=%q", seed, msg, src)
				}
				overlaps += c.overlaps
				if swallowed(c) != "" {
					swallowedCount++
				}
			}
		}
		t.Logf("seed=%d edits=%d overlaps=%d with_text_in_lead_or_trailer=%d", seed, edits, overlaps, swallowedCount)
	}
}
