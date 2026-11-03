package qa

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// The image lightbox (02 §5.12, 08 batch 7).

func (a *app) picture(name string, w, h int) {
	a.t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 200, 255})
		}
	}
	f, err := os.Create(filepath.Join(a.dir, name))
	if err != nil {
		a.t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		a.t.Fatal(err)
	}
}

// openPicture opens a document with an image in its third row and waits for
// it to load; it returns the image's middle in the window.
func (a *app) openPicture() (x, y float32) {
	a.t.Helper()
	a.picture("pic.png", 480, 270)
	a.open("img.md", "# Pictures\n\nBefore.\n\n![a picture](pic.png)\n\nAfter.\n")
	l := a.s.ActiveLive()
	a.until("the image loads", func() bool { return l.RowHeight(2) > 200 })
	p := l.RowText(2)
	return p.X + 100, p.Y + 100
}

func (a *app) lightbox() (bool, float32) { return a.s.ActiveLive().LightboxOpen() }

func TestLightboxOpensOnDoubleClick(t *testing.T) {
	a := newApp(t)
	x, y := a.openPicture()
	a.caret(11) // in "Before."
	before := a.text()
	a.tt.ClickAt(x, y)
	a.tt.Frame()
	a.tt.ClickAt(x, y)
	a.animate()
	if open, z := a.lightbox(); !open || z != 1 {
		t.Fatalf("a double click did not open the lightbox: %v %v", open, z)
	}
	a.want(a.text(), before)
	a.wantCaret(11)
	if !a.shows("100%") {
		t.Fatal("the pill does not show the zoom")
	}
	// The image fits the window's width, less its margins, and goes no
	// further than twice its size.
	if _, to := a.s.ActiveLive().LightboxBox(); to.W != 960 || to.H != 540 {
		t.Fatalf("the image is %v", to)
	}
	a.key(0, ui.KeyEscape)
	a.animate()
	if open, _ := a.lightbox(); open || !a.s.ActiveLive().Focused() {
		t.Fatal("Escape did not close the lightbox and give the document back the focus")
	}
	// One click only reveals the image's source, as before.
	a.tt.ClickAt(x, y)
	a.animate()
	if open, _ := a.lightbox(); open {
		t.Fatal("one click opened the lightbox")
	}
}

func TestLightboxZoom(t *testing.T) {
	a := newApp(t)
	a.openPicture()
	if !a.s.ActiveLive().OpenLightbox(2) {
		t.Fatal("no image in row 2")
	}
	a.animate()
	zoom := func(want float32) {
		t.Helper()
		if _, z := a.lightbox(); z != want {
			t.Fatalf("zoom %v, want %v", z, want)
		}
	}
	for range 10 {
		a.click(locales.LiveeditZoomIn())
	}
	zoom(1.6)
	if !a.shows("160%") {
		t.Fatal("the pill does not show 160%")
	}
	a.click(locales.LiveeditFit())
	zoom(1)
	for range 10 {
		a.key(0, ui.KeyMinus)
	}
	zoom(0.6)
	a.key(0, ui.KeyEqual)
	zoom(0.7)
	a.key(0, ui.Key0)
	zoom(1)
	a.click(locales.LiveeditZoomOut())
	a.animate()
	if now, to := a.s.ActiveLive().LightboxBox(); now != to || to.W != 960*0.9 {
		t.Fatalf("at 90%% the image is %v, going to %v", now, to)
	}
	a.click(locales.LiveeditCloseLightbox())
	a.animate()
	if open, _ := a.lightbox(); open {
		t.Fatal("Close did not close the lightbox")
	}
	// A click on the scrim closes it too.
	a.s.ActiveLive().OpenLightbox(2)
	a.animate()
	a.tt.ClickAt(20, 400)
	a.animate()
	if open, _ := a.lightbox(); open {
		t.Fatal("a click on the scrim did not close the lightbox")
	}
}

// The image flies from its place in the document; with motion reduced it is
// there at once.
func TestLightboxFlight(t *testing.T) {
	a := newApp(t)
	a.openPicture()
	l := a.s.ActiveLive()
	l.OpenLightbox(2)
	a.tt.Frame()
	now, to := l.LightboxBox()
	if now == to || now.W != 480 {
		t.Fatalf("the flight starts at %v, not at the image", now)
	}
	a.tt.Advance(100 * time.Millisecond)
	a.tt.Frame()
	if mid, _ := l.LightboxBox(); !(mid.W > now.W && mid.W < to.W) {
		t.Fatalf("mid-flight the image is %v", mid)
	}
	a.key(0, ui.KeyEscape)
	a.animate()

	if err := a.store.Update(func(v *contract.Settings) { v.Appearance.ReduceMotion = "on" }); err != nil {
		t.Fatal(err)
	}
	a.animate()
	l.OpenLightbox(2)
	a.tt.Frame()
	if now, to := l.LightboxBox(); now != to {
		t.Fatalf("with motion reduced the image starts at %v, not %v", now, to)
	}
	a.key(0, ui.KeyEscape)
	a.tt.Frame()
	a.tt.Frame()
	if open, _ := a.lightbox(); open {
		t.Fatal("with motion reduced the lightbox did not close at once")
	}
}
