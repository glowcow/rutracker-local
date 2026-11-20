package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// gzipPool reuses the writers, each about 1 MB of window; BestSpeed costs a
// few percent of ratio and saves three times the CPU.
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
		// The event stream must reach the writer unbuffered for its flushes.
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

// gzipResponseWriter decides at the first write, when Content-Type and
// Content-Length are known.
type gzipResponseWriter struct {
	http.ResponseWriter
	gz      *gzip.Writer // non-nil once compression is enabled
	decided bool
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	g.decide(code)
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(data []byte) (int, error) {
	if !g.decided {
		// Mirror net/http's implicit-200 path, including its content
		// sniffing — decide() needs a Content-Type to look at.
		if g.Header().Get("Content-Type") == "" {
			g.Header().Set("Content-Type", http.DetectContentType(data))
		}
		g.decide(http.StatusOK)
	}
	if g.gz != nil {
		return g.gz.Write(data)
	}
	return g.ResponseWriter.Write(data)
}

func (g *gzipResponseWriter) decide(code int) {
	if g.decided {
		return
	}
	g.decided = true
	header := g.Header()
	// Already-encoded responses (promhttp does its own gzip negotiation)
	// and bodyless statuses pass through untouched.
	if header.Get("Content-Encoding") != "" ||
		code == http.StatusNoContent || code == http.StatusNotModified {
		return
	}
	if !isCompressible(header.Get("Content-Type")) {
		return
	}
	// Tiny bodies grow under gzip. Only skippable when the size is known
	// up front (FileServer sets it; streamed JSON doesn't).
	if cl := header.Get("Content-Length"); cl != "" {
		if n, err := strconv.Atoi(cl); err == nil && n < 1024 {
			return
		}
	}
	header.Del("Content-Length")
	header.Set("Content-Encoding", "gzip")
	header.Add("Vary", "Accept-Encoding")
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
