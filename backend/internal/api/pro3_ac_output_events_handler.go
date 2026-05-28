package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type Pro3ACOutputEventProvider interface {
	ListPro3ACOutputEventsPage(ctx context.Context, limit int, offset int) ([]domain.Pro3ACOutputEvent, int, error)
}

type pro3ACOutputEventsPageResponse struct {
	Items  []domain.Pro3ACOutputEvent `json:"items"`
	Total  int                        `json:"total"`
	Limit  int                        `json:"limit"`
	Offset int                        `json:"offset"`
}

func pro3ACOutputEventsHandler(provider Pro3ACOutputEventProvider, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 20
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
		events, total, err := provider.ListPro3ACOutputEventsPage(r.Context(), limit, offset)
		if err != nil {
			logger.Error("failed to list Pro3 AC output events", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list Pro3 AC output events"})
			return
		}
		writeJSON(w, http.StatusOK, pro3ACOutputEventsPageResponse{
			Items:  events,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		})
	}
}
