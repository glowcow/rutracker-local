package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/glowcow/rutracker-local/internal/config"
	"github.com/glowcow/rutracker-local/internal/db"
	"github.com/glowcow/rutracker-local/internal/server"
)

var version = "dev" // overridden by -ldflags at build time

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected argument %q: the binary takes no commands\n", flag.Arg(0))
		os.Exit(2)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	if err := run(ctx); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Idempotent: a no-op once at head, so every boot aligns the schema
	// with the embedded migrations.
	slog.Info("running migrations")
	if err := db.MigrateUp(ctx, cfg.DatabaseURL); err != nil {
		return err
	}

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	slog.Info("starting server", "addr", cfg.HTTPAddr, "version", version)
	return server.Run(ctx, cfg, pool)
}
