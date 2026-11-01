//go:build windows

package fsstore

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32        = syscall.NewLazyDLL("kernel32.dll")
	procReplaceFile = kernel32.NewProc("ReplaceFileW")
	procMoveFileEx  = kernel32.NewProc("MoveFileExW")
)

func multipleLinks(path string, _ os.FileInfo) (bool, error) {
	// Query metadata without requiring permission to read the contents.
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false, err
	}
	var info syscall.ByHandleFileInformation
	err = syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return info.NumberOfLinks > 1, err
}

func preserveMode(_ *os.File, _ os.FileInfo) error { return nil }

func replaceFile(target, replacement string) error {
	to, err := syscall.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	from, err := syscall.UTF16PtrFromString(replacement)
	if err != nil {
		return err
	}
	// ReplaceFile preserves the destination's ACL and other file metadata.
	r, _, _ := procReplaceFile.Call(uintptr(unsafe.Pointer(to)), uintptr(unsafe.Pointer(from)), 0, 0, 0, 0)
	if r != 0 {
		return nil
	}
	const moveReplaceExisting, moveWriteThrough = 0x1, 0x8
	r, _, err = procMoveFileEx.Call(uintptr(unsafe.Pointer(from)), uintptr(unsafe.Pointer(to)), moveReplaceExisting|moveWriteThrough)
	if r == 0 {
		return err
	}
	return nil
}
