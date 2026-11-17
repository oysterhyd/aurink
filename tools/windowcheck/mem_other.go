//go:build !windows

package main

import "runtime"

func processMemory() (workingSetBytes, privateBytes uint64, ok bool) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.Sys, ms.HeapInuse + ms.StackInuse, false
}
