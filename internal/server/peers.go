package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/singleflight"

	"github.com/glowcow/rutracker-local/internal/config"
	"github.com/glowcow/rutracker-local/internal/peers"
	"github.com/glowcow/rutracker-local/internal/store"
)

// newPeersClient builds the scrape client, or returns nil when the feature is
// off — either not enabled (RT_PEERS_ENABLED) or no credentials. The
// last-known cookie is loaded from Postgres so a restart doesn't force a
// re-login; fresh cookies are persisted back.
func newPeersClient(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) *peers.Client {
	if !cfg.PeersEnabled || cfg.RutrackerUser == "" || cfg.RutrackerPass == "" {
		return nil
	}
	cookie, _, err := store.LoadRutrackerSession(ctx, pool)
	if err != nil {
		slog.Warn("load rutracker session", "err", err)
	}
	return peers.New(cfg.RutrackerBaseURL, cfg.RutrackerUser, cfg.RutrackerPass, cookie,
		cfg.PeersMinInterval, func(c context.Context, ck string) error {
			return store.SaveRutrackerSession(c, pool, ck)
		})
}

// peersResponse is the JSON contract the drawer consumes. seeders/leechers/
// checked_at are null when nothing is cached; error ("auth" | "unavailable")
// is set when the freshest scrape attempt failed (a stale cache may still be
// returned alongside it).
type peersResponse struct {
	Configured bool    `json:"configured"`
	Seeders    *int    `json:"seeders"`
	Leechers   *int    `json:"leechers"`
	CheckedAt  *string `json:"checked_at"`
	Error      string  `json:"error,omitempty"`
}

func peersBody(p *store.Peers, errCode string) peersResponse {
	resp := peersResponse{Configured: true, Error: errCode}
	if p != nil {
		s, l := p.Seeders, p.Leechers
		ts := p.CheckedAt.UTC().Format(time.RFC3339)
		resp.Seeders, resp.Leechers, resp.CheckedAt = &s, &l, &ts
	}
	return resp
}

// peersHandler serves cached peer counts, refreshing from rutracker when the
// cache is missing or older than ttl. pc == nil (no credentials) → the feature
// is off and we report configured:false so the UI hides the block.
func peersHandler(pool *pgxpool.Pool, pc *peers.Client, ttl time.Duration) http.HandlerFunc {
	// Collapses concurrent refreshes of the same torrent into one scrape.
	var sf singleflight.Group

	return func(w http.ResponseWriter, r *http.Request) {
		if pc == nil {
			writeJSON(w, http.StatusOK, peersResponse{Configured: false})
			return
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, errResp("invalid id"))
			return
		}

		// /peers is exempt from the global apiTimeout (a cold re-login + retry
		// can exceed 10s); bound it ourselves, still tied to the request so a
		// client disconnect cancels the scrape.
		ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
		defer cancel()

		cached, hasCache, err := store.GetPeers(ctx, pool, id)
		if err != nil {
			slog.Warn("peers cache read", "err", err, "id", id)
		}

		// Fresh enough → serve cache, no outbound request.
		if hasCache && time.Since(cached.CheckedAt) < ttl {
			writeJSON(w, http.StatusOK, peersBody(&cached, ""))
			return
		}

		// Missing/stale → confirm the id is ours, then scrape (single-flight).
		if exists, err := store.TorrentExists(ctx, pool, id); err != nil {
			slog.Error("peers exists check", "err", err, "id", id)
			writeJSON(w, http.StatusInternalServerError, errResp("lookup failed"))
			return
		} else if !exists {
			writeJSON(w, http.StatusNotFound, errResp("not found"))
			return
		}

		v, err, _ := sf.Do(strconv.FormatInt(id, 10), func() (any, error) {
			return pc.Fetch(ctx, id)
		})
		if err != nil {
			// Scrape failed — hand back the stale cache (if any) plus a reason
			// so the UI greys the numbers instead of dropping them.
			code := "unavailable"
			if errors.Is(err, peers.ErrAuth) {
				code = "auth"
			}
			var cp *store.Peers
			if hasCache {
				cp = &cached
			}
			slog.Warn("peers scrape", "err", err, "id", id)
			writeJSON(w, http.StatusOK, peersBody(cp, code))
			return
		}

		stat := v.(peers.Stat)
		if err := store.UpsertPeers(ctx, pool, id, stat.Seeders, stat.Leechers); err != nil {
			slog.Warn("peers upsert", "err", err, "id", id)
		}
		fresh := store.Peers{Seeders: stat.Seeders, Leechers: stat.Leechers, CheckedAt: time.Now()}
		writeJSON(w, http.StatusOK, peersBody(&fresh, ""))
	}
}
