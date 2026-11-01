package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type formulaMetric struct {
	formula
	ParseOK       bool    `json:"parse_ok"`
	RenderMS      float64 `json:"render_ms"`
	TextElements  int     `json:"text_elements"`
	ErrorElements int     `json:"error_elements"`
	PixelMAE      float64 `json:"pixel_mae_0_to_1"`
	Error         string  `json:"error,omitempty"`
}

type timing struct {
	Count int     `json:"count"`
	P50   float64 `json:"p50_ms"`
	P95   float64 `json:"p95_ms"`
	Max   float64 `json:"max_ms"`
}

type probeReport struct {
	Platform          string           `json:"platform"`
	Go                string           `json:"go"`
	Ready             any              `json:"page_ready"`
	StartupMS         float64          `json:"startup_ms"`
	Formulas          []formulaMetric  `json:"formulas"`
	Uncached          timing           `json:"uncached"`
	Cached            timing           `json:"cached"`
	CachedBatchSize   int              `json:"cached_batch_size"`
	StabilityCount    int              `json:"stability_count"`
	Stability         timing           `json:"stability"`
	Mermaid           []map[string]any `json:"mermaid"`
	FailureDetection  map[string]bool  `json:"failure_detection"`
	Memory            []memorySample   `json:"memory"`
	WebViewEverPlaced bool             `json:"webview_ever_placed"`
}

