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
	"github.com/eisles/energy-controller/backend/internal/control"
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
	nightChargePlanRepository := store.NewNightChargePlanRepository(db)
	surplusControlCommandRepository := store.NewSurplusControlCommandRepository(db)
	surplusWriteClient := newSurplusWriteClient(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	recordStatus(ctx, cfg, statusProvider, statusRepository, logRepository, nightChargePlanRepository, surplusControlCommandRepository, surplusWriteClient, energyMeterReader, energyMeterRepository, logger)
	go runControlLoop(ctx, cfg, statusProvider, statusRepository, logRepository, nightChargePlanRepository, surplusControlCommandRepository, surplusWriteClient, energyMeterReader, energyMeterRepository, logger)

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

type nightChargePlanLogWriter interface {
	InsertNightChargePlanLog(ctx context.Context, status domain.Status) error
	LatestNightChargePlanWriteCandidateLog(ctx context.Context) (*domain.NightChargePlanLog, error)
}

type surplusControlCommandLogWriter interface {
	InsertSurplusControlCommandLog(ctx context.Context, log domain.SurplusControlCommandLog) error
	LatestSurplusControlCommandLog(ctx context.Context) (*domain.SurplusControlCommandLog, error)
	LatestSurplusControlWriteCandidateLog(ctx context.Context) (*domain.SurplusControlCommandLog, error)
}

type surplusWriteClient interface {
	SetACChargePower(ctx context.Context, watts int) error
	SetBackupReserveSoc(ctx context.Context, percent int) error
	SetTOUMode(ctx context.Context, enabled bool) error
	SetSelfPoweredMode(ctx context.Context, enabled bool) error
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
	loadEstimator := store.NewEcoFlowLoadRepository(db)
	if cfg.NatureMode == "cloud" {
		natureClient := nature.NewCloudClient(nature.CloudConfig{
			AccessToken: cfg.NatureAccessToken,
			ApplianceID: cfg.NatureApplianceID,
		})
		provider := mock.NewStatusProviderWithReaders(cfg.Clock, cfg.ControlSettings, cfg.MockMode, cfg.SimulationMode, cfg.EnableRealControl, cfg.AutoControlEnabled, natureClient, ecoflowClient, ecoflowWriteClient, "nature-cloud+ecoflow-read", weatherReader)
		provider.SetEcoFlowLoadEstimator(loadEstimator)
		return provider, natureClient
	}
	provider := mock.NewStatusProviderWithReaders(cfg.Clock, cfg.ControlSettings, cfg.MockMode, cfg.SimulationMode, cfg.EnableRealControl, cfg.AutoControlEnabled, nil, ecoflowClient, ecoflowWriteClient, "ecoflow-read", weatherReader)
	provider.SetEcoFlowLoadEstimator(loadEstimator)
	return provider, nil
}

func newSurplusWriteClient(cfg config.Config) surplusWriteClient {
	if cfg.MockMode {
		return nil
	}
	return ecoflow.NewSignedWriteClient(ecoflow.Config{
		AccessKey: cfg.EcoFlowAccessKey,
		SecretKey: cfg.EcoFlowSecretKey,
		DeviceSN:  cfg.EcoFlowDeviceSN,
		BaseURL:   cfg.EcoFlowBaseURL,
	}, ecoflow.WriteGuards{
		MockMode:           cfg.MockMode,
		SimulationMode:     cfg.SimulationMode,
		EnableRealControl:  cfg.EnableRealControl,
		AutoControlEnabled: cfg.AutoControlEnabled,
		ManualOneShot:      false,
	})
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

func runControlLoop(ctx context.Context, cfg config.Config, provider api.StatusProvider, statusRepository statusWriter, logRepository logWriter, nightChargePlanRepository nightChargePlanLogWriter, surplusControlCommandRepository surplusControlCommandLogWriter, writeClient surplusWriteClient, meterReader energyMeterReader, meterRepository energyMeterWriter, logger *slog.Logger) {
	interval := cfg.PollInterval
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
			recordStatus(ctx, cfg, provider, statusRepository, logRepository, nightChargePlanRepository, surplusControlCommandRepository, writeClient, meterReader, meterRepository, logger)
		}
	}
}

