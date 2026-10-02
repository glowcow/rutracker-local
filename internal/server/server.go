package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/glowcow/rutracker-local/internal/config"
	"github.com/glowcow/rutracker-local/internal/store"
	"github.com/glowcow/rutracker-local/web"
)

// Run starts the HTTP server and blocks until ctx is cancelled.
func Run(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz(pool))
	mux.Handle("GET /metrics", promhttp.Handler())

	txClient := newTransmissionClient(cfg)
	mux.Handle("GET /api/stats", statsHandler(pool, cfg.PeersEnabled))
	mux.Handle("GET /api/forums", forumsHandler(pool))
	mux.Handle("GET /api/search", searchHandler(pool))
	mux.Handle("GET /api/torrents/{id}", torrentHandler(pool))
	mux.Handle("GET /api/torrents/{id}/files", filesHandler(pool))
	mux.Handle("GET /api/torrents/{id}/peers", peersHandler(pool, newPeersClient(ctx, cfg, pool), cfg.PeersTTL))
	mux.Handle("POST /api/torrents/{id}/download", downloadHandler(pool, txClient))
	mux.Handle("GET /api/transmission/status", transmissionStatusHandler(txClient))

	mux.Handle("GET /api/favorites", listFavoritesHandler(pool))
	mux.Handle("POST /api/favorites/{id}", addFavoriteHandler(pool))
	mux.Handle("DELETE /api/favorites/{id}", removeFavoriteHandler(pool))
	mux.Handle("DELETE /api/favorites", clearFavoritesHandler(pool))

	// Admin — parser control. Read endpoints (dumps/runs/status/stream) are
	// open on the LAN; only the mutating start is gated by requireAdmin.
	// The parse runs on `ctx` (server lifetime) so a shutdown cancels an
	// in-flight parse.
	ctrl := newParseController(ctx, cfg)
	mux.Handle("GET /api/admin/dumps", dumpsHandler(cfg.DumpDir))
	mux.Handle("GET /api/admin/parse/runs", parseRunsHandler(pool))
	mux.Handle("GET /api/admin/parse/status", parseStatusHandler(pool, ctrl))
	mux.Handle("GET /api/admin/parse/stream", parseStreamHandler(ctrl))
	mux.Handle("POST /api/admin/parse", requireAdmin(cfg.AdminToken, parseStartHandler(ctrl, cfg.DumpDir)))

	// SPA: serve the embedded Vite dist/. The catch-all `GET /` falls back to
	// index.html for any URL that doesn't match a real file — supports
	// in-app routing on refresh without us having to know the route table.
	distFS, err := web.Dist()
	if err != nil {
		return err
	}
	mux.Handle("GET /", spaHandler(distFS))

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		// Outer→inner: accessLog → metrics → securityHeaders → crossOriginGuard
		// → gzip → apiTimeout → mux. gzip low so loggers see the real status;
		// apiTimeout innermost so the deadline covers handler work, not streaming.
		Handler: accessLog(metrics(securityHeaders(
			crossOriginGuard(gzipMiddleware(apiTimeout(mux)))))),
		ReadHeaderTimeout: 5 * time.Second,
		// A slow client mustn't hold a connection forever; Write has headroom
		// for the API deadline plus streaming the gzipped bundle over a bad link.
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Gauges refreshed every 30 s — below the ~60 s Prometheus scrape, so every
	// scrape gets fresh data without hammering the db.
	startMetricsRefresher(ctx, pool, 30*time.Second)

	// A `running` parser_runs row at boot is always an orphan (single replica
	// can't survive a restart) — mark it failed so status doesn't hang forever.
	if n, err := store.ReconcileRunningRuns(ctx, pool); err != nil {
		slog.Warn("reconcile orphaned parser runs", "err", err)
	} else if n > 0 {
		slog.Info("reconciled orphaned parser runs", "count", n)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// securityHeaders: CSP is load-bearing — torrent descriptions are untrusted
// dump HTML (sanitised by bbcode); default-src 'self' neutralises XSS if a
// sanitiser edit slips. style-src 'unsafe-inline' stays for React style props.
func securityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; object-src 'none'; base-uri 'self'; " +
		"frame-ancestors 'none'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// apiTimeout bounds /api/* handler work so a pathological query can't hold a
// pooled connection forever and starve other requests. Non-API paths pass through.
func apiTimeout(next http.Handler) http.Handler {
	const deadline = 10 * time.Second
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The SSE stream is a long-lived response — the 10 s deadline would
		// kill it; /peers can outlast 10s too (re-login + scrape). Both bound
		// their own lifetime via the request context.
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != parseStreamPath && !strings.HasSuffix(r.URL.Path, "/peers") {
			ctx, cancel := context.WithTimeout(r.Context(), deadline)
			defer cancel()
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// crossOriginGuard rejects cross-origin mutations: POST favorites is a CORS
// "simple request" any page can blind-fire, so we check Sec-Fetch-Site/Origin.
// Same-origin and header-less clients (curl) pass untouched.
func crossOriginGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		// "none" = address bar, "same-origin" = the SPA; "same-site" is rejected
		// on purpose (sibling subdomains shouldn't mutate favorites).
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" &&
			site != "same-origin" && site != "none" {
			writeJSON(w, http.StatusForbidden, errResp("cross-origin request rejected"))
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !strings.EqualFold(u.Host, r.Host) {
				writeJSON(w, http.StatusForbidden, errResp("cross-origin request rejected"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func healthz(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			// Log the detail, never echo it: pgx ping errors carry
			// host/user/database topology.
			slog.Warn("healthz ping", "err", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "degraded",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// spaHandler serves distFS; unknown paths fall back to index.html (SPA history
// mode). 404 only if index.html itself is missing (empty dist/ in tests).
func spaHandler(distFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(distFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		// Cache policy by path class. embed.FS files carry a zero ModTime,
		// so http.FileServer emits no Last-Modified/ETag validators at all —
		// without explicit Cache-Control the browser re-downloads the full
		// bundle every visit. Vite asset names are content-hashed → safe to
		// pin forever; index.html (which references them) must revalidate
		// every load or deploys wouldn't propagate; favicons are unhashed
		// but effectively static → a day.
		switch {
		case strings.HasPrefix(path, "assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case path == "index.html":
			w.Header().Set("Cache-Control", "no-cache")
		default:
			w.Header().Set("Cache-Control", "public, max-age=86400")
		}
		if _, err := fs.Stat(distFS, path); err != nil {
			f, err := distFS.Open("index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer f.Close()
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.Copy(w, f)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// accessLog logs request method/path/status/duration as one slog line.
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"dur_ms", time.Since(start).Milliseconds(),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the underlying ResponseWriter
// (through both accessLog's and metrics' statusRecorder wrappers) so the SSE
// handler can Flush and clear its write deadline.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
