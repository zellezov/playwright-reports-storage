package staticfiles

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var files embed.FS

// FS returns an http.FileSystem serving the embedded static files.
// Used to serve processing.html via http.FileServer.
func FS() http.FileSystem {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}

// Files returns the raw embedded filesystem so callers can parse templates.
// Files are accessible under the "static/" prefix, e.g. "static/failed.html".
func Files() embed.FS {
	return files
}
