package config

import (
	"strings"
	"testing"
	"time"
)

// env is a getenv over a map.
func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDatabaseURLWins(t *testing.T) {
	cfg, err := LoadFrom(env(map[string]string{
		"RT_DATABASE_URL": "postgres://override:pw@somewhere:5432/db?sslmode=disable",
		"POSTGRES_USER":   "ignored", "POSTGRES_PASSWORD": "ignored", "POSTGRES_DB": "ignored",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg.DatabaseURL, "override:pw@somewhere") {
		t.Errorf("DatabaseURL = %q, want the explicit one", cfg.DatabaseURL)
	}
}

func TestLoadBuildsFromPGVars(t *testing.T) {
	cfg, err := LoadFrom(env(map[string]string{
		"POSTGRES_USER": "rutracker", "POSTGRES_PASSWORD": "s3cret", "POSTGRES_DB": "rutracker",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := "postgres://rutracker:s3cret@rt_postgres:5432/rutracker?sslmode=disable"
	if cfg.DatabaseURL != want {
		t.Errorf("DatabaseURL = %q\nwant %q", cfg.DatabaseURL, want)
	}
}

func TestLoadEscapesPassword(t *testing.T) {
	cfg, err := LoadFrom(env(map[string]string{
		"POSTGRES_USER": "rutracker", "POSTGRES_PASSWORD": `p@ss/word#1?&`, "POSTGRES_DB": "rutracker",
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"@ss/word", "#1?&"} {
		if strings.Contains(cfg.DatabaseURL, raw) {
			t.Errorf("password not escaped, URL = %q", cfg.DatabaseURL)
		}
	}
	if !strings.HasPrefix(cfg.DatabaseURL, "postgres://rutracker:") {
		t.Errorf("URL malformed: %q", cfg.DatabaseURL)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := LoadFrom(env(map[string]string{"RT_DATABASE_URL": "postgres://u:p@h:5432/d", "RT_TRANSMISSION_LABEL": " Torrwheel "}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.DumpDir != "/dumps" || cfg.PeersEnabled ||
		cfg.PeersTTL != 24*time.Hour || cfg.PeersMinInterval != 2*time.Second || cfg.TransmissionLabel != "Torrwheel" {
		t.Errorf("defaults %+v", cfg)
	}
}

// The peers layer is off unless RT_PEERS_ENABLED says true.
func TestLoadPeersEnabled(t *testing.T) {
	for v, want := range map[string]bool{"": false, "true": true, "1": true, "false": false} {
		cfg, err := LoadFrom(env(map[string]string{"RT_DATABASE_URL": "postgres://u:p@h:5432/d", "RT_PEERS_ENABLED": v}))
		if err != nil || cfg.PeersEnabled != want {
			t.Errorf("RT_PEERS_ENABLED=%q: %v %v, want %v", v, cfg.PeersEnabled, err, want)
		}
	}
}

func TestLoadJoinsProblems(t *testing.T) {
	_, err := LoadFrom(env(map[string]string{
		"RT_PEERS_ENABLED": "nonsense", "RT_PEERS_TTL": "24", "RT_PEERS_MIN_INTERVAL": "-1s",
	}))
	if err == nil {
		t.Fatal("a broken config loaded")
	}
	for _, want := range []string{"RT_DATABASE_URL, or POSTGRES_USER", "RT_PEERS_ENABLED", "RT_PEERS_TTL", "RT_PEERS_MIN_INTERVAL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%v\nlacks %q", err, want)
		}
	}
}
