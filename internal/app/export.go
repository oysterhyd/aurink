package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui/editor"

	"aurink/internal/contract"
	"aurink/internal/core/export"
	"aurink/internal/core/markdown"
	"aurink/internal/core/pdf"
	"aurink/internal/core/text"
)

// ExportHTML writes a document as an HTML page off the main thread (INT-2.1)
// and answers on it, with math and diagrams as source.
func ExportHTML(files contract.FileStore, d contract.Dispatcher, exts contract.Extensions, stores ...contract.SettingsStore) func(string, string, contract.ExportOptions, func(error)) {
	x := &Exports{Files: files, D: d, Exts: exts}
	if len(stores) > 0 {
		x.Settings = stores[0]
	}
	return x.HTML
}

// Exports writes documents out (INT-3.1): HTML pages and PDFs off the main
// thread, answering on it. Its methods are called on the main thread.
type Exports struct {
	Files    contract.FileStore
	D        contract.Dispatcher
	Exts     contract.Extensions
	Settings contract.SettingsStore // nil: soft breaks as line breaks
	Renderer contract.Renderer      // nil: math and diagrams as source
	Printer  contract.Printer       // nil: PDF export fails as unavailable
	// ObjectLangs are the fenced languages drawn as diagrams besides Mermaid.
	ObjectLangs []string

	once sync.Once
	ex   *export.Exporter
}

func (x *Exports) exporter() *export.Exporter {
	x.once.Do(func() { x.ex = &export.Exporter{ObjectLangs: x.ObjectLangs} })
	return x.ex
}

// options completes what the shell gives with what the app knows: the soft
// breaks the editor shows, the renderer, the code colours. SettingsStore.Get
// belongs to the main thread, so this runs there, before the worker starts.
func (x *Exports) options(opts contract.ExportOptions) contract.ExportOptions {
	if x.Settings != nil {
		opts.SoftBreak = x.Settings.Get().Editor.SoftBreak
	}
	if x.Renderer != nil {
		opts.Renderer = x.Renderer
	}
	opts.Highlight = highlight
	return opts
}

// HTML writes markdown as an HTML page at path.
func (x *Exports) HTML(md, path string, opts contract.ExportOptions, done func(error)) {
	opts = x.options(opts)
	go func() {
		err := x.writeHTML(md, path, opts)
		x.D.Post(func() { done(err) })
	}()
}

func (x *Exports) writeHTML(md, path string, opts contract.ExportOptions) error {
	page, err := x.page(md, opts)
	if err != nil {
		return err
	}
	_, err = x.Files.Write(path, page, contract.DocMeta{Dominant: "\n", Encoding: "utf-8"}, nil)
	return err
}

// page is markdown as a whole HTML page.
func (x *Exports) page(md string, opts contract.ExportOptions) (string, error) {
	p, err := markdown.New(md, 1, x.Exts)
	if err != nil {
		return "", err
	}
	opts.Format = contract.ExportHTML
	var out bytes.Buffer
	if err := x.exporter().Export(context.Background(), text.New(md, 1), p.Snapshot(), opts, &out); err != nil {
		return "", err
	}
	return out.String(), nil
}

// PDF prints markdown as a PDF at path. opts.OnProgress is called on the main
// thread, as done is.
func (x *Exports) PDF(md, path string, opts contract.ExportOptions, done func(error)) {
	opts = x.options(opts)
	if progress := opts.OnProgress; progress != nil {
		opts.OnProgress = func(f float32) { x.D.Post(func() { progress(f) }) }
	}
	go func() {
		err := x.writePDF(md, path, opts)
		x.D.Post(func() { done(err) })
	}()
}

func (x *Exports) writePDF(md, path string, opts contract.ExportOptions) error {
	const op = "app.ExportPDF"
	if x.Printer == nil {
		return contract.E(contract.CodeUnavailable, op, nil)
	}
	p, err := markdown.New(md, 1, x.Exts)
	if err != nil {
		return err
	}
	data, err := pdf.Export(context.Background(), text.New(md, 1), p.Snapshot(), opts, x.exporter(), x.Printer)
	if err != nil {
		return err
	}
	return writeFileAtomic(path, data)
}

// PrintPage is markdown as the light, self-contained page the print preview
// shows. It runs off the main thread; done comes on it.
func (x *Exports) PrintPage(md string, opts contract.ExportOptions, done func(page string, err error)) {
	opts = x.options(opts)
	opts.Dark, opts.EmbedImages, opts.Fragment = false, true, false
	go func() {
		page, err := x.page(md, opts)
		x.D.Post(func() { done(page, err) })
	}()
}

// writeFileAtomic writes data beside path and renames it over path, so a
// failed export leaves no half a file.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// highlight marks a code block's keywords, functions, comments and strings
// with MyGo's tokenizers, the colors the live view gives them (D17).
func highlight(lang, code string) []contract.CodeToken {
	l := editor.LanguageNamed(lang)
	if l == nil || l.Tokenizer == nil {
		return nil
	}
	var out []contract.CodeToken
	var tokens []editor.Token
	var state uint64
	at := 0
	for line := range strings.SplitAfterSeq(code, "\n") {
		body := strings.TrimSuffix(line, "\n")
		tokens, state = l.Tokenizer.Tokenize(body, state, tokens[:0])
		from := 0
		for _, tok := range tokens {
			end := min(tok.End, len(body))
			if end <= from {
				continue
			}
			if kind := tokenClass(tok.Kind); kind != "" {
				out = append(out, contract.CodeToken{Range: contract.Range{Start: at + from, End: at + end}, Kind: kind})
			}
			from = end
		}
		at += len(line)
	}
	return out
}

func tokenClass(k editor.TokenKind) string {
	switch k {
	case editor.TokenKeyword:
		return "keyword"
	case editor.TokenFunction:
		return "function"
	case editor.TokenComment:
		return "comment"
	case editor.TokenString:
		return "string"
	}
	return ""
}
