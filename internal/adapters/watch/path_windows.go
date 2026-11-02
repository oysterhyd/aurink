//go:build windows

package watch

import (
	"os"
	"strings"
	"syscall"
	"unsafe"
)

var finalPath = syscall.NewLazyDLL("kernel32.dll").NewProc("GetFinalPathNameByHandleW")

// Go's EvalSymlinks leaves a directory junction unresolved. Resolve the
// opened target before handing it to MyGo, which otherwise watches the
// junction as a file rather than watching the directory it names.
func watchPath(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	buf := make([]uint16, 256)
	for {
		n, _, callErr := finalPath.Call(f.Fd(), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
		if n == 0 {
			return "", callErr
		}
		if n >= uintptr(len(buf)) {
			buf = make([]uint16, n+1)
			continue
		}
		resolved := syscall.UTF16ToString(buf[:n])
		if strings.HasPrefix(resolved, `\\?\UNC\`) {
			return `\\` + resolved[len(`\\?\UNC\`):], nil
		}
		if strings.HasPrefix(resolved, `\\?\`) {
			tail := resolved[len(`\\?\`):]
			if len(tail) >= 2 && tail[1] == ':' {
				return tail, nil
			}
		}
		return resolved, nil
	}
}