func runProbe(output string, a *app, win *mygo.Window, view *mygo.WebView, ready any, startup time.Duration) error {
	page := view.Page()
	eval := func(ctx context.Context, code string) (any, error) {
		bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return page.EvalContext(bounded, code)
	}
	report := probeReport{Platform: runtime.GOOS + "/" + runtime.GOARCH, Go: runtime.Version(),
		Ready: ready, StartupMS: milliseconds(startup), FailureDetection: make(map[string]bool), CachedBatchSize: 10000}
	report.Memory = append(report.Memory, processMemory("ready", page.ProcessID()))
	defer func() {
		if err := saveJSON(filepath.Join(output, "results.json"), report); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	formulas, err := corpus()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(output, "svg"), 0755); err != nil {
		return err
	}
	r := newRenderer(eval)
	var coldTimes, cacheTimes []float64
	var browser, native []image.Image
	for _, f := range formulas {
		start := time.Now()
		result, renderErr := r.render(context.Background(), f.TeX)
		duration := milliseconds(time.Since(start))
		coldTimes = append(coldTimes, duration)
		m := formulaMetric{formula: f, ParseOK: renderErr == nil, RenderMS: duration,
			TextElements: result.TextElements, ErrorElements: result.ErrorElements}
		if renderErr != nil {
			m.Error = renderErr.Error()
			report.Formulas = append(report.Formulas, m)
			return fmt.Errorf("%s: %w", f.ID, renderErr)
		}
		if err := os.WriteFile(filepath.Join(output, "svg", f.ID+".svg"), []byte(result.SVG), 0644); err != nil {
			return err
		}
		// Reference rasterization is deliberately outside the rendering latency.
		v, err := eval(context.Background(), fmt.Sprintf("return await window.s2.raster(%s,%g,%g)", jsString(result.SVG), result.Width, result.Height))
		if err != nil {
			return fmt.Errorf("%s browser reference: %w", f.ID, err)
		}
		ref, err := dataImage(v)
		if err != nil {
			return err
		}
		w, h := int(math.Ceil(result.Width)), int(math.Ceil(result.Height))
		img := ui.Render(func(c *ui.Context) {
			ui.Box(c).Fill().Background(ui.RGB(255, 255, 255)).Children(func() {
				ui.Image(c, result.parsed).Size(float32(result.Width), float32(result.Height)).TextColor(ui.RGB(17, 17, 17))
			})
		}, w, h, 1)
		m.PixelMAE = pixelMAE(ref, img)
		browser, native = append(browser, ref), append(native, img)
		report.Formulas = append(report.Formulas, m)
		if result.TextElements == 0 && result.ErrorElements == 0 {
			for i := 0; i < 10; i++ {
				start := time.Now()
				for range report.CachedBatchSize {
					if _, err := r.render(context.Background(), f.TeX); err != nil {
						return err
					}
				}
				cacheTimes = append(cacheTimes, milliseconds(time.Since(start))/float64(report.CachedBatchSize))
			}
		}
		fmt.Printf("%s parse=%t text=%d merror=%d uncached=%.3fms pixelMAE=%.4f\n", f.ID, m.ParseOK, m.TextElements, m.ErrorElements, duration, m.PixelMAE)
		if f.ID == "F17" {
			win.Update(func() { a.sample = result.parsed })
		}
	}
	report.Uncached, report.Cached = summarize(coldTimes), summarize(cacheTimes)
	report.Memory = append(report.Memory, processMemory("after_formulas", page.ProcessID()))
	for i := 0; i < len(formulas); i += 5 {
		end := min(i+5, len(formulas))
		left, right := formulaSheet(formulas[i:end], browser[i:end], "BROWSER REFERENCE"), formulaSheet(formulas[i:end], native[i:end], "MYGO ParseSVG")
		combined := ui.Render(func(c *ui.Context) {
			ui.Row(c).Fill().Background(ui.RGB(255, 255, 255)).Gap(20).Children(func() {
				ui.Image(c, ui.NewBitmap(left)).Size(540, 620)
				ui.Image(c, ui.NewBitmap(right)).Size(540, 620)
			})
		}, 1100, 620, 1)
		if err := savePNG(filepath.Join(output, fmt.Sprintf("comparison-%02d.png", i/5+1)), combined); err != nil {
			return err
		}
		if i == 45 {
			done := make(chan struct{})
			win.Update(func() {
				a.left, a.right = ui.NewBitmap(left), ui.NewBitmap(right)
				a.caption = "F46–F50 · browser reference vs Native · CJK compatibility"
				close(done)
			})
			<-done
		}
	}
	// Every call awaits MathJax, returns a sequence number, and validates a
	// nonempty SVG. No cache participates in this 1000-call study.
	var stabilityTimes []float64
	for i := 0; i < 1000; i++ {
		f := formulas[i%len(formulas)]
		start := time.Now()
		v, err := eval(context.Background(), fmt.Sprintf("return await window.s2.probe(%d,%s)", i, jsString(f.TeX)))
		if err != nil {
			return fmt.Errorf("stability Eval %d: %w", i, err)
		}
		var result struct{ Index, Bytes, Errors int }
		if err := decode(v, &result); err != nil {
			return err
		}
		if result.Index != i || result.Bytes == 0 || result.Errors != 0 {
			return fmt.Errorf("stability Eval %d: invalid response %+v", i, result)
		}
		report.StabilityCount++
		stabilityTimes = append(stabilityTimes, milliseconds(time.Since(start)))
	}
	report.Stability = summarize(stabilityTimes)
	report.Memory = append(report.Memory, processMemory("after_1000_eval", page.ProcessID()))
	// CapturePage captures the last painted frame. Take the window evidence
	// after the queued UI update and the 1000-call run, not in that update.
	windowPNG, err := win.CapturePage()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(output, "native-window.png"), windowPNG, 0644); err != nil {
		return err
	}
	diagrams := []struct{ name, source string }{
		{"flowchart", "flowchart LR\nA[源码] --> B{缓存}\nB -->|命中| C[Native]\nB -->|未命中| D[WebView]\nD --> C"},
		{"sequence", "sequenceDiagram\nparticipant Go\nparticipant WebView\nGo->>WebView: Eval\nWebView-->>Go: SVG / PNG"},
		{"class", "classDiagram\nclass Renderer {\n+Render()\n}\nclass Page {\n+Eval()\n}\nRenderer --> Page"},
		{"state", "stateDiagram-v2\n[*] --> Idle\nIdle --> Loading\nLoading --> Ready\nLoading --> Unavailable\nReady --> Idle"},
		{"er", "erDiagram\nDOCUMENT ||--o{ BLOCK : contains\nBLOCK ||--o| RENDER : has"},
		{"pie", "pie title Cache\n\"Hit\" : 80\n\"Miss\" : 20"},
	}
	for _, d := range diagrams {
		start := time.Now()
		v, err := eval(context.Background(), "return await window.s2.diagram("+jsString(d.source)+")")
		if err != nil {
			return fmt.Errorf("Mermaid %s: %w", d.name, err)
		}
		var result struct {
			PNG           string `json:"png"`
			Width, Height int
		}
		if err := decode(v, &result); err != nil {
			return err
		}
		img, err := dataImage(result.PNG)
		if err != nil {
			return err
		}
		if err := savePNG(filepath.Join(output, "mermaid-"+d.name+".png"), img); err != nil {
			return err
		}
		report.Mermaid = append(report.Mermaid, map[string]any{"kind": d.name, "width": result.Width, "height": result.Height, "ms": milliseconds(time.Since(start))})
	}
	_, jsErr := eval(context.Background(), "throw new Error('S2 injected failure')")
	report.FailureDetection["javascript_exception"] = jsErr != nil
	_, promiseErr := eval(context.Background(), "return await Promise.reject(new Error('S2 rejection'))")
	report.FailureDetection["rejected_promise"] = promiseErr != nil
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, cancelErr := eval(canceled, "return 42")
	report.FailureDetection["canceled_context"] = cancelErr != nil
	_, timeoutErr := eval(context.Background(), "return await new Promise(() => {})")
	report.FailureDetection["nonresolving_promise_timeout"] = timeoutErr != nil
	v, err := eval(context.Background(), "return 42")
	report.FailureDetection["responsive_after_timeout"] = err == nil && v == float64(42)
	report.Memory = append(report.Memory, processMemory("final", page.ProcessID()))
	if err := saveJSON(filepath.Join(output, "results.json"), report); err != nil {
		return err
	}
	fmt.Printf("startup=%.3fms uncached=%+v cached=%+v stability=%d/1000\n", report.StartupMS, report.Uncached, report.Cached, report.StabilityCount)
	return nil
}

