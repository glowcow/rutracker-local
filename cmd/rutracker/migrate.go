package main

import (
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/glowcow/rutracker-local/internal/config"
	"github.com/glowcow/rutracker-local/internal/db"
)

func migrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "up",
		Short: "Apply all pending migrations",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			slog.Info("running migrations up")
			return db.MigrateUp(cmd.Context(), cfg.DatabaseURL)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return db.MigrateStatus(cmd.Context(), cfg.DatabaseURL)
		},
	})

	return cmd
}
