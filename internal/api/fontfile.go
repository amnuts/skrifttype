package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/amnuts/skrifttype/internal/library"
	"github.com/amnuts/skrifttype/internal/store"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// fontFilePrefix is the asset-server path under which catalogued font
// files are served to the webview, as /fontfile/<faceID>.
const fontFilePrefix = "/fontfile/"

// FontFileMiddleware serves catalogued font files so the frontend can
// register them with the FontFace API and preview fonts that are not
// installed. Only files present in the fonts table are reachable, and
// only by face ID — the webview can never name an arbitrary path.
func FontFileMiddleware(st *store.Store) application.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rest, ok := strings.CutPrefix(r.URL.Path, fontFilePrefix)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			serveFontFile(w, r, st, rest)
		})
	}
}

func serveFontFile(w http.ResponseWriter, r *http.Request, st *store.Store, idStr string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	f, err := st.GetFontByID(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(f.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	if f.Format != "ttc" && f.Format != "otc" {
		w.Header().Set("Content-Type", fontContentType(f.Format))
		http.ServeContent(w, r, "", info.ModTime(), file)
		return
	}
	// Browsers cannot load a single face out of a collection, so serve
	// the face as a standalone sfnt.
	face, err := faceCache.get(faceKey{id: id, mtime: info.ModTime().UnixNano(), size: info.Size()}, func() ([]byte, error) {
		data, err := io.ReadAll(file)
		if err != nil {
			return nil, err
		}
		return library.ExtractFace(data, f.FaceIndex)
	})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", fontContentType(strings.TrimPrefix(library.OutlineExt(face), ".")))
	http.ServeContent(w, r, "", info.ModTime(), bytes.NewReader(face))
}

func fontContentType(format string) string {
	switch format {
	case "otf":
		return "font/otf"
	case "woff":
		return "font/woff"
	case "woff2":
		return "font/woff2"
	case "otc":
		return "font/otf"
	case "ttc":
		return "font/ttf"
	default:
		return "font/ttf"
	}
}
