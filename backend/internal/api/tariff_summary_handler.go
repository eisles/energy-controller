package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type TariffSummaryProvider interface {
	EnergyCostSummary(ctx context.Context, from *time.Time, to *time.Time) (domain.TariffSummary, error)
}

func tariffSummaryHandler(provider TariffSummaryProvider, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		summary, err := provider.EnergyCostSummary(r.Context(), from, to)
		if err != nil {
			logger.Error("failed to summarize energy cost", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to summarize energy cost"})
			return
		}
		writeJSON(w, http.StatusOK, summary)
	}
}
