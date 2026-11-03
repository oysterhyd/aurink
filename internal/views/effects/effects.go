// Package effects holds the window ground, the glass levels and the theme dissolve
// of docs/adr/ADR-004-look.md.
package effects

import (
	"math"
	"strconv"
	"time"

	"aurink/internal/views/theme"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/shader"
)

// frost is a glass level of §4.3: the backdrop blurred and saturated, under a
// fill, with a highlight along the top.
var frost = shader.MustCompile(`package main

var Tint vec4
var Sat float

func Fragment(dst vec4, pos vec2) vec4 {
	b := backdropSample(pos)
	g := dot(b.rgb, vec3(0.2126, 0.7152, 0.0722))
	c := mix(vec3(g), b.rgb, Sat)
	c = mix(c, Tint.rgb, Tint.a)
	if pos.y < 1 {
		c = mix(c, vec3(1), 0.3)
	}
	return vec4(c, 1)
}
`)

// tint is the window's ground: three soft pools of the aurora colors.
var tint = shader.MustCompile(`package main

var Base, A, B, C vec4
var Strength float

func pool(pos, at, r vec2) float {
	return 1 - smoothstep(0, 1, length((pos-at)/r))
}

func Fragment(dst vec4, pos vec2) vec4 {
	s := rectSize()
	c := Base.rgb
	c = mix(c, A.rgb, Strength*pool(pos, vec2(0, 0), s*vec2(.9, .7)))
	c = mix(c, B.rgb, Strength*.8*pool(pos, vec2(s.x, 0), s*vec2(.7, .6)))
	c = mix(c, C.rgb, Strength*.6*pool(pos, s, s*vec2(.8, .7)))
	return vec4(c, 1)
}
`)

// dissolve shows an element inside a circle growing from Origin.
var dissolve = shader.MustCompile(`package main

var Origin vec2
var Radius float

func ColorEffect(pos vec2, color vec4) vec4 {
	return color * (1 - smoothstep(Radius-48, Radius, length(pos-Origin)))
}
`)

// aurora is the welcome page's and an empty document's light (02 §6): up to
// three soft pools of the aurora colors drifting slowly, composited over Base
// (premultiplied; transparent over an empty document).
var aurora = shader.MustCompile(`package main

var Base vec4
var P1, P2, P3 vec4
var D1, D2, D3 vec4
var C1, C2, C3 vec4
var Phase float

func pool(pos, s vec2, p, d vec4) float {
	at := (p.xy + d.xy*Phase) * s
	r := p.zw * s * (1 + 0.14*Phase)
	return 1 - smoothstep(0, 1, length((pos-at)/r))
}

func over(dst vec4, c vec4, a float) vec4 {
	return vec4(c.rgb*a, a) + dst*(1-a)
}

func Fragment(dst vec4, pos vec2) vec4 {
	s := rectSize()
	o := Base
	o = over(o, C1, C1.a*pool(pos, s, P1, D1))
	o = over(o, C2, C2.a*pool(pos, s, P2, D2))
	o = over(o, C3, C3.a*pool(pos, s, P3, D3))
	return o
}
`)

// AuroraPool is one pool of an aurora: its centre and radii as fractions of
// the box, how far it drifts at the top of its cycle, its color and strength.
type AuroraPool struct {
	X, Y, RX, RY float32
	DX, DY       float32
	Color        ui.Color
	Strength     float32
}

// auroraLife is how long the aurora drifts before it stops asking for
// frames (02 §4.4 rule 4); auroraCycle is one drift there and back.
const (
	auroraLife  = 12 * time.Second
	auroraCycle = 16 * time.Second
)

// AuroraPhase is how far the pools have drifted, 0 to 1, after elapsed, and
// whether another frame is wanted: none past auroraLife, with motion reduced,
// or without a GPU.
func AuroraPhase(elapsed time.Duration, still bool) (phase float32, more bool) {
	if still || elapsed <= 0 {
		return 0, !still && elapsed <= 0
	}
	more = elapsed < auroraLife
	elapsed = min(elapsed, auroraLife)
	return float32(1-math.Cos(2*math.Pi*elapsed.Seconds()/auroraCycle.Seconds())) / 2, more
}

