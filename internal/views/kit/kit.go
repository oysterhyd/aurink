// Package kit holds the components the M1 interface uses, built from theme tokens.
// The rest of 05's list arrives with the cards that use them.
package kit

import (
	"aurink/internal/views/effects"
	"aurink/internal/views/motion"
	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
)

// IconButton is a square icon control with a hover tint and a press rebound.
func IconButton(c *ui.Context, t *theme.Tokens, s *ui.SVG, label string, size, iconSize float32) ui.Element {
	b := ui.Row(c).Size(size, size).Justify(ui.Center).AlignItems(ui.Center).Radius(theme.RadiusButton).
		Role(ui.RoleButton).Label(label).Tooltip(label).Focusable()
	press(b)
	if b.Hovered() {
		b.Background(t.Hover)
	}
	b.Children(func() { ui.Icon(c, s).Size(iconSize, iconSize).TextColor(t.Muted) })
	return b
}

func press(b ui.Element) {
	var target float32 = 1
	if b.Pressed() {
		target = motion.PressScale
	}
	s := b.AnimateTo("press", target, motion.Snappy)
	b.Scale(s, s)
}

// Button is a text button. A primary one is the ink button of the welcome page.
func Button(c *ui.Context, t *theme.Tokens, text, hint string, primary bool) ui.Element {
	bg, fg := t.Sheet, t.Text
	if primary {
		bg, fg = t.InkButton, t.OnInkButton
	}
	b := ui.Row(c).Padding(10, 15).Gap(10).AlignItems(ui.Center).Radius(10).Background(bg).
		Shadow(0, 2, 8, 0, t.ShadowColor).Role(ui.RoleButton).Label(text).Focusable()
	press(b)
	b.Children(func() {
		ui.Text(c, text).FontSize(theme.UIText + 1).FontWeight(500).TextColor(fg)
		if hint != "" {
			ui.Text(c, hint).FontSize(theme.UIBadge).TextColor(fg.Alpha(.65))
		}
	})
	return b
}

// KeyHint is a keyboard shortcut's little plate.
func KeyHint(c *ui.Context, t *theme.Tokens, keys string) ui.Element {
	return ui.Text(c, keys).FontSize(theme.UIBadge).TextColor(t.Muted).Padding(2, 6).Radius(5).
		Background(t.Surface).Border(1, t.Border)
}

// Capsule is the glass chip of the status capsule (§3).
func Capsule(c *ui.Context, t *theme.Tokens) ui.Element {
	return ui.Row(c).Padding(6, 14).Gap(8).AlignItems(ui.Center).Radius(theme.RadiusPill).Border(1, t.Border).
		Shadow(0, 4, 14, 0, t.ShadowColor).Material(effects.Glass(t, effects.Chip))
}

type segmentPill struct{ group any }

// Segmented is the sliding pill control: one pill slides between its items (§4.4).
// It returns the index clicked, or -1.
func Segmented(c *ui.Context, t *theme.Tokens, group any, items []string, selected int, live bool) int {
	clicked := -1
	ui.Row(c).Padding(3).Radius(10).Background(t.Surface).Children(func() {
		for i, label := range items {
			tab := ui.Box(c.Key(i)).Grow(1).Height(26).Role(ui.RoleTab).Label(label)
			tab.Children(func() {
				if selected == i {
					pill := ui.Box(c.Key("pill")).Absolute().Fill().Radius(7).Background(t.Sheet)
					if live {
						pill.Transition(ui.ElementTransition{Animation: &motion.Smooth, Match: segmentPill{group}})
					}
				}
				ui.Row(c).Absolute().Fill().Justify(ui.Center).AlignItems(ui.Center).Children(func() {
					ui.Text(c, label).FontSize(theme.UIText)
				})
			})
			if tab.Clicked() {
				clicked = i
			}
		}
	})
	return clicked
}

// Notice is one glass notification (§5.13). dismiss and act may be nil.
func Notice(c *ui.Context, t *theme.Tokens, s *ui.SVG, tone ui.Color, title, detail, action string, act, dismiss func()) ui.Element {
	return ui.Row(c).Padding(10, 12).Gap(10).AlignItems(ui.Center).Radius(theme.RadiusFloat).MaxWidth(460).
		Border(1, t.Border).Shadow(0, 12, 32, 0, t.ShadowColor).Material(effects.Glass(t, effects.Panel)).
		Role(ui.RoleStatus).Label(title).
		Transition(ui.ElementTransition{Animation: &motion.Snappy, Enter: &ui.Motion{Y: 12, Opacity: 0}, Exit: &ui.Motion{Y: 12, Opacity: 0}}).
		Children(func() {
			ui.Icon(c, s).Size(16, 16).TextColor(tone)
			ui.Column(c).Shrink(1).Gap(2).Children(func() {
				ui.Text(c, title).FontSize(theme.UIText).FontWeight(600)
				if detail != "" {
					ui.Text(c, detail).FontSize(theme.UIStatus).TextColor(t.Muted)
				}
			})
			if action != "" && act != nil {
				a := ui.Text(c, action).FontSize(theme.UIStatus).FontWeight(600).TextColor(t.Accent).Padding(4, 6).Role(ui.RoleButton).Label(action)
				if a.Clicked() {
					act()
				}
			}
			if dismiss != nil {
				if IconButton(c, t, IconClose, "×", 22, 12).Clicked() {
					dismiss()
				}
			}
		})
}
