package store

import (
	"context"
	"database/sql"
	"math"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"

	_ "modernc.org/sqlite"
)

func TestStatusRepositoryUpdatesAndReadsCurrentStatus(t *testing.T) {
	db := openTestDB(t)
	repo := NewStatusRepository(db)
	now := time.Date(2026, 5, 18, 8, 15, 0, 123, time.UTC)
	lastError := "sample error"
	fullEnergyWh := 12288

	want := domain.Status{
		GridW:               -850,
		ImportW:             0,
		ExportW:             850,
		BatterySoc:          62,
		BatteryInputW:       500,
		BatteryOutputW:      0,
		ACChargeLimitW:      1500,
		BatteryFullEnergyWh: &fullEnergyWh,
		TargetChargeW:       700,
		State:               "simulation",
		Mode:                "mock",
		LastDecisionReason:  "export power is above start threshold",
		LastError:           &lastError,
		Delta3AuxPlan: &domain.Delta3AuxPlan{
			Mode:                      "read-only",
			StrategyState:             "READY",
			RecommendedACChargeLimitW: 300,
			ResidualExportW:           850,
			SafetyMarginW:             50,
			Reason:                    "test delta3 aux plan",
		},
		UpdatedAt: now,
	}

	if err := repo.UpdateCurrentStatus(context.Background(), want); err != nil {
		t.Fatalf("UpdateCurrentStatus failed: %v", err)
	}
	got, err := repo.CurrentStatus(context.Background())
	if err != nil {
		t.Fatalf("CurrentStatus failed: %v", err)
	}

	if got.GridW != want.GridW || got.ExportW != want.ExportW || got.TargetChargeW != want.TargetChargeW || got.ACChargeLimitW != want.ACChargeLimitW {
		t.Fatalf("status mismatch: got %+v want %+v", got, want)
	}
	if got.LastError == nil || *got.LastError != lastError {
		t.Fatalf("LastError = %#v, want %q", got.LastError, lastError)
	}
	if got.BatteryFullEnergyWh == nil || *got.BatteryFullEnergyWh != fullEnergyWh {
		t.Fatalf("BatteryFullEnergyWh = %v, want %d", got.BatteryFullEnergyWh, fullEnergyWh)
	}
	if got.Delta3AuxPlan == nil || got.Delta3AuxPlan.StrategyState != "READY" || got.Delta3AuxPlan.RecommendedACChargeLimitW != 300 {
		t.Fatalf("Delta3AuxPlan = %+v, want READY 300W", got.Delta3AuxPlan)
	}
	if !got.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %s, want %s", got.UpdatedAt, now)
	}
}

func TestDelta3AuxControlCommandRepositoryInsertsAndPages(t *testing.T) {
	db := openTestDB(t)
	repo := NewDelta3AuxControlCommandRepository(db)
	base := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	soc := 70
	previous := 100
	target := 300

	if err := repo.InsertDelta3AuxControlCommandLog(context.Background(), domain.Delta3AuxControlCommandLog{
		MeasuredAt:                base,
		StrategyState:             "READY",
		CommandFingerprint:        "delta3_aux;state=READY;ac=300;adjust_ac=true",
		GridW:                     -900,
		ImportW:                   0,
		ExportW:                   900,
		ResidualExportW:           900,
		Delta3Soc:                 &soc,
		PreviousACChargeLimitW:    &previous,
		TargetACChargeLimitW:      &target,
		DryRun:                    true,
		WouldWrite:                true,
		ShouldAdjustACChargeLimit: true,
		DecisionReason:            "test",
		CreatedAt:                 base,
	}); err != nil {
		t.Fatalf("InsertDelta3AuxControlCommandLog failed: %v", err)
	}

	logs, total, err := repo.ListDelta3AuxControlCommandLogsPage(context.Background(), 25, 0, Delta3AuxControlCommandLogPageFilter{})
	if err != nil {
		t.Fatalf("ListDelta3AuxControlCommandLogsPage failed: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("total=%d len=%d, want 1/1", total, len(logs))
	}
	if logs[0].Delta3Soc == nil || *logs[0].Delta3Soc != soc || logs[0].TargetACChargeLimitW == nil || *logs[0].TargetACChargeLimitW != target {
		t.Fatalf("unexpected log: %+v", logs[0])
	}
}

func TestChargingDeviceRepositorySeedsAndUpsertsDevices(t *testing.T) {
	db := openTestDB(t)
	repo := NewChargingDeviceRepository(db)

	devices, err := repo.ListChargingDevices(context.Background())
	if err != nil {
		t.Fatalf("ListChargingDevices failed: %v", err)
	}
	if len(devices) != 3 {
		t.Fatalf("seeded devices = %d, want 3", len(devices))
	}
	if devices[0].Kind != "ecoflow_delta_pro3" || devices[0].CredentialRef != "ecoflow_pro3_primary" || devices[0].StatusSource != "ecoflow_cloud" || devices[0].ControlEnabled {
		t.Fatalf("unexpected first seed device: %+v", devices[0])
	}

	saved, err := repo.UpsertChargingDevice(context.Background(), domain.ChargingDevice{
		Name:                  "DELTA 3 Plus 2",
		Kind:                  "ecoflow_delta3_plus",
		Provider:              "ecoflow",
		Role:                  "auxiliary",
		CredentialRef:         "ecoflow_delta3_secondary",
		DeviceType:            "DELTA_3",
		StatusSource:          "ecoflow_private_mqtt",
		Enabled:               true,
		ControlEnabled:        false,
		Priority:              30,
		MinChargeW:            100,
		MaxChargeW:            1500,
		ChargeStepW:           100,
		CapacityWh:            2048,
		TargetSoc:             90,
		ReserveSoc:            20,
		SupportsSocRead:       true,
		SupportsACChargeLimit: true,
		SupportsOnOff:         true,
		Notes:                 "secondary unit",
	})
	if err != nil {
		t.Fatalf("UpsertChargingDevice insert failed: %v", err)
	}
	if saved.ID == 0 || saved.CredentialRef != "ecoflow_delta3_secondary" {
		t.Fatalf("saved device = %+v", saved)
	}

	saved.Name = "DELTA 3 Plus 書斎"
	saved.Priority = 25
	saved.ControlEnabled = true
	updated, err := repo.UpsertChargingDevice(context.Background(), saved)
	if err != nil {
		t.Fatalf("UpsertChargingDevice update failed: %v", err)
	}
	if updated.Name != "DELTA 3 Plus 書斎" || updated.Priority != 25 || !updated.ControlEnabled {
		t.Fatalf("updated device = %+v", updated)
	}

	if err := repo.DeleteChargingDevice(context.Background(), updated.ID); err != nil {
		t.Fatalf("DeleteChargingDevice failed: %v", err)
	}
	devices, err = repo.ListChargingDevices(context.Background())
	if err != nil {
		t.Fatalf("ListChargingDevices after delete failed: %v", err)
	}
	if len(devices) != 3 {
		t.Fatalf("devices after delete = %d, want 3", len(devices))
	}
}

func TestLogRepositoryInsertsAndListsNewestFirst(t *testing.T) {
	db := openTestDB(t)
	repo := NewLogRepository(db)
	firstAt := time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Minute)
	soc := 61
	acLimit := 1500

	if err := repo.InsertPowerLog(context.Background(), domain.PowerLog{
		MeasuredAt:     firstAt,
		GridW:          200,
		ImportW:        200,
		ExportW:        0,
		TargetChargeW:  0,
		DecisionReason: "importing from grid, do not charge",
		Mode:           "mock",
		CommandSent:    false,
		CreatedAt:      firstAt,
	}); err != nil {
		t.Fatalf("InsertPowerLog first failed: %v", err)
	}
	if err := repo.InsertPowerLog(context.Background(), domain.PowerLog{
		MeasuredAt:     secondAt,
		GridW:          -900,
		ImportW:        0,
		ExportW:        900,
		BatterySoc:     &soc,
		ACChargeLimitW: &acLimit,
		TargetChargeW:  700,
		DecisionReason: "export power is above start threshold",
		Mode:           "mock",
		CommandSent:    false,
		CreatedAt:      secondAt,
	}); err != nil {
		t.Fatalf("InsertPowerLog second failed: %v", err)
	}

	logs, err := repo.ListPowerLogs(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListPowerLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs) = %d, want 1", len(logs))
	}
	if logs[0].GridW != -900 || logs[0].BatterySoc == nil || *logs[0].BatterySoc != soc || logs[0].ACChargeLimitW == nil || *logs[0].ACChargeLimitW != acLimit {
		t.Fatalf("unexpected newest log: %+v", logs[0])
	}
}

