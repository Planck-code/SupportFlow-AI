package main

import (
	"errors"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"

	"github.com/Planck-code/SupportFlow-AI/internal/config"
	"github.com/Planck-code/SupportFlow-AI/internal/database"
)

func main() { os.Exit(run()) }

func run() int {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "version") {
		logger.Error("usage: go run ./cmd/migrate up|version (from project root)")
		return 2
	}
	cfg, err := config.LoadDatabase()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		return 1
	}
	runner, err := database.NewMigrator(cfg, "migrations")
	if err != nil {
		// Driver errors may contain database details. Never print a DSN or password.
		logger.Error("cannot initialize migrations; check database access and migration files")
		return 1
	}
	defer runner.Close()
	if os.Args[1] == "version" {
		version, dirty, err := runner.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			logger.Info("no migration applied")
			return 0
		}
		if err != nil {
			logger.Error("cannot read migration version")
			return 1
		}
		logger.Info("migration version", "version", version, "dirty", dirty)
		if dirty {
			return 1
		}
		return 0
	}
	if err := runner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		logger.Error("migration failed; inspect schema_migrations before retrying, do not force a version")
		return 1
	}
	logger.Info("migrations are up to date")
	return 0
}
