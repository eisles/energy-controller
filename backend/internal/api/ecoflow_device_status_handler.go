package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/ecoflow"
	"github.com/eisles/energy-controller/backend/internal/ecoflowdeveloper"
	"github.com/eisles/energy-controller/backend/internal/ecoflowprivate"
)

const (
	delta3StatusSuccessCacheTTL = 30 * time.Second
	delta3StatusErrorCacheTTL   = 5 * time.Minute
	delta3StatusBusyBackoffTTL  = 10 * time.Minute
	deviceStatusReadTimeout     = 3 * time.Second
	delta3CycleSuccessCacheTTL  = 10 * time.Minute
	delta3CycleErrorCacheTTL    = 10 * time.Second
	cycleCountCandidateSource   = "ecoflow_private_mqtt_candidate"
)

type preferredCycleCountCandidateField struct {
	CmdFunc int
	CmdID   int
	Field   int
	Reason  string
}

var preferredCycleCountCandidateFields = map[string][]preferredCycleCountCandidateField{
	"DELTA_3": {
		{CmdFunc: 254, CmdID: 21, Field: 427, Reason: "DELTA 3 Plus observed private MQTT candidate; not accepted cycleCount"},
		{CmdFunc: 254, CmdID: 22, Field: 280, Reason: "DELTA 3 Plus secondary observed private MQTT candidate; not accepted cycleCount"},
	},
	"DELTA_3_PLUS": {
		{CmdFunc: 254, CmdID: 21, Field: 427, Reason: "DELTA 3 Plus observed private MQTT candidate; not accepted cycleCount"},
		{CmdFunc: 254, CmdID: 22, Field: 280, Reason: "DELTA 3 Plus secondary observed private MQTT candidate; not accepted cycleCount"},
	},
	"DELTA_3_MAX_PLUS": {
		{CmdFunc: 32, CmdID: 50, Field: 85, Reason: "DELTA 3 Max Plus observed private MQTT candidate; not accepted cycleCount"},
		{CmdFunc: 32, CmdID: 50, Field: 86, Reason: "DELTA 3 Max Plus secondary observed private MQTT candidate; not accepted cycleCount"},
	},
}

type delta3ProbeClient interface {
	Probe(ctx context.Context) (ecoflowprivate.Status, error)
}

type ecoFlowCloudBatteryReader interface {
	GetBatteryStatus(ctx context.Context) (domain.BatteryStatus, error)
}

type ecoFlowDeveloperCycleReader interface {
	ReadCycleStatus(ctx context.Context) (ecoflowdeveloper.CycleStatus, error)
}

type Delta3StatusTargetProvider interface {
	Delta3ReadTarget(ctx context.Context) (domain.ChargingDevice, bool, error)
}

type DeviceStatusStore interface {
	ListChargingDevices(ctx context.Context) ([]domain.ChargingDevice, error)
}

type Delta3StatusResponse struct {
	Available                 bool                          `json:"available"`
	DeviceType                string                        `json:"deviceType,omitempty"`
	SOC                       *int                          `json:"soc,omitempty"`
	ACInW                     *int                          `json:"acInW,omitempty"`
	ACOutW                    *int                          `json:"acOutW,omitempty"`
	ACChargeLimitW            *int                          `json:"acChargeLimitW,omitempty"`
	GridBypassDisabled        *bool                         `json:"gridBypassDisabled,omitempty"`
	ACOutputEnabled           *bool                         `json:"acOutputEnabled,omitempty"`
	ACOutput1Enabled          *bool                         `json:"acOutput1Enabled,omitempty"`
	ACOutput2Enabled          *bool                         `json:"acOutput2Enabled,omitempty"`
	ACOutputProtectionChannel *int                          `json:"acOutputProtectionChannel,omitempty"`
	MaxChargeSoc              *int                          `json:"maxChargeSoc,omitempty"`
	MinDischargeSoc           *int                          `json:"minDischargeSoc,omitempty"`
	BackupReserveSoc          *int                          `json:"backupReserveSoc,omitempty"`
	BackupReserveEnabled      *bool                         `json:"backupReserveEnabled,omitempty"`
	TOUModeEnabled            *bool                         `json:"touModeEnabled,omitempty"`
	SelfPoweredEnabled        *bool                         `json:"selfPoweredEnabled,omitempty"`
	ScheduledEnabled          *bool                         `json:"scheduledEnabled,omitempty"`
	IntelligentEnabled        *bool                         `json:"intelligentEnabled,omitempty"`
	CycleCount                *int                          `json:"cycleCount,omitempty"`
	CycleCountSource          string                        `json:"cycleCountSource,omitempty"`
	CycleCountCandidate       *CycleCountCandidateResponse  `json:"cycleCountCandidate,omitempty"`
	CycleCountCandidates      []CycleCountCandidateResponse `json:"cycleCountCandidates,omitempty"`
	UpdatedAt                 string                        `json:"updatedAt,omitempty"`
	LastError                 string                        `json:"lastError,omitempty"`
	Cached                    bool                          `json:"cached,omitempty"`
	TelemetryDiagnostics      *PrivateTelemetryDiagnostics  `json:"telemetryDiagnostics,omitempty"`
}

