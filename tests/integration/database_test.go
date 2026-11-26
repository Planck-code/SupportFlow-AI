//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"gorm.io/gorm"

	"github.com/Planck-code/SupportFlow-AI/internal/config"
	"github.com/Planck-code/SupportFlow-AI/internal/database"
)

const latestVersion = 5

// Each test owns an initially empty dedicated schema. Never run these tests in parallel.
func testDatabase(t *testing.T) (*gorm.DB, *migrate.Migrate) {
	t.Helper()
	name := os.Getenv("TEST_DATABASE_NAME")
	if !regexp.MustCompile(`^[A-Za-z0-9_]+_test$`).MatchString(name) {
		t.Fatal("set TEST_DATABASE_NAME to an EMPTY dedicated database ending in _test; integration validation was not performed")
	}
	cfg, err := config.LoadDatabase()
	if err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(name, cfg.Name) {
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
	t.Cleanup(func() { pool.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := pool.PingContext(ctx); err != nil {
		t.Fatal("real MySQL connection failed")
	}
	var actual string
	if err := db.WithContext(ctx).Raw("SELECT DATABASE()").Scan(&actual).Error; err != nil || actual != name {
		t.Fatal("unexpected selected database")
	}
	var count int
	if err := db.WithContext(ctx).Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&count).Error; err != nil {
		t.Fatal("cannot inspect test schema")
	}
	if count != 0 {
		t.Fatal("test database must be empty; refusing to change existing tables")
	}
	// Only after verifying schema identity AND emptiness may we own the tables created by our migrations.
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		for _, statement := range []string{"DROP TABLE IF EXISTS ai_runs", "DROP TABLE IF EXISTS ticket_messages", "DROP TABLE IF EXISTS ticket_events", "DROP TABLE IF EXISTS tickets", "DROP TABLE IF EXISTS users", "DROP TABLE IF EXISTS schema_migrations"} {
			if err := db.WithContext(cleanupCtx).Exec(statement).Error; err != nil {
				t.Error("cannot clean owned test tables; inspect test schema before rerunning")
			}
		}
	})
	runner, err := database.NewMigrator(cfg, "../../migrations")
	if err != nil {
		t.Fatal("cannot initialize real MySQL migrations")
	}
	t.Cleanup(func() {
		sourceErr, dbErr := runner.Close()
		if sourceErr != nil || dbErr != nil {
			t.Error("cannot close migrator")
		}
	})
	return db, runner
}

func assertVersion(t *testing.T, runner *migrate.Migrate, want uint) {
	t.Helper()
	version, dirty, err := runner.Version()
	if err != nil || dirty || version != want {
		t.Fatalf("migration version=%d dirty=%v err=%v, want clean %d", version, dirty, err, want)
	}
}

func TestDatabaseAndMigrations(t *testing.T) {
	db, runner := testDatabase(t)
	var charset, timezone string
	if err := db.Raw("SELECT @@character_set_connection").Scan(&charset).Error; err != nil {
		t.Fatal("cannot inspect charset")
	}
	if err := db.Raw("SELECT @@session.time_zone").Scan(&timezone).Error; err != nil {
		t.Fatal("cannot inspect timezone")
	}
	if charset != "utf8mb4" || timezone != "+00:00" {
		t.Fatal("unexpected session settings")
	}
	if err := runner.Steps(1); err != nil {
		t.Fatal("baseline up failed")
	}
	assertVersion(t, runner, 1)
	if db.Migrator().HasTable("users") {
		t.Fatal("baseline unexpectedly created users")
	}
	if err := runner.Steps(1); err != nil {
		t.Fatal("users migration up failed")
	}
	assertVersion(t, runner, 2)
	if !db.Migrator().HasTable("users") {
		t.Fatal("users table missing")
	}
	if err := runner.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, runner, 3)
	if !db.Migrator().HasTable("tickets") || !db.Migrator().HasTable("ticket_events") {
		t.Fatal("missing ticket tables")
	}
	if err := runner.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, runner, 4)
	if !db.Migrator().HasTable("ticket_messages") || !db.Migrator().HasIndex("ticket_messages", "idx_messages_ticket") {
		t.Fatal("missing messages table or pagination index")
	}
	var foreignKeys int
	if err := db.Raw(`SELECT COUNT(*) FROM information_schema.key_column_usage
		WHERE table_schema = DATABASE() AND table_name = 'ticket_messages'
		  AND referenced_table_schema = DATABASE() AND referenced_column_name = 'id'
		  AND (column_name, referenced_table_name) IN (('ticket_id','tickets'), ('author_id','users'))`).Scan(&foreignKeys).Error; err != nil || foreignKeys != 2 {
		t.Fatal("missing message foreign keys")
	}
	if err := runner.Steps(1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, runner, 5)
	if !db.Migrator().HasTable("ai_runs") || !db.Migrator().HasIndex("ai_runs", "idx_ai_runs_ticket") {
		t.Fatal("missing AI schema")
	}
	if err := runner.Up(); !errors.Is(err, migrate.ErrNoChange) {
		t.Fatal("expected no change at version 5")
	}
	if err := runner.Steps(-1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, runner, 4)
	if db.Migrator().HasTable("ai_runs") || !db.Migrator().HasTable("ticket_messages") {
		t.Fatal("AI down did not preserve v4 schema")
	}

	if err := runner.Steps(-1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, runner, 3)
	if db.Migrator().HasTable("ticket_messages") || !db.Migrator().HasTable("tickets") {
		t.Fatal("message down did not preserve ticket schema")
	}
	if err := runner.Steps(-1); err != nil {
		t.Fatal(err)
	}
	assertVersion(t, runner, 2)
	if db.Migrator().HasTable("tickets") || db.Migrator().HasTable("ticket_events") {
		t.Fatal("ticket down left tables")
	}
	if err := runner.Steps(-1); err != nil {
		t.Fatal("users down failed")
	}
	assertVersion(t, runner, 1)
	if db.Migrator().HasTable("users") {
		t.Fatal("users down left a table behind")
	}
	if err := runner.Up(); err != nil {
		t.Fatal("users re-up failed")
	}
	assertVersion(t, runner, latestVersion)
	if err := runner.Down(); err != nil {
		t.Fatal("full down failed")
	}
	if _, _, err := runner.Version(); !errors.Is(err, migrate.ErrNilVersion) {
		t.Fatal("expected no applied version after down")
	}
	if err := runner.Up(); err != nil {
		t.Fatal("full re-up failed")
	}
	assertVersion(t, runner, latestVersion)
}
