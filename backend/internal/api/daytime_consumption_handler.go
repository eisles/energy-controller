package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type DaytimeConsumptionProvider interface {
	EstimateDaytimeConsumption(ctx context.Context, now time.Time, days int) (domain.DaytimeConsumptionEstimate, error)
}

func daytimeConsumptionHandler(provider DaytimeConsumptionProvider, logger *slog.Logger) http.HandlerFunc {
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
		estimate, err := provider.EstimateDaytimeConsumption(r.Context(), time.Now(), days)
		if err != nil {
			logger.Error("failed to estimate daytime consumption", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to estimate daytime consumption"})
			return
		}
		writeJSON(w, http.StatusOK, estimate)
	}
}