func recordStatus(ctx context.Context, cfg config.Config, provider api.StatusProvider, statusRepository statusWriter, logRepository logWriter, nightChargePlanRepository nightChargePlanLogWriter, surplusControlCommandRepository surplusControlCommandLogWriter, writeClient surplusWriteClient, meterReader energyMeterReader, meterRepository energyMeterWriter, logger *slog.Logger) {
	status, err := provider.CurrentStatus(ctx)
	if err != nil {
		logger.Error("failed to evaluate current status", "error", err)
		return
	}
	if err := logRepository.InsertPowerLog(ctx, powerLogFromStatus(status, lastCommandStatus(provider))); err != nil {
		logger.Error("failed to save power log", "error", err)
		return
	}
	var previousNightPlan *domain.NightChargePlanLog
	if nightChargePlanRepository != nil {
		var err error
		previousNightPlan, err = nightChargePlanRepository.LatestNightChargePlanWriteCandidateLog(ctx)
		if err != nil {
			logger.Warn("failed to load latest night charge write candidate log", "error", err)
		}
	}
	nightPlanOwnsControl := applyNightChargePlanControl(ctx, cfg, &status, writeClient, previousNightPlan)
	if nightChargePlanRepository != nil {
		if err := nightChargePlanRepository.InsertNightChargePlanLog(ctx, status); err != nil {
			logger.Warn("failed to save night charge plan log", "error", err)
		}
	}
	if surplusControlCommandRepository != nil && !nightPlanOwnsControl {
		previous, err := surplusControlCommandRepository.LatestSurplusControlWriteCandidateLog(ctx)
		if err != nil {
			logger.Warn("failed to load latest surplus control write candidate log", "error", err)
		} else {
			commandLog := control.EvaluateSurplusCommandGuard(control.SurplusCommandGuardInput{
				Status:                 status,
				MockMode:               cfg.MockMode,
				SimulationMode:         cfg.SimulationMode,
				EnableRealControl:      cfg.EnableRealControl,
				AutoControl:            cfg.AutoControlEnabled,
				ConfirmEcoFlowWrite:    cfg.ConfirmEcoFlowWrite,
				RealControlTrialActive: realControlTrialActive(cfg),
				Previous:               previous,
			}, cfg.ControlSettings)
			commandLog = control.ExecuteSurplusCommand(ctx, commandLog, writeClient)
			if err := surplusControlCommandRepository.InsertSurplusControlCommandLog(ctx, commandLog); err != nil {
				logger.Warn("failed to save surplus control command log", "error", err)
			}
		}
	}
	if err := statusRepository.UpdateCurrentStatus(ctx, status); err != nil {
		logger.Error("failed to update current status", "error", err)
		return
	}
	recordEnergyMeterReading(ctx, meterReader, meterRepository, logger)
	logger.Info("control decision saved", "mode", status.Mode, "state", status.State, "gridW", status.GridW, "targetChargeW", status.TargetChargeW, "reason", status.LastDecisionReason)
}

func applyNightChargePlanControl(ctx context.Context, cfg config.Config, status *domain.Status, writeClient surplusWriteClient, previous *domain.NightChargePlanLog) bool {
	if status == nil || status.NightChargePlan == nil {
		return false
	}
	if previous != nil && previous.MeasuredAt.Equal(status.UpdatedAt) {
		previous = nil
	}
	plan := control.GuardNightChargeCommand(control.NightChargeCommandGuardInput{
		Plan:                   *status.NightChargePlan,
		MockMode:               cfg.MockMode,
		SimulationMode:         cfg.SimulationMode,
		EnableRealControl:      cfg.EnableRealControl,
		AutoControl:            cfg.AutoControlEnabled,
		ConfirmEcoFlowWrite:    cfg.ConfirmEcoFlowWrite,
		RealControlTrialActive: realControlTrialActive(cfg),
		Previous:               previous,
		Now:                    controlClockNow(cfg),
		Settings:               cfg.ControlSettings,
	})
	plan = control.ExecuteNightChargeCommand(ctx, plan, writeClient)
	status.NightChargePlan = &plan
	return nightPlanOwnsEnergyControl(plan, status.UpdatedAt)
}

func nightPlanOwnsEnergyControl(plan domain.NightChargePlan, measuredAt time.Time) bool {
	if plan.StrategyState == "NIGHT_CHARGE_WINDOW" {
		return true
	}
	if plan.StrategyState != "NIGHT_RECOVER" {
		return false
	}
	if measuredAt.IsZero() {
		measuredAt = time.Now()
	}
	hour := measuredAt.Hour()
	return hour >= 23 || hour < 7
}

func realControlTrialActive(cfg config.Config) bool {
	if cfg.RealControlTrialUntil.IsZero() {
		return false
	}
	now := controlClockNow(cfg)
	return now.Before(cfg.RealControlTrialUntil)
}

func controlClockNow(cfg config.Config) time.Time {
	if cfg.Clock == nil {
		return time.Now()
	}
	return cfg.Clock.Now()
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
