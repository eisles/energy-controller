package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type PVForecastHistoryProvider interface {
	ListPVForecastHistory(ctx context.Context, days int) ([]domain.PVForecastHistoryItem, error)
}

func nightChargeForecastHistoryHandler(provider PVForecastHistoryProvider, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		days := 30
		if rawDays := r.URL.Query().Get("days"); rawDays != "" {
			parsedDays, err := strconv.Atoi(rawDays)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "days must be an integer"})
				return
			}
			days = parsedDays
		}
		if days < 1 {
			days = 1
		}
		if days > 90 {
			days = 90
		}
		items, err := provider.ListPVForecastHistory(r.Context(), days)
		if err != nil {
			logger.Error("failed to list PV forecast history", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list PV forecast history"})
			return
		}
		writeJSON(w, http.StatusOK, domain.PVForecastHistoryResponse{Items: items})
	}
}
