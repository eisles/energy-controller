package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/store"
)

type LogProvider interface {
	ListPowerLogs(ctx context.Context, limit int) ([]domain.PowerLog, error)
	ListPowerLogsPage(ctx context.Context, limit int, offset int, filter store.LogPageFilter) ([]domain.PowerLog, int, error)
	ListPowerLogsSince(ctx context.Context, since time.Time, limit int) ([]domain.PowerLog, error)
}

type logsPageResponse struct {
	Items  []domain.PowerLog `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
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
		offset := 0
		offsetSpecified := false
		if rawOffset := r.URL.Query().Get("offset"); rawOffset != "" {
			parsedOffset, err := strconv.Atoi(rawOffset)
			if err != nil || parsedOffset < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "offset must be a non-negative integer"})
				return
			}
			offset = parsedOffset
			offsetSpecified = true
		}
		query := r.URL.Query().Get("q")
		var from *time.Time
		if rawFrom := r.URL.Query().Get("from"); rawFrom != "" {
			parsedFrom, parseErr := time.Parse(time.RFC3339Nano, rawFrom)
			if parseErr != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from must be RFC3339 timestamp"})
				return
			}
			from = &parsedFrom
		}
		var to *time.Time
		if rawTo := r.URL.Query().Get("to"); rawTo != "" {
			parsedTo, parseErr := time.Parse(time.RFC3339Nano, rawTo)
			if parseErr != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to must be RFC3339 timestamp"})
				return
			}
			to = &parsedTo
		}
		if from != nil && to != nil && from.After(*to) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from must be before or equal to to"})
			return
		}

		var logs []domain.PowerLog
		var err error
		if rawSince := r.URL.Query().Get("since"); rawSince != "" {
			if query != "" || from != nil || to != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q, from, and to cannot be used with since"})
				return
			}
			if offsetSpecified {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "offset cannot be used with since"})
				return
			}
			since, parseErr := time.Parse(time.RFC3339Nano, rawSince)
			if parseErr != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "since must be RFC3339 timestamp"})
				return
			}
			if !limitSpecified {
				limit = 0
			}
			logs, err = provider.ListPowerLogsSince(r.Context(), since, limit)
		} else if offsetSpecified || query != "" || from != nil || to != nil {
			var total int
			logs, total, err = provider.ListPowerLogsPage(r.Context(), limit, offset, store.LogPageFilter{
				Query: query,
				From:  from,
				To:    to,
			})
			if err != nil {
				logger.Error("failed to list power logs", "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list power logs"})
				return
			}
			writeJSON(w, http.StatusOK, logsPageResponse{
				Items:  logs,
				Total:  total,
				Limit:  limit,
				Offset: offset,
			})
			return
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
