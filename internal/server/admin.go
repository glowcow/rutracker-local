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

// parseStreamPath is the event stream, which apiTimeout and gzip leave alone.
const parseStreamPath = "/api/admin/parse/stream"

// requireAdmin checks the bearer token; without one configured it answers
// 503. Only starting a parse needs it.
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

// dumpsHandler lists the dumps for the panel; a missing directory is an empty
// list, as in local development.
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

// parseStatusHandler reports this process's single-flight flag and the latest
// parser_runs row.
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

// parseStreamHandler replays the run's events so far, then streams them live
// with a heartbeat and no write deadline.
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

// parseStartHandler starts a parse in the background: 202, or 409 when one
// runs; filepath.Base keeps the dump inside dumpDir.
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
