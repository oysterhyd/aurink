package contracttest

import (
	"testing"

	"aurink/internal/contract"
	"aurink/internal/mock"
)

func TestMockText(t *testing.T) {
	TestText(t, func(t *testing.T, s string) contract.Text { return mock.NewText(s, 1) })
}

func TestMockFileStore(t *testing.T) {
	TestFileStore(t, func(t *testing.T) FileStoreFixture {
		s := mock.NewFileStore()
		return FileStoreFixture{
			Store: s,
			Path:  func(name string) string { return "/" + name },
			Put:   s.Put,
			Bytes: func(path string) []byte {
				b, ok := s.Bytes(path)
				if !ok {
					t.Fatalf("no file %s", path)
				}
				return b
			},
		}
	})
}

func TestQueuedRunsInOrderOnFlush(t *testing.T) {
	var q mock.Queued
	var got []int
	q.Post(func() {
		got = append(got, 1)
		q.Post(func() { got = append(got, 3) })
	})
	q.Post(func() { got = append(got, 2) })
	if len(got) != 0 {
		t.Fatal("Post ran the function")
	}
	q.Flush()
	if len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("ran %v", got)
	}
}

var (
	_ contract.Text       = (*mock.Text)(nil)
	_ contract.FileStore  = (*mock.FileStore)(nil)
	_ contract.Dispatcher = mock.Immediate{}
	_ contract.Dispatcher = (*mock.Queued)(nil)
)
