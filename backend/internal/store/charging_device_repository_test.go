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
		StatusSource:          "ecoflow_private_mqtt",
		Enabled:               true,
		ControlEnabled:        true,
		Priority:              5,
		MinChargeW:            100,
		MaxChargeW:            1500,
		ChargeStepW:           100,
		CapacityWh:            2048,
		TargetSoc:             90,
		ReserveSoc:            20,
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   85,
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
	if found.DeviceSN != "TESTSN123" || found.DeviceType != "DELTA_3" || found.StatusSource != "ecoflow_private_mqtt" {
		t.Fatalf("device identity = %q/%q/%q, want TESTSN123/DELTA_3/ecoflow_private_mqtt", found.DeviceSN, found.DeviceType, found.StatusSource)
	}
	if found.BackupReserveMinSoc != 20 || found.BackupReserveMaxSoc != 85 {
		t.Fatalf("backup reserve range = %d-%d, want 20-85", found.BackupReserveMinSoc, found.BackupReserveMaxSoc)
	}
}

func TestChargingDeviceRepositoryNormalizesOmittedBackupReserveMaxBelowMin(t *testing.T) {
	db := newChargingDeviceTestDB(t)
	repo := NewChargingDeviceRepository(db)

	saved, err := repo.UpsertChargingDevice(context.Background(), domain.ChargingDevice{
		Name:          "manual battery",
		Kind:          "manual_battery",
		Provider:      "manual",
		Role:          "auxiliary",
		CredentialRef: "manual_aux",
		Enabled:       true,
		Priority:      20,
		MinChargeW:    100,
		MaxChargeW:    1000,
		ChargeStepW:   100,
		CapacityWh:    1000,
		TargetSoc:     0,
		ReserveSoc:    20,
	})
	if err != nil {
		t.Fatalf("UpsertChargingDevice failed: %v", err)
	}
	if saved.BackupReserveMinSoc != 20 || saved.BackupReserveMaxSoc != 20 {
		t.Fatalf("backup reserve range = %d-%d, want 20-20", saved.BackupReserveMinSoc, saved.BackupReserveMaxSoc)
	}
	if saved.ReserveSoc != 20 {
		t.Fatalf("ReserveSoc = %d, want synced backup reserve min 20", saved.ReserveSoc)
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
		StatusSource:          "ecoflow_private_mqtt",
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
		StatusSource:          "ecoflow_private_mqtt",
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

	readTargets, err := repo.Delta3ReadTargets(ctx)
	if err != nil {
		t.Fatalf("Delta3ReadTargets failed: %v", err)
	}
	if len(readTargets) != 2 {
		t.Fatalf("Delta3ReadTargets len = %d, want 2", len(readTargets))
	}
	if readTargets[0].DeviceSN != "READONLYSN" || readTargets[1].DeviceSN != "WRITESN" {
		t.Fatalf("Delta3ReadTargets order = %q/%q, want READONLYSN/WRITESN", readTargets[0].DeviceSN, readTargets[1].DeviceSN)
	}
}

func TestChargingDeviceRepositorySelectsEcoFlowCloudTargets(t *testing.T) {
	db := newChargingDeviceTestDB(t)
	repo := NewChargingDeviceRepository(db)
	ctx := context.Background()

	_, err := repo.UpsertChargingDevice(ctx, domain.ChargingDevice{
		Name:                  "blank sn write pro3",
		Kind:                  "ecoflow_delta_pro3",
		Provider:              "ecoflow",
		Role:                  "primary",
		CredentialRef:         "blank_write_pro3",
		DeviceSN:              "   ",
		DeviceType:            "DELTA_PRO3",
		StatusSource:          "ecoflow_cloud",
		Enabled:               true,
		ControlEnabled:        true,
		Priority:              1,
		MinChargeW:            400,
		MaxChargeW:            1500,
		ChargeStepW:           100,
		CapacityWh:            12288,
		TargetSoc:             90,
		ReserveSoc:            30,
		SupportsSocRead:       true,
		SupportsACChargeLimit: true,
	})
	if err != nil {
		t.Fatalf("save blank write target failed: %v", err)
	}
	_, err = repo.UpsertChargingDevice(ctx, domain.ChargingDevice{
		Name:                  "read only pro3",
		Kind:                  "ecoflow_delta_pro3",
		Provider:              "ecoflow",
		Role:                  "primary",
		CredentialRef:         "read_only_pro3",
		DeviceSN:              "PRO3READ",
		DeviceType:            "DELTA_PRO3",
		StatusSource:          "ecoflow_cloud",
		Enabled:               true,
		ControlEnabled:        false,
		Priority:              2,
		MinChargeW:            400,
		MaxChargeW:            1500,
		ChargeStepW:           100,
		CapacityWh:            12288,
		TargetSoc:             90,
		ReserveSoc:            30,
		SupportsSocRead:       true,
		SupportsACChargeLimit: true,
	})
	if err != nil {
		t.Fatalf("save read target failed: %v", err)
	}
	_, err = repo.UpsertChargingDevice(ctx, domain.ChargingDevice{
		Name:                  "write pro3",
		Kind:                  "ecoflow_delta_pro3",
		Provider:              "ecoflow",
		Role:                  "primary",
		CredentialRef:         "write_pro3",
		DeviceSN:              "PRO3WRITE",
		DeviceType:            "DELTA_PRO3",
		StatusSource:          "ecoflow_cloud",
		Enabled:               true,
		ControlEnabled:        true,
		Priority:              3,
		MinChargeW:            400,
		MaxChargeW:            1500,
		ChargeStepW:           100,
		CapacityWh:            12288,
		TargetSoc:             90,
		ReserveSoc:            30,
		SupportsSocRead:       true,
		SupportsACChargeLimit: true,
	})
	if err != nil {
		t.Fatalf("save write target failed: %v", err)
	}

	writeTarget, ok, err := repo.EcoFlowCloudWriteTarget(ctx)
	if err != nil || !ok {
		t.Fatalf("EcoFlowCloudWriteTarget = ok %v err %v", ok, err)
	}
	if writeTarget.DeviceSN != "PRO3WRITE" {
		t.Fatalf("write target SN = %q, want PRO3WRITE", writeTarget.DeviceSN)
	}
	readTarget, ok, err := repo.EcoFlowCloudReadTarget(ctx)
	if err != nil || !ok {
		t.Fatalf("EcoFlowCloudReadTarget = ok %v err %v", ok, err)
	}
	if readTarget.DeviceSN != "PRO3WRITE" {
		t.Fatalf("read target SN = %q, want aligned write target PRO3WRITE", readTarget.DeviceSN)
	}
}

func TestChargingDeviceRepositorySelectsEcoFlowCloudReadTargetWithoutWriteTarget(t *testing.T) {
	db := newChargingDeviceTestDB(t)
	repo := NewChargingDeviceRepository(db)
	ctx := context.Background()

	_, err := repo.UpsertChargingDevice(ctx, domain.ChargingDevice{
		Name:                  "read only pro3",
		Kind:                  "ecoflow_delta_pro3",
		Provider:              "ecoflow",
		Role:                  "primary",
		CredentialRef:         "read_only_pro3",
		DeviceSN:              "PRO3READ",
		DeviceType:            "DELTA_PRO3",
		StatusSource:          "ecoflow_cloud",
		Enabled:               true,
		ControlEnabled:        false,
		Priority:              1,
		MinChargeW:            400,
		MaxChargeW:            1500,
		ChargeStepW:           100,
		CapacityWh:            12288,
		TargetSoc:             90,
		ReserveSoc:            30,
		SupportsSocRead:       true,
		SupportsACChargeLimit: true,
	})
	if err != nil {
		t.Fatalf("save read target failed: %v", err)
	}

	readTarget, ok, err := repo.EcoFlowCloudReadTarget(ctx)
	if err != nil || !ok {
		t.Fatalf("EcoFlowCloudReadTarget = ok %v err %v", ok, err)
	}
	if readTarget.DeviceSN != "PRO3READ" {
		t.Fatalf("read target SN = %q, want PRO3READ", readTarget.DeviceSN)
	}
	writeTarget, ok, err := repo.EcoFlowCloudWriteTarget(ctx)
	if err != nil || ok {
		t.Fatalf("EcoFlowCloudWriteTarget = target %+v ok %v err %v, want no write target", writeTarget, ok, err)
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
		StatusSource:          "ecoflow_private_mqtt",
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
