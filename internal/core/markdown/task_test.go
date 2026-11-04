package markdown

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"aurink/internal/contract"
	"aurink/internal/core/export"
	"aurink/internal/core/projection"
)

func TestEmptyTasks(t *testing.T) {
	for _, shape := range []struct {
		name, prefix, marker, whitespace, before string
		task                                     contract.TaskState
	}{
		{"dash", "- ", "[ ]", " ", "", contract.TaskOpen},
		{"star", "* ", "[x]", " ", "", contract.TaskDone},
		{"plus", "+ ", "[X]", "\t", "", contract.TaskDone},
		{"ordered", "12. ", "[ ]", "  \t ", "", contract.TaskOpen},
		{"ordered-paren", "3) ", "[x]", " ", "", contract.TaskDone},
		{"quote", "> - ", "[ ]", " ", "", contract.TaskOpen},
		{"nested", "  + ", "[X]", "\t", "- parent\n", contract.TaskDone},
		{"second", "- ", "[ ]", " ", "- previous\n", contract.TaskOpen},
	} {
		for _, ending := range []string{"", "\n", "\r\n"} {
			t.Run(fmt.Sprintf("%s/%q", shape.name, ending), func(t *testing.T) {
				before := shape.before
				if ending == "\r\n" {
					before = strings.ReplaceAll(before, "\n", ending)
				}
				src := before + shape.prefix + shape.marker + shape.whitespace + ending
				end := len(src) - len(ending)
				mark := len(before) + len(shape.prefix)
				s := mustNew(t, src, 1, contract.ExtDefault).Snapshot()
				checkCoverage(t, s)
				i := s.LeafAt(end)
				leaf := s.Block(i)
				want := contract.Range{Start: end, End: end}
				if leaf.Kind != contract.BlockBlank || leaf.Range != want || leaf.Content != want || len(leaf.Lines) != 0 {
					t.Fatalf("empty leaf = %+v, want blank at %d", leaf, end)
				}
				item := s.Block(leaf.Parent)
				if item.Kind != contract.BlockItem || item.Task != shape.task || item.TaskMarker != (contract.Range{Start: mark, End: mark + 3}) {
					t.Fatalf("task item = %+v", item)
				}
				pr := projection.New()
				p := pr.Project(s, s.text, i, contract.Reveal{Mode: contract.RevealNone})
				if got := pr.DisplayText(&p); got != "" {
					t.Errorf("display = %q", got)
				}
				for _, aff := range []contract.Affinity{contract.Upstream, contract.Downstream} {
					if got := pr.ToSource(&p, 0, aff); got != end {
						t.Errorf("caret mapping = %d, want %d", got, end)
					}
				}
			})
		}
	}
}

func TestEmptyTaskRequiresWhitespace(t *testing.T) {
	for _, src := range []string{"- [ ]", "- [x]", "- [X]\n", "- [ ]\n- [ ] \n"} {
		t.Run(src, func(t *testing.T) {
			mark := strings.IndexByte(src, '[')
			if _, ok := emptyTaskLine(src, mark); ok {
				t.Fatal("marker without following whitespace accepted")
			}
			s := mustNew(t, src, 1, 0).Snapshot()
			leaf := s.Block(s.LeafAt(mark))
			item := s.Block(leaf.Parent)
			if item.Kind != contract.BlockItem || item.Task != contract.TaskNone || leaf.Kind != contract.BlockParagraph {
				t.Fatalf("item=%+v leaf=%+v", item, leaf)
			}
			pr := projection.New()
			p := pr.Project(s, s.text, s.LeafAt(mark), contract.Reveal{Mode: contract.RevealNone})
			if got, want := pr.DisplayText(&p), src[mark:mark+3]; got != want {
				t.Fatalf("display = %q, want %q", got, want)
			}
		})
	}
}

func TestEmptyTaskContinuations(t *testing.T) {
	for _, indent := range []string{"", "  "} {
		for _, whitespace := range []string{" ", "  \t  "} {
			for _, eol := range []string{"\n", "\r\n"} {
				t.Run(fmt.Sprintf("%q/%q/%q", indent, whitespace, eol), func(t *testing.T) {
					src := "- [x]" + whitespace + eol + indent + "[link] **bold**" + eol + eol + "[link]: /url" + eol
					end := len("- [x]" + whitespace)
					s := mustNew(t, src, 1, contract.ExtDefault).Snapshot()
					checkCoverage(t, s)
					i := s.LeafAt(end)
					b := s.Block(i)
					item := s.Block(b.Parent)
					if b.Kind != contract.BlockParagraph || item.Task != contract.TaskDone || len(b.Lines) != 2 || b.Lines[0].Source != (contract.Range{Start: end, End: end}) {
						t.Fatalf("item=%+v paragraph=%+v", item, b)
					}
					var soft, link, strong bool
					for _, in := range s.Inlines(i) {
						if in.Range.Start < end || strings.Contains(in.Text, "\x01") {
							t.Fatalf("synthetic content leaked: %+v", in)
						}
						soft = soft || in.Kind == contract.InlineSoftBreak
						link = link || in.Kind == contract.InlineLink && in.Dest == "/url"
						strong = strong || in.Kind == contract.InlineStrong
					}
					if !soft || !link || !strong {
						t.Fatalf("continuation inlines = %+v", s.Inlines(i))
					}
				})
			}
		}
	}
}

