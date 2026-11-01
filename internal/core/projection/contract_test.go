package projection

import (
	"testing"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
	"aurink/internal/core/markdown"
	"aurink/internal/core/text"
)

func parse(t testing.TB, src string) (contract.Text, contract.AST) {
	t.Helper()
	p, err := markdown.New(src, 1, contract.ExtFrontMatter)
	if err != nil {
		t.Fatal(err)
	}
	return text.New(src, 1), p.Snapshot()
}

func TestContract(t *testing.T) {
	contracttest.TestProjection(t, contracttest.ProjectionFixture{Projector: New(), Parse: func(src string) (contract.Text, contract.AST) {
		return parse(t, src)
	}})
}
