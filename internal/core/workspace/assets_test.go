package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSetFiles(t *testing.T) {
	for _, tt := range []struct {
		name      string
		dir       string
		copy      bool
		absolute  bool
		filePath  string
		imagePath string
		actualDir string
	}{
		{"copy to new directory", "assets", true, false, "assets/a%20b.jpg", "assets/paste.png", "assets"},
		{"link with filename template", "${filename}.pics", false, false, "src/a%20b.jpg", "my%20doc.pics/paste.png", "my doc.pics"},
		{"copy to absolute directory", "images", true, true, "images/a%20b.jpg", "images/paste.png", "images"},
		{"link with absolute directory", "images", false, true, "src/a%20b.jpg", "images/paste.png", "images"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			src := filepath.Join(root, "src", "a b.jpg")
			if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(src, []byte("jpg"), 0o600); err != nil {
				t.Fatal(err)
			}
			doc := filepath.Join(root, "my doc.md")
			dir := tt.dir
			if tt.absolute {
				dir = filepath.Join(root, dir)
			}
			a := NewAssets("old", !tt.copy)
			a.SetFiles(dir, tt.copy)
			paths, err := a.ImportFiles(doc, []string{src})
			if err != nil || !reflect.DeepEqual(paths, []string{tt.filePath}) {
				t.Fatalf("ImportFiles = %q, %v; want %q", paths, err, tt.filePath)
			}
			image, err := a.ImportImage(doc, []byte("png"), "paste")
			if err != nil || image != tt.imagePath {
				t.Fatalf("ImportImage = %q, %v; want %q", image, err, tt.imagePath)
			}
			if data, err := os.ReadFile(filepath.Join(root, tt.actualDir, "paste.png")); err != nil || string(data) != "png" {
				t.Fatalf("pasted image = %q, %v", data, err)
			}
			if tt.copy {
				if data, err := os.ReadFile(filepath.Join(root, tt.actualDir, "a b.jpg")); err != nil || string(data) != "jpg" {
					t.Fatalf("copied image = %q, %v", data, err)
				}
			}
			if _, err := os.Stat(filepath.Join(root, "old")); !os.IsNotExist(err) {
				t.Fatalf("old directory exists or cannot be checked: %v", err)
			}
		})
	}
}

func TestSetFilesDuringImport(t *testing.T) {
	for _, tt := range []struct {
		name string
		copy bool
		want string
	}{{"disable copying", false, "src/first.jpg"}, {"change directory", true, "new/first.jpg"}} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			src := filepath.Join(root, "src")
			if err := os.Mkdir(src, 0o755); err != nil {
				t.Fatal(err)
			}
			files := []string{filepath.Join(src, "first.jpg"), filepath.Join(src, "second.jpg")}
			for _, path := range files {
				if err := os.WriteFile(path, []byte("image"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			doc := filepath.Join(root, "doc.md")
			a := NewAssets("old", true)
			entered, release := make(chan struct{}), make(chan struct{})
			var blockOnce, releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			type result struct {
				paths []string
				err   error
			}
			resultCh := make(chan result, 1)
			go func() {
				paths, err := a.importFiles(doc, files, func(path string) (*os.File, error) {
					blockOnce.Do(func() { close(entered); <-release })
					return os.Open(path)
				})
				resultCh <- result{paths, err}
			}()
			select {
			case <-entered:
			case got := <-resultCh:
				t.Fatalf("import ended before opening a file: %v", got.err)
			case <-time.After(20 * time.Second):
				t.Fatal("import did not reach the first file")
			}
			updated := make(chan struct{})
			go func() { a.SetFiles("new", tt.copy); close(updated) }()
			select {
			case <-updated:
			case <-time.After(20 * time.Second):
				unblock()
				t.Fatal("SetFiles blocked on the import's file I/O")
			}
			unblock()
			select {
			case got := <-resultCh:
				if want := []string{"old/first.jpg", "old/second.jpg"}; got.err != nil || !reflect.DeepEqual(got.paths, want) {
					t.Fatalf("in-flight import = %q, %v; want %q", got.paths, got.err, want)
				}
			case <-time.After(20 * time.Second):
				t.Fatal("import did not finish")
			}
			paths, err := a.ImportFiles(doc, files[:1])
			if err != nil || !reflect.DeepEqual(paths, []string{tt.want}) {
				t.Fatalf("next ImportFiles = %q, %v; want %q", paths, err, tt.want)
			}
			image, err := a.ImportImage(doc, []byte("png"), "paste")
			if err != nil || image != "new/paste.png" {
				t.Fatalf("next ImportImage = %q, %v", image, err)
			}
		})
	}
}

func TestSetFilesConcurrentImports(t *testing.T) {
	const workers, rounds, fileCount = 4, 16, 3
	root := t.TempDir()
	var files [workers][rounds][]string
	for worker := range workers {
		src := filepath.Join(root, fmt.Sprintf("worker-%d", worker), "src")
		if err := os.MkdirAll(src, 0o755); err != nil {
			t.Fatal(err)
		}
		for round := range rounds {
			for index := range fileCount {
				path := filepath.Join(src, fmt.Sprintf("%d-%d.jpg", round, index))
				if err := os.WriteFile(path, []byte("image"), 0o600); err != nil {
					t.Fatal(err)
				}
				files[worker][round] = append(files[worker][round], path)
			}
		}
	}
	a := NewAssets("alpha", true)
	start, ready, stop, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		<-start
		a.SetFiles("alpha", true)
		close(ready)
		settings := []struct {
			dir  string
			copy bool
		}{{"alpha", true}, {"beta", false}, {"gamma", true}}
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			setting := settings[i%len(settings)]
			a.SetFiles(setting.dir, setting.copy)
			runtime.Gosched()
		}
	}()
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for worker := range workers {
		wg.Go(func() {
			<-start
			<-ready
			doc := filepath.Join(root, fmt.Sprintf("worker-%d", worker), "doc.md")
			for round := range rounds {
				paths, err := a.ImportFiles(doc, files[worker][round])
				if err != nil || len(paths) != fileCount {
					errs <- fmt.Errorf("ImportFiles = %q, %v", paths, err)
					return
				}
				dir, _, _ := strings.Cut(paths[0], "/")
				if dir != "alpha" && dir != "src" && dir != "gamma" {
					errs <- fmt.Errorf("unexpected file directory in %q", paths)
					return
				}
				for index, path := range paths {
					if want := dir + "/" + filepath.Base(files[worker][round][index]); path != want {
						errs <- fmt.Errorf("mixed settings in import: %q", paths)
						return
					}
				}
				hint := fmt.Sprintf("pasted-%d", round)
				image, err := a.ImportImage(doc, []byte("png"), hint)
				dir, _, _ = strings.Cut(image, "/")
				if err != nil || (dir != "alpha" && dir != "beta" && dir != "gamma") || image != dir+"/"+hint+".png" {
					errs <- fmt.Errorf("ImportImage = %q, %v", image, err)
					return
				}
			}
		})
	}
	close(start)
	wg.Wait()
	close(stop)
	<-stopped
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
