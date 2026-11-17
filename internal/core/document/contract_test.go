package document

import (
	"testing"
	"time"

	"aurink/internal/contract"
	"aurink/internal/contracttest"
)

func TestContract(t *testing.T) {
	contracttest.TestDocument(t, func(t *testing.T) contracttest.DocumentFixture {
		now := time.Unix(0, 0)
		f := Factory{Clock: func() time.Time { return now }}
		return contracttest.DocumentFixture{
			New: func(source string) contract.DocumentModel {
				return f.New("test", source, contract.DocMeta{}, contract.ExtFrontMatter)
			},
			Advance: func(d time.Duration) { now = now.Add(d) },
		}
	})
}

func TestLargeDocumentContract(t *testing.T) {
	contracttest.TestLargeDocument(t, Factory{})
}
