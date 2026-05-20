package store

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const (
	nightSummaryPlanStartHour      = 21
	nightSummaryChargeStartHour    = 23
	nightSummaryChargeEndHour      = 7
	nightSummaryFollowUpEndHour    = 16
	nightSummaryToleranceSoc       = 3
	nightSummaryOverchargedSoc     = 10
	maxNightSummarySampleGap       = 2 * time.Hour
	nightSummaryBoundaryTolerance  = 30 * time.Minute
	nightSummaryPending            = "pending"
	nightSummaryOK                 = "ok"
	nightSummaryUndercharged       = "undercharged"
	nightSummaryOvercharged        = "overcharged"
	nightSummaryInsufficientData   = "insufficient-data"
	nightSummaryDataSourcePowerLog = "power-log"
)

type NightChargeSummaryRepository struct {
	db       *sql.DB
	location *time.Location
}

type NightChargeSummaryPageFilter struct {
	From *time.Time
	To   *time.Time
}

func NewNightChargeSummaryRepository(db *sql.DB) *NightChargeSummaryRepository {
	return NewNightChargeSummaryRepositoryWithTimezone(db, "Asia/Tokyo")
}

func NewNightChargeSummaryRepositoryWithTimezone(db *sql.DB, timezone string) *NightChargeSummaryRepository {
	return &NightChargeSummaryRepository{db: db, location: loadEcoFlowLoadLocation(timezone)}
}

