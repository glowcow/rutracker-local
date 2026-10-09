package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/glowcow/rutracker-local/internal/config"
	"github.com/glowcow/rutracker-local/internal/store"
	"github.com/glowcow/rutracker-local/internal/transmission"
)

// newTransmissionClient builds the RPC client, or nil when no endpoint is
// configured (feature off → the button is hidden and the route 503s). The Host
// header defaults to the RPC URL's hostname (port stripped) so a plain
// RT_TRANSMISSION_RPC_URL already satisfies the daemon's rpc-host-whitelist.
func newTransmissionClient(cfg config.Config) *transmission.Client {
	if cfg.TransmissionRPCURL == "" {
		return nil
	}
	host := cfg.TransmissionHost
	if host == "" {
		if u, err := url.Parse(cfg.TransmissionRPCURL); err == nil {
			host = u.Hostname()
		}
	}
	return transmission.New(cfg.TransmissionRPCURL, host,
		cfg.TransmissionUser, cfg.TransmissionPass)
}

// transmissionStatusResponse tells the drawer whether to render the button
// (configured) and whether it's live (online). A dead endpoint greys it out
// instead of letting a click fail.
type transmissionStatusResponse struct {
	Configured bool `json:"configured"`
	Online     bool `json:"online"`
	// Label is the daemon's name for the button; absent → "Transmission".
	Label string `json:"label,omitempty"`
}

// transmissionStatusHandler reports reachability, caching the probe result for
// a short window so opening drawers doesn't ping the daemon on every request.
func transmissionStatusHandler(tc *transmission.Client, label string) http.HandlerFunc {
	const ttl = 20 * time.Second
	var (
		mu        sync.Mutex
		checkedAt time.Time
		online    bool
	)
	return func(w http.ResponseWriter, r *http.Request) {
		if tc == nil {
			writeJSON(w, http.StatusOK, transmissionStatusResponse{})
			return
		}
		mu.Lock()
		if time.Since(checkedAt) >= ttl {
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			err := tc.Ping(ctx)
			cancel()
			online = err == nil
			checkedAt = time.Now()
			if err != nil {
				slog.Debug("transmission ping", "err", err)
			}
		}
		cur := online
		mu.Unlock()
		writeJSON(w, http.StatusOK, transmissionStatusResponse{Configured: true, Online: cur, Label: label})
	}
}

// downloadResponse tells the drawer button which state to show: "added" (fresh)
// or "duplicate" (already in the queue). name is the torrent's display name.
type downloadResponse struct {
	Status string `json:"status"`
	Name   string `json:"name,omitempty"`
}

var hashRe = regexp.MustCompile(`^[0-9a-f]{40}$`)

// downloadHandler pushes a torrent's magnet into Transmission. The magnet is
// built server-side from the stored info-hash (never trusting client input);
// an unclean hash 404s just like the drawer hides the action for one.
func downloadHandler(pool *pgxpool.Pool, tc *transmission.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if tc == nil {
			writeJSON(w, http.StatusServiceUnavailable, errResp("transmission not configured"))
			return
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, errResp("invalid id"))
			return
		}

		t, err := store.Get(r.Context(), pool, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, errResp("not found"))
				return
			}
			slog.Error("download get", "err", err, "id", id)
			writeJSON(w, http.StatusInternalServerError, errResp("get failed"))
			return
		}
		if !hashRe.MatchString(strings.ToLower(t.Hash)) {
			writeJSON(w, http.StatusUnprocessableEntity, errResp("torrent has no valid hash"))
			return
		}

		name, err := tc.Add(r.Context(), buildMagnet(t.Hash, t.Title))
		if err != nil {
			if errors.Is(err, transmission.ErrDuplicate) {
				writeJSON(w, http.StatusOK, downloadResponse{Status: "duplicate", Name: name})
				return
			}
			slog.Error("transmission add", "err", err, "id", id)
			writeJSON(w, http.StatusBadGateway, errResp("transmission unavailable"))
			return
		}
		writeJSON(w, http.StatusOK, downloadResponse{Status: "added", Name: name})
	}
}

// trackers baked into every magnet — mirror of web/DetailDrawer TRACKERS.
// bt[1-4].t-ru.org are rutracker's own; the udp:// ones are open-trackers kept
// as a fallback. opentrackr/openbittorrent were dropped — unreachable from the
// daemon ("Could not connect to tracker" on every scrape).
var trackers = []string{
	"http://bt.t-ru.org/ann?magnet",
	"http://bt2.t-ru.org/ann?magnet",
	"http://bt3.t-ru.org/ann?magnet",
	"http://bt4.t-ru.org/ann?magnet",
	"udp://exodus.desync.com:6969/announce",
	"udp://tracker.torrent.eu.org:451/announce",
	"udp://open.demonii.com:1337/announce",
}

func buildMagnet(hash, title string) string {
	v := url.Values{}
	v.Set("dn", title)
	for _, tr := range trackers {
		v.Add("tr", tr)
	}
	return "magnet:?xt=urn:btih:" + hash + "&" + v.Encode()
}
