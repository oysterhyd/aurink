// Package pdf coordinates HTML export and printing away from the UI thread.
package pdf

import (
	"bytes"
	"context"
	"errors"
	"math"
	"net/url"
	"path/filepath"
	"strings"

	"aurink/internal/contract"
)

// Export produces a light, self-contained PDF. Exporter and printer must be
// called off the UI thread and must honor ctx.
func Export(ctx context.Context, text contract.Text, ast contract.AST, opts contract.ExportOptions, exporter contract.Exporter, printer contract.Printer) ([]byte, error) {
	const op = "pdf.Export"
	if ctx == nil || text == nil || ast == nil || exporter == nil || printer == nil {
		return nil, contract.E(contract.CodeInvalid, op, nil)
	}
	if err := ctx.Err(); err != nil {
		return nil, contract.E(contract.CodeCanceled, op, err)
	}
	pageSize := opts.PageSize
	if pageSize == "" {
		pageSize = "A4"
	}
	if pageSize != "A4" && pageSize != "Letter" {
		return nil, contract.E(contract.CodeInvalid, op, errors.New("unsupported page size"))
	}
	margins := opts.Margins
	for _, margin := range margins {
		if margin < 0 || math.IsNaN(float64(margin)) || math.IsInf(float64(margin), 0) {
			return nil, contract.E(contract.CodeInvalid, op, errors.New("invalid margin"))
		}
	}
	if margins == [4]float32{} {
		margins = [4]float32{.6, .6, .6, .6}
	}
	base, err := baseURL(opts.BaseDir)
	if err != nil {
		return nil, contract.E(contract.CodeInvalid, op, err)
	}

	last := float32(-1)
	report := func(f float32) {
		if ctx.Err() == nil && f > last && opts.OnProgress != nil {
			last = f
			opts.OnProgress(f)
		}
	}
	report(0)
	if err := ctx.Err(); err != nil {
		return nil, contract.E(contract.CodeCanceled, op, err)
	}
	var exportFraction, renderFraction float32
	var langs []string
	if configured, ok := exporter.(interface{ ObjectLanguages() []string }); ok {
		langs = configured.ObjectLanguages()
	}
	total, err := objects(ctx, text, ast, langs)
	if err != nil {
		return nil, contract.E(contract.CodeCanceled, op, err)
	}
	if total == 0 || opts.Renderer == nil || !opts.Renderer.Available() {
		renderFraction = 1
	}
	update := func() { report(.2*exportFraction + .5*renderFraction) }
	exportOpts := opts
	exportOpts.Format, exportOpts.Fragment, exportOpts.EmbedImages, exportOpts.Dark = contract.ExportHTML, false, true, false
	exportOpts.OnProgress = func(f float32) {
		if f > exportFraction {
			exportFraction = min(f, 1)
			update()
		}
	}
	if opts.Renderer != nil {
		exportOpts.Renderer = &progressRenderer{Renderer: opts.Renderer, total: total, seen: make(map[contract.RenderRequest]bool), done: func(n int) {
			if total > 0 {
				renderFraction = min(float32(n)/float32(total), 1)
				update()
			}
		}}
	}
	var html bytes.Buffer
	err = exporter.Export(ctx, text, ast, exportOpts, &html)
	if ctx.Err() != nil {
		return nil, contract.E(contract.CodeCanceled, op, ctx.Err())
	}
	if err != nil {
		return nil, serviceError(op, err)
	}
	report(.7)
	if ctx.Err() != nil {
		return nil, contract.E(contract.CodeCanceled, op, ctx.Err())
	}
	data, err := printer.HTMLToPDF(ctx, html.String(), base, contract.PDFOptions{PageSize: pageSize, Margins: margins, Background: true})
	if ctx.Err() != nil {
		return nil, contract.E(contract.CodeCanceled, op, ctx.Err())
	}
	if err != nil {
		return nil, serviceError(op, err)
	}
	report(1)
	if ctx.Err() != nil {
		return nil, contract.E(contract.CodeCanceled, op, ctx.Err())
	}
	return data, nil
}

func serviceError(op string, err error) error {
	var e *contract.Error
	if errors.As(err, &e) {
		return contract.E(e.Code, op, err)
	}
	return contract.E(contract.CodeIO, op, err)
}

func baseURL(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	path := filepath.ToSlash(abs)
	u := &url.URL{Scheme: "file"}
	if strings.HasPrefix(path, "//") {
		u.Host, path, _ = strings.Cut(strings.TrimPrefix(path, "//"), "/")
		path = "/" + path
	} else if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	u.Path = strings.TrimSuffix(path, "/") + "/"
	return u.String(), nil
}

type progressRenderer struct {
	contract.Renderer
	total int
	seen  map[contract.RenderRequest]bool
	done  func(int)
}

func (p *progressRenderer) Render(ctx context.Context, req contract.RenderRequest) (contract.RenderResult, error) {
	result, err := p.Renderer.Render(ctx, req)
	if ctx.Err() == nil && !p.seen[req] {
		p.seen[req] = true
		p.done(len(p.seen))
	}
	return result, err
}

func objects(ctx context.Context, t contract.Text, ast contract.AST, langs []string) (int, error) {
	type key struct {
		kind         contract.RenderKind
		source, lang string
	}
	seen := map[key]bool{}
	for i := 1; i < ast.Len(); i++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		b := ast.Block(i)
		diagram := strings.EqualFold(b.Lang, "mermaid")
		for _, lang := range langs {
			if lang != "" && strings.EqualFold(b.Lang, lang) {
				diagram = true
			}
		}
		if b.Kind == contract.BlockMath || b.Kind == contract.BlockCode && diagram {
			var source strings.Builder
			for _, line := range b.Lines {
				source.WriteString(strings.Repeat(" ", int(line.Padding)))
				source.WriteString(t.Slice(line.Source))
				source.WriteByte('\n')
			}
			kind := contract.RenderMathBlock
			if b.Kind == contract.BlockCode {
				kind = contract.RenderDiagram
			}
			seen[key{kind, strings.TrimSuffix(source.String(), "\n"), b.Lang}] = true
		}
		for _, n := range ast.Inlines(i) {
			if n.Kind == contract.InlineMath {
				seen[key{contract.RenderMathInline, n.Text, ""}] = true
			}
		}
	}
	return len(seen), nil
}
