// rendercheck measures the real, offscreen render service in a native window.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"time"

	"aurink/internal/adapters/webviewhost"
	renderassets "aurink/internal/assets/render"
	"aurink/internal/contract"
	"aurink/internal/core/render"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	xdraw "golang.org/x/image/draw"
)

type formula struct {
	ID, Group, TeX string
	Inline, Fail   bool
}
type diagram struct{ ID, Source string }
type measurement struct {
	ID                           string  `json:"id"`
	ExpectedFailure              bool    `json:"expected_failure,omitempty"`
	Format                       string  `json:"format,omitempty"`
	UncachedMS                   float64 `json:"uncached_ms"`
	CachedMS                     float64 `json:"cached_ms,omitempty"`
	ParseMS                      float64 `json:"parse_ms,omitempty"`
	Width, Height, Ascent, Scale float32
	Pixels                       image.Point `json:"pixels"`
	Parsed, Ink                  bool
	Error                        string `json:"error,omitempty"`
}
type timing struct {
	Count         int `json:"count"`
	P50, P95, Max float64
}
type report struct {
	Platform, Go                               string
	ColdStartMS, RestartMS                     float64
	Formulas, Diagrams, PNGChecks, ThemeChecks []measurement
	Uncached, WithParse, Cached                timing
	Memory                                     []memorySample
	IdleStopped                                bool
	NetworkResources                           int
	Failures                                   []string
}
type measuredHost struct {
	*webviewhost.Host
	forced bool
	mu     sync.Mutex
	starts []time.Duration
	stops  chan struct{}
	active bool
}

func (h *measuredHost) Available() bool { return !h.forced && h.Host.Available() }
func (h *measuredHost) Start(ctx context.Context, page, base string) error {
	start := time.Now()
	err := h.Host.Start(ctx, page, base)
	h.mu.Lock()
	h.starts = append(h.starts, time.Since(start))
	h.active = err == nil
	h.mu.Unlock()
	return err
}
func (h *measuredHost) Stop() {
	h.mu.Lock()
	started := h.active
	h.active = false
	h.mu.Unlock()
	h.Host.Stop()
	if started {
		select {
		case h.stops <- struct{}{}:
		default:
		}
	}
}
func (h *measuredHost) startTimes() []time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.starts)
}

type app struct{ fallback bool }

func (a *app) view(c *ui.Context) {
	ui.Column(c).Fill().Padding(20).Gap(12).Background(ui.RGB(255, 255, 255)).TextColor(ui.RGB(17, 17, 17)).Children(func() {
		ui.Text(c, "Aurink render check").FontSize(22).Bold()
		if a.fallback {
			ui.Text(c, "WebView unavailable · source fallback").FontSize(16)
			ui.Text(c, `\frac{a}{b}`).FontSize(16)
			ui.Text(c, "flowchart LR\nA --> B").FontSize(16)
		} else {
			ui.Text(c, "Measuring the offscreen rendering service…").FontSize(16)
		}
	})
}