func TestLogRepositoryListsPowerLogsPage(t *testing.T) {
	db := openTestDB(t)
	repo := NewLogRepository(db)
	base := time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		if err := repo.InsertPowerLog(context.Background(), domain.PowerLog{
			MeasuredAt:     base.Add(time.Duration(i) * time.Minute),
			GridW:          100 + i,
			ImportW:        100 + i,
			ExportW:        0,
			TargetChargeW:  0,
			DecisionReason: "sample",
			Mode:           "mock",
			CommandSent:    false,
			CreatedAt:      base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("InsertPowerLog failed: %v", err)
		}
	}

	logs, total, err := repo.ListPowerLogsPage(context.Background(), 1, 1, LogPageFilter{})
	if err != nil {
		t.Fatalf("ListPowerLogsPage failed: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs) = %d, want 1", len(logs))
	}
	if logs[0].GridW != 101 {
		t.Fatalf("GridW = %d, want 101", logs[0].GridW)
	}
}

func TestLogRepositorySearchesPowerLogsPage(t *testing.T) {
	db := openTestDB(t)
	repo := NewLogRepository(db)
	base := time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC)

	for _, log := range []domain.PowerLog{
		{
			MeasuredAt:     base,
			GridW:          100,
			ImportW:        100,
			ExportW:        0,
			TargetChargeW:  0,
			DecisionReason: "importing from grid",
			Mode:           "mock",
			CommandSent:    false,
			CreatedAt:      base,
		},
		{
			MeasuredAt:     base.Add(time.Minute),
			GridW:          -900,
			ImportW:        0,
			ExportW:        900,
			TargetChargeW:  700,
			DecisionReason: "export power is above start threshold",
			Mode:           "nature-cloud+ecoflow-read",
			CommandSent:    false,
			CreatedAt:      base.Add(time.Minute),
		},
	} {
		if err := repo.InsertPowerLog(context.Background(), log); err != nil {
			t.Fatalf("InsertPowerLog failed: %v", err)
		}
	}

	logs, total, err := repo.ListPowerLogsPage(context.Background(), 25, 0, LogPageFilter{Query: "export"})
	if err != nil {
		t.Fatalf("ListPowerLogsPage failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	if len(logs) != 1 || logs[0].ExportW != 900 {
		t.Fatalf("logs = %+v, want export log", logs)
	}
}

func TestLogRepositoryFiltersPowerLogsPageByDateRange(t *testing.T) {
	db := openTestDB(t)
	repo := NewLogRepository(db)
	base := time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		at := base.Add(time.Duration(i) * time.Hour)
		if err := repo.InsertPowerLog(context.Background(), domain.PowerLog{
			MeasuredAt:     at,
			GridW:          100 + i,
			ImportW:        100 + i,
			ExportW:        0,
			TargetChargeW:  0,
			DecisionReason: "sample",
			Mode:           "mock",
			CommandSent:    false,
			CreatedAt:      at,
		}); err != nil {
			t.Fatalf("InsertPowerLog failed: %v", err)
		}
	}

	from := base.Add(30 * time.Minute)
	to := base.Add(90 * time.Minute)
	logs, total, err := repo.ListPowerLogsPage(context.Background(), 25, 0, LogPageFilter{From: &from, To: &to})
	if err != nil {
		t.Fatalf("ListPowerLogsPage failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	if len(logs) != 1 || logs[0].GridW != 101 {
		t.Fatalf("logs = %+v, want only middle log", logs)
	}
}

func TestLogRepositoryListsSinceTimestamp(t *testing.T) {
	db := openTestDB(t)
	repo := NewLogRepository(db)
	firstAt := time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(2 * time.Hour)

	for _, log := range []domain.PowerLog{
		{
			MeasuredAt:     firstAt,
			GridW:          200,
			ImportW:        200,
			ExportW:        0,
			TargetChargeW:  0,
			DecisionReason: "importing from grid, do not charge",
			Mode:           "mock",
			CommandSent:    false,
			CreatedAt:      firstAt,
		},
		{
			MeasuredAt:     secondAt,
			GridW:          -900,
			ImportW:        0,
			ExportW:        900,
			TargetChargeW:  700,
			DecisionReason: "export power is above start threshold",
			Mode:           "mock",
			CommandSent:    false,
			CreatedAt:      secondAt,
		},
	} {
		if err := repo.InsertPowerLog(context.Background(), log); err != nil {
			t.Fatalf("InsertPowerLog failed: %v", err)
		}
	}

	logs, err := repo.ListPowerLogsSince(context.Background(), firstAt.Add(time.Hour), 0)
	if err != nil {
		t.Fatalf("ListPowerLogsSince failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs) = %d, want 1", len(logs))
	}
	if logs[0].MeasuredAt != secondAt {
		t.Fatalf("MeasuredAt = %s, want %s", logs[0].MeasuredAt, secondAt)
	}
}

func TestEnergyMeterRepositoryInsertsDeltaFromPreviousReading(t *testing.T) {
	db := openTestDB(t)
	repo := NewEnergyMeterRepository(db)
	base := time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC)

	readings := []domain.EnergyMeterReading{
		{
			MeasuredAt:           base,
			ImportCumulativeKWh:  1000,
			ExportCumulativeKWh:  500,
			Coefficient:          1,
			CumulativeUnit:       0.1,
			RawImportCumulative:  "00002710",
			RawExportCumulative:  "00001388",
			ImportValueUpdatedAt: base,
			ExportValueUpdatedAt: base,
		},
		{
			MeasuredAt:           base.Add(time.Hour),
			ImportCumulativeKWh:  1001.2,
			ExportCumulativeKWh:  500.4,
			Coefficient:          1,
			CumulativeUnit:       0.1,
			RawImportCumulative:  "0000271c",
			RawExportCumulative:  "0000138c",
			ImportValueUpdatedAt: base.Add(time.Hour),
			ExportValueUpdatedAt: base.Add(time.Hour),
		},
	}
	for _, reading := range readings {
		if err := repo.InsertEnergyMeterReading(context.Background(), reading); err != nil {
			t.Fatalf("InsertEnergyMeterReading failed: %v", err)
		}
	}

	logs, err := repo.ListEnergyMeterLogs(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListEnergyMeterLogs failed: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("len(logs) = %d, want 2", len(logs))
	}
	if logs[0].ImportDeltaKWh == nil || !floatAlmostEqual(*logs[0].ImportDeltaKWh, 1.2) {
		t.Fatalf("ImportDeltaKWh = %v, want 1.2", logs[0].ImportDeltaKWh)
	}
	if logs[0].ExportDeltaKWh == nil || !floatAlmostEqual(*logs[0].ExportDeltaKWh, 0.4) {
		t.Fatalf("ExportDeltaKWh = %v, want 0.4", logs[0].ExportDeltaKWh)
	}
	if logs[1].ImportDeltaKWh != nil || logs[1].ExportDeltaKWh != nil {
		t.Fatalf("first log delta = %v, %v; want nil deltas", logs[1].ImportDeltaKWh, logs[1].ExportDeltaKWh)
	}
}

