// Package palette implements the command palette and quick open (02 §5.4):
// files by default, commands after ">", the active document's headings after "@".
package palette

import (
	"strings"
	"time"

	"aurink/internal/contract"
	"aurink/internal/views/effects"
	"aurink/internal/views/kit"
	"aurink/internal/views/motion"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
)

// Mode is what the palette lists, chosen by the query's first character.
type Mode uint8

const (
	ModeFiles    Mode = iota
	ModeCommands      // ">"
	ModeHeadings      // "@"
)

// Item is one row.
type Item struct {
	Title  string
	Detail string // a file's folder, a heading's level
	Keys   string // a command's shortcut
	Indent int    // a heading's level minus one
	Run    func()
}

// Source gives the items of each mode. It is asked again when the palette
// opens and when the mode changes, not on every keystroke.
type Source interface {
	Files() []Item
	Commands() []Item
	Headings() []Item
}

// Labels are the words the palette shows, in the app's language.
type Labels struct {
	Files, Commands, Headings string // mode badges and group titles
	Placeholder               string
	Footer                    string // "↑↓ select · Enter run · Esc close"
	Count                     func(n int) string
	Empty                     string
}

// MaxRows bounds the rows built: the list stays one frame's work with 10,000 files.
const MaxRows = 200

// Palette is the modal palette. The zero value is closed; use New.
type Palette struct {
	fuzzy  contract.Fuzzy
	source Source

	open  bool
	query string
	sel   int
	// opens counts the openings; glow is set once this opening's glow has swept.
	opens       int
	glow, swept bool

	mode  Mode
	items []Item
	// hits are the matches of query over items, cached by the query they answer.
	hits    []contract.FuzzyHit
	hitsFor string
	scroll  bool
	rows    ui.ListState
	// Built counts the rows built in the last frame, for tests.
	Built int
}

// New returns a closed palette over a fuzzy matcher (nil matches by substring).
func New(fuzzy contract.Fuzzy, source Source) *Palette {
	return &Palette{fuzzy: fuzzy, source: source}
}

// Open shows the palette in a mode, its query holding the mode's prefix.
func (p *Palette) Open(m Mode) {
	p.open, p.glow, p.swept = true, false, false
	p.opens++
	p.query = prefix(m)
	p.sel = 0
	p.load(m)
}

// Close hides the palette.
func (p *Palette) Close() { p.open = false }

// IsOpen reports whether the palette shows.
func (p *Palette) IsOpen() bool { return p.open }

// Query is the text typed, prefix included.
func (p *Palette) Query() string { return p.query }

// SetQuery replaces the text typed, as typing would.
func (p *Palette) SetQuery(q string) { p.query = q; p.sel = 0 }

// Mode is the mode the query is in.
func (p *Palette) Mode() Mode { return modeOf(p.query) }

// Rows returns the titles listed, best first, as the palette shows them.
func (p *Palette) Rows() []string {
	p.refresh()
	out := make([]string, len(p.hits))
	for i, h := range p.hits {
		out[i] = p.items[h.Index].Title
	}
	return out
}

// Selected is the row Enter runs, or -1.
func (p *Palette) Selected() int {
	p.refresh()
	if len(p.hits) == 0 {
		return -1
	}
	return p.sel
}

// Move moves the selection by d rows, wrapping around.
func (p *Palette) Move(d int) {
	p.refresh()
	if n := len(p.hits); n > 0 {
		p.sel = ((p.sel+d)%n + n) % n
		p.scroll = true
	}
}

// Run runs the selected row and closes the palette.
func (p *Palette) Run() {
	p.refresh()
	if p.sel < 0 || p.sel >= len(p.hits) {
		return
	}
	item := p.items[p.hits[p.sel].Index]
	p.open = false
	if item.Run != nil {
		item.Run()
	}
}

func prefix(m Mode) string {
	switch m {
	case ModeCommands:
		return ">"
	case ModeHeadings:
		return "@"
	}
	return ""
}

func modeOf(q string) Mode {
	switch {
	case strings.HasPrefix(q, ">"):
		return ModeCommands
	case strings.HasPrefix(q, "@"):
		return ModeHeadings
	}
	return ModeFiles
}

