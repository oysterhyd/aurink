package editing

import (
	"testing"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
)

func TestContractV2(t *testing.T) {
	contracttest.TestEditingV2(t, contracttest.EditingFixture{Editing: New(), Context: func(src string, sel contract.Selection, dominant string) contract.EditContext {
		return contextFor(t, src, sel, dominant)
	}})
}
