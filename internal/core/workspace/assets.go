package workspace

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"aurink/internal/contract"
)

// Assets imports images using a snapshot of the file settings.
type Assets struct {
	mu         sync.RWMutex
	imageDir   string
	copyImages bool
}

var _ contract.Assets = (*Assets)(nil)

// NewAssets creates an importer. Import methods block and must run off the UI thread.
func NewAssets(imageDir string, copyImages bool) *Assets {
	return &Assets{imageDir: imageDir, copyImages: copyImages}
}

// SetFiles updates the settings for subsequent imports. In-flight imports keep
// their initial settings, and SetFiles can be called while they run.
func (a *Assets) SetFiles(imageDir string, copyImages bool) {
	a.mu.Lock()
	a.imageDir, a.copyImages = imageDir, copyImages
	a.mu.Unlock()
}

func (a *Assets) fileSettings() (string, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.imageDir, a.copyImages
}

// ImportFiles returns Markdown paths to the supplied images.
func (a *Assets) ImportFiles(docPath string, files []string) ([]string, error) {
	return a.importFiles(docPath, files, os.Open)
}

func (a *Assets) importFiles(docPath string, files []string, open func(string) (*os.File, error)) ([]string, error) {
	imageDir, copyImages := a.fileSettings()
	if docPath == "" {
		return nil, failure(contract.CodeInvalid, "assets.ImportFiles", docPath, nil)
	}
	var dir string
	if copyImages {
		var err error
		dir, err = assetDirectory(docPath, imageDir)
		if err != nil {
			return nil, err
		}
	}
	paths := make([]string, 0, len(files))
	for _, path := range files {
		target := path
		if copyImages {
			f, err := open(path)
			if err != nil {
				return nil, ioFailure("assets.ImportFiles", path, err)
			}
			target, err = copyUnique(dir, filepath.Base(path), f)
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
			if err != nil {
				return nil, ioFailure("assets.ImportFiles", path, err)
			}
		}
		paths = append(paths, markdownPath(docPath, target))
	}
	return paths, nil
}

// ImportImage writes PNG data to a new file in the configured directory.
func (a *Assets) ImportImage(docPath string, png []byte, nameHint string) (string, error) {
	imageDir, _ := a.fileSettings()
	if docPath == "" {
		return "", failure(contract.CodeInvalid, "assets.ImportImage", docPath, nil)
	}
	if nameHint == "" {
		nameHint = "image"
	}
	if !validName(nameHint) {
		return "", failure(contract.CodeInvalid, "assets.ImportImage", nameHint, nil)
	}
	dir, err := assetDirectory(docPath, imageDir)
	if err != nil {
		return "", err
	}
	name := strings.TrimSuffix(nameHint, filepath.Ext(nameHint)) + ".png"
	path, err := copyUnique(dir, name, bytes.NewReader(png))
	if err != nil {
		return "", ioFailure("assets.ImportImage", path, err)
	}
	return markdownPath(docPath, path), nil
}

func assetDirectory(docPath, imageDir string) (string, error) {
	name := filepath.Base(docPath)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	dir := strings.ReplaceAll(imageDir, "${filename}", name)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(filepath.Dir(docPath), dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", ioFailure("assets.directory", dir, err)
	}
	return dir, nil
}

func copyUnique(dir, name string, src io.Reader) (string, error) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 0; ; n++ {
		candidate := stem + ext
		if n != 0 {
			candidate = stem + "-" + strconv.Itoa(n) + ext
		}
		path := filepath.Join(dir, candidate)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return path, err
		}
		_, err = io.Copy(f, src)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(path)
		}
		return path, err
	}
}

func markdownPath(docPath, target string) string {
	docDir, err := filepath.Abs(filepath.Dir(docPath))
	if err == nil {
		if abs, err := filepath.Abs(target); err == nil {
			target = abs
			if rel, err := filepath.Rel(docDir, abs); err == nil {
				target = rel
			}
		}
	}
	return strings.ReplaceAll(filepath.ToSlash(target), " ", "%20")
}
