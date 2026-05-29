package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/ecoflow"
	"github.com/eisles/energy-controller/backend/internal/ecoflowprivate"
)

const (
	delta3StatusSuccessCacheTTL = 30 * time.Second
	delta3StatusErrorCacheTTL   = 5 * time.Minute
	delta3StatusBusyBackoffTTL  = 10 * time.Minute
)

type delta3ProbeClient interface {
	Probe(ctx context.Context) (ecoflowprivate.Status, error)
}

type ecoFlowCloudBatteryReader interface {
	GetBatteryStatus(ctx context.Context) (domain.BatteryStatus, error)
}

type Delta3StatusTargetProvider interface {
	Delta3ReadTarget(ctx context.Context) (domain.ChargingDevice, bool, error)
}

type DeviceStatusStore interface {
	ListChargingDevices(ctx context.Context) ([]domain.ChargingDevice, error)
}

type Delta3StatusResponse struct {
	Available            bool   `json:"available"`
	DeviceType           string `json:"deviceType,omitempty"`
	SOC                  *int   `json:"soc,omitempty"`
	ACInW                *int   `json:"acInW,omitempty"`
	ACOutW               *int   `json:"acOutW,omitempty"`
	ACChargeLimitW       *int   `json:"acChargeLimitW,omitempty"`
	GridBypassDisabled   *bool  `json:"gridBypassDisabled,omitempty"`
	ACOutputEnabled      *bool  `json:"acOutputEnabled,omitempty"`
	MaxChargeSoc         *int   `json:"maxChargeSoc,omitempty"`
	MinDischargeSoc      *int   `json:"minDischargeSoc,omitempty"`
	BackupReserveSoc     *int   `json:"backupReserveSoc,omitempty"`
	BackupReserveEnabled *bool  `json:"backupReserveEnabled,omitempty"`
	TOUModeEnabled       *bool  `json:"touModeEnabled,omitempty"`
	SelfPoweredEnabled   *bool  `json:"selfPoweredEnabled,omitempty"`
	ScheduledEnabled     *bool  `json:"scheduledEnabled,omitempty"`
	IntelligentEnabled   *bool  `json:"intelligentEnabled,omitempty"`
	UpdatedAt            string `json:"updatedAt,omitempty"`
	LastError            string `json:"lastError,omitempty"`
	Cached               bool   `json:"cached,omitempty"`
}

type DeviceStatusResponse struct {
	ID                   int64                `json:"id"`
	Name                 string               `json:"name"`
	Kind                 string               `json:"kind"`
	Provider             string               `json:"provider"`
	Role                 string               `json:"role"`
	CredentialRef        string               `json:"credentialRef"`
	DeviceSN             string               `json:"deviceSn"`
	DeviceType           string               `json:"deviceType"`
	StatusSource         string               `json:"statusSource"`
	Enabled              bool                 `json:"enabled"`
	Priority             int                  `json:"priority"`
	MinChargeW           int                  `json:"minChargeW"`
	MaxChargeW           int                  `json:"maxChargeW"`
	ChargeStepW          int                  `json:"chargeStepW"`
	CapacityWh           int                  `json:"capacityWh"`
	TargetSoc            int                  `json:"targetSoc"`
	ReserveSoc           int                  `json:"reserveSoc"`
	BackupReserveMinSoc  int                  `json:"backupReserveMinSoc"`
	BackupReserveMaxSoc  int                  `json:"backupReserveMaxSoc"`
	ExpectedDaytimeLoadW int                  `json:"expectedDaytimeLoadW"`
	AutoRecoverACOutput  bool                 `json:"autoRecoverAcOutput"`
	ControlEnabled       bool                 `json:"controlEnabled"`
	Status               Delta3StatusResponse `json:"status"`
}

func delta3StatusHandler(cfg config.Config, logger *slog.Logger, targetProvider Delta3StatusTargetProvider) http.HandlerFunc {
	reader := NewDelta3StatusReaderWithTargetProvider(cfg, logger, targetProvider)
	return func(w http.ResponseWriter, r *http.Request) {
		response := reader.CurrentStatus(r.Context())
		writeJSON(w, http.StatusOK, response)
	}
}

