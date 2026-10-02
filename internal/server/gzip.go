package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// Hand-rolled gzip middleware — ~80 lines beats a dependency for a single
// content-encoding. Biggest wins: /api/forums (~70 KB JSON → ~8 KB) and the
// JS bundle (~480 KB → ~150 KB), both previously served raw.

// gzipPool recycles writers across requests — each gzip.Writer holds ~1 MB
// of window buffers, which would otherwise be reallocated per response.
// BestSpeed: at LAN sizes the ratio difference vs DefaultCompression is a
// few percent, the CPU difference is ~3×.
var gzipPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		return w
	},
}

// compressibleTypes is a prefix whitelist — fonts (woff2) and images are
// already compressed and only burn CPU for negative gains.
var compressibleTypes = []string{
	"application/json",
	"text/html",
	"text/css",
	"text/javascript",
	"application/javascript",
	"image/svg+xml",
	"text/plain",
}

func isCompressible(contentType string) bool {
	for _, t := range compressibleTypes {
		if strings.HasPrefix(contentType, t) {
			return true
		}
	}
	return false
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The SSE stream must reach the raw ResponseWriter unbuffered — the
		// gzip wrapper would both buffer chunks and block ResponseController's
		// Flush. Skip it entirely for that path.
		if r.URL.Path == parseStreamPath {
			next.ServeHTTP(w, r)
			return
		}
		// Range responses must not be transformed (the offsets refer to the
		// uncompressed file), and a client that didn't ask gets identity.
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") ||
			r.Header.Get("Range") != "" {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}

// gzipResponseWriter defers the compress-or-not decision until the response
// headers are final (first Write / WriteHeader): only then are Content-Type
// and Content-Length known.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz      *gzip.Writer // non-nil once compression is enabled
	decided bool
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	g.decide(code)
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.decided {
		// Mirror net/http's implicit-200 path, including its content
		// sniffing — decide() needs a Content-Type to look at.
		if g.Header().Get("Content-Type") == "" {
			g.Header().Set("Content-Type", http.DetectContentType(b))
		}
		g.decide(http.StatusOK)
	}
	if g.gz != nil {
		return g.gz.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

func (g *gzipResponseWriter) decide(code int) {
	if g.decided {
		return
	}
	g.decided = true
	h := g.Header()
	// Already-encoded responses (promhttp does its own gzip negotiation)
	// and bodyless statuses pass through untouched.
	if h.Get("Content-Encoding") != "" ||
		code == http.StatusNoContent || code == http.StatusNotModified {
		return
	}
	if !isCompressible(h.Get("Content-Type")) {
		return
	}
	// Tiny bodies grow under gzip. Only skippable when the size is known
	// up front (FileServer sets it; streamed JSON doesn't).
	if cl := h.Get("Content-Length"); cl != "" {
		if n, err := strconv.Atoi(cl); err == nil && n < 1024 {
			return
		}
	}
	h.Del("Content-Length")
	h.Set("Content-Encoding", "gzip")
	h.Add("Vary", "Accept-Encoding")
	gz := gzipPool.Get().(*gzip.Writer)
	gz.Reset(g.ResponseWriter)
	g.gz = gz
}

func (g *gzipResponseWriter) close() {
	if g.gz != nil {
		_ = g.gz.Close()
		gzipPool.Put(g.gz)
		g.gz = nil
	}
}
