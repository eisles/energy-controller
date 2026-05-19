package main

import (
	"context"
	"database/sql"
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
	"github.com/eisles/energy-controller/backend/internal/weather"
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

	statusProvider, energyMeterReader := newStatusProvider(cfg, db)
	statusRepository := store.NewStatusRepository(db)
	logRepository := store.NewLogRepository(db)
	energyMeterRepository := store.NewEnergyMeterRepository(db)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	recordStatus(ctx, statusProvider, statusRepository, logRepository, energyMeterReader, energyMeterRepository, logger)
	go runControlLoop(ctx, cfg.PollInterval, statusProvider, statusRepository, logRepository, energyMeterReader, energyMeterRepository, logger)

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

type energyMeterWriter interface {
	InsertEnergyMeterReading(ctx context.Context, reading domain.EnergyMeterReading) error
}

type energyMeterReader interface {
	CurrentEnergyMeterReading(ctx context.Context) (domain.EnergyMeterReading, error)
}

type commandStatus struct {
	ActualCommandW *int
	CommandSent    bool
}

type commandStatusProvider interface {
	LastCommandActualW() *int
	LastCommandSent() bool
}

func newStatusProvider(cfg config.Config, db *sql.DB) (api.StatusProvider, energyMeterReader) {
	if cfg.MockMode {
		return mock.NewStatusProvider(cfg.Clock, cfg.ControlSettings, cfg.MockMode, cfg.SimulationMode, cfg.EnableRealControl, cfg.AutoControlEnabled), nil
	}
	ecoflowClient := ecoflow.NewSignedClient(ecoflow.Config{
		AccessKey: cfg.EcoFlowAccessKey,
		SecretKey: cfg.EcoFlowSecretKey,
		DeviceSN:  cfg.EcoFlowDeviceSN,
		BaseURL:   cfg.EcoFlowBaseURL,
	})
	ecoflowWriteClient := ecoflow.NewMockWriteClient()
	weatherReader := newWeatherReader(cfg, db)
	if cfg.NatureMode == "cloud" {
		natureClient := nature.NewCloudClient(nature.CloudConfig{
			AccessToken: cfg.NatureAccessToken,
			ApplianceID: cfg.NatureApplianceID,
		})
		return mock.NewStatusProviderWithReaders(cfg.Clock, cfg.ControlSettings, cfg.MockMode, cfg.SimulationMode, cfg.EnableRealControl, cfg.AutoControlEnabled, natureClient, ecoflowClient, ecoflowWriteClient, "nature-cloud+ecoflow-read", weatherReader), natureClient
	}
	return mock.NewStatusProviderWithReaders(cfg.Clock, cfg.ControlSettings, cfg.MockMode, cfg.SimulationMode, cfg.EnableRealControl, cfg.AutoControlEnabled, nil, ecoflowClient, ecoflowWriteClient, "ecoflow-read", weatherReader), nil
}

func newWeatherReader(cfg config.Config, db *sql.DB) mock.WeatherReader {
	forecastClient := weather.NewOpenMeteoClient(weather.OpenMeteoConfig{
		Latitude:  cfg.WeatherLatitude,
		Longitude: cfg.WeatherLongitude,
		Timezone:  cfg.WeatherTimezone,
		BaseURL:   cfg.WeatherBaseURL,
	})
	if db != nil {
		return weather.NewLocationForecastClient(store.NewWeatherSettingsRepository(db), forecastClient)
	}
	if !cfg.WeatherEnabled {
		return nil
	}
	return forecastClient
}

func runControlLoop(ctx context.Context, interval time.Duration, provider api.StatusProvider, statusRepository statusWriter, logRepository logWriter, meterReader energyMeterReader, meterRepository energyMeterWriter, logger *slog.Logger) {
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
			recordStatus(ctx, provider, statusRepository, logRepository, meterReader, meterRepository, logger)
		}
	}
}

func recordStatus(ctx context.Context, provider api.StatusProvider, statusRepository statusWriter, logRepository logWriter, meterReader energyMeterReader, meterRepository energyMeterWriter, logger *slog.Logger) {
	status, err := provider.CurrentStatus(ctx)
	if err != nil {
		logger.Error("failed to evaluate current status", "error", err)
		return
	}
	if err := logRepository.InsertPowerLog(ctx, powerLogFromStatus(status, lastCommandStatus(provider))); err != nil {
		logger.Error("failed to save power log", "error", err)
		return
	}
	if err := statusRepository.UpdateCurrentStatus(ctx, status); err != nil {
		logger.Error("failed to update current status", "error", err)
		return
	}
	recordEnergyMeterReading(ctx, meterReader, meterRepository, logger)
	logger.Info("control decision saved", "mode", status.Mode, "state", status.State, "gridW", status.GridW, "targetChargeW", status.TargetChargeW, "reason", status.LastDecisionReason)
}

func recordEnergyMeterReading(ctx context.Context, reader energyMeterReader, repository energyMeterWriter, logger *slog.Logger) {
	if reader == nil || repository == nil {
		return
	}
	reading, err := reader.CurrentEnergyMeterReading(ctx)
	if err != nil {
		logger.Warn("failed to read Nature Remo cumulative energy", "error", err)
		return
	}
	if err := repository.InsertEnergyMeterReading(ctx, reading); err != nil {
		logger.Warn("failed to save energy meter log", "error", err)
	}
}

func lastCommandStatus(provider api.StatusProvider) commandStatus {
	commandProvider, ok := provider.(commandStatusProvider)
	if !ok {
		return commandStatus{}
	}
	return commandStatus{
		ActualCommandW: commandProvider.LastCommandActualW(),
		CommandSent:    commandProvider.LastCommandSent(),
	}
}

func powerLogFromStatus(status domain.Status, command commandStatus) domain.PowerLog {
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
		ActualCommandW: command.ActualCommandW,
		DecisionReason: status.LastDecisionReason,
		Mode:           status.Mode,
		CommandSent:    command.CommandSent,
		ErrorMessage:   status.LastError,
		CreatedAt:      status.UpdatedAt,
	}
}

func intPtr(value int) *int {
	return &value
}