type CycleCountCandidateResponse struct {
	Value      int    `json:"value"`
	Source     string `json:"source"`
	CmdFunc    int    `json:"cmdFunc"`
	CmdID      int    `json:"cmdId"`
	Field      int    `json:"field"`
	Confidence string `json:"confidence"`
	Reason     string `json:"reason"`
}

type PrivateTelemetryDiagnostics struct {
	DecodedMessages       int                            `json:"decodedMessages"`
	UnsupportedMessages   int                            `json:"unsupportedMessages"`
	ReplyCount            int                            `json:"replyCount"`
	InspectErrorCount     int                            `json:"inspectErrorCount,omitempty"`
	LastInspectError      string                         `json:"lastInspectError,omitempty"`
	FieldCount            int                            `json:"fieldCount"`
	FieldSummaryTruncated bool                           `json:"fieldSummaryTruncated,omitempty"`
	FieldSummaries        []PrivateTelemetryFieldSummary `json:"fieldSummaries,omitempty"`
}

type PrivateTelemetryFieldSummary struct {
	MessageIndex int    `json:"messageIndex"`
	CmdFunc      int    `json:"cmdFunc"`
	CmdID        int    `json:"cmdId"`
	Field        int    `json:"field"`
	Wire         int    `json:"wire"`
	Value        string `json:"value"`
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

type ecoFlowDeveloperCycleCacheEntry struct {
	response   ecoflowdeveloper.CycleStatus
	cacheUntil time.Time
}

type Delta3StatusReader struct {
	cfg                                config.Config
	logger                             *slog.Logger
	client                             delta3ProbeClient
	clientFactory                      func(config.Config) delta3ProbeClient
	ecoFlowCloudReaderFactory          func(ecoflow.Config) ecoFlowCloudBatteryReader
	ecoFlowDeveloperCycleReaderFactory func(ecoflowdeveloper.Config) ecoFlowDeveloperCycleReader
	targetProvider                     Delta3StatusTargetProvider
	now                                func() time.Time
	mu                                 sync.Mutex
	cache                              map[string]delta3StatusCacheEntry
	ecoFlowCloudCache                  map[string]ecoFlowCloudStatusCacheEntry
	ecoFlowDeveloperCycleCache         map[string]ecoFlowDeveloperCycleCacheEntry
	delta3ProbeLocks                   map[string]*sync.Mutex
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
		ecoFlowDeveloperCycleReaderFactory: func(cfg ecoflowdeveloper.Config) ecoFlowDeveloperCycleReader {
			return ecoflowdeveloper.NewClient(cfg)
		},
		now:                        time.Now,
		cache:                      make(map[string]delta3StatusCacheEntry),
		ecoFlowCloudCache:          make(map[string]ecoFlowCloudStatusCacheEntry),
		ecoFlowDeveloperCycleCache: make(map[string]ecoFlowDeveloperCycleCacheEntry),
		delta3ProbeLocks:           make(map[string]*sync.Mutex),
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
	responses := make([]DeviceStatusResponse, len(devices))
	var wg sync.WaitGroup
	for i, device := range devices {
		i, device := i, device
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses[i] = deviceStatusResponse(device, r.currentDeviceStatus(ctx, device))
		}()
	}
	wg.Wait()
	return responses
}

