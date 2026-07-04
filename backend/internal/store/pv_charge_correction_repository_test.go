package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

func insertPVCorrectionPlanLog(t *testing.T, db *sql.DB, targetDate string, forecastPVKWh float64) {
	t.Helper()
	measuredAt := targetDate + "T00:10:00+09:00"
	_, err := db.Exec(`INSERT INTO night_charge_plan_logs (
		measured_at, strategy_state, recommended_mode, recommended_night_target_soc,
		recommended_night_target_kwh, current_battery_energy_kwh, required_night_charge_kwh,
		daily_estimated_pv_kwh, pv_effective_start_at, pv_effective_end_at,
		battery_soc, battery_input_w, battery_output_w, grid_w, import_w, export_w,
		should_charge_tonight, would_write, command_block_reason, action_summary, reason,
		target_forecast_date, created_at
	) VALUES (?, 'NIGHT_CHARGE_WINDOW', 'tou', 50, 6.0, 3.0, 3.0, ?, ?, ?, 30, 0, 300, 500, 500, 0, 1, 0, '', '', '', ?, ?)`,
		measuredAt, forecastPVKWh, targetDate+"T10:00:00+09:00", targetDate+"T14:00:00+09:00", targetDate, measuredAt)
	if err != nil {
		t.Fatalf("insert night charge plan log: %v", err)
	}
}

func insertPVCorrectionPowerLogs(t *testing.T, db *sql.DB, date string, startHour, endHour, inputW, exportW, importW int) {
	t.Helper()
	location, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	day, err := time.ParseInLocation("2006-01-02", date, location)
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	start := day.Add(time.Duration(startHour) * time.Hour)
	minutes := (endHour - startHour) * 60
	for minute := 0; minute <= minutes; minute++ {
		measuredAt := start.Add(time.Duration(minute) * time.Minute).Format(time.RFC3339Nano)
		_, err := db.Exec(`INSERT INTO power_logs (
			measured_at, grid_w, import_w, export_w, battery_soc, battery_input_w, battery_output_w,
			decision_reason, mode, created_at
		) VALUES (?, ?, ?, ?, 50, ?, 300, 'test', 'test', ?)`,
			measuredAt, importW-exportW, importW, exportW, inputW, measuredAt)
		if err != nil {
			t.Fatalf("insert power log: %v", err)
		}
	}
}

func TestPVChargeCorrectionEnsureUpToDateBuildsDailyLog(t *testing.T) {
	db := openTestDB(t)
	repo := NewPVChargeCorrectionRepositoryWithTimezone(db, "Asia/Tokyo")
	location, _ := time.LoadLocation("Asia/Tokyo")

	// 予測PV 10kWh、実績はPV窓(10-14時)で input 1000W + export 500W, import 0 → usable 6kWh → factor 0.6
	insertPVCorrectionPlanLog(t, db, "2026-07-01", 10)
	insertPVCorrectionPowerLogs(t, db, "2026-07-01", 10, 14, 1000, 500, 0)

	now := time.Date(2026, 7, 3, 12, 0, 0, 0, location)
	if err := repo.EnsureUpToDate(context.Background(), now); err != nil {
		t.Fatalf("EnsureUpToDate: %v", err)
	}

	var factor float64
	var quality string
	err := db.QueryRow(`SELECT actual_capture_factor, sample_quality FROM pv_charge_correction_daily_logs WHERE date = '2026-07-01'`).Scan(&factor, &quality)
	if err != nil {
		t.Fatalf("daily log not created: %v", err)
	}
	if quality != "ok" {
		t.Fatalf("sample_quality = %q, want ok", quality)
	}
	if math.Abs(factor-0.6) > 0.02 {
		t.Fatalf("actual_capture_factor = %f, want ~0.6", factor)
	}
}

