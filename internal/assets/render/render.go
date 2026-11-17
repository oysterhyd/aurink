// Package render provides the self-contained offline rendering page (09 D14).
package render

import (
	"embed"
	"encoding/base64"
	"strings"
	"sync"
)

//go:embed page.html vendor/*
var assets embed.FS

var pageOnce sync.Once
var page string

// Page returns the HTML with its pinned libraries embedded as data URLs.
func Page() string {
	pageOnce.Do(func() {
		data, _ := assets.ReadFile("page.html")
		page = string(data)
		for _, bundle := range []struct{ token, path string }{
			{"{{MATHJAX}}", "vendor/mathjax-3.2.2-tex-svg.js"},
			{"{{MERMAID}}", "vendor/mermaid-10.9.3.min.js"},
		} {
			data, _ := assets.ReadFile(bundle.path)
			page = strings.ReplaceAll(page, bundle.token, base64.StdEncoding.EncodeToString(data))
		}
	})
	return page
}
