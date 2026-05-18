package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func statusHandler(provider StatusProvider, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := provider.CurrentStatus(r.Context())
		if err != nil {
			logger.Error("failed to get current status", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get current status"})
			return
		}
		logger.Info("control decision", "mode", status.Mode, "state", status.State, "gridW", status.GridW, "targetChargeW", status.TargetChargeW, "reason", status.LastDecisionReason)
		writeJSON(w, http.StatusOK, status)
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
