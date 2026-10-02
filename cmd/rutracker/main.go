package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var version = "dev" // overridden by -ldflags at build time

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	ctx, cancel := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT, syscall.SIGTERM,
	)
	defer cancel()

	root := &cobra.Command{
		Use:          "rutracker",
		Short:        "rutracker.local — local rutracker XML browser",
		Version:      version,
		SilenceUsage: true, // don't dump usage on RunE error
	}
	root.AddCommand(serveCmd(), parseCmd(), migrateCmd())

	if err := root.ExecuteContext(ctx); err != nil {
		slog.Error("command failed", "err", err)
		os.Exit(1)
	}
}
