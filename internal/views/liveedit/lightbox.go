package liveedit

import (
	"strconv"
	"time"

	"aurink/internal/contract"
	"aurink/internal/views/effects"
	"aurink/internal/views/motion"
	"aurink/internal/views/theme"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// The image lightbox (02 §5.12): a double click on an image flies it to the
// middle of the window over a blurred scrim, with a glass pill below to zoom
// it from 60% to 160%; Escape flies it back.

const (
	zoomMin, zoomMax, zoomStep = 0.6, 1.6, 0.1
	// lightboxMargin is the room kept around the fitted image, and pillRoom
	// the room under it for the pill.
	lightboxMargin, pillRoom = 56, 88
	// maxUpscale keeps a small image from blurring as it fills the window.
	maxUpscale = 2
)

type lightbox struct {
	open, closing bool
	src           string
	row           int
	from          ui.Rect // the image's box in the window, where it flies from and back to
	zoom          float32
	fresh         bool // the first frame, where the flight starts
	focus         bool
	// box is where the image was last built, target where it goes; for tests.
	box, target ui.Rect
}

// imageHit is an image under a first click: its row, source and box in the
// view, and the selection before the click revealed its source.
type imageHit struct {
	ok   bool
	row  int
	src  string
	sel  contract.Selection
	when time.Time
}

// flying reports whether the image src of row r is in the lightbox, where
// the row leaves its place empty.
func (e *Editor) flying(r int, src string) bool {
	return e.lb.open && e.lb.row == r && e.lb.src == src
}

// imageAt returns the loaded image at (x, y) of the view: its row, source and
// box in the view.
func (e *Editor) imageAt(x, y float32) (row int, src string, box ui.Rect, ok bool) {
	r := e.rowAtPoint(x, y)
	if r < 0 {
		return 0, "", box, false
	}
	for _, im := range e.imageBoxes(r) {
		if x >= im.box.X && x < im.box.X+im.box.W && y >= im.box.Y && y < im.box.Y+im.box.H {
			return r, im.src, im.box, true
		}
	}
	return 0, "", box, false
}

type imageBox struct {
	src string
	box ui.Rect
}

// imageBoxes lists a row's loaded images and their boxes in the view.
func (e *Editor) imageBoxes(r int) []imageBox {
	l := e.current(r)
	objs := l.layout.Objects()
	x0, y0 := e.contentX()+l.m.x, e.rowTextY(r)
	var out []imageBox
	k := 0
	for i := range l.proj.Segments {
		s := &l.proj.Segments[i]
		if s.Kind != contract.SegObject {
			continue
		}
		if k >= len(objs) {
			break
		}
		o := objs[k].Rect
		k++
		if s.Object == nil || s.Object.Kind != contract.ObjImage || e.image(s.Object.Source).src == nil {
			continue
		}
		out = append(out, imageBox{s.Object.Source, ui.Rect{X: x0 + o.X, Y: y0 + o.Y, W: o.W, H: o.H}})
	}
	return out
}

// imageClick handles a press on an image: the first click is noted, as it
// reveals the image's source; the second, soon after, gives back the
// selection the first one moved and opens the image in the lightbox. It
// reports whether it took the press.
func (e *Editor) imageClick(ev ui.InputEvent) bool {
	if ev.Clicks == 2 && e.lastImage.ok && ev.Time.Sub(e.lastImage.when) < time.Second {
		hit := e.lastImage
		e.lastImage = imageHit{}
		e.model.SetSelection(hit.sel)
		e.dragging = false
		e.openLightbox(hit.row, hit.src)
		return true
	}
	e.lastImage = imageHit{}
	if ev.Clicks != 1 || ev.Mods != 0 {
		return false
	}
	if r, src, _, ok := e.imageAt(ev.X, ev.Y); ok {
		e.lastImage = imageHit{ok: true, row: r, src: src, sel: e.model.Selection(), when: ev.Time}
	}
	return false
}

// OpenLightbox opens the first loaded image of row r in the lightbox; false
// when it has none. For tests and tools; a double click does it.
func (e *Editor) OpenLightbox(r int) bool {
	if r < 0 || r >= len(e.rows) {
		return false
	}
	ims := e.imageBoxes(r)
	if len(ims) == 0 {
		return false
	}
	e.openLightbox(r, ims[0].src)
	return true
}

// LightboxOpen reports whether the lightbox shows, and its zoom.
func (e *Editor) LightboxOpen() (bool, float32) {
	return e.lb.open && !e.lb.closing, e.lb.zoom
}

// LightboxBox returns where the lightbox's image is in the window, and
// where it goes. For tests.
func (e *Editor) LightboxBox() (now, target ui.Rect) { return e.lb.box, e.lb.target }

func (e *Editor) openLightbox(row int, src string) {
	e.lb = lightbox{open: true, src: src, row: row, zoom: 1, fresh: true, focus: true, from: e.imageWindowBox(row, src)}
	e.invalidate()
}

// imageWindowBox returns the box in the window of the image src in row r, as
// it is now; the window's middle when it does not show.
func (e *Editor) imageWindowBox(r int, src string) ui.Rect {
	if r >= 0 && r < len(e.rows) && r >= e.first && r <= e.last {
		for _, im := range e.imageBoxes(r) {
			if im.src == src {
				b := im.box
				b.X += e.orgX
				b.Y += e.orgY
				return b
			}
		}
	}
	return ui.Rect{X: e.orgX + e.viewW/2, Y: e.orgY + e.viewH/2}
}

func (e *Editor) closeLightbox() {
	if !e.lb.open || e.lb.closing {
		return
	}
	e.lb.closing = true
	e.lb.from = e.imageWindowBox(e.lb.row, e.lb.src)
	e.focusReq = true
	e.invalidate()
}

func (e *Editor) zoomLightbox(z float32) {
	// Steps of a tenth, without the float's drift.
	z = float32(int(z*10+0.5)) / 10
	e.lb.zoom = max(zoomMin, min(zoomMax, z))
	e.invalidate()
}

// Lightbox builds the lightbox over the whole window; the shell calls it last,
// in the window's root.
func (e *Editor) Lightbox(c *ui.Context) {
	lb := &e.lb
	if !lb.open {
		return
	}
	st := e.image(lb.src)
	if st.src == nil {
		lb.open = false
		return
	}
	t := e.t
	ww, wh := c.Size()
	root := ui.Box(c.Key("lightbox")).Absolute().Fill().Focusable().FocusRing(false).
		Role(ui.RoleDialog).Label(locales.LiveeditLightbox())
	target := float32(1)
	if lb.closing {
		target = 0
	}
	var k float32
	switch {
	case e.reduceMotion:
		k = root.Jump("open", target)
	case lb.fresh:
		root.Jump("open", 0)
		k = root.AnimateTo("open", target, motion.Gentle)
	default:
		k = root.AnimateTo("open", target, motion.Gentle)
	}
	lb.fresh = false
	if lb.closing && k <= 0.001 {
		lb.open = false
		e.invalidate()
		return
	}
	if lb.focus {
		lb.focus = false
		root.Focus()
	}
	root.OnShortcut(0, ui.KeyEscape, e.closeLightbox)
	root.OnShortcut(0, ui.KeyMinus, func() { e.zoomLightbox(lb.zoom - zoomStep) })
	root.OnShortcut(0, ui.KeyEqual, func() { e.zoomLightbox(lb.zoom + zoomStep) })
	root.OnShortcut(ui.Shift, ui.KeyEqual, func() { e.zoomLightbox(lb.zoom + zoomStep) })
	root.OnShortcut(0, ui.Key0, func() { e.zoomLightbox(1) })

	// Fitted in the window, at most twice its own size, then zoomed.
	iw, ih := ui.ImageSize(st.src)
	fit := min(maxUpscale, (ww-2*lightboxMargin)/iw, (wh-lightboxMargin-pillRoom)/ih)
	var z float32
	if e.reduceMotion {
		z = root.Jump("zoom", lb.zoom)
	} else {
		z = root.AnimateTo("zoom", lb.zoom, motion.Snappy)
	}
	w, h := iw*fit*z, ih*fit*z
	to := ui.Rect{X: (ww - w) / 2, Y: (wh - pillRoom + lightboxMargin - h) / 2, W: w, H: h}
	from := lb.from
	box := ui.Rect{X: lerp(from.X, to.X, k), Y: lerp(from.Y, to.Y, k), W: lerp(from.W, to.W, k), H: lerp(from.H, to.H, k)}
	lb.box, lb.target = box, to

	root.Children(func() {
		scrim := ui.Box(c.Key("scrim")).Absolute().Fill().Opacity(min(1, k)).Material(effects.Scrim(t))
		if scrim.Clicked() {
			e.closeLightbox()
		}
		ui.Box(c.Key("image")).Absolute().Fill().PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
			b := box
			b.X += r.X
			b.Y += r.Y
			p.Shadow(b, 6, 0, 18, 48, 0, t.ShadowColor.Alpha(k))
			p.Clip(b, 6, func() { p.Image(st.src, b, ui.Contain) })
		})
		ui.Row(c.Key("pillRow")).Absolute().Left(0).Right(0).Bottom(24).Justify(ui.Center).Opacity(min(1, k)).
			PassThrough().Children(func() { e.lightboxPill(c, t) })
	})
}

