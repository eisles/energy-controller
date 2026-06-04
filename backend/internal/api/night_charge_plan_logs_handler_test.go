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

type stubNightChargePlanLogProvider struct {
	limit  int
	offset int
	filter store.NightChargePlanLogPageFilter
	days   int
}

func (p *stubNightChargePlanLogProvider) ListNightChargePlanLogsPage(_ context.Context, limit int, offset int, filter store.NightChargePlanLogPageFilter) ([]domain.NightChargePlanLog, int, error) {
	p.limit = limit
	p.offset = offset
	p.filter = filter
	return []domain.NightChargePlanLog{
		{
			ID:                        1,
			MeasuredAt:                time.Date(2026, 5, 19, 22, 0, 0, 0, time.UTC),
			StrategyState:             "NIGHT_PLAN_READY",
			RecommendedMode:           "tou",
			RecommendedNightTargetSoc: 60,
			BatterySoc:                80,
			CreatedAt:                 time.Date(2026, 5, 19, 22, 0, 0, 0, time.UTC),
		},
	}, 42, nil
}

func (p *stubNightChargePlanLogProvider) ListPVForecastHistory(_ context.Context, days int) ([]domain.PVForecastHistoryItem, error) {
	p.days = days
	return []domain.PVForecastHistoryItem{
		{
			ForecastDate:              "2026-06-02",
			FirstMeasuredAt:           time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
			LastMeasuredAt:            time.Date(2026, 6, 2, 6, 0, 0, 0, time.UTC),
			SampleCount:               12,
			EstimatedPVKWh:            2.7,
			CorrectedEstimatedPVKWh:   1.9,
			ForecastDaytimeDeficitKWh: 1.6,
			RecommendedNightTargetSoc: 59,
			RequiredNightChargeKWh:    0,
			ShouldChargeTonight:       false,
		},
	}, nil
}

func TestNightChargePlanLogsHandlerReturnsPagedJSON(t *testing.T) {
	provider := &stubNightChargePlanLogProvider{}
	req := httptest.NewRequest(http.MethodGet, "/api/night-charge/plans?limit=25&offset=50", nil)
	rec := httptest.NewRecorder()

	nightChargePlanLogsHandler(provider, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if provider.limit != 25 || provider.offset != 50 {
		t.Fatalf("limit, offset = %d, %d; want 25, 50", provider.limit, provider.offset)
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

func TestNightChargePlanLogsHandlerAcceptsDateRange(t *testing.T) {
	provider := &stubNightChargePlanLogProvider{}
	from := "2026-05-19T21:00:00Z"
	to := "2026-05-20T07:00:00Z"
	req := httptest.NewRequest(http.MethodGet, "/api/night-charge/plans?from="+from+"&to="+to, nil)
	rec := httptest.NewRecorder()

	nightChargePlanLogsHandler(provider, slog.Default())(rec, req)

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

func TestNightChargePlanLogsHandlerRejectsInvalidDateRange(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/night-charge/plans?from=2026-05-20T07:00:00Z&to=2026-05-19T21:00:00Z", nil)
	rec := httptest.NewRecorder()

	nightChargePlanLogsHandler(&stubNightChargePlanLogProvider{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestNightChargeForecastHistoryHandlerReturnsItems(t *testing.T) {
	provider := &stubNightChargePlanLogProvider{}
	req := httptest.NewRequest(http.MethodGet, "/api/night-charge/forecast-history?days=14", nil)
	rec := httptest.NewRecorder()

	nightChargeForecastHistoryHandler(provider, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if provider.days != 14 {
		t.Fatalf("days = %d, want 14", provider.days)
	}
	var payload domain.PVForecastHistoryResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].ForecastDate != "2026-06-02" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestNightChargeForecastHistoryHandlerClampsDays(t *testing.T) {
	provider := &stubNightChargePlanLogProvider{}
	req := httptest.NewRequest(http.MethodGet, "/api/night-charge/forecast-history?days=999", nil)
	rec := httptest.NewRecorder()

	nightChargeForecastHistoryHandler(provider, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if provider.days != 90 {
		t.Fatalf("days = %d, want 90", provider.days)
	}
}

func TestNightChargeForecastHistoryHandlerRejectsInvalidDays(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/night-charge/forecast-history?days=abc", nil)
	rec := httptest.NewRecorder()

	nightChargeForecastHistoryHandler(&stubNightChargePlanLogProvider{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
