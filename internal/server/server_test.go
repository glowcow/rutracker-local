package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// An unknown /api path must not fall through to the SPA's index.html.
func TestUnknownAPIPathIsJSON404(t *testing.T) {
	dist := fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}}
	mux := http.NewServeMux()
	mux.Handle("GET /api/stats", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]int{"torrents_total": 1})
	}))
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		mux.Handle(m+" /api/", apiNotFound())
	}
	mux.Handle("GET /", spaHandler(dist))

	cases := []struct {
		method, path string
		code         int
		ctype        string
	}{
		{http.MethodGet, "/api/nope", http.StatusNotFound, "application/json"},
		{http.MethodGet, "/api/", http.StatusNotFound, "application/json"},
		{http.MethodPost, "/api/nope", http.StatusNotFound, "application/json"},
		{http.MethodDelete, "/api/torrents", http.StatusNotFound, "application/json"},
		{http.MethodGet, "/api/stats", http.StatusOK, "application/json"},
		{http.MethodGet, "/some/page", http.StatusOK, "text/html"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != c.code {
			t.Errorf("%s %s: status %d, want %d", c.method, c.path, rec.Code, c.code)
		}
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, c.ctype) {
			t.Errorf("%s %s: content type %q, want %s", c.method, c.path, got, c.ctype)
		}
		if c.code == http.StatusNotFound {
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
				t.Errorf("%s %s: body %q is not a JSON error", c.method, c.path, rec.Body.String())
			}
		}
	}
}
