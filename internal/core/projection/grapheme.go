package projection

//go:generate go run gen_grapheme.go

type propertyRange struct {
	first, last rune
	value       uint16
}

const (
	gOther uint16 = iota
	gCR
	gLF
	gControl
	gExtend
	gZWJ
	gRI
	gPrepend
	gSpacing
	gL
	gV
	gT
	gLV
	gLVT
)

func graphemeProperty(r rune) uint16 {
	if r >= 0xAC00 && r <= 0xD7A3 {
		if (r-0xAC00)%28 == 0 {
			return gLV
		}
		return gLVT
	}
	if r >= 0x20 && r < 0x7F {
		return 0
	}
	lo, hi := 0, len(graphemeProperties)
	for lo < hi {
		mid := (lo + hi) / 2
		p := graphemeProperties[mid]
		if r < p.first {
			hi = mid
		} else if r > p.last {
			lo = mid + 1
		} else {
			return p.value
		}
	}
	return 0
}

// UAX #29 extended grapheme rules, including Indic conjuncts (GB9c).
type graphemeState struct {
	prev              uint16
	started, ep, zwj  bool
	ri                int
	consonant, linked bool
}

func (s *graphemeState) push(r rune) bool {
	p := graphemeProperty(r)
	g := p & 15
	br := true
	if s.started {
		a := s.prev
		switch {
		case a == gCR && g == gLF:
			br = false
		case a == gCR || a == gLF || a == gControl || g == gCR || g == gLF || g == gControl:
		case a == gL && (g == gL || g == gV || g == gLV || g == gLVT):
			br = false
		case (a == gLV || a == gV) && (g == gV || g == gT):
			br = false
		case (a == gLVT || a == gT) && g == gT:
			br = false
		case g == gExtend || g == gZWJ || g == gSpacing || a == gPrepend:
			br = false
		case p&32 != 0 && s.consonant && s.linked:
			br = false
		case p&16 != 0 && s.zwj:
			br = false
		case a == gRI && g == gRI && s.ri%2 == 1:
			br = false
		}
	}
	if g == gRI {
		s.ri++
	} else {
		s.ri = 0
	}
	s.zwj = g == gZWJ && s.ep
	if g != gExtend {
		s.ep = p&16 != 0
	}
	if p&32 != 0 {
		s.consonant, s.linked = true, false
	} else if p&128 != 0 {
		s.linked = s.consonant
	} else if p&64 == 0 {
		s.consonant, s.linked = false, false
	}
	s.started, s.prev = true, g
	return br
}
