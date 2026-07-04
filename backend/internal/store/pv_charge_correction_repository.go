package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const (
	pvChargeCorrectionLookbackDays = 14
	// PV発電予測がこのkWh未満の日は capture factor のノイズが大きいため学習対象外にする
	pvChargeCorrectionMinForecastKWh = 1.0
	// PV有効時間帯のうち最低このサンプル時間が無い日は insufficient-data 扱いにする
	pvChargeCorrectionMinSampledHours = 2.0
	// 学習係数の反映は現行値との差がこの値以上のときだけ行う
	pvChargeCorrectionMinFactorDelta = 0.01
)

type PVChargeCorrectionRepository struct {
	db       *sql.DB
	location *time.Location
}

func NewPVChargeCorrectionRepository(db *sql.DB) *PVChargeCorrectionRepository {
	return NewPVChargeCorrectionRepositoryWithTimezone(db, "Asia/Tokyo")
}

func NewPVChargeCorrectionRepositoryWithTimezone(db *sql.DB, timezone string) *PVChargeCorrectionRepository {
	return &PVChargeCorrectionRepository{db: db, location: loadEcoFlowLoadLocation(timezone)}
}

// EnsureUpToDate は直近 lookback 日のうち未集計の日次実績を集計し、
// サンプルが十分に貯まっていれば補正係数を自動更新する。
func (r *PVChargeCorrectionRepository) EnsureUpToDate(ctx context.Context, now time.Time) error {
	nowLocal := now.In(r.location)
	var errs []error
	for offset := pvChargeCorrectionLookbackDays; offset >= 1; offset-- {
		date := nowLocal.AddDate(0, 0, -offset).Format("2006-01-02")
		exists, err := r.dailyLogExists(ctx, date)
		if err != nil {
			return fmt.Errorf("failed to check pv charge correction daily log for %s: %w", date, err)
		}
		if exists {
			continue
		}
		log, ok, err := r.buildDailyLog(ctx, date)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to build pv charge correction daily log for %s: %w", date, err))
			continue
		}
		if !ok {
			continue
		}
		if err := r.UpsertDailyLog(ctx, log); err != nil {
			errs = append(errs, fmt.Errorf("failed to save pv charge correction daily log for %s: %w", date, err))
		}
	}
	if err := r.refreshFactor(ctx, now); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// PVChargeCorrectionRecommendation は現在の日次実績から推奨係数を返す。
// 夜間充電計画の表示用で、取得失敗は呼び出し側で無視してよい。
func (r *PVChargeCorrectionRepository) PVChargeCorrectionRecommendation(ctx context.Context, now time.Time) (*domain.PVChargeCorrectionRecommendation, error) {
	settings, err := NewWeatherSettingsRepository(r.db).CurrentWeatherLocation(ctx)
	if err != nil {
		return nil, err
	}
	factors, err := r.recentOKCaptureFactors(ctx)
	if err != nil {
		return nil, err
	}
	recommendation := &domain.PVChargeCorrectionRecommendation{
		RecommendedFactor: settings.PVChargeCorrectionFactor,
		OKSampleDays:      len(factors),
		MinSampleDays:     settings.PVChargeCorrectionMinSampleDays,
		Applicable:        false,
		Status:            "insufficient-samples",
	}
	if len(factors) < settings.PVChargeCorrectionMinSampleDays {
		return recommendation, nil
	}
	recommendation.RecommendedFactor = clampFactor(medianFloat(factors), settings.PVChargeCorrectionMinFactor, settings.PVChargeCorrectionMaxFactor)
	recommendation.Applicable = true
	recommendation.Status = "ok"
	return recommendation, nil
}

