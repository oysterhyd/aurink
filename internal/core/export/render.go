package export

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"io"
	"math"
	"strconv"
	"strings"

	"aurink/internal/contract"
)

type rendered struct {
	result contract.RenderResult
	ok     bool
}

func (r *renderer) diagramLang(lang string) bool {
	if strings.EqualFold(lang, "mermaid") {
		return true
	}
	for _, object := range r.objectLangs {
		if object != "" && strings.EqualFold(object, lang) {
			return true
		}
	}
	return false
}

func (r *renderer) renderObject(kind contract.RenderKind, source, lang string) (contract.RenderResult, bool) {
	if r.err != nil || r.opts.Renderer == nil || !r.opts.Renderer.Available() {
		return contract.RenderResult{}, false
	}
	color := "#15161c"
	if r.opts.Dark {
		color = "#ecedf2"
	}
	req := contract.RenderRequest{Kind: kind, Source: source, Lang: lang, Dark: r.opts.Dark, FontSize: 16, Color: color, Scale: 2}
	if cached, ok := r.rendered[req]; ok {
		return cached.result, cached.ok
	}
	result, err := r.opts.Renderer.Render(r.ctx, req)
	if r.ctx.Err() != nil {
		r.err = r.ctx.Err()
		return contract.RenderResult{}, false
	}
	ok := err == nil && validResult(kind, result)
	r.rendered[req] = rendered{result: result, ok: ok}
	return result, ok
}

func validResult(kind contract.RenderKind, result contract.RenderResult) bool {
	finitePositive := func(v float32) bool { return v > 0 && !math.IsInf(float64(v), 0) && !math.IsNaN(float64(v)) }
	if !finitePositive(result.Width) || !finitePositive(result.Height) || (len(result.SVG) == 0) == (len(result.PNG) == 0) {
		return false
	}
	if kind == contract.RenderMathInline && (!finitePositive(result.Ascent) || result.Ascent > result.Height) {
		return false
	}
	if kind == contract.RenderDiagram && len(result.PNG) == 0 {
		return false
	}
	return len(result.SVG) == 0 || safeSVG(result.SVG)
}

func number(f float32) string { return strconv.FormatFloat(float64(f), 'f', -1, 32) }

func (r *renderer) renderImage(result contract.RenderResult, inline bool) {
	style := "width:" + number(result.Width) + "px;height:" + number(result.Height) + "px;"
	if inline {
		style += "vertical-align:-" + number(result.Height-result.Ascent) + "px;"
	}
	if len(result.SVG) != 0 {
		// A wrapper sets the dimensions and baseline even if SVG has its own style.
		r.write("<span style=\"display:inline-block;" + style + "\">")
		r.write(string(result.SVG))
		r.write("</span>")
		return
	}
	r.write("<img alt=\"\" style=\"" + style + "\" src=\"data:image/png;base64,")
	r.write(base64.StdEncoding.EncodeToString(result.PNG))
	r.write("\" />")
}

// SVG is kept verbatim only when it is a static SVG with no executable content
// or external references. XML parsing also rejects malformed markup/entities.
func safeSVG(data []byte) bool {
	allowed := map[string]bool{"svg": true, "g": true, "path": true, "defs": true, "use": true,
		"rect": true, "line": true, "polyline": true, "polygon": true, "circle": true,
		"ellipse": true, "clippath": true, "mask": true, "title": true, "desc": true}
	attrs := map[string]bool{"xmlns": true, "viewbox": true, "preserveaspectratio": true, "width": true, "height": true,
		"x": true, "y": true, "x1": true, "y1": true, "x2": true, "y2": true, "cx": true, "cy": true,
		"r": true, "rx": true, "ry": true, "points": true, "d": true, "transform": true, "id": true, "class": true,
		"role": true, "focusable": true, "href": true, "style": true, "color": true, "fill": true, "fill-rule": true,
		"stroke": true, "stroke-width": true, "stroke-linecap": true, "stroke-linejoin": true, "stroke-dasharray": true,
		"opacity": true, "fill-opacity": true, "stroke-opacity": true, "vector-effect": true}
	d := xml.NewDecoder(bytes.NewReader(data))
	depth, roots := 0, 0
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return depth == 0 && roots == 1
		}
		if err != nil {
			return false
		}
		switch v := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(v.Name.Local)
			if !allowed[name] || (v.Name.Space != "" && v.Name.Space != "http://www.w3.org/2000/svg") {
				return false
			}
			if depth == 0 {
				roots++
				if name != "svg" || roots != 1 {
					return false
				}
			}
			depth++
			for _, a := range v.Attr {
				name := strings.ToLower(a.Name.Local)
				val := strings.ToLower(strings.TrimSpace(a.Value))
				if a.Name.Space == "xmlns" {
					if val != "http://www.w3.org/1999/xlink" && val != "http://www.w3.org/2000/svg" {
						return false
					}
					continue
				}
				if (!attrs[name] && !strings.HasPrefix(name, "aria-") && !strings.HasPrefix(name, "data-")) || strings.HasPrefix(name, "on") || strings.Contains(val, "javascript:") || strings.Contains(val, "expression(") || strings.Contains(val, "url(") || strings.ContainsAny(val, "\\@") {
					return false
				}
				if name == "xmlns" && val != "http://www.w3.org/2000/svg" {
					return false
				}
				if name == "style" && !safeSVGStyle(val) {
					return false
				}
				if name == "href" && (!strings.HasPrefix(val, "#") || len(val) == 1) {
					return false
				}
			}
		case xml.EndElement:
			depth--
		case xml.Directive, xml.ProcInst:
			return false
		case xml.CharData:
			if depth == 0 && len(bytes.TrimSpace(v)) != 0 {
				return false
			}
		}
	}
}

func safeSVGStyle(style string) bool {
	allowed := map[string]bool{"color": true, "max-width": true, "vertical-align": true,
		"stroke-width": true, "stroke-dasharray": true, "stroke-linecap": true, "stroke-linejoin": true}
	for _, declaration := range strings.Split(style, ";") {
		if strings.TrimSpace(declaration) == "" {
			continue
		}
		name, value, ok := strings.Cut(declaration, ":")
		if !ok || !allowed[strings.TrimSpace(name)] || strings.TrimSpace(value) == "" {
			return false
		}
	}
	return true
}
