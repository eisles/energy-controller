package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type ChargingDeviceStore interface {
	ListChargingDevices(ctx context.Context) ([]domain.ChargingDevice, error)
	UpsertChargingDevice(ctx context.Context, device domain.ChargingDevice) (domain.ChargingDevice, error)
	DeleteChargingDevice(ctx context.Context, id int64) error
}

type chargingDevicePayload struct {
	ID                    int64  `json:"id"`
	Name                  string `json:"name"`
	Kind                  string `json:"kind"`
	Provider              string `json:"provider"`
	Role                  string `json:"role"`
	CredentialRef         string `json:"credentialRef"`
	DeviceSN              string `json:"deviceSn"`
	DeviceType            string `json:"deviceType"`
	StatusSource          string `json:"statusSource"`
	Enabled               bool   `json:"enabled"`
	ControlEnabled        bool   `json:"controlEnabled"`
	Priority              int    `json:"priority"`
	MinChargeW            int    `json:"minChargeW"`
	MaxChargeW            int    `json:"maxChargeW"`
	ChargeStepW           int    `json:"chargeStepW"`
	CapacityWh            int    `json:"capacityWh"`
	TargetSoc             int    `json:"targetSoc"`
	ReserveSoc            int    `json:"reserveSoc"`
	BackupReserveMinSoc   int    `json:"backupReserveMinSoc"`
	BackupReserveMaxSoc   int    `json:"backupReserveMaxSoc"`
	ExpectedDaytimeLoadW  int    `json:"expectedDaytimeLoadW"`
	AutoRecoverACOutput   bool   `json:"autoRecoverAcOutput"`
	SupportsSocRead       bool   `json:"supportsSocRead"`
	SupportsACChargeLimit bool   `json:"supportsAcChargeLimit"`
	SupportsOnOff         bool   `json:"supportsOnOff"`
	Notes                 string `json:"notes"`
}

func getChargingDevicesHandler(store ChargingDeviceStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		devices, err := store.ListChargingDevices(r.Context())
		if err != nil {
			logger.Error("failed to list charging devices", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list charging devices"})
			return
		}
		writeJSON(w, http.StatusOK, devices)
	}
}

func postChargingDeviceHandler(store ChargingDeviceStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload chargingDevicePayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid charging device payload"})
			return
		}
		device := chargingDeviceFromPayload(payload)
		if !validChargingDevice(device) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "charging device is out of range"})
			return
		}
		saved, err := store.UpsertChargingDevice(r.Context(), device)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "charging device not found"})
				return
			}
			logger.Error("failed to save charging device", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save charging device"})
			return
		}
		writeJSON(w, http.StatusOK, saved)
	}
}

func deleteChargingDeviceHandler(store ChargingDeviceStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid charging device id"})
			return
		}
		if err := store.DeleteChargingDevice(r.Context(), id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "charging device not found"})
				return
			}
			logger.Error("failed to delete charging device", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete charging device"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func chargingDeviceFromPayload(payload chargingDevicePayload) domain.ChargingDevice {
	device := domain.ChargingDevice{
		ID:                    payload.ID,
		Name:                  strings.TrimSpace(payload.Name),
		Kind:                  strings.TrimSpace(payload.Kind),
		Provider:              strings.TrimSpace(payload.Provider),
		Role:                  strings.TrimSpace(payload.Role),
		CredentialRef:         strings.TrimSpace(payload.CredentialRef),
		DeviceSN:              strings.TrimSpace(payload.DeviceSN),
		DeviceType:            defaultChargingDeviceType(strings.TrimSpace(payload.Kind), strings.TrimSpace(payload.DeviceType)),
		StatusSource:          defaultChargingDeviceStatusSource(strings.TrimSpace(payload.Kind), strings.TrimSpace(payload.StatusSource)),
		Enabled:               payload.Enabled,
		ControlEnabled:        payload.ControlEnabled,
		Priority:              payload.Priority,
		MinChargeW:            payload.MinChargeW,
		MaxChargeW:            payload.MaxChargeW,
		ChargeStepW:           payload.ChargeStepW,
		CapacityWh:            payload.CapacityWh,
		TargetSoc:             payload.TargetSoc,
		ReserveSoc:            payload.ReserveSoc,
		BackupReserveMinSoc:   payload.BackupReserveMinSoc,
		BackupReserveMaxSoc:   payload.BackupReserveMaxSoc,
		ExpectedDaytimeLoadW:  payload.ExpectedDaytimeLoadW,
		AutoRecoverACOutput:   payload.AutoRecoverACOutput,
		SupportsSocRead:       payload.SupportsSocRead,
		SupportsACChargeLimit: payload.SupportsACChargeLimit,
		SupportsOnOff:         payload.SupportsOnOff,
		Notes:                 strings.TrimSpace(payload.Notes),
	}
	return normalizeChargingDevicePayloadReserveBounds(device)
}

