package search_test

import (
	"testing"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
	"aurink/internal/core/search"
)

func TestFinder(t *testing.T) {
	contracttest.TestFinder(t, search.NewFinder())
}

func TestWorkspaceSearch(t *testing.T) {
	contracttest.TestWorkspaceSearch(t, func(t *testing.T) contracttest.SearchFixture {
		return contracttest.SearchFixture{
			New: func() contract.WorkspaceSearch { return search.NewWorkspaceSearch() },
			Dir: t.TempDir(),
		}
	})
}

func TestFuzzy(t *testing.T) {
	contracttest.TestFuzzy(t, search.NewFuzzy())
}
