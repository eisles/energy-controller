package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/eisles/energy-controller/backend/internal/domain"
	_ "modernc.org/sqlite"
)

func TestChargingDeviceRepositoryPersistsDeviceIdentity(t *testing.T) {
	db := newChargingDeviceTestDB(t)
	repo := NewChargingDeviceRepository(db)

	saved, err := repo.UpsertChargingDevice(context.Background(), domain.ChargingDevice{
		Name:                  "DELTA 3 Plus 2",
		Kind:                  "ecoflow_delta3_plus",
		Provider:              "ecoflow",
		Role:                  "auxiliary",
		CredentialRef:         "ecoflow_delta3_secondary",
		DeviceSN:              "TESTSN123",
		DeviceType:            "DELTA_3",
		Enabled:               true,
		ControlEnabled:        true,
		Priority:              5,
		MinChargeW:            100,
		MaxChargeW:            1500,
		ChargeStepW:           100,
		CapacityWh:            2048,
		TargetSoc:             90,
		ReserveSoc:            20,
		SupportsSocRead:       true,
		SupportsACChargeLimit: true,
		SupportsOnOff:         true,
		Notes:                 "secondary",
	})
	if err != nil {
		t.Fatalf("UpsertChargingDevice failed: %v", err)
	}

	devices, err := repo.ListChargingDevices(context.Background())
	if err != nil {
		t.Fatalf("ListChargingDevices failed: %v", err)
	}
	var found domain.ChargingDevice
	for _, device := range devices {
		if device.ID == saved.ID {
			found = device
			break
		}
	}
	if found.DeviceSN != "TESTSN123" || found.DeviceType != "DELTA_3" {
		t.Fatalf("device identity = %q/%q, want TESTSN123/DELTA_3", found.DeviceSN, found.DeviceType)
	}
}

func TestChargingDeviceRepositorySelectsDelta3Targets(t *testing.T) {
	db := newChargingDeviceTestDB(t)
	repo := NewChargingDeviceRepository(db)
	ctx := context.Background()

	_, err := repo.UpsertChargingDevice(ctx, domain.ChargingDevice{
		Name:                  "read only delta3",
		Kind:                  "ecoflow_delta3_plus",
		Provider:              "ecoflow",
		Role:                  "auxiliary",
		CredentialRef:         "read_only_delta3",
		DeviceSN:              "READONLYSN",
		DeviceType:            "DELTA_3",
		Enabled:               true,
		ControlEnabled:        false,
		Priority:              1,
		MinChargeW:            100,
		MaxChargeW:            1500,
		ChargeStepW:           100,
		CapacityWh:            2048,
		TargetSoc:             90,
		ReserveSoc:            20,
		SupportsSocRead:       true,
		SupportsACChargeLimit: true,
		SupportsOnOff:         true,
	})
	if err != nil {
		t.Fatalf("save read target failed: %v", err)
	}
	_, err = repo.UpsertChargingDevice(ctx, domain.ChargingDevice{
		Name:                  "write delta3",
		Kind:                  "ecoflow_delta3_plus",
		Provider:              "ecoflow",
		Role:                  "auxiliary",
		CredentialRef:         "write_delta3",
		DeviceSN:              "WRITESN",
		DeviceType:            "DELTA_3",
		Enabled:               true,
		ControlEnabled:        true,
		Priority:              2,
		MinChargeW:            100,
		MaxChargeW:            1500,
		ChargeStepW:           100,
		CapacityWh:            2048,
		TargetSoc:             90,
		ReserveSoc:            20,
		SupportsSocRead:       true,
		SupportsACChargeLimit: true,
		SupportsOnOff:         true,
	})
	if err != nil {
		t.Fatalf("save write target failed: %v", err)
	}

	readTarget, ok, err := repo.Delta3ReadTarget(ctx)
	if err != nil || !ok {
		t.Fatalf("Delta3ReadTarget = ok %v err %v", ok, err)
	}
	if readTarget.DeviceSN != "READONLYSN" {
		t.Fatalf("read target SN = %q, want READONLYSN", readTarget.DeviceSN)
	}
	writeTarget, ok, err := repo.Delta3WriteTarget(ctx)
	if err != nil || !ok {
		t.Fatalf("Delta3WriteTarget = ok %v err %v", ok, err)
	}
	if writeTarget.DeviceSN != "WRITESN" {
		t.Fatalf("write target SN = %q, want WRITESN", writeTarget.DeviceSN)
	}
}

func TestChargingDeviceDeviceSNUniqueWhenNonEmpty(t *testing.T) {
	db := newChargingDeviceTestDB(t)
	repo := NewChargingDeviceRepository(db)
	device := domain.ChargingDevice{
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		Provider:              "ecoflow",
		Role:                  "auxiliary",
		CredentialRef:         "delta3_one",
		DeviceSN:              "DUPSN",
		DeviceType:            "DELTA_3",
		Enabled:               true,
		ControlEnabled:        true,
		Priority:              10,
		MinChargeW:            100,
		MaxChargeW:            1500,
		ChargeStepW:           100,
		CapacityWh:            2048,
		TargetSoc:             90,
		ReserveSoc:            20,
		SupportsSocRead:       true,
		SupportsACChargeLimit: true,
	}
	if _, err := repo.UpsertChargingDevice(context.Background(), device); err != nil {
		t.Fatalf("first save failed: %v", err)
	}
	device.CredentialRef = "delta3_two"
	if _, err := repo.UpsertChargingDevice(context.Background(), device); err == nil {
		t.Fatal("second save with duplicate device SN succeeded, want error")
	}
}

func newChargingDeviceTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := migrate(db); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return db
}
