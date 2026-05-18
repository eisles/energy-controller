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

type stubStatusProvider struct{}

func (stubStatusProvider) CurrentStatus(context.Context) (domain.Status, error) {
	return domain.Status{
		GridW:              -850,
		ImportW:            0,
		ExportW:            850,
		BatterySoc:         62,
		BatteryInputW:      500,
		BatteryOutputW:     0,
		TargetChargeW:      700,
		State:              "simulation",
		Mode:               "mock",
		LastDecisionReason: "export power is enough, simulation only",
		UpdatedAt:          time.Date(2026, 5, 18, 7, 30, 0, 0, time.FixedZone("JST", 9*60*60)),
	}, nil
}

func TestStatusHandlerReturnsJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()

	statusHandler(stubStatusProvider{}, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload["mode"] != "mock" || payload["state"] != "simulation" {
		t.Fatalf("unexpected mode/state: %#v", payload)
	}
	if payload["gridW"] != float64(-850) {
		t.Fatalf("gridW = %#v, want -850", payload["gridW"])
	}
}
