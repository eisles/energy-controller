package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/eisles/energy-controller/backend/internal/api"
	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/control"
	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/ecoflow"
	"github.com/eisles/energy-controller/backend/internal/ecoflowprivate"
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

	chargingDeviceRepository := store.NewChargingDeviceRepository(db)
	statusProvider, energyMeterReader := newStatusProvider(cfg, db, chargingDeviceRepository)
	statusRepository := store.NewStatusRepository(db)
	logRepository := store.NewLogRepository(db)
	energyMeterRepository := store.NewEnergyMeterRepository(db)
	nightChargePlanRepository := store.NewNightChargePlanRepository(db)
	surplusControlCommandRepository := store.NewSurplusControlCommandRepository(db)
	delta3AuxControlCommandRepository := store.NewDelta3AuxControlCommandRepository(db)
	pro3ACOutputEventRepository := store.NewPro3ACOutputEventRepository(db)
	notificationRepository := store.NewNotificationRepository(db)
	surplusWriteClient := newSurplusWriteClient(cfg, chargingDeviceRepository)
	delta3StatusReader := api.NewDelta3StatusReaderWithTargetProvider(cfg, logger, chargingDeviceRepository)
	delta3AuxWriteClient := newDelta3AuxWriteClient(cfg, chargingDeviceRepository)
	manualChargeAlertService := newManualChargeAlertService(cfg)
	pro3ACOutputAlertService := newPro3ACOutputAlertService(cfg)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	recordStatus(ctx, cfg, statusProvider, statusRepository, logRepository, nightChargePlanRepository, surplusControlCommandRepository, delta3AuxControlCommandRepository, pro3ACOutputEventRepository, notificationRepository, manualChargeAlertService, pro3ACOutputAlertService, surplusWriteClient, delta3StatusReader, delta3AuxWriteClient, chargingDeviceRepository, energyMeterReader, energyMeterRepository, logger)
	go runControlLoop(ctx, cfg, statusProvider, statusRepository, logRepository, nightChargePlanRepository, surplusControlCommandRepository, delta3AuxControlCommandRepository, pro3ACOutputEventRepository, notificationRepository, manualChargeAlertService, pro3ACOutputAlertService, surplusWriteClient, delta3StatusReader, delta3AuxWriteClient, chargingDeviceRepository, energyMeterReader, energyMeterRepository, logger)

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
	LatestDelta3AuxReserveCommandLog(ctx context.Context) (*domain.Delta3AuxControlCommandLog, error)
}

type pro3ACOutputEventLogWriter interface {
	InsertPro3ACOutputEvent(ctx context.Context, event domain.Pro3ACOutputEvent) error
	LatestPro3ACOutputEvent(ctx context.Context) (*domain.Pro3ACOutputEvent, error)
	LatestPro3ACOutputEventByType(ctx context.Context, eventType string) (*domain.Pro3ACOutputEvent, error)
}

type notificationLogWriter interface {
	InsertNotificationLog(ctx context.Context, log domain.NotificationLog) error
	LatestNotificationLog(ctx context.Context, kind string, fingerprint string) (*domain.NotificationLog, error)
}

type manualChargeAlertEvaluator interface {
	Fingerprint() string
	EvaluateAndSend(ctx context.Context, status domain.Status, latest *domain.NotificationLog, now time.Time) (*domain.NotificationLog, error)
}

