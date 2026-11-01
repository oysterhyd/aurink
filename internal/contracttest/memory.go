package contracttest

import "unsafe"

// sameMemory reports whether part points into whole.
func sameMemory(part, whole string) bool {
	if part == "" || whole == "" {
		return false
	}
	p, w := uintptr(unsafe.Pointer(unsafe.StringData(part))), uintptr(unsafe.Pointer(unsafe.StringData(whole)))
	return p >= w && p < w+uintptr(len(whole))
}
