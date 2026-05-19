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

type fakeEcoFlowLoadProvider struct {
	days int
}

func (p *fakeEcoFlowLoadProvider) EstimateEcoFlowLoad(_ context.Context, _ time.Time, days int) (domain.EcoFlowLoadEstimate, error) {
	p.days = days
	return domain.EcoFlowLoadEstimate{
		Days:                        days,
		AverageDaytimeOutputKWh:     3.2,
		SuggestedDaytimeBaseLoadKWh: 3.2,
		SampleCount:                 42,
	}, nil
}

func TestEcoFlowLoadHandlerReturnsEstimate(t *testing.T) {
	provider := &fakeEcoFlowLoadProvider{}
	handler := ecoFlowLoadHandler(provider, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/analytics/ecoflow-load?days=14", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if provider.days != 14 {
		t.Fatalf("days = %d, want 14", provider.days)
	}
	var estimate domain.EcoFlowLoadEstimate
	if err := json.NewDecoder(rec.Body).Decode(&estimate); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if estimate.SuggestedDaytimeBaseLoadKWh != 3.2 {
		t.Fatalf("SuggestedDaytimeBaseLoadKWh = %f, want 3.2", estimate.SuggestedDaytimeBaseLoadKWh)
	}
}

func TestEcoFlowLoadHandlerRejectsInvalidDays(t *testing.T) {
	handler := ecoFlowLoadHandler(&fakeEcoFlowLoadProvider{}, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/analytics/ecoflow-load?days=31", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
