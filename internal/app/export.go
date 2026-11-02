package app

import (
	"bytes"
	"context"
	"strings"

	"github.com/egoist/mygo/ui/editor"

	"aurink/internal/contract"
	"aurink/internal/core/export"
	"aurink/internal/core/markdown"
	"aurink/internal/core/text"
)

// ExportHTML writes a document as an HTML page off the main thread (INT-2.1)
// and answers on it.
func ExportHTML(files contract.FileStore, d contract.Dispatcher, exts contract.Extensions) func(string, string, contract.ExportOptions, func(error)) {
	ex := export.New()
	return func(md, path string, opts contract.ExportOptions, done func(error)) {
		go func() {
			err := writeHTML(ex, files, exts, md, path, opts)
			d.Post(func() { done(err) })
		}()
	}
}

func writeHTML(ex *export.Exporter, files contract.FileStore, exts contract.Extensions, md, path string, opts contract.ExportOptions) error {
	p, err := markdown.New(md, 1, exts)
	if err != nil {
		return err
	}
	opts.Format = contract.ExportHTML
	opts.Highlight = highlight
	var out bytes.Buffer
	if err := ex.Export(context.Background(), text.New(md, 1), p.Snapshot(), opts, &out); err != nil {
		return err
	}
	_, err = files.Write(path, out.String(), contract.DocMeta{Dominant: "\n", Encoding: "utf-8"}, nil)
	return err
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