func (r *Delta3StatusReader) CurrentStatusForConfig(ctx context.Context, cfg config.Config) Delta3StatusResponse {
	return r.currentStatusForConfig(ctx, cfg, true, false)
}

func (r *Delta3StatusReader) currentDeviceStatus(ctx context.Context, device domain.ChargingDevice) Delta3StatusResponse {
	status := deviceStatusNotAvailable(device)
	if canReadEcoFlowPrivateMQTTStatus(device) {
		cfg := Delta3ConfigForDevice(r.cfg, device)
		cfg.Delta3Timeout = deviceStatusTimeout(r.cfg)
		status = r.currentStatusForConfig(ctx, cfg, false, true)
		cycleCtx, cancel := context.WithTimeout(ctx, deviceStatusTimeout(r.cfg))
		defer cancel()
		return r.augmentCycleCountFromDeveloperMQTT(cycleCtx, status, device)
	} else if canReadEcoFlowCloudStatus(device) {
		deviceCtx, cancel := context.WithTimeout(ctx, deviceStatusTimeout(r.cfg))
		defer cancel()
		cfg := EcoFlowCloudConfigForDevice(r.cfg, device)
		status = r.currentEcoFlowCloudStatusForConfig(deviceCtx, cfg, device.DeviceType)
		return r.augmentCycleCountFromDeveloperMQTT(deviceCtx, status, device)
	}
	return r.augmentCycleCountFromDeveloperMQTT(ctx, status, device)
}

func deviceStatusResponse(device domain.ChargingDevice, status Delta3StatusResponse) DeviceStatusResponse {
	return DeviceStatusResponse{
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
	}
}

func deviceStatusTimeout(cfg config.Config) time.Duration {
	timeout := deviceStatusReadTimeout
	if cfg.Delta3Timeout > 0 && cfg.Delta3Timeout < timeout {
		timeout = cfg.Delta3Timeout
	}
	return timeout
}

func (r *Delta3StatusReader) currentStatusForConfig(ctx context.Context, cfg config.Config, allowSharedClient bool, allowStaleFallback bool) Delta3StatusResponse {
	if !cfg.Delta3ReadEnabled {
		return readDelta3Status(ctx, cfg, nil, r.logger)
	}
	key := delta3StatusCacheKey(cfg)
	r.mu.Lock()
	now := r.now()
	entry := r.cache[key]
	if !entry.cacheUntil.IsZero() && now.Before(entry.cacheUntil) {
		response := entry.response
		response.Cached = true
		if !allowStaleFallback && isStaleDelta3StatusResponse(response) {
			response.Available = false
		}
		r.mu.Unlock()
		return response
	}
	r.mu.Unlock()

	probeLock := r.delta3ProbeLock(delta3ProbeLockKey(cfg))
	probeLock.Lock()
	defer probeLock.Unlock()

	r.mu.Lock()
	now = r.now()
	entry = r.cache[key]
	if !entry.cacheUntil.IsZero() && now.Before(entry.cacheUntil) {
		response := entry.response
		response.Cached = true
		if !allowStaleFallback && isStaleDelta3StatusResponse(response) {
			response.Available = false
		}
		r.mu.Unlock()
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
	previous := entry.response
	r.mu.Unlock()

	readCtx := ctx
	cancel := func() {}
	if cfg.Delta3Timeout > 0 {
		readCtx, cancel = context.WithTimeout(ctx, cfg.Delta3Timeout)
	}
	response := readDelta3Status(readCtx, cfg, client, r.logger)
	cancel()

	r.mu.Lock()
	defer r.mu.Unlock()
	entry = r.cache[key]
	entry.client = client
	if response, ok := freshAvailableDelta3CacheResponse(entry, now); ok {
		r.cache[key] = entry
		return response
	}
	if entry.response.Available {
		previous = entry.response
	}
	if allowStaleFallback && shouldUseStaleDelta3StatusResponse(response) && previous.Available {
		stale := previous
		stale.Cached = true
		if strings.TrimSpace(response.LastError) != "" {
			stale.LastError = "refresh failed: " + response.LastError
		}
		entry.response = stale
		entry.cacheUntil = now.Add(delta3StatusCacheTTL(response))
		r.cache[key] = entry
		return stale
	}
	if shouldCacheDelta3StatusResponse(response) {
		entry.response = response
		entry.cacheUntil = now.Add(delta3StatusCacheTTL(response))
		r.cache[key] = entry
	}
	return response
}

func (r *Delta3StatusReader) delta3ProbeLock(key string) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	lock := r.delta3ProbeLocks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		r.delta3ProbeLocks[key] = lock
	}
	return lock
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

