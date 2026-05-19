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

type NightChargeSummaryProvider interface {
	ListNightChargeDailySummariesPage(ctx context.Context, now time.Time, limit int, offset int, filter store.NightChargeSummaryPageFilter) ([]domain.NightChargeDailySummary, int, error)
}

type nightChargeSummariesPageResponse struct {
	Items  []domain.NightChargeDailySummary `json:"items"`
	Total  int                              `json:"total"`
	Limit  int                              `json:"limit"`
	Offset int                              `json:"offset"`
}

func nightChargeSummariesHandler(provider NightChargeSummaryProvider, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 30
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
		summaries, total, err := provider.ListNightChargeDailySummariesPage(r.Context(), time.Now(), limit, offset, store.NightChargeSummaryPageFilter{
			From: from,
			To:   to,
		})
		if err != nil {
			logger.Error("failed to list night charge summaries", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list night charge summaries"})
			return
		}
		writeJSON(w, http.StatusOK, nightChargeSummariesPageResponse{
			Items:  summaries,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		})
	}
}
