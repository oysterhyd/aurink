package export

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"aurink/internal/contract"
)

// The owner's acceptance probe of BE-2.8: common script vectors, in a block and inline,
// leave no tag, attribute or URL that runs.
func TestExportIsSafe(t *testing.T) {
	vectors := []string{
		`<a href="jav&#x09;ascript:alert(1)">x</a>`,
		`<a href="&#106;avascript:alert(1)">x</a>`,
		`<a href="JaVaScRiPt:alert(1)">x</a>`,
		`<a href="%6Aavascript:alert(1)">x</a>`,
		`[x](<javascript:alert(1)>)`,
		`[x](data:text/html,<script>alert(1)</script>)`,
		`![x](javascript:alert(1))`,
		`<img src="x" OnError="alert(1)">`,
		`<img src=x onerror=alert(1)//>`,
		`<svg onload=alert(1)>`,
		`<math><mi xlink:href="javascript:alert(1)">x</mi></math>`,
		`<form action="javascript:alert(1)"><button>x</button></form>`,
		`<base href="javascript:alert(1)//">`,
		`<meta http-equiv="refresh" content="0;url=javascript:alert(1)">`,
		`<div style="background:url(javascript:alert(1))">x</div>`,
		`<scr<script>ipt>alert(1)</script>`,
		`<SCRIPT>alert(1)</SCRIPT>`,
		`<script src=//evil></script>`,
		`<iframe srcdoc="<script>alert(1)</script>"></iframe>`,
		"<a href=\"x\" title='a\" onmouseover=\"alert(1)'>x</a>",
		`<details open ontoggle=alert(1)>x</details>`,
		`<object data="javascript:alert(1)"></object>`,
		`<a href="vbscript:msgbox(1)">x</a>`,
		"`<script>alert(1)</script>`",
		"```html\n<script>alert(1)</script>\n```",
	}
	tagRe := regexp.MustCompile(`<([a-zA-Z][a-zA-Z0-9]*)([^>]*)>`)
	attrRe := regexp.MustCompile(`([a-zA-Z:-]+)="([^"]*)"`)
	dangerous := map[string]bool{"script": true, "svg": true, "math": true, "form": true, "base": true, "meta": true, "iframe": true, "object": true, "embed": true, "style": true}
	check := func(out string) string {
		for _, m := range tagRe.FindAllStringSubmatch(out, -1) {
			if dangerous[strings.ToLower(m[1])] {
				return "tag " + m[1]
			}
			rest := attrRe.ReplaceAllString(m[2], "")
			if strings.Trim(rest, " /") != "" {
				return "unquoted attribute text " + rest
			}
			for _, a := range attrRe.FindAllStringSubmatch(m[2], -1) {
				name, val := strings.ToLower(a[1]), strings.ToLower(a[2])
				if strings.HasPrefix(name, "on") || name == "style" {
					return "attribute " + name
				}
				if (name == "href" || name == "src") && (strings.Contains(val, "javascript:") || strings.Contains(val, "vbscript:") || strings.HasPrefix(val, "data:text")) {
					return "url " + val
				}
			}
		}
		return ""
	}
	for _, v := range vectors {
		for _, src := range []string{v + "\n", "para " + v + " end\n"} {
			text, ast := parse(t, src)
			var b bytes.Buffer
			if err := New().Export(context.Background(), text, ast, contract.ExportOptions{Format: contract.ExportHTML, Fragment: true}, &b); err != nil {
				t.Fatal(err)
			}
			if m := check(b.String()); m != "" {
				t.Errorf("%q -> %q (matched %q)", src, b.String(), m)
			}
		}
	}
}
