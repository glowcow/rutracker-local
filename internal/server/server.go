// Package server serves the UI, the JSON API, the parser panel and /metrics.
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
	mux.Handle("GET /api/transmission/status", transmissionStatusHandler(txClient, cfg.TransmissionLabel))

	mux.Handle("GET /api/favorites", listFavoritesHandler(pool))
	mux.Handle("POST /api/favorites/{id}", addFavoriteHandler(pool))
	mux.Handle("DELETE /api/favorites/{id}", removeFavoriteHandler(pool))
	mux.Handle("DELETE /api/favorites", clearFavoritesHandler(pool))

	// The parser panel: only starting a parse needs the token; the parse
	// runs on ctx, so a shutdown cancels it.
	ctrl := newParseController(ctx, cfg)
	mux.Handle("GET /api/admin/dumps", dumpsHandler(cfg.DumpDir))
	mux.Handle("GET /api/admin/parse/runs", parseRunsHandler(pool))
	mux.Handle("GET /api/admin/parse/status", parseStatusHandler(pool, ctrl))
	mux.Handle("GET /api/admin/parse/stream", parseStreamHandler(ctrl))
	mux.Handle("POST /api/admin/parse", requireAdmin(cfg.AdminToken, parseStartHandler(ctrl, cfg.DumpDir)))

	// Any other /api path is a JSON 404, not the SPA; per method, as a bare
	// "/api/" conflicts with "GET /".
	for _, m := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		mux.Handle(m+" /api/", apiNotFound())
	}

	// Unknown paths get index.html, so the SPA's routes survive a reload.
	if distFS, ok := web.Dist(); ok {
		mux.Handle("GET /", spaHandler(distFS))
	} else {
		slog.Warn("ui not built into this binary, serving the api only")
	}

	srv := &http.Server{
		Addr: cfg.HTTPAddr,
		// Outermost first; gzip sits low so the loggers see the real status,
		// apiTimeout covers the handler's work, not the streaming.
		Handler: accessLog(metrics(securityHeaders(
			crossOriginGuard(gzipMiddleware(apiTimeout(mux)))))),
		ReadHeaderTimeout: 5 * time.Second,
		// Write leaves room for the API deadline and a slow bundle download.
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

	slog.Info("listening", "addr", cfg.HTTPAddr)
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

// securityHeaders sets the CSP that contains XSS should the bbcode sanitiser
// slip on a dump's description; inline styles stay for React.
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

// apiTimeout bounds /api/* work, so a pathological query cannot hold a pooled
// connection.
func apiTimeout(next http.Handler) http.Handler {
	const deadline = 10 * time.Second
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The event stream and /peers (a re-login and a scrape) outlive 10 s
		// and bound themselves through the request's context.
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != parseStreamPath && !strings.HasSuffix(r.URL.Path, "/peers") {
			ctx, cancel := context.WithTimeout(r.Context(), deadline)
			defer cancel()
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// crossOriginGuard rejects cross-origin mutations, which any page could fire
// as simple requests; same-origin and header-less clients pass.
func crossOriginGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		// "none" is the address bar; "same-site" is refused, so a sibling
		// subdomain cannot change favourites.
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

// apiNotFound answers an unknown /api path with a JSON 404.
func apiNotFound() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusNotFound, errResp("not found"))
	})
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
		// embed.FS has no ModTime, so no validators: hashed assets are
		// immutable, index.html revalidates, the rest keeps a day.
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
			defer func() { _ = f.Close() }()
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

// Unwrap lets http.ResponseController reach the writer below the recorders,
// so the event stream can flush and clear its deadline.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
