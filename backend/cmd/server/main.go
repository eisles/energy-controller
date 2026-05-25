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
	"github.com/eisles/energy-controller/backend/internal/ecoflowdelta3"
	"github.com/eisles/energy-controller/backend/internal/mock"
	"github.com/eisles/energy-controller/backend/internal/nature"
	"github.com/eisles/energy-controller/backend/internal/notify"
	"github.com/eisles/energy-controller/backend/internal/store"
	"github.com/eisles/energy-controller/backend/internal/weather"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	cfg := config.Load()
	if !cfg.SimulationMode || cfg.EnableRealControl {
		logger.Warn("real device control flags are enabled; verify guards, trial window, and EcoFlow app state",
			"mockMode", cfg.MockMode,
			"simulationMode", cfg.SimulationMode,
			"enableRealControl", cfg.EnableRealControl,
			"autoControlEnabled", cfg.AutoControlEnabled,
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
	delta3AuxControlCommandRepository := store.NewDelta3AuxControlCommandRepository(db)
	notificationRepository := store.NewNotificationRepository(db)
	chargingDeviceRepository := store.NewChargingDeviceRepository(db)
	surplusWriteClient := newSurplusWriteClient(cfg)
	delta3StatusReader := api.NewDelta3StatusReaderWithTargetProvider(cfg, logger, chargingDeviceRepository)
	delta3AuxWriteClient := newDelta3AuxWriteClient(cfg, chargingDeviceRepository)
	manualChargeAlertService := newManualChargeAlertService(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	recordStatus(ctx, cfg, statusProvider, statusRepository, logRepository, nightChargePlanRepository, surplusControlCommandRepository, delta3AuxControlCommandRepository, notificationRepository, manualChargeAlertService, surplusWriteClient, delta3StatusReader, delta3AuxWriteClient, chargingDeviceRepository, energyMeterReader, energyMeterRepository, logger)
	go runControlLoop(ctx, cfg, statusProvider, statusRepository, logRepository, nightChargePlanRepository, surplusControlCommandRepository, delta3AuxControlCommandRepository, notificationRepository, manualChargeAlertService, surplusWriteClient, delta3StatusReader, delta3AuxWriteClient, chargingDeviceRepository, energyMeterReader, energyMeterRepository, logger)

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

type delta3AuxControlCommandLogWriter interface {
	InsertDelta3AuxControlCommandLog(ctx context.Context, log domain.Delta3AuxControlCommandLog) error
	LatestDelta3AuxControlCommandLog(ctx context.Context) (*domain.Delta3AuxControlCommandLog, error)
	LatestDelta3AuxControlWriteCandidateLog(ctx context.Context) (*domain.Delta3AuxControlCommandLog, error)
}

type notificationLogWriter interface {
	InsertNotificationLog(ctx context.Context, log domain.NotificationLog) error
	LatestNotificationLog(ctx context.Context, kind string, fingerprint string) (*domain.NotificationLog, error)
}

type manualChargeAlertEvaluator interface {
	Fingerprint() string
	EvaluateAndSend(ctx context.Context, status domain.Status, latest *domain.NotificationLog, now time.Time) (*domain.NotificationLog, error)
}

type surplusWriteClient interface {
	SetACChargePower(ctx context.Context, watts int) error
	SetBackupReserveSoc(ctx context.Context, percent int) error
	SetTOUMode(ctx context.Context, enabled bool) error
	SetSelfPoweredMode(ctx context.Context, enabled bool) error
}

type delta3StatusReader interface {
	CurrentStatus(ctx context.Context) api.Delta3StatusResponse
}

type delta3ConfigStatusReader interface {
	CurrentStatusForConfig(ctx context.Context, cfg config.Config) api.Delta3StatusResponse
}

type delta3AuxWriteClient interface {
	SetACChargePower(ctx context.Context, watts int) error
}

type delta3WriteTargetProvider interface {
	Delta3WriteTarget(ctx context.Context) (domain.ChargingDevice, bool, error)
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

type ecoFlowDelta3AuxWriteClient struct {
	cfg            config.Config
	targetProvider delta3WriteTargetProvider
}

func newDelta3AuxWriteClient(cfg config.Config, targetProvider delta3WriteTargetProvider) delta3AuxWriteClient {
	if cfg.MockMode {
		return nil
	}
	return ecoFlowDelta3AuxWriteClient{
		cfg:            cfg,
		targetProvider: targetProvider,
	}
}

func (w ecoFlowDelta3AuxWriteClient) SetACChargePower(ctx context.Context, watts int) error {
	cfg, ok, err := delta3WriteConfig(ctx, w.cfg, w.targetProvider)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("DELTA 3 Plus master write target is unavailable")
	}
	client := ecoflowdelta3.NewClient(ecoflowdelta3.Config{
		PrivateAPIHost: cfg.Delta3PrivateAPIHost,
		Email:          cfg.Delta3PrivateEmail,
		Password:       cfg.Delta3PrivatePassword,
		DeviceSN:       cfg.Delta3DeviceSN,
		DeviceType:     cfg.Delta3DeviceType,
		MQTTClientID:   cfg.Delta3MQTTClientID,
		Timeout:        cfg.Delta3Timeout,
	})
	_, err = client.ExecuteACChargePower(ctx, watts, ecoflowdelta3.WriteGuards{
		MockMode:                cfg.MockMode,
		SimulationMode:          cfg.SimulationMode,
		EnableRealControl:       cfg.EnableRealControl,
		AutoControlEnabled:      cfg.AutoControlEnabled,
		AllowAutoControlOverlap: cfg.Delta3AllowAutoWrite,
		ConfirmEcoFlowWrite:     cfg.ConfirmEcoFlowWrite,
		Execute:                 cfg.Delta3ExecuteWrite,
		AllowPrivateAPIWrite:    cfg.Delta3AllowPrivateWrite,
		Command:                 "set_ac_charge_power",
		DeviceType:              cfg.Delta3DeviceType,
	})
	return err
}

func delta3WriteConfig(ctx context.Context, cfg config.Config, targetProvider delta3WriteTargetProvider) (config.Config, bool, error) {
	if targetProvider == nil {
		return cfg, true, nil
	}
	device, ok, err := targetProvider.Delta3WriteTarget(ctx)
	if err != nil {
		return config.Config{}, false, errors.New("failed to resolve DELTA 3 Plus write target")
	}
	if !ok {
		return cfg, false, nil
	}
	return api.Delta3ConfigForDevice(cfg, device), true, nil
}

func newManualChargeAlertService(cfg config.Config) *notify.ManualChargeAlertService {
	var notifier notify.Notifier = notify.NoopNotifier{}
	if cfg.NotificationEnabled && cfg.NotificationProvider == "slack" {
		notifier = notify.SlackWebhookNotifier{WebhookURL: cfg.SlackWebhookURL}
	}
	return notify.NewManualChargeAlertService(notify.ManualChargeAlertSettings{
		Enabled:          cfg.NotificationEnabled,
		ExportThresholdW: cfg.ManualChargeAlert.ExportThresholdW,
		SocThreshold:     cfg.ManualChargeAlert.SocThreshold,
		ConsecutiveCount: cfg.ManualChargeAlert.ConsecutiveCount,
		Cooldown:         cfg.ManualChargeAlert.Cooldown,
		MaxChargeW:       cfg.ControlSettings.MaxChargeW,
	}, notifier)
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

func runControlLoop(ctx context.Context, cfg config.Config, provider api.StatusProvider, statusRepository statusWriter, logRepository logWriter, nightChargePlanRepository nightChargePlanLogWriter, surplusControlCommandRepository surplusControlCommandLogWriter, delta3AuxControlCommandRepository delta3AuxControlCommandLogWriter, notificationRepository notificationLogWriter, manualChargeAlertService manualChargeAlertEvaluator, writeClient surplusWriteClient, delta3Reader delta3StatusReader, delta3Writer delta3AuxWriteClient, delta3TargetProvider delta3WriteTargetProvider, meterReader energyMeterReader, meterRepository energyMeterWriter, logger *slog.Logger) {
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
			recordStatus(ctx, cfg, provider, statusRepository, logRepository, nightChargePlanRepository, surplusControlCommandRepository, delta3AuxControlCommandRepository, notificationRepository, manualChargeAlertService, writeClient, delta3Reader, delta3Writer, delta3TargetProvider, meterReader, meterRepository, logger)
		}
	}
}

func recordStatus(ctx context.Context, cfg config.Config, provider api.StatusProvider, statusRepository statusWriter, logRepository logWriter, nightChargePlanRepository nightChargePlanLogWriter, surplusControlCommandRepository surplusControlCommandLogWriter, delta3AuxControlCommandRepository delta3AuxControlCommandLogWriter, notificationRepository notificationLogWriter, manualChargeAlertService manualChargeAlertEvaluator, writeClient surplusWriteClient, delta3Reader delta3StatusReader, delta3Writer delta3AuxWriteClient, delta3TargetProvider delta3WriteTargetProvider, meterReader energyMeterReader, meterRepository energyMeterWriter, logger *slog.Logger) {
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
	if surplusControlCommandRepository != nil {
		if nightPlanOwnsControl {
			if err := surplusControlCommandRepository.InsertSurplusControlCommandLog(ctx, nightOwnedSurplusCommandLog(status)); err != nil {
				logger.Warn("failed to save night-owned surplus control skip log", "error", err)
			}
		} else {
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
	}
	if delta3AuxControlCommandRepository != nil {
		applyDelta3AuxControl(ctx, cfg, &status, delta3Reader, delta3Writer, delta3TargetProvider, surplusControlCommandRepository, delta3AuxControlCommandRepository, logger)
	}
	if err := statusRepository.UpdateCurrentStatus(ctx, status); err != nil {
		logger.Error("failed to update current status", "error", err)
		return
	}
	recordManualChargeAlert(ctx, cfg, status, notificationRepository, manualChargeAlertService, logger)
	recordEnergyMeterReading(ctx, meterReader, meterRepository, logger)
	logger.Info("control decision saved", "mode", status.Mode, "state", status.State, "gridW", status.GridW, "targetChargeW", status.TargetChargeW, "reason", status.LastDecisionReason)
}

func applyDelta3AuxControl(ctx context.Context, cfg config.Config, status *domain.Status, delta3Reader delta3StatusReader, delta3Writer delta3AuxWriteClient, delta3TargetProvider delta3WriteTargetProvider, surplusControlCommandRepository surplusControlCommandLogWriter, delta3AuxControlCommandRepository delta3AuxControlCommandLogWriter, logger *slog.Logger) {
	if status == nil {
		return
	}
	delta3ControlEnabled := true
	delta3ControlCfg := cfg
	if delta3TargetProvider != nil {
		device, ok, err := delta3TargetProvider.Delta3WriteTarget(ctx)
		if err != nil {
			logger.Warn("failed to resolve DELTA 3 Plus write target", "error", err)
			delta3ControlEnabled = false
		} else if !ok {
			delta3ControlEnabled = false
		} else {
			delta3ControlCfg = api.Delta3ConfigForDevice(cfg, device)
		}
	}
	delta3Status := api.Delta3StatusResponse{Available: false, LastError: "DELTA 3 Plus status reader is unavailable"}
	if delta3Reader != nil {
		if configReader, ok := delta3Reader.(delta3ConfigStatusReader); ok && delta3ControlEnabled {
			delta3Status = configReader.CurrentStatusForConfig(ctx, delta3ControlCfg)
		} else {
			delta3Status = delta3Reader.CurrentStatus(ctx)
		}
	}
	var previousPro3 *domain.SurplusControlCommandLog
	if surplusControlCommandRepository != nil {
		var err error
		previousPro3, err = surplusControlCommandRepository.LatestSurplusControlWriteCandidateLog(ctx)
		if err != nil {
			logger.Warn("failed to load latest surplus control write candidate for DELTA 3 Plus aux plan", "error", err)
		}
	}
	status.Delta3AuxPlan = ptrToDelta3AuxPlan(control.PlanDelta3AuxCharging(control.Delta3AuxPlanInput{
		Status: *status,
		Delta3: control.Delta3AuxStatus{
			Available:      delta3Status.Available,
			DeviceType:     delta3Status.DeviceType,
			SOC:            delta3Status.SOC,
			ACInW:          delta3Status.ACInW,
			ACOutW:         delta3Status.ACOutW,
			ACChargeLimitW: delta3Status.ACChargeLimitW,
			MaxChargeSoc:   delta3Status.MaxChargeSoc,
			LastError:      delta3Status.LastError,
		},
		Pro3PreviousCommand: previousPro3,
	}, delta3AuxSettingsFromConfig(cfg), cfg.ControlSettings))

	previous, err := delta3AuxControlCommandRepository.LatestDelta3AuxControlWriteCandidateLog(ctx)
	if err != nil {
		logger.Warn("failed to load latest DELTA 3 Plus aux command log", "error", err)
		return
	}
	commandLog := control.EvaluateDelta3AuxCommandGuard(control.Delta3AuxCommandGuardInput{
		Status:                 *status,
		MockMode:               cfg.MockMode,
		SimulationMode:         cfg.SimulationMode,
		EnableRealControl:      cfg.EnableRealControl,
		AutoControl:            cfg.AutoControlEnabled,
		ConfirmEcoFlowWrite:    cfg.ConfirmEcoFlowWrite,
		RealControlTrialActive: realControlTrialActive(cfg),
		Delta3ReadEnabled:      cfg.Delta3ReadEnabled,
		AllowAutoControlWrite:  cfg.Delta3AllowAutoWrite,
		Execute:                cfg.Delta3ExecuteWrite,
		AllowPrivateAPIWrite:   cfg.Delta3AllowPrivateWrite,
		Delta3ControlEnabled:   delta3ControlEnabled,
		Previous:               previous,
	}, delta3AuxSettingsFromConfig(cfg))
	commandLog = control.ExecuteDelta3AuxCommand(ctx, commandLog, delta3Writer)
	if status.Delta3AuxPlan != nil {
		status.Delta3AuxPlan.WouldWrite = commandLog.WouldWrite
		status.Delta3AuxPlan.SuppressedReason = commandLog.SuppressedReason
	}
	if err := delta3AuxControlCommandRepository.InsertDelta3AuxControlCommandLog(ctx, commandLog); err != nil {
		logger.Warn("failed to save DELTA 3 Plus aux command log", "error", err)
	}
}

func ptrToDelta3AuxPlan(plan domain.Delta3AuxPlan) *domain.Delta3AuxPlan {
	return &plan
}

func delta3AuxSettingsFromConfig(cfg config.Config) control.Delta3AuxSettings {
	return control.Delta3AuxSettings{
		Enabled:                   cfg.Delta3Aux.Enabled,
		MinChargeW:                cfg.Delta3Aux.MinChargeW,
		MaxChargeW:                cfg.Delta3Aux.MaxChargeW,
		SafetyMarginW:             cfg.Delta3Aux.SafetyMarginW,
		MinCommandDiffW:           cfg.Delta3Aux.MinCommandDiffW,
		MaxIncreaseStepW:          cfg.Delta3Aux.MaxIncreaseStepW,
		MaxDecreaseStepW:          cfg.Delta3Aux.MaxDecreaseStepW,
		MinCommandInterval:        cfg.Delta3Aux.MinCommandInterval,
		StopImportThresholdW:      cfg.Delta3Aux.StopImportThresholdW,
		TargetMaxSocBufferPercent: cfg.Delta3Aux.TargetMaxSocBufferPercent,
	}
}

func recordManualChargeAlert(ctx context.Context, cfg config.Config, status domain.Status, repository notificationLogWriter, service manualChargeAlertEvaluator, logger *slog.Logger) {
	if repository == nil || service == nil {
		return
	}
	latest, err := repository.LatestNotificationLog(ctx, notify.ManualChargeAlertKind, service.Fingerprint())
	if err != nil {
		logger.Warn("failed to load latest manual charge notification log", "error", err)
		return
	}
	log, err := service.EvaluateAndSend(ctx, status, latest, controlClockNow(cfg))
	if log == nil {
		return
	}
	if err != nil {
		logger.Warn("failed to send manual charge notification", "error", err)
	}
	if insertErr := repository.InsertNotificationLog(ctx, *log); insertErr != nil {
		logger.Warn("failed to save manual charge notification log", "error", insertErr)
	}
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

func nightOwnedSurplusCommandLog(status domain.Status) domain.SurplusControlCommandLog {
	decisionReason := "night charge plan owns control"
	strategyState := "NIGHT_CHARGE_WINDOW"
	if status.NightChargePlan != nil {
		strategyState = status.NightChargePlan.StrategyState
		if status.NightChargePlan.ActionSummary != "" {
			decisionReason += ": " + status.NightChargePlan.ActionSummary
		} else if status.NightChargePlan.Reason != "" {
			decisionReason += ": " + status.NightChargePlan.Reason
		}
	}
	return domain.SurplusControlCommandLog{
		MeasuredAt:               status.UpdatedAt,
		StrategyState:            strategyState,
		CommandKind:              "none",
		CommandFingerprint:       "night-charge-owns-control",
		GridW:                    status.GridW,
		ImportW:                  status.ImportW,
		ExportW:                  status.ExportW,
		BatterySoc:               status.BatterySoc,
		BatteryInputW:            status.BatteryInputW,
		BatteryOutputW:           status.BatteryOutputW,
		PreviousACChargeLimitW:   intPtr(status.ACChargeLimitW),
		PreviousBackupReserveSoc: status.BackupReserveSoc,
		CommandSent:              false,
		DryRun:                   true,
		WouldWrite:               false,
		SuppressedReason:         "night charge plan owns control",
		DecisionReason:           decisionReason,
		CreatedAt:                status.UpdatedAt,
	}
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
