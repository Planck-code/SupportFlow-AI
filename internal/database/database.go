package database

import (
	"net"
	"strconv"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/Planck-code/SupportFlow-AI/internal/config"
)

func dsn(cfg config.Database, multiStatements bool) string {
	driver := drivermysql.NewConfig()
	driver.User, driver.Passwd, driver.DBName = cfg.User, cfg.Password, cfg.Name
	driver.Net, driver.Addr = "tcp", net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	driver.ParseTime, driver.Loc = true, time.UTC
	driver.Timeout, driver.ReadTimeout, driver.WriteTimeout = cfg.Timeout, cfg.Timeout, cfg.Timeout
	driver.MultiStatements = multiStatements
	driver.Collation = "utf8mb4_0900_ai_ci"
	driver.Params = map[string]string{"charset": "utf8mb4", "time_zone": "'+00:00'"}
	return driver.FormatDSN()
}

// Open constructs a pool without contacting MySQL. Readiness verifies access.
func Open(cfg config.Database) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN: dsn(cfg, false), SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Discard})
	if err != nil {
		return nil, err
	}
	pool, err := db.DB()
	if err != nil {
		return nil, err
	}
	pool.SetMaxOpenConns(cfg.MaxOpenConns)
	pool.SetMaxIdleConns(cfg.MaxIdleConns)
	pool.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	return db, nil
}
