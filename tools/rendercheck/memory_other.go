//go:build !windows

package main

type memorySample struct {
	Stage         string `json:"stage"`
	PageProcessID int    `json:"page_process_id"`
	Error         string `json:"error"`
}

func processMemory(stage string, pagePID int) memorySample {
	return memorySample{stage, pagePID, "S2 memory sampler is Windows-only; measure on target OS"}
}
