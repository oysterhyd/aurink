//go:build !windows

package fsstore

import (
	"os"
	"syscall"
)

func multipleLinks(_ string, info os.FileInfo) (bool, error) {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Nlink > 1, nil
}

func preserveMode(f *os.File, info os.FileInfo) error {
	if info == nil {
		return nil
	}
	return f.Chmod(info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky))
}

func replaceFile(target, replacement string) error { return os.Rename(replacement, target) }
