package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL string
	HTTPAddr    string
	// AdminToken gates the mutating admin endpoint (POST /api/admin/parse).
	// Empty → the endpoint is disabled (fail-closed). Read-only admin
	// endpoints (dumps/status/stream/runs) are open on the LAN regardless.
	AdminToken string
	// DumpDir is where the admin panel lists dumps from, and where a bare dump
	// name picked in the UI is resolved. Matches the container's /dumps mount.
	DumpDir string

	// Peers (seeders/leechers) live layer. Off unless PeersEnabled — rutracker
	// now serves the forum behind a Cloudflare challenge, so every scrape 403s
	// until a clearance path exists. Off → the /peers endpoint reports
	// configured:false, the UI hides the row and the list badge, and nothing
	// goes out to rutracker.
	PeersEnabled bool
	// RutrackerUser+RutrackerPass are the login for scraping the topic page's
	// peer block; either empty → the feature stays off even when enabled.
	// Credentials live in .rutracker.env (server-only), never in git.
	RutrackerUser string
	RutrackerPass string
	// RutrackerBaseURL is the site root scraped for peers (default rutracker.org).
	RutrackerBaseURL string
	// PeersTTL — a torrent's cached peer counts are considered fresh for this
	// long; a detail open past it triggers one re-scrape. Default 24h (kind to
	// the account: at most one hit per torrent per day).
	PeersTTL time.Duration
	// PeersMinInterval — global spacing between outbound scrape requests (a
	// politeness rate limit). Default 2s.
	PeersMinInterval time.Duration

	// Transmission "send to download" layer. TransmissionRPCURL is the daemon's
	// RPC endpoint; empty → the feature is off (the button is hidden). No baked
	// default — the target lives in .rutracker.env (server-only), so moving the
	// daemon never means rebuilding the binary.
	TransmissionRPCURL string
	// TransmissionHost overrides the outbound Host header to satisfy the
	// daemon's rpc-host-whitelist. Empty → derived from the RPC URL's hostname.
	TransmissionHost string
	// Optional Basic auth — unset when rpc-authentication-required is false.
	TransmissionUser string
	TransmissionPass string
	// TransmissionLabel names the daemon on the drawer's button and in its
	// tooltips. Empty → the UI says "Transmission".
	TransmissionLabel string
}

// Load reads config from env. Every app-owned var is RT_-prefixed; the only
// exceptions are POSTGRES_* — the postgres image's own variable names, shared
// with the db container via one env_file. DB DSN priority: RT_DATABASE_URL,
// else built from POSTGRES_USER/PASSWORD/DB (+ optional HOST/PORT). Errors if
// neither is set.
func Load() (Config, error) {
	dbURL := os.Getenv("RT_DATABASE_URL")
	if dbURL == "" {
		built, err := buildPGURL()
		if err != nil {
			return Config{}, err
		}
		dbURL = built
	}
	return Config{
		DatabaseURL: dbURL,
		HTTPAddr:    getenv("RT_HTTP_ADDR", ":8080"),
		AdminToken:  os.Getenv("RT_ADMIN_TOKEN"),
		DumpDir:     getenv("RT_DUMP_DIR", "/dumps"),

		PeersEnabled:     getbool("RT_PEERS_ENABLED", false),
		RutrackerUser:    os.Getenv("RT_USER"),
		RutrackerPass:    os.Getenv("RT_PASS"),
		RutrackerBaseURL: getenv("RT_BASE_URL", "https://rutracker.org"),
		PeersTTL:         getdur("RT_PEERS_TTL", 24*time.Hour),
		PeersMinInterval: getdur("RT_PEERS_MIN_INTERVAL", 2*time.Second),

		TransmissionRPCURL: os.Getenv("RT_TRANSMISSION_RPC_URL"),
		TransmissionHost:   os.Getenv("RT_TRANSMISSION_HOST"),
		TransmissionUser:   os.Getenv("RT_TRANSMISSION_USER"),
		TransmissionPass:   os.Getenv("RT_TRANSMISSION_PASS"),
		TransmissionLabel:  strings.TrimSpace(os.Getenv("RT_TRANSMISSION_LABEL")),
	}, nil
}

// getbool reads a boolean (1/t/true/0/f/false) from env k, falling back on
// parse failure or absence.
func getbool(k string, fallback bool) bool {
	if v := os.Getenv(k); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

// getdur reads a Go duration string (e.g. "24h", "2s") from env k, falling
// back on parse failure or absence.
func getdur(k string, fallback time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func buildPGURL() (string, error) {
	user := os.Getenv("POSTGRES_USER")
	pass := os.Getenv("POSTGRES_PASSWORD")
	db := os.Getenv("POSTGRES_DB")
	if user == "" || pass == "" || db == "" {
		return "", fmt.Errorf(
			"either RT_DATABASE_URL must be set, or POSTGRES_USER+POSTGRES_PASSWORD+POSTGRES_DB",
		)
	}
	host := getenv("POSTGRES_HOST", "rt_postgres")
	port := getenv("POSTGRES_PORT", "5432")

	// net/url handles the password escape correctly so e.g. `p@ss/word#1`
	// doesn't get parsed as user:p@ss/word#1.
	u := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(user, pass),
		Host:     host + ":" + port,
		Path:     "/" + db,
		RawQuery: "sslmode=" + getenv("POSTGRES_SSLMODE", "disable"),
	}
	return u.String(), nil
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