func TestEmptyTaskContext(t *testing.T) {
	for _, src := range []string{
		"```\n- [ ] \n```\n",
		"    - [ ] \n",
		"- first\n  [ ] \n",
		"- \\[ ] \n",
		"paragraph [ ] \n",
		"- [ ]\n  |---|\n",
	} {
		t.Run(src, func(t *testing.T) {
			s := mustNew(t, src, 1, 0).Snapshot()
			checkCoverage(t, s)
			for i := 0; i < s.Len(); i++ {
				if s.Block(i).Task != contract.TaskNone {
					t.Fatalf("unexpected task in %q: %+v", src, s.Block(i))
				}
			}
		})
	}
	for _, src := range []string{
		"- [ ] \n  - nested\n",
		"- [ ] \n\n  paragraph\n",
		"- [ ] \n- [x] \n\n$$inline$$\n",
	} {
		t.Run(src, func(t *testing.T) {
			s := mustNew(t, src, 1, contract.ExtDefault).Snapshot()
			checkCoverage(t, s)
			end := strings.IndexByte(src, '\n')
			leaf := s.Block(s.LeafAt(end))
			if leaf.Kind != contract.BlockBlank || leaf.Range != (contract.Range{Start: end, End: end}) || s.Block(leaf.Parent).Task != contract.TaskOpen {
				t.Fatalf("first task = %+v, parent %+v", leaf, s.Block(leaf.Parent))
			}
			if s.LeafCount() < 2 {
				t.Fatalf("following content was lost: %q", src)
			}
		})
	}
}

func TestEmptyTaskBeforeTable(t *testing.T) {
	for _, prefix := range []string{"- ", "> 2. "} {
		for _, whitespace := range []string{" ", "\t  "} {
			for _, eol := range []string{"\n", "\r\n"} {
				t.Run(fmt.Sprintf("%q/%q/%q", prefix, whitespace, eol), func(t *testing.T) {
					continuation := "  |---|" + eol
					if strings.HasPrefix(prefix, ">") {
						continuation = ">    |---|" + eol
					}
					src := prefix + "[ ]" + whitespace + eol + continuation
					end := len(prefix + "[ ]" + whitespace)
					s := mustNew(t, src, 1, contract.ExtDefault).Snapshot()
					checkCoverage(t, s)
					i := s.LeafAt(end)
					leaf := s.Block(i)
					if leaf.Kind != contract.BlockTableCell || leaf.Range != (contract.Range{Start: end, End: end}) || len(s.Inlines(i)) != 0 {
						t.Fatalf("header cell = %+v, inlines %+v", leaf, s.Inlines(i))
					}
					var item *contract.Block
					for j := leaf.Parent; j > 0; j = s.Block(j).Parent {
						if s.Block(j).Kind == contract.BlockItem {
							item = s.Block(j)
							break
						}
					}
					if item == nil || item.Task != contract.TaskOpen || src[item.TaskMarker.Start:item.TaskMarker.End] != "[ ]" {
						t.Fatalf("task item = %+v", item)
					}
					pr := projection.New()
					p := pr.Project(s, s.text, i, contract.Reveal{Mode: contract.RevealNone})
					if got := pr.ToSource(&p, 0, contract.Downstream); got != end {
						t.Errorf("caret mapping = %d, want %d", got, end)
					}
				})
			}
		}
	}
}

func TestEmptyTaskIncremental(t *testing.T) {
	for _, shape := range []string{"- [ ]", "> * [ ]", "- parent\n  2. [ ]", "- [ ]\n  |---|\n", "before\n\n- [ ]\n- next\n\nafter\n"} {
		for _, eol := range []string{"\n", "\r\n"} {
			t.Run(fmt.Sprintf("%q/%q", shape, eol), func(t *testing.T) {
				src := strings.ReplaceAll(shape, "\n", eol)
				at := strings.Index(src, "[ ]") + 3
				p := mustNew(t, src, 1, contract.ExtDefault)
				for _, edit := range []contract.TextEdit{
					{Range: contract.Range{Start: at, End: at}, Text: " "},
					{Range: contract.Range{Start: at + 1, End: at + 1}, Text: "b"},
					{Range: contract.Range{Start: at + 1, End: at + 2}},
					{Range: contract.Range{Start: at, End: at + 1}},
					{Range: contract.Range{Start: at, End: at}, Text: "\t"},
					{Range: contract.Range{Start: at, End: at + 1}},
				} {
					old := p.Snapshot()
					u, err := p.Apply([]contract.TextEdit{edit})
					if err != nil {
						t.Fatal(err)
					}
					src = src[:edit.Range.Start] + edit.Text + src[edit.Range.End:]
					fresh := mustNew(t, src, u.Snapshot.Version(), contract.ExtDefault).Snapshot()
					if err := equivalent(u.Snapshot, fresh); err != nil {
						t.Fatalf("after %+v in %q: %v", edit, src, err)
					}
					checkCoverage(t, u.Snapshot)
					checkDirty(t, old, u)
				}
			})
		}
	}
}

func TestEmptyTaskHTMLExport(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"- [ ] ", "<ul>\n<li><input type=\"checkbox\" disabled=\"\" /> </li>\n</ul>\n"},
		{"- [X] \r\n", "<ul>\n<li><input type=\"checkbox\" disabled=\"\" checked=\"\" /> </li>\n</ul>\n"},
	} {
		s := mustNew(t, tc.src, 1, 0).Snapshot()
		var out bytes.Buffer
		if err := export.New().Export(context.Background(), s.text, s, contract.ExportOptions{Format: contract.ExportHTML, Fragment: true}, &out); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != tc.want {
			t.Errorf("export %q = %q, want %q", tc.src, got, tc.want)
		}
	}
}
