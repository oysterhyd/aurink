package liveedit

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"aurink/internal/contract"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// Math and diagrams (FE-1.6, 02 §5.9): rendered off the main thread by the
// render service; a skeleton sweeps while they render, then they fade in.
// Without the service, or when a source does not render, the source shows.

const (
	// renderScale is the PNG's pixels per DIP: crisp at 200%.
	renderScale = 2
	// skeletonDelay hides the skeleton for answers faster than it, as cache
	// hits are: they show at once.
	skeletonDelay = 120 * time.Millisecond
	fadeIn        = 220 * time.Millisecond
	blockMathH    = 56
	diagramH      = 220
	// maxRenders bounds the renderings an editor keeps.
	maxRenders = 512
)

type renderKey struct {
	kind     contract.RenderKind
	source   string
	lang     string
	dark     bool
	size     float32
	colorHex string
}

type renderState struct {
	img     ui.ImageSource
	res     contract.RenderResult
	failed  bool
	err     string
	loading bool
	start   time.Time // when the request went out
	arrived time.Time // when the answer came, for the fade; zero for at once
}

func renderKind(o *contract.ObjectRef) (contract.RenderKind, bool) {
	switch o.Kind {
	case contract.ObjMath:
		if o.Display {
			return contract.RenderMathBlock, true
		}
		return contract.RenderMathInline, true
	case contract.ObjDiagram:
		return contract.RenderDiagram, true
	}
	return 0, false
}

func hex(c ui.Color) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// rendered returns what is known of an object's rendering, starting it when
// the service is there; nil when it is not a rendered kind.
func (e *Editor) rendered(o *contract.ObjectRef, size float32) *renderState {
	return e.renderedIn(o, size, e.t.Text, e.t.Dark)
}

// renderedIn is rendered in another text color, as on the dark bubble of the
// inline math preview.
func (e *Editor) renderedIn(o *contract.ObjectRef, size float32, color ui.Color, dark bool) *renderState {
	kind, ok := renderKind(o)
	if !ok {
		return nil
	}
	k := renderKey{kind: kind, source: o.Source, lang: o.Lang, dark: dark, size: size, colorHex: hex(color)}
	st := e.renders[k]
	if st != nil {
		return st
	}
	if e.renders == nil || len(e.renders) >= maxRenders {
		// Typing in a formula leaves a rendering per keystroke: past the
		// bound they go, but for those on their way and the previews'. The
		// rest come back from the service's cache when asked again.
		keep := map[renderKey]*renderState{}
		shown := slices.Collect(maps.Values(e.previews))
		for k, v := range e.renders {
			if v.loading || slices.Contains(shown, v) {
				keep[k] = v
			}
		}
		e.renders = keep
	}
	st = &renderState{}
	e.renders[k] = st
	r := e.opts.Renderer
	if r == nil || e.opts.Dispatcher == nil || !r.Available() {
		st.failed = true
		e.renderOff = true
		return st
	}
	st.loading, st.start = true, e.clock()
	req := contract.RenderRequest{Kind: kind, Source: o.Source, Lang: o.Lang, Dark: k.dark, FontSize: size, Color: k.colorHex, Scale: renderScale}
	d := e.opts.Dispatcher
	go func() {
		res, err := r.Render(context.Background(), req)
		var img ui.ImageSource
		if err == nil {
			if len(res.SVG) > 0 {
				var s *ui.SVG
				if s, err = ui.ParseSVG(res.SVG); err == nil {
					img = s
				}
			} else {
				img, err = ui.DecodeImage(res.PNG)
			}
		}
		d.Post(func() {
			st.loading = false
			if err != nil || img == nil {
				st.failed = true
				if err != nil {
					st.err = err.Error()
				}
				if contract.CodeOf(err) == contract.CodeUnavailable {
					e.renderOff = true
				}
			} else {
				st.img, st.res = img, res
				if now := e.clock(); now.Sub(st.start) >= skeletonDelay {
					st.arrived = now
				}
			}
			// The row's size changes with the answer.
			e.dropAllForms()
			e.invalidate()
		})
	}()
	return st
}

// clock is the time on the frames' clock: the last frame's, or the wall
// clock's before the first.
func (e *Editor) clock() time.Time {
	if e.frameNow.IsZero() {
		return time.Now()
	}
	return e.frameNow
}

// RenderState names what is known of the rendering of the math or diagram
// source: "loading", "rendered", "failed", or "" when none was asked. For tests.
func (e *Editor) RenderState(source string) string {
	for k, st := range e.renders {
		if k.source != source {
			continue
		}
		switch {
		case st.loading:
			return "loading"
		case st.img != nil:
			return "rendered"
		}
		return "failed"
	}
	return ""
}

// RenderUnavailable reports whether math or a diagram could not render because
// the service is not there; the shell says so once.
func (e *Editor) RenderUnavailable() bool { return e.renderOff }

