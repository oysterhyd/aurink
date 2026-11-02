//go:build windows

package workspace

import (
	"syscall"
	"unsafe"
)

var moveFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func replaceFile(from, to string) error {
	src, err := syscall.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	dst, err := syscall.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	const replaceExisting, writeThrough = 0x1, 0x8
	ok, _, callErr := moveFileEx.Call(uintptr(unsafe.Pointer(src)), uintptr(unsafe.Pointer(dst)), replaceExisting|writeThrough)
	if ok == 0 {
		return callErr
	}
	return nil
}