func TestNightChargePlanRepositoryInsertsAndListsNewestFirst(t *testing.T) {
	db := openTestDB(t)
	repo := NewNightChargePlanRepository(db)
	base := time.Date(2026, 5, 19, 22, 0, 0, 0, time.UTC)

	for i := 0; i < 2; i++ {
		measuredAt := base.Add(time.Duration(i) * time.Hour)
		targetDate := "2026-05-20"
		if err := repo.InsertNightChargePlanLog(context.Background(), domain.Status{
			GridW:          100 + i,
			ImportW:        100 + i,
			BatterySoc:     80 + i,
			BatteryInputW:  10,
			BatteryOutputW: 200,
			UpdatedAt:      measuredAt,
			NightChargePlan: &domain.NightChargePlan{
				StrategyState:             "NIGHT_PLAN_READY",
				RecommendedMode:           "tou",
				RecommendedNightTargetSoc: 60 + i,
				RecommendedNightTargetKWh: 7.3,
				CurrentBatteryEnergyKWh:   9.8,
				RequiredNightChargeKWh:    0,
				ShouldChargeTonight:       false,
				WouldWrite:                false,
				CommandFingerprint:        "none",
				CommandBlockReason:        "outside night charge window",
				ActionSummary:             "深夜充電は抑制",
				Reason:                    "sunny forecast",
				TargetForecast:            &domain.WeatherForecast{Date: targetDate},
			},
		}); err != nil {
			t.Fatalf("InsertNightChargePlanLog failed: %v", err)
		}
	}

	logs, total, err := repo.ListNightChargePlanLogsPage(context.Background(), 1, 0, NightChargePlanLogPageFilter{})
	if err != nil {
		t.Fatalf("ListNightChargePlanLogsPage failed: %v", err)
	}
	if total != 2 || len(logs) != 1 {
		t.Fatalf("total,len = %d,%d; want 2,1", total, len(logs))
	}
	if logs[0].BatterySoc != 81 || logs[0].RecommendedNightTargetSoc != 61 {
		t.Fatalf("unexpected newest log: %+v", logs[0])
	}
	if logs[0].TargetForecastDate == nil || *logs[0].TargetForecastDate != "2026-05-20" {
		t.Fatalf("TargetForecastDate = %v, want 2026-05-20", logs[0].TargetForecastDate)
	}
	if logs[0].CommandFingerprint != "none" {
		t.Fatalf("CommandFingerprint = %q, want none", logs[0].CommandFingerprint)
	}
}

func TestNightChargePlanRepositoryReturnsLatestWriteCandidate(t *testing.T) {
	db := openTestDB(t)
	repo := NewNightChargePlanRepository(db)
	base := time.Date(2026, 5, 20, 3, 0, 0, 0, time.UTC)
	message := "api down"

	logs := []domain.NightChargePlan{
		{
			StrategyState:             "NIGHT_RECOVER",
			RecommendedMode:           "self-powered",
			RecommendedNightTargetSoc: 42,
			CommandFingerprint:        "reserve=42|self-powered=on",
			WouldWrite:                true,
			CommandSent:               true,
			ActionSummary:             "sent",
			Reason:                    "sent",
		},
		{
			StrategyState:             "NIGHT_CHARGE_WINDOW",
			RecommendedMode:           "tou",
			RecommendedNightTargetSoc: 90,
			CommandFingerprint:        "ac=1500|reserve=90",
			CommandError:              &message,
			ActionSummary:             "error",
			Reason:                    "error",
		},
	}
	for i, plan := range logs {
		if err := repo.InsertNightChargePlanLog(context.Background(), domain.Status{
			BatterySoc:      80 + i,
			UpdatedAt:       base.Add(time.Duration(i) * time.Minute),
			NightChargePlan: &plan,
		}); err != nil {
			t.Fatalf("InsertNightChargePlanLog failed: %v", err)
		}
	}

	latest, err := repo.LatestNightChargePlanWriteCandidateLog(context.Background())
	if err != nil {
		t.Fatalf("LatestNightChargePlanWriteCandidateLog failed: %v", err)
	}
	if latest == nil {
		t.Fatal("latest = nil, want write candidate")
	}
	if latest.CommandFingerprint != "ac=1500|reserve=90" {
		t.Fatalf("CommandFingerprint = %q, want latest errored candidate", latest.CommandFingerprint)
	}
	if latest.CommandError == nil || *latest.CommandError != message {
		t.Fatalf("CommandError = %v, want %q", latest.CommandError, message)
	}
}

func TestNightChargePlanRepositoryFiltersByDateRange(t *testing.T) {
	db := openTestDB(t)
	repo := NewNightChargePlanRepository(db)
	base := time.Date(2026, 5, 19, 21, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		measuredAt := base.Add(time.Duration(i) * time.Hour)
		if err := repo.InsertNightChargePlanLog(context.Background(), domain.Status{
			BatterySoc: i,
			UpdatedAt:  measuredAt,
			NightChargePlan: &domain.NightChargePlan{
				StrategyState:             "NIGHT_PLAN_READY",
				RecommendedMode:           "observe",
				RecommendedNightTargetSoc: i,
				ActionSummary:             "sample",
				Reason:                    "sample",
			},
		}); err != nil {
			t.Fatalf("InsertNightChargePlanLog failed: %v", err)
		}
	}

	from := base.Add(30 * time.Minute)
	to := base.Add(90 * time.Minute)
	logs, total, err := repo.ListNightChargePlanLogsPage(context.Background(), 25, 0, NightChargePlanLogPageFilter{From: &from, To: &to})
	if err != nil {
		t.Fatalf("ListNightChargePlanLogsPage failed: %v", err)
	}
	if total != 1 || len(logs) != 1 || logs[0].BatterySoc != 1 {
		t.Fatalf("logs,total = %+v,%d; want only middle log", logs, total)
	}
}

func TestSurplusControlCommandRepositoryInsertsAndListsNewestFirst(t *testing.T) {
	db := openTestDB(t)
	repo := NewSurplusControlCommandRepository(db)
	base := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	reserve := 79

	for i := 0; i < 2; i++ {
		measuredAt := base.Add(time.Duration(i) * time.Minute)
		previousAC := 500
		targetAC := 800 + i*100
		if err := repo.InsertSurplusControlCommandLog(context.Background(), domain.SurplusControlCommandLog{
			MeasuredAt:                measuredAt,
			StrategyState:             "CHARGING",
			CommandKind:               "ac_charge_limit",
			CommandFingerprint:        "sample",
			GridW:                     -500 - i,
			ImportW:                   0,
			ExportW:                   500 + i,
			BatterySoc:                77,
			BatteryInputW:             740,
			BatteryOutputW:            250,
			PreviousACChargeLimitW:    &previousAC,
			TargetACChargeLimitW:      &targetAC,
			PreviousBackupReserveSoc:  &reserve,
			DryRun:                    true,
			WouldWrite:                i == 0,
			ShouldAdjustACChargeLimit: true,
			DecisionReason:            "AC充電上限を調整",
			CreatedAt:                 measuredAt,
		}); err != nil {
			t.Fatalf("InsertSurplusControlCommandLog failed: %v", err)
		}
	}

	logs, total, err := repo.ListSurplusControlCommandLogsPage(context.Background(), 1, 0, SurplusControlCommandLogPageFilter{})
	if err != nil {
		t.Fatalf("ListSurplusControlCommandLogsPage failed: %v", err)
	}
	if total != 2 || len(logs) != 1 {
		t.Fatalf("total,len = %d,%d; want 2,1", total, len(logs))
	}
	if logs[0].TargetACChargeLimitW == nil || *logs[0].TargetACChargeLimitW != 900 {
		t.Fatalf("TargetACChargeLimitW = %v, want 900", logs[0].TargetACChargeLimitW)
	}
	if !logs[0].DryRun || logs[0].CommandSent {
		t.Fatalf("DryRun,CommandSent = %v,%v; want true,false", logs[0].DryRun, logs[0].CommandSent)
	}
	latest, err := repo.LatestSurplusControlCommandLog(context.Background())
	if err != nil {
		t.Fatalf("LatestSurplusControlCommandLog failed: %v", err)
	}
	if latest == nil || latest.TargetACChargeLimitW == nil || *latest.TargetACChargeLimitW != 900 {
		t.Fatalf("latest = %+v, want target AC 900", latest)
	}
	latestCandidate, err := repo.LatestSurplusControlWriteCandidateLog(context.Background())
	if err != nil {
		t.Fatalf("LatestSurplusControlWriteCandidateLog failed: %v", err)
	}
	if latestCandidate == nil || latestCandidate.TargetACChargeLimitW == nil || *latestCandidate.TargetACChargeLimitW != 800 {
		t.Fatalf("latestCandidate = %+v, want target AC 800", latestCandidate)
	}
}