func EcoFlowDeveloperMQTTConfigForDevice(cfg config.Config, device domain.ChargingDevice) ecoflowdeveloper.Config {
	return ecoflowdeveloper.Config{
		AccessKey:      cfg.EcoFlowAccessKey,
		SecretKey:      cfg.EcoFlowSecretKey,
		BaseURL:        cfg.EcoFlowBaseURL,
		PrivateAPIHost: cfg.Delta3PrivateAPIHost,
		Email:          cfg.Delta3PrivateEmail,
		Password:       cfg.Delta3PrivatePassword,
		DeviceSN:       strings.TrimSpace(device.DeviceSN),
		MQTTClientID:   cfg.Delta3MQTTClientID,
		Timeout:        cfg.Delta3Timeout,
	}
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

func canReadEcoFlowDeveloperMQTTCycle(device domain.ChargingDevice) bool {
	return device.Enabled &&
		device.Provider == "ecoflow" &&
		device.Kind == "ecoflow_delta_pro3" &&
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

func delta3ProbeLockKey(cfg config.Config) string {
	mqttClientID := strings.TrimSpace(cfg.Delta3MQTTClientID)
	if mqttClientID != "" {
		return strings.Join([]string{
			"mqtt-client",
			strings.TrimSpace(cfg.Delta3PrivateAPIHost),
			strings.ToLower(strings.TrimSpace(cfg.Delta3PrivateEmail)),
			mqttClientID,
		}, "\x00")
	}
	return "device\x00" + delta3StatusCacheKey(cfg)
}

func ecoFlowCloudStatusCacheKey(cfg config.Config) string {
	return cfg.EcoFlowBaseURL + "\x00" + cfg.EcoFlowDeviceSN
}

func ecoFlowDeveloperCycleCacheKey(cfg ecoflowdeveloper.Config) string {
	return cfg.PrivateAPIHost + "\x00" + cfg.DeviceSN
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

func shouldUseStaleDelta3StatusResponse(response Delta3StatusResponse) bool {
	if response.Available {
		return false
	}
	lastError := strings.ToLower(response.LastError)
	return strings.Contains(lastError, "context deadline exceeded") ||
		strings.Contains(lastError, "timed out") ||
		strings.Contains(lastError, "timeout") ||
		strings.Contains(lastError, "server is too busy")
}

func isStaleDelta3StatusResponse(response Delta3StatusResponse) bool {
	return response.Cached && strings.HasPrefix(response.LastError, "refresh failed:")
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
			Available:            false,
			DeviceType:           cfg.Delta3DeviceType,
			LastError:            fmt.Sprintf("EcoFlow private MQTT returned no supported telemetry fields for %s", cfg.Delta3DeviceType),
			TelemetryDiagnostics: privateTelemetryDiagnostics(status, true),
		}
	}
	return mapDelta3Status(status, time.Now())
}

func (r *Delta3StatusReader) currentEcoFlowCloudStatusForConfig(ctx context.Context, cfg config.Config, deviceType string) Delta3StatusResponse {
	r.mu.Lock()
	now := r.now()
	key := ecoFlowCloudStatusCacheKey(cfg)
	entry := r.ecoFlowCloudCache[key]
	if !entry.cacheUntil.IsZero() && now.Before(entry.cacheUntil) {
		response := entry.response
		response.Cached = true
		r.mu.Unlock()
		return response
	}
	previous := entry.response
	r.mu.Unlock()

	response := readEcoFlowCloudStatus(ctx, cfg, r.ecoFlowCloudReaderFactory, r.logger, deviceType, now)

	r.mu.Lock()
	defer r.mu.Unlock()
	entry = r.ecoFlowCloudCache[key]
	if response, ok := freshAvailableEcoFlowCloudCacheResponse(entry, now); ok {
		return response
	}
	if entry.response.Available {
		previous = entry.response
	}
	if shouldUseStaleDelta3StatusResponse(response) && previous.Available {
		stale := previous
		stale.Cached = true
		if strings.TrimSpace(response.LastError) != "" {
			stale.LastError = "refresh failed: " + response.LastError
		}
		entry.response = stale
		entry.cacheUntil = now.Add(delta3StatusCacheTTL(response))
		r.ecoFlowCloudCache[key] = entry
		return stale
	}
	if shouldCacheDelta3StatusResponse(response) {
		entry.response = response
		entry.cacheUntil = now.Add(delta3StatusCacheTTL(response))
		r.ecoFlowCloudCache[key] = entry
	}
	return response
}

func freshAvailableDelta3CacheResponse(entry delta3StatusCacheEntry, refreshStartedAt time.Time) (Delta3StatusResponse, bool) {
	if entry.cacheUntil.IsZero() || !entry.cacheUntil.After(refreshStartedAt) || !entry.response.Available || isStaleDelta3StatusResponse(entry.response) {
		return Delta3StatusResponse{}, false
	}
	response := entry.response
	response.Cached = true
	return response, true
}

func freshAvailableEcoFlowCloudCacheResponse(entry ecoFlowCloudStatusCacheEntry, refreshStartedAt time.Time) (Delta3StatusResponse, bool) {
	if entry.cacheUntil.IsZero() || !entry.cacheUntil.After(refreshStartedAt) || !entry.response.Available || isStaleDelta3StatusResponse(entry.response) {
		return Delta3StatusResponse{}, false
	}
	response := entry.response
	response.Cached = true
	return response, true
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

func (r *Delta3StatusReader) augmentCycleCountFromDeveloperMQTT(ctx context.Context, status Delta3StatusResponse, device domain.ChargingDevice) Delta3StatusResponse {
	if !status.Available || status.CycleCount != nil || !canReadEcoFlowDeveloperMQTTCycle(device) {
		return status
	}
	cycleStatus, err := r.currentEcoFlowDeveloperCycleStatusForConfig(ctx, EcoFlowDeveloperMQTTConfigForDevice(r.cfg, device))
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("failed to read EcoFlow Developer MQTT cycle count", "deviceKind", device.Kind, "deviceName", device.Name, "error", err)
		}
		return status
	}
	if cycleStatus.CycleCount == nil {
		return status
	}
	status.CycleCount = cycleStatus.CycleCount
	status.CycleCountSource = cycleStatus.CycleCountSource
	return status
}

