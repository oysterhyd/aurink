package markdown

import (
	"testing"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
)

func mustNew(t testing.TB, src string, v contract.Version, ext contract.Extensions) *Parser {
	t.Helper()
	p, err := New(src, v, ext)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func fixture(t *testing.T) contracttest.ASTFixture {
	return contracttest.ASTFixture{
		Parse: func(src string, ext contract.Extensions) contract.AST { return mustNew(t, src, 1, ext).Snapshot() },
		Edit: func(prev contract.AST, src string, edits []contract.TextEdit) (contract.AST, string) {
			old := prev.(*Snapshot)
			p := mustNew(t, src, old.Version(), old.exts)
			p.current = old
			p.next = old.highest
			for i := range old.entries {
				p.next = max(p.next, old.entries[i].id)
			}
			u, err := p.Apply(edits)
			if err != nil {
				t.Fatal(err)
			}
			return u.Snapshot, u.Snapshot.text.String()
		},
	}
}

func TestContract(t *testing.T) { contracttest.TestAST(t, fixture(t)) }

func TestExtensionsContract(t *testing.T) {
	contracttest.TestExtensions(t, fixture(t))
}
