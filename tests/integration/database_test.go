//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"

	"github.com/Planck-code/SupportFlow-AI/internal/config"
	"github.com/Planck-code/SupportFlow-AI/internal/database"
)

func TestDatabaseAndMigrations(t *testing.T) {
	name := os.Getenv("TEST_DATABASE_NAME")
	if !strings.HasSuffix(name, "_test") {
		t.Fatal("set TEST_DATABASE_NAME to an EMPTY dedicated database ending in _test; integration validation was not performed")
	}
	cfg, err := config.LoadDatabase()
	if err != nil {
		t.Fatal(err)
	}
	if name == cfg.Name {
		t.Fatal("test database must differ from application database")
	}
	cfg.Name = name
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatal("cannot initialize GORM test pool")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := pool.PingContext(ctx); err != nil {
		t.Fatal("real MySQL connection failed; database validation was not performed")
	}
	var count int
	if err := db.WithContext(ctx).Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&count).Error; err != nil {
		t.Fatal("cannot inspect test schema")
	}
	if count != 0 {
		t.Fatal("test database must be empty; refusing to change existing tables")
	}
	var charset, timezone string
	if err := db.WithContext(ctx).Raw("SELECT @@character_set_connection").Scan(&charset).Error; err != nil {
		t.Fatal("cannot inspect charset")
	}
	if err := db.WithContext(ctx).Raw("SELECT @@session.time_zone").Scan(&timezone).Error; err != nil {
		t.Fatal("cannot inspect timezone")
	}
	if charset != "utf8mb4" || timezone != "+00:00" {
		t.Fatalf("unexpected session settings charset=%s timezone=%s", charset, timezone)
	}
	runner, err := database.NewMigrator(cfg, "../../migrations")
	if err != nil {
		t.Fatal("cannot initialize real MySQL migrations")
	}
	defer runner.Close()
	// The schema was empty; only the migration metadata table is owned by this test.
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		if err := db.WithContext(cleanupCtx).Exec("DROP TABLE IF EXISTS schema_migrations").Error; err != nil {
			t.Error("cannot clean migration metadata; inspect test database before rerunning")
		}
	}()
	if err := runner.Up(); err != nil {
		t.Fatalf("migration up failed: %T", err)
	}
	version, dirty, err := runner.Version()
	if err != nil || dirty || version != 1 {
		t.Fatal("expected clean migration version 1")
	}
	if err := runner.Up(); !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal("repeated up should report no change")
	}
	if err := runner.Down(); err != nil {
		t.Fatal("baseline down failed")
	}
	if _, _, err := runner.Version(); !errors.Is(err, migrate.ErrNilVersion) {
		t.Fatal("expected no applied version after down")
	}
	if err := runner.Up(); err != nil {
		t.Fatal("baseline up after down failed")
	}
}