func (r *PVChargeCorrectionRepository) UpsertDailyLog(ctx context.Context, log domain.PVChargeCorrectionDailyLog) error {
	now := time.Now().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(ctx, `INSERT INTO pv_charge_correction_daily_logs (
		date, forecast_pv_kwh, forecast_pv_to_battery_kwh, actual_battery_input_kwh,
		actual_export_kwh, actual_capture_factor, weather_code, cloud_cover_mean_percent,
		sample_quality, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(date) DO UPDATE SET
		forecast_pv_kwh = excluded.forecast_pv_kwh,
		forecast_pv_to_battery_kwh = excluded.forecast_pv_to_battery_kwh,
		actual_battery_input_kwh = excluded.actual_battery_input_kwh,
		actual_export_kwh = excluded.actual_export_kwh,
		actual_capture_factor = excluded.actual_capture_factor,
		weather_code = excluded.weather_code,
		cloud_cover_mean_percent = excluded.cloud_cover_mean_percent,
		sample_quality = excluded.sample_quality,
		updated_at = excluded.updated_at`,
		log.Date,
		log.ForecastPVKWh,
		log.ForecastPVToBatteryKWh,
		log.ActualBatteryInputKWh,
		log.ActualExportKWh,
		log.ActualCaptureFactor,
		log.WeatherCode,
		log.CloudCoverMeanPercent,
		log.SampleQuality,
		now,
		now,
	)
	return err
}