func (p *Palette) load(m Mode) {
	p.mode = m
	switch m {
	case ModeCommands:
		p.items = p.source.Commands()
	case ModeHeadings:
		p.items = p.source.Headings()
	default:
		p.items = p.source.Files()
	}
	p.hitsFor = "\x00" // stale
}

// refresh matches the query when it or the mode changed.
func (p *Palette) refresh() {
	if m := modeOf(p.query); m != p.mode {
		p.load(m)
	}
	q := strings.TrimSpace(strings.TrimPrefix(p.query, prefix(p.mode)))
	if q == p.hitsFor {
		return
	}
	p.hitsFor = q
	p.hits = p.match(q)
	if p.sel >= len(p.hits) {
		p.sel = max(len(p.hits)-1, 0)
	}
}

func (p *Palette) match(q string) []contract.FuzzyHit {
	if p.fuzzy != nil {
		fi := make([]contract.FuzzyItem, len(p.items))
		for i, it := range p.items {
			fi[i] = contract.FuzzyItem{Text: it.Title, Detail: it.Detail}
		}
		return p.fuzzy.Match(q, fi, MaxRows)
	}
	var out []contract.FuzzyHit
	lq := strings.ToLower(q)
	for i, it := range p.items {
		if len(out) == MaxRows {
			break
		}
		if strings.Contains(strings.ToLower(it.Title), lq) {
			out = append(out, contract.FuzzyHit{Index: i})
		}
	}
	return out
}

type paletteMatch struct{}

// Match is the shared element the palette grows out of: give it to the omnibox.
var Match any = paletteMatch{}

// View builds the palette while it is open. live turns motion on: the shell
// passes false when motion is reduced.
func (p *Palette) View(c *ui.Context, t *theme.Tokens, l Labels, live bool) {
	if !p.open {
		return
	}
	p.refresh()
	p.Built = 0
	ui.DialogBase(c.Key("palette"), &p.open, func(backdrop, panel ui.Element) {
		backdrop.Background(t.Scrim).Justify(ui.Start).PaddingY(86)
		panel.Width(640).MaxHeight(460).Radius(theme.RadiusFloat+4).Border(1, t.Border).
			Shadow(0, 24, 64, 0, t.ShadowColor).Material(effects.Glass(t, effects.Modal)).Clip().Label(l.Placeholder)
		if live {
			panel.Transition(motion.Frame(Match))
			if !p.glow {
				key := [2]any{"glow", p.opens}
				if !p.swept {
					panel.Jump(key, 0)
					p.swept = true
				}
				glow := panel.Animate(key, 1, 1500*time.Millisecond)
				if glow < 1 {
					panel.BorderConicGradient(ui.ConicGradient{Angle: glow * 360, Stops: []ui.GradientStop{
						{At: 0, Color: t.Aurora1}, {At: .33, Color: t.Aurora2},
						{At: .66, Color: t.Aurora3}, {At: 1, Color: t.Aurora1}}})
				} else {
					p.glow = true
				}
			}
		}
		ui.Column(c).Fill().Children(func() {
			p.input(c, t, l)
			p.list(c, t, l)
			p.footer(c, t, l)
		})
	})
}

func (p *Palette) badge(l Labels) string {
	switch p.mode {
	case ModeCommands:
		return l.Commands
	case ModeHeadings:
		return l.Headings
	}
	return l.Files
}

func (p *Palette) input(c *ui.Context, t *theme.Tokens, l Labels) {
	ui.Row(c).Height(56).Padding(0, 16).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, p.badge(l)).FontSize(theme.UIBadge).FontWeight(600).TextColor(t.Accent).
			Padding(3, 8).Radius(6).Background(t.AccentSoft)
		in := ui.TextInputBase(c.Key("query"), &p.query).Grow(1).FontSize(theme.UIText + 2).
			Placeholder(l.Placeholder).Label(l.Placeholder).Focus()
		in.OnTextKey(func(k ui.TextKey) bool {
			if k.Mods != 0 {
				return false
			}
			switch k.Key {
			case ui.KeyDown:
				p.Move(1)
			case ui.KeyUp:
				p.Move(-1)
			case ui.KeyEnter:
				p.Run()
			default:
				return false
			}
			return true
		})
		if in.Changed() {
			p.sel = 0
		}
	})
	ui.Box(c).Height(1).FillWidth().Background(t.Border)
}

