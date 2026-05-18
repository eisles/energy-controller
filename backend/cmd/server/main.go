package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eisles/energy-controller/backend/internal/api"
	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/mock"
	"github.com/eisles/energy-controller/backend/internal/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := config.Load()
	if !cfg.MockMode || !cfg.SimulationMode || cfg.EnableRealControl {
		logger.Warn("unsafe mode flags detected; Phase 1 does not implement real device control",
			"mockMode", cfg.MockMode,
			"simulationMode", cfg.SimulationMode,
			"enableRealControl", cfg.EnableRealControl,
		)
	}

	db, err := store.Open(cfg.DBPath)
	if err != nil {
		logger.Error("failed to initialize sqlite", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	statusProvider := mock.NewStatusProvider(cfg.Clock, cfg.ControlSettings, cfg.SimulationMode, cfg.EnableRealControl)
	router := api.NewRouter(api.Dependencies{
		Config:         cfg,
		DB:             db,
		StatusProvider: statusProvider,
		Logger:         logger,
	})

	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("energy controller server started", "addr", server.Addr, "frontendDir", cfg.FrontendDir)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}
}
