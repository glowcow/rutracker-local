package main

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/glowcow/rutracker-local/internal/config"
	"github.com/glowcow/rutracker-local/internal/ingest"
)

// parseCmd is a thin CLI wrapper over ingest.Run — kept for local/offline runs
// (the homelab triggers a parse through the in-app admin endpoint instead).
// Progress goes to slog; no stream emitter here.
func parseCmd() *cobra.Command {
	var (
		source     string
		batchSize  int
		numWorkers int
		sweep      bool
	)
	cmd := &cobra.Command{
		Use:   "parse",
		Short: "Stream a rutracker XML dump into Postgres",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if source == "" {
				return errors.New("--source is required")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			_, err = ingest.Run(cmd.Context(), cfg.DatabaseURL, ingest.Options{
				Source:     source,
				BatchSize:  batchSize,
				NumWorkers: numWorkers,
				Sweep:      sweep,
			}, nil)
			return err
		},
	}
	cmd.Flags().StringVar(&source, "source", "", "path or shell glob (newest mtime wins) to the dump (.xml, .xml.xz, .xml.gz)")
	cmd.Flags().IntVar(&batchSize, "batch-size", 500, "rows per COPY batch")
	cmd.Flags().IntVar(&numWorkers, "workers", 4, "parallel COPY workers (each takes one pgx connection)")
	cmd.Flags().BoolVar(&sweep, "sweep", false, "after the parse, delete torrents that no longer appear in the new dump")
	return cmd
}
