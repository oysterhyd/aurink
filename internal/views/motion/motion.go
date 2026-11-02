// Package motion holds the animation tokens of docs/02-product-ux.md §4.4.
package motion

import (
	"time"

	"github.com/egoist/mygo/ui"
)

var (
	// Quick is for hover colors and markers fading.
	Quick = ui.Animation{Duration: 120 * time.Millisecond, Ease: ui.EaseOut}
	// Snappy is for buttons, the capsule, small floats and the press rebound.
	Snappy = ui.Animation{Spring: ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: .15}}
	// Smooth is for layout changes, the sidebar, scrolling and the selection pill.
	Smooth = ui.Animation{Spring: ui.SpringOptions{Duration: 360 * time.Millisecond}}
	// Gentle is for large panels and shared element flights.
	Gentle = ui.Animation{Spring: ui.SpringOptions{Duration: 520 * time.Millisecond, Bounce: .1}}
	// ContentIn fades a panel's content in once its frame has mostly opened.
	ContentIn = ui.Animation{Duration: 120 * time.Millisecond, Delay: 160 * time.Millisecond}
)

// PressScale is the scale of a pressed control.
const PressScale = 0.965

// Stagger returns the delay of the i-th item of a list, capped at eight items.
func Stagger(i int) time.Duration { return time.Duration(min(i, 8)) * 24 * time.Millisecond }

// Flight is ADR-004's way to open a float from its trigger. The frame carries the
// Match and flies; the content is laid out at the final size inside the clipping
// frame, and takes its opacity from Fade. An Enter inside a Match does not fire, so
// Fade must come from an element that outlives the panel.
type Flight struct{ Fade float32 }

// Drive runs the content's fade on an element that is always there.
func Drive(always ui.Element, key any, open bool) Flight {
	var target float32
	if open {
		target = 1
	}
	return Flight{Fade: always.AnimateTo(key, target, ContentIn)}
}

// Frame returns the transition of the flying frame.
func Frame(match any) ui.ElementTransition {
	return ui.ElementTransition{Animation: &Gentle, Match: match}
}