func main() {
	out := flag.String("out", "reports/render", "output directory")
	unavailable := flag.Bool("force-unavailable", false, "simulate a missing browser runtime")
	flag.Parse()
	if err := os.MkdirAll(*out, 0755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	done := make(chan error, 1)
	mygo.App.WhenReady(func() {
		a := &app{fallback: *unavailable}
		win := mygo.NewWindow(mygo.WindowOptions{Title: "Aurink render check", Width: 800, Height: 400, Hidden: true, Content: ui.View(a.view)})
		win.ShowInactive()
		go func() {
			var err error
			if *unavailable {
				err = fallback(*out, win)
			} else {
				err = run(*out, win)
			}
			done <- err
			mygo.App.Quit()
		}()
	})
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := <-done; err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func fallback(out string, win *mygo.Window) error {
	h := &measuredHost{Host: webviewhost.New(win), forced: true, stops: make(chan struct{}, 1)}
	r := render.New(h, render.Options{Page: renderassets.Page()})
	source := `\frac{a}{b}`
	_, err := r.Render(context.Background(), contract.RenderRequest{Kind: contract.RenderMathInline, Source: source})
	if contract.CodeOf(err) != contract.CodeUnavailable || len(h.startTimes()) != 0 {
		return fmt.Errorf("fallback returned %v", err)
	}
	if b, e := win.CapturePage(); e == nil {
		if e = os.WriteFile(filepath.Join(out, "fallback-window.png"), b, 0644); e != nil {
			return e
		}
	} else {
		return e
	}
	return saveJSON(filepath.Join(out, "fallback.json"), map[string]any{"forced": true, "source": source, "error": err.Error(), "webview_started": false})
}

func run(out string, win *mygo.Window) error {
	var formulas []formula
	var diagrams []diagram
	if err := readJSON("testdata/render/formulas.json", &formulas); err != nil {
		return err
	}
	if err := readJSON("testdata/render/diagrams.json", &diagrams); err != nil {
		return err
	}
	h := &measuredHost{Host: webviewhost.New(win), stops: make(chan struct{}, 4)}
	r := render.New(h, render.Options{Page: renderassets.Page(), Idle: 2 * time.Second})
	rep := report{Platform: runtime.GOOS + "/" + runtime.GOARCH, Go: runtime.Version()}
	var uncached, withParse, cached []float64
	var sheet []image.Image
	var pid int
	for _, f := range formulas {
		kind := contract.RenderMathBlock
		if f.Inline {
			kind = contract.RenderMathInline
		}
		req := contract.RenderRequest{Kind: kind, Source: f.TeX, FontSize: 20, Color: "#111111", Scale: 1}
		start := time.Now()
		res, err := r.Render(context.Background(), req)
		elapsed := ms(time.Since(start))
		m := measurement{ID: f.ID, ExpectedFailure: f.Fail, UncachedMS: elapsed}
		if f.Fail {
			if contract.CodeOf(err) != contract.CodeRender {
				rep.Failures = append(rep.Failures, fmt.Sprintf("%s: expected CodeRender, got %v", f.ID, err))
			}
			if err != nil {
				m.Error = err.Error()
			}
		} else if err != nil {
			m.Error = err.Error()
			rep.Failures = append(rep.Failures, fmt.Sprintf("%s: %v", f.ID, err))
		} else {
			uncached = append(uncached, elapsed)
			if pid == 0 {
				pid = h.ProcessID()
				rep.Memory = append(rep.Memory, processMemory("after_start", pid))
			}
			img, e := inspect(&m, res)
			if e != nil {
				m.Error = e.Error()
				rep.Failures = append(rep.Failures, fmt.Sprintf("%s: %v", f.ID, e))
			}
			withParse = append(withParse, elapsed+m.ParseMS)
			if img != nil && (len(sheet) < 16 || f.ID == "F20" || (f.Group == "cjk" && len(sheet) < 24)) {
				sheet = append(sheet, img)
			}
			if img != nil && f.ID == "F20" {
				if err := savePNG(filepath.Join(out, "array-lines.png"), img); err != nil {
					rep.Failures = append(rep.Failures, err.Error())
				}
			}
			start = time.Now()
			for range 1000 {
				_, e = r.Render(context.Background(), req)
				if e != nil {
					break
				}
			}
			m.CachedMS = ms(time.Since(start)) / 1000
			cached = append(cached, m.CachedMS)
			if e != nil {
				rep.Failures = append(rep.Failures, fmt.Sprintf("%s cached: %v", f.ID, e))
			}
		}
		rep.Formulas = append(rep.Formulas, m)
		fmt.Printf("%s %.3f ms %s %s\n", f.ID, elapsed, m.Format, m.Error)
	}
	for _, d := range diagrams {
		req := contract.RenderRequest{Kind: contract.RenderDiagram, Lang: "mermaid", Source: d.Source, FontSize: 16, Color: "#111111", Scale: 1}
		start := time.Now()
		res, err := r.Render(context.Background(), req)
		m := measurement{ID: d.ID, UncachedMS: ms(time.Since(start))}
		if err == nil {
			var img image.Image
			img, err = inspect(&m, res)
			if err == nil {
				err = savePNG(filepath.Join(out, "diagram-"+d.ID+".png"), img)
			}
		}
		if err != nil {
			m.Error = err.Error()
			rep.Failures = append(rep.Failures, fmt.Sprintf("diagram %s: %v", d.ID, err))
		}
		rep.Diagrams = append(rep.Diagrams, m)
	}
	for _, scale := range []float32{1, 2} {
		req := contract.RenderRequest{Kind: contract.RenderMathInline, Source: `\text{中文基线}=x^2`, FontSize: 20, Color: "#111111", Scale: scale}
		start := time.Now()
		res, err := r.Render(context.Background(), req)
		m := measurement{ID: fmt.Sprintf("cjk-scale-%g", scale), UncachedMS: ms(time.Since(start))}
		if err == nil {
			var img image.Image
			img, err = inspect(&m, res)
			if err == nil {
				err = savePNG(filepath.Join(out, m.ID+".png"), img)
			}
		}
		if err == nil && (m.Format != "png" || math.Abs(float64(m.Width*scale)-float64(m.Pixels.X)) > 0.001 || math.Abs(float64(m.Height*scale)-float64(m.Pixels.Y)) > 0.001 || m.Ascent <= 0 || m.Ascent > m.Height) {
			err = fmt.Errorf("PNG size or ascent invalid")
		}
		if err != nil {
			m.Error = err.Error()
			rep.Failures = append(rep.Failures, fmt.Sprintf("%s: %v", m.ID, err))
		}
		rep.PNGChecks = append(rep.PNGChecks, m)
	}
	for _, probe := range []struct {
		id           string
		kind         contract.RenderKind
		source, lang string
	}{
		{"colored-svg", contract.RenderMathInline, `x^2+1`, ""},
		{"colored-png", contract.RenderMathInline, `\text{颜色}=x`, ""},
		{"dark-diagram", contract.RenderDiagram, "flowchart LR\nA[Start] --> B[End]", "mermaid"},
	} {
		req := contract.RenderRequest{Kind: probe.kind, Source: probe.source, Lang: probe.lang, Dark: true, FontSize: 20, Color: "#d02030", Scale: 1}
		start := time.Now()
		res, err := r.Render(context.Background(), req)
		m := measurement{ID: probe.id, UncachedMS: ms(time.Since(start))}
		if err == nil {
			var img image.Image
			img, err = inspect(&m, res)
			if err == nil && !hasRed(img) {
				err = fmt.Errorf("requested color is not visible")
			}
			if err == nil {
				err = savePNG(filepath.Join(out, probe.id+".png"), img)
			}
		}
		if err != nil {
			m.Error = err.Error()
			rep.Failures = append(rep.Failures, fmt.Sprintf("%s: %v", m.ID, err))
		}
		rep.ThemeChecks = append(rep.ThemeChecks, m)
	}
	if pid != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		data, err := h.Eval(ctx, `performance.getEntriesByType('resource').filter(r => /^https?:/.test(r.name)).length`)
		cancel()
		if err != nil || json.Unmarshal(data, &rep.NetworkResources) != nil {
			rep.Failures = append(rep.Failures, fmt.Sprintf("network resource audit: %v", err))
		} else if rep.NetworkResources != 0 {
			rep.Failures = append(rep.Failures, fmt.Sprintf("%d network resource loads", rep.NetworkResources))
		}
	}
	if len(sheet) > 0 {
		if err := savePNG(filepath.Join(out, "formula-grid.png"), grid(sheet)); err != nil {
			rep.Failures = append(rep.Failures, err.Error())
		}
	}
	rep.Uncached, rep.WithParse, rep.Cached = summarize(uncached), summarize(withParse), summarize(cached)
	rep.Memory = append(rep.Memory, processMemory("after_corpus", pid))
	select {
	case <-h.stops:
		rep.IdleStopped = true
	case <-time.After(15 * time.Second):
		rep.Failures = append(rep.Failures, "idle stop timed out")
	}
	rep.Memory = append(rep.Memory, processMemory("after_idle", pid))
	start := time.Now()
	_, err := r.Render(context.Background(), contract.RenderRequest{Kind: contract.RenderMathInline, Source: "x+restart", FontSize: 20, Color: "#111111", Scale: 1})
	rep.RestartMS = ms(time.Since(start))
	if err != nil {
		rep.Failures = append(rep.Failures, fmt.Sprintf("restart: %v", err))
	}
	starts := h.startTimes()
	if len(starts) > 0 {
		rep.ColdStartMS = ms(starts[0])
	}
	h.Stop()
	if rep.WithParse.Count == 0 {
		rep.Failures = append(rep.Failures, "no successful formulas: budgets are not measurable")
	} else if rep.WithParse.P95 >= 50 {
		rep.Failures = append(rep.Failures, fmt.Sprintf("uncached plus parse p95 %.3f ms exceeds 50 ms budget", rep.WithParse.P95))
	}
	if rep.Cached.Max >= 1 {
		rep.Failures = append(rep.Failures, fmt.Sprintf("cached maximum %.3f ms exceeds 1 ms budget", rep.Cached.Max))
	}
	if err := saveJSON(filepath.Join(out, "results.json"), rep); err != nil {
		return err
	}
	if err := writeSummary(filepath.Join(out, "summary.md"), rep); err != nil {
		return err
	}
	if len(rep.Failures) > 0 {
		return fmt.Errorf("render check recorded %d failures; see %s", len(rep.Failures), filepath.Join(out, "summary.md"))
	}
	return nil
}

func inspect(m *measurement, r contract.RenderResult) (image.Image, error) {
	m.Width, m.Height, m.Ascent, m.Scale = r.Width, r.Height, r.Ascent, r.Scale
	var img image.Image
	if len(r.SVG) > 0 {
		m.Format = "svg"
		start := time.Now()
		svg, err := ui.ParseSVG(r.SVG)
		m.ParseMS = ms(time.Since(start))
		if err != nil {
			return nil, err
		}
		m.Parsed = true
		w, h := int(math.Ceil(float64(r.Width))), int(math.Ceil(float64(r.Height)))
		if w <= 0 || h <= 0 || w*h > 16_000_000 {
			return nil, fmt.Errorf("invalid render size %dx%d", w, h)
		}
		img = ui.Render(func(c *ui.Context) {
			ui.Box(c).Fill().Background(ui.RGB(255, 255, 255)).Children(func() { ui.Image(c, svg).Size(r.Width, r.Height).TextColor(ui.RGB(17, 17, 17)) })
		}, w, h, 1)
	} else {
		m.Format = "png"
		start := time.Now()
		decoded, err := png.Decode(bytes.NewReader(r.PNG))
		m.ParseMS = ms(time.Since(start))
		if err != nil {
			return nil, err
		}
		m.Parsed = true
		img = decoded
	}
	m.Pixels = image.Pt(img.Bounds().Dx(), img.Bounds().Dy())
	m.Ink = hasInk(img)
	if !m.Ink {
		return img, fmt.Errorf("image has no visible ink")
	}
	return img, nil
}
func hasInk(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a > 0 && (r+0xffff-a < 0xf000 || g+0xffff-a < 0xf000 || bl+0xffff-a < 0xf000) {
				return true
			}
		}
	}
	return false
}
func hasRed(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a > 0 && r > g+5000 && r > bl+5000 {
				return true
			}
		}
	}
	return false
}
func grid(imgs []image.Image) image.Image {
	const cw, ch, cols = 220, 100, 4
	result := image.NewRGBA(image.Rect(0, 0, cw*cols, ch*((len(imgs)+cols-1)/cols)))
	draw.Draw(result, result.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for i, img := range imgs {
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		s := math.Min(1, math.Min(float64(cw-12)/float64(w), float64(ch-12)/float64(h)))
		dst := image.Rect((i%cols)*cw+6, (i/cols)*ch+6, (i%cols)*cw+6+int(float64(w)*s), (i/cols)*ch+6+int(float64(h)*s))
		xdraw.CatmullRom.Scale(result, dst, img, img.Bounds(), draw.Over, nil)
	}
	return result
}
func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
func saveJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}
func savePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
func summarize(v []float64) timing {
	if len(v) == 0 {
		return timing{}
	}
	v = slices.Clone(v)
	slices.Sort(v)
	return timing{len(v), v[int(math.Ceil(float64(len(v))*.5))-1], v[int(math.Ceil(float64(len(v))*.95))-1], v[len(v)-1]}
}
func writeSummary(path string, r report) error {
	s := fmt.Sprintf("# Render check\n\nPlatform: %s; %s.\n\nFormulas: %d (%d successful timings); diagrams: %d. Uncached service p50 %.3f ms, p95 %.3f ms, max %.3f ms. Including SVG parsing or PNG decoding: p50 %.3f ms, p95 %.3f ms, max %.3f ms (budget p95 < 50 ms). Cached p50 %.6f ms, p95 %.6f ms, max %.6f ms (budget < 1 ms); each cache sample is the mean of 1,000 calls.\n\nCold start %.3f ms (page assembly excluded); idle stopped: %t; restart %.3f ms; HTTP(S) resource loads: %d. Memory is WebView2 process-tree working set, not exclusive physical memory; S2 recorded 361–565 MiB.\n\n", r.Platform, r.Go, len(r.Formulas), r.Uncached.Count, len(r.Diagrams), r.Uncached.P50, r.Uncached.P95, r.Uncached.Max, r.WithParse.P50, r.WithParse.P95, r.WithParse.Max, r.Cached.P50, r.Cached.P95, r.Cached.Max, r.ColdStartMS, r.IdleStopped, r.RestartMS, r.NetworkResources)
	for _, m := range r.Memory {
		s += fmt.Sprintf("- %s: %+v\n", m.Stage, m)
	}
	s += "\nPNG path:\n\n"
	for _, m := range r.PNGChecks {
		s += fmt.Sprintf("- %s: %.3f ms, DIP %.2f × %.2f, pixels %d × %d, ascent %.2f, error %s\n", m.ID, m.UncachedMS, m.Width, m.Height, m.Pixels.X, m.Pixels.Y, m.Ascent, m.Error)
	}
	s += "\nFailures and missed budgets:\n\n"
	if len(r.Failures) == 0 {
		s += "None.\n"
	}
	for _, f := range r.Failures {
		s += "- " + f + "\n"
	}
	return os.WriteFile(path, []byte(s), 0644)
}
