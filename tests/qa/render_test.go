package qa

import (
	"context"
	"fmt"
	"image"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/locales"
	"github.com/egoist/mygo/ui"
)

// Math, diagrams, the Front Matter header and the table of contents
// (FE-1.6, 08 batch 6).

// fakeRenderer answers with an SVG box the size of the source; hold makes
// it wait until the channel closes.
type fakeRenderer struct {
	mu        sync.Mutex
	available bool
	calls     int
	hold      chan struct{}
	fail      string // a source that does not render
}

func (f *fakeRenderer) Available() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.available
}

func (f *fakeRenderer) Render(ctx context.Context, req contract.RenderRequest) (contract.RenderResult, error) {
	f.mu.Lock()
	f.calls++
	hold, fail := f.hold, f.fail
	f.mu.Unlock()
	if hold != nil {
		<-hold
	}
	if req.Source == fail {
		return contract.RenderResult{}, contract.E(contract.CodeRender, "fake", fmt.Errorf("bad TeX"))
	}
	w, h := float32(10*len(req.Source)), float32(20)
	if req.Kind == contract.RenderDiagram {
		h = 120
	}
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%g" height="%g" viewBox="0 0 %g %g"><rect width="%g" height="%g" fill="#336"/></svg>`, w, h, w, h, w, h)
	return contract.RenderResult{SVG: []byte(svg), Width: w, Height: h, Ascent: h * .75, Scale: 1}, nil
}

func (f *fakeRenderer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (a *app) renderState(src string) string { return a.s.ActiveLive().RenderState(src) }

func TestMathAndDiagramsRender(t *testing.T) {
	a := newApp(t)
	a.open("m.md", "# Math\n\ninline $x^2$ here\n\n$$\nsum_0^1 f\n$$\n\n```mermaid\ngraph TD; A-->B\n```\n\nend\n")
	a.caret(len(a.text()) - 1)
	a.until("everything renders", func() bool {
		return a.renderState("x^2") == "rendered" && a.renderState("sum_0^1 f") == "rendered" && a.renderState("graph TD; A-->B") == "rendered"
	})
	// The rows take the answers' sizes: the diagram's is 120 high.
	l := a.s.ActiveLive()
	if h := l.RowHeight(3); h < 120 {
		t.Fatalf("the diagram's row is %v high", h)
	}
	calls := a.render.count()
	// Another document with the same formula asks nothing more of the service
	// for what this editor has; a new editor asks once.
	a.caret(0)
	a.animate()
	if a.render.count() != calls {
		t.Fatalf("moving the caret rendered again: %d calls, were %d", a.render.count(), calls)
	}
}

func TestMathShowsSourceWithoutTheService(t *testing.T) {
	a := newApp(t)
	a.render.mu.Lock()
	a.render.available = false
	a.render.mu.Unlock()
	a.open("m.md", "inline $x^2$ here\n\nend\n")
	a.caret(len(a.text()) - 1)
	a.animate()
	if a.renderState("x^2") != "failed" || !a.s.ActiveLive().RenderUnavailable() || a.render.count() != 0 {
		t.Fatalf("state %q, unavailable %v, calls %d", a.renderState("x^2"), a.s.ActiveLive().RenderUnavailable(), a.render.count())
	}
}

func TestBadMathShowsSource(t *testing.T) {
	a := newApp(t)
	a.render.fail = "frac{"
	a.open("m.md", "$$\nfrac{\n$$\n\nend\n")
	a.caret(len(a.text()) - 1)
	a.until("the formula fails", func() bool { return a.renderState("frac{") == "failed" })
	if a.s.ActiveLive().RenderUnavailable() {
		t.Fatal("a bad formula made the service unavailable")
	}
}

// While a formula renders a skeleton sweeps (frames keep coming); after,
// none do.
func TestRenderSkeletonThenRest(t *testing.T) {
	a := newApp(t)
	hold := make(chan struct{})
	a.render.hold = hold
	a.open("m.md", "$$\nE=mc^2\n$$\n\nend\n")
	a.caret(len(a.text()) - 1)
	a.tt.Advance(200 * time.Millisecond)
	a.tt.Frame()
	if a.renderState("E=mc^2") != "loading" {
		t.Fatalf("the formula is %q while it renders", a.renderState("E=mc^2"))
	}
	// The light moves across the skeleton from one frame to the next.
	p := a.s.ActiveLive().RowText(0)
	box := image.Rect(int(p.X+a.s.SheetLeft()), int(p.Y+46), int(p.X+a.s.SheetLeft()+400), int(p.Y+46+40))
	snap := func() []uint32 {
		img, out := a.tt.Image(), []uint32(nil)
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				out = append(out, r+g<<1+b<<2)
			}
		}
		return out
	}
	before := snap()
	a.tt.Advance(300 * time.Millisecond)
	a.tt.Frame()
	if slices.Equal(before, snap()) || !a.tt.FramePending() {
		t.Fatalf("the skeleton does not sweep: pending %v", a.tt.FramePending())
	}
	close(hold)
	a.until("the formula renders", func() bool { return a.renderState("E=mc^2") == "rendered" })
	a.tt.Advance(time.Second)
	a.tt.Frame()
	if a.tt.FramePending() {
		t.Fatal("frames keep coming after the formula faded in")
	}
}

