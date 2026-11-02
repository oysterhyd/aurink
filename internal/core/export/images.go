package export

import (
	"encoding/base64"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func (r *renderer) imageURL(dest string) string {
	dest = safeURL(dest, true)
	if dest == "" || !r.opts.EmbedImages {
		return dest
	}
	path := dest
	if strings.HasPrefix(path, "//") || strings.HasPrefix(path, `\\`) {
		return dest
	}
	if filepath.IsAbs(path) && !strings.HasPrefix(path, "/") {
		decoded, err := url.PathUnescape(path)
		if err != nil {
			return dest
		}
		path = decoded
	} else {
		u, err := url.Parse(dest)
		if err != nil {
			return dest
		}
		if strings.EqualFold(u.Scheme, "file") && (u.Host == "" || strings.EqualFold(u.Host, "localhost")) {
			path = filepath.FromSlash(u.Path)
			// file:///C:/... has one extra slash on Windows.
			if len(path) > 2 && path[0] == filepath.Separator && filepath.VolumeName(path[1:]) != "" {
				path = path[1:]
			}
		} else {
			if u.IsAbs() || u.Host != "" || strings.HasPrefix(dest, "//") {
				return dest
			}
			path = u.Path
		}
	}
	if path == "" {
		return dest
	}
	base := r.opts.BaseDir
	if base == "" {
		base = "."
	}
	base, err := filepath.Abs(base)
	if err != nil {
		return dest
	}
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		return dest
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, filepath.FromSlash(path))
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return dest
	}
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return dest
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return dest
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return dest
	}
	kind := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if i := strings.IndexByte(kind, ';'); i >= 0 {
		kind = kind[:i]
	}
	if !strings.HasPrefix(kind, "image/") {
		kind = http.DetectContentType(data)
	}
	if !strings.HasPrefix(kind, "image/") {
		return dest
	}
	return "data:" + kind + ";base64," + base64.StdEncoding.EncodeToString(data)
}
