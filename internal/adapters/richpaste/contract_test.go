package richpaste

import (
	"testing"

	"aurink/internal/contracttest"
)

func TestContract(t *testing.T) { contracttest.TestPasteConverter(t, New()) }
