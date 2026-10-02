package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glowcow/rutracker-local/internal/config"
	"github.com/glowcow/rutracker-local/internal/ingest"
)

func TestRequireAdmin(t *testing.T) {
	const token = "s3cret-token"
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

	cases := []struct {
		name       string
		token      string
		authHeader string
		want       int
	}{
		{"empty token disables endpoint", "", "Bearer " + token, http.StatusServiceUnavailable},
		{"missing header", token, "", http.StatusUnauthorized},
		{"wrong token", token, "Bearer nope", http.StatusUnauthorized},
		{"no bearer prefix", token, token, http.StatusUnauthorized},
		{"correct token", token, "Bearer " + token, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := requireAdmin(tc.token, ok)
			req := httptest.NewRequest(http.MethodPost, "/api/admin/parse", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			h(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestParseStartSingleFlight(t *testing.T) {
	ctrl := newParseController(context.Background(), config.Config{})
	// Simulate a run already in flight — start() must refuse without launching
	// anything (so no DB is touched).
	ctrl.running.Store(true)

	h := parseStartHandler(ctrl, "/dumps")
	req := httptest.NewRequest(http.MethodPost, "/api/admin/parse", strings.NewReader(`{"sweep":false}`))
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (409 while running)", rec.Code, http.StatusConflict)
	}
}

func TestParseHubReplayAndLive(t *testing.T) {
	h := newParseHub()
	h.publish(ingest.Event{Kind: "status", Status: "running"})
	h.publish(ingest.Event{Kind: "log", Msg: "parse start"})

	replay, ch, cancel := h.subscribe()
	defer cancel()

	if len(replay) != 2 {
		t.Fatalf("replay len = %d, want 2", len(replay))
	}

	// A new event after subscribing must arrive live.
	h.publish(ingest.Event{Kind: "log", Msg: "parse done"})
	select {
	case ev := <-ch:
		if ev.Msg != "parse done" {
			t.Fatalf("live event msg = %q, want %q", ev.Msg, "parse done")
		}
	default:
		t.Fatal("expected a live event on the subscriber channel")
	}

	// reset clears the buffer for the next run; a fresh subscriber sees nothing.
	h.reset()
	replay2, _, cancel2 := h.subscribe()
	defer cancel2()
	if len(replay2) != 0 {
		t.Fatalf("replay after reset = %d, want 0", len(replay2))
	}
}
