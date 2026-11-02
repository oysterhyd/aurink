package search_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/core/search"
	coretext "aurink/internal/core/text"
	"aurink/tests/corpus"
)

func BenchmarkFind(b *testing.B) {
	text := coretext.New(corpus.Generate(1<<20, 26), 1)
	finder := search.NewFinder()
	for _, tc := range []struct {
		name  string
		query contract.Query
	}{
		{"literal", contract.Query{Text: "English"}},
		{"regex", contract.Query{Text: "En[a-z]+h", Regex: true, CaseSensitive: true}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(text.Len()))
			for b.Loop() {
				if _, err := finder.Find(text, tc.query, 0); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkFuzzy(b *testing.B) {
	items := make([]contract.FuzzyItem, 0, 10500)
	for i := 0; i < 10000; i++ {
		if i%2 == 0 {
			items = append(items, contract.FuzzyItem{Text: fmt.Sprintf("projects/group-%03d/notes/document-%05d.md", i%100, i)})
		} else {
			items = append(items, contract.FuzzyItem{Text: fmt.Sprintf("projects/group-%03d/src/component-%05d.go", i%100, i)})
		}
	}
	for i := 0; i < 500; i++ {
		items = append(items, contract.FuzzyItem{Text: fmt.Sprintf("Open Markdown Document %03d", i)})
	}
	fuzzy := search.NewFuzzy()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		fuzzy.Match("md", items, 100)
	}
}

func BenchmarkWorkspaceCancel10000(b *testing.B) {
	root := b.TempDir()
	for i := 0; i < 100; i++ {
		dir := filepath.Join(root, fmt.Sprintf("dir-%03d", i))
		if err := os.Mkdir(dir, 0o700); err != nil {
			b.Fatal(err)
		}
		for j := 0; j < 100; j++ {
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file-%03d.md", j)), []byte("hello world\n"), 0o600); err != nil {
				b.Fatal(err)
			}
		}
	}
	s := search.NewWorkspaceSearch()
	var maxSearch, maxCancel time.Duration
	b.ResetTimer()
	for b.Loop() {
		ctx, cancel := context.WithCancel(context.Background())
		var canceledAt time.Time
		startedAt := time.Now()
		err := s.Search(ctx, root, contract.Query{Text: "hello"}, contract.SearchOptions{}, func([]contract.FileMatches) {
			canceledAt = time.Now()
			cancel()
		})
		if contract.CodeOf(err) != contract.CodeCanceled || canceledAt.IsZero() {
			b.Fatalf("search cancellation: %v", err)
		}
		finishedAt := time.Now()
		maxSearch = max(maxSearch, finishedAt.Sub(startedAt))
		maxCancel = max(maxCancel, finishedAt.Sub(canceledAt))
		cancel()
	}
	b.ReportMetric(float64(maxCancel)/float64(time.Millisecond), "max-cancel-ms")
	b.ReportMetric(float64(maxSearch)/float64(time.Millisecond), "max-search-ms")
}
