package panel

import (
	"bytes"
	"embed"
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed static/*
var staticFiles embed.FS

func staticHandler(basePath string) http.Handler {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			files.ServeHTTP(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		switch name {
		case "", ".":
			servePanelHTML(w, r, sub, "index.html", basePath)
			return
		case "login", "login.html":
			servePanelHTML(w, r, sub, "login.html", basePath)
			return
		}
		if _, err := fs.Stat(sub, name); err == nil {
			files.ServeHTTP(w, r)
			return
		}
		if path.Ext(name) == "" || acceptsHTML(r) {
			servePanelHTML(w, r, sub, "index.html", basePath)
			return
		}
		http.NotFound(w, r)
	})
}

func servePanelHTML(w http.ResponseWriter, r *http.Request, files fs.FS, name, basePath string) {
	data, err := fs.ReadFile(files, name)
	if err != nil {
		http.Error(w, "panel UI is unavailable", http.StatusInternalServerError)
		return
	}
	normalized := normalizeBasePath(basePath)
	baseHref := normalized + "/"
	if baseHref == "" {
		baseHref = "/"
	}
	runtime := fmt.Sprintf(
		`<base href="%s"><meta name="tapx-base-path" content="%s">`,
		html.EscapeString(baseHref),
		html.EscapeString(normalized),
	)
	data = bytes.Replace(data, []byte("</head>"), []byte(runtime+"</head>"), 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		return
	}
	_, _ = w.Write(data)
}

func acceptsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}
