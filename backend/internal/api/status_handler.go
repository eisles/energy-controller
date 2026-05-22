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

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
