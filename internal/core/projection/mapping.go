package projection

import (
	"strings"
	"unicode/utf8"

	"aurink/internal/contract"
)

// DisplayText concatenates the projection's display segments.
func (Projector) DisplayText(p *contract.BlockProjection) string {
	var out strings.Builder
	for _, s := range p.Segments {
		out.WriteString(s.Display)
	}
	return out.String()
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

func transparent(s contract.Segment) bool {
	return s.Kind == contract.SegText || s.Kind == contract.SegMarker
}

// displayBoundary scans across segment boundaries, since a style or a hidden
// delimiter can sit inside a displayed grapheme. It allocates no joined string.
func displayBoundary(p *contract.BlockProjection, d int, aff contract.Affinity) int {
	if d <= 0 {
		return 0
	}
	if at, ok := simpleBoundary(p, d); ok {
		return at
	}
	var state graphemeState
	u, prev := 0, 0
	for _, s := range p.Segments {
		for _, r := range s.Display {
			if state.push(r) {
				if u >= d {
					if u == d || aff == contract.Downstream {
						return u
					}
					return prev
				}
				prev = u
			}
			u++
			if r > 0xFFFF {
				u++
			}
		}
	}
	if d < u && aff == contract.Upstream {
		return prev
	}
	return u
}

// Most editor positions lie between ordinary letters. Their boundary depends
// only on the adjacent runes; contextual rules need the full scanner below.
func simpleBoundary(p *contract.BlockProjection, d int) (int, bool) {
	u := 0
	var prev rune
	have := false
	for _, s := range p.Segments {
		for _, r := range s.Display {
			if u == d {
				return d, !have || definiteBoundary(prev, r)
			}
			u++
			if r > 0xFFFF {
				u++
			}
			if u > d {
				return 0, false
			}
			prev, have = r, true
		}
	}
	return u, true
}

func definiteBoundary(left, right rune) bool {
	if left >= 0x20 && left < 0x7F && right >= 0x20 && right < 0x7F {
		return true
	}
	a, b := graphemeProperty(left), graphemeProperty(right)
	x, y := a&15, b&15
	if x == gCR && y == gLF {
		return false
	}
	if x == gCR || x == gLF || x == gControl || y == gCR || y == gLF || y == gControl {
		return true
	}
	if y == gExtend || y == gZWJ || y == gSpacing || x == gPrepend {
		return false
	}
	if x == gL && (y == gL || y == gV || y == gLV || y == gLVT) ||
		(x == gLV || x == gV) && (y == gV || y == gT) || (x == gLVT || x == gT) && y == gT {
		return false
	}
	return !(b&32 != 0 && a&(64|128) != 0 || b&16 != 0 && x == gZWJ || x == gRI && y == gRI)
}

// ToDisplay maps source bytes to UTF-16, snapping inside graphemes and atomic
// replacements toward affinity. Lead and Trailer have zero display width.
func (Projector) ToDisplay(p *contract.BlockProjection, src int, aff contract.Affinity) int {
	u := 0
	for _, s := range p.Segments {
		if src >= s.Src.End {
			u += utf16Len(s.Display)
			continue
		}
		if src > s.Src.Start {
			if transparent(s) {
				k := src - s.Src.Start
				for k > 0 && !utf8.RuneStart(s.Display[k]) {
					k--
				}
				_, width := utf8.DecodeRuneInString(s.Display[k:])
				if k+width <= src-s.Src.Start {
					k = src - s.Src.Start
				}
				if aff == contract.Downstream && k < src-s.Src.Start {
					k += width
				}
				u += utf16Len(s.Display[:k])
			} else if aff == contract.Downstream {
				u += utf16Len(s.Display)
			}
		}
		break
	}
	return displayBoundary(p, u, aff)
}

// ToSource maps UTF-16 to source bytes. At a zero-width run it selects the first
// or last source edge, restricted to the editable range encoded by the boundary
// segments. Objects and replacements are atomic in both directions.
func (Projector) ToSource(p *contract.BlockProjection, disp int, aff contract.Affinity) int {
	if len(p.Segments) < 2 {
		return 0
	}
	lo, hi := p.Segments[0].Src.End, p.Segments[len(p.Segments)-1].Src.Start
	d := displayBoundary(p, disp, aff)
	u, before := 0, lo
	for _, s := range p.Segments {
		n := utf16Len(s.Display)
		if n == 0 {
			continue
		}
		if d <= u {
			if aff == contract.Upstream {
				return min(hi, max(lo, before))
			}
			return min(hi, max(lo, s.Src.Start))
		}
		if d < u+n {
			if transparent(s) {
				at := 0
				for at < len(s.Display) {
					if u >= d {
						break
					}
					r, width := utf8.DecodeRuneInString(s.Display[at:])
					at += width
					u++
					if r > 0xFFFF {
						u++
					}
				}
				return min(hi, max(lo, s.Src.Start+at))
			}
			if aff == contract.Upstream {
				return min(hi, max(lo, s.Src.Start))
			}
			return min(hi, max(lo, s.Src.End))
		}
		u += n
		before = s.Src.End
	}
	if aff == contract.Downstream {
		return hi
	}
	return min(hi, max(lo, before))
}

// Snap returns the canonical source position for the requested affinity.
func (pr Projector) Snap(p *contract.BlockProjection, src int, aff contract.Affinity) int {
	return pr.ToSource(p, pr.ToDisplay(p, src, aff), aff)
}