func (r *Delta3StatusReader) currentEcoFlowDeveloperCycleStatusForConfig(ctx context.Context, cfg ecoflowdeveloper.Config) (ecoflowdeveloper.CycleStatus, error) {
	r.mu.Lock()
	now := r.now()
	key := ecoFlowDeveloperCycleCacheKey(cfg)
	entry := r.ecoFlowDeveloperCycleCache[key]
	if !entry.cacheUntil.IsZero() && now.Before(entry.cacheUntil) {
		response := entry.response
		r.mu.Unlock()
		return response, nil
	}
	previous := entry.response
	r.mu.Unlock()

	status, err := readEcoFlowDeveloperCycleStatus(ctx, cfg, r.ecoFlowDeveloperCycleReaderFactory)

	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		if previous.CycleCount != nil {
			entry.response = previous
			entry.cacheUntil = now.Add(delta3StatusSuccessCacheTTL)
			r.ecoFlowDeveloperCycleCache[key] = entry
			return previous, nil
		}
		entry.response = status
		entry.cacheUntil = now.Add(delta3CycleErrorCacheTTL)
	} else {
		entry.response = status
		entry.cacheUntil = now.Add(delta3CycleSuccessCacheTTL)
	}
	r.ecoFlowDeveloperCycleCache[key] = entry
	return status, err
}

