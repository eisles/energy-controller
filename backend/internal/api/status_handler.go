package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/domain"
)

func statusHandler(provider StatusProvider, logger *slog.Logger, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := provider.CurrentStatus(r.Context())
		if err != nil {
			logger.Error("failed to get current status", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get current status"})
			return
		}
		applyRealControlTrialStatus(&status, cfg)
		status.ControlWriteReadiness = controlWriteReadiness(cfg, status.RealControlTrialActive)
		logger.Info("current status read", "mode", status.Mode, "state", status.State, "gridW", status.GridW, "targetChargeW", status.TargetChargeW, "reason", status.LastDecisionReason)
		writeJSON(w, http.StatusOK, status)
	}
}

func applyRealControlTrialStatus(status *domain.Status, cfg config.Config) {
	if status == nil || cfg.RealControlTrialUntil.IsZero() {
		return
	}
	now := time.Now()
	if cfg.Clock != nil {
		now = cfg.Clock.Now()
	}
	until := cfg.RealControlTrialUntil
	status.RealControlTrialUntil = &until
	if now.Before(until) {
		status.RealControlTrialActive = true
		status.RealControlTrialRemainingSeconds = int64(until.Sub(now).Seconds())
	}
}

func controlWriteReadiness(cfg config.Config, realControlTrialActive bool) *domain.ControlWriteReadiness {
	gates := domain.ControlWriteGates{
		MockMode:                    cfg.MockMode,
		SimulationMode:              cfg.SimulationMode,
		EnableRealControl:           cfg.EnableRealControl,
		AutoControlEnabled:          cfg.AutoControlEnabled,
		ConfirmEcoFlowWriteAccepted: cfg.ConfirmEcoFlowWrite == "I_UNDERSTAND",
		RealControlTrialConfigured:  !cfg.RealControlTrialUntil.IsZero(),
		RealControlTrialActive:      realControlTrialActive,
		Delta3ReadEnabled:           cfg.Delta3ReadEnabled,
		Delta3AuxEnabled:            cfg.Delta3Aux.Enabled,
		Delta3ExecuteWrite:          cfg.Delta3ExecuteWrite,
		Delta3AllowPrivateWrite:     cfg.Delta3AllowPrivateWrite,
		Delta3AllowAutoWrite:        cfg.Delta3AllowAutoWrite,
	}
	reasons := make([]string, 0, 6)
	if gates.MockMode {
		reasons = append(reasons, "mock mode keeps device write disabled")
	}
	if gates.SimulationMode {
		reasons = append(reasons, "simulation mode keeps device write disabled")
	}
	if !gates.EnableRealControl {
		reasons = append(reasons, "ENABLE_REAL_CONTROL=false keeps device write disabled")
	}
	if !gates.AutoControlEnabled {
		reasons = append(reasons, "auto control disabled keeps device write disabled")
	}
	if !gates.ConfirmEcoFlowWriteAccepted {
		reasons = append(reasons, "CONFIRM_ECOFLOW_WRITE is not I_UNDERSTAND")
	}
	if !gates.RealControlTrialConfigured || !gates.RealControlTrialActive {
		reasons = append(reasons, "real control trial window inactive")
	}
	ready := len(reasons) == 0
	mode := "dry-run"
	if ready {
		mode = "ready"
	}
	return &domain.ControlWriteReadiness{
		Ready:   ready,
		Mode:    mode,
		Reasons: reasons,
		Gates:   gates,
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
