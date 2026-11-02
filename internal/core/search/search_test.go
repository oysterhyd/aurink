package search_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"aurink/internal/contract"
	"aurink/internal/core/search"
	coretext "aurink/internal/core/text"
)

func TestFinderUnicodeAndBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		query      contract.Query
		want       []contract.Range
	}{
		{"simple fold with different byte lengths", "kKK", contract.Query{Text: "k"}, []contract.Range{{Start: 0, End: 1}, {Start: 1, End: 4}, {Start: 4, End: 5}}},
		{"simple fold long s", "sſS", contract.Query{Text: "ſ"}, []contract.Range{{Start: 0, End: 1}, {Start: 1, End: 3}, {Start: 3, End: 4}}},
		{"simple fold sigma", "σςΣ", contract.Query{Text: "σ"}, []contract.Range{{Start: 0, End: 2}, {Start: 2, End: 4}, {Start: 4, End: 6}}},
		{"rejected word can overlap", "xa-a-a", contract.Query{Text: "a-a", WholeWord: true}, []contract.Range{{Start: 3, End: 6}}},
		{"rejected regex word can overlap", "xa-a-a", contract.Query{Text: "a-a", Regex: true, WholeWord: true}, []contract.Range{{Start: 3, End: 6}}},
		{"regex word boundary retains left context", "a ba a", contract.Query{Text: `\ba`, Regex: true, WholeWord: true}, []contract.Range{{Start: 0, End: 1}, {Start: 5, End: 6}}},
		{"regex multiline anchor retains left context", "a\na", contract.Query{Text: `(?m)^a`, Regex: true, WholeWord: true}, []contract.Range{{Start: 0, End: 1}, {Start: 2, End: 3}}},
		{"regex text anchor stays at document start", "a a", contract.Query{Text: `\Aa`, Regex: true, WholeWord: true}, []contract.Range{{Start: 0, End: 1}}},
		{"regex anchors apply to the document", "a\na", contract.Query{Text: "^a", Regex: true}, []contract.Range{{Start: 0, End: 1}}},
		{"regex skips empty results", "ba", contract.Query{Text: "a*", Regex: true}, []contract.Range{{Start: 1, End: 2}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			matches, err := search.NewFinder().Find(coretext.New(tc.text, 1), tc.query, 0)
			if err != nil {
				t.Fatal(err)
			}
			var got []contract.Range
			for _, m := range matches {
				got = append(got, m.Range)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ranges %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNullableRegexMatchesStandardRegexp(t *testing.T) {
	patterns := []string{`a*`, `a*?`, `a?|b`, `(?m)^|a`, `\b|a`, `\B|a`, `\Aa?|b`, `(?:a|b)*`, `a*\b`, `(?m)^a?`}
	texts := []string{"", "abc", "ba a", "a\na", "中ab", "ab\n"}
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		for _, text := range texts {
			var want []contract.Range
			for _, match := range re.FindAllStringIndex(text, -1) {
				if match[0] != match[1] {
					want = append(want, contract.Range{Start: match[0], End: match[1]})
				}
			}
			matches, err := search.NewFinder().Find(coretext.New(text, 1), contract.Query{Text: pattern, CaseSensitive: true, Regex: true}, 0)
			if err != nil {
				t.Fatal(err)
			}
			var got []contract.Range
			for _, match := range matches {
				got = append(got, match.Range)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("pattern %q, text %q: got %v, want %v", pattern, text, got, want)
			}
		}
	}
}

func TestWholeWordRegexAfterLongRejectedWord(t *testing.T) {
	text := "x" + strings.Repeat("a", 10000) + "! aaa"
	matches, err := search.NewFinder().Find(coretext.New(text, 1), contract.Query{Text: `a+`, Regex: true, WholeWord: true}, 0)
	if err != nil || len(matches) != 1 || matches[0].Range != (contract.Range{Start: len(text) - 3, End: len(text)}) {
		t.Fatalf("matches %v, error %v", matches, err)
	}
}

func TestFinderPreviewForLongAndNewlineMatches(t *testing.T) {
	text := strings.Repeat("中", 100) + strings.Repeat("a", 400) + "\r\nend"
	for _, q := range []contract.Query{{Text: strings.Repeat("a", 400)}, {Text: "\r\n"}} {
		matches, err := search.NewFinder().Find(coretext.New(text, 1), q, 0)
		if err != nil || len(matches) != 1 {
			t.Fatalf("matches %v, error %v", matches, err)
		}
		m := matches[0]
		if !utf8.ValidString(m.Preview) || len(m.Preview) > contract.PreviewMax+2*len("…") || m.Hit.Start > m.Hit.End || m.Hit.End > len(m.Preview) || strings.ContainsAny(m.Preview, "\r\n") {
			t.Fatalf("invalid preview: %+v", m)
		}
		if q.Text == "\r\n" && m.Hit.Start != m.Hit.End {
			t.Fatalf("newline should have an empty hit in the line preview: %+v", m)
		}
	}
}

func TestWorkspaceSearchEncodingAndSizes(t *testing.T) {
	dir := t.TempDir()
	files := map[string][]byte{
		"utf8bom":  append([]byte{0xef, 0xbb, 0xbf}, []byte("hello")...),
		"utf16be":  {0xfe, 0xff, 0, 'h', 0, 'e', 0, 'l', 0, 'l', 0, 'o'},
		"bad16":    {0xff, 0xfe, 0, 0xd8, 'h', 0},
		"odd16":    {0xff, 0xfe, 'h'},
		"nul16":    {0xfe, 0xff, 0, 'h', 0, 0},
		"badutf8":  {'h', 'e', 'l', 'l', 'o', 0xff},
		"large":    []byte(strings.Repeat("x", 13) + "hello"),
		"at-limit": []byte("hello" + strings.Repeat("x", 7)),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got := map[string]bool{}
	err := search.NewWorkspaceSearch().Search(context.Background(), dir, contract.Query{Text: "hello"}, contract.SearchOptions{MaxFileSize: 12}, func(batch []contract.FileMatches) {
		for _, file := range batch {
			if got[filepath.Base(file.Path)] {
				t.Errorf("duplicate file %s", file.Path)
			}
			got[filepath.Base(file.Path)] = true
		}
	})
	want := map[string]bool{"utf8bom": true, "utf16be": true, "at-limit": true}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, %v; want %v", got, err, want)
	}
}

func TestWorkspaceSearchCancellationDoesNotWaitForWalk(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.md"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	returned := make(chan error, 1)
	go func() {
		returned <- search.NewWorkspaceSearch().Search(ctx, dir, contract.Query{Text: "hello"}, contract.SearchOptions{Ignore: func(string, bool) bool {
			close(entered)
			<-release
			return false
		}}, nil)
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("walk did not reach ignore callback")
	}
	cancel()
	select {
	case err := <-returned:
		if contract.CodeOf(err) != contract.CodeCanceled {
			t.Fatalf("got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("search did not return after cancellation")
	}
}

func TestFuzzyChoosesBestPositions(t *testing.T) {
	f := search.NewFuzzy()
	hits := f.Match("ab", []contract.FuzzyItem{{Text: "a---a_b"}, {Text: "axxb"}, {Text: "aB"}}, 0)
	if len(hits) != 3 || hits[0].Index != 2 {
		t.Fatalf("unexpected ranking: %+v", hits)
	}
	for _, hit := range hits {
		if hit.Index == 0 && !reflect.DeepEqual(hit.Positions, []int{4, 6}) {
			t.Fatalf("did not choose the word starts: %+v", hit)
		}
	}
	hits = f.Match("k σ", []contract.FuzzyItem{{Text: "Kς"}}, 0)
	if len(hits) != 1 || !reflect.DeepEqual(hits[0].Positions, []int{0, 1}) {
		t.Fatalf("Unicode folding: %+v", hits)
	}
}