func readEcoFlowDeveloperCycleStatus(ctx context.Context, cfg ecoflowdeveloper.Config, factory func(ecoflowdeveloper.Config) ecoFlowDeveloperCycleReader) (ecoflowdeveloper.CycleStatus, error) {
	if missing := cfg.MissingReadCredentials(); len(missing) > 0 {
		return ecoflowdeveloper.CycleStatus{}, fmt.Errorf("EcoFlow Developer MQTT cycle count missing required env: %v", missing)
	}
	if factory == nil {
		factory = func(cfg ecoflowdeveloper.Config) ecoFlowDeveloperCycleReader {
			return ecoflowdeveloper.NewClient(cfg)
		}
	}
	reader := factory(cfg)
	return reader.ReadCycleStatus(ctx)
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
		CycleCount:           status.CycleCount,
		CycleCountSource:     status.CycleCountSource,
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
	cycleCountCandidates := privateCycleCountCandidates(status)
	return Delta3StatusResponse{
		Available:                 true,
		DeviceType:                status.DeviceType,
		SOC:                       firstIntPtr(status.CMSBatterySoc, status.BMSBatterySoc),
		ACInW:                     status.ACInW,
		ACOutW:                    positiveIntPtr(status.ACOutW),
		ACChargeLimitW:            status.ACChargeLimitW,
		GridBypassDisabled:        status.GridBypassDisabled,
		ACOutputEnabled:           status.ACOutputEnabled,
		ACOutput1Enabled:          status.ACOutput1Enabled,
		ACOutput2Enabled:          status.ACOutput2Enabled,
		ACOutputProtectionChannel: status.ACOutputProtectionChannel,
		MaxChargeSoc:              status.MaxChargeSoc,
		MinDischargeSoc:           status.MinDischargeSoc,
		BackupReserveSoc:          status.BackupReserveSoc,
		BackupReserveEnabled:      status.BackupReserveEnabled,
		CycleCount:                status.CycleCount,
		CycleCountSource:          status.CycleCountSource,
		CycleCountCandidate:       primaryCycleCountCandidate(cycleCountCandidates),
		CycleCountCandidates:      cycleCountCandidates,
		UpdatedAt:                 now.Format(time.RFC3339),
		TelemetryDiagnostics:      privateTelemetryDiagnostics(status, false),
	}
}

func privateCycleCountCandidate(status ecoflowprivate.Status) *CycleCountCandidateResponse {
	return primaryCycleCountCandidate(privateCycleCountCandidates(status))
}

func primaryCycleCountCandidate(candidates []CycleCountCandidateResponse) *CycleCountCandidateResponse {
	for _, candidate := range candidates {
		if candidate.Value < 2 {
			continue
		}
		candidate := candidate
		return &candidate
	}
	return nil
}

func privateCycleCountCandidates(status ecoflowprivate.Status) []CycleCountCandidateResponse {
	if status.CycleCount != nil {
		return nil
	}
	preferredFields := preferredCycleCountCandidateFields[strings.ToUpper(strings.TrimSpace(status.DeviceType))]
	if len(preferredFields) == 0 {
		return nil
	}
	candidates := make([]ecoflowprivate.CycleFieldCandidate, 0, len(status.CycleFieldCandidates)+len(status.FieldSummaries))
	candidates = append(candidates, status.CycleFieldCandidates...)
	candidates = append(candidates, preferredCycleCountCandidatesFromFieldSummaries(preferredFields, status.FieldSummaries)...)
	return privateCycleCountCandidatesFromCandidates(preferredFields, candidates)
}

func privateCycleCountCandidateFromCandidates(preferredFields []preferredCycleCountCandidateField, candidates []ecoflowprivate.CycleFieldCandidate) *CycleCountCandidateResponse {
	return primaryCycleCountCandidate(privateCycleCountCandidatesFromCandidates(preferredFields, candidates))
}

