package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Planck-code/SupportFlow-AI/internal/auth"
	"github.com/Planck-code/SupportFlow-AI/internal/config"
	"github.com/Planck-code/SupportFlow-AI/internal/database"
	"github.com/Planck-code/SupportFlow-AI/internal/middleware"
	"github.com/Planck-code/SupportFlow-AI/internal/repository"
	"github.com/Planck-code/SupportFlow-AI/internal/router"
	"github.com/Planck-code/SupportFlow-AI/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	secret, err := config.LoadJWT()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	tokens, err := auth.NewJWT(secret)
	if err != nil {
		logger.Error("cannot initialize JWT signing")
		os.Exit(1)
	}
	gin.SetMode(gin.ReleaseMode)
	dbCfg, err := config.LoadDatabase()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	db, err := database.Open(dbCfg)
	if err != nil {
		logger.Error("cannot initialize database pool")
		os.Exit(1)
	}
	pool, err := db.DB()
	if err != nil {
		logger.Error("cannot access database pool")
		os.Exit(1)
	}
	defer pool.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	err = pool.PingContext(ctx)
	cancel()
	if err != nil {
		logger.Warn("database unavailable; readiness will return 503")
	}
	users := repository.NewUsers(db)
	registration := service.NewRegistration(users)
	login, err := service.NewLogin(users, tokens)
	if err != nil {
		logger.Error("cannot initialize login service")
		os.Exit(1)
	}
	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           router.New(logger, pool, registration, login, middleware.Authenticate(tokens, users)),
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("starting HTTP server", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		logger.Error("HTTP server stopped", "error", err)
		os.Exit(1)
	}
}