func (p *Palette) list(c *ui.Context, t *theme.Tokens, l Labels) {
	if len(p.hits) == 0 {
		ui.Text(c, l.Empty).FontSize(theme.UIText).TextColor(t.Muted).Padding(18, 20)
		return
	}
	ui.Text(c, p.badge(l)).FontSize(theme.UIBadge).FontWeight(600).TextColor(t.Muted).Padding(12, 17, 4)
	if p.scroll {
		p.rows.ScrollIntoView(p.sel)
		p.scroll = false
	}
	// The list builds the rows in view only: 10,000 files cost what ten do.
	ui.List(c.Key("rows"), &p.rows, len(p.hits), func(i int) {
		h := p.hits[i]
		p.row(c.Key(i), t, i, p.items[h.Index], h.Positions)
	}).Grow(1).MinHeight(0).Padding(0, 8, 8)
}

type selPill struct{}

func (p *Palette) row(c *ui.Context, t *theme.Tokens, i int, it Item, pos []int) {
	p.Built++
	r := ui.Row(c).Height(40).Padding(0, 10).Gap(10).AlignItems(ui.Center).Radius(theme.RadiusRow).
		Role(ui.RoleListItem).Label(it.Title).KeepFocus()
	if r.Hovered() {
		r.Background(t.Hover)
	}
	r.Children(func() {
		if i == p.sel {
			ui.Box(c.Key("pill")).Absolute().Fill().Radius(theme.RadiusRow).Background(t.AccentSoft).
				Border(1, t.AccentRing).Transition(motion.Still(t.Still, ui.ElementTransition{Animation: &motion.Snappy, Match: selPill{}}))
		}
		icon := kit.IconFile
		switch p.mode {
		case ModeCommands:
			icon = kit.IconCommand
		case ModeHeadings:
			icon = kit.IconHeading
		}
		ui.Row(c).Size(24, 24).Justify(ui.Center).AlignItems(ui.Center).Radius(6).Background(t.Surface).
			MarginStart(float32(it.Indent) * 14).Children(func() {
			ui.Icon(c, icon).Size(14, 14).TextColor(t.Muted)
		})
		highlighted(c, t, it.Title, pos)
		if it.Detail != "" {
			ui.Text(c, it.Detail).FontSize(theme.UIStatus).TextColor(t.Faint).SingleLine().Shrink(1)
		}
		ui.Spacer(c)
		if it.Keys != "" {
			kit.KeyHint(c, t, it.Keys)
		}
	})
	if r.Clicked() {
		p.sel = i
		p.Run()
	}
}

// highlighted writes title with the runes at pos in the accent color.
func highlighted(c *ui.Context, t *theme.Tokens, title string, pos []int) {
	ui.Row(c).Shrink(1).MinWidth(0).Children(func() {
		if len(pos) == 0 {
			ui.Text(c, title).FontSize(theme.UIText + 1).SingleLine()
			return
		}
		hit := make(map[int]bool, len(pos))
		for _, i := range pos {
			hit[i] = true
		}
		var run strings.Builder
		on, n, k := false, 0, 0
		flush := func() {
			if run.Len() == 0 {
				return
			}
			s := ui.Text(c.Key(k), run.String()).FontSize(theme.UIText + 1).NoWrap()
			if on {
				s.TextColor(t.Accent).FontWeight(600)
			}
			run.Reset()
			k++
		}
		for _, r := range title {
			if hit[n] != on {
				flush()
				on = hit[n]
			}
			run.WriteRune(r)
			n++
		}
		flush()
	})
}

func (p *Palette) footer(c *ui.Context, t *theme.Tokens, l Labels) {
	ui.Box(c).Height(1).FillWidth().Background(t.Border)
	ui.Row(c).Height(36).Padding(0, 14).AlignItems(ui.Center).Children(func() {
		ui.Text(c, l.Footer).FontSize(theme.UIBadge).TextColor(t.Muted)
		ui.Spacer(c)
		ui.Text(c, l.Count(len(p.hits))).FontSize(theme.UIBadge).TextColor(t.Muted)
	})
}
