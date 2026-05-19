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

type stubEnergyMeterLogProvider struct {
	limit  int
	offset int
	filter store.EnergyMeterLogPageFilter
}

func (p *stubEnergyMeterLogProvider) ListEnergyMeterLogsPage(_ context.Context, limit int, offset int, filter store.EnergyMeterLogPageFilter) ([]domain.EnergyMeterLog, int, error) {
	p.limit = limit
	p.offset = offset
	p.filter = filter
	return []domain.EnergyMeterLog{
		{
			ID:                  1,
			MeasuredAt:          time.Date(2026, 5, 18, 21, 38, 10, 0, time.UTC),
			ImportCumulativeKWh: 47002.2,
			ExportCumulativeKWh: 13611.3,
			Coefficient:         1,
			CumulativeUnit:      0.1,
			CreatedAt:           time.Date(2026, 5, 18, 21, 38, 10, 0, time.UTC),
		},
	}, 42, nil
}

func TestEnergyMeterLogsHandlerReturnsPagedJSON(t *testing.T) {
	provider := &stubEnergyMeterLogProvider{}
	req := httptest.NewRequest(http.MethodGet, "/api/energy-meter/logs?limit=25&offset=50", nil)
	rec := httptest.NewRecorder()

	energyMeterLogsHandler(provider, slog.Default())(rec, req)

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

func TestEnergyMeterLogsHandlerAcceptsDateRange(t *testing.T) {
	provider := &stubEnergyMeterLogProvider{}
	from := "2026-05-18T21:00:00Z"
	to := "2026-05-18T22:00:00Z"
	req := httptest.NewRequest(http.MethodGet, "/api/energy-meter/logs?from="+from+"&to="+to, nil)
	rec := httptest.NewRecorder()

	energyMeterLogsHandler(provider, slog.Default())(rec, req)

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

func TestEnergyMeterLogsHandlerRejectsInvalidDateRange(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/energy-meter/logs?from=2026-05-18T22:00:00Z&to=2026-05-18T21:00:00Z", nil)
	rec := httptest.NewRecorder()

	energyMeterLogsHandler(&stubEnergyMeterLogProvider{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