func privateCycleCountCandidatesFromCandidates(preferredFields []preferredCycleCountCandidateField, candidates []ecoflowprivate.CycleFieldCandidate) []CycleCountCandidateResponse {
	if len(preferredFields) == 0 || len(candidates) == 0 {
		return nil
	}
	byField := make(map[string]ecoflowprivate.CycleFieldCandidate, len(candidates))
	for _, candidate := range candidates {
		key := cycleFieldCandidateKey(candidate.CmdFunc, candidate.CmdID, candidate.Field)
		if _, exists := byField[key]; !exists {
			byField[key] = candidate
		}
	}
	responses := make([]CycleCountCandidateResponse, 0, len(preferredFields))
	for _, preferred := range preferredFields {
		candidate, ok := byField[cycleFieldCandidateKey(preferred.CmdFunc, preferred.CmdID, preferred.Field)]
		if !ok {
			continue
		}
		responses = append(responses, CycleCountCandidateResponse{
			Value:      candidate.Value,
			Source:     cycleCountCandidateSource,
			CmdFunc:    candidate.CmdFunc,
			CmdID:      candidate.CmdID,
			Field:      candidate.Field,
			Confidence: "candidate",
			Reason:     preferred.Reason,
		})
	}
	return responses
}

func preferredCycleCountCandidatesFromFieldSummaries(preferredFields []preferredCycleCountCandidateField, fields []ecoflowprivate.TelemetryFieldSummary) []ecoflowprivate.CycleFieldCandidate {
	if len(preferredFields) == 0 || len(fields) == 0 {
		return nil
	}
	preferredKeys := make(map[string]struct{}, len(preferredFields))
	for _, preferred := range preferredFields {
		preferredKeys[cycleFieldCandidateKey(preferred.CmdFunc, preferred.CmdID, preferred.Field)] = struct{}{}
	}
	seen := make(map[string]struct{}, len(preferredFields))
	candidates := make([]ecoflowprivate.CycleFieldCandidate, 0, len(preferredFields))
	for _, field := range fields {
		key := cycleFieldCandidateKey(field.CmdFunc, field.CmdID, field.Field)
		if _, ok := preferredKeys[key]; !ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		value, err := strconv.Atoi(field.Value)
		if err != nil || value < 0 {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, ecoflowprivate.CycleFieldCandidate{
			MessageIndex: field.MessageIndex,
			CmdFunc:      field.CmdFunc,
			CmdID:        field.CmdID,
			Field:        field.Field,
			Value:        value,
		})
	}
	return candidates
}

func cycleFieldCandidateKey(cmdFunc int, cmdID int, field int) string {
	return fmt.Sprintf("%d/%d/%d", cmdFunc, cmdID, field)
}

func privateTelemetryDiagnostics(status ecoflowprivate.Status, includeEmpty bool) *PrivateTelemetryDiagnostics {
	if !includeEmpty &&
		status.ReplyCount == 0 &&
		status.DecodedMessages == 0 &&
		status.UnsupportedMessages == 0 &&
		status.InspectErrorCount == 0 &&
		status.FieldCount == 0 &&
		len(status.FieldSummaries) == 0 &&
		!status.FieldSummaryTruncated {
		return nil
	}
	fieldCount := status.FieldCount
	if fieldCount == 0 {
		fieldCount = len(status.FieldSummaries)
	}
	fields := make([]PrivateTelemetryFieldSummary, 0, len(status.FieldSummaries))
	for _, field := range status.FieldSummaries {
		fields = append(fields, PrivateTelemetryFieldSummary{
			MessageIndex: field.MessageIndex,
			CmdFunc:      field.CmdFunc,
			CmdID:        field.CmdID,
			Field:        field.Field,
			Wire:         field.Wire,
			Value:        field.Value,
		})
	}
	return &PrivateTelemetryDiagnostics{
		DecodedMessages:       status.DecodedMessages,
		UnsupportedMessages:   status.UnsupportedMessages,
		ReplyCount:            status.ReplyCount,
		InspectErrorCount:     status.InspectErrorCount,
		LastInspectError:      status.LastInspectError,
		FieldCount:            fieldCount,
		FieldSummaryTruncated: status.FieldSummaryTruncated,
		FieldSummaries:        fields,
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
		status.ACOutput1Enabled != nil ||
		status.ACOutput2Enabled != nil ||
		status.ACOutputProtectionChannel != nil ||
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
