package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/store"
)

type stubNightChargeSummaryProvider struct {
	now    time.Time
	limit  int
	offset int
	filter store.NightChargeSummaryPageFilter
}

func (p *stubNightChargeSummaryProvider) ListNightChargeDailySummariesPage(_ context.Context, now time.Time, limit int, offset int, filter store.NightChargeSummaryPageFilter) ([]domain.NightChargeDailySummary, int, error) {
	p.now = now
	p.limit = limit
	p.offset = offset
	p.filter = filter
	plannedTargetSoc := 60
	nightEndSoc := 62
	return []domain.NightChargeDailySummary{
		{
			SummaryDate:       "2026-05-19",
			PlannedTargetSoc:  &plannedTargetSoc,
			NightEndSoc:       &nightEndSoc,
			MorningStatus:     "ok",
			FinalResultStatus: "ok",
			DataSource:        "power-log",
		},
	}, 42, nil
}

func TestNightChargeSummariesHandlerReturnsPagedJSON(t *testing.T) {
	provider := &stubNightChargeSummaryProvider{}
	req := httptest.NewRequest(http.MethodGet, "/api/night-charge/summaries?limit=25&offset=50", nil)
	rec := httptest.NewRecorder()

	nightChargeSummariesHandler(provider, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if provider.limit != 25 || provider.offset != 50 {
		t.Fatalf("limit, offset = %d, %d; want 25, 50", provider.limit, provider.offset)
	}
	if provider.now.IsZero() {
		t.Fatal("now was not passed to provider")
	}
	var payload struct {
		Items  []map[string]any `json:"items"`
		Total  int              `json:"total"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload.Total != 42 || payload.Limit != 25 || payload.Offset != 50 || len(payload.Items) != 1 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestNightChargeSummariesHandlerAcceptsDateRange(t *testing.T) {
	provider := &stubNightChargeSummaryProvider{}
	from := "2026-05-19T21:00:00Z"
	to := "2026-05-20T16:00:00Z"
	req := httptest.NewRequest(http.MethodGet, "/api/night-charge/summaries?from="+from+"&to="+to, nil)
	rec := httptest.NewRecorder()

	nightChargeSummariesHandler(provider, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if provider.filter.From == nil || provider.filter.From.Format(time.RFC3339) != from {
		t.Fatalf("from = %v, want %s", provider.filter.From, from)
	}
	if provider.filter.To == nil || provider.filter.To.Format(time.RFC3339) != to {
		t.Fatalf("to = %v, want %s", provider.filter.To, to)
	}
}

func TestNightChargeSummariesHandlerRejectsInvalidDateRange(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/night-charge/summaries?from=2026-05-20T16:00:00Z&to=2026-05-19T21:00:00Z", nil)
	rec := httptest.NewRecorder()

	nightChargeSummariesHandler(&stubNightChargeSummaryProvider{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
