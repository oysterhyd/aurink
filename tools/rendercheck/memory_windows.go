//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

type memorySample struct {
	Stage            string `json:"stage"`
	PageProcessID    int    `json:"page_process_id"`
	WebViewProcesses int    `json:"webview_processes"`
	WorkingSetBytes  uint64 `json:"working_set_bytes"`
	PrivateBytes     uint64 `json:"private_bytes"`
	Error            string `json:"error,omitempty"`
}

// Page.ProcessID on Windows is the WebView2 browser PID. Sum that process
// and its descendants, never the user's unrelated Edge/WebView2 instances.
func processMemory(stage string, pagePID int) memorySample {
	sample := memorySample{Stage: stage, PageProcessID: pagePID}
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		sample.Error = err.Error()
		return sample
	}
	defer syscall.CloseHandle(snapshot)
	parents := make(map[uint32]uint32)
	entry := syscall.ProcessEntry32{Size: uint32(unsafe.Sizeof(syscall.ProcessEntry32{}))}
	for err := syscall.Process32First(snapshot, &entry); err == nil; err = syscall.Process32Next(snapshot, &entry) {
		parents[entry.ProcessID] = entry.ParentProcessID
	}
	if _, exists := parents[uint32(pagePID)]; !exists || pagePID == 0 {
		return sample
	}
	pids := map[uint32]bool{uint32(pagePID): true}
	for changed := true; changed; {
		changed = false
		for pid, parent := range parents {
			if pids[parent] && !pids[pid] {
				pids[pid], changed = true, true
			}
		}
	}
	getMemory := syscall.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo")
	for pid := range pids {
		if pid == 0 || pid == uint32(os.Getpid()) {
			continue
		}
		h, err := syscall.OpenProcess(0x0400|0x0010, false, pid)
		if err != nil {
			sample.Error = err.Error()
			continue
		}
		var counters struct {
			Size, PageFaultCount                                               uint32
			PeakWorkingSetSize, WorkingSetSize, QuotaPeakPagedPoolUsage        uintptr
			QuotaPagedPoolUsage, QuotaPeakNonPagedPoolUsage, QuotaNonPagedPool uintptr
			PagefileUsage, PeakPagefileUsage, PrivateUsage                     uintptr
		}
		counters.Size = uint32(unsafe.Sizeof(counters))
		ok, _, callErr := getMemory.Call(uintptr(h), uintptr(unsafe.Pointer(&counters)), uintptr(counters.Size))
		syscall.CloseHandle(h)
		if ok == 0 {
			sample.Error = callErr.Error()
			continue
		}
		sample.WebViewProcesses++
		sample.WorkingSetBytes += uint64(counters.WorkingSetSize)
		sample.PrivateBytes += uint64(counters.PrivateUsage)
	}
	return sample
}
