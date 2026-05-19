package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type EcoFlowLoadProvider interface {
	EstimateEcoFlowLoad(ctx context.Context, now time.Time, days int) (domain.EcoFlowLoadEstimate, error)
}

func ecoFlowLoadHandler(provider EcoFlowLoadProvider, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		days := 7
		if rawDays := r.URL.Query().Get("days"); rawDays != "" {
			parsedDays, err := strconv.Atoi(rawDays)
			if err != nil || parsedDays < 1 || parsedDays > 30 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "days must be an integer from 1 to 30"})
				return
			}
			days = parsedDays
		}
		estimate, err := provider.EstimateEcoFlowLoad(r.Context(), time.Now(), days)
		if err != nil {
			logger.Error("failed to estimate EcoFlow load", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to estimate EcoFlow load"})
			return
		}
		writeJSON(w, http.StatusOK, estimate)
	}
}