// renderedSize sizes a math or diagram object: the answer's size, the source's
// while it shows as source, a placeholder while it renders.
func (e *Editor) renderedSize(o *contract.ObjectRef, m *rowMetrics) (w, h, descent float32, ok bool) {
	if o == nil {
		return 0, 0, 0, false
	}
	st := e.rendered(o, m.font.Size)
	if st == nil {
		return 0, 0, 0, false
	}
	switch {
	case st.img != nil:
		w, h = st.res.Width, st.res.Height
		if w > m.width {
			w, h = m.width, h*m.width/w
		}
		if o.Display {
			// The row's width: the result centres on the column.
			return m.width, h, 0, true
		}
		descent = max(0, st.res.Height-st.res.Ascent)
		return w, h, descent, true
	case st.failed:
		l := e.shaped(sourceLine(o), e.ty.CodeFont())
		w, h = l.Size()
		if o.Display {
			return m.width, h + 16, 0, true
		}
		return w + 8, h, h * .25, true
	case o.Kind == contract.ObjDiagram:
		return m.width, diagramH, 0, true
	case o.Display:
		return m.width, blockMathH, 0, true
	}
	return m.font.Size * 3, m.font.Size, m.font.Size * .25, true
}

// sourceLine is what shows of an object that did not render: its source on
// one line, as written.
func sourceLine(o *contract.ObjectRef) string {
	if o.Kind == contract.ObjMath && !o.Display {
		return "$" + o.Source + "$"
	}
	return o.Source
}

// paintRendered paints a math or diagram object into its box.
func (rc *rowContext) paintRendered(o *contract.ObjectRef, box ui.Rect) bool {
	e, t, p := rc.e, rc.t, rc.p
	st := e.rendered(o, rc.l.m.font.Size)
	if st == nil {
		return false
	}
	now := p.Now()
	switch {
	case st.img != nil:
		alpha := float32(1)
		if !st.arrived.IsZero() && !e.reduceMotion {
			if f := float32(now.Sub(st.arrived)) / float32(fadeIn); f < 1 {
				alpha = max(f, 0)
				p.AnimationFrame()
			}
		}
		r := box
		if o.Display && r.W > st.res.Width {
			// Centered on the column, as Typora does.
			w := min(st.res.Width, r.W)
			r.X += (r.W - w) / 2
			r.W = w
		}
		if alpha < 1 {
			p.Fill(box, t.Surface.Alpha(1-alpha), 6)
		}
		p.Image(st.img, r, ui.Contain)
		if alpha < 1 {
			// The fade: the sheet over the result, thinning out.
			p.Fill(box, t.Sheet.Alpha(1-alpha), 0)
		}
	case st.failed:
		f := e.ty.CodeFont()
		l := e.shaped(sourceLine(o), f)
		_, h := l.Size()
		if o.Display {
			p.Fill(box, t.Surface, 6)
			if st.err != "" && !e.renderOff {
				p.Stroke(box, t.Danger.Alpha(.6), 6, 1)
			}
			p.Clip(box, 6, func() { p.TextLayout(l, box.X+12, box.Y+(box.H-h)/2, t.Muted) })
			return true
		}
		p.TextLayout(l, box.X+4, box.Y+(box.H-h)/2, t.Muted)
	default:
		if now.Sub(st.start) < skeletonDelay {
			p.After(skeletonDelay - now.Sub(st.start))
			return true
		}
		rc.skeleton(o, box, now)
	}
	return true
}

// skeleton is the loading placeholder: a tinted box with a light sweeping
// across it, and for a block what is happening.
func (rc *rowContext) skeleton(o *contract.ObjectRef, box ui.Rect, now time.Time) {
	e, t, p := rc.e, rc.t, rc.p
	p.Fill(box, t.Surface, 6)
	if !e.reduceMotion {
		const period = 1400 * time.Millisecond
		f := float32(now.UnixMilli()%period.Milliseconds()) / float32(period.Milliseconds())
		x := box.X - box.W*.4 + (box.W*1.4)*f
		p.Clip(box, 6, func() {
			half := box.W * .2
			band := func(x float32, from, to ui.Color) {
				p.FillGradient(ui.Rect{X: x, Y: box.Y, W: half, H: box.H}, ui.LinearGradient{From: from, To: to, Angle: 90}, 0)
			}
			band(x, t.Sheet.Alpha(0), t.Sheet.Alpha(.7))
			band(x+half, t.Sheet.Alpha(.7), t.Sheet.Alpha(0))
		})
		p.AnimationFrame()
	}
	if !o.Display {
		return
	}
	text := locales.LiveeditRenderingMath()
	if o.Kind == contract.ObjDiagram {
		text = locales.LiveeditRenderingDiagram()
	}
	l := e.shaped(text, ui.Font{Family: theme.Sans, Size: theme.UIStatus})
	w, h := l.Size()
	p.TextLayout(l, box.X+(box.W-w)/2, box.Y+(box.H-h)/2, t.Muted)
}