func (r *PVChargeCorrectionRepository) dailyLogExists(ctx context.Context, date string) (bool, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pv_charge_correction_daily_logs WHERE date = ?`, date).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// buildDailyLog は対象日のPV予測(夜間充電計画ログ)と実績(power_logs)から日次実績を作る。
// 対象日の計画ログが無い場合は ok=false を返す。
func (r *PVChargeCorrectionRepository) buildDailyLog(ctx context.Context, date string) (domain.PVChargeCorrectionDailyLog, bool, error) {
	var forecastPVKWh, forecastPVToBatteryKWh float64
	var pvStartText, pvEndText string
	err := r.db.QueryRowContext(ctx, `SELECT
		daily_estimated_pv_kwh, corrected_estimated_pv_to_battery_kwh, pv_effective_start_at, pv_effective_end_at
		FROM night_charge_plan_logs
		WHERE target_forecast_date = ? AND daily_estimated_pv_kwh > 0
		ORDER BY measured_at DESC, id DESC LIMIT 1`, date).Scan(&forecastPVKWh, &forecastPVToBatteryKWh, &pvStartText, &pvEndText)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PVChargeCorrectionDailyLog{}, false, nil
	}
	if err != nil {
		return domain.PVChargeCorrectionDailyLog{}, false, err
	}

	windowStart, windowEnd, err := r.pvWindowForDate(date, pvStartText, pvEndText)
	if err != nil {
		return domain.PVChargeCorrectionDailyLog{}, false, err
	}
	inputKWh, exportKWh, usableKWh, sampledHours, err := r.integratePowerLogs(ctx, windowStart, windowEnd)
	if err != nil {
		return domain.PVChargeCorrectionDailyLog{}, false, err
	}

	log := domain.PVChargeCorrectionDailyLog{
		Date:                   date,
		ForecastPVKWh:          forecastPVKWh,
		ForecastPVToBatteryKWh: forecastPVToBatteryKWh,
		ActualBatteryInputKWh:  inputKWh,
		ActualExportKWh:        exportKWh,
		SampleQuality:          "ok",
	}
	if forecastPVKWh > 0 {
		log.ActualCaptureFactor = usableKWh / forecastPVKWh
	}
	if sampledHours < pvChargeCorrectionMinSampledHours {
		log.SampleQuality = "insufficient-data"
	} else if forecastPVKWh < pvChargeCorrectionMinForecastKWh {
		log.SampleQuality = "low-forecast"
	}
	return log, true, nil
}

func (r *PVChargeCorrectionRepository) pvWindowForDate(date, startText, endText string) (time.Time, time.Time, error) {
	start, startErr := parsePVWindowTime(startText, r.location)
	end, endErr := parsePVWindowTime(endText, r.location)
	if startErr == nil && endErr == nil && end.After(start) {
		return start, end, nil
	}
	day, err := time.ParseInLocation("2006-01-02", date, r.location)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to parse pv correction date %q: %w", date, err)
	}
	return day.Add(time.Duration(defaultEcoFlowDayStartHour) * time.Hour), day.Add(time.Duration(defaultEcoFlowDayEndHour) * time.Hour), nil
}

func parsePVWindowTime(value string, location *time.Location) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("empty pv window time")
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported pv window time format %q", value)
}

// integratePowerLogs はPV有効時間帯の battery_input_w / export_w / import_w を時間積分する。
// EcoFlowのAC入力はパススルー負荷込みのため、系統からの持ち出し(import)を差し引いた
// max(0, input + export - import) を「実際にPVから利用できた電力」の近似として使う。
func (r *PVChargeCorrectionRepository) integratePowerLogs(ctx context.Context, start, end time.Time) (inputKWh, exportKWh, usableKWh, sampledHours float64, err error) {
	rows, err := r.db.QueryContext(ctx, `SELECT measured_at, battery_input_w, export_w, import_w
		FROM power_logs
		WHERE julianday(measured_at) >= julianday(?) AND julianday(measured_at) < julianday(?)
		ORDER BY measured_at ASC, id ASC`,
		start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))
	if err != nil {
		return 0, 0, 0, 0, err
	}
	defer rows.Close()

	type sample struct {
		measuredAt time.Time
		inputW     int
		exportW    int
		importW    int
	}
	samples := []sample{}
	for rows.Next() {
		var measuredAtText string
		var inputW, exportW, importW sql.NullInt64
		if err := rows.Scan(&measuredAtText, &inputW, &exportW, &importW); err != nil {
			return 0, 0, 0, 0, err
		}
		measuredAt, parseErr := parsePVWindowTime(measuredAtText, r.location)
		if parseErr != nil {
			continue
		}
		samples = append(samples, sample{
			measuredAt: measuredAt,
			inputW:     intFromNull(inputW),
			exportW:    intFromNull(exportW),
			importW:    intFromNull(importW),
		})
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, 0, err
	}
	for i := 0; i < len(samples)-1; i++ {
		duration := samples[i+1].measuredAt.Sub(samples[i].measuredAt)
		if duration <= 0 || duration > maxDaytimeSampleGap {
			continue
		}
		hours := duration.Hours()
		sampledHours += hours
		inputKWh += float64(samples[i].inputW) * hours / 1000
		exportKWh += float64(samples[i].exportW) * hours / 1000
		usableW := samples[i].inputW + samples[i].exportW - samples[i].importW
		if usableW > 0 {
			usableKWh += float64(usableW) * hours / 1000
		}
	}
	return inputKWh, exportKWh, usableKWh, sampledHours, nil
}

func (r *PVChargeCorrectionRepository) refreshFactor(ctx context.Context, now time.Time) error {
	settings, err := NewWeatherSettingsRepository(r.db).CurrentWeatherLocation(ctx)
	if err != nil {
		return fmt.Errorf("failed to load weather settings for pv charge correction: %w", err)
	}
	if settings.PVChargeCorrectionManual {
		return nil
	}
	factors, err := r.recentOKCaptureFactors(ctx)
	if err != nil {
		return fmt.Errorf("failed to load pv charge correction samples: %w", err)
	}
	if len(factors) < settings.PVChargeCorrectionMinSampleDays {
		return nil
	}
	recommended := clampFactor(medianFloat(factors), settings.PVChargeCorrectionMinFactor, settings.PVChargeCorrectionMaxFactor)
	if math.Abs(recommended-settings.PVChargeCorrectionFactor) < pvChargeCorrectionMinFactorDelta {
		return nil
	}
	// 手動設定を上書きしないよう pv_charge_correction_manual = 0 の場合だけ更新する
	_, err = r.db.ExecContext(ctx, `UPDATE settings SET
		pv_charge_correction_factor = ?,
		pv_charge_correction_updated_at = ?
		WHERE id = 1 AND IFNULL(pv_charge_correction_manual, 0) = 0`,
		recommended, now.Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("failed to update pv charge correction factor: %w", err)
	}
	return nil
}

func (r *PVChargeCorrectionRepository) recentOKCaptureFactors(ctx context.Context) ([]float64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT actual_capture_factor
		FROM pv_charge_correction_daily_logs
		WHERE sample_quality = 'ok' AND actual_capture_factor > 0
		ORDER BY date DESC LIMIT ?`, pvChargeCorrectionLookbackDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	factors := []float64{}
	for rows.Next() {
		var factor float64
		if err := rows.Scan(&factor); err != nil {
			return nil, err
		}
		factors = append(factors, factor)
	}
	return factors, rows.Err()
}

func clampFactor(value, minFactor, maxFactor float64) float64 {
	if minFactor > 0 && value < minFactor {
		return minFactor
	}
	if maxFactor > 0 && value > maxFactor {
		return maxFactor
	}
	return value
}

func medianFloat(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64{}, values...)
	sort.Float64s(sorted)
	middle := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[middle]
	}
	return (sorted[middle-1] + sorted[middle]) / 2
}
