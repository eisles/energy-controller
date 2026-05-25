package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type stubChargingDeviceStore struct {
	saved     domain.ChargingDevice
	deletedID int64
	deleteErr error
}

func (s *stubChargingDeviceStore) ListChargingDevices(context.Context) ([]domain.ChargingDevice, error) {
	return []domain.ChargingDevice{
		{
			ID:            1,
			Name:          "DELTA Pro 3",
			Kind:          "ecoflow_delta_pro3",
			Provider:      "ecoflow",
			Role:          "primary",
			CredentialRef: "ecoflow_pro3_primary",
			DeviceSN:      "TESTSN123",
			DeviceType:    "DELTA_PRO3",
			Enabled:       true,
			Priority:      10,
			MinChargeW:    400,
			MaxChargeW:    1500,
			ChargeStepW:   100,
			TargetSoc:     90,
			ReserveSoc:    30,
			CreatedAt:     time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
			UpdatedAt:     time.Date(2026, 5, 25, 0, 0, 0, 0, time.UTC),
		},
	}, nil
}

func (s *stubChargingDeviceStore) UpsertChargingDevice(_ context.Context, device domain.ChargingDevice) (domain.ChargingDevice, error) {
	s.saved = device
	if device.ID == 404 {
		return domain.ChargingDevice{}, sql.ErrNoRows
	}
	if device.ID == 0 {
		device.ID = 2
	}
	return device, nil
}

func (s *stubChargingDeviceStore) DeleteChargingDevice(_ context.Context, id int64) error {
	s.deletedID = id
	return s.deleteErr
}

func TestGetChargingDevicesHandlerReturnsJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/settings/charging-devices", nil)
	rec := httptest.NewRecorder()

	getChargingDevicesHandler(&stubChargingDeviceStore{}, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	var payload []domain.ChargingDevice
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if len(payload) != 1 || payload[0].CredentialRef != "ecoflow_pro3_primary" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestPostChargingDeviceHandlerSavesDevice(t *testing.T) {
	store := &stubChargingDeviceStore{}
	body := []byte(`{"name":"DELTA 3 Plus 2","kind":"ecoflow_delta3_plus","provider":"ecoflow","role":"auxiliary","credentialRef":"ecoflow_delta3_secondary","deviceSn":" TESTSN456 ","deviceType":"","enabled":true,"controlEnabled":false,"priority":30,"minChargeW":100,"maxChargeW":1500,"chargeStepW":100,"capacityWh":2048,"targetSoc":90,"reserveSoc":20,"supportsSocRead":true,"supportsAcChargeLimit":true,"supportsOnOff":true,"notes":"secondary"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/charging-devices", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	postChargingDeviceHandler(store, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.saved.Name != "DELTA 3 Plus 2" || store.saved.CredentialRef != "ecoflow_delta3_secondary" || store.saved.ControlEnabled {
		t.Fatalf("saved device = %#v", store.saved)
	}
	if store.saved.DeviceSN != "TESTSN456" || store.saved.DeviceType != "DELTA_3" {
		t.Fatalf("saved identity = %q/%q, want TESTSN456/DELTA_3", store.saved.DeviceSN, store.saved.DeviceType)
	}
}

func TestPostChargingDeviceHandlerRejectsSecretLookingCredentialRef(t *testing.T) {
	body := []byte(`{"name":"bad","kind":"ecoflow_delta3_plus","provider":"ecoflow","role":"auxiliary","credentialRef":"","enabled":true,"priority":1,"minChargeW":100,"maxChargeW":1500,"chargeStepW":100,"targetSoc":90,"reserveSoc":20}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/charging-devices", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	postChargingDeviceHandler(&stubChargingDeviceStore{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPostChargingDeviceHandlerRejectsDeviceSNWithWhitespace(t *testing.T) {
	body := []byte(`{"name":"bad","kind":"ecoflow_delta3_plus","provider":"ecoflow","role":"auxiliary","credentialRef":"ecoflow_delta3_bad","deviceSn":"BAD SN","enabled":true,"priority":1,"minChargeW":100,"maxChargeW":1500,"chargeStepW":100,"targetSoc":90,"reserveSoc":20}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/charging-devices", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	postChargingDeviceHandler(&stubChargingDeviceStore{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPostChargingDeviceHandlerRejectsMismatchedDeviceType(t *testing.T) {
	body := []byte(`{"name":"bad","kind":"ecoflow_delta3_plus","provider":"ecoflow","role":"auxiliary","credentialRef":"ecoflow_delta3_bad","deviceSn":"BADSN","deviceType":"DELTA_PRO3","enabled":true,"priority":1,"minChargeW":100,"maxChargeW":1500,"chargeStepW":100,"targetSoc":90,"reserveSoc":20}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/charging-devices", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	postChargingDeviceHandler(&stubChargingDeviceStore{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDeleteChargingDeviceHandlerDeletesDevice(t *testing.T) {
	store := &stubChargingDeviceStore{}
	req := httptest.NewRequest(http.MethodDelete, "/api/settings/charging-devices/2", nil)
	req.SetPathValue("id", "2")
	rec := httptest.NewRecorder()

	deleteChargingDeviceHandler(store, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.deletedID != 2 {
		t.Fatalf("deletedID = %d, want 2", store.deletedID)
	}
}