func formulaSheet(formulas []formula, images []image.Image, heading string) *image.RGBA {
	bitmaps := make([]*ui.Bitmap, len(images))
	for i, img := range images {
		bitmaps[i] = ui.NewBitmap(img)
	}
	return ui.Render(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(12).Gap(6).Background(ui.RGB(255, 255, 255)).TextColor(ui.RGB(17, 17, 17)).Children(func() {
			ui.Text(c, heading).Bold().FontSize(16)
			for i, f := range formulas {
				ui.Column(c).Height(108).Gap(4).Children(func() {
					ui.Text(c, f.ID+" · "+f.TeX).FontSize(11)
					w, h := images[i].Bounds().Dx(), images[i].Bounds().Dy()
					scale := min(1.0, 500/float64(w), 78/float64(h))
					ui.Image(c, bitmaps[i]).Size(float32(float64(w)*scale), float32(float64(h)*scale))
				})
			}
		})
	}, 540, 620, 1)
}

func milliseconds(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func summarize(values []float64) timing {
	if len(values) == 0 {
		return timing{}
	}
	v := slices.Clone(values)
	slices.Sort(v)
	return timing{len(v), v[int(math.Ceil(float64(len(v))*0.5))-1], v[int(math.Ceil(float64(len(v))*0.95))-1], v[len(v)-1]}
}

func saveJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
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
	encodeErr := png.Encode(f, img)
	closeErr := f.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}

func dataImage(v any) (image.Image, error) {
	s, ok := v.(string)
	if !ok || !strings.HasPrefix(s, "data:image/png;base64,") {
		return nil, fmt.Errorf("invalid PNG data URI: %T", v)
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(s, "data:image/png;base64,"))
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(b))
}

// A diagnostic only; antialiasing differences are not visual failures.
func pixelMAE(a, b image.Image) float64 {
	if a.Bounds() != b.Bounds() {
		return 1
	}
	var sum uint64
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			ar, ag, ab, _ := a.At(x, y).RGBA()
			br, bg, bb, _ := b.At(x, y).RGBA()
			for _, pair := range [][2]uint32{{ar, br}, {ag, bg}, {ab, bb}} {
				sum += uint64(math.Abs(float64(pair[0]) - float64(pair[1])))
			}
		}
	}
	return float64(sum) / float64(a.Bounds().Dx()*a.Bounds().Dy()*3*65535)
}
