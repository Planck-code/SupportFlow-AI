package database

import (
	"database/sql"
	"net/url"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/Planck-code/SupportFlow-AI/internal/config"
)

// NewMigrator owns a separate pool; Close must be called by the command or test.
func NewMigrator(cfg config.Database, directory string) (*migrate.Migrate, error) {
	abs, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	source := (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()
	pool, err := sql.Open("mysql", dsn(cfg, true))
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(2)
	pool.SetMaxIdleConns(2)
	pool.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	driver, err := migratemysql.WithInstance(pool, &migratemysql.Config{DatabaseName: cfg.Name})
	if err != nil {
		pool.Close()
		return nil, err
	}
	runner, err := migrate.NewWithDatabaseInstance(source, "mysql", driver)
	if err != nil {
		driver.Close()
		return nil, err
	}
	return runner, nil
}