func TestSurplusControlCommandRepositoryTreatsErroredAttemptsAsWriteCandidates(t *testing.T) {
	db := openTestDB(t)
	repo := NewSurplusControlCommandRepository(db)
	base := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	targetAC := 800
	errorMessage := "set AC charge power: rejected"
	if err := repo.InsertSurplusControlCommandLog(context.Background(), domain.SurplusControlCommandLog{
		MeasuredAt:                base,
		StrategyState:             "CHARGING",
		CommandKind:               "ac_charge_limit",
		CommandFingerprint:        "kind=ac_charge_limit;ac=800;reserve=-;adjust_ac=true;set_reserve=false;disable_modes=false;enable_tou=false",
		GridW:                     -900,
		ExportW:                   900,
		BatterySoc:                77,
		TargetACChargeLimitW:      &targetAC,
		DryRun:                    false,
		WouldWrite:                false,
		ShouldAdjustACChargeLimit: true,
		ErrorMessage:              &errorMessage,
		CreatedAt:                 base,
	}); err != nil {
		t.Fatalf("InsertSurplusControlCommandLog failed: %v", err)
	}

	latestCandidate, err := repo.LatestSurplusControlWriteCandidateLog(context.Background())
	if err != nil {
		t.Fatalf("LatestSurplusControlWriteCandidateLog failed: %v", err)
	}
	if latestCandidate == nil || latestCandidate.ErrorMessage == nil || *latestCandidate.ErrorMessage != errorMessage {
		t.Fatalf("latestCandidate = %+v, want errored write attempt", latestCandidate)
	}
}

func TestNotificationRepositoryInsertsAndReadsLatest(t *testing.T) {
	db := openTestDB(t)
	repo := NewNotificationRepository(db)
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	message := "slack unavailable"

	if err := repo.InsertNotificationLog(context.Background(), domain.NotificationLog{
		MeasuredAt:      now,
		Kind:            "manual_charge_surplus",
		Fingerprint:     "sample",
		Severity:        "warning",
		Message:         "manual charge alert",
		Reason:          "AC charge is max",
		ExportW:         900,
		BatterySoc:      96,
		ACChargeLimitW:  1500,
		Sent:            false,
		ErrorMessage:    &message,
		ConsecutiveHits: 3,
		CreatedAt:       now,
	}); err != nil {
		t.Fatalf("InsertNotificationLog failed: %v", err)
	}

	got, err := repo.LatestNotificationLog(context.Background(), "manual_charge_surplus", "sample")
	if err != nil {
		t.Fatalf("LatestNotificationLog failed: %v", err)
	}
	if got == nil || got.ExportW != 900 || got.BatterySoc != 96 || got.ACChargeLimitW != 1500 {
		t.Fatalf("unexpected notification log: %+v", got)
	}
	if got.ErrorMessage == nil || *got.ErrorMessage != message {
		t.Fatalf("ErrorMessage = %v, want %q", got.ErrorMessage, message)
	}
}

func TestNightChargeSummaryRepositoryBuildsDailySummary(t *testing.T) {
	db := openTestDB(t)
	repo := NewNightChargeSummaryRepositoryWithTimezone(db, "UTC")
	base := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)

	insertNightSummaryPlanLog(t, db, base.Add(21*time.Hour+10*time.Minute), 70, 8.6, 0.4)
	insertNightSummaryPlanLog(t, db, base.Add(22*time.Hour+30*time.Minute), 75, 9.2, 1.1)
	for _, sample := range []struct {
		at             time.Time
		soc            int
		importW        int
		exportW        int
		batteryInputW  int
		batteryOutputW int
	}{
		{base.Add(23 * time.Hour), 80, 100, 0, 50, 250},
		{base.Add(25 * time.Hour), 79, 100, 0, 50, 250},
		{base.Add(27 * time.Hour), 78, 100, 0, 50, 250},
		{base.Add(29 * time.Hour), 77, 100, 0, 50, 250},
		{base.Add(31 * time.Hour), 76, 0, 300, 400, 100},
		{base.Add(33 * time.Hour), 76, 0, 300, 400, 100},
		{base.Add(35 * time.Hour), 77, 0, 300, 400, 100},
		{base.Add(37 * time.Hour), 78, 0, 300, 400, 100},
		{base.Add(39 * time.Hour), 79, 0, 300, 400, 100},
		{base.Add(40 * time.Hour), 80, 0, 300, 400, 100},
	} {
		insertNightSummaryPowerLog(t, db, sample.at, sample.soc, sample.importW, sample.exportW, sample.batteryInputW, sample.batteryOutputW)
	}

	summaries, total, err := repo.ListNightChargeDailySummariesPage(context.Background(), base.Add(41*time.Hour), 10, 0, NightChargeSummaryPageFilter{})
	if err != nil {
		t.Fatalf("ListNightChargeDailySummariesPage failed: %v", err)
	}
	if total != 1 || len(summaries) != 1 {
		t.Fatalf("total,len = %d,%d; want 1,1", total, len(summaries))
	}
	got := summaries[0]
	if got.SummaryDate != "2026-05-19" {
		t.Fatalf("SummaryDate = %s, want 2026-05-19", got.SummaryDate)
	}
	if got.PlannedTargetSoc == nil || *got.PlannedTargetSoc != 75 {
		t.Fatalf("PlannedTargetSoc = %v, want latest 75", got.PlannedTargetSoc)
	}
	if got.NightStartSoc == nil || *got.NightStartSoc != 80 || got.NightEndSoc == nil || *got.NightEndSoc != 76 {
		t.Fatalf("night start/end SOC = %v/%v, want 80/76", got.NightStartSoc, got.NightEndSoc)
	}
	if got.NightSocDelta == nil || *got.NightSocDelta != -4 {
		t.Fatalf("NightSocDelta = %v, want -4", got.NightSocDelta)
	}
	if got.MinNightSoc == nil || *got.MinNightSoc != 76 || got.MaxNightSoc == nil || *got.MaxNightSoc != 80 {
		t.Fatalf("min/max SOC = %v/%v, want 76/80", got.MinNightSoc, got.MaxNightSoc)
	}
	if got.NightImportKWh == nil || !floatAlmostEqual(*got.NightImportKWh, 0.8) {
		t.Fatalf("NightImportKWh = %v, want 0.8", got.NightImportKWh)
	}
	if got.NightBatteryOutputKWh == nil || !floatAlmostEqual(*got.NightBatteryOutputKWh, 2.0) {
		t.Fatalf("NightBatteryOutputKWh = %v, want 2.0", got.NightBatteryOutputKWh)
	}
	if got.DaytimeBatteryInputKWh == nil || got.DaytimeExportKWh == nil {
		t.Fatalf("daytime follow-up = %v/%v, want non-nil", got.DaytimeBatteryInputKWh, got.DaytimeExportKWh)
	}
	if got.MorningTargetSocGap == nil || *got.MorningTargetSocGap != 1 {
		t.Fatalf("MorningTargetSocGap = %v, want 1", got.MorningTargetSocGap)
	}
	if got.NightNetBatteryKWh == nil || !floatAlmostEqual(*got.NightNetBatteryKWh, -1.6) {
		t.Fatalf("NightNetBatteryKWh = %v, want -1.6", got.NightNetBatteryKWh)
	}
	if got.NightRequiredChargeGapKWh == nil || !floatAlmostEqual(*got.NightRequiredChargeGapKWh, -2.7) {
		t.Fatalf("NightRequiredChargeGapKWh = %v, want -2.7", got.NightRequiredChargeGapKWh)
	}
	if got.DaytimeChargeAndExportKWh == nil || !floatAlmostEqual(*got.DaytimeChargeAndExportKWh, 6.3) {
		t.Fatalf("DaytimeChargeAndExportKWh = %v, want 6.3", got.DaytimeChargeAndExportKWh)
	}
	if got.MorningStatus != "ok" || got.FinalResultStatus != "ok" {
		t.Fatalf("statuses = %s/%s, want ok/ok", got.MorningStatus, got.FinalResultStatus)
	}
	if got.DataSource != "power-log" {
		t.Fatalf("DataSource = %s, want power-log", got.DataSource)
	}
}

