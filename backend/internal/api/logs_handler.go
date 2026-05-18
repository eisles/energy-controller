package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type LogProvider interface {
	ListPowerLogs(ctx context.Context, limit int) ([]domain.PowerLog, error)
	ListPowerLogsSince(ctx context.Context, since time.Time, limit int) ([]domain.PowerLog, error)
}

func logsHandler(provider LogProvider, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 100
		limitSpecified := false
		if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
			parsedLimit, err := strconv.Atoi(rawLimit)
			if err != nil || parsedLimit < 1 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
				return
			}
			limit = parsedLimit
			limitSpecified = true
		}

		var logs []domain.PowerLog
		var err error
		if rawSince := r.URL.Query().Get("since"); rawSince != "" {
			since, parseErr := time.Parse(time.RFC3339Nano, rawSince)
			if parseErr != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "since must be RFC3339 timestamp"})
				return
			}
			if !limitSpecified {
				limit = 0
			}
			logs, err = provider.ListPowerLogsSince(r.Context(), since, limit)
		} else {
			logs, err = provider.ListPowerLogs(r.Context(), limit)
		}
		if err != nil {
			logger.Error("failed to list power logs", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list power logs"})
			return
		}
		writeJSON(w, http.StatusOK, logs)
	}
}