func (r *NightChargeSummaryRepository) ListNightChargeDailySummariesPage(ctx context.Context, now time.Time, limit int, offset int, filter NightChargeSummaryPageFilter) ([]domain.NightChargeDailySummary, int, error) {
	limit = normalizeLimit(limit)
	offset = normalizeOffset(offset)
	dates, err := r.summaryDates(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	total := len(dates)
	if offset >= total {
		return []domain.NightChargeDailySummary{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	summaries := make([]domain.NightChargeDailySummary, 0, end-offset)
	for _, summaryDate := range dates[offset:end] {
		summary, err := r.buildSummary(ctx, now, summaryDate)
		if err != nil {
			return nil, 0, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, total, nil
}

func (r *NightChargeSummaryRepository) summaryDates(ctx context.Context, filter NightChargeSummaryPageFilter) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT measured_at FROM night_charge_plan_logs ORDER BY measured_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var measuredAt string
		if err := rows.Scan(&measuredAt); err != nil {
			return nil, err
		}
		parsed, err := parseTime(measuredAt)
		if err != nil {
			return nil, err
		}
		summaryDate, ok := r.summaryDateFor(parsed)
		if !ok {
			continue
		}
		session := r.sessionWindow(summaryDate)
		if filter.From != nil && session.followUpEnd.Before(*filter.From) {
			continue
		}
		if filter.To != nil && session.start.After(*filter.To) {
			continue
		}
		seen[summaryDate] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	dates := make([]string, 0, len(seen))
	for date := range seen {
		dates = append(dates, date)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	return dates, nil
}

func (r *NightChargeSummaryRepository) buildSummary(ctx context.Context, now time.Time, summaryDate string) (domain.NightChargeDailySummary, error) {
	session := r.sessionWindow(summaryDate)
	summary := domain.NightChargeDailySummary{
		SummaryDate:       summaryDate,
		MorningStatus:     nightSummaryInsufficientData,
		MorningReason:     "plan or 07:00 SOC data is missing",
		FinalResultStatus: nightSummaryInsufficientData,
		FinalResultReason: "daytime follow-up data is missing",
		DataSource:        nightSummaryDataSourcePowerLog,
	}

	plan, err := r.latestPlanInWindow(ctx, session.start, session.chargeStart)
	if err != nil {
		return summary, err
	}
	if plan != nil {
		summary.PlanCreatedAt = &plan.MeasuredAt
		summary.TargetForecastDate = plan.TargetForecastDate
		summary.PlannedTargetSoc = &plan.RecommendedNightTargetSoc
		summary.PlannedTargetKWh = &plan.RecommendedNightTargetKWh
		summary.PlannedRequiredChargeKWh = &plan.RequiredNightChargeKWh
		summary.PlannedMode = plan.RecommendedMode
	}

	nightStartSoc, err := r.closestSoc(ctx, session.chargeStart)
	if err != nil {
		return summary, err
	}
	summary.NightStartSoc = nightStartSoc
	if !now.Before(session.chargeEnd) {
		nightEndSoc, err := r.closestSoc(ctx, session.chargeEnd)
		if err != nil {
			return summary, err
		}
		summary.NightEndSoc = nightEndSoc
	}
	if summary.NightStartSoc != nil && summary.NightEndSoc != nil {
		delta := *summary.NightEndSoc - *summary.NightStartSoc
		summary.NightSocDelta = &delta
	}

	minSoc, maxSoc, err := r.minMaxSoc(ctx, session.chargeStart, session.chargeEnd)
	if err != nil {
		return summary, err
	}
	summary.MinNightSoc = minSoc
	summary.MaxNightSoc = maxSoc

	nightEnergy, err := r.powerEnergy(ctx, session.chargeStart, session.chargeEnd)
	if err != nil {
		return summary, err
	}
	daytimeEnergy, err := r.powerEnergy(ctx, session.chargeEnd, session.followUpEnd)
	if err != nil {
		return summary, err
	}
	summary.NightImportKWh = nightEnergy.importKWh
	summary.NightExportKWh = nightEnergy.exportKWh
	summary.NightBatteryInputKWh = nightEnergy.batteryInputKWh
	summary.NightBatteryOutputKWh = nightEnergy.batteryOutputKWh
	summary.DaytimeBatteryInputKWh = daytimeEnergy.batteryInputKWh
	summary.DaytimeExportKWh = daytimeEnergy.exportKWh

	if meterNight, ok, err := r.energyMeterDelta(ctx, session.chargeStart, session.chargeEnd); err != nil {
		return summary, err
	} else if ok {
		summary.NightImportKWh = &meterNight.importKWh
		summary.NightExportKWh = &meterNight.exportKWh
		summary.DataSource = "energy-meter+power-log"
	}
	if meterDaytime, ok, err := r.energyMeterDelta(ctx, session.chargeEnd, session.followUpEnd); err != nil {
		return summary, err
	} else if ok {
		summary.DaytimeExportKWh = &meterDaytime.exportKWh
		summary.DataSource = "energy-meter+power-log"
	}

	applyNightSummaryStatus(&summary, now, session)
	return summary, nil
}

func applyNightSummaryStatus(summary *domain.NightChargeDailySummary, now time.Time, session nightSummaryWindow) {
	if now.Before(session.chargeEnd) {
		summary.MorningStatus = nightSummaryPending
		summary.MorningReason = "07:00 has not arrived yet"
		summary.FinalResultStatus = nightSummaryPending
		summary.FinalResultReason = "daytime follow-up has not completed yet"
		return
	}
	if summary.PlannedTargetSoc == nil || summary.NightEndSoc == nil {
		summary.MorningStatus = nightSummaryInsufficientData
		summary.MorningReason = "planned target SOC or 07:00 SOC is missing"
	} else if *summary.NightEndSoc < *summary.PlannedTargetSoc-nightSummaryToleranceSoc {
		summary.MorningStatus = nightSummaryUndercharged
		summary.MorningReason = "07:00 SOC was below the planned target"
	} else {
		summary.MorningStatus = nightSummaryOK
		summary.MorningReason = "07:00 SOC stayed above the planned target"
	}

	if now.Before(session.followUpEnd) {
		summary.FinalResultStatus = nightSummaryPending
		summary.FinalResultReason = "16:00 daytime follow-up has not completed yet"
		return
	}
	if summary.MorningStatus == nightSummaryInsufficientData || (summary.DaytimeBatteryInputKWh == nil && summary.DaytimeExportKWh == nil) {
		summary.FinalResultStatus = nightSummaryInsufficientData
		summary.FinalResultReason = "morning result or daytime follow-up data is missing"
		return
	}
	if summary.MorningStatus == nightSummaryUndercharged {
		summary.FinalResultStatus = nightSummaryUndercharged
		summary.FinalResultReason = "07:00 SOC was below target before daytime follow-up"
		return
	}
	if summary.PlannedTargetSoc != nil && summary.NightEndSoc != nil && *summary.NightEndSoc > *summary.PlannedTargetSoc+nightSummaryOverchargedSoc && summary.DaytimeExportKWh != nil && *summary.DaytimeExportKWh > 0 {
		summary.FinalResultStatus = nightSummaryOvercharged
		summary.FinalResultReason = "night SOC exceeded target and daytime export remained"
		return
	}
	summary.FinalResultStatus = nightSummaryOK
	summary.FinalResultReason = "SOC stayed above target and daytime follow-up data was available"
}

func (r *NightChargeSummaryRepository) latestPlanInWindow(ctx context.Context, start time.Time, end time.Time) (*domain.NightChargePlanLog, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, measured_at, strategy_state, recommended_mode, recommended_night_target_soc,
		recommended_night_target_kwh, current_battery_energy_kwh, required_night_charge_kwh,
		daily_estimated_pv_kwh, pv_effective_start_at, pv_effective_end_at, pv_effective_window_source,
		morning_to_pv_start_load_kwh, forecast_daytime_deficit_kwh,
		battery_soc, battery_input_w, battery_output_w, grid_w, import_w, export_w,
		should_charge_tonight, would_write, command_fingerprint, command_sent, command_error, command_block_reason, action_summary, reason,
		target_forecast_date, created_at
		FROM night_charge_plan_logs
		WHERE julianday(measured_at) >= julianday(?)
			AND julianday(measured_at) < julianday(?)
			AND strategy_state = 'NIGHT_PLAN_READY'
		ORDER BY measured_at DESC, id DESC
		LIMIT 1`, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs, err := scanNightChargePlanLogs(rows, 1)
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return nil, nil
	}
	return &logs[0], nil
}

func (r *NightChargeSummaryRepository) closestSoc(ctx context.Context, target time.Time) (*int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT battery_soc
		FROM power_logs
		WHERE battery_soc IS NOT NULL
			AND julianday(measured_at) >= julianday(?)
			AND julianday(measured_at) <= julianday(?)
		ORDER BY ABS(julianday(measured_at) - julianday(?)) ASC, id DESC
		LIMIT 1`,
		target.Add(-nightSummaryBoundaryTolerance).Format(time.RFC3339Nano),
		target.Add(nightSummaryBoundaryTolerance).Format(time.RFC3339Nano),
		target.Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var soc sql.NullInt64
	if err := rows.Scan(&soc); err != nil {
		return nil, err
	}
	return intPtrFromNull(soc), rows.Err()
}

func (r *NightChargeSummaryRepository) minMaxSoc(ctx context.Context, start time.Time, end time.Time) (*int, *int, error) {
	var minSoc, maxSoc sql.NullInt64
	if err := r.db.QueryRowContext(ctx, `SELECT MIN(battery_soc), MAX(battery_soc)
		FROM power_logs
		WHERE battery_soc IS NOT NULL
			AND julianday(measured_at) >= julianday(?)
			AND julianday(measured_at) <= julianday(?)`,
		start.Format(time.RFC3339Nano),
		end.Format(time.RFC3339Nano),
	).Scan(&minSoc, &maxSoc); err != nil {
		return nil, nil, err
	}
	return intPtrFromNull(minSoc), intPtrFromNull(maxSoc), nil
}

func (r *NightChargeSummaryRepository) powerEnergy(ctx context.Context, start time.Time, end time.Time) (nightSummaryPowerEnergy, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		measured_at, import_w, export_w, battery_input_w, battery_output_w
		FROM power_logs
		WHERE julianday(measured_at) >= julianday(?)
			AND julianday(measured_at) <= julianday(?)
		ORDER BY measured_at ASC, id ASC`, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano))
	if err != nil {
		return nightSummaryPowerEnergy{}, err
	}
	defer rows.Close()
	samples := make([]nightSummaryPowerSample, 0)
	for rows.Next() {
		var measuredAt string
		var sample nightSummaryPowerSample
		var batteryInputW, batteryOutputW sql.NullInt64
		if err := rows.Scan(&measuredAt, &sample.importW, &sample.exportW, &batteryInputW, &batteryOutputW); err != nil {
			return nightSummaryPowerEnergy{}, err
		}
		parsed, err := parseTime(measuredAt)
		if err != nil {
			return nightSummaryPowerEnergy{}, err
		}
		sample.measuredAt = parsed
		sample.batteryInputW = intFromNull(batteryInputW)
		sample.batteryOutputW = intFromNull(batteryOutputW)
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nightSummaryPowerEnergy{}, err
	}
	return integrateNightSummaryPower(samples), nil
}

func integrateNightSummaryPower(samples []nightSummaryPowerSample) nightSummaryPowerEnergy {
	var energy nightSummaryPowerEnergy
	for i := 0; i < len(samples)-1; i++ {
		current := samples[i]
		next := samples[i+1]
		duration := next.measuredAt.Sub(current.measuredAt)
		if duration <= 0 || duration > maxNightSummarySampleGap {
			continue
		}
		hours := duration.Hours()
		energy.addImport(wattsToKWh(current.importW, hours))
		energy.addExport(wattsToKWh(current.exportW, hours))
		energy.addBatteryInput(wattsToKWh(current.batteryInputW, hours))
		energy.addBatteryOutput(wattsToKWh(current.batteryOutputW, hours))
	}
	return energy
}

func (r *NightChargeSummaryRepository) energyMeterDelta(ctx context.Context, start time.Time, end time.Time) (nightSummaryMeterEnergy, bool, error) {
	var importKWh, exportKWh sql.NullFloat64
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT
		SUM(import_delta_kwh), SUM(export_delta_kwh), COUNT(*)
		FROM energy_meter_logs
		WHERE julianday(measured_at) > julianday(?)
			AND julianday(measured_at) <= julianday(?)
			AND (import_delta_kwh IS NOT NULL OR export_delta_kwh IS NOT NULL)`,
		start.Format(time.RFC3339Nano),
		end.Format(time.RFC3339Nano),
	).Scan(&importKWh, &exportKWh, &count); err != nil {
		return nightSummaryMeterEnergy{}, false, err
	}
	if count == 0 {
		return nightSummaryMeterEnergy{}, false, nil
	}
	return nightSummaryMeterEnergy{
		importKWh: float64FromNull(importKWh),
		exportKWh: float64FromNull(exportKWh),
	}, true, nil
}

func (r *NightChargeSummaryRepository) summaryDateFor(measuredAt time.Time) (string, bool) {
	local := measuredAt.In(r.location)
	hour := local.Hour()
	if hour < nightSummaryChargeEndHour {
		return local.AddDate(0, 0, -1).Format("2006-01-02"), true
	}
	if hour >= nightSummaryPlanStartHour {
		return local.Format("2006-01-02"), true
	}
	return "", false
}

func (r *NightChargeSummaryRepository) sessionWindow(summaryDate string) nightSummaryWindow {
	base, err := time.ParseInLocation("2006-01-02", summaryDate, r.location)
	if err != nil {
		base = time.Now().In(r.location)
	}
	return nightSummaryWindow{
		start:       time.Date(base.Year(), base.Month(), base.Day(), nightSummaryPlanStartHour, 0, 0, 0, r.location),
		chargeStart: time.Date(base.Year(), base.Month(), base.Day(), nightSummaryChargeStartHour, 0, 0, 0, r.location),
		chargeEnd:   time.Date(base.Year(), base.Month(), base.Day()+1, nightSummaryChargeEndHour, 0, 0, 0, r.location),
		followUpEnd: time.Date(base.Year(), base.Month(), base.Day()+1, nightSummaryFollowUpEndHour, 0, 0, 0, r.location),
	}
}

type nightSummaryWindow struct {
	start       time.Time
	chargeStart time.Time
	chargeEnd   time.Time
	followUpEnd time.Time
}

type nightSummaryPowerSample struct {
	measuredAt     time.Time
	importW        int
	exportW        int
	batteryInputW  int
	batteryOutputW int
}

type nightSummaryPowerEnergy struct {
	importKWh        *float64
	exportKWh        *float64
	batteryInputKWh  *float64
	batteryOutputKWh *float64
}

func (e *nightSummaryPowerEnergy) addImport(value float64) {
	e.importKWh = addFloatPtr(e.importKWh, value)
}

func (e *nightSummaryPowerEnergy) addExport(value float64) {
	e.exportKWh = addFloatPtr(e.exportKWh, value)
}

func (e *nightSummaryPowerEnergy) addBatteryInput(value float64) {
	e.batteryInputKWh = addFloatPtr(e.batteryInputKWh, value)
}

func (e *nightSummaryPowerEnergy) addBatteryOutput(value float64) {
	e.batteryOutputKWh = addFloatPtr(e.batteryOutputKWh, value)
}

type nightSummaryMeterEnergy struct {
	importKWh float64
	exportKWh float64
}

func addFloatPtr(current *float64, value float64) *float64 {
	if current == nil {
		result := value
		return &result
	}
	*current += value
	return current
}

func float64FromNull(value sql.NullFloat64) float64 {
	if !value.Valid {
		return 0
	}
	return value.Float64
}
