package document

import (
	"syscall"
	"unsafe"
)

func init() {
	// QPC preserves submillisecond samples on Windows runtimes with coarse clocks.
	kernel := syscall.NewLazyDLL("kernel32.dll")
	counter := kernel.NewProc("QueryPerformanceCounter")
	freq := kernel.NewProc("QueryPerformanceFrequency")
	var frequency int64
	if ok, _, _ := freq.Call(uintptr(unsafe.Pointer(&frequency))); ok == 0 {
		return
	}
	benchmarkNow = func() int64 {
		var tick int64
		counter.Call(uintptr(unsafe.Pointer(&tick)))
		return tick/frequency*1e9 + (tick%frequency)*1e9/frequency
	}
}
