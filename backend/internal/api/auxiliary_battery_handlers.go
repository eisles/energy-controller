package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/store"
)

type Delta3AuxControlCommandLogProvider interface {
	ListDelta3AuxControlCommandLogsPage(ctx context.Context, limit int, offset int, filter store.Delta3AuxControlCommandLogPageFilter) ([]domain.Delta3AuxControlCommandLog, int, error)
}

type delta3AuxControlCommandLogsPageResponse struct {
	Items  []domain.Delta3AuxControlCommandLog `json:"items"`
	Total  int                                 `json:"total"`
	Limit  int                                 `json:"limit"`
	Offset int                                 `json:"offset"`
}

func delta3AuxPlanHandler(provider StatusProvider, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := provider.CurrentStatus(r.Context())
		if err != nil {
			logger.Error("failed to read current status for DELTA 3 Plus aux plan", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to read DELTA 3 Plus aux plan"})
			return
		}
		if status.Delta3AuxPlan == nil {
			writeJSON(w, http.StatusOK, domain.Delta3AuxPlan{
				Mode:          "read-only",
				StrategyState: "UNAVAILABLE",
				Reason:        "DELTA 3 Plus auxiliary plan is not available",
			})
			return
		}
		writeJSON(w, http.StatusOK, status.Delta3AuxPlan)
	}
}

func delta3AuxControlCommandLogsHandler(provider Delta3AuxControlCommandLogProvider, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 25
		if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
			parsedLimit, err := strconv.Atoi(rawLimit)
			if err != nil || parsedLimit < 1 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a positive integer"})
				return
			}
			limit = parsedLimit
		}
		offset := 0
		if rawOffset := r.URL.Query().Get("offset"); rawOffset != "" {
			parsedOffset, err := strconv.Atoi(rawOffset)
			if err != nil || parsedOffset < 0 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "offset must be a non-negative integer"})
				return
			}
			offset = parsedOffset
		}
		from, ok := parseOptionalLogTime(w, r.URL.Query().Get("from"), "from")
		if !ok {
			return
		}
		to, ok := parseOptionalLogTime(w, r.URL.Query().Get("to"), "to")
		if !ok {
			return
		}
		if from != nil && to != nil && from.After(*to) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from must be before or equal to to"})
			return
		}
		logs, total, err := provider.ListDelta3AuxControlCommandLogsPage(r.Context(), limit, offset, store.Delta3AuxControlCommandLogPageFilter{
			From: from,
			To:   to,
		})
		if err != nil {
			logger.Error("failed to list DELTA 3 Plus aux command logs", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list DELTA 3 Plus aux command logs"})
			return
		}
		writeJSON(w, http.StatusOK, delta3AuxControlCommandLogsPageResponse{
			Items:  logs,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		})
	}
}
