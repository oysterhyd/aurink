package app

import (
	"bytes"
	"context"

	"aurink/internal/contract"
	"aurink/internal/core/export"
	"aurink/internal/core/markdown"
	"aurink/internal/core/text"
)

// clipHTML turns copied Markdown into the HTML fragment a copy puts beside it
// (FE-1.5), as the HTML export writes it; "" when it cannot.
func clipHTML(exts contract.Extensions) func(string) string {
	ex := export.New()
	return func(md string) string {
		p, err := markdown.New(md, 1, exts)
		if err != nil {
			return ""
		}
		var out bytes.Buffer
		if err := ex.Export(context.Background(), text.New(md, 1), p.Snapshot(),
			contract.ExportOptions{Format: contract.ExportHTML, Fragment: true}, &out); err != nil {
			return ""
		}
		return out.String()
	}
}
