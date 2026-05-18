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
	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/ecoflow"
	"github.com/eisles/energy-controller/backend/internal/mock"
	"github.com/eisles/energy-controller/backend/internal/nature"
	"github.com/eisles/energy-controller/backend/internal/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := config.Load()
	if !cfg.SimulationMode || cfg.EnableRealControl {
		logger.Warn("unsafe mode flags detected; real device control is not implemented in this phase",
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

	statusProvider := newStatusProvider(cfg)
	statusRepository := store.NewStatusRepository(db)
	logRepository := store.NewLogRepository(db)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	recordStatus(ctx, statusProvider, statusRepository, logRepository, logger)
	go runControlLoop(ctx, cfg.PollInterval, statusProvider, statusRepository, logRepository, logger)

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

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown failed", "error", err)
		os.Exit(1)
	}
}

type statusWriter interface {
	UpdateCurrentStatus(ctx context.Context, status domain.Status) error
}

type logWriter interface {
	InsertPowerLog(ctx context.Context, log domain.PowerLog) error
}

func newStatusProvider(cfg config.Config) api.StatusProvider {
	if cfg.MockMode {
		return mock.NewStatusProvider(cfg.Clock, cfg.ControlSettings, cfg.SimulationMode, cfg.EnableRealControl)
	}
	ecoflowClient := ecoflow.NewSignedClient(ecoflow.Config{
		AccessKey: cfg.EcoFlowAccessKey,
		SecretKey: cfg.EcoFlowSecretKey,
		DeviceSN:  cfg.EcoFlowDeviceSN,
		BaseURL:   cfg.EcoFlowBaseURL,
	})
	if cfg.NatureMode == "cloud" {
		natureClient := nature.NewCloudClient(nature.CloudConfig{
			AccessToken: cfg.NatureAccessToken,
			ApplianceID: cfg.NatureApplianceID,
		})
		return mock.NewStatusProviderWithReaders(cfg.Clock, cfg.ControlSettings, cfg.SimulationMode, cfg.EnableRealControl, natureClient, ecoflowClient, "nature-cloud+ecoflow-read")
	}
	return mock.NewStatusProviderWithReaders(cfg.Clock, cfg.ControlSettings, cfg.SimulationMode, cfg.EnableRealControl, nil, ecoflowClient, "ecoflow-read")
}

func runControlLoop(ctx context.Context, interval time.Duration, provider api.StatusProvider, statusRepository statusWriter, logRepository logWriter, logger *slog.Logger) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			recordStatus(ctx, provider, statusRepository, logRepository, logger)
		}
	}
}

func recordStatus(ctx context.Context, provider api.StatusProvider, statusRepository statusWriter, logRepository logWriter, logger *slog.Logger) {
	status, err := provider.CurrentStatus(ctx)
	if err != nil {
		logger.Error("failed to evaluate current status", "error", err)
		return
	}
	if err := logRepository.InsertPowerLog(ctx, powerLogFromStatus(status)); err != nil {
		logger.Error("failed to save power log", "error", err)
		return
	}
	if err := statusRepository.UpdateCurrentStatus(ctx, status); err != nil {
		logger.Error("failed to update current status", "error", err)
		return
	}
	logger.Info("control decision saved", "mode", status.Mode, "state", status.State, "gridW", status.GridW, "targetChargeW", status.TargetChargeW, "reason", status.LastDecisionReason)
}

func powerLogFromStatus(status domain.Status) domain.PowerLog {
	return domain.PowerLog{
		MeasuredAt:     status.UpdatedAt,
		GridW:          status.GridW,
		ImportW:        status.ImportW,
		ExportW:        status.ExportW,
		BatterySoc:     intPtr(status.BatterySoc),
		BatteryInputW:  intPtr(status.BatteryInputW),
		BatteryOutputW: intPtr(status.BatteryOutputW),
		ACChargeLimitW: intPtr(status.ACChargeLimitW),
		TargetChargeW:  status.TargetChargeW,
		ActualCommandW: nil,
		DecisionReason: status.LastDecisionReason,
		Mode:           status.Mode,
		CommandSent:    false,
		ErrorMessage:   status.LastError,
		CreatedAt:      status.UpdatedAt,
	}
}

func intPtr(value int) *int {
	return &value
}