// lightboxPill is the glass pill under the image: zoom out, the zoom, zoom
// in, fit, close.
func (e *Editor) lightboxPill(c *ui.Context, t *theme.Tokens) {
	lb := &e.lb
	pill := ui.Row(c.Key("pill")).Height(40).Padding(0, 6).Gap(2).AlignItems(ui.Center).Radius(20).
		Material(effects.Clear(t)).Border(1, ui.RGBA(255, 255, 255, .18)).Role(ui.RoleToolbar).Label(locales.LiveeditLightbox())
	white := ui.RGBA(255, 255, 255, .92)
	button := func(key, text string, icon *ui.SVG, label string, fn func()) {
		b := ui.Row(c.Key(key)).Height(30).MinWidth(30).Padding(0, 8).Radius(15).Justify(ui.Center).AlignItems(ui.Center).
			Role(ui.RoleButton).Label(label)
		switch {
		case b.Hovered():
			b.Background(ui.RGBA(255, 255, 255, .2))
		case key == "close":
			b.Background(ui.RGBA(255, 255, 255, .12))
		}
		b.Children(func() {
			if icon != nil {
				ui.Icon(c, icon).Size(14, 14).TextColor(white)
			} else {
				ui.Text(c, text).FontSize(theme.UIText).TextColor(white).SingleLine()
			}
		})
		if b.Clicked() {
			fn()
		}
	}
	pill.Children(func() {
		button("out", "−", nil, locales.LiveeditZoomOut(), func() { e.zoomLightbox(lb.zoom - zoomStep) })
		ui.Text(c, strconv.Itoa(int(lb.zoom*100+0.5))+"%").FontSize(theme.UIText).TextColor(white).
			FontFeatures("tnum").MinWidth(44).TextAlign(ui.Center).Role(ui.RoleStatus)
		button("in", "+", nil, locales.LiveeditZoomIn(), func() { e.zoomLightbox(lb.zoom + zoomStep) })
		ui.Box(c).Size(1, 16).Margin(0, 4).Background(ui.RGBA(255, 255, 255, .22))
		button("fit", locales.LiveeditFit(), nil, locales.LiveeditFit(), func() { e.zoomLightbox(1) })
		button("close", locales.LiveeditCloseLightbox(), nil, locales.LiveeditCloseLightbox(), e.closeLightbox)
	})
}

func lerp(a, b, k float32) float32 { return a + (b-a)*k }
