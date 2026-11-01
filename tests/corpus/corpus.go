// Package corpus reads the fidelity corpus of testdata/corpus and generates
// large documents.
package corpus

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Root returns the directory of the corpus.
func Root() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "corpus")
}

// Entry is one row of INDEX.md.
type Entry struct {
	Path   string // relative to Root, with forward slashes
	Size   int
	SHA256 string // hex prefix
}

var rowRe = regexp.MustCompile("^\\| `([^`]+)` \\| (\\d+) \\| `([0-9a-f]+)` \\|$")

// Index reads INDEX.md.
func Index() ([]Entry, error) {
	data, err := os.ReadFile(filepath.Join(Root(), "INDEX.md"))
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for line := range strings.SplitSeq(string(data), "\n") {
		m := rowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		size, err := strconv.Atoi(m[2])
		if err != nil {
			return nil, err
		}
		entries = append(entries, Entry{m[1], size, m[3]})
	}
	return entries, nil
}

// Read returns the bytes of an entry.
func Read(e Entry) ([]byte, error) {
	return os.ReadFile(filepath.Join(Root(), filepath.FromSlash(e.Path)))
}

// Generate returns a Markdown document of at least size bytes. The same size
// and seed give the same document.
func Generate(size int, seed int64) string {
	r := rand.New(rand.NewSource(seed))
	var b strings.Builder
	b.Grow(size + 256)
	for n := 0; b.Len() < size; n++ {
		switch r.Intn(8) {
		case 0:
			fmt.Fprintf(&b, "%s 标题 Heading %d\n\n", strings.Repeat("#", 1+r.Intn(6)), n)
		case 1:
			fmt.Fprintf(&b, "> 引用 quote %d\n> 第二行 **粗体**\n\n", n)
		case 2:
			fmt.Fprintf(&b, "- 项 %d\n- [ ] 任务 `code`\n  续行\n\n", n)
		case 3:
			fmt.Fprintf(&b, "```go\nfunc f%d() {}\n```\n\n", n)
		case 4:
			fmt.Fprintf(&b, "| 甲 | b |\n|---|---|\n| %d | *x* |\n\n", n)
		default:
			fmt.Fprintf(&b, "段落 %d，中文与 English 混排，**粗体**、*斜体*、[链接](https://example.com/%d)，以及 &amp; 实体。\n\n", n, n)
		}
	}
	return b.String()
}