type pro3ACOutputAlertEvaluator interface {
	Fingerprint() string
	EvaluateAndSend(ctx context.Context, event domain.Pro3ACOutputEvent, latest *domain.NotificationLog, now time.Time) (*domain.NotificationLog, error)
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

type deviceStatusesReader interface {
	CurrentDeviceStatuses(ctx context.Context, devices []domain.ChargingDevice) []api.DeviceStatusResponse
}

type delta3ConfigStatusReader interface {
	CurrentStatusForConfig(ctx context.Context, cfg config.Config) api.Delta3StatusResponse
}

type delta3AuxWriteClient interface {
	SetACChargePower(ctx context.Context, watts int) error
	SetEnergyBackupEnabled(ctx context.Context, enabled bool, startSoc int) error
}

type delta3WriteTargetProvider interface {
	Delta3WriteTarget(ctx context.Context) (domain.ChargingDevice, bool, error)
}

type chargingDeviceLister interface {
	ListChargingDevices(ctx context.Context) ([]domain.ChargingDevice, error)
}

type chargingPriorityTargetProvider interface {
	ecoFlowCloudWriteTargetProvider
	delta3WriteTargetProvider
}

type ecoFlowCloudReadTargetProvider interface {
	EcoFlowCloudReadTarget(ctx context.Context) (domain.ChargingDevice, bool, error)
}

type ecoFlowCloudWriteTargetProvider interface {
	EcoFlowCloudWriteTarget(ctx context.Context) (domain.ChargingDevice, bool, error)
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

type chargingPriorityContext struct {
	pro3     domain.ChargingDevice
	pro3OK   bool
	delta3   domain.ChargingDevice
	delta3OK bool
}

type commandStatusProvider interface {
	LastCommandActualW() *int
	LastCommandSent() bool
}

func newStatusProvider(cfg config.Config, db *sql.DB, targetProvider ecoFlowCloudReadTargetProvider) (api.StatusProvider, energyMeterReader) {
	if cfg.MockMode {
		return mock.NewStatusProvider(cfg.Clock, cfg.ControlSettings, cfg.MockMode, cfg.SimulationMode, cfg.EnableRealControl, cfg.AutoControlEnabled), nil
	}
	ecoflowClient := ecoFlowCloudBatteryReader{cfg: cfg, targetProvider: targetProvider}
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

func newSurplusWriteClient(cfg config.Config, targetProvider ecoFlowCloudWriteTargetProvider) surplusWriteClient {
	if cfg.MockMode {
		return nil
	}
	return ecoFlowCloudSurplusWriteClient{
		cfg:            cfg,
		targetProvider: targetProvider,
		factory: func(cfg ecoflow.Config, guards ecoflow.WriteGuards) surplusWriteClient {
			return ecoflow.NewSignedWriteClient(cfg, guards)
		},
	}
}

type ecoFlowCloudBatteryReader struct {
	cfg            config.Config
	targetProvider ecoFlowCloudReadTargetProvider
}

func (r ecoFlowCloudBatteryReader) GetBatteryStatus(ctx context.Context) (domain.BatteryStatus, error) {
	cfg, ok, err := ecoFlowCloudReadConfig(ctx, r.cfg, r.targetProvider)
	if err != nil {
		return domain.BatteryStatus{}, err
	}
	if !ok {
		return domain.BatteryStatus{}, errors.New("DELTA Pro 3 read target is unavailable")
	}
	client := ecoflow.NewSignedClient(ecoflow.Config{
		AccessKey: cfg.EcoFlowAccessKey,
		SecretKey: cfg.EcoFlowSecretKey,
		DeviceSN:  cfg.EcoFlowDeviceSN,
		BaseURL:   cfg.EcoFlowBaseURL,
	})
	return client.GetBatteryStatus(ctx)
}

func ecoFlowCloudReadConfig(ctx context.Context, cfg config.Config, targetProvider ecoFlowCloudReadTargetProvider) (config.Config, bool, error) {
	if targetProvider == nil {
		return cfg, true, nil
	}
	device, ok, err := targetProvider.EcoFlowCloudReadTarget(ctx)
	if err != nil {
		return config.Config{}, false, errors.New("failed to resolve DELTA Pro 3 read target")
	}
	if !ok {
		return cfg, strings.TrimSpace(cfg.EcoFlowDeviceSN) != "", nil
	}
	return api.EcoFlowCloudConfigForDevice(cfg, device), true, nil
}

type ecoFlowCloudSurplusWriteClient struct {
	cfg            config.Config
	targetProvider ecoFlowCloudWriteTargetProvider
	factory        func(ecoflow.Config, ecoflow.WriteGuards) surplusWriteClient
}

func (w ecoFlowCloudSurplusWriteClient) SetACChargePower(ctx context.Context, watts int) error {
	return w.withClient(ctx, func(client surplusWriteClient) error {
		return client.SetACChargePower(ctx, watts)
	})
}

func (w ecoFlowCloudSurplusWriteClient) SetBackupReserveSoc(ctx context.Context, percent int) error {
	return w.withClient(ctx, func(client surplusWriteClient) error {
		return client.SetBackupReserveSoc(ctx, percent)
	})
}

func (w ecoFlowCloudSurplusWriteClient) SetTOUMode(ctx context.Context, enabled bool) error {
	return w.withClient(ctx, func(client surplusWriteClient) error {
		return client.SetTOUMode(ctx, enabled)
	})
}

func (w ecoFlowCloudSurplusWriteClient) SetSelfPoweredMode(ctx context.Context, enabled bool) error {
	return w.withClient(ctx, func(client surplusWriteClient) error {
		return client.SetSelfPoweredMode(ctx, enabled)
	})
}

func (w ecoFlowCloudSurplusWriteClient) withClient(ctx context.Context, run func(surplusWriteClient) error) error {
	cfg, ok, err := ecoFlowCloudWriteConfig(ctx, w.cfg, w.targetProvider)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("DELTA Pro 3 master write target is unavailable")
	}
	factory := w.factory
	if factory == nil {
		factory = func(cfg ecoflow.Config, guards ecoflow.WriteGuards) surplusWriteClient {
			return ecoflow.NewSignedWriteClient(cfg, guards)
		}
	}
	return run(factory(ecoflow.Config{
		AccessKey: cfg.EcoFlowAccessKey,
		SecretKey: cfg.EcoFlowSecretKey,
		DeviceSN:  cfg.EcoFlowDeviceSN,
		BaseURL:   cfg.EcoFlowBaseURL,
	}, ecoFlowWriteGuards(cfg)))
}

func ecoFlowCloudWriteConfig(ctx context.Context, cfg config.Config, targetProvider ecoFlowCloudWriteTargetProvider) (config.Config, bool, error) {
	if targetProvider == nil {
		return config.Config{}, false, nil
	}
	device, ok, err := targetProvider.EcoFlowCloudWriteTarget(ctx)
	if err != nil {
		return config.Config{}, false, errors.New("failed to resolve DELTA Pro 3 write target")
	}
	if !ok {
		return config.Config{}, false, nil
	}
	return api.EcoFlowCloudConfigForDevice(cfg, device), true, nil
}

func ecoFlowWriteGuards(cfg config.Config) ecoflow.WriteGuards {
	return ecoflow.WriteGuards{
		MockMode:           cfg.MockMode,
		SimulationMode:     cfg.SimulationMode,
		EnableRealControl:  cfg.EnableRealControl,
		AutoControlEnabled: cfg.AutoControlEnabled,
		ManualOneShot:      false,
	}
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
	client := ecoflowprivate.NewClient(ecoflowprivate.Config{
		PrivateAPIHost: cfg.Delta3PrivateAPIHost,
		Email:          cfg.Delta3PrivateEmail,
		Password:       cfg.Delta3PrivatePassword,
		DeviceSN:       cfg.Delta3DeviceSN,
		DeviceType:     cfg.Delta3DeviceType,
		MQTTClientID:   cfg.Delta3MQTTClientID,
		Timeout:        cfg.Delta3Timeout,
	})
	_, err = client.ExecuteACChargePower(ctx, watts, ecoflowprivate.WriteGuards{
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

func (w ecoFlowDelta3AuxWriteClient) SetEnergyBackupEnabled(ctx context.Context, enabled bool, startSoc int) error {
	cfg, ok, err := delta3WriteConfig(ctx, w.cfg, w.targetProvider)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("DELTA 3 Plus master write target is unavailable")
	}
	client := ecoflowprivate.NewClient(ecoflowprivate.Config{
		PrivateAPIHost: cfg.Delta3PrivateAPIHost,
		Email:          cfg.Delta3PrivateEmail,
		Password:       cfg.Delta3PrivatePassword,
		DeviceSN:       cfg.Delta3DeviceSN,
		DeviceType:     cfg.Delta3DeviceType,
		MQTTClientID:   cfg.Delta3MQTTClientID,
		Timeout:        cfg.Delta3Timeout,
	})
	guards := ecoflowprivate.WriteGuards{
		MockMode:                cfg.MockMode,
		SimulationMode:          cfg.SimulationMode,
		EnableRealControl:       cfg.EnableRealControl,
		AutoControlEnabled:      cfg.AutoControlEnabled,
		AllowAutoControlOverlap: cfg.Delta3AllowAutoWrite,
		ConfirmEcoFlowWrite:     cfg.ConfirmEcoFlowWrite,
		Execute:                 cfg.Delta3ExecuteWrite,
		AllowPrivateAPIWrite:    cfg.Delta3AllowPrivateWrite,
		DeviceType:              cfg.Delta3DeviceType,
	}
	if enabled {
		_, err = client.ExecuteBackupReserve(ctx, startSoc, guards)
		return err
	}
	_, err = client.ExecuteEnergyBackupEnabled(ctx, false, startSoc, guards)
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

func newPro3ACOutputAlertService(cfg config.Config) *notify.Pro3ACOutputAlertService {
	var notifier notify.Notifier = notify.NoopNotifier{}
	if cfg.NotificationEnabled && cfg.NotificationProvider == "slack" {
		notifier = notify.SlackWebhookNotifier{WebhookURL: cfg.SlackWebhookURL}
	}
	return notify.NewPro3ACOutputAlertService(notify.Pro3ACOutputAlertSettings{
		Enabled:  cfg.NotificationEnabled,
		Cooldown: cfg.ManualChargeAlert.Cooldown,
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

func runControlLoop(ctx context.Context, cfg config.Config, provider api.StatusProvider, statusRepository statusWriter, logRepository logWriter, nightChargePlanRepository nightChargePlanLogWriter, surplusControlCommandRepository surplusControlCommandLogWriter, delta3AuxControlCommandRepository delta3AuxControlCommandLogWriter, pro3ACOutputEventRepository pro3ACOutputEventLogWriter, notificationRepository notificationLogWriter, manualChargeAlertService manualChargeAlertEvaluator, pro3ACOutputAlertService pro3ACOutputAlertEvaluator, writeClient surplusWriteClient, delta3Reader delta3StatusReader, delta3Writer delta3AuxWriteClient, delta3TargetProvider delta3WriteTargetProvider, meterReader energyMeterReader, meterRepository energyMeterWriter, logger *slog.Logger) {
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
			recordStatus(ctx, cfg, provider, statusRepository, logRepository, nightChargePlanRepository, surplusControlCommandRepository, delta3AuxControlCommandRepository, pro3ACOutputEventRepository, notificationRepository, manualChargeAlertService, pro3ACOutputAlertService, writeClient, delta3Reader, delta3Writer, delta3TargetProvider, meterReader, meterRepository, logger)
		}
	}
}

func recordStatus(ctx context.Context, cfg config.Config, provider api.StatusProvider, statusRepository statusWriter, logRepository logWriter, nightChargePlanRepository nightChargePlanLogWriter, surplusControlCommandRepository surplusControlCommandLogWriter, delta3AuxControlCommandRepository delta3AuxControlCommandLogWriter, pro3ACOutputEventRepository pro3ACOutputEventLogWriter, notificationRepository notificationLogWriter, manualChargeAlertService manualChargeAlertEvaluator, pro3ACOutputAlertService pro3ACOutputAlertEvaluator, writeClient surplusWriteClient, delta3Reader delta3StatusReader, delta3Writer delta3AuxWriteClient, delta3TargetProvider delta3WriteTargetProvider, meterReader energyMeterReader, meterRepository energyMeterWriter, logger *slog.Logger) {
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
	var previousDelta3Aux *domain.Delta3AuxControlCommandLog
	if delta3AuxControlCommandRepository != nil {
		var err error
		previousDelta3Aux, err = delta3AuxControlCommandRepository.LatestDelta3AuxControlWriteCandidateLog(ctx)
		if err != nil {
			logger.Warn("failed to load latest DELTA 3 Plus aux write candidate log", "error", err)
		}
	}
	var previousSurplusForEvent *domain.SurplusControlCommandLog
	if surplusControlCommandRepository != nil {
		var err error
		previousSurplusForEvent, err = surplusControlCommandRepository.LatestSurplusControlWriteCandidateLog(ctx)
		if err != nil {
			logger.Warn("failed to load latest surplus control write candidate for Pro3 AC output event", "error", err)
		}
	}
	recordPro3ACOutputEvent(ctx, cfg, &status, previousSurplusForEvent, pro3ACOutputEventRepository, notificationRepository, pro3ACOutputAlertService, logger)
	priorityContext := resolveChargingPriorityContext(ctx, delta3TargetProvider, logger)
	applyNightChargeDevicePlans(ctx, cfg, &status, delta3TargetProvider, delta3Reader, previousNightPlan, previousDelta3Aux, logger)
	nightPlanOwnsControl := applyNightChargePlanControl(ctx, cfg, &status, writeClient, previousNightPlan)
	applyPro3MasterReserveToSurplusPlan(cfg, &status, priorityContext)
	higherPriorityDevice := higherPriorityDelta3ChargeCandidateDevice(ctx, cfg, status, priorityContext, delta3Reader, delta3Writer, delta3AuxControlCommandRepository, logger)
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
					HigherPriorityDevice:   higherPriorityDevice,
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
		applyDelta3AuxControl(ctx, cfg, &status, delta3Reader, delta3Writer, delta3TargetProvider, surplusControlCommandRepository, delta3AuxControlCommandRepository, priorityContext.ignorePro3WaitForDelta3(cfg), logger)
	}
	if err := statusRepository.UpdateCurrentStatus(ctx, status); err != nil {
		logger.Error("failed to update current status", "error", err)
		return
	}
	recordManualChargeAlert(ctx, cfg, status, notificationRepository, manualChargeAlertService, logger)
	recordEnergyMeterReading(ctx, meterReader, meterRepository, logger)
	logger.Info("control decision saved", "mode", status.Mode, "state", status.State, "gridW", status.GridW, "targetChargeW", status.TargetChargeW, "reason", status.LastDecisionReason)
}

func applyDelta3AuxControl(ctx context.Context, cfg config.Config, status *domain.Status, delta3Reader delta3StatusReader, delta3Writer delta3AuxWriteClient, delta3TargetProvider delta3WriteTargetProvider, surplusControlCommandRepository surplusControlCommandLogWriter, delta3AuxControlCommandRepository delta3AuxControlCommandLogWriter, ignorePro3Wait bool, logger *slog.Logger) {
	if status == nil {
		return
	}
	delta3ControlEnabled := true
	delta3ControlCfg := cfg
	var delta3ControlDevice domain.ChargingDevice
	if delta3TargetProvider != nil {
		device, ok, err := delta3TargetProvider.Delta3WriteTarget(ctx)
		if err != nil {
			logger.Warn("failed to resolve DELTA 3 Plus write target", "error", err)
			delta3ControlEnabled = false
		} else if !ok {
			delta3ControlEnabled = false
		} else {
			delta3ControlDevice = device
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
	delta3Settings := delta3AuxSettingsForDevice(cfg, delta3ControlDevice)
	delta3Plan := control.PlanDelta3AuxCharging(control.Delta3AuxPlanInput{
		Status: *status,
		Delta3: control.Delta3AuxStatus{
			Available:            delta3Status.Available,
			DeviceType:           delta3Status.DeviceType,
			SOC:                  delta3Status.SOC,
			ACInW:                delta3Status.ACInW,
			ACOutW:               delta3Status.ACOutW,
			ACChargeLimitW:       delta3Status.ACChargeLimitW,
			MaxChargeSoc:         delta3Status.MaxChargeSoc,
			BackupReserveSoc:     delta3Status.BackupReserveSoc,
			BackupReserveEnabled: delta3Status.BackupReserveEnabled,
			ACOutputEnabled:      delta3Status.ACOutputEnabled,
			LastError:            delta3Status.LastError,
		},
		IgnorePro3Wait:      ignorePro3Wait,
		Pro3PreviousCommand: previousPro3,
	}, delta3Settings, cfg.ControlSettings)
	if delta3ControlDevice.ID != 0 {
		delta3Plan.DeviceID = delta3ControlDevice.ID
		delta3Plan.DeviceName = chargingDeviceName(delta3ControlDevice)
		delta3Plan.DeviceType = delta3ControlDevice.DeviceType
		if delta3Plan.DeviceType == "" {
			delta3Plan.DeviceType = delta3Status.DeviceType
		}
	}
	status.Delta3AuxPlan = ptrToDelta3AuxPlan(delta3Plan)

	previous, err := delta3AuxControlCommandRepository.LatestDelta3AuxControlWriteCandidateLog(ctx)
	if err != nil {
		logger.Warn("failed to load latest DELTA 3 Plus aux command log", "error", err)
		return
	}
	previousReserve, err := delta3AuxControlCommandRepository.LatestDelta3AuxReserveCommandLog(ctx)
	if err != nil {
		logger.Warn("failed to load latest DELTA 3 Plus reserve command log", "error", err)
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
		Delta3ControlEnabled:   delta3ControlEnabled,
		AllowAutoControlWrite:  cfg.Delta3AllowAutoWrite,
		Execute:                cfg.Delta3ExecuteWrite,
		AllowPrivateAPIWrite:   cfg.Delta3AllowPrivateWrite,
		Previous:               previous,
		PreviousReserve:        previousReserve,
	}, delta3Settings)
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

func delta3AuxSettingsForDevice(cfg config.Config, device domain.ChargingDevice) control.Delta3AuxSettings {
	settings := delta3AuxSettingsFromConfig(cfg)
	if device.MinChargeW > 0 {
		settings.MinChargeW = device.MinChargeW
	}
	if device.MaxChargeW > 0 {
		settings.MaxChargeW = device.MaxChargeW
	}
	if deviceRange, ok := ecoflowprivate.RangeForDeviceType(device.DeviceType); ok {
		if settings.MinChargeW < deviceRange.MinACChargeW {
			settings.MinChargeW = deviceRange.MinACChargeW
		}
		if settings.MaxChargeW > deviceRange.MaxACChargeW {
			settings.MaxChargeW = deviceRange.MaxACChargeW
		}
	}
	if device.ReserveSoc > 0 {
		settings.MinDischargeReserveSoc = device.ReserveSoc
		settings.BackupReserveMinSoc = device.ReserveSoc
	}
	if device.BackupReserveMinSoc > 0 {
		settings.BackupReserveMinSoc = device.BackupReserveMinSoc
		settings.MinDischargeReserveSoc = device.BackupReserveMinSoc
	}
	if device.BackupReserveMaxSoc > 0 {
		settings.BackupReserveMaxSoc = device.BackupReserveMaxSoc
	} else if device.TargetSoc > 0 {
		settings.BackupReserveMaxSoc = device.TargetSoc
	}
	settings.AutoRecoverACOutput = device.AutoRecoverACOutput
	return settings
}

func applyPro3MasterReserveToSurplusPlan(cfg config.Config, status *domain.Status, priorityContext chargingPriorityContext) {
	if status == nil || !priorityContext.pro3OK || status.ImportW <= 0 {
		return
	}
	minReserveSoc := pro3MasterMinReserveSoc(priorityContext.pro3)
	if minReserveSoc <= 0 {
		return
	}
	plan := control.PlanSurplusCharging(control.SurplusPlanInput{
		GridW:                  status.GridW,
		MockMode:               cfg.MockMode,
		BatterySoc:             status.BatterySoc,
		BatteryInputW:          status.BatteryInputW,
		BatteryOutputW:         status.BatteryOutputW,
		ACChargeLimitW:         status.ACChargeLimitW,
		BackupReserveSoc:       status.BackupReserveSoc,
		DefaultReserveSoc:      cfg.ControlSettings.DefaultReserveSoc,
		MinDischargeReserveSoc: minReserveSoc,
		TOUModeEnabled:         status.TOUModeEnabled,
		SelfPoweredEnabled:     status.SelfPoweredEnabled,
		ScheduledEnabled:       status.ScheduledEnabled,
		IntelligentEnabled:     status.IntelligentEnabled,
		SimulationMode:         cfg.SimulationMode,
		EnableRealControl:      cfg.EnableRealControl,
		AutoControl:            cfg.AutoControlEnabled,
	}, cfg.ControlSettings)
	status.SurplusPlan = &plan
	if plan.ActionSummary != "" && !strings.Contains(status.LastDecisionReason, plan.ActionSummary) {
		if status.LastDecisionReason != "" {
			status.LastDecisionReason += "; "
		}
		status.LastDecisionReason += "surplus plan: " + plan.ActionSummary
	}
}

func pro3MasterMinReserveSoc(device domain.ChargingDevice) int {
	if device.BackupReserveMinSoc > 0 {
		return device.BackupReserveMinSoc
	}
	return device.ReserveSoc
}

func recordPro3ACOutputEvent(ctx context.Context, cfg config.Config, status *domain.Status, previous *domain.SurplusControlCommandLog, repository pro3ACOutputEventLogWriter, notificationRepository notificationLogWriter, service pro3ACOutputAlertEvaluator, logger *slog.Logger) {
	if status == nil || repository == nil {
		return
	}
	now := controlClockNow(cfg)
	event, ok := control.BuildPro3ACOutputEvent(*status, previous, now)
	if !ok {
		status.Pro3ACOutputEvent = nil
		return
	}
	latest, err := repository.LatestPro3ACOutputEventByType(ctx, event.EventType)
	if err != nil {
		logger.Warn("failed to load latest Pro3 AC output event by type", "error", err)
		return
	}
	if latest != nil && now.Sub(latest.CreatedAt) < 30*time.Minute {
		status.Pro3ACOutputEvent = latest
		return
	}
	if err := repository.InsertPro3ACOutputEvent(ctx, *event); err != nil {
		logger.Warn("failed to save Pro3 AC output event", "error", err)
		return
	}
	status.Pro3ACOutputEvent = event
	recordPro3ACOutputAlert(ctx, cfg, *event, notificationRepository, service, logger)
}

func recordPro3ACOutputAlert(ctx context.Context, cfg config.Config, event domain.Pro3ACOutputEvent, repository notificationLogWriter, service pro3ACOutputAlertEvaluator, logger *slog.Logger) {
	if repository == nil || service == nil {
		return
	}
	latest, err := repository.LatestNotificationLog(ctx, notify.Pro3ACOutputOffAlertKind, service.Fingerprint())
	if err != nil {
		logger.Warn("failed to load latest Pro3 AC output notification log", "error", err)
		return
	}
	log, err := service.EvaluateAndSend(ctx, event, latest, controlClockNow(cfg))
	if log == nil {
		return
	}
	if err != nil {
		logger.Warn("failed to send Pro3 AC output notification", "error", err)
	}
	if insertErr := repository.InsertNotificationLog(ctx, *log); insertErr != nil {
		logger.Warn("failed to save Pro3 AC output notification log", "error", insertErr)
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

func applyNightChargeDevicePlans(ctx context.Context, cfg config.Config, status *domain.Status, targetProvider delta3WriteTargetProvider, delta3Reader delta3StatusReader, previous *domain.NightChargePlanLog, previousDelta3Aux *domain.Delta3AuxControlCommandLog, logger *slog.Logger) {
	if status == nil || status.NightChargePlan == nil || targetProvider == nil || delta3Reader == nil {
		return
	}
	deviceLister, ok := targetProvider.(chargingDeviceLister)
	if !ok || deviceLister == nil {
		return
	}
	statusReader, ok := delta3Reader.(deviceStatusesReader)
	if !ok || statusReader == nil {
		return
	}
	devices, err := deviceLister.ListChargingDevices(ctx)
	if err != nil {
		logger.Warn("failed to list devices for night charge plan", "error", err)
		return
	}
	deviceStatuses := statusReader.CurrentDeviceStatuses(ctx, devices)
	now := controlClockNow(cfg)
	control.ApplyNightChargeDevicePlans(status.NightChargePlan, nightChargeDeviceInputs(ctx, targetProvider, deviceStatuses, devices, logger), cfg.ControlSettings, control.NightChargeDeviceWriteGuard{
		MockMode:                cfg.MockMode,
		SimulationMode:          cfg.SimulationMode,
		EnableRealControl:       cfg.EnableRealControl,
		AutoControl:             cfg.AutoControlEnabled,
		ConfirmEcoFlowWrite:     cfg.ConfirmEcoFlowWrite,
		RealControlTrialActive:  realControlTrialActive(cfg),
		IsNightChargeTime:       nightChargeDeviceWindowActive(now),
		Delta3AllowAutoWrite:    cfg.Delta3AllowAutoWrite,
		Delta3ExecuteWrite:      cfg.Delta3ExecuteWrite,
		Delta3AllowPrivateWrite: cfg.Delta3AllowPrivateWrite,
		Delta3AuxEnabled:        cfg.Delta3Aux.Enabled,
		Delta3AuxPrevious:       previousDelta3Aux,
		Delta3AuxMinInterval:    cfg.Delta3Aux.MinCommandInterval,
		Previous:                previous,
		Now:                     now,
	})
}

func nightChargeDeviceInputs(ctx context.Context, targetProvider delta3WriteTargetProvider, deviceStatuses []api.DeviceStatusResponse, devices []domain.ChargingDevice, logger *slog.Logger) []control.NightChargeDeviceInput {
	deviceByID := make(map[int64]domain.ChargingDevice, len(devices))
	for _, device := range devices {
		deviceByID[device.ID] = device
	}
	var pro3WriteTargetID int64
	if pro3TargetProvider, ok := targetProvider.(ecoFlowCloudWriteTargetProvider); ok && pro3TargetProvider != nil {
		device, ok, err := pro3TargetProvider.EcoFlowCloudWriteTarget(ctx)
		if err != nil {
			logger.Warn("failed to resolve DELTA Pro 3 write target for night charge device plan", "error", err)
		} else if ok {
			pro3WriteTargetID = device.ID
		}
	}
	inputs := make([]control.NightChargeDeviceInput, 0, len(deviceStatuses))
	for _, deviceStatus := range deviceStatuses {
		device := deviceByID[deviceStatus.ID]
		inputs = append(inputs, control.NightChargeDeviceInput{
			DeviceID:                  deviceStatus.ID,
			Name:                      deviceStatus.Name,
			Kind:                      deviceStatus.Kind,
			Priority:                  deviceStatus.Priority,
			Enabled:                   deviceStatus.Enabled,
			ControlEnabled:            deviceStatus.ControlEnabled,
			WriteTarget:               deviceStatus.ID == pro3WriteTargetID,
			CapacityWh:                deviceStatus.CapacityWh,
			CurrentSoc:                deviceStatus.Status.SOC,
			CurrentACChargeLimitW:     deviceStatus.Status.ACChargeLimitW,
			CurrentBackupReserveSoc:   deviceStatus.Status.BackupReserveSoc,
			CurrentTOUModeEnabled:     deviceStatus.Status.TOUModeEnabled,
			CurrentSelfPoweredEnabled: deviceStatus.Status.SelfPoweredEnabled,
			ReserveSoc:                deviceStatus.ReserveSoc,
			TargetSoc:                 deviceStatus.TargetSoc,
			BackupReserveMinSoc:       deviceStatus.BackupReserveMinSoc,
			BackupReserveMaxSoc:       deviceStatus.BackupReserveMaxSoc,
			ExpectedDaytimeLoadW:      deviceStatus.ExpectedDaytimeLoadW,
			MinChargeW:                deviceStatus.MinChargeW,
			MaxChargeW:                deviceStatus.MaxChargeW,
			SupportsACChargeLimit:     device.SupportsACChargeLimit,
			StatusAvailable:           deviceStatus.Status.Available,
			StatusUnavailableReason:   deviceStatus.Status.LastError,
			DataSource:                deviceStatus.StatusSource,
		})
	}
	return inputs
}

func nightChargeDeviceWindowActive(now time.Time) bool {
	if now.IsZero() {
		return false
	}
	hour := now.Hour()
	return hour >= 23 || hour < 7
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

func resolveChargingPriorityContext(ctx context.Context, provider delta3WriteTargetProvider, logger *slog.Logger) chargingPriorityContext {
	priorityProvider, ok := provider.(chargingPriorityTargetProvider)
	if !ok || priorityProvider == nil {
		return chargingPriorityContext{}
	}
	var result chargingPriorityContext
	pro3, pro3OK, err := priorityProvider.EcoFlowCloudWriteTarget(ctx)
	if err != nil {
		logger.Warn("failed to resolve DELTA Pro 3 priority target", "error", err)
	} else {
		result.pro3 = pro3
		result.pro3OK = pro3OK
	}
	delta3, delta3OK, err := priorityProvider.Delta3WriteTarget(ctx)
	if err != nil {
		logger.Warn("failed to resolve DELTA 3 Plus priority target", "error", err)
	} else {
		result.delta3 = delta3
		result.delta3OK = delta3OK
	}
	return result
}

func higherPriorityDelta3ChargeCandidateDevice(ctx context.Context, cfg config.Config, status domain.Status, priorityContext chargingPriorityContext, delta3Reader delta3StatusReader, delta3Writer delta3AuxWriteClient, delta3AuxControlCommandRepository delta3AuxControlCommandLogWriter, logger *slog.Logger) string {
	if !priorityContext.delta3PriorityControlEnabled(cfg) || !priorityContext.delta3HasHigherPriorityThanPro3() {
		return ""
	}
	if delta3Reader == nil || delta3Writer == nil || delta3AuxControlCommandRepository == nil {
		return ""
	}
	delta3ControlCfg := api.Delta3ConfigForDevice(cfg, priorityContext.delta3)
	delta3Status := api.Delta3StatusResponse{Available: false, LastError: "DELTA 3 Plus status reader is unavailable"}
	if configReader, ok := delta3Reader.(delta3ConfigStatusReader); ok {
		delta3Status = configReader.CurrentStatusForConfig(ctx, delta3ControlCfg)
	} else {
		delta3Status = delta3Reader.CurrentStatus(ctx)
	}
	delta3Settings := delta3AuxSettingsForDevice(cfg, priorityContext.delta3)
	plan := control.PlanDelta3AuxCharging(control.Delta3AuxPlanInput{
		Status: status,
		Delta3: control.Delta3AuxStatus{
			Available:            delta3Status.Available,
			DeviceType:           delta3Status.DeviceType,
			SOC:                  delta3Status.SOC,
			ACInW:                delta3Status.ACInW,
			ACOutW:               delta3Status.ACOutW,
			ACChargeLimitW:       delta3Status.ACChargeLimitW,
			MaxChargeSoc:         delta3Status.MaxChargeSoc,
			BackupReserveSoc:     delta3Status.BackupReserveSoc,
			BackupReserveEnabled: delta3Status.BackupReserveEnabled,
			ACOutputEnabled:      delta3Status.ACOutputEnabled,
			LastError:            delta3Status.LastError,
		},
		IgnorePro3Wait: true,
	}, delta3Settings, cfg.ControlSettings)
	if !plan.ShouldAdjustACChargeLimit && !plan.ShouldSetBackupReserve {
		return ""
	}
	previous, err := delta3AuxControlCommandRepository.LatestDelta3AuxControlWriteCandidateLog(ctx)
	if err != nil {
		logger.Warn("failed to load latest DELTA 3 Plus aux command log for priority decision", "error", err)
		return ""
	}
	previousReserve, err := delta3AuxControlCommandRepository.LatestDelta3AuxReserveCommandLog(ctx)
	if err != nil {
		logger.Warn("failed to load latest DELTA 3 Plus reserve command log for priority decision", "error", err)
		return ""
	}
	previewStatus := status
	previewStatus.Delta3AuxPlan = &plan
	commandLog := control.EvaluateDelta3AuxCommandGuard(control.Delta3AuxCommandGuardInput{
		Status:                 previewStatus,
		MockMode:               cfg.MockMode,
		SimulationMode:         cfg.SimulationMode,
		EnableRealControl:      cfg.EnableRealControl,
		AutoControl:            cfg.AutoControlEnabled,
		ConfirmEcoFlowWrite:    cfg.ConfirmEcoFlowWrite,
		RealControlTrialActive: realControlTrialActive(cfg),
		Delta3ReadEnabled:      cfg.Delta3ReadEnabled,
		Delta3ControlEnabled:   true,
		AllowAutoControlWrite:  cfg.Delta3AllowAutoWrite,
		Execute:                cfg.Delta3ExecuteWrite,
		AllowPrivateAPIWrite:   cfg.Delta3AllowPrivateWrite,
		Previous:               previous,
		PreviousReserve:        previousReserve,
	}, delta3Settings)
	if !commandLog.WouldWrite {
		return ""
	}
	return chargingDeviceName(priorityContext.delta3)
}

func (c chargingPriorityContext) ignorePro3WaitForDelta3(cfg config.Config) bool {
	return c.delta3PriorityControlEnabled(cfg) && (!c.pro3OK || c.delta3HasHigherPriorityThanPro3())
}

func (c chargingPriorityContext) delta3PriorityControlEnabled(cfg config.Config) bool {
	return c.delta3OK && cfg.Delta3Aux.Enabled && cfg.Delta3ReadEnabled
}

func (c chargingPriorityContext) delta3HasHigherPriorityThanPro3() bool {
	return c.pro3OK && c.delta3OK && c.delta3.Priority < c.pro3.Priority
}

func chargingDeviceName(device domain.ChargingDevice) string {
	if strings.TrimSpace(device.Name) != "" {
		return strings.TrimSpace(device.Name)
	}
	if strings.TrimSpace(device.Kind) != "" {
		return strings.TrimSpace(device.Kind)
	}
	return "unknown"
}

func powerLogFromStatus(status domain.Status, command commandStatus) domain.PowerLog {
	return domain.PowerLog{
		MeasuredAt:         status.UpdatedAt,
		GridW:              status.GridW,
		ImportW:            status.ImportW,
		ExportW:            status.ExportW,
		BatterySoc:         intPtr(status.BatterySoc),
		BatteryInputW:      intPtr(status.BatteryInputW),
		BatteryOutputW:     intPtr(status.BatteryOutputW),
		ACChargeLimitW:     intPtr(status.ACChargeLimitW),
		TargetChargeW:      status.TargetChargeW,
		ActualCommandW:     command.ActualCommandW,
		DecisionReason:     status.LastDecisionReason,
		Mode:               status.Mode,
		CommandSent:        command.CommandSent,
		ErrorMessage:       status.LastError,
		EcoFlowDiagnostics: status.EcoFlowDiagnostics,
		CreatedAt:          status.UpdatedAt,
	}
}

func intPtr(value int) *int {
	return &value
}
