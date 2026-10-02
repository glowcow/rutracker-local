package main

import (
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/glowcow/rutracker-local/internal/config"
	"github.com/glowcow/rutracker-local/internal/db"
	"github.com/glowcow/rutracker-local/internal/server"
)

func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the HTTP API",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			// Idempotent — re-runs on every boot, no-op once at head.
			// Keeps fresh installs / new replicas from racing a separate
			// migration job, and keeps prod aligned with whatever the
			// embedded migrations FS says.
			slog.Info("running migrations")
			if err := db.MigrateUp(ctx, cfg.DatabaseURL); err != nil {
				return err
			}

			pool, err := db.Connect(ctx, cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer pool.Close()

			slog.Info("starting server", "addr", cfg.HTTPAddr)
			return server.Run(ctx, cfg, pool)
		},
	}
}
