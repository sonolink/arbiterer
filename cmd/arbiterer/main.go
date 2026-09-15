package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/sonolink/arbiterer/internal/config"
	"github.com/sonolink/arbiterer/internal/discord"
	"github.com/sonolink/arbiterer/internal/github"
	"github.com/sonolink/arbiterer/internal/secrets"
	"github.com/sonolink/arbiterer/internal/server"
	"github.com/sonolink/arbiterer/internal/storage"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
	}

	logCfg, err := config.LoadLog()
	if err != nil {
		fmt.Fprintf(os.Stderr, "arbiterer: %v\n", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(logCfg.Handler(os.Stderr)))

	switch os.Args[1] {
	case "migrate":
		if err := runMigrate(); err != nil {
			slog.Error("migrate failed", "error", err)
			os.Exit(1)
		}
	case "serve":
		if err := runServe(); err != nil {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "arbiterer: unknown command %q\n\n", os.Args[1])
		printUsage()
	}
}

func printUsage() {
	fmt.Fprint(os.Stderr, `usage: arbiterer <command>

commands:
  migrate: apply pending database migrations
`)
	os.Exit(2)
}

func runMigrate() error {
	ctx := context.Background()

	pg, err := config.LoadPostgres()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	store, err := storage.NewStore(ctx, pg.DSN())
	if err != nil {
		return fmt.Errorf("connecting to postgres: %w", err)
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}

	slog.Info("migrations applied")
	return nil
}

func runServe() error {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}

	store, err := storage.NewStore(ctx, cfg.Postgres.DSN())
	if err != nil {
		return fmt.Errorf("connecting to postgres: %w", err)
	}
	defer store.Close()

	discordClient := discord.NewClient(cfg.Discord)

	sealer, err := secrets.NewSealer(cfg.Crypto.TokenKey)
	if err != nil {
		return fmt.Errorf("creating sealer: %w", err)
	}

	verifier := github.NewVerifier(ctx, cfg.GitHub)
	githubApp := github.NewAppClient(cfg.GitHub)

	srv := server.New(
		cfg.Server,
		slog.Default(),
		store,
		discordClient,
		sealer,
		verifier,
		githubApp,
	)
	if err := srv.Run(); err != nil {
		return fmt.Errorf("running the server: %w", err)
	}

	return nil
}