func TestNightChargeSummaryRepositoryKeepsCurrentNightPending(t *testing.T) {
	db := openTestDB(t)
	repo := NewNightChargeSummaryRepositoryWithTimezone(db, "UTC")
	base := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)

	insertNightSummaryPlanLog(t, db, base.Add(21*time.Hour+30*time.Minute), 75, 9.2, 1.1)
	insertNightSummaryPowerLog(t, db, base.Add(23*time.Hour), 80, 0, 0, 0, 250)

	summaries, total, err := repo.ListNightChargeDailySummariesPage(context.Background(), base.Add(28*time.Hour), 10, 0, NightChargeSummaryPageFilter{})
	if err != nil {
		t.Fatalf("ListNightChargeDailySummariesPage failed: %v", err)
	}
	if total != 1 || len(summaries) != 1 {
		t.Fatalf("total,len = %d,%d; want 1,1", total, len(summaries))
	}
	if summaries[0].MorningStatus != "pending" || summaries[0].FinalResultStatus != "pending" {
		t.Fatalf("statuses = %s/%s, want pending/pending", summaries[0].MorningStatus, summaries[0].FinalResultStatus)
	}
	if summaries[0].NightEndSoc != nil {
		t.Fatalf("NightEndSoc = %v, want nil before 07:00", summaries[0].NightEndSoc)
	}
}

func TestNightChargeSummaryRepositoryPagesSummaryDates(t *testing.T) {
	db := openTestDB(t)
	repo := NewNightChargeSummaryRepositoryWithTimezone(db, "UTC")
	base := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)

	for day := 0; day < 3; day++ {
		insertNightSummaryPlanLog(t, db, base.AddDate(0, 0, day).Add(21*time.Hour+10*time.Minute), 70+day, 8.6, 0.4)
	}
	insertNightSummaryPlanLog(t, db, base.AddDate(0, 0, 3).Add(12*time.Hour), 80, 9.8, 0.8)

	summaries, total, err := repo.ListNightChargeDailySummariesPage(context.Background(), base.AddDate(0, 0, 4), 1, 1, NightChargeSummaryPageFilter{})
	if err != nil {
		t.Fatalf("ListNightChargeDailySummariesPage failed: %v", err)
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3", total)
	}
	if len(summaries) != 1 || summaries[0].SummaryDate != "2026-05-20" {
		t.Fatalf("summaries = %#v, want only 2026-05-20", summaries)
	}
}

func TestNightChargeSummaryRepositoryFiltersSummaryDatesBySessionWindow(t *testing.T) {
	db := openTestDB(t)
	repo := NewNightChargeSummaryRepositoryWithTimezone(db, "UTC")
	base := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)

	for day := 0; day < 3; day++ {
		insertNightSummaryPlanLog(t, db, base.AddDate(0, 0, day).Add(21*time.Hour+10*time.Minute), 70+day, 8.6, 0.4)
	}

	from := base.AddDate(0, 0, 1).Add(21 * time.Hour)
	to := base.AddDate(0, 0, 2).Add(16 * time.Hour)
	summaries, total, err := repo.ListNightChargeDailySummariesPage(context.Background(), base.AddDate(0, 0, 4), 10, 0, NightChargeSummaryPageFilter{
		From: &from,
		To:   &to,
	})
	if err != nil {
		t.Fatalf("ListNightChargeDailySummariesPage failed: %v", err)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
	if len(summaries) != 1 || summaries[0].SummaryDate != "2026-05-20" {
		t.Fatalf("summaries = %#v, want only 2026-05-20", summaries)
	}
}

func TestNightChargeSummaryRepositoryPagesSummaryDatesInDSTTimezone(t *testing.T) {
	db := openTestDB(t)
	repo := NewNightChargeSummaryRepositoryWithTimezone(db, "America/New_York")
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("LoadLocation failed: %v", err)
	}

	insertNightSummaryPlanLog(t, db, time.Date(2026, 1, 5, 21, 10, 0, 0, location), 70, 8.6, 0.4)
	insertNightSummaryPlanLog(t, db, time.Date(2026, 7, 5, 21, 10, 0, 0, location), 75, 9.2, 1.1)
	insertNightSummaryPlanLog(t, db, time.Date(2026, 7, 6, 12, 0, 0, 0, location), 80, 9.8, 0.8)

	summaries, total, err := repo.ListNightChargeDailySummariesPage(context.Background(), time.Date(2026, 7, 7, 12, 0, 0, 0, location), 10, 0, NightChargeSummaryPageFilter{})
	if err != nil {
		t.Fatalf("ListNightChargeDailySummariesPage failed: %v", err)
	}
	if total != 2 {
		t.Fatalf("total = %d, want 2", total)
	}
	if len(summaries) != 2 || summaries[0].SummaryDate != "2026-07-05" || summaries[1].SummaryDate != "2026-01-05" {
		t.Fatalf("summaries = %#v, want 2026-07-05 then 2026-01-05", summaries)
	}
}

func insertNightSummaryPlanLog(t *testing.T, db *sql.DB, measuredAt time.Time, targetSoc int, targetKWh float64, requiredKWh float64) {
	t.Helper()
	targetDate := measuredAt.AddDate(0, 0, 1).Format("2006-01-02")
	if err := NewNightChargePlanRepository(db).InsertNightChargePlanLog(context.Background(), domain.Status{
		GridW:          100,
		ImportW:        100,
		BatterySoc:     80,
		BatteryInputW:  50,
		BatteryOutputW: 250,
		UpdatedAt:      measuredAt,
		NightChargePlan: &domain.NightChargePlan{
			StrategyState:             "NIGHT_PLAN_READY",
			RecommendedMode:           "tou",
			RecommendedNightTargetSoc: targetSoc,
			RecommendedNightTargetKWh: targetKWh,
			CurrentBatteryEnergyKWh:   9.8,
			RequiredNightChargeKWh:    requiredKWh,
			ShouldChargeTonight:       requiredKWh > 0,
			ActionSummary:             "sample night plan",
			Reason:                    "sample night plan reason",
			TargetForecast:            &domain.WeatherForecast{Date: targetDate},
		},
	}); err != nil {
		t.Fatalf("InsertNightChargePlanLog failed: %v", err)
	}
}

