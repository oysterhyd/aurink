package shell

import (
	"slices"
	"time"

	"aurink/internal/contract"
	"aurink/internal/views/kit"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
)

const noticeLife = 5 * time.Second

type notice struct {
	s     *Shell
	n     contract.Notice
	until time.Time // zero until first shown
}

func (h *notice) Update(n contract.Notice) {
	h.n = n
	h.s.invalidate()
}

func (h *notice) Dismiss() {
	h.s.notices = slices.DeleteFunc(h.s.notices, func(x *notice) bool { return x == h })
	h.s.invalidate()
}

// Notify shows a notice at the sheet's bottom. Past four, the oldest that may go goes.
func (s *Shell) Notify(n contract.Notice) contract.NoticeHandle {
	h := &notice{s: s, n: n}
	if n.Message == "" {
		return h // Nothing to say, as for a canceled dialog.
	}
	s.notices = append(s.notices, h)
	if len(s.notices) > maxNotices {
		i := slices.IndexFunc(s.notices, func(x *notice) bool { return !x.n.Sticky })
		s.notices = slices.Delete(s.notices, max(i, 0), max(i, 0)+1)
	}
	s.invalidate()
	return h
}

func (s *Shell) noticeStack(c *ui.Context, t *theme.Tokens) {
	if len(s.notices) == 0 {
		return
	}
	now := c.Now()
	if s.live {
		s.notices = slices.DeleteFunc(s.notices, func(h *notice) bool {
			if h.n.Sticky {
				return false
			}
			if h.until.IsZero() {
				h.until = now.Add(noticeLife)
			}
			if !now.Before(h.until) {
				return true
			}
			c.After(h.until.Sub(now))
			return false
		})
	}
	ui.Column(c.Key("notices")).Absolute().Bottom(16).Left(0).Right(0).Gap(8).AlignItems(ui.Center).PassThrough().Children(func() {
		for _, h := range s.notices {
			icon, tone := kit.IconInfo, t.Accent
			switch h.n.Level {
			case contract.NoticeWarning:
				icon, tone = kit.IconWarn, t.Warning
			case contract.NoticeError:
				icon, tone = kit.IconWarn, t.Danger
			}
			kit.Notice(c.Key(h), t, icon, tone, h.n.Message, h.n.Detail, h.n.Action, h.n.Run, h.Dismiss)
		}
	})
}
