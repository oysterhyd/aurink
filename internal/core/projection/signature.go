package projection

import "aurink/internal/contract"

type digest uint64

func (h *digest) number(n uint64) {
	if n == 0 {
		// Eight zero bytes only multiply the FNV state by the prime eight times.
		*h *= 0x1efac7090aef4a21
		return
	}
	for range 8 {
		*h = (*h ^ digest(byte(n))) * 1099511628211
		n >>= 8
	}
}

func (h *digest) text(s string) {
	h.number(uint64(len(s)))
	for i := 0; i < len(s); i++ {
		*h = (*h ^ digest(s[i])) * 1099511628211
	}
}

func (h *digest) flag(b bool) {
	if b {
		h.number(1)
	} else {
		h.number(0)
	}
}

func signature(p *contract.BlockProjection) uint64 {
	h := digest(14695981039346656037)
	h.number(uint64(p.Kind))
	h.number(uint64(p.Level))
	h.text(p.Lang)
	h.flag(p.Revealed)
	h.number(uint64(len(p.Containers)))
	for _, c := range p.Containers {
		h.number(uint64(c.Kind))
		h.number(uint64(c.Depth))
		h.flag(c.Ordered)
		h.number(uint64(c.Number))
		h.number(uint64(c.Marker))
		h.number(uint64(c.Task))
		h.flag(c.First)
		h.text(c.Label)
	}
	h.number(uint64(len(p.Segments)))
	for _, s := range p.Segments {
		h.number(uint64(s.Kind))
		h.text(s.Display)
		h.number(uint64(s.Style))
		h.flag(s.Object != nil)
		if o := s.Object; o != nil {
			h.number(uint64(o.Kind))
			h.text(o.Source)
			h.text(o.Alt)
			h.text(o.Title)
			h.text(o.Lang)
			h.flag(o.Display)
			h.flag(o.Checked)
		}
	}
	return uint64(h)
}