func insertNightSummaryPowerLog(t *testing.T, db *sql.DB, measuredAt time.Time, soc int, importW int, exportW int, batteryInputW int, batteryOutputW int) {
	t.Helper()
	if err := NewLogRepository(db).InsertPowerLog(context.Background(), domain.PowerLog{
		MeasuredAt:     measuredAt,
		GridW:          importW - exportW,
		ImportW:        importW,
		ExportW:        exportW,
		BatterySoc:     &soc,
		BatteryInputW:  &batteryInputW,
		BatteryOutputW: &batteryOutputW,
		TargetChargeW:  batteryInputW,
		DecisionReason: "sample",
		Mode:           "mock",
		CommandSent:    false,
		CreatedAt:      measuredAt,
	}); err != nil {
		t.Fatalf("InsertPowerLog failed: %v", err)
	}
}

func TestEnergyMeterRepositoryIgnoresDuplicateReading(t *testing.T) {
	db := openTestDB(t)
	repo := NewEnergyMeterRepository(db)
	now := time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC)
	reading := domain.EnergyMeterReading{
		MeasuredAt:           now,
		ImportCumulativeKWh:  1000,
		ExportCumulativeKWh:  500,
		Coefficient:          1,
		CumulativeUnit:       0.1,
		RawImportCumulative:  "00002710",
		RawExportCumulative:  "00001388",
		ImportValueUpdatedAt: now,
		ExportValueUpdatedAt: now,
	}

	if err := repo.InsertEnergyMeterReading(context.Background(), reading); err != nil {
		t.Fatalf("InsertEnergyMeterReading first failed: %v", err)
	}
	if err := repo.InsertEnergyMeterReading(context.Background(), reading); err != nil {
		t.Fatalf("InsertEnergyMeterReading duplicate failed: %v", err)
	}

	logs, err := repo.ListEnergyMeterLogs(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListEnergyMeterLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs) = %d, want 1", len(logs))
	}
}

