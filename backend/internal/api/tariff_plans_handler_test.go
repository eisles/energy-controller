package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/store"
)

type stubTariffPlanStore struct {
	saved     domain.TariffPlan
	deletedID int64
	deleteErr error
}

func (s *stubTariffPlanStore) ListTariffPlans(_ context.Context) ([]domain.TariffPlan, error) {
	return []domain.TariffPlan{
		{
			ID:            1,
			PlanName:      "current",
			DayRateYen:    34.06,
			HomeRateYen:   26,
			NightRateYen:  16.11,
			ExportRateYen: 7,
			Timezone:      "Asia/Tokyo",
			EffectiveFrom: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		},
	}, nil
}

func (s *stubTariffPlanStore) UpsertTariffPlan(_ context.Context, plan domain.TariffPlan) (domain.TariffPlan, error) {
	s.saved = plan
	plan.ID = 2
	return plan, nil
}

func (s *stubTariffPlanStore) DeleteTariffPlan(_ context.Context, id int64) error {
	s.deletedID = id
	return s.deleteErr
}

func TestGetTariffPlansHandlerReturnsJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/settings/tariff-plans", nil)
	rec := httptest.NewRecorder()

	getTariffPlansHandler(&stubTariffPlanStore{}, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var payload []domain.TariffPlan
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if len(payload) != 1 || payload[0].PlanName != "current" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestPostTariffPlanHandlerSavesPlan(t *testing.T) {
	store := &stubTariffPlanStore{}
	body := []byte(`{"planName":"next","dayRateYen":35,"homeRateYen":27,"nightRateYen":17,"exportRateYen":7,"timezone":"Asia/Tokyo","effectiveFrom":"2026-06-01T00:00:00+09:00"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/tariff-plans", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	postTariffPlanHandler(store, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.saved.PlanName != "next" || store.saved.DayRateYen != 35 || store.saved.ExportRateYen != 7 {
		t.Fatalf("saved plan = %#v", store.saved)
	}
	if store.saved.EffectiveFrom.Format(time.RFC3339) != "2026-06-01T00:00:00+09:00" {
		t.Fatalf("effectiveFrom = %s", store.saved.EffectiveFrom.Format(time.RFC3339))
	}
	if store.saved.PeriodRules != nil {
		t.Fatalf("PeriodRules = %#v, want nil when omitted so existing rules are preserved", store.saved.PeriodRules)
	}
}

func TestPostTariffPlanHandlerAllowsExplicitEmptyPeriodRules(t *testing.T) {
	store := &stubTariffPlanStore{}
	body := []byte(`{"planName":"next","dayRateYen":35,"homeRateYen":27,"nightRateYen":17,"exportRateYen":7,"timezone":"Asia/Tokyo","effectiveFrom":"2026-06-01T00:00:00+09:00","periodRules":[]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/tariff-plans", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	postTariffPlanHandler(store, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.saved.PeriodRules == nil || len(store.saved.PeriodRules) != 0 {
		t.Fatalf("PeriodRules = %#v, want explicit empty slice for clearing custom rules", store.saved.PeriodRules)
	}
}

func TestPostTariffPlanHandlerAllowsBaselinePlan(t *testing.T) {
	store := &stubTariffPlanStore{}
	body := []byte(`{"planName":"baseline","dayRateYen":34.06,"homeRateYen":26,"nightRateYen":16.11,"exportRateYen":7,"timezone":"Asia/Tokyo","effectiveFrom":"1970-01-01T00:00:00Z"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/tariff-plans", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	postTariffPlanHandler(store, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.saved.EffectiveFrom.Format(time.RFC3339) != "1970-01-01T00:00:00Z" {
		t.Fatalf("effectiveFrom = %s", store.saved.EffectiveFrom.Format(time.RFC3339))
	}
}

func TestPostTariffPlanHandlerRejectsInvalidPlan(t *testing.T) {
	body := []byte(`{"planName":"","dayRateYen":35,"homeRateYen":27,"nightRateYen":17,"exportRateYen":7,"timezone":"Asia/Tokyo","effectiveFrom":"2026-06-01T00:00:00+09:00"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/tariff-plans", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	postTariffPlanHandler(&stubTariffPlanStore{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPostTariffPlanHandlerRejectsInvalidTimezone(t *testing.T) {
	body := []byte(`{"planName":"next","dayRateYen":35,"homeRateYen":27,"nightRateYen":17,"exportRateYen":7,"timezone":"Asia/Tokyoo","effectiveFrom":"2026-06-01T00:00:00+09:00"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/tariff-plans", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	postTariffPlanHandler(&stubTariffPlanStore{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPostTariffPlanHandlerRejectsMissingRate(t *testing.T) {
	body := []byte(`{"planName":"next","dayRateYen":35,"homeRateYen":27,"nightRateYen":17,"timezone":"Asia/Tokyo","effectiveFrom":"2026-06-01T00:00:00+09:00"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/tariff-plans", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	postTariffPlanHandler(&stubTariffPlanStore{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPostTariffPlanHandlerRejectsZeroRate(t *testing.T) {
	body := []byte(`{"planName":"next","dayRateYen":0,"homeRateYen":27,"nightRateYen":17,"exportRateYen":7,"timezone":"Asia/Tokyo","effectiveFrom":"2026-06-01T00:00:00+09:00"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/tariff-plans", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	postTariffPlanHandler(&stubTariffPlanStore{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteTariffPlanHandlerDeletesPlan(t *testing.T) {
	store := &stubTariffPlanStore{}
	req := httptest.NewRequest(http.MethodDelete, "/api/settings/tariff-plans/2", nil)
	req.SetPathValue("id", "2")
	rec := httptest.NewRecorder()

	deleteTariffPlanHandler(store, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if store.deletedID != 2 {
		t.Fatalf("deletedID = %d, want 2", store.deletedID)
	}
}

func TestDeleteTariffPlanHandlerRejectsLastPlan(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/settings/tariff-plans/1", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	deleteTariffPlanHandler(&stubTariffPlanStore{deleteErr: store.ErrCannotDeleteLastTariffPlan}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteTariffPlanHandlerRejectsCurrentCoverageGap(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/settings/tariff-plans/1", nil)
	req.SetPathValue("id", "1")
	rec := httptest.NewRecorder()

	deleteTariffPlanHandler(&stubTariffPlanStore{deleteErr: store.ErrTariffPlanCoverageRequired}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteTariffPlanHandlerReturnsNotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/settings/tariff-plans/999", nil)
	req.SetPathValue("id", "999")
	rec := httptest.NewRecorder()

	deleteTariffPlanHandler(&stubTariffPlanStore{deleteErr: sql.ErrNoRows}, slog.Default())(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteTariffPlanHandlerReturnsServerError(t *testing.T) {
	req := httptest.NewRequest(http.MethodDelete, "/api/settings/tariff-plans/999", nil)
	req.SetPathValue("id", "999")
	rec := httptest.NewRecorder()

	deleteTariffPlanHandler(&stubTariffPlanStore{deleteErr: errors.New("db failed")}, slog.Default())(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}
