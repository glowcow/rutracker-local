package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/glowcow/rutracker-local/internal/transmission"
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
	for _, testCase := range cases {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(testCase.method, testCase.path, nil))
		if rec.Code != testCase.code {
			t.Errorf("%s %s: status %d, want %d", testCase.method, testCase.path, rec.Code, testCase.code)
		}
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, testCase.ctype) {
			t.Errorf("%s %s: content type %q, want %s", testCase.method, testCase.path, got, testCase.ctype)
		}
		if testCase.code == http.StatusNotFound {
			var body map[string]string
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
				t.Errorf("%s %s: body %q is not a JSON error", testCase.method, testCase.path, rec.Body.String())
			}
		}
	}
}

// The button's name comes from the server only when the feature is on.
func TestTransmissionStatusCarriesTheLabel(t *testing.T) {
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer daemon.Close()

	cases := []struct {
		name   string
		client *transmission.Client
		label  string
		want   string
	}{
		{"off", nil, "torrwheel", `{"configured":false,"online":false}`},
		{"on, named", transmission.New(daemon.URL, "", "", ""), "torrwheel", `{"configured":true,"online":true,"label":"torrwheel"}`},
		{"on, unnamed", transmission.New(daemon.URL, "", "", ""), "", `{"configured":true,"online":true}`},
	}
	for _, testCase := range cases {
		rec := httptest.NewRecorder()
		transmissionStatusHandler(testCase.client, testCase.label)(rec, httptest.NewRequest(http.MethodGet, "/api/transmission/status", nil))
		if got := strings.TrimSpace(rec.Body.String()); got != testCase.want {
			t.Errorf("%s: got %s, want %s", testCase.name, got, testCase.want)
		}
	}
}