func deviceStatusesHandler(cfg config.Config, logger *slog.Logger, deviceStore DeviceStatusStore) http.HandlerFunc {
	reader := NewDelta3StatusReader(cfg, logger)
	return func(w http.ResponseWriter, r *http.Request) {
		devices, err := deviceStore.ListChargingDevices(r.Context())
		if err != nil {
			logger.Error("failed to list device statuses", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list device statuses"})
			return
		}
		response := reader.CurrentDeviceStatuses(r.Context(), devices)
		writeJSON(w, http.StatusOK, response)
	}
}

type delta3StatusCacheEntry struct {
	client     delta3ProbeClient
	response   Delta3StatusResponse
	cacheUntil time.Time
}

type ecoFlowCloudStatusCacheEntry struct {
	response   Delta3StatusResponse
	cacheUntil time.Time
}

type Delta3StatusReader struct {
	cfg                       config.Config
	logger                    *slog.Logger
	client                    delta3ProbeClient
	clientFactory             func(config.Config) delta3ProbeClient
	ecoFlowCloudReaderFactory func(ecoflow.Config) ecoFlowCloudBatteryReader
	targetProvider            Delta3StatusTargetProvider
	now                       func() time.Time
	mu                        sync.Mutex
	cache                     map[string]delta3StatusCacheEntry
	ecoFlowCloudCache         map[string]ecoFlowCloudStatusCacheEntry
}

func NewDelta3StatusReader(cfg config.Config, logger *slog.Logger) *Delta3StatusReader {
	return newDelta3StatusReader(cfg, logger, nil)
}

func NewDelta3StatusReaderWithTargetProvider(cfg config.Config, logger *slog.Logger, targetProvider Delta3StatusTargetProvider) *Delta3StatusReader {
	reader := newDelta3StatusReader(cfg, logger, nil)
	reader.targetProvider = targetProvider
	return reader
}

func newDelta3StatusReader(cfg config.Config, logger *slog.Logger, client delta3ProbeClient) *Delta3StatusReader {
	if client == nil && cfg.Delta3ReadEnabled {
		client = ecoflowprivate.NewClient(delta3ProbeConfig(cfg))
	}
	return &Delta3StatusReader{
		cfg:    cfg,
		logger: logger,
		client: client,
		clientFactory: func(cfg config.Config) delta3ProbeClient {
			return ecoflowprivate.NewClient(delta3ProbeConfig(cfg))
		},
		ecoFlowCloudReaderFactory: func(cfg ecoflow.Config) ecoFlowCloudBatteryReader {
			return ecoflow.NewSignedClient(cfg)
		},
		now:               time.Now,
		cache:             make(map[string]delta3StatusCacheEntry),
		ecoFlowCloudCache: make(map[string]ecoFlowCloudStatusCacheEntry),
	}
}

func (r *Delta3StatusReader) CurrentStatus(ctx context.Context) Delta3StatusResponse {
	cfg, err := r.readConfig(ctx)
	if err != nil {
		return Delta3StatusResponse{Available: false, LastError: err.Error()}
	}
	return r.CurrentStatusForConfig(ctx, cfg)
}

func (r *Delta3StatusReader) CurrentDeviceStatuses(ctx context.Context, devices []domain.ChargingDevice) []DeviceStatusResponse {
	responses := make([]DeviceStatusResponse, 0, len(devices))
	for _, device := range devices {
		status := deviceStatusNotAvailable(device)
		if canReadEcoFlowPrivateMQTTStatus(device) {
			cfg := Delta3ConfigForDevice(r.cfg, device)
			status = r.currentStatusForConfig(ctx, cfg, false)
		} else if canReadEcoFlowCloudStatus(device) {
			cfg := EcoFlowCloudConfigForDevice(r.cfg, device)
			status = r.currentEcoFlowCloudStatusForConfig(ctx, cfg, device.DeviceType)
		}
		responses = append(responses, DeviceStatusResponse{
			ID:                   device.ID,
			Name:                 device.Name,
			Kind:                 device.Kind,
			Provider:             device.Provider,
			Role:                 device.Role,
			CredentialRef:        device.CredentialRef,
			DeviceSN:             device.DeviceSN,
			DeviceType:           device.DeviceType,
			StatusSource:         device.StatusSource,
			Enabled:              device.Enabled,
			Priority:             device.Priority,
			MinChargeW:           device.MinChargeW,
			MaxChargeW:           device.MaxChargeW,
			ChargeStepW:          device.ChargeStepW,
			CapacityWh:           device.CapacityWh,
			TargetSoc:            device.TargetSoc,
			ReserveSoc:           device.ReserveSoc,
			BackupReserveMinSoc:  device.BackupReserveMinSoc,
			BackupReserveMaxSoc:  device.BackupReserveMaxSoc,
			ExpectedDaytimeLoadW: device.ExpectedDaytimeLoadW,
			AutoRecoverACOutput:  device.AutoRecoverACOutput,
			ControlEnabled:       device.ControlEnabled,
			Status:               status,
		})
	}
	return responses
}

func (r *Delta3StatusReader) CurrentStatusForConfig(ctx context.Context, cfg config.Config) Delta3StatusResponse {
	return r.currentStatusForConfig(ctx, cfg, true)
}

func (r *Delta3StatusReader) currentStatusForConfig(ctx context.Context, cfg config.Config, allowSharedClient bool) Delta3StatusResponse {
	if !cfg.Delta3ReadEnabled {
		return readDelta3Status(ctx, cfg, nil, r.logger)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	key := delta3StatusCacheKey(cfg)
	entry := r.cache[key]
	if !entry.cacheUntil.IsZero() && now.Before(entry.cacheUntil) {
		response := entry.response
		response.Cached = true
		return response
	}

	client := entry.client
	if client == nil {
		if allowSharedClient && r.targetProvider == nil && r.client != nil && delta3StatusCacheKey(cfg) == delta3StatusCacheKey(r.cfg) {
			client = r.client
		} else if r.clientFactory != nil {
			client = r.clientFactory(cfg)
		} else {
			client = ecoflowprivate.NewClient(delta3ProbeConfig(cfg))
		}
	}
	response := readDelta3Status(ctx, cfg, client, r.logger)
	entry.client = client
	if shouldCacheDelta3StatusResponse(response) {
		entry.response = response
		entry.cacheUntil = now.Add(delta3StatusCacheTTL(response))
		r.cache[key] = entry
	}
	return response
}

func (r *Delta3StatusReader) readConfig(ctx context.Context) (config.Config, error) {
	if r.targetProvider == nil {
		return r.cfg, nil
	}
	device, ok, err := r.targetProvider.Delta3ReadTarget(ctx)
	if err != nil {
		return config.Config{}, fmt.Errorf("failed to resolve DELTA 3 Plus target")
	}
	if !ok {
		return r.cfg, nil
	}
	return Delta3ConfigForDevice(r.cfg, device), nil
}

func Delta3ConfigForDevice(cfg config.Config, device domain.ChargingDevice) config.Config {
	if strings.TrimSpace(device.DeviceSN) != "" {
		cfg.Delta3DeviceSN = strings.TrimSpace(device.DeviceSN)
	}
	if strings.TrimSpace(device.DeviceType) != "" {
		cfg.Delta3DeviceType = strings.TrimSpace(device.DeviceType)
	}
	return cfg
}

func EcoFlowCloudConfigForDevice(cfg config.Config, device domain.ChargingDevice) config.Config {
	cfg.EcoFlowDeviceSN = strings.TrimSpace(device.DeviceSN)
	return cfg
}

func canReadEcoFlowPrivateMQTTStatus(device domain.ChargingDevice) bool {
	return device.Enabled &&
		device.Provider == "ecoflow" &&
		(device.Kind == "ecoflow_delta3_plus" || device.Kind == "ecoflow_river2") &&
		device.StatusSource == "ecoflow_private_mqtt" &&
		strings.TrimSpace(device.DeviceSN) != "" &&
		device.SupportsSocRead
}

func canReadEcoFlowCloudStatus(device domain.ChargingDevice) bool {
	return device.Enabled &&
		device.Provider == "ecoflow" &&
		device.Kind == "ecoflow_delta_pro3" &&
		device.StatusSource == "ecoflow_cloud" &&
		strings.TrimSpace(device.DeviceSN) != "" &&
		device.SupportsSocRead
}

func deviceStatusNotAvailable(device domain.ChargingDevice) Delta3StatusResponse {
	reason := "read-only status is not implemented for this device"
	if device.Provider == "ecoflow" && (device.Kind == "ecoflow_delta3_plus" || device.Kind == "ecoflow_delta_pro3" || device.Kind == "ecoflow_river2") && strings.TrimSpace(device.DeviceSN) == "" {
		reason = "device SN is not configured"
	}
	return Delta3StatusResponse{
		Available:  false,
		DeviceType: device.DeviceType,
		LastError:  reason,
	}
}

func delta3StatusCacheKey(cfg config.Config) string {
	return cfg.Delta3DeviceType + "\x00" + cfg.Delta3DeviceSN
}

func ecoFlowCloudStatusCacheKey(cfg config.Config) string {
	return cfg.EcoFlowBaseURL + "\x00" + cfg.EcoFlowDeviceSN
}

func shouldCacheDelta3StatusResponse(response Delta3StatusResponse) bool {
	if response.Available {
		return true
	}
	lastError := strings.ToLower(response.LastError)
	if strings.Contains(lastError, "context canceled") {
		return false
	}
	return true
}

func delta3StatusCacheTTL(response Delta3StatusResponse) time.Duration {
	if response.Available {
		return delta3StatusSuccessCacheTTL
	}
	if strings.Contains(strings.ToLower(response.LastError), "server is too busy") {
		return delta3StatusBusyBackoffTTL
	}
	return delta3StatusErrorCacheTTL
}

func readDelta3Status(ctx context.Context, cfg config.Config, client delta3ProbeClient, logger *slog.Logger) Delta3StatusResponse {
	if !cfg.Delta3ReadEnabled {
		return Delta3StatusResponse{Available: false, LastError: "ECOFLOW_DELTA3_READ_ENABLED=false"}
	}
	probeCfg := delta3ProbeConfig(cfg)
	if missing := probeCfg.MissingReadCredentials(); len(missing) > 0 {
		return Delta3StatusResponse{
			Available:  false,
			DeviceType: cfg.Delta3DeviceType,
			LastError:  fmt.Sprintf("missing required env: %v", missing),
		}
	}
	if client == nil {
		client = ecoflowprivate.NewClient(probeCfg)
	}
	timeout := cfg.Delta3Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout+5*time.Second)
	defer cancel()
	status, err := client.Probe(probeCtx)
	if err != nil {
		if logger != nil {
			logger.Warn("failed to read DELTA_3 status", "error", err)
		}
		return Delta3StatusResponse{
			Available:  false,
			DeviceType: cfg.Delta3DeviceType,
			LastError:  err.Error(),
		}
	}
	if !hasReadablePrivateMQTTTelemetry(status) {
		return Delta3StatusResponse{
			Available:  false,
			DeviceType: cfg.Delta3DeviceType,
			LastError:  fmt.Sprintf("EcoFlow private MQTT returned no supported telemetry fields for %s", cfg.Delta3DeviceType),
		}
	}
	return mapDelta3Status(status, time.Now())
}

func (r *Delta3StatusReader) currentEcoFlowCloudStatusForConfig(ctx context.Context, cfg config.Config, deviceType string) Delta3StatusResponse {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	key := ecoFlowCloudStatusCacheKey(cfg)
	entry := r.ecoFlowCloudCache[key]
	if !entry.cacheUntil.IsZero() && now.Before(entry.cacheUntil) {
		response := entry.response
		response.Cached = true
		return response
	}

	response := readEcoFlowCloudStatus(ctx, cfg, r.ecoFlowCloudReaderFactory, r.logger, deviceType, now)
	if shouldCacheDelta3StatusResponse(response) {
		entry.response = response
		entry.cacheUntil = now.Add(delta3StatusCacheTTL(response))
		r.ecoFlowCloudCache[key] = entry
	}
	return response
}

func readEcoFlowCloudStatus(ctx context.Context, cfg config.Config, factory func(ecoflow.Config) ecoFlowCloudBatteryReader, logger *slog.Logger, deviceType string, now time.Time) Delta3StatusResponse {
	if cfg.EcoFlowAccessKey == "" || cfg.EcoFlowSecretKey == "" || cfg.EcoFlowDeviceSN == "" {
		return Delta3StatusResponse{
			Available:  false,
			DeviceType: deviceType,
			LastError:  "EcoFlow access key, secret key, or device SN is empty",
		}
	}
	if factory == nil {
		factory = func(cfg ecoflow.Config) ecoFlowCloudBatteryReader {
			return ecoflow.NewSignedClient(cfg)
		}
	}
	reader := factory(ecoflow.Config{
		AccessKey: cfg.EcoFlowAccessKey,
		SecretKey: cfg.EcoFlowSecretKey,
		DeviceSN:  cfg.EcoFlowDeviceSN,
		BaseURL:   cfg.EcoFlowBaseURL,
	})
	status, err := reader.GetBatteryStatus(ctx)
	if err != nil {
		if logger != nil {
			logger.Warn("failed to read EcoFlow Cloud status", "error", err)
		}
		return Delta3StatusResponse{
			Available:  false,
			DeviceType: deviceType,
			LastError:  err.Error(),
		}
	}
	return mapEcoFlowCloudStatus(status, deviceType, now)
}

func mapEcoFlowCloudStatus(status domain.BatteryStatus, deviceType string, now time.Time) Delta3StatusResponse {
	return Delta3StatusResponse{
		Available:            status.IsOnline,
		DeviceType:           deviceType,
		SOC:                  intPtr(status.Soc),
		ACInW:                intPtr(status.InputW),
		ACOutW:               intPtr(status.OutputW),
		ACChargeLimitW:       intPtr(status.ACChargeLimitW),
		MaxChargeSoc:         status.MaxChargeSoc,
		MinDischargeSoc:      status.MinDischargeSoc,
		BackupReserveSoc:     status.BackupReserveSoc,
		BackupReserveEnabled: status.EnergyBackupEnabled,
		TOUModeEnabled:       status.TOUModeEnabled,
		SelfPoweredEnabled:   status.SelfPoweredEnabled,
		ScheduledEnabled:     status.ScheduledEnabled,
		IntelligentEnabled:   status.IntelligentEnabled,
		UpdatedAt:            now.Format(time.RFC3339),
	}
}

func delta3ProbeConfig(cfg config.Config) ecoflowprivate.Config {
	return ecoflowprivate.Config{
		PrivateAPIHost: cfg.Delta3PrivateAPIHost,
		Email:          cfg.Delta3PrivateEmail,
		Password:       cfg.Delta3PrivatePassword,
		DeviceSN:       cfg.Delta3DeviceSN,
		DeviceType:     cfg.Delta3DeviceType,
		MQTTClientID:   cfg.Delta3MQTTClientID,
		Timeout:        cfg.Delta3Timeout,
	}
}

func intPtr(value int) *int {
	return &value
}

func mapDelta3Status(status ecoflowprivate.Status, now time.Time) Delta3StatusResponse {
	return Delta3StatusResponse{
		Available:            true,
		DeviceType:           status.DeviceType,
		SOC:                  firstIntPtr(status.CMSBatterySoc, status.BMSBatterySoc),
		ACInW:                status.ACInW,
		ACOutW:               positiveIntPtr(status.ACOutW),
		ACChargeLimitW:       status.ACChargeLimitW,
		GridBypassDisabled:   status.GridBypassDisabled,
		ACOutputEnabled:      status.ACOutputEnabled,
		MaxChargeSoc:         status.MaxChargeSoc,
		MinDischargeSoc:      status.MinDischargeSoc,
		BackupReserveSoc:     status.BackupReserveSoc,
		BackupReserveEnabled: status.BackupReserveEnabled,
		UpdatedAt:            now.Format(time.RFC3339),
	}
}

func hasReadablePrivateMQTTTelemetry(status ecoflowprivate.Status) bool {
	return status.CMSBatterySoc != nil ||
		status.BMSBatterySoc != nil ||
		status.ACInW != nil ||
		status.ACOutW != nil ||
		status.ACChargeLimitW != nil ||
		status.GridBypassDisabled != nil ||
		status.ACOutputEnabled != nil ||
		status.MaxChargeSoc != nil ||
		status.MinDischargeSoc != nil ||
		status.BackupReserveSoc != nil ||
		status.BackupReserveEnabled != nil
}

func firstIntPtr(values ...*int) *int {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func positiveIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	positive := *value
	if positive < 0 {
		positive = -positive
	}
	return &positive
}
