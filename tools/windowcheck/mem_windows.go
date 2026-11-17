//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

type processMemoryCountersEx struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentProcess = kernel32.NewProc("GetCurrentProcess")
	procGetProcessMemInfo = kernel32.NewProc("K32GetProcessMemoryInfo")
)

func processMemory() (workingSetBytes, privateBytes uint64, ok bool) {
	h, _, _ := procGetCurrentProcess.Call()
	var pmc processMemoryCountersEx
	pmc.CB = uint32(unsafe.Sizeof(pmc))
	ret, _, _ := procGetProcessMemInfo.Call(h, uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.CB))
	if ret == 0 {
		return 0, 0, false
	}
	priv := uint64(pmc.PrivateUsage)
	if priv == 0 {
		priv = uint64(pmc.PagefileUsage)
	}
	return uint64(pmc.WorkingSetSize), priv, true
}
