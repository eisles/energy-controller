package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/store"
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

type fixedAPIClock struct {
	now time.Time
}

func (c fixedAPIClock) Now() time.Time {
	return c.now
}

func TestStatusHandlerReturnsJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()

	statusHandler(stubStatusProvider{}, slog.Default(), config.Config{})(rec, req)

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

func TestRouterDelta3StatusWorksWithoutDB(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/delta3/status", nil)
	rec := httptest.NewRecorder()

	NewRouter(Dependencies{StatusProvider: stubStatusProvider{}, Logger: slog.Default(), Config: config.Config{}}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload["available"] != false {
		t.Fatalf("available = %#v, want false", payload["available"])
	}
}

func TestStatusHandlerAddsRealControlTrialStatus(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	now := time.Date(2026, 5, 23, 7, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	until := now.Add(2 * time.Hour)

	statusHandler(stubStatusProvider{}, slog.Default(), config.Config{
		RealControlTrialUntil: until,
		Clock:                 fixedAPIClock{now: now},
	})(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload["realControlTrialActive"] != true {
		t.Fatalf("realControlTrialActive = %#v, want true", payload["realControlTrialActive"])
	}
	if payload["realControlTrialUntil"] != until.Format(time.RFC3339) {
		t.Fatalf("realControlTrialUntil = %#v, want %s", payload["realControlTrialUntil"], until.Format(time.RFC3339))
	}
	if payload["realControlTrialRemainingSeconds"] != float64(7200) {
		t.Fatalf("realControlTrialRemainingSeconds = %#v, want 7200", payload["realControlTrialRemainingSeconds"])
	}
}

func TestStatusHandlerAddsDefaultControlWriteReadiness(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()

	statusHandler(stubStatusProvider{}, slog.Default(), config.Config{
		MockMode:                true,
		SimulationMode:          true,
		EnableRealControl:       false,
		AutoControlEnabled:      false,
		ConfirmEcoFlowWrite:     "",
		Delta3ReadEnabled:       true,
		Delta3ExecuteWrite:      true,
		Delta3AllowPrivateWrite: true,
		Delta3AllowAutoWrite:    true,
		Delta3Aux:               config.Delta3AuxConfig{Enabled: true},
	})(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var payload struct {
		ControlWriteReadiness domain.ControlWriteReadiness `json:"controlWriteReadiness"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload.ControlWriteReadiness.Ready {
		t.Fatal("ControlWriteReadiness.Ready = true, want false")
	}
	if payload.ControlWriteReadiness.Mode != "dry-run" {
		t.Fatalf("ControlWriteReadiness.Mode = %q, want dry-run", payload.ControlWriteReadiness.Mode)
	}
	if len(payload.ControlWriteReadiness.Reasons) == 0 {
		t.Fatal("ControlWriteReadiness.Reasons is empty, want blockers")
	}
	gates := payload.ControlWriteReadiness.Gates
	if !gates.MockMode || !gates.SimulationMode || gates.EnableRealControl || gates.AutoControlEnabled {
		t.Fatalf("unexpected Pro 3 write gates: %+v", gates)
	}
	if !gates.Delta3ReadEnabled || !gates.Delta3ExecuteWrite || !gates.Delta3AllowPrivateWrite || !gates.Delta3AllowAutoWrite || !gates.Delta3AuxEnabled {
		t.Fatalf("unexpected DELTA 3 gates: %+v", gates)
	}
}

func TestStatusHandlerAddsReadyControlWriteReadiness(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	now := time.Date(2026, 6, 4, 8, 0, 0, 0, time.UTC)

	statusHandler(stubStatusProvider{}, slog.Default(), config.Config{
		MockMode:                false,
		SimulationMode:          false,
		EnableRealControl:       true,
		AutoControlEnabled:      true,
		ConfirmEcoFlowWrite:     "I_UNDERSTAND",
		RealControlTrialUntil:   now.Add(time.Hour),
		Clock:                   fixedAPIClock{now: now},
		Delta3ReadEnabled:       true,
		Delta3ExecuteWrite:      true,
		Delta3AllowPrivateWrite: true,
		Delta3AllowAutoWrite:    true,
		Delta3Aux:               config.Delta3AuxConfig{Enabled: true},
	})(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var payload struct {
		ControlWriteReadiness domain.ControlWriteReadiness `json:"controlWriteReadiness"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if !payload.ControlWriteReadiness.Ready {
		t.Fatalf("ControlWriteReadiness.Ready = false, reasons = %#v", payload.ControlWriteReadiness.Reasons)
	}
	if payload.ControlWriteReadiness.Mode != "ready" {
		t.Fatalf("ControlWriteReadiness.Mode = %q, want ready", payload.ControlWriteReadiness.Mode)
	}
	if len(payload.ControlWriteReadiness.Reasons) != 0 {
		t.Fatalf("ControlWriteReadiness.Reasons = %#v, want empty", payload.ControlWriteReadiness.Reasons)
	}
	if !payload.ControlWriteReadiness.Gates.ConfirmEcoFlowWriteAccepted || !payload.ControlWriteReadiness.Gates.RealControlTrialActive {
		t.Fatalf("unexpected gates: %+v", payload.ControlWriteReadiness.Gates)
	}
}

func TestStatusHandlerControlWriteReadinessDoesNotExposeSecrets(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	now := time.Date(2026, 6, 4, 8, 0, 0, 0, time.UTC)

	statusHandler(stubStatusProvider{}, slog.Default(), config.Config{
		MockMode:              false,
		SimulationMode:        false,
		EnableRealControl:     true,
		AutoControlEnabled:    true,
		ConfirmEcoFlowWrite:   "SECRET_CONFIRM",
		RealControlTrialUntil: now.Add(time.Hour),
		Clock:                 fixedAPIClock{now: now},
		EcoFlowAccessKey:      "access-secret",
		EcoFlowSecretKey:      "secret-key",
		EcoFlowDeviceSN:       "device-serial",
		Delta3PrivateEmail:    "user@example.com",
		Delta3PrivatePassword: "private-password",
		Delta3DeviceSN:        "delta3-serial",
		Delta3MQTTClientID:    "mqtt-client",
	})(rec, req)

	body := rec.Body.String()
	for _, secret := range []string{"SECRET_CONFIRM", "access-secret", "secret-key", "device-serial", "user@example.com", "private-password", "delta3-serial", "mqtt-client"} {
		if strings.Contains(body, secret) {
			t.Fatalf("response leaked secret %q: %s", secret, body)
		}
	}
	if !strings.Contains(body, "CONFIRM_ECOFLOW_WRITE is not I_UNDERSTAND") {
		t.Fatalf("response body does not include sanitized confirmation blocker: %s", body)
	}
}

func TestRouterStatusReadsCurrentStatusFromDatabase(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "energy.db"))
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 5, 18, 9, 0, 0, 0, time.UTC)
	if err := store.NewStatusRepository(db).UpdateCurrentStatus(context.Background(), domain.Status{
		GridW:              -640,
		ImportW:            0,
		ExportW:            640,
		BatterySoc:         63,
		BatteryInputW:      400,
		BatteryOutputW:     0,
		ACChargeLimitW:     1500,
		TargetChargeW:      400,
		State:              "simulation",
		Mode:               "mock",
		LastDecisionReason: "database status",
		UpdatedAt:          now,
	}); err != nil {
		t.Fatalf("UpdateCurrentStatus failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	NewRouter(Dependencies{DB: db, StatusProvider: stubStatusProvider{}, Logger: slog.Default()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload["gridW"] != float64(-640) || payload["acChargeLimitW"] != float64(1500) || payload["lastDecisionReason"] != "database status" {
		t.Fatalf("unexpected database status payload: %#v", payload)
	}
}

func TestRouterUpdatesWeatherLocation(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "energy.db"))
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer db.Close()

	body := strings.NewReader(`{"enabled":true,"latitude":35.362502,"longitude":136.9253633,"timezone":"Asia/Tokyo","pvCapacityKw":5.5,"pvPerformanceRatio":0.78,"dailyBaseLoadKwh":8.2,"batteryCapacityKwh":4.096,"minimumReserveSoc":35}`)
	req := httptest.NewRequest(http.MethodPut, "/api/settings/weather-location", body)
	rec := httptest.NewRecorder()
	NewRouter(Dependencies{DB: db, StatusProvider: stubStatusProvider{}, Logger: slog.Default()}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/settings/weather-location", nil)
	getRec := httptest.NewRecorder()
	NewRouter(Dependencies{DB: db, StatusProvider: stubStatusProvider{}, Logger: slog.Default()}).ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status code = %d, want %d", getRec.Code, http.StatusOK)
	}
	var payload map[string]any
	if err := json.NewDecoder(getRec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload["enabled"] != true || payload["latitude"] != 35.362502 || payload["longitude"] != 136.9253633 {
		t.Fatalf("unexpected weather location payload: %#v", payload)
	}
	if payload["pvCapacityKw"] != 5.5 || payload["pvPerformanceRatio"] != 0.78 || payload["minimumReserveSoc"] != float64(35) {
		t.Fatalf("unexpected solar settings payload: %#v", payload)
	}
}
