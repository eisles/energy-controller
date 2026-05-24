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
	"github.com/eisles/energy-controller/backend/internal/ecoflowdelta3"
)

const (
	delta3StatusSuccessCacheTTL = 2 * time.Minute
	delta3StatusErrorCacheTTL   = 5 * time.Minute
	delta3StatusBusyBackoffTTL  = 10 * time.Minute
)

type delta3ProbeClient interface {
	Probe(ctx context.Context) (ecoflowdelta3.Status, error)
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
	UpdatedAt            string `json:"updatedAt,omitempty"`
	LastError            string `json:"lastError,omitempty"`
	Cached               bool   `json:"cached,omitempty"`
}

func delta3StatusHandler(cfg config.Config, logger *slog.Logger) http.HandlerFunc {
	reader := newDelta3StatusReader(cfg, logger, nil)
	return func(w http.ResponseWriter, r *http.Request) {
		response := reader.CurrentStatus(r.Context())
		writeJSON(w, http.StatusOK, response)
	}
}

type delta3StatusReader struct {
	cfg        config.Config
	logger     *slog.Logger
	client     delta3ProbeClient
	now        func() time.Time
	mu         sync.Mutex
	cached     Delta3StatusResponse
	cacheUntil time.Time
}

func newDelta3StatusReader(cfg config.Config, logger *slog.Logger, client delta3ProbeClient) *delta3StatusReader {
	if client == nil && cfg.Delta3ReadEnabled {
		client = ecoflowdelta3.NewClient(delta3ProbeConfig(cfg))
	}
	return &delta3StatusReader{
		cfg:    cfg,
		logger: logger,
		client: client,
		now:    time.Now,
	}
}

func (r *delta3StatusReader) CurrentStatus(ctx context.Context) Delta3StatusResponse {
	if !r.cfg.Delta3ReadEnabled {
		return readDelta3Status(ctx, r.cfg, r.client, r.logger)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	if !r.cacheUntil.IsZero() && now.Before(r.cacheUntil) {
		response := r.cached
		response.Cached = true
		return response
	}

	response := readDelta3Status(ctx, r.cfg, r.client, r.logger)
	if shouldCacheDelta3StatusResponse(response) {
		r.cached = response
		r.cacheUntil = now.Add(delta3StatusCacheTTL(response))
	}
	return response
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
		client = ecoflowdelta3.NewClient(probeCfg)
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
	return mapDelta3Status(status, time.Now())
}

func delta3ProbeConfig(cfg config.Config) ecoflowdelta3.Config {
	return ecoflowdelta3.Config{
		PrivateAPIHost: cfg.Delta3PrivateAPIHost,
		Email:          cfg.Delta3PrivateEmail,
		Password:       cfg.Delta3PrivatePassword,
		DeviceSN:       cfg.Delta3DeviceSN,
		DeviceType:     cfg.Delta3DeviceType,
		MQTTClientID:   cfg.Delta3MQTTClientID,
		Timeout:        cfg.Delta3Timeout,
	}
}

func mapDelta3Status(status ecoflowdelta3.Status, now time.Time) Delta3StatusResponse {
	return Delta3StatusResponse{
		Available:            true,
		DeviceType:           status.DeviceType,
		SOC:                  firstIntPtr(status.CMSBatterySoc, status.BMSBatterySoc),
		ACInW:                status.ACInW,
		ACOutW:               status.ACOutW,
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

func firstIntPtr(values ...*int) *int {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