func TestTariffRepositorySummarizesEnergyCost(t *testing.T) {
	db := openTestDB(t)
	meterRepo := NewEnergyMeterRepository(db)
	tariffRepo := NewTariffRepository(db)
	base := time.Date(2026, 5, 18, 8, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	readings := []domain.EnergyMeterReading{
		{
			MeasuredAt:           base,
			ImportCumulativeKWh:  1000,
			ExportCumulativeKWh:  500,
			Coefficient:          1,
			CumulativeUnit:       0.1,
			RawImportCumulative:  "00002710",
			RawExportCumulative:  "00001388",
			ImportValueUpdatedAt: base,
			ExportValueUpdatedAt: base,
		},
		{
			MeasuredAt:           base.Add(2 * time.Hour),
			ImportCumulativeKWh:  1001,
			ExportCumulativeKWh:  500.2,
			Coefficient:          1,
			CumulativeUnit:       0.1,
			RawImportCumulative:  "0000271a",
			RawExportCumulative:  "0000138a",
			ImportValueUpdatedAt: base.Add(2 * time.Hour),
			ExportValueUpdatedAt: base.Add(2 * time.Hour),
		},
	}
	for _, reading := range readings {
		if err := meterRepo.InsertEnergyMeterReading(context.Background(), reading); err != nil {
			t.Fatalf("InsertEnergyMeterReading failed: %v", err)
		}
	}

	summary, err := tariffRepo.EnergyCostSummary(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("EnergyCostSummary failed: %v", err)
	}
	if summary.SampleCount != 1 {
		t.Fatalf("SampleCount = %d, want 1", summary.SampleCount)
	}
	if !floatAlmostEqual(summary.TotalImportKWh, 1) || !floatAlmostEqual(summary.TotalExportKWh, 0.2) {
		t.Fatalf("totals = import %v export %v, want 1 and 0.2", summary.TotalImportKWh, summary.TotalExportKWh)
	}
	if !floatAlmostEqual(summary.TotalImportCostYen, 34.06) {
		t.Fatalf("TotalImportCostYen = %v, want 34.06", summary.TotalImportCostYen)
	}
	if !floatAlmostEqual(summary.TotalExportIncomeYen, 1.4) {
		t.Fatalf("TotalExportIncomeYen = %v, want 1.4", summary.TotalExportIncomeYen)
	}
	if !floatAlmostEqual(summary.NetCostYen, 32.66) {
		t.Fatalf("NetCostYen = %v, want 32.66", summary.NetCostYen)
	}
}

func TestTariffRepositoryUsesHistoricalPlanAtMeasuredAt(t *testing.T) {
	db := openTestDB(t)
	meterRepo := NewEnergyMeterRepository(db)
	tariffRepo := NewTariffRepository(db)
	jst := time.FixedZone("JST", 9*60*60)
	base := time.Date(2026, 5, 18, 8, 0, 0, 0, jst)
	if _, err := tariffRepo.UpsertTariffPlan(context.Background(), domain.TariffPlan{
		PlanName:      "new rates",
		DayRateYen:    40,
		HomeRateYen:   30,
		NightRateYen:  20,
		ExportRateYen: 8,
		Timezone:      "Asia/Tokyo",
		EffectiveFrom: base.Add(3 * time.Hour),
	}); err != nil {
		t.Fatalf("UpsertTariffPlan failed: %v", err)
	}
	readings := []domain.EnergyMeterReading{
		{
			MeasuredAt:           base,
			ImportCumulativeKWh:  1000,
			ExportCumulativeKWh:  500,
			Coefficient:          1,
			CumulativeUnit:       0.1,
			RawImportCumulative:  "00002710",
			RawExportCumulative:  "00001388",
			ImportValueUpdatedAt: base,
			ExportValueUpdatedAt: base,
		},
		{
			MeasuredAt:           base.Add(2 * time.Hour),
			ImportCumulativeKWh:  1001,
			ExportCumulativeKWh:  500,
			Coefficient:          1,
			CumulativeUnit:       0.1,
			RawImportCumulative:  "0000271a",
			RawExportCumulative:  "00001388",
			ImportValueUpdatedAt: base.Add(2 * time.Hour),
			ExportValueUpdatedAt: base.Add(2 * time.Hour),
		},
		{
			MeasuredAt:           base.Add(4 * time.Hour),
			ImportCumulativeKWh:  1002,
			ExportCumulativeKWh:  500,
			Coefficient:          1,
			CumulativeUnit:       0.1,
			RawImportCumulative:  "00002724",
			RawExportCumulative:  "00001388",
			ImportValueUpdatedAt: base.Add(4 * time.Hour),
			ExportValueUpdatedAt: base.Add(4 * time.Hour),
		},
	}
	for _, reading := range readings {
		if err := meterRepo.InsertEnergyMeterReading(context.Background(), reading); err != nil {
			t.Fatalf("InsertEnergyMeterReading failed: %v", err)
		}
	}

	summary, err := tariffRepo.EnergyCostSummary(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("EnergyCostSummary failed: %v", err)
	}
	if summary.SampleCount != 2 {
		t.Fatalf("SampleCount = %d, want 2", summary.SampleCount)
	}
	if !floatAlmostEqual(summary.TotalImportCostYen, 74.06) {
		t.Fatalf("TotalImportCostYen = %v, want 74.06", summary.TotalImportCostYen)
	}
	if len(summary.Periods) != 2 {
		t.Fatalf("len(Periods) = %d, want 2: %#v", len(summary.Periods), summary.Periods)
	}
}

func TestTariffRepositoryDeletesPlanAndRecalculatesEffectiveTo(t *testing.T) {
	db := openTestDB(t)
	repo := NewTariffRepository(db)
	jst := time.FixedZone("JST", 9*60*60)
	firstStart := time.Date(2026, 6, 1, 0, 0, 0, 0, jst)
	secondStart := time.Date(2026, 7, 1, 0, 0, 0, 0, jst)
	first, err := repo.UpsertTariffPlan(context.Background(), domain.TariffPlan{
		PlanName:      "first",
		DayRateYen:    35,
		HomeRateYen:   27,
		NightRateYen:  17,
		ExportRateYen: 7,
		Timezone:      "Asia/Tokyo",
		EffectiveFrom: firstStart,
	})
	if err != nil {
		t.Fatalf("UpsertTariffPlan first failed: %v", err)
	}
	if _, err := repo.UpsertTariffPlan(context.Background(), domain.TariffPlan{
		PlanName:      "second",
		DayRateYen:    36,
		HomeRateYen:   28,
		NightRateYen:  18,
		ExportRateYen: 8,
		Timezone:      "Asia/Tokyo",
		EffectiveFrom: secondStart,
	}); err != nil {
		t.Fatalf("UpsertTariffPlan second failed: %v", err)
	}

	if err := repo.DeleteTariffPlan(context.Background(), first.ID); err != nil {
		t.Fatalf("DeleteTariffPlan failed: %v", err)
	}
	plans, err := repo.ListTariffPlans(context.Background())
	if err != nil {
		t.Fatalf("ListTariffPlans failed: %v", err)
	}
	if len(plans) != 2 {
		t.Fatalf("len(plans) = %d, want 2: %#v", len(plans), plans)
	}
	if plans[0].PlanName != "second" || plans[0].EffectiveTo != nil {
		t.Fatalf("newest plan = %#v, want open-ended second", plans[0])
	}
	if plans[1].EffectiveTo == nil || !plans[1].EffectiveTo.Equal(plans[0].EffectiveFrom) {
		t.Fatalf("baseline effectiveTo = %v, want %s", plans[1].EffectiveTo, plans[0].EffectiveFrom)
	}
}

func TestTariffRepositoryDoesNotDeleteCurrentCoverage(t *testing.T) {
	db := openTestDB(t)
	repo := NewTariffRepository(db)
	plans, err := repo.ListTariffPlans(context.Background())
	if err != nil {
		t.Fatalf("ListTariffPlans failed: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("len(plans) = %d, want seeded single plan", len(plans))
	}
	if _, err := repo.UpsertTariffPlan(context.Background(), domain.TariffPlan{
		PlanName:      "future",
		DayRateYen:    35,
		HomeRateYen:   27,
		NightRateYen:  17,
		ExportRateYen: 7,
		Timezone:      "Asia/Tokyo",
		EffectiveFrom: time.Now().Add(24 * time.Hour),
	}); err != nil {
		t.Fatalf("UpsertTariffPlan future failed: %v", err)
	}

	if err := repo.DeleteTariffPlan(context.Background(), plans[0].ID); err != ErrTariffPlanCoverageRequired {
		t.Fatalf("DeleteTariffPlan error = %v, want ErrTariffPlanCoverageRequired", err)
	}
	after, err := repo.ListTariffPlans(context.Background())
	if err != nil {
		t.Fatalf("ListTariffPlans after failed: %v", err)
	}
	if len(after) != 2 {
		t.Fatalf("len(after) = %d, want rollback to keep both plans", len(after))
	}
}

func TestTariffRepositoryDoesNotDeleteLastPlan(t *testing.T) {
	db := openTestDB(t)
	repo := NewTariffRepository(db)
	plans, err := repo.ListTariffPlans(context.Background())
	if err != nil {
		t.Fatalf("ListTariffPlans failed: %v", err)
	}
	if len(plans) != 1 {
		t.Fatalf("len(plans) = %d, want seeded single plan", len(plans))
	}
	if err := repo.DeleteTariffPlan(context.Background(), plans[0].ID); err != ErrCannotDeleteLastTariffPlan {
		t.Fatalf("DeleteTariffPlan error = %v, want ErrCannotDeleteLastTariffPlan", err)
	}
}

func TestWeatherSettingsRepositoryUpdatesAndReadsLocation(t *testing.T) {
	db := openTestDB(t)
	repo := NewWeatherSettingsRepository(db)
	want := domain.WeatherLocation{
		Enabled:            true,
		Latitude:           35.362502,
		Longitude:          136.9253633,
		Timezone:           "Asia/Tokyo",
		PVCapacityKW:       5.5,
		PVPerformanceRatio: 0.78,
		DailyBaseLoadKWh:   8.2,
		BatteryCapacityKWh: 4.096,
		MinimumReserveSoc:  35,
	}

	if err := repo.UpdateWeatherLocation(context.Background(), want); err != nil {
		t.Fatalf("UpdateWeatherLocation failed: %v", err)
	}
	got, err := repo.CurrentWeatherLocation(context.Background())
	if err != nil {
		t.Fatalf("CurrentWeatherLocation failed: %v", err)
	}
	if got != want {
		t.Fatalf("WeatherLocation = %+v, want %+v", got, want)
	}
}

func TestDaytimeConsumptionRepositoryEstimatesFromLogs(t *testing.T) {
	db := openTestDB(t)
	logRepo := NewLogRepository(db)
	estimateRepo := NewDaytimeConsumptionRepository(db)
	base := time.Date(2026, 5, 18, 9, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	for i := 0; i <= 2; i++ {
		at := base.Add(time.Duration(i) * time.Minute)
		input := 200
		output := 100
		if err := logRepo.InsertPowerLog(context.Background(), domain.PowerLog{
			MeasuredAt:     at,
			GridW:          300,
			ImportW:        300,
			ExportW:        0,
			BatteryInputW:  &input,
			BatteryOutputW: &output,
			DecisionReason: "sample",
			Mode:           "test",
			CreatedAt:      at,
		}); err != nil {
			t.Fatalf("InsertPowerLog failed: %v", err)
		}
	}

	estimate, err := estimateRepo.EstimateDaytimeConsumption(context.Background(), base.Add(24*time.Hour), 7)
	if err != nil {
		t.Fatalf("EstimateDaytimeConsumption failed: %v", err)
	}
	if estimate.SampleCount != 2 {
		t.Fatalf("SampleCount = %d, want 2", estimate.SampleCount)
	}
	if !floatAlmostEqual(estimate.SuggestedDailyBaseLoadKWh, 0.0066666667) {
		t.Fatalf("SuggestedDailyBaseLoadKWh = %f, want about 0.006667", estimate.SuggestedDailyBaseLoadKWh)
	}
}

func TestEcoFlowLoadRepositoryEstimatesSpecificCircuitOutput(t *testing.T) {
	db := openTestDB(t)
	logRepo := NewLogRepository(db)
	estimateRepo := NewEcoFlowLoadRepository(db)
	base := time.Date(2026, 5, 18, 9, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	for i := 0; i <= 2; i++ {
		at := base.Add(time.Duration(i) * time.Minute)
		input := 500
		output := 300
		if err := logRepo.InsertPowerLog(context.Background(), domain.PowerLog{
			MeasuredAt:     at,
			BatteryInputW:  &input,
			BatteryOutputW: &output,
			DecisionReason: "sample",
			Mode:           "test",
			CreatedAt:      at,
		}); err != nil {
			t.Fatalf("InsertPowerLog daytime failed: %v", err)
		}
	}
	nightBase := time.Date(2026, 5, 18, 23, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	for i := 0; i <= 2; i++ {
		at := nightBase.Add(time.Duration(i) * time.Minute)
		output := 150
		if err := logRepo.InsertPowerLog(context.Background(), domain.PowerLog{
			MeasuredAt:     at,
			BatteryOutputW: &output,
			DecisionReason: "sample",
			Mode:           "test",
			CreatedAt:      at,
		}); err != nil {
			t.Fatalf("InsertPowerLog night failed: %v", err)
		}
	}
	shoulderBase := time.Date(2026, 5, 18, 17, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	for i := 0; i <= 2; i++ {
		at := shoulderBase.Add(time.Duration(i) * time.Minute)
		output := 240
		if err := logRepo.InsertPowerLog(context.Background(), domain.PowerLog{
			MeasuredAt:     at,
			BatteryOutputW: &output,
			DecisionReason: "sample",
			Mode:           "test",
			CreatedAt:      at,
		}); err != nil {
			t.Fatalf("InsertPowerLog shoulder failed: %v", err)
		}
	}

	estimate, err := estimateRepo.EstimateEcoFlowLoad(context.Background(), base.Add(24*time.Hour), 7)
	if err != nil {
		t.Fatalf("EstimateEcoFlowLoad failed: %v", err)
	}
	if estimate.SampleCount != 6 {
		t.Fatalf("SampleCount = %d, want 6", estimate.SampleCount)
	}
	if !floatAlmostEqual(estimate.AverageDaytimeOutputKWh, 0.01) {
		t.Fatalf("AverageDaytimeOutputKWh = %f, want 0.01", estimate.AverageDaytimeOutputKWh)
	}
	if !floatAlmostEqual(estimate.AverageNightOutputKWh, 0.005) {
		t.Fatalf("AverageNightOutputKWh = %f, want 0.005", estimate.AverageNightOutputKWh)
	}
	if !floatAlmostEqual(estimate.AverageShoulderOutputKWh, 0.008) {
		t.Fatalf("AverageShoulderOutputKWh = %f, want 0.008", estimate.AverageShoulderOutputKWh)
	}
	if !floatAlmostEqual(estimate.AverageDailyOutputKWh, 0.023) {
		t.Fatalf("AverageDailyOutputKWh = %f, want 0.023", estimate.AverageDailyOutputKWh)
	}
	if !floatAlmostEqual(estimate.SuggestedDaytimeBaseLoadKWh, 0.01) {
		t.Fatalf("SuggestedDaytimeBaseLoadKWh = %f, want 0.01", estimate.SuggestedDaytimeBaseLoadKWh)
	}
	if estimate.DaytimeSampleDays != 1 || estimate.CompleteDaytimeSampleDays != 1 {
		t.Fatalf("daytime sample days = %d/%d, want 1/1", estimate.DaytimeSampleDays, estimate.CompleteDaytimeSampleDays)
	}
}

func TestEcoFlowLoadRepositoryExcludesIncompleteDaytimeFromAverage(t *testing.T) {
	db := openTestDB(t)
	logRepo := NewLogRepository(db)
	estimateRepo := NewEcoFlowLoadRepositoryWithTimezone(db, "Asia/Tokyo")
	completeDayBase := time.Date(2026, 5, 18, 9, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	incompleteDayBase := time.Date(2026, 5, 19, 9, 0, 0, 0, time.FixedZone("JST", 9*60*60))

	for i := 0; i <= 2; i++ {
		output := 300
		at := completeDayBase.Add(time.Duration(i) * time.Minute)
		if err := logRepo.InsertPowerLog(context.Background(), domain.PowerLog{
			MeasuredAt:     at,
			BatteryOutputW: &output,
			DecisionReason: "complete daytime sample",
			Mode:           "test",
			CreatedAt:      at,
		}); err != nil {
			t.Fatalf("InsertPowerLog complete daytime failed: %v", err)
		}
	}
	for i := 0; i <= 2; i++ {
		output := 900
		at := incompleteDayBase.Add(time.Duration(i) * time.Minute)
		if err := logRepo.InsertPowerLog(context.Background(), domain.PowerLog{
			MeasuredAt:     at,
			BatteryOutputW: &output,
			DecisionReason: "incomplete daytime sample",
			Mode:           "test",
			CreatedAt:      at,
		}); err != nil {
			t.Fatalf("InsertPowerLog incomplete daytime failed: %v", err)
		}
	}

	estimate, err := estimateRepo.EstimateEcoFlowLoad(context.Background(), incompleteDayBase.Add(time.Hour), 7)
	if err != nil {
		t.Fatalf("EstimateEcoFlowLoad failed: %v", err)
	}
	if estimate.DaytimeSampleDays != 2 || estimate.CompleteDaytimeSampleDays != 1 {
		t.Fatalf("daytime sample days = %d/%d, want 2/1", estimate.DaytimeSampleDays, estimate.CompleteDaytimeSampleDays)
	}
	if !floatAlmostEqual(estimate.AverageDaytimeOutputKWh, 0.01) {
		t.Fatalf("AverageDaytimeOutputKWh = %f, want completed day only 0.01", estimate.AverageDaytimeOutputKWh)
	}
	if !floatAlmostEqual(estimate.SuggestedDaytimeBaseLoadKWh, 0.01) {
		t.Fatalf("SuggestedDaytimeBaseLoadKWh = %f, want completed day only 0.01", estimate.SuggestedDaytimeBaseLoadKWh)
	}
	for _, day := range estimate.Daily {
		if day.Date == "2026-05-19" && day.DaytimeComplete {
			t.Fatalf("2026-05-19 DaytimeComplete = true, want false before 16:00")
		}
	}
}

func TestEcoFlowLoadRepositoryUsesConfiguredTimezoneForBuckets(t *testing.T) {
	db := openTestDB(t)
	logRepo := NewLogRepository(db)
	estimateRepo := NewEcoFlowLoadRepositoryWithTimezone(db, "Asia/Tokyo")
	base := time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC) // 09:00 JST
	samples := []struct {
		at     time.Time
		output int
	}{
		{at: base, output: 300},
		{at: base.Add(time.Minute), output: 300},
		{at: time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC), output: 240}, // 17:00 JST
		{at: time.Date(2026, 5, 18, 8, 1, 0, 0, time.UTC), output: 240},
		{at: time.Date(2026, 5, 18, 14, 0, 0, 0, time.UTC), output: 150}, // 23:00 JST
		{at: time.Date(2026, 5, 18, 14, 1, 0, 0, time.UTC), output: 150},
	}
	for _, sample := range samples {
		output := sample.output
		if err := logRepo.InsertPowerLog(context.Background(), domain.PowerLog{
			MeasuredAt:     sample.at,
			BatteryOutputW: &output,
			DecisionReason: "sample",
			Mode:           "test",
			CreatedAt:      sample.at,
		}); err != nil {
			t.Fatalf("InsertPowerLog failed: %v", err)
		}
	}

	estimate, err := estimateRepo.EstimateEcoFlowLoad(context.Background(), base.Add(24*time.Hour), 7)
	if err != nil {
		t.Fatalf("EstimateEcoFlowLoad failed: %v", err)
	}
	if estimate.SampleCount != 3 {
		t.Fatalf("SampleCount = %d, want 3", estimate.SampleCount)
	}
	if !floatAlmostEqual(estimate.AverageDaytimeOutputKWh, 0.005) {
		t.Fatalf("AverageDaytimeOutputKWh = %f, want 0.005", estimate.AverageDaytimeOutputKWh)
	}
	if !floatAlmostEqual(estimate.AverageShoulderOutputKWh, 0.004) {
		t.Fatalf("AverageShoulderOutputKWh = %f, want 0.004", estimate.AverageShoulderOutputKWh)
	}
	if !floatAlmostEqual(estimate.AverageNightOutputKWh, 0.0025) {
		t.Fatalf("AverageNightOutputKWh = %f, want 0.0025", estimate.AverageNightOutputKWh)
	}
}

func floatAlmostEqual(got float64, want float64) bool {
	return math.Abs(got-want) < 0.000001
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db close failed: %v", err)
		}
	})
	if err := migrate(db); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return db
}
