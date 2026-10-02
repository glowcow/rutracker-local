package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/glowcow/rutracker-local/internal/ingest"
	"github.com/glowcow/rutracker-local/internal/store"
)

// parseStreamPath is the SSE endpoint. Named so the middleware chain can exempt
// it from the /api/* request-deadline and gzip buffering (a long-lived stream
// must not be capped at 10 s or buffered by the gzip writer).
const parseStreamPath = "/api/admin/parse/stream"

// requireAdmin gates a handler behind the shared-secret bearer token. Empty
// configured token → 503 (fail-closed). Used only on the mutating endpoint
// (POST /api/admin/parse); read endpoints stay open (LAN-only).
func requireAdmin(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if token == "" {
			writeJSON(w, http.StatusServiceUnavailable, errResp("admin endpoint disabled: RT_ADMIN_TOKEN not set"))
			return
		}
		const prefix = "Bearer "
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, prefix) {
			writeJSON(w, http.StatusUnauthorized, errResp("unauthorized"))
			return
		}
		got := strings.TrimPrefix(auth, prefix)
		if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			writeJSON(w, http.StatusUnauthorized, errResp("unauthorized"))
			return
		}
		next(w, r)
	}
}

type dumpJSON struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	Mtime     string `json:"mtime"` // RFC3339
}

func isDumpName(n string) bool {
	return strings.HasSuffix(n, ".xml") ||
		strings.HasSuffix(n, ".xml.xz") ||
		strings.HasSuffix(n, ".xml.gz")
}

// dumpsHandler lists dump files in the mount for the panel's picker. A missing
// or unreadable dir is not an error — it returns an empty list so the panel
// shows "no dumps" (e.g. local dev without the mount).
func dumpsHandler(dumpDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entries, err := os.ReadDir(dumpDir)
		if err != nil {
			slog.Warn("admin: read dump dir", "dir", dumpDir, "err", err)
			writeJSON(w, http.StatusOK, map[string]any{"items": []dumpJSON{}})
			return
		}
		out := make([]dumpJSON, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() || !isDumpName(e.Name()) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			out = append(out, dumpJSON{
				Name:      e.Name(),
				SizeBytes: info.Size(),
				Mtime:     info.ModTime().UTC().Format(time.RFC3339),
			})
		}
		// Newest mtime first — RFC3339 sorts lexically.
		sort.Slice(out, func(i, j int) bool { return out[i].Mtime > out[j].Mtime })
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}
}

type runJSON struct {
	ID         int64   `json:"id"`
	StartedAt  string  `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
	Source     string  `json:"source"`
	Rows       int64   `json:"rows"`
	Swept      int64   `json:"swept"`
	Sweep      bool    `json:"sweep"`
	Status     string  `json:"status"`
	DurationS  int64   `json:"duration_s"`
}

func toRunJSON(r store.ParserRun) runJSON {
	rj := runJSON{
		ID:        r.ID,
		StartedAt: r.StartedAt.UTC().Format(time.RFC3339),
		Source:    filepath.Base(r.Source),
		Rows:      r.RowsInserted,
		Swept:     r.RowsSwept,
		Sweep:     r.Sweep,
		Status:    r.Status,
	}
	if r.FinishedAt != nil {
		f := r.FinishedAt.UTC().Format(time.RFC3339)
		rj.FinishedAt = &f
		rj.DurationS = int64(r.FinishedAt.Sub(r.StartedAt).Seconds())
	}
	return rj
}

// parseRunsHandler → the "recent loads" list. Open read.
func parseRunsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 3
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				limit = min(n, 20)
			}
		}
		runs, err := store.RecentParserRuns(r.Context(), pool, limit)
		if err != nil {
			slog.Warn("admin: recent runs", "err", err)
			writeJSON(w, http.StatusInternalServerError, errResp("failed to load recent runs"))
			return
		}
		out := make([]runJSON, 0, len(runs))
		for _, run := range runs {
			out = append(out, toRunJSON(run))
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	}
}

// parseStatusHandler → current run state. `running` is the in-process
// single-flight flag (authoritative for this process); `run` is the latest
// parser_runs row (may be in-flight or the last completed one). Open read.
func parseStatusHandler(pool *pgxpool.Pool, ctrl *parseController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"running": ctrl.isRunning(), "run": nil}
		run, err := store.LatestParserRun(r.Context(), pool)
		if err == nil {
			resp["run"] = toRunJSON(run)
		} else if !errors.Is(err, store.ErrNotFound) {
			slog.Warn("admin: latest run", "err", err)
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// parseStreamHandler streams the current run's events over SSE. On connect it
// replays the hub's buffered log (full run so far), then streams live with a
// periodic heartbeat. Open read. Relies on being exempt from apiTimeout/gzip
// (see server.Run) and clears the per-connection write deadline itself.
func parseStreamHandler(ctrl *parseController) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering

		rc := http.NewResponseController(w)
		// A long-lived stream must not be killed by the server WriteTimeout.
		_ = rc.SetWriteDeadline(time.Time{})

		replay, ch, cancel := ctrl.hub.subscribe()
		defer cancel()

		writeEvent := func(ev ingest.Event) bool {
			b, err := json.Marshal(ev)
			if err != nil {
				return true // skip a bad event, keep the stream open
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
				return false
			}
			return rc.Flush() == nil
		}

		for _, ev := range replay {
			if !writeEvent(ev) {
				return
			}
		}

		hb := time.NewTicker(20 * time.Second)
		defer hb.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				if !writeEvent(ev) {
					return
				}
			case <-hb.C:
				if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
					return
				}
				if rc.Flush() != nil {
					return
				}
			}
		}
	}
}

type startReq struct {
	Source    string `json:"source"`
	BatchSize int    `json:"batch_size"`
	Sweep     bool   `json:"sweep"`
	Workers   int    `json:"workers"`
}

// parseStartHandler kicks off a parse. Gated by requireAdmin. Single-flight →
// 409 if one is already running; otherwise 202 (the parse runs in the
// background, observed via /status and /stream). The dump name is constrained
// to dumpDir via filepath.Base so a request can't point the parser elsewhere.
func parseStartHandler(ctrl *parseController, dumpDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req startReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, errResp("invalid json body"))
			return
		}

		name := strings.TrimSpace(req.Source)
		if name == "" {
			name = "rutracker-*.xml.xz" // default glob → newest mtime wins
		}
		// Base() strips any directory components (incl. ../) so the source is
		// always inside dumpDir. Preserves glob metacharacters.
		source := filepath.Join(dumpDir, filepath.Base(name))

		opts := ingest.Options{
			Source:     source,
			BatchSize:  req.BatchSize,
			NumWorkers: req.Workers,
			Sweep:      req.Sweep,
		}
		if !ctrl.start(opts) {
			writeJSON(w, http.StatusConflict, errResp("a parse is already running"))
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "started"})
	}
}
