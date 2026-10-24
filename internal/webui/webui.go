package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler serving the Next.js static export.
// If webDir exists on disk with an index.html, it serves from webDir;
// otherwise it serves from the embedded distFS built into the binary.
func Handler(webDir string) http.Handler {
	if webDir != "" {
		if info, err := os.Stat(filepath.Join(webDir, "index.html")); err == nil && !info.IsDir() {
			return spaFileServer(os.DirFS(webDir))
		}
	}
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return http.NotFoundHandler()
	}
	return spaFileServer(sub)
}

func spaFileServer(root fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if upath == "" || upath == "." {
			fileServer.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(root, upath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		htmlPath := upath + ".html"
		if _, err := fs.Stat(root, htmlPath); err == nil {
			r2 := *r
			u2 := *r.URL
			u2.Path = "/" + htmlPath
			r2.URL = &u2
			fileServer.ServeHTTP(w, &r2)
			return
		}
		idxPath := path.Join(upath, "index.html")
		if _, err := fs.Stat(root, idxPath); err == nil {
			r2 := *r
			u2 := *r.URL
			u2.Path = "/" + upath + "/"
			r2.URL = &u2
			fileServer.ServeHTTP(w, &r2)
			return
		}
		r2 := *r
		u2 := *r.URL
		u2.Path = "/"
		r2.URL = &u2
		fileServer.ServeHTTP(w, &r2)
	})
}
