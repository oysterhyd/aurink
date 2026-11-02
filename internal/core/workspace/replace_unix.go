//go:build !windows

package workspace

import "os"

func replaceFile(from, to string) error { return os.Rename(from, to) }
