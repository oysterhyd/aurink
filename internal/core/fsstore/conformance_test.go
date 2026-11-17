package fsstore

import (
	"path/filepath"
	"testing"

	"aurink/internal/contracttest"
)

func TestFileStore(t *testing.T) {
	contracttest.TestFileStore(t, func(t *testing.T) contracttest.FileStoreFixture {
		dir := t.TempDir()
		return contracttest.FileStoreFixture{
			Store: New(),
			Path:  func(name string) string { return filepath.Join(dir, name) },
			Put:   func(path string, data []byte) { put(t, path, data) },
			Bytes: func(path string) []byte { return fileBytes(t, path) },
		}
	})
}

func TestFileStoreGB18030(t *testing.T) {
	t.Skip("BE-2.11")
	contracttest.TestFileStoreGB18030(t, func(t *testing.T) contracttest.FileStoreFixture {
		dir := t.TempDir()
		return contracttest.FileStoreFixture{
			Store: New(),
			Path:  func(name string) string { return filepath.Join(dir, name) },
			Put:   func(path string, data []byte) { put(t, path, data) },
			Bytes: func(path string) []byte { return fileBytes(t, path) },
		}
	})
}
