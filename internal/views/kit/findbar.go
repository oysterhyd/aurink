package kit

import (
	"strconv"

	"aurink/internal/views/effects"
	"aurink/internal/views/motion"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// Finder is what the find bar shows and does (02 §5.14). The source view's is
// MyGo's own (*editor.FindBar); the live view's is the shell's.
type Finder interface {
	Query() *string
	Replacement() *string
	CaseSensitive() *bool
	WholeWord() *bool
	Regexp() *bool
	Replacing() bool
	SetReplacing(on bool)
	CanReplace() bool
	// Matches returns how many there are and the index of the current one, -1
	// for none.
	Matches() (count, current int)
	Err() error
	Next()
	Previous()
	Replace()
	ReplaceAll()
	Close()
	// QueryInput and ReplacementInput give the fields their keys: Enter,
	// Shift+Enter, Escape.
	QueryInput(in ui.Element) ui.Element
	ReplacementInput(in ui.Element) ui.Element
}

// FindBarWidth is the bar's width (02 §5.14).
const FindBarWidth = 440

// FindBar builds the find bar into bar, which the caller places: the query, the
// count, the options and the steps; the replacement row collapses in below.
func FindBar(c *ui.Context, t *theme.Tokens, bar ui.Element, f Finder, live bool) {
	bar.Width(FindBarWidth).Padding(8).Gap(6).Radius(14).Border(1, t.Border).
		Shadow(0, 12, 32, 0, t.ShadowColor).Material(effects.Glass(t, effects.Panel)).
		Role(ui.RoleToolbar).Label(locales.KitFind()).Cursor(ui.CursorDefault)
	field := func(key string, value *string, placeholder string) ui.Element {
		return ui.TextInputBase(c.Key(key), value).Grow(1).Shrink(1).Height(30).Padding(0, 10).Radius(8).
			Background(t.Surface).FontSize(theme.UIText).Placeholder(placeholder).Label(placeholder)
	}
	bar.Children(func() {
		ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
			if f.CanReplace() {
				chev := IconButton(c, t, IconChevron, locales.KitToggleReplace(), 24, 12).KeepFocus()
				if !f.Replacing() {
					chev.Rotate(-90)
				}
				if chev.Clicked() {
					f.SetReplacing(!f.Replacing())
				}
			} else {
				ui.Box(c).Size(24, 24)
			}
			f.QueryInput(field("query", f.Query(), locales.KitFind()))
			count, cur := f.Matches()
			status, tone := "", t.Muted
			switch {
			case f.Err() != nil:
				status, tone = locales.KitBadRegex(), t.Danger
			case *f.Query() == "":
			case count == 0:
				status = locales.KitNoResults()
			case cur < 0:
				status = strconv.Itoa(count)
			default:
				status = strconv.Itoa(cur+1) + " / " + strconv.Itoa(count)
			}
			ui.Text(c, status).FontSize(theme.UIBadge + 1).TextColor(tone).FontFeatures("tnum").SingleLine().
				MinWidth(44).TextAlign(ui.Center).Role(ui.RoleStatus)
			findToggle(c, t, f.CaseSensitive(), "Aa", locales.KitMatchCase())
			findToggle(c, t, f.WholeWord(), "W", locales.KitWholeWord())
			findToggle(c, t, f.Regexp(), ".*", locales.KitRegex())
			if IconButton(c, t, IconArrowUp, locales.KitPrevious(), 24, 12).KeepFocus().Clicked() {
				f.Previous()
			}
			if IconButton(c, t, IconArrowDown, locales.KitNext(), 24, 12).KeepFocus().Clicked() {
				f.Next()
			}
			if IconButton(c, t, IconClose, locales.KitCloseFind(), 24, 11).KeepFocus().Clicked() {
				f.Close()
			}
		})
		if f.Replacing() {
			row := ui.Row(c).Key("replace").Gap(4).AlignItems(ui.Center)
			if live {
				row.Transition(motion.Still(t.Still, ui.ElementTransition{Animation: &motion.Snappy,
					Enter: &ui.Motion{Collapse: true, Opacity: 0}, Exit: &ui.Motion{Collapse: true, Opacity: 0}}))
			}
			row.Children(func() {
				ui.Box(c).Size(24, 24)
				f.ReplacementInput(field("replacement", f.Replacement(), locales.KitReplaceWith()))
				if findButton(c, t, locales.KitReplace()).Clicked() {
					f.Replace()
				}
				if findButton(c, t, locales.KitReplaceAll()).Clicked() {
					f.ReplaceAll()
				}
			})
		}
	})
}

func findToggle(c *ui.Context, t *theme.Tokens, on *bool, text, label string) {
	b := ui.Row(c).Height(24).MinWidth(26).Padding(0, 5).Justify(ui.Center).AlignItems(ui.Center).Radius(6).
		Role(ui.RoleToggleButton).Checked(*on).Label(label).Tooltip(label).KeepFocus()
	fg := t.Muted
	switch {
	case *on:
		b.Background(t.AccentSoft).Border(1, t.AccentRing)
		fg = t.Accent
	case b.Hovered():
		b.Background(t.Hover)
	}
	if b.Clicked() {
		*on = !*on
	}
	b.Children(func() { ui.Text(c, text).Font(theme.Mono).FontSize(theme.UIBadge + 1).FontWeight(600).TextColor(fg) })
}

func findButton(c *ui.Context, t *theme.Tokens, text string) ui.Element {
	b := ui.Row(c).Height(30).Padding(0, 10).AlignItems(ui.Center).Radius(8).Background(t.Surface).
		Role(ui.RoleButton).Label(text).KeepFocus()
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	press(t, b)
	b.Children(func() { ui.Text(c, text).FontSize(theme.UIBadge + 1).TextColor(t.Text).SingleLine() })
	return b
}
