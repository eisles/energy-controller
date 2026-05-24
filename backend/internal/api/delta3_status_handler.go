package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/ecoflowdelta3"
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
}

func delta3StatusHandler(cfg config.Config, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := readDelta3Status(r.Context(), cfg, nil, logger)
		writeJSON(w, http.StatusOK, response)
	}
}

func readDelta3Status(ctx context.Context, cfg config.Config, client delta3ProbeClient, logger *slog.Logger) Delta3StatusResponse {
	if !cfg.Delta3ReadEnabled {
		return Delta3StatusResponse{Available: false, LastError: "ECOFLOW_DELTA3_READ_ENABLED=false"}
	}
	probeCfg := ecoflowdelta3.Config{
		PrivateAPIHost: cfg.Delta3PrivateAPIHost,
		Email:          cfg.Delta3PrivateEmail,
		Password:       cfg.Delta3PrivatePassword,
		DeviceSN:       cfg.Delta3DeviceSN,
		DeviceType:     cfg.Delta3DeviceType,
		MQTTClientID:   cfg.Delta3MQTTClientID,
		Timeout:        cfg.Delta3Timeout,
	}
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
