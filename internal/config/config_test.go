package config

import (
	"strings"
	"testing"
)

func TestLoad_DatabaseURLWins(t *testing.T) {
	t.Setenv("RT_DATABASE_URL", "postgres://override:pw@somewhere:5432/db?sslmode=disable")
	t.Setenv("POSTGRES_USER", "ignored")
	t.Setenv("POSTGRES_PASSWORD", "ignored")
	t.Setenv("POSTGRES_DB", "ignored")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg.DatabaseURL, "override:pw@somewhere") {
		t.Errorf("DatabaseURL = %q, want explicit override", cfg.DatabaseURL)
	}
}

func TestLoad_BuildsFromPGVars(t *testing.T) {
	t.Setenv("RT_DATABASE_URL", "")
	t.Setenv("POSTGRES_USER", "rutracker")
	t.Setenv("POSTGRES_PASSWORD", "s3cret")
	t.Setenv("POSTGRES_DB", "rutracker")
	t.Setenv("POSTGRES_HOST", "rt_postgres")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := "postgres://rutracker:s3cret@rt_postgres:5432/rutracker?sslmode=disable"
	if cfg.DatabaseURL != want {
		t.Errorf("DatabaseURL = %q\nwant %q", cfg.DatabaseURL, want)
	}
}

func TestLoad_EscapesPassword(t *testing.T) {
	t.Setenv("RT_DATABASE_URL", "")
	t.Setenv("POSTGRES_USER", "rutracker")
	t.Setenv("POSTGRES_PASSWORD", `p@ss/word#1?&`)
	t.Setenv("POSTGRES_DB", "rutracker")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	// Bare special chars must not survive — otherwise they'd corrupt the URL.
	for _, raw := range []string{"@ss/word", "#1?&"} {
		if strings.Contains(cfg.DatabaseURL, raw) {
			t.Errorf("password not escaped, URL = %q", cfg.DatabaseURL)
		}
	}
	// And it should be a parseable URL.
	if !strings.HasPrefix(cfg.DatabaseURL, "postgres://rutracker:") {
		t.Errorf("URL malformed: %q", cfg.DatabaseURL)
	}
}

// The peers layer is fail-closed: absent or unparseable RT_PEERS_ENABLED
// leaves it off, so a stale env can't silently resume scraping rutracker.
func TestLoad_PeersEnabledDefaultsOff(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want bool
	}{{"", false}, {"nonsense", false}, {"true", true}, {"1", true}, {"false", false}} {
		t.Run(tc.env, func(t *testing.T) {
			t.Setenv("RT_DATABASE_URL", "postgres://u:p@h:5432/d")
			t.Setenv("RT_PEERS_ENABLED", tc.env)

			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.PeersEnabled != tc.want {
				t.Errorf("RT_PEERS_ENABLED=%q → PeersEnabled = %v, want %v", tc.env, cfg.PeersEnabled, tc.want)
			}
		})
	}
}

func TestLoad_MissingVarsErrors(t *testing.T) {
	t.Setenv("RT_DATABASE_URL", "")
	t.Setenv("POSTGRES_USER", "")
	t.Setenv("POSTGRES_PASSWORD", "")
	t.Setenv("POSTGRES_DB", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected error when nothing set, got nil")
	}
}
