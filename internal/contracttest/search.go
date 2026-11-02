package contracttest

import (
	"context"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/internal/mock"
)

type found struct {
	Range contract.Range
	Line  int
	Hit   string // Preview[Hit]
}

func finds(t *testing.T, ms []contract.Match) []found {
	t.Helper()
	var out []found
	for _, m := range ms {
		if !utf8.ValidString(m.Preview) || m.Hit.Start < 0 || m.Hit.End > len(m.Preview) || m.Hit.Start > m.Hit.End {
			t.Fatalf("bad preview %q hit %+v", m.Preview, m.Hit)
		}
		out = append(out, found{m.Range, m.Line, m.Preview[m.Hit.Start:m.Hit.End]})
	}
	return out
}

// TestFinder checks BE-2.6 and the Match rules of docs/04 for finders.
func TestFinder(t *testing.T, f contract.Finder) {
	find := func(t *testing.T, src string, q contract.Query, limit int) []found {
		t.Helper()
		ms, err := f.Find(mock.NewText(src, 1), q, limit)
		if err != nil {
			t.Fatal(err)
		}
		return finds(t, ms)
	}
	r := func(s, e int) contract.Range { return contract.Range{Start: s, End: e} }
	for _, tt := range []struct {
		name  string
		src   string
		q     contract.Query
		limit int
		want  []found
	}{
		{"literal ignores case", "Go go GO", contract.Query{Text: "go"}, 0,
			[]found{{r(0, 2), 0, "Go"}, {r(3, 5), 0, "go"}, {r(6, 8), 0, "GO"}}},
		{"case sensitive", "Go go GO", contract.Query{Text: "go", CaseSensitive: true}, 0,
			[]found{{r(3, 5), 0, "go"}}},
		{"unicode folding", "ÄBC äbc", contract.Query{Text: "äbc"}, 0,
			[]found{{r(0, 4), 0, "ÄBC"}, {r(5, 9), 0, "äbc"}}},
		{"limit", "a a a", contract.Query{Text: "a"}, 2,
			[]found{{r(0, 1), 0, "a"}, {r(2, 3), 0, "a"}}},
		{"no overlap", "aaaa", contract.Query{Text: "aa"}, 0,
			[]found{{r(0, 2), 0, "aa"}, {r(2, 4), 0, "aa"}}},
		{"lines and crlf", "first\nsecond line here\r\nthird line", contract.Query{Text: "line"}, 0,
			[]found{{r(13, 17), 1, "line"}, {r(30, 34), 2, "line"}}},
		{"whole word", "cat concat cat_x cat.", contract.Query{Text: "cat", WholeWord: true}, 0,
			[]found{{r(0, 3), 0, "cat"}, {r(17, 20), 0, "cat"}}},
		{"whole word treats CJK as letters", "说中文\n中文 abc", contract.Query{Text: "中文", WholeWord: true}, 0,
			[]found{{r(10, 16), 1, "中文"}}},
		{"regex", "a1 b22 c333", contract.Query{Text: `[a-z]\d{2,}`, Regex: true}, 0,
			[]found{{r(3, 6), 0, "b22"}, {r(7, 11), 0, "c333"}}},
		{"regex ignores case", "Abc", contract.Query{Text: `abc`, Regex: true}, 0,
			[]found{{r(0, 3), 0, "Abc"}}},
		{"empty matches are not reported", "abc", contract.Query{Text: `z*`, Regex: true}, 0, nil},
		{"multi-line hit stops at the line end", "x line here\r\nthird", contract.Query{Text: `here\r?\nthi`, Regex: true}, 0,
			[]found{{r(7, 16), 0, "here"}}},
		{"empty query", "abc", contract.Query{}, 0, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := find(t, tt.src, tt.q, tt.limit); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
	t.Run("preview is the line", func(t *testing.T) {
		ms, err := f.Find(mock.NewText("first\nsecond line here\r\nthird", 1), contract.Query{Text: "line"}, 0)
		if err != nil || len(ms) != 1 || ms[0].Preview != "second line here" || ms[0].Hit != r(7, 11) {
			t.Fatalf("%+v, %v", ms, err)
		}
	})
	t.Run("long lines are cut near the hit", func(t *testing.T) {
		src := strings.Repeat("中", 400) + "needle" + strings.Repeat("文", 400)
		ms, err := f.Find(mock.NewText(src, 1), contract.Query{Text: "needle"}, 0)
		if err != nil || len(ms) != 1 {
			t.Fatalf("%+v, %v", ms, err)
		}
		m := ms[0]
		if len(m.Preview) > contract.PreviewMax+2*len("…") || !strings.Contains(m.Preview, "…") {
			t.Fatalf("preview of %d bytes: %q", len(m.Preview), m.Preview)
		}
		if got := finds(t, ms); got[0].Hit != "needle" || m.Range != r(1200, 1206) {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("bad regex", func(t *testing.T) {
		_, err := f.Find(mock.NewText("abc", 1), contract.Query{Text: "(", Regex: true}, 0)
		wantCode(t, err, contract.CodeInvalid)
	})
}

// SearchFixture builds a directory searcher.
type SearchFixture struct {
	New func() contract.WorkspaceSearch
	Dir string
}

// TestWorkspaceSearch checks BE-2.6 for directory searchers.
func TestWorkspaceSearch(t *testing.T, newFixture func(t *testing.T) SearchFixture) {
	ignore := func(rel string, isDir bool) bool {
		return rel == "node_modules" || strings.HasSuffix(rel, "/node_modules")
	}
	tree := func(t *testing.T) SearchFixture {
		f := newFixture(t)
		files := map[string]string{
			"a.md":                  "hello world\nsay Hello",
			"sub/b.md":              "HELLO",
			"c.txt":                 "hello",
			"none.md":               "nothing here",
			"node_modules/x.md":     "hello",
			"sub/node_modules/y.md": "hello",
			"bin.dat":               "hello\x00\x01",
			"big.md":                strings.Repeat("hello ", 200),
			"utf16.md":              "\xFF\xFEh\x00e\x00l\x00l\x00o\x00",
		}
		for name, text := range files {
			write(t, filepath.Join(f.Dir, filepath.FromSlash(name)), text)
		}
		return f
	}
	search := func(t *testing.T, f SearchFixture, ctx context.Context, q contract.Query, opts contract.SearchOptions) (map[string]int, error) {
		t.Helper()
		got := map[string]int{}
		err := f.New().Search(ctx, f.Dir, q, opts, func(batch []contract.FileMatches) {
			for _, fm := range batch {
				if _, dup := got[fm.Path]; dup {
					t.Errorf("%s came twice", fm.Path)
				}
				if len(fm.Matches) == 0 {
					t.Errorf("%s came without matches", fm.Path)
				}
				rel, _ := filepath.Rel(f.Dir, fm.Path)
				got[filepath.ToSlash(rel)] = len(fm.Matches)
			}
		})
		return got, err
	}
	t.Run("finds text files", func(t *testing.T) {
		f := tree(t)
		got, err := search(t, f, context.Background(), contract.Query{Text: "hello"}, contract.SearchOptions{Ignore: ignore, MaxFileSize: 1000})
		want := map[string]int{"a.md": 2, "sub/b.md": 1, "c.txt": 1, "utf16.md": 1}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, %v; want %v", got, err, want)
		}
	})
	t.Run("default size limit and no ignore", func(t *testing.T) {
		f := tree(t)
		got, err := search(t, f, context.Background(), contract.Query{Text: "hello", CaseSensitive: true}, contract.SearchOptions{})
		want := map[string]int{"a.md": 1, "c.txt": 1, "utf16.md": 1, "big.md": 200, "node_modules/x.md": 1, "sub/node_modules/y.md": 1}
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, %v; want %v", got, err, want)
		}
	})
	t.Run("per file limit", func(t *testing.T) {
		f := tree(t)
		got, err := search(t, f, context.Background(), contract.Query{Text: "hello"}, contract.SearchOptions{Ignore: ignore, PerFile: 3})
		if err != nil || got["big.md"] != 3 || got["a.md"] != 2 {
			t.Fatalf("got %v, %v", got, err)
		}
	})
	t.Run("bad regex", func(t *testing.T) {
		f := tree(t)
		_, err := search(t, f, context.Background(), contract.Query{Text: "(", Regex: true}, contract.SearchOptions{})
		wantCode(t, err, contract.CodeInvalid)
	})
	t.Run("canceled before", func(t *testing.T) {
		f := tree(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		got, err := search(t, f, ctx, contract.Query{Text: "hello"}, contract.SearchOptions{})
		wantCode(t, err, contract.CodeCanceled)
		if len(got) != 0 {
			t.Fatalf("batches after cancel: %v", got)
		}
	})
	t.Run("canceled during", func(t *testing.T) {
		f := newFixture(t)
		for i := 0; i < 2000; i++ {
			write(t, filepath.Join(f.Dir, "d"+string(rune('a'+i%26)), strings.Repeat("x", i%7+1)+string(rune('a'+i/26%26))+string(rune('a'+i/676))+".md"), "hello")
		}
		ctx, cancel := context.WithCancel(context.Background())
		batches, after := 0, 0
		err := f.New().Search(ctx, f.Dir, contract.Query{Text: "hello"}, contract.SearchOptions{}, func(batch []contract.FileMatches) {
			if ctx.Err() != nil {
				after++
			}
			batches++
			cancel()
		})
		wantCode(t, err, contract.CodeCanceled)
		if batches == 0 || after != 0 {
			t.Fatalf("%d batches, %d after cancel", batches, after)
		}
	})
}

// TestFuzzy checks the ranking rules of docs/04 for fuzzy matchers.
func TestFuzzy(t *testing.T, f contract.Fuzzy) {
	items := func(texts ...string) []contract.FuzzyItem {
		out := make([]contract.FuzzyItem, len(texts))
		for i, s := range texts {
			out[i] = contract.FuzzyItem{Text: s}
		}
		return out
	}
	order := func(hits []contract.FuzzyHit) []int {
		var out []int
		for _, h := range hits {
			out = append(out, h.Index)
		}
		return out
	}
	t.Run("subsequence, any case", func(t *testing.T) {
		hits := f.Match("RM", items("readme.md", "rm", "mr", "Rome"), 0)
		got := order(hits)
		sort.Ints(got)
		if !reflect.DeepEqual(got, []int{0, 1, 3}) {
			t.Fatalf("hits %v", got)
		}
	})
	t.Run("word starts rank higher", func(t *testing.T) {
		if got := order(f.Match("fb", items("fxxxb", "foo bar", "bar"), 0)); !reflect.DeepEqual(got, []int{1, 0}) {
			t.Fatalf("order %v", got)
		}
		if got := order(f.Match("ab", items("xaxb", "a_b"), 0)); len(got) != 2 || got[0] != 1 {
			t.Fatalf("order %v", got)
		}
	})
	t.Run("contiguous ranks higher", func(t *testing.T) {
		if got := order(f.Match("note", items("n o t e", "notes"), 0)); len(got) == 0 || got[0] != 1 {
			t.Fatalf("order %v", got)
		}
	})
	t.Run("file name ranks higher", func(t *testing.T) {
		if got := order(f.Match("main", items("main/other.go", "src/main.go"), 0)); len(got) != 2 || got[0] != 1 {
			t.Fatalf("order %v", got)
		}
		if got := order(f.Match("main", items(`main\other.go`, `src\main.go`), 0)); len(got) != 2 || got[0] != 1 {
			t.Fatalf("order with backslashes %v", got)
		}
	})
	t.Run("positions are rune indexes", func(t *testing.T) {
		hits := f.Match("中文", items("我的中文笔记.md"), 0)
		if len(hits) != 1 || !reflect.DeepEqual(hits[0].Positions, []int{2, 3}) {
			t.Fatalf("hits %+v", hits)
		}
		hits = f.Match("ab", items("xaxb"), 0)
		if len(hits) != 1 || !reflect.DeepEqual(hits[0].Positions, []int{1, 3}) {
			t.Fatalf("hits %+v", hits)
		}
	})
	t.Run("spaces in the query are ignored", func(t *testing.T) {
		if hits := f.Match("foo bar", items("foobar"), 0); len(hits) != 1 {
			t.Fatalf("hits %+v", hits)
		}
	})
	t.Run("ties keep item order, and limit", func(t *testing.T) {
		if got := order(f.Match("a", items("a.md", "a.md", "a.md"), 2)); !reflect.DeepEqual(got, []int{0, 1}) {
			t.Fatalf("order %v", got)
		}
	})
	t.Run("empty query", func(t *testing.T) {
		hits := f.Match("", items("b", "a", "c"), 0)
		if !reflect.DeepEqual(order(hits), []int{0, 1, 2}) {
			t.Fatalf("hits %+v", hits)
		}
		for _, h := range hits {
			if h.Score != 0 || len(h.Positions) != 0 {
				t.Fatalf("hit %+v", h)
			}
		}
	})
}
