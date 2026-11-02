package main

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Planck-code/SupportFlow-AI/internal/config"
	"github.com/Planck-code/SupportFlow-AI/internal/router"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	gin.SetMode(gin.ReleaseMode)
	server := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           router.New(logger),
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("starting HTTP server", "addr", server.Addr)
	if err := server.ListenAndServe(); err != nil {
		logger.Error("HTTP server stopped", "error", err)
		os.Exit(1)
	}
}
