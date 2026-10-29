// Command rutracker serves a local search over the RuTracker dump: it applies
// the migrations, then serves the UI, the API and the parser panel.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/glowcow/rutracker-local/internal/config"
	"github.com/glowcow/rutracker-local/internal/db"
	"github.com/glowcow/rutracker-local/internal/server"
)

// Set at build time through -ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	showVersion := flag.Bool("version", false, "print version, commit and build date, and exit")
	flag.Parse()
	if *showVersion {
		fmt.Printf("rutracker\n  version:    %s\n  commit:     %s\n  build date: %s\n", version, commit, date)
		return
	}
	if flag.NArg() > 0 {
		slog.Error("fatal", "err", fmt.Errorf("unexpected argument %q: the binary takes no commands", flag.Arg(0)))
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	slog.Info("starting", "version", version, "commit", commit, "date", date)
	prometheus.MustRegister(buildInfo())
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	// Every boot brings the schema to the embedded migrations; at head a no-op.
	slog.Info("running migrations")
	if err := db.MigrateUp(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	return server.Run(ctx, cfg, pool)
}

// buildInfo is rutracker_build_info, the version a PromQL query can read.
func buildInfo() prometheus.Collector {
	info := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "rutracker_build_info",
		Help: "rutracker build information; constant 1.",
	}, []string{"version", "commit", "build_date"})
	info.WithLabelValues(version, commit, date).Set(1)
	return info
}
