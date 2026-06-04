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

type statusProviderFunc func(context.Context) (domain.Status, error)

func (f statusProviderFunc) CurrentStatus(ctx context.Context) (domain.Status, error) {
	return f(ctx)
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

func TestStatusHandlerAddsExportingControlDiagnostics(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	now := time.Date(2026, 6, 5, 9, 0, 0, 0, time.UTC)
	reserve := 70

	provider := statusProviderFunc(func(context.Context) (domain.Status, error) {
		return domain.Status{
			GridW:              -900,
			ImportW:            0,
			ExportW:            900,
			BatterySoc:         65,
			BatteryInputW:      120,
			BatteryOutputW:     0,
			TargetChargeW:      500,
			LastDecisionReason: "surplus tracking condition met; planner recommends charging adjustments",
			UpdatedAt:          now.Add(-30 * time.Second),
			SurplusPlan: &domain.SurplusPlan{
				StrategyState:               "READY",
				RecommendedACChargeLimitW:   500,
				RecommendedBackupReserveSoc: &reserve,
				ShouldAdjustACChargeLimit:   true,
				WouldWrite:                  true,
				ActionSummary:               "surplus tracking condition met; planner recommends charging adjustments",
			},
		}, nil
	})

	statusHandler(provider, slog.Default(), readyControlConfig(now))(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var payload struct {
		ControlDiagnostics domain.ControlDiagnostics `json:"controlDiagnostics"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload.ControlDiagnostics.GridState != "exporting" {
		t.Fatalf("GridState = %q, want exporting", payload.ControlDiagnostics.GridState)
	}
	if payload.ControlDiagnostics.Pro3.Action != "surplus_charge_candidate" {
		t.Fatalf("Pro3.Action = %q, want surplus_charge_candidate", payload.ControlDiagnostics.Pro3.Action)
	}
	if !payload.ControlDiagnostics.Pro3.WriteCandidate {
		t.Fatal("Pro3.WriteCandidate = false, want true")
	}
	if payload.ControlDiagnostics.Summary != "export_absorb_candidate" {
		t.Fatalf("Summary = %q, want export_absorb_candidate", payload.ControlDiagnostics.Summary)
	}
}

func TestStatusHandlerPrioritizesPassthroughControlDiagnostics(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	now := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	reserve := 65

	provider := statusProviderFunc(func(context.Context) (domain.Status, error) {
		return domain.Status{
			GridW:              -80,
			ImportW:            0,
			ExportW:            80,
			BatterySoc:         65,
			BatteryInputW:      420,
			BatteryOutputW:     410,
			LastDecisionReason: "small surplus; keep passthrough aligned",
			UpdatedAt:          now.Add(-20 * time.Second),
			SurplusPlan: &domain.SurplusPlan{
				StrategyState:               "PASSTHROUGH",
				RecommendedACChargeLimitW:   400,
				RecommendedBackupReserveSoc: &reserve,
				ShouldAlignBackupReserve:    true,
				WouldWrite:                  true,
				ActionSummary:               "small surplus; keep passthrough aligned",
			},
		}, nil
	})

	statusHandler(provider, slog.Default(), readyControlConfig(now))(rec, req)

	var payload struct {
		ControlDiagnostics domain.ControlDiagnostics `json:"controlDiagnostics"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload.ControlDiagnostics.Pro3.Action != "passthrough" {
		t.Fatalf("Pro3.Action = %q, want passthrough", payload.ControlDiagnostics.Pro3.Action)
	}
}

func TestStatusHandlerDoesNotLetInactiveNightPlanHideSurplusDiagnostics(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	now := time.Date(2026, 6, 5, 14, 0, 0, 0, time.UTC)
	reserve := 72

	provider := statusProviderFunc(func(context.Context) (domain.Status, error) {
		return domain.Status{
			GridW:              -760,
			ImportW:            0,
			ExportW:            760,
			BatterySoc:         60,
			BatteryInputW:      100,
			BatteryOutputW:     0,
			LastDecisionReason: "daytime surplus should be handled by surplus plan",
			UpdatedAt:          now.Add(-20 * time.Second),
			NightChargePlan: &domain.NightChargePlan{
				ShouldChargeTonight: true,
				WouldWrite:          false,
				ActionSummary:       "night charge target exists outside write window",
			},
			SurplusPlan: &domain.SurplusPlan{
				StrategyState:               "READY",
				RecommendedACChargeLimitW:   500,
				RecommendedBackupReserveSoc: &reserve,
				ShouldAdjustACChargeLimit:   true,
				WouldWrite:                  true,
				ActionSummary:               "surplus tracking condition met; planner recommends charging adjustments",
			},
		}, nil
	})

	statusHandler(provider, slog.Default(), readyControlConfig(now))(rec, req)

	var payload struct {
		ControlDiagnostics domain.ControlDiagnostics `json:"controlDiagnostics"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload.ControlDiagnostics.Pro3.Action != "surplus_charge_candidate" {
		t.Fatalf("Pro3.Action = %q, want surplus_charge_candidate", payload.ControlDiagnostics.Pro3.Action)
	}
	if payload.ControlDiagnostics.Pro3.ControlSource != "surplus_plan" {
		t.Fatalf("Pro3.ControlSource = %q, want surplus_plan", payload.ControlDiagnostics.Pro3.ControlSource)
	}
}

func TestStatusHandlerMapsActiveSurplusTrackingToChargingDiagnostics(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)

	provider := statusProviderFunc(func(context.Context) (domain.Status, error) {
		return domain.Status{
			GridW:              -180,
			ImportW:            0,
			ExportW:            180,
			BatterySoc:         72,
			BatteryInputW:      420,
			BatteryOutputW:     40,
			LastDecisionReason: "already tracking surplus charging",
			UpdatedAt:          now.Add(-20 * time.Second),
			SurplusPlan: &domain.SurplusPlan{
				StrategyState:             "CHARGING",
				RecommendedACChargeLimitW: 400,
				WouldWrite:                false,
				ActionSummary:             "already tracking surplus charging",
			},
		}, nil
	})

	statusHandler(provider, slog.Default(), readyControlConfig(now))(rec, req)

	var payload struct {
		ControlDiagnostics domain.ControlDiagnostics `json:"controlDiagnostics"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload.ControlDiagnostics.Pro3.Action != "charging" {
		t.Fatalf("Pro3.Action = %q, want charging", payload.ControlDiagnostics.Pro3.Action)
	}
}

func TestStatusHandlerAddsImportingControlDiagnostics(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	now := time.Date(2026, 6, 5, 19, 0, 0, 0, time.UTC)

	provider := statusProviderFunc(func(context.Context) (domain.Status, error) {
		return domain.Status{
			GridW:              650,
			ImportW:            650,
			ExportW:            0,
			BatterySoc:         70,
			BatteryInputW:      0,
			BatteryOutputW:     480,
			TargetChargeW:      0,
			LastDecisionReason: "importing from grid; recover by stopping surplus charge and restoring default reserve",
			UpdatedAt:          now.Add(-15 * time.Second),
			SurplusPlan: &domain.SurplusPlan{
				StrategyState:             "RECOVERING",
				ShouldLowerBackupReserve:  true,
				ShouldDisableEnergyModes:  true,
				RecommendedACChargeLimitW: 0,
				Reason:                    "importing from grid; recover by stopping surplus charge and restoring default reserve",
			},
		}, nil
	})

	statusHandler(provider, slog.Default(), readyControlConfig(now))(rec, req)

	var payload struct {
		ControlDiagnostics domain.ControlDiagnostics `json:"controlDiagnostics"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload.ControlDiagnostics.GridState != "importing" {
		t.Fatalf("GridState = %q, want importing", payload.ControlDiagnostics.GridState)
	}
	if payload.ControlDiagnostics.Pro3.Action != "discharge_recovery_candidate" {
		t.Fatalf("Pro3.Action = %q, want discharge_recovery_candidate", payload.ControlDiagnostics.Pro3.Action)
	}
	if payload.ControlDiagnostics.Summary != "import_discharge_candidate" {
		t.Fatalf("Summary = %q, want import_discharge_candidate", payload.ControlDiagnostics.Summary)
	}
}

func TestStatusHandlerAddsStaleErrorControlDiagnostics(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	now := time.Date(2026, 6, 5, 21, 0, 0, 0, time.UTC)
	lastErr := "Nature Remo grid power read failed: nature cloud returned HTTP 429"

	provider := statusProviderFunc(func(context.Context) (domain.Status, error) {
		return domain.Status{
			GridW:              0,
			ImportW:            0,
			ExportW:            0,
			LastDecisionReason: "status acquisition failed",
			LastError:          &lastErr,
			UpdatedAt:          now.Add(-10 * time.Minute),
		}, nil
	})

	statusHandler(provider, slog.Default(), readyControlConfig(now))(rec, req)

	var payload struct {
		ControlDiagnostics domain.ControlDiagnostics `json:"controlDiagnostics"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if !payload.ControlDiagnostics.DataFreshness.Stale {
		t.Fatal("DataFreshness.Stale = false, want true")
	}
	if !payload.ControlDiagnostics.DataFreshness.HasError {
		t.Fatal("DataFreshness.HasError = false, want true")
	}
	if payload.ControlDiagnostics.Pro3.Action != "unavailable" {
		t.Fatalf("Pro3.Action = %q, want unavailable", payload.ControlDiagnostics.Pro3.Action)
	}
	if payload.ControlDiagnostics.Summary != "status_error" {
		t.Fatalf("Summary = %q, want status_error", payload.ControlDiagnostics.Summary)
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
	var payload struct {
		ControlDiagnostics domain.ControlDiagnostics `json:"controlDiagnostics"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload.ControlDiagnostics.WriteReadiness.BlockedReason != "CONFIRM_ECOFLOW_WRITE is not I_UNDERSTAND" {
		t.Fatalf("BlockedReason = %q, want sanitized confirmation blocker", payload.ControlDiagnostics.WriteReadiness.BlockedReason)
	}
}

func readyControlConfig(now time.Time) config.Config {
	return config.Config{
		MockMode:              false,
		SimulationMode:        false,
		EnableRealControl:     true,
		AutoControlEnabled:    true,
		ConfirmEcoFlowWrite:   "I_UNDERSTAND",
		RealControlTrialUntil: now.Add(time.Hour),
		Clock:                 fixedAPIClock{now: now},
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
