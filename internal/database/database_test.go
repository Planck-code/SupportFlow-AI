package database

import (
	"context"
	"io"
	"net"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4/source/file"

	"github.com/Planck-code/SupportFlow-AI/internal/config"
)

func testConfig() config.Database {
	return config.Database{Host: "127.0.0.1", Port: 3306, User: "test-user", Password: "test-only:@/?&", Name: "supportflow_test", Timeout: time.Second, MaxOpenConns: 3, MaxIdleConns: 1, ConnMaxLifetime: time.Minute}
}

func TestDSNRoundTrip(t *testing.T) {
	cfg := testConfig()
	parsed, err := drivermysql.ParseDSN(dsn(cfg, false))
	if err != nil {
		t.Fatal("cannot parse generated DSN")
	}
	if parsed.User != cfg.User || parsed.Passwd != cfg.Password || parsed.DBName != cfg.Name || parsed.Addr != "127.0.0.1:3306" {
		t.Fatal("DSN lost connection fields")
	}
	if !parsed.ParseTime || parsed.Loc != time.UTC || parsed.MultiStatements || parsed.Timeout != cfg.Timeout || parsed.ReadTimeout != cfg.Timeout || parsed.WriteTimeout != cfg.Timeout || parsed.Params["charset"] != "utf8mb4" || parsed.Params["time_zone"] != "'+00:00'" {
		t.Fatal("DSN options incorrect")
	}
	parsed, err = drivermysql.ParseDSN(dsn(cfg, true))
	if err != nil || !parsed.MultiStatements {
		t.Fatal("migration connection needs multi-statements")
	}
}

func TestOpenUnavailableDatabase(t *testing.T) {
	// Reserve and release a local port to exercise the connection failure path.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	cfg := testConfig()
	cfg.Port = port
	db, err := Open(cfg)
	if err != nil {
		t.Fatal("pool construction should not require a live server")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if pool.Stats().MaxOpenConnections != cfg.MaxOpenConns {
		t.Fatal("connection pool limit not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pool.PingContext(ctx); err == nil {
		t.Fatal("unavailable database unexpectedly passed ping")
	}
}

func TestBaselineMigrationSource(t *testing.T) {
	abs, err := filepath.Abs("../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	u := (&url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}).String()
	source, err := (&file.File{}).Open(u)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	version, err := source.First()
	if err != nil || version != 1 {
		t.Fatalf("first version=%d, error=%v", version, err)
	}
	for _, up := range []bool{true, false} {
		var reader io.ReadCloser
		if up {
			reader, _, err = source.ReadUp(version)
		} else {
			reader, _, err = source.ReadDown(version)
		}
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(reader)
		reader.Close()
		if readErr != nil || len(body) == 0 {
			t.Fatal("cannot read baseline migration")
		}
	}
}