// PaintAurora paints the pools over box, elapsed into their drift; base is
// what is under them, transparent for none. In low power or without a GPU a
// still gradient stands in. It reports whether it wants another frame.
func PaintAurora(p *ui.Painter, box ui.Rect, t *theme.Tokens, base ui.Color, pools []AuroraPool, elapsed time.Duration, reduce bool) bool {
	if base.A > 0 {
		p.Fill(box, base, 0)
	}
	if len(pools) == 0 {
		return false
	}
	if t.LowPower || !p.Accelerated() {
		// The still gradient: the first pool's color fading across.
		p.FillGradient(box, ui.LinearGradient{From: pools[0].Color.Alpha(pools[0].Strength * .5), To: pools[len(pools)-1].Color.Alpha(0), Angle: 135}, 0)
		return false
	}
	phase, more := AuroraPhase(elapsed, reduce)
	u := map[string]any{"Base": premultiplied(base), "Phase": phase}
	for i := range 3 {
		n := strconv.Itoa(i + 1)
		if i >= len(pools) {
			u["P"+n], u["D"+n], u["C"+n] = []float32{0, 0, 1, 1}, []float32{0, 0, 0, 0}, []float32{0, 0, 0, 0}
			continue
		}
		q := pools[i]
		u["P"+n] = []float32{q.X, q.Y, q.RX, q.RY}
		u["D"+n] = []float32{q.DX, q.DY, 0, 0}
		u["C"+n] = []float32{float32(q.Color.R) / 255, float32(q.Color.G) / 255, float32(q.Color.B) / 255, q.Strength}
	}
	aurora.Draw(p, box, shader.Options{Uniforms: u})
	return more
}

func premultiplied(c ui.Color) []float32 {
	a := float32(c.A) / 255
	return []float32{float32(c.R) / 255 * a, float32(c.G) / 255 * a, float32(c.B) / 255 * a, a}
}

// Level is a glass level of §4.3.
type Level int

// The levels.
const (
	Chip Level = iota + 1
	Panel
	Modal
)

// material fills an element with a glass level, or its solid low-power color.
type material struct {
	blur, sat float32
	fill      ui.Color
	solid     ui.Color
	low       bool
}

// PaintMaterial implements ui.Material.
func (m material) PaintMaterial(p *ui.Painter, box ui.Rect, radii [4]float32) {
	if m.low || !p.Accelerated() {
		p.Fill(box, m.solid, radii[0])
		return
	}
	frost.Draw(p, box, shader.Options{
		BackdropBlur: m.blur,
		Radius:       radii[0],
		Uniforms:     map[string]any{"Tint": m.fill, "Sat": m.sat},
	})
}

// Glass returns the material of a level.
func Glass(t *theme.Tokens, l Level) ui.Material {
	switch l {
	case Chip:
		return material{16, 1.4, t.Chip, t.Solid, t.LowPower}
	case Panel:
		return material{24, 1.5, t.Panel, t.Solid, t.LowPower}
	}
	return material{36, 1.6, t.Modal, t.Solid, t.LowPower}
}

// Scrim is the lightbox's ground: the strong scrim over a blur of the window
// (02 §6), the scrim alone in low power.
func Scrim(t *theme.Tokens) ui.Material {
	return material{20, 1, t.ScrimStrong, t.ScrimStrong, t.LowPower}
}

// Clear is the lightbox controls' glass, half-transparent black in low power.
func Clear(t *theme.Tokens) ui.Material {
	return material{12, 1.2, ui.RGBA(20, 20, 24, .38), ui.RGBA(0, 0, 0, .6), t.LowPower}
}

// PaintWindow paints the window's ground and reports whether a GPU drew it, which
// the next theme change uses to choose its transition.
func PaintWindow(p *ui.Painter, r ui.Rect, t *theme.Tokens) (accelerated bool) {
	accelerated = p.Accelerated()
	if t.LowPower || !accelerated {
		p.Fill(r, t.Window, 0)
		return accelerated
	}
	tint.Draw(p, r, shader.Options{Uniforms: map[string]any{
		"Base": t.Window, "A": t.Aurora1, "B": t.Aurora2, "C": t.Aurora3, "Strength": t.TintStrength,
	}})
	return accelerated
}

// Dissolve applies the theme change's circle to the new theme's copy.
func Dissolve(e ui.Element, origin ui.Point, radius float32) ui.Element {
	return e.Effect(dissolve.Effect(shader.Options{Uniforms: map[string]any{
		"Origin": []float32{origin.X, origin.Y}, "Radius": radius,
	}}))
}