func TestPVChargeCorrectionRefreshUpdatesFactorAfterEnoughSamples(t *testing.T) {
	db := openTestDB(t)
	repo := NewPVChargeCorrectionRepositoryWithTimezone(db, "Asia/Tokyo")
	location, _ := time.LoadLocation("Asia/Tokyo")

	for day := 1; day <= 7; day++ {
		date := fmt.Sprintf("2026-07-%02d", day)
		if err := repo.UpsertDailyLog(context.Background(), pvCorrectionLogFixture(date, 0.5)); err != nil {
			t.Fatalf("UpsertDailyLog: %v", err)
		}
	}

	now := time.Date(2026, 7, 9, 12, 0, 0, 0, location)
	if err := repo.EnsureUpToDate(context.Background(), now); err != nil {
		t.Fatalf("EnsureUpToDate: %v", err)
	}

	settings, err := NewWeatherSettingsRepository(db).CurrentWeatherLocation(context.Background())
	if err != nil {
		t.Fatalf("CurrentWeatherLocation: %v", err)
	}
	if math.Abs(settings.PVChargeCorrectionFactor-0.5) > 0.001 {
		t.Fatalf("PVChargeCorrectionFactor = %f, want 0.5", settings.PVChargeCorrectionFactor)
	}
	if settings.PVChargeCorrectionUpdatedAt == "" {
		t.Fatal("PVChargeCorrectionUpdatedAt should be set by auto update")
	}

	recommendation, err := repo.PVChargeCorrectionRecommendation(context.Background(), now)
	if err != nil {
		t.Fatalf("PVChargeCorrectionRecommendation: %v", err)
	}
	if !recommendation.Applicable || recommendation.Status != "ok" {
		t.Fatalf("recommendation = %+v, want applicable ok", recommendation)
	}
	if recommendation.OKSampleDays != 7 {
		t.Fatalf("OKSampleDays = %d, want 7", recommendation.OKSampleDays)
	}
}

func TestPVChargeCorrectionRefreshSkipsManualFactor(t *testing.T) {
	db := openTestDB(t)
	repo := NewPVChargeCorrectionRepositoryWithTimezone(db, "Asia/Tokyo")
	location, _ := time.LoadLocation("Asia/Tokyo")

	if _, err := db.Exec(`UPDATE settings SET pv_charge_correction_manual = 1, pv_charge_correction_factor = 0.85 WHERE id = 1`); err != nil {
		t.Fatalf("set manual factor: %v", err)
	}
	for day := 1; day <= 7; day++ {
		date := fmt.Sprintf("2026-07-%02d", day)
		if err := repo.UpsertDailyLog(context.Background(), pvCorrectionLogFixture(date, 0.4)); err != nil {
			t.Fatalf("UpsertDailyLog: %v", err)
		}
	}

	now := time.Date(2026, 7, 9, 12, 0, 0, 0, location)
	if err := repo.EnsureUpToDate(context.Background(), now); err != nil {
		t.Fatalf("EnsureUpToDate: %v", err)
	}

	settings, err := NewWeatherSettingsRepository(db).CurrentWeatherLocation(context.Background())
	if err != nil {
		t.Fatalf("CurrentWeatherLocation: %v", err)
	}
	if math.Abs(settings.PVChargeCorrectionFactor-0.85) > 0.001 {
		t.Fatalf("PVChargeCorrectionFactor = %f, want 0.85 (manual value kept)", settings.PVChargeCorrectionFactor)
	}
}

func TestPVChargeCorrectionSkipsRefreshWithFewSamples(t *testing.T) {
	db := openTestDB(t)
	repo := NewPVChargeCorrectionRepositoryWithTimezone(db, "Asia/Tokyo")
	location, _ := time.LoadLocation("Asia/Tokyo")

	for day := 1; day <= 3; day++ {
		date := fmt.Sprintf("2026-07-%02d", day)
		if err := repo.UpsertDailyLog(context.Background(), pvCorrectionLogFixture(date, 0.4)); err != nil {
			t.Fatalf("UpsertDailyLog: %v", err)
		}
	}

	now := time.Date(2026, 7, 9, 12, 0, 0, 0, location)
	if err := repo.EnsureUpToDate(context.Background(), now); err != nil {
		t.Fatalf("EnsureUpToDate: %v", err)
	}

	settings, err := NewWeatherSettingsRepository(db).CurrentWeatherLocation(context.Background())
	if err != nil {
		t.Fatalf("CurrentWeatherLocation: %v", err)
	}
	if math.Abs(settings.PVChargeCorrectionFactor-0.7) > 0.001 {
		t.Fatalf("PVChargeCorrectionFactor = %f, want default 0.7 (not enough samples)", settings.PVChargeCorrectionFactor)
	}
}

func pvCorrectionLogFixture(date string, factor float64) domain.PVChargeCorrectionDailyLog {
	return domain.PVChargeCorrectionDailyLog{
		Date:                  date,
		ForecastPVKWh:         10,
		ActualBatteryInputKWh: 10 * factor,
		ActualCaptureFactor:   factor,
		SampleQuality:         "ok",
	}
}