func TestFrontMatterHeaderAndTOC(t *testing.T) {
	a := newApp(t)
	a.open("h.md", "---\ntitle: Notes\ntags: [a, b]\n---\n\n# One\n\n[TOC]\n\ntext\n\n## Two\n\nmore\n")
	a.caret(len(a.text()) - 1)
	a.animate()
	l := a.s.ActiveLive()
	// The header is 28 high, with the paragraph's space around it.
	if l.Display(0) != "￼" || l.RowHeight(0) < 28+10 {
		t.Fatalf("the header row shows %q, %v high", l.Display(0), l.RowHeight(0))
	}
	// The table of contents lists two headings; a click on the second goes to it.
	r := 2
	if l.Display(r) != "￼" || l.RowHeight(r) < 2*28 {
		t.Fatalf("the TOC row shows %q, %v high", l.Display(r), l.RowHeight(r))
	}
	p := l.RowText(r)
	a.tt.ClickAt(p.X+30+a.s.SheetLeft(), p.Y+10+28+14+46)
	a.animate()
	a.wantCaret(strings.Index(a.text(), "## Two"))
	if strings.Contains(locales.LiveeditTocEmpty(), "%") {
		t.Fatal("the empty TOC text has a placeholder")
	}
}

// A wiki link shows as a link; Ctrl+click opens the note it names next to
// the document.
func TestWikiLinkOpensTheNote(t *testing.T) {
	a := newApp(t)
	note := a.file("beta.md", "# Beta\n")
	a.open("alpha.md", "[[beta]] is next\n\nend\n")
	a.caret(len(a.text()) - 1)
	a.animate()
	if l := a.s.ActiveLive(); l.Display(0) != "beta is next" {
		t.Fatalf("the wiki link row shows %q", l.Display(0))
	}
	p := a.s.ActiveLive().RowText(0)
	// "beta" is in a color; the text after it is not.
	// The ink's mean spread between channels: the antialiasing's fringes
	// average out on black text.
	tint := func(x0 float32) int {
		img, n := a.tt.Image(), 0
		var sr, sg, sb int
		for y := int(p.Y + 46); y < int(p.Y+46+22); y++ {
			for x := int(x0 + a.s.SheetLeft()); x < int(x0+a.s.SheetLeft()+30); x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				r, g, b = r>>8, g>>8, b>>8
				if r+g+b < 3*160 {
					sr, sg, sb, n = sr+int(r), sg+int(g), sb+int(b), n+1
				}
			}
		}
		n = max(n, 1)
		sr, sg, sb = sr/n, sg/n, sb/n
		return max(sr, sg, sb) - min(sr, sg, sb)
	}
	if in, after := tint(p.X), tint(p.X+70); in < 40 || after > 25 {
		t.Fatalf("the ink's tint: %d in the link, %d after it", in, after)
	}
	a.tt.ClickAtWith(ui.Cmd, p.X+10+a.s.SheetLeft(), p.Y+8+46)
	a.settle()
	if got := a.s.ActiveSession().State().Path; got != note {
		t.Fatalf("the active document is %q, want %q", got, note)
	}
}

func TestRenderUnavailableSaysSoOnce(t *testing.T) {
	a := newApp(t)
	a.render.mu.Lock()
	a.render.available = false
	a.render.mu.Unlock()
	a.open("m.md", "$a$\n\n$b$\n\nend\n")
	a.caret(len(a.text()) - 1)
	a.animate()
	if !a.shows(locales.ShellRenderUnavailable()) {
		t.Fatal("no notice that math shows as source")
	}
	a.tt.Advance(6 * time.Second) // the notice goes
	a.tt.Frame()
	if a.shows(locales.ShellRenderUnavailable()) {
		t.Fatal("the notice stays")
	}
	a.open("n.md", "$c$\n\nend\n")
	a.caret(len(a.text()) - 1)
	a.animate()
	if a.shows(locales.ShellRenderUnavailable()) {
		t.Fatal("the notice came back for the next document")
	}
}
