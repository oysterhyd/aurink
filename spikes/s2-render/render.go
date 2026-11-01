package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

//go:embed assets/page.html assets/formulas.json assets/vendor/*
var assets embed.FS

type formula struct {
	ID    string `json:"id"`
	Group string `json:"group"`
	TeX   string `json:"tex"`
}

type mathResult struct {
	SVG           string  `json:"svg"`
	Width         float64 `json:"width"`
	Height        float64 `json:"height"`
	TextElements  int     `json:"textElements"`
	ErrorElements int     `json:"errorElements"`
	parsed        *ui.SVG
}

type evalFunc func(context.Context, string) (any, error)

// renderer is deliberately private to S2. All calls are serialized by its
// caller. Render blocks and must not run on the UI thread.
type renderer struct {
	eval  evalFunc
	cache map[[32]byte]mathResult
}

var errUnavailable = errors.New("render: WebView unavailable; display source")

func newRenderer(eval evalFunc) *renderer {
	return &renderer{eval: eval, cache: make(map[[32]byte]mathResult)}
}

func (r *renderer) render(ctx context.Context, source string) (mathResult, error) {
	if err := ctx.Err(); err != nil {
		return mathResult{}, err
	}
	key := sha256.Sum256([]byte(source))
	if result, ok := r.cache[key]; ok {
		return result, nil
	}
	if r.eval == nil {
		return mathResult{}, errUnavailable
	}
	v, err := r.eval(ctx, "return await window.s2.math("+jsString(source)+")")
	if err != nil {
		return mathResult{}, err
	}
	var result mathResult
	if err := decode(v, &result); err != nil {
		return result, err
	}
	if result.Width <= 0 || result.Height <= 0 || result.SVG == "" {
		return result, errors.New("render: empty SVG or dimensions")
	}
	result.parsed, err = ui.ParseSVG([]byte(result.SVG))
	if err != nil {
		return result, fmt.Errorf("ParseSVG: %w", err)
	}
	// Preserve raw results for the compatibility study, but never cache a
	// known-incomplete rendering as a successful service result.
	if result.TextElements == 0 && result.ErrorElements == 0 {
		r.cache[key] = result
	}
	return result, nil
}

func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func decode(v any, out any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func localHTML() (string, error) {
	page, err := assets.ReadFile("assets/page.html")
	if err != nil {
		return "", err
	}
	for _, bundle := range []struct{ token, file string }{
		{"{{MATHJAX}}", "mathjax-3.2.2-tex-svg.js"},
		{"{{MERMAID}}", "mermaid-10.9.3.min.js"},
	} {
		data, err := assets.ReadFile("assets/vendor/" + bundle.file)
		if err != nil {
			return "", err
		}
		page = []byte(strings.ReplaceAll(string(page), bundle.token, base64.StdEncoding.EncodeToString(data)))
	}
	return string(page), nil
}

func corpus() ([]formula, error) {
	b, err := assets.ReadFile("assets/formulas.json")
	if err != nil {
		return nil, err
	}
	var formulas []formula
	err = json.Unmarshal(b, &formulas)
	return formulas, err
}
