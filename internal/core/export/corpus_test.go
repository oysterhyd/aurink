package export

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/tests/corpus"
	"github.com/egoist/mygo/ui/markdown/parser"
)

var mathBlock = regexp.MustCompile(`(?s)<div class="math">.*?</div>\n?`)

// Intentional whole-file differences: front matter is omitted, math uses a
// div fallback, and the two JSON files contain raw HTML embedded in JSON
// strings; treating each entire JSON file as Markdown triggers script/style
// removal. Their actual Markdown examples are also checked individually below.
var rawJSONStructure = map[string][2]string{
	"spec/commonmark.json": {"b2a55733d03b12bd6f72046dcb6de2a8660d763fb3be4382761681755bd2a6e6", "6cc36d8b6f690dfcfff6c22480344dc9a49722245a047b9f45512ae84b517800"},
	"spec/gfm.json":        {"b2a55733d03b12bd6f72046dcb6de2a8660d763fb3be4382761681755bd2a6e6", "eb7b9d71561846cc5350e1e4b3f60636d1a8576a5152f55214fb302b16604562"},
}

var mathFiles = map[string]int{"spec/extras.md": 2, "edge/math.md": 1}
var frontFiles = map[string]bool{"edge/front-matter.md": true, "edge/front-matter-crlf.md": true}

// structure removes attributes and text but retains the order and nesting of
// the block elements shared by the parser and the exporter.
func structure(s string) []string {
	var tags []string
	for len(s) > 0 {
		i := strings.IndexByte(s, '<')
		if i < 0 {
			break
		}
		s = s[i:]
		if strings.HasPrefix(s, "<!--") {
			end := strings.Index(s, "-->")
			if end < 0 {
				break
			}
			s = s[end+3:]
			continue
		}
		name, close, _, _, end := parseTag(s)
		if end == 0 {
			s = s[1:]
			continue
		}
		s = s[end:]
		switch name {
		case "h1", "h2", "h3", "h4", "h5", "h6", "p", "ul", "ol", "li", "pre",
			"table", "thead", "tbody", "tr", "th", "td", "blockquote":
			if close {
				name = "/" + name
			}
			tags = append(tags, name)
		}
	}
	return tags
}

func TestCorpusStructure(t *testing.T) {
	entries, err := corpus.Index()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Run(entry.Path, func(t *testing.T) {
			src, err := corpus.Read(entry)
			if err != nil {
				t.Fatal(err)
			}
			text, ast := parse(t, string(src))
			var out bytes.Buffer
			if err := New().Export(context.Background(), text, ast, contract.ExportOptions{Format: contract.ExportHTML, Fragment: true}, &out); err != nil {
				t.Fatal(err)
			}
			exportHTML := out.String()
			parserHTML := parser.Parse(string(src)).HTML()
			if hashes, ok := rawJSONStructure[entry.Path]; ok {
				gotHash := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(structure(exportHTML), "\n"))))
				wantHash := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(structure(parserHTML), "\n"))))
				if gotHash != hashes[0] || wantHash != hashes[1] {
					t.Errorf("raw JSON structure changed: export %s, parser %s", gotHash, wantHash)
				}
				return
			}
			if count := mathFiles[entry.Path]; count > 0 {
				if got := len(mathBlock.FindAllStringIndex(exportHTML, -1)); got != count {
					t.Fatalf("math fallback blocks = %d, want %d", got, count)
				}
				exportHTML = mathBlock.ReplaceAllString(exportHTML, "")
			}
			if frontFiles[entry.Path] {
				front, ok := ast.FrontMatter()
				if !ok {
					t.Fatal("expected front matter")
				}
				parserHTML = parser.Parse(string(src[front.Range.End:])).HTML()
			}
			got, want := structure(exportHTML), structure(parserHTML)
			if !slices.Equal(got, want) {
				t.Errorf("block structure differs: export %v, parser %v", got, want)
			}
		})
	}
}

// The JSON specs are collections of examples. Compare each example as well as
// the corpus entries above; the small exception lists document the exact cases
// where front matter, math fallback or safe HTML deliberately changes output.
func TestSpecificationExamples(t *testing.T) {
	for _, file := range []struct {
		path       string
		front      map[int]bool
		math       map[int]bool
		badNesting int
		unsafeHTML int
	}{
		{"spec/commonmark.json", map[int]bool{96: true}, map[int]bool{14: true, 563: true}, 148, 0},
		{"spec/gfm.json", map[int]bool{66: true}, map[int]bool{310: true, 571: true}, 118, 652},
	} {
		data, err := corpus.Read(corpus.Entry{Path: file.path})
		if err != nil {
			t.Fatal(err)
		}
		var examples []struct {
			Markdown string
			Example  int
		}
		if err := json.Unmarshal(data, &examples); err != nil || len(examples) == 0 {
			t.Fatalf("%s: examples: %v", file.path, err)
		}
		for _, example := range examples {
			text, ast := parse(t, example.Markdown)
			var out bytes.Buffer
			if err := New().Export(context.Background(), text, ast, contract.ExportOptions{Format: contract.ExportHTML, Fragment: true}, &out); err != nil {
				t.Fatalf("%s example %d: %v", file.path, example.Example, err)
			}
			exportHTML := out.String()
			parserHTML := parser.Parse(example.Markdown).HTML()
			if file.front[example.Example] {
				front, ok := ast.FrontMatter()
				if !ok {
					t.Fatalf("%s example %d: expected front matter", file.path, example.Example)
				}
				parserHTML = parser.Parse(example.Markdown[front.Range.End:]).HTML()
			}
			if file.math[example.Example] {
				if len(mathBlock.FindAllStringIndex(exportHTML, -1)) != 1 {
					t.Fatalf("%s example %d: expected one math fallback", file.path, example.Example)
				}
				exportHTML = mathBlock.ReplaceAllString(exportHTML, "")
			}
			got, want := structure(exportHTML), structure(parserHTML)
			if example.Example == file.badNesting {
				// The parser crosses </pre> and </p>; the exporter balances them.
				if !slices.Equal(got, []string{"table", "tr", "td", "pre", "p", "/p", "/pre", "/td", "/tr", "/table"}) ||
					!slices.Equal(want, []string{"table", "tr", "td", "pre", "p", "/pre", "/p", "/td", "/tr", "/table"}) {
					t.Errorf("%s example %d: unexpected nesting: %v, %v", file.path, example.Example, got, want)
				}
				continue
			}
			if example.Example == file.unsafeHTML {
				// An unclosed <style> consumes the rest of the input in the exporter.
				if !slices.Equal(got, []string{"p", "/p"}) || !slices.Equal(want, []string{"p", "/p", "blockquote", "/blockquote"}) {
					t.Errorf("%s example %d: unexpected style handling: %v, %v", file.path, example.Example, got, want)
				}
				continue
			}
			if !slices.Equal(got, want) {
				t.Errorf("%s example %d: export %v, parser %v", file.path, example.Example, got, want)
			}
		}
	}
}
