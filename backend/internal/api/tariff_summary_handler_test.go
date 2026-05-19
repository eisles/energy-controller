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
)

type stubTariffSummaryProvider struct {
	from *time.Time
	to   *time.Time
}

func (p *stubTariffSummaryProvider) EnergyCostSummary(_ context.Context, from *time.Time, to *time.Time) (domain.TariffSummary, error) {
	p.from = from
	p.to = to
	return domain.TariffSummary{
		PlanName:             "中部電力 Eライフプラン（3時間帯別電灯）",
		Timezone:             "Asia/Tokyo",
		SampleCount:          2,
		TotalImportKWh:       1.2,
		TotalExportKWh:       0.3,
		TotalImportCostYen:   24.12,
		TotalExportIncomeYen: 2.1,
		NetCostYen:           22.02,
		Periods: []domain.TariffPeriodSummary{
			{Period: "night", ImportKWh: 1.2, ExportKWh: 0.3, ImportCostYen: 24.12, ExportIncomeYen: 2.1, RateYen: 16.11, ExportRateYen: 7},
		},
		Note: "test note",
	}, nil
}

func TestTariffSummaryHandlerReturnsJSON(t *testing.T) {
	provider := &stubTariffSummaryProvider{}
	req := httptest.NewRequest(http.MethodGet, "/api/tariff/summary", nil)
	rec := httptest.NewRecorder()

	tariffSummaryHandler(provider, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var payload domain.TariffSummary
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload.PlanName == "" || payload.SampleCount != 2 || payload.TotalImportCostYen != 24.12 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestTariffSummaryHandlerAcceptsDateRange(t *testing.T) {
	provider := &stubTariffSummaryProvider{}
	from := "2026-05-18T21:00:00Z"
	to := "2026-05-18T22:00:00Z"
	req := httptest.NewRequest(http.MethodGet, "/api/tariff/summary?from="+from+"&to="+to, nil)
	rec := httptest.NewRecorder()

	tariffSummaryHandler(provider, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if provider.from == nil || provider.from.Format(time.RFC3339) != from {
		t.Fatalf("from = %v, want %s", provider.from, from)
	}
	if provider.to == nil || provider.to.Format(time.RFC3339) != to {
		t.Fatalf("to = %v, want %s", provider.to, to)
	}
}

func TestTariffSummaryHandlerRejectsInvalidDateRange(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/tariff/summary?from=2026-05-18T22:00:00Z&to=2026-05-18T21:00:00Z", nil)
	rec := httptest.NewRecorder()

	tariffSummaryHandler(&stubTariffSummaryProvider{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