func validChargingDevice(device domain.ChargingDevice) bool {
	return device.Name != "" &&
		allowedChargingDeviceKind(device.Kind) &&
		allowedChargingDeviceProvider(device.Provider) &&
		allowedChargingDeviceRole(device.Role) &&
		device.CredentialRef != "" &&
		device.Priority >= 1 &&
		device.MinChargeW >= 0 &&
		device.MaxChargeW >= device.MinChargeW &&
		device.ChargeStepW >= 1 &&
		device.CapacityWh >= 0 &&
		device.TargetSoc >= 0 &&
		device.TargetSoc <= 100 &&
		device.ReserveSoc >= 0 &&
		device.ReserveSoc <= 100 &&
		device.BackupReserveMinSoc >= 5 &&
		device.BackupReserveMinSoc <= 100 &&
		device.BackupReserveMaxSoc >= 5 &&
		device.BackupReserveMaxSoc <= 100 &&
		device.BackupReserveMaxSoc >= device.BackupReserveMinSoc &&
		device.ExpectedDaytimeLoadW >= 0 &&
		validChargingDeviceSN(device.DeviceSN) &&
		validChargingDeviceType(device) &&
		validChargingDeviceStatusSource(device)
}

func normalizeChargingDevicePayloadReserveBounds(device domain.ChargingDevice) domain.ChargingDevice {
	explicitMax := device.BackupReserveMaxSoc != 0
	if device.BackupReserveMinSoc == 0 {
		device.BackupReserveMinSoc = clampChargingDeviceSoc(device.ReserveSoc)
	}
	if device.BackupReserveMaxSoc == 0 {
		device.BackupReserveMaxSoc = clampChargingDeviceSoc(device.TargetSoc)
	}
	if device.BackupReserveMaxSoc < device.BackupReserveMinSoc && !explicitMax {
		device.BackupReserveMaxSoc = device.BackupReserveMinSoc
	}
	device.ReserveSoc = device.BackupReserveMinSoc
	return device
}

func clampChargingDeviceSoc(value int) int {
	if value < 5 {
		return 5
	}
	if value > 100 {
		return 100
	}
	return value
}

func allowedChargingDeviceKind(value string) bool {
	switch value {
	case "ecoflow_delta_pro3", "ecoflow_delta3_plus", "switchbot_plug", "manual":
		return true
	default:
		return false
	}
}

func defaultChargingDeviceType(kind string, value string) string {
	if value != "" {
		return value
	}
	switch kind {
	case "ecoflow_delta_pro3":
		return "DELTA_PRO3"
	case "ecoflow_delta3_plus":
		return "DELTA_3"
	default:
		return ""
	}
}

func defaultChargingDeviceStatusSource(kind string, value string) string {
	if value != "" {
		return value
	}
	switch kind {
	case "ecoflow_delta_pro3":
		return "ecoflow_cloud"
	case "ecoflow_delta3_plus":
		return "ecoflow_private_mqtt"
	case "switchbot_plug":
		return "switchbot_cloud"
	default:
		return "manual"
	}
}

func validChargingDeviceSN(value string) bool {
	for _, r := range value {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	return true
}

func validChargingDeviceType(device domain.ChargingDevice) bool {
	if device.Provider != "ecoflow" {
		return device.DeviceType == ""
	}
	switch device.Kind {
	case "ecoflow_delta_pro3":
		return device.DeviceType == "DELTA_PRO3"
	case "ecoflow_delta3_plus":
		switch device.DeviceType {
		case "DELTA_3", "DELTA_3_PLUS", "DELTA_3_1500", "DELTA_3_MAX_PLUS":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func validChargingDeviceStatusSource(device domain.ChargingDevice) bool {
	switch device.Kind {
	case "ecoflow_delta_pro3":
		return device.StatusSource == "ecoflow_cloud"
	case "ecoflow_delta3_plus":
		return device.StatusSource == "ecoflow_private_mqtt"
	case "switchbot_plug":
		return device.StatusSource == "switchbot_cloud"
	case "manual":
		return device.StatusSource == "manual"
	default:
		return false
	}
}

func allowedChargingDeviceProvider(value string) bool {
	switch value {
	case "ecoflow", "switchbot", "manual":
		return true
	default:
		return false
	}
}

func allowedChargingDeviceRole(value string) bool {
	switch value {
	case "primary", "auxiliary", "manual_auxiliary":
		return true
	default:
		return false
	}
}
