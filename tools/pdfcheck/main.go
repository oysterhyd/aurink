// pdfcheck prints the PDF fixture with the real renderer and MyGo printer.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"aurink/internal/adapters/printer"
	"aurink/internal/adapters/webviewhost"
	"aurink/internal/contract"
	"aurink/internal/core/export"
	"aurink/internal/core/markdown"
	"aurink/internal/core/pdf"
	"aurink/internal/core/render"
	"aurink/internal/core/text"
	renderassets "aurink/resources/render"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type measuredExporter struct {
	contract.Exporter
	duration time.Duration
	html     bytes.Buffer
}

func (e *measuredExporter) Export(ctx context.Context, t contract.Text, a contract.AST, o contract.ExportOptions, w io.Writer) error {
	start := time.Now()
	err := e.Exporter.Export(ctx, t, a, o, io.MultiWriter(w, &e.html))
	e.duration = time.Since(start)
	return err
}

type measuredPrinter struct {
	contract.Printer
	duration time.Duration
}

func (p *measuredPrinter) HTMLToPDF(ctx context.Context, html, base string, o contract.PDFOptions) ([]byte, error) {
	start := time.Now()
	data, err := p.Printer.HTMLToPDF(ctx, html, base, o)
	p.duration = time.Since(start)
	return data, err
}

type checkedRenderer struct {
	contract.Renderer
	calls, failures int
}

func (r *checkedRenderer) Render(ctx context.Context, req contract.RenderRequest) (contract.RenderResult, error) {
	r.calls++
	result, err := r.Renderer.Render(ctx, req)
	if err != nil {
		r.failures++
		fmt.Fprintf(os.Stderr, "render %v %q: %v\n", req.Kind, req.Source, err)
	}
	return result, err
}

type measurement struct {
	Size           string `json:"size"`
	Pages          int    `json:"pages"`
	Bytes          int    `json:"bytes"`
	ExportMS       int64  `json:"export_ms"`
	PrintMS        int64  `json:"print_ms"`
	RenderCalls    int    `json:"render_calls"`
	RenderFailures int    `json:"render_failures"`
	TextVerified   bool   `json:"text_verified"`
}

func main() {
	done := make(chan error, 1)
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{Hidden: true, Content: ui.View(func(*ui.Context) {})})
		go func() {
			err := run(win)
			win.Destroy()
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

func run(win *mygo.Window) error {
	const fixture = "testdata/pdf/sample.md"
	const out = "reports/pdf"
	if err := os.MkdirAll(out, 0755); err != nil {
		return err
	}
	src, err := os.ReadFile(fixture)
	if err != nil {
		return err
	}
	parsed, err := markdown.New(string(src), 1, contract.ExtDefault)
	if err != nil {
		return err
	}
	r := &checkedRenderer{Renderer: render.New(webviewhost.New(win), render.Options{Page: renderassets.Page()})}
	if !r.Available() {
		return fmt.Errorf("WebView renderer unavailable")
	}
	pr := printer.New()
	var results []measurement
	for _, size := range []string{"A4", "Letter"} {
		e := &measuredExporter{Exporter: export.New()}
		p := &measuredPrinter{Printer: pr}
		beforeCalls, beforeFailures := r.calls, r.failures
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		data, exportErr := pdf.Export(ctx, text.New(string(src), 1), parsed.Snapshot(), contract.ExportOptions{PageSize: size, BaseDir: filepath.Dir(fixture), Renderer: r}, e, p)
		cancel()
		if exportErr != nil {
			return exportErr
		}
		if r.failures != beforeFailures {
			return fmt.Errorf("%s: renderer failed; see stderr", size)
		}
		markup := e.html.String()
		if strings.Count(markup, `<figure class="diagram">`) != 2 || strings.Count(markup, `<span class="math"><`) != 2 || strings.Count(markup, `<div class="math"><`) != 2 {
			return fmt.Errorf("%s: an object fell back to source in the exported HTML", size)
		}
		path := filepath.Join(out, "sample-"+size+".pdf")
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
		info, err := exec.Command("pdfinfo", path).CombinedOutput()
		if err != nil {
			return fmt.Errorf("pdfinfo: %w: %s", err, info)
		}
		match := regexp.MustCompile(`(?m)^Pages:\s+(\d+)`).FindSubmatch(info)
		if len(match) != 2 {
			return fmt.Errorf("pdfinfo: page count missing")
		}
		pages, _ := strconv.Atoi(string(match[1]))
		if pages < 4 {
			return fmt.Errorf("%s has only %d pages", size, pages)
		}
		prefix := filepath.Join(out, "sample-"+size)
		oldPages, err := filepath.Glob(prefix + "-*.png")
		if err != nil {
			return err
		}
		pageName := regexp.MustCompile(`^sample-(A4|Letter)-[0-9]+\.png$`)
		for _, old := range oldPages {
			if pageName.MatchString(filepath.Base(old)) {
				if err := os.Remove(old); err != nil {
					return err
				}
			}
		}
		if output, err := exec.Command("pdftoppm.exe", "-png", "-r", "110", path, prefix).CombinedOutput(); err != nil {
			return fmt.Errorf("pdftoppm: %w: %s", err, output)
		}
		textPath := prefix + ".txt"
		if output, err := exec.Command("pdftotext", "-layout", "-enc", "UTF-8", path, textPath).CombinedOutput(); err != nil {
			return fmt.Errorf("pdftotext: %w: %s", err, output)
		}
		layer, err := os.ReadFile(textPath)
		if err != nil {
			return err
		}
		// pdftotext adds line breaks where the browser wrapped a paragraph.
		flat := strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return r
		}, string(layer))
		for _, sentence := range []string{"这是一段用于验证中文字体和文本提取的句子。", "排版应当清晰，标点正确，字形不应显示为方框。"} {
			if !strings.Contains(flat, sentence) {
				return fmt.Errorf("%s: text layer missing %q", size, sentence)
			}
		}
		results = append(results, measurement{Size: size, Pages: pages, Bytes: len(data), ExportMS: e.duration.Milliseconds(), PrintMS: p.duration.Milliseconds(), RenderCalls: r.calls - beforeCalls, RenderFailures: r.failures - beforeFailures, TextVerified: true})
		fmt.Printf("%s: %d pages, %d bytes, export %v, print %v, renders %d\n", size, pages, len(data), e.duration, p.duration, r.calls-beforeCalls)
	}
	encoded, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "measurements.json"), append(encoded, '\n'), 0644)
}
