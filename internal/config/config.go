// Package config reads the RT_ environment, and POSTGRES_* for the database.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the whole runtime configuration.
type Config struct {
	DatabaseURL string
	HTTPAddr    string
	// AdminToken gates starting a parse; empty: it cannot be started.
	AdminToken string
	// DumpDir is where the admin panel lists dumps from, and where a bare dump
	// name picked in the UI is resolved. Matches the container's /dumps mount.
	DumpDir string

	// Seeders and leechers, scraped from the topic page; off while RuTracker
	// sits behind a Cloudflare challenge, and nothing goes out.
	PeersEnabled bool
	// The forum login; either empty keeps the peers off.
	RutrackerUser string
	RutrackerPass string
	// RutrackerBaseURL is the site the peers are scraped from.
	RutrackerBaseURL string
	// PeersTTL keeps a torrent's counts fresh; opened later, it is scraped
	// again, at most once a day by default.
	PeersTTL time.Duration
	// PeersMinInterval spaces all scrape requests, 2 s by default.
	PeersMinInterval time.Duration

	// The download button; an empty TransmissionRPCURL hides it.
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

	// Warnings are problems that only switched the peers off; run logs them.
	Warnings []error
}

// Load reads the process environment.
func Load() (Config, error) { return LoadFrom(os.Getenv) }

// LoadFrom reads through getenv and reports every problem; the DSN is
// RT_DATABASE_URL, else built from the postgres image's POSTGRES_*.
func LoadFrom(getenv func(string) string) (Config, error) {
	var errs, warnings []error
	str := func(k, fallback string) string {
		if v := getenv(k); v != "" {
			return v
		}
		return fallback
	}
	// A bad peers setting warns and keeps the peers off: a typo there must
	// not take search down (operator, 2026-08-01).
	peersOK := true
	boolean := func(k string, fallback bool) bool {
		v := getenv(k)
		if v == "" {
			return fallback
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			warnings = append(warnings, fmt.Errorf("%s: want true or false, got %q; peers off", k, v))
			peersOK = false
		}
		return b
	}
	duration := func(k string, fallback time.Duration) time.Duration {
		v := getenv(k)
		if v == "" {
			return fallback
		}
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			warnings = append(warnings, fmt.Errorf("%s: want a positive duration like 24h, got %q; peers off", k, v))
			peersOK = false
			return fallback
		}
		return d
	}

	dbURL := getenv("RT_DATABASE_URL")
	if dbURL == "" {
		user, pass, db := getenv("POSTGRES_USER"), getenv("POSTGRES_PASSWORD"), getenv("POSTGRES_DB")
		if user == "" || pass == "" || db == "" {
			errs = append(errs, errors.New("RT_DATABASE_URL, or POSTGRES_USER, POSTGRES_PASSWORD and POSTGRES_DB, are required"))
		} else {
			// url.UserPassword escapes a password like p@ss/word#1.
			dbURL = (&url.URL{
				Scheme:   "postgres",
				User:     url.UserPassword(user, pass),
				Host:     str("POSTGRES_HOST", "rt_postgres") + ":" + str("POSTGRES_PORT", "5432"),
				Path:     "/" + db,
				RawQuery: "sslmode=" + str("POSTGRES_SSLMODE", "disable"),
			}).String()
		}
	}

	cfg := Config{
		DatabaseURL: dbURL,
		HTTPAddr:    str("RT_HTTP_ADDR", ":8080"),
		AdminToken:  getenv("RT_ADMIN_TOKEN"),
		DumpDir:     str("RT_DUMP_DIR", "/dumps"),

		PeersEnabled:     boolean("RT_PEERS_ENABLED", false),
		RutrackerUser:    getenv("RT_USER"),
		RutrackerPass:    getenv("RT_PASS"),
		RutrackerBaseURL: str("RT_BASE_URL", "https://rutracker.org"),
		PeersTTL:         duration("RT_PEERS_TTL", 24*time.Hour),
		PeersMinInterval: duration("RT_PEERS_MIN_INTERVAL", 2*time.Second),

		TransmissionRPCURL: getenv("RT_TRANSMISSION_RPC_URL"),
		TransmissionHost:   getenv("RT_TRANSMISSION_HOST"),
		TransmissionUser:   getenv("RT_TRANSMISSION_USER"),
		TransmissionPass:   getenv("RT_TRANSMISSION_PASS"),
		TransmissionLabel:  strings.TrimSpace(getenv("RT_TRANSMISSION_LABEL")),
	}
	cfg.PeersEnabled = cfg.PeersEnabled && peersOK
	cfg.Warnings = warnings
	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}
