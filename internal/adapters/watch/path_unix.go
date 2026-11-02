//go:build !windows

package watch

func watchPath(path string) (string, error) { return path, nil }
