package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type TariffRepository struct {
	db *sql.DB
}

const batteryComparisonMaxSampleInterval = 120 * time.Second

func NewTariffRepository(db *sql.DB) *TariffRepository {
	return &TariffRepository{db: db}
}

func (r *TariffRepository) CurrentTariffSettings(ctx context.Context) (domain.TariffSettings, error) {
	plan, err := r.tariffPlanAt(ctx, time.Now())
	if err != nil {
		return domain.TariffSettings{}, err
	}
	return domain.TariffSettings{
		PlanName:      plan.PlanName,
		DayRateYen:    plan.DayRateYen,
		HomeRateYen:   plan.HomeRateYen,
		NightRateYen:  plan.NightRateYen,
		ExportRateYen: plan.ExportRateYen,
		Timezone:      plan.Timezone,
		EffectiveFrom: plan.EffectiveFrom,
		EffectiveTo:   plan.EffectiveTo,
		UpdatedAt:     plan.UpdatedAt,
	}, nil
}

func (r *TariffRepository) ListTariffPlans(ctx context.Context) ([]domain.TariffPlan, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, plan_name, day_rate_yen, home_rate_yen, night_rate_yen, export_rate_yen, timezone,
		effective_from, effective_to, created_at, updated_at
		FROM tariff_plans
		ORDER BY effective_from DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	plans, err := scanTariffPlans(rows)
	if err != nil {
		return nil, err
	}
	if err := r.attachEffectiveTariffRules(ctx, plans); err != nil {
		return nil, err
	}
	return plans, nil
}

func (r *TariffRepository) UpsertTariffPlan(ctx context.Context, plan domain.TariffPlan) (domain.TariffPlan, error) {
	now := time.Now().Format(time.RFC3339Nano)
	effectiveFrom := plan.EffectiveFrom.UTC().Format(time.RFC3339Nano)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.TariffPlan{}, err
	}
	defer tx.Rollback()

	var nextEffectiveTo sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT MIN(effective_from)
		FROM tariff_plans
		WHERE effective_from > ?`, effectiveFrom).Scan(&nextEffectiveTo); err != nil {
		return domain.TariffPlan{}, err
	}

	if _, err := tx.ExecContext(ctx, `UPDATE tariff_plans
		SET effective_to = ?, updated_at = ?
		WHERE effective_from < ?
			AND (effective_to IS NULL OR effective_to > ?)`,
		effectiveFrom,
		now,
		effectiveFrom,
		effectiveFrom,
	); err != nil {
		return domain.TariffPlan{}, err
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO tariff_plans (
		plan_name, day_rate_yen, home_rate_yen, night_rate_yen, export_rate_yen, timezone,
		effective_from, effective_to, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(effective_from) DO UPDATE SET
		plan_name = excluded.plan_name,
		day_rate_yen = excluded.day_rate_yen,
		home_rate_yen = excluded.home_rate_yen,
		night_rate_yen = excluded.night_rate_yen,
		export_rate_yen = excluded.export_rate_yen,
		timezone = excluded.timezone,
		effective_to = excluded.effective_to,
		updated_at = excluded.updated_at`,
		plan.PlanName,
		plan.DayRateYen,
		plan.HomeRateYen,
		plan.NightRateYen,
		plan.ExportRateYen,
		normalizeTimezone(plan.Timezone),
		effectiveFrom,
		nullStringValue(nextEffectiveTo),
		now,
		now,
	)
	if err != nil {
		return domain.TariffPlan{}, err
	}
	var savedID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM tariff_plans WHERE effective_from = ?`, effectiveFrom).Scan(&savedID); err != nil {
		return domain.TariffPlan{}, err
	}
	if plan.PeriodRules != nil {
		if err := saveTariffPeriodRules(ctx, tx, savedID, plan.PeriodRules, now); err != nil {
			return domain.TariffPlan{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.TariffPlan{}, err
	}
	saved, err := r.tariffPlanAt(ctx, plan.EffectiveFrom)
	if err != nil {
		return domain.TariffPlan{}, err
	}
	plans := []domain.TariffPlan{saved}
	if err := r.attachEffectiveTariffRules(ctx, plans); err != nil {
		return domain.TariffPlan{}, err
	}
	return plans[0], nil
}

func (r *TariffRepository) DeleteTariffPlan(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tariff_plans`).Scan(&count); err != nil {
		return err
	}
	if count <= 1 {
		return ErrCannotDeleteLastTariffPlan
	}

	result, err := tx.ExecContext(ctx, `DELETE FROM tariff_plans WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	if err := recalculateTariffPlanEffectiveTo(ctx, tx); err != nil {
		return err
	}
	if ok, err := tariffPlansCoverTime(ctx, tx, time.Now()); err != nil {
		return err
	} else if !ok {
		return ErrTariffPlanCoverageRequired
	}
	return tx.Commit()
}

func (r *TariffRepository) EnergyCostSummary(ctx context.Context, from *time.Time, to *time.Time) (domain.TariffSummary, error) {
	plans, err := r.listTariffPlansAscending(ctx)
	if err != nil {
		return domain.TariffSummary{}, err
	}
	current, err := r.CurrentTariffSettings(ctx)
	if err != nil {
		return domain.TariffSummary{}, err
	}
	rulesByPlanID := make(map[int64][]domain.TariffPeriodRule, len(plans))
	for _, plan := range plans {
		rules, _, err := r.effectiveTariffRulesForPlan(ctx, plan)
		if err != nil {
			return domain.TariffSummary{}, err
		}
		rulesByPlanID[plan.ID] = rules
	}
	rows, err := r.queryEnergyMeterDeltas(ctx, from, to)
	if err != nil {
		return domain.TariffSummary{}, err
	}
	defer rows.Close()

	periods := map[string]*domain.TariffPeriodSummary{}
	summary := domain.TariffSummary{
		PlanName: current.PlanName,
		Timezone: current.Timezone,
		From:     from,
		To:       to,
		Note:     "燃料費調整額、再エネ賦課金、基本料金、割引は含まない概算です。祝日は土日と一部指定日のみを休日扱いします。電力量差分は測定終了時刻の時間帯・料金プランにまとめて計上します。",
	}
	for rows.Next() {
		var measuredAt string
		var importDelta, exportDelta sql.NullFloat64
		if err := rows.Scan(&measuredAt, &importDelta, &exportDelta); err != nil {
			return domain.TariffSummary{}, err
		}
		if !importDelta.Valid && !exportDelta.Valid {
			continue
		}
		parsedMeasuredAt, err := parseTime(measuredAt)
		if err != nil {
			return domain.TariffSummary{}, err
		}
		plan := effectiveTariffPlan(plans, parsedMeasuredAt)
		if plan == nil {
			continue
		}
		location, err := time.LoadLocation(plan.Timezone)
		if err != nil {
			return domain.TariffSummary{}, err
		}
		rules := rulesByPlanID[plan.ID]
		rule := resolveTariffRule(rules, parsedMeasuredAt.In(location))
		period := rule.Period
		rate := rule.RateYen
		if period == "" {
			period = tariffPeriod(parsedMeasuredAt.In(location))
			rate = tariffRate(*plan, period)
		}
		key := fmt.Sprintf("%s:%s:%.6f", plan.EffectiveFrom.Format(time.RFC3339Nano), period, rate)
		periodSummary := periods[key]
		if periodSummary == nil {
			periodSummary = &domain.TariffPeriodSummary{
				PlanName:      plan.PlanName,
				Period:        period,
				RateYen:       rate,
				ExportRateYen: plan.ExportRateYen,
				EffectiveFrom: plan.EffectiveFrom,
				EffectiveTo:   plan.EffectiveTo,
			}
			periods[key] = periodSummary
		}
		if importDelta.Valid {
			periodSummary.ImportKWh += importDelta.Float64
			summary.TotalImportKWh += importDelta.Float64
		}
		if exportDelta.Valid {
			periodSummary.ExportKWh += exportDelta.Float64
			periodSummary.ExportIncomeYen += exportDelta.Float64 * plan.ExportRateYen
			summary.TotalExportKWh += exportDelta.Float64
		}
		summary.SampleCount++
	}
	if err := rows.Err(); err != nil {
		return domain.TariffSummary{}, err
	}
	keys := make([]string, 0, len(periods))
	for key := range periods {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		period := periods[key]
		period.ImportKWh = round4(period.ImportKWh)
		period.ExportKWh = round4(period.ExportKWh)
		period.ImportCostYen = round2(period.ImportKWh * period.RateYen)
		period.ExportIncomeYen = round2(period.ExportIncomeYen)
		summary.TotalImportCostYen += period.ImportCostYen
		summary.TotalExportIncomeYen += period.ExportIncomeYen
		summary.Periods = append(summary.Periods, *period)
	}
	summary.TotalImportKWh = round4(summary.TotalImportKWh)
	summary.TotalExportKWh = round4(summary.TotalExportKWh)
	summary.TotalImportCostYen = round2(summary.TotalImportCostYen)
	summary.TotalExportIncomeYen = round2(summary.TotalExportIncomeYen)
	summary.NetCostYen = round2(summary.TotalImportCostYen - summary.TotalExportIncomeYen)
	comparison, err := r.batteryCostComparison(ctx, plans, rulesByPlanID, from, to)
	if err != nil {
		return domain.TariffSummary{}, err
	}
	summary.BatteryComparison = &comparison
	return summary, nil
}

func (r *TariffRepository) batteryCostComparison(ctx context.Context, plans []domain.TariffPlan, rulesByPlanID map[int64][]domain.TariffPeriodRule, from *time.Time, to *time.Time) (domain.BatteryCostComparison, error) {
	comparison := domain.BatteryCostComparison{
		Available:                false,
		Method:                   "power_logs_no_battery_estimate",
		Quality:                  "approximate",
		MaxSampleIntervalSeconds: int(batteryComparisonMaxSampleInterval.Seconds()),
		Note:                     "power_logs の DELTA Pro 3 入出力から、grid_w - battery_input_w + battery_output_w でポータブルバッテリー無しを近似します。機器別入出力ログが揃うまでは補助バッテリー分は限定的な推定です。在庫調整は充放電と pass-through の測定検証が完了するまで optimizer 判定に使用できません。",
	}
	inventoryCapacityWh, err := r.batteryInventoryCapacityWh(ctx)
	if err != nil {
		return comparison, err
	}
	rows, err := r.queryPowerLogBatterySamples(ctx, from, to)
	if err != nil {
		return comparison, err
	}
	defer rows.Close()

	dailyBreakdown := map[string]*domain.BatteryCostComparisonDailyBreakdown{}
	rateBounds := map[string]tariffRateBound{}
	var previous *batteryCostSample
	for rows.Next() {
		current, err := scanBatteryCostSample(rows)
		if err != nil {
			return comparison, err
		}
		if previous != nil {
			if err := addBatteryCostInterval(&comparison, dailyBreakdown, rateBounds, plans, rulesByPlanID, *previous, current, inventoryCapacityWh); err != nil {
				return comparison, err
			}
		}
		previous = &current
	}
	if err := rows.Err(); err != nil {
		return comparison, err
	}

	if comparison.SampleCount > 0 {
		comparison.Available = true
	}
	comparison.ActualImportKWh = round4(comparison.ActualImportKWh)
	comparison.ActualExportKWh = round4(comparison.ActualExportKWh)
	comparison.ActualImportCostYen = round2(comparison.ActualImportCostYen)
	comparison.ActualExportIncomeYen = round2(comparison.ActualExportIncomeYen)
	comparison.ActualNetCostYen = round2(comparison.ActualImportCostYen - comparison.ActualExportIncomeYen)
	comparison.EstimatedNoBatteryImportKWh = round4(comparison.EstimatedNoBatteryImportKWh)
	comparison.EstimatedNoBatteryExportKWh = round4(comparison.EstimatedNoBatteryExportKWh)
	comparison.EstimatedNoBatteryImportCostYen = round2(comparison.EstimatedNoBatteryImportCostYen)
	comparison.EstimatedNoBatteryExportIncomeYen = round2(comparison.EstimatedNoBatteryExportIncomeYen)
	comparison.EstimatedNoBatteryNetCostYen = round2(comparison.EstimatedNoBatteryImportCostYen - comparison.EstimatedNoBatteryExportIncomeYen)
	comparison.EstimatedSavingsYen = round2(comparison.EstimatedNoBatteryNetCostYen - comparison.ActualNetCostYen)
	comparison.BatteryInputKWh = round4(comparison.BatteryInputKWh)
	comparison.BatteryOutputKWh = round4(comparison.BatteryOutputKWh)
	comparison.DailyBreakdown = finalizeBatteryCostDailyBreakdown(dailyBreakdown, inventoryCapacityWh)
	finalizeBatteryCostComparisonInventory(&comparison, inventoryCapacityWh)
	return comparison, nil
}

type batteryCostSample struct {
	measuredAt     time.Time
	gridW          int
	batterySOC     sql.NullInt64
	batteryInputW  int
	batteryOutputW int
}

type tariffRateBound struct {
	lowest  float64
	highest float64
}

func (r *TariffRepository) queryPowerLogBatterySamples(ctx context.Context, from *time.Time, to *time.Time) (*sql.Rows, error) {
	filter := EnergyMeterLogPageFilter{From: from, To: to}
	whereClause, args := energyMeterLogWhere(filter)
	return r.db.QueryContext(ctx, `SELECT measured_at, grid_w,
		battery_soc, COALESCE(battery_input_w, 0), COALESCE(battery_output_w, 0)
		FROM power_logs
		`+whereClause+`
		ORDER BY measured_at ASC, id ASC`, args...)
}

func scanBatteryCostSample(rows *sql.Rows) (batteryCostSample, error) {
	var sample batteryCostSample
	var measuredAt string
	if err := rows.Scan(&measuredAt, &sample.gridW, &sample.batterySOC, &sample.batteryInputW, &sample.batteryOutputW); err != nil {
		return batteryCostSample{}, err
	}
	parsedMeasuredAt, err := parseTime(measuredAt)
	if err != nil {
		return batteryCostSample{}, err
	}
	sample.measuredAt = parsedMeasuredAt
	return sample, nil
}

func (r *TariffRepository) batteryInventoryCapacityWh(ctx context.Context) (int, error) {
	var capacityWh int
	err := r.db.QueryRowContext(ctx, `SELECT capacity_wh
		FROM charging_devices
		WHERE enabled = 1 AND capacity_wh > 0
		ORDER BY
			CASE
				WHEN kind = 'ecoflow_delta_pro3' OR device_type = 'DELTA_PRO3' THEN 0
				ELSE 1
			END,
			capacity_wh DESC
		LIMIT 1`).Scan(&capacityWh)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || strings.Contains(err.Error(), "no such table: charging_devices") {
			return 0, nil
		}
		return 0, err
	}
	return capacityWh, nil
}

func addBatteryCostInterval(comparison *domain.BatteryCostComparison, dailyBreakdown map[string]*domain.BatteryCostComparisonDailyBreakdown, rateBounds map[string]tariffRateBound, plans []domain.TariffPlan, rulesByPlanID map[int64][]domain.TariffPeriodRule, sample batteryCostSample, next batteryCostSample, inventoryCapacityWh int) error {
	duration := next.measuredAt.Sub(sample.measuredAt)
	if duration <= 0 {
		comparison.SkippedSampleCount++
		return nil
	}
	if duration > batteryComparisonMaxSampleInterval {
		comparison.SkippedSampleCount++
		return nil
	}
	plan, rule, err := tariffPlanAndRuleAt(plans, rulesByPlanID, sample.measuredAt)
	if err != nil {
		return err
	}
	if plan == nil {
		comparison.SkippedSampleCount++
		return nil
	}
	location, err := time.LoadLocation(plan.Timezone)
	if err != nil {
		return err
	}
	localMeasuredAt := sample.measuredAt.In(location)
	rules := rulesByPlanID[plan.ID]
	dayType := tariffDayType(localMeasuredAt)
	lowestRate, highestRate := cachedTariffRateBoundsForDayType(rateBounds, plan.ID, rules, dayType)

	hours := duration.Hours()
	actualKWh := float64(sample.gridW) * hours / 1000
	noBatteryW := sample.gridW - sample.batteryInputW + sample.batteryOutputW
	noBatteryKWh := float64(noBatteryW) * hours / 1000
	batteryInputKWh := float64(sample.batteryInputW) * hours / 1000
	batteryOutputKWh := float64(sample.batteryOutputW) * hours / 1000
	netChargeKWh := positiveFloat(batteryInputKWh - batteryOutputKWh)
	netDischargeKWh := positiveFloat(batteryOutputKWh - batteryInputKWh)

	addTariffEnergy(
		&comparison.ActualImportKWh,
		&comparison.ActualExportKWh,
		&comparison.ActualImportCostYen,
		&comparison.ActualExportIncomeYen,
		actualKWh,
		rule.RateYen,
		plan.ExportRateYen,
	)
	addTariffEnergy(
		&comparison.EstimatedNoBatteryImportKWh,
		&comparison.EstimatedNoBatteryExportKWh,
		&comparison.EstimatedNoBatteryImportCostYen,
		&comparison.EstimatedNoBatteryExportIncomeYen,
		noBatteryKWh,
		rule.RateYen,
		plan.ExportRateYen,
	)
	if batteryInputKWh > 0 {
		comparison.BatteryInputKWh += batteryInputKWh
	}
	if batteryOutputKWh > 0 {
		comparison.BatteryOutputKWh += batteryOutputKWh
	}
	comparison.SampleCount++
	updateBatteryCostComparisonInventory(comparison, sample, next, inventoryCapacityWh)
	addBatteryCostDailyInterval(
		dailyBreakdown,
		localMeasuredAt.Format("2006-01-02"),
		localMeasuredAt,
		next.measuredAt.In(location),
		actualKWh,
		noBatteryKWh,
		batteryInputKWh,
		batteryOutputKWh,
		netChargeKWh,
		netDischargeKWh,
		rule.RateYen,
		plan.ExportRateYen,
		lowestRate,
		highestRate,
		sample.batterySOC,
		next.batterySOC,
		inventoryCapacityWh,
	)
	return nil
}

func cachedTariffRateBoundsForDayType(cache map[string]tariffRateBound, planID int64, rules []domain.TariffPeriodRule, dayType string) (float64, float64) {
	if cache == nil {
		return tariffRateBoundsForDayType(rules, dayType)
	}
	key := fmt.Sprintf("%d:%s", planID, dayType)
	if bounds, ok := cache[key]; ok {
		return bounds.lowest, bounds.highest
	}
	lowest, highest := tariffRateBoundsForDayType(rules, dayType)
	cache[key] = tariffRateBound{lowest: lowest, highest: highest}
	return lowest, highest
}

func addBatteryCostDailyInterval(dailyBreakdown map[string]*domain.BatteryCostComparisonDailyBreakdown, date string, localMeasuredAt time.Time, nextLocalMeasuredAt time.Time, actualKWh float64, noBatteryKWh float64, batteryInputKWh float64, batteryOutputKWh float64, netChargeKWh float64, netDischargeKWh float64, rateYen float64, exportRateYen float64, lowestRateYen float64, highestRateYen float64, sampleSOC sql.NullInt64, nextSOC sql.NullInt64, inventoryCapacityWh int) {
	if dailyBreakdown == nil || date == "" {
		return
	}
	day := dailyBreakdown[date]
	if day == nil {
		day = &domain.BatteryCostComparisonDailyBreakdown{Date: date}
		dailyBreakdown[date] = day
	}
	day.SampleCount++
	updateBatteryCostDailyInventory(day, localMeasuredAt, nextLocalMeasuredAt, sampleSOC, nextSOC, highestRateYen, inventoryCapacityWh)
	day.ActualNetCostYen += signedEnergyNetCost(actualKWh, rateYen, exportRateYen)
	day.EstimatedNoBatteryNetCostYen += signedEnergyNetCost(noBatteryKWh, rateYen, exportRateYen)
	if batteryInputKWh > 0 {
		day.BatteryInputKWh += batteryInputKWh
	}
	if batteryOutputKWh > 0 {
		day.BatteryOutputKWh += batteryOutputKWh
	}
	if netChargeKWh > 0 && isLowestTariffRate(rateYen, lowestRateYen, highestRateYen) {
		day.LowPriceChargeKWh += netChargeKWh
	}
	if netDischargeKWh > 0 && hasTariffRateSpread(lowestRateYen, highestRateYen) {
		if isHighestTariffRate(rateYen, lowestRateYen, highestRateYen) {
			day.HighPriceDischargeKWh += netDischargeKWh
		} else if !isLowestTariffRate(rateYen, lowestRateYen, highestRateYen) {
			day.MidPriceDischargeKWh += netDischargeKWh
		}
	}
	if netChargeKWh > 0 && noBatteryKWh < 0 {
		day.ExportAbsorptionKWh += math.Min(netChargeKWh, -noBatteryKWh)
	}
}

func updateBatteryCostComparisonInventory(comparison *domain.BatteryCostComparison, sample batteryCostSample, next batteryCostSample, inventoryCapacityWh int) {
	if comparison == nil || inventoryCapacityWh <= 0 {
		return
	}
	if validBatterySOC(sample.batterySOC) {
		soc := int(sample.batterySOC.Int64)
		if comparison.InventoryStartSoc == nil {
			comparison.InventoryStartSoc = intPtr(soc)
		}
		comparison.InventoryEndSoc = intPtr(soc)
	}
	if validBatterySOC(next.batterySOC) {
		comparison.InventoryEndSoc = intPtr(int(next.batterySOC.Int64))
	}
}

func updateBatteryCostDailyInventory(day *domain.BatteryCostComparisonDailyBreakdown, localMeasuredAt time.Time, nextLocalMeasuredAt time.Time, sampleSOC sql.NullInt64, nextSOC sql.NullInt64, highestRateYen float64, inventoryCapacityWh int) {
	if day == nil || inventoryCapacityWh <= 0 {
		return
	}
	if highestRateYen > 0 && (day.InventoryValueRateYen == nil || highestRateYen > *day.InventoryValueRateYen) {
		day.InventoryValueRateYen = floatPtr(highestRateYen)
	}
	if validBatterySOC(sampleSOC) {
		soc := int(sampleSOC.Int64)
		if day.InventoryStartSoc == nil {
			day.InventoryStartSoc = intPtr(soc)
		}
		day.InventoryEndSoc = intPtr(soc)
	}
	if validBatterySOC(nextSOC) && localMeasuredAt.Format("2006-01-02") == nextLocalMeasuredAt.Format("2006-01-02") {
		day.InventoryEndSoc = intPtr(int(nextSOC.Int64))
	}
}

func finalizeBatteryCostDailyBreakdown(dailyBreakdown map[string]*domain.BatteryCostComparisonDailyBreakdown, inventoryCapacityWh int) []domain.BatteryCostComparisonDailyBreakdown {
	if len(dailyBreakdown) == 0 {
		return nil
	}
	dates := make([]string, 0, len(dailyBreakdown))
	for date := range dailyBreakdown {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	results := make([]domain.BatteryCostComparisonDailyBreakdown, 0, len(dates))
	for _, date := range dates {
		day := *dailyBreakdown[date]
		day.ActualNetCostYen = round2(day.ActualNetCostYen)
		day.EstimatedNoBatteryNetCostYen = round2(day.EstimatedNoBatteryNetCostYen)
		day.EstimatedSavingsYen = round2(day.EstimatedNoBatteryNetCostYen - day.ActualNetCostYen)
		finalizeDailyInventory(&day, inventoryCapacityWh)
		day.LowPriceChargeKWh = round4(day.LowPriceChargeKWh)
		day.MidPriceDischargeKWh = round4(day.MidPriceDischargeKWh)
		day.HighPriceDischargeKWh = round4(day.HighPriceDischargeKWh)
		day.ExportAbsorptionKWh = round4(day.ExportAbsorptionKWh)
		day.BatteryInputKWh = round4(day.BatteryInputKWh)
		day.BatteryOutputKWh = round4(day.BatteryOutputKWh)
		day.EstimatedLossKWh = round4(positiveFloat(day.BatteryInputKWh - day.BatteryOutputKWh))
		results = append(results, day)
	}
	return results
}

func finalizeDailyInventory(day *domain.BatteryCostComparisonDailyBreakdown, inventoryCapacityWh int) {
	if day == nil {
		return
	}
	day.InventoryDeltaKWh = nil
	day.InventoryValueYen = nil
	day.InventoryValueRateYen = nil
	day.AdjustedEstimatedSavingsYen = nil
	if inventoryCapacityWh <= 0 || day.InventoryStartSoc == nil || day.InventoryEndSoc == nil {
		return
	}
	deltaKWh := batteryInventoryDeltaKWh(inventoryCapacityWh, *day.InventoryStartSoc, *day.InventoryEndSoc)
	day.InventoryDeltaKWh = floatPtr(round4(deltaKWh))
}

func finalizeBatteryCostComparisonInventory(comparison *domain.BatteryCostComparison, inventoryCapacityWh int) {
	if comparison == nil {
		return
	}
	comparison.InventoryDeltaKWh = nil
	comparison.InventoryValueYen = nil
	comparison.InventoryValueRateYen = nil
	comparison.AdjustedEstimatedSavingsYen = nil
	for i := range comparison.DailyBreakdown {
		comparison.DailyBreakdown[i].InventoryValueYen = nil
		comparison.DailyBreakdown[i].InventoryValueRateYen = nil
		comparison.DailyBreakdown[i].AdjustedEstimatedSavingsYen = nil
	}
	if inventoryCapacityWh <= 0 || comparison.InventoryStartSoc == nil || comparison.InventoryEndSoc == nil {
		return
	}
	deltaKWh := batteryInventoryDeltaKWh(inventoryCapacityWh, *comparison.InventoryStartSoc, *comparison.InventoryEndSoc)
	comparison.InventoryDeltaKWh = floatPtr(round4(deltaKWh))
}

func batteryInventoryDeltaKWh(capacityWh int, startSoc int, endSoc int) float64 {
	return float64(capacityWh) * float64(endSoc-startSoc) / 100000
}

func validBatterySOC(value sql.NullInt64) bool {
	return value.Valid && value.Int64 >= 0 && value.Int64 <= 100
}

func intPtr(value int) *int {
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}

func signedEnergyNetCost(signedKWh float64, importRateYen float64, exportRateYen float64) float64 {
	if signedKWh >= 0 {
		return signedKWh * importRateYen
	}
	return signedKWh * exportRateYen
}

func hasTariffRateSpread(lowestRateYen float64, highestRateYen float64) bool {
	return lowestRateYen > 0 && highestRateYen > 0 && !rateEquals(highestRateYen, lowestRateYen)
}

func isLowestTariffRate(rateYen float64, lowestRateYen float64, highestRateYen float64) bool {
	return hasTariffRateSpread(lowestRateYen, highestRateYen) && rateEquals(rateYen, lowestRateYen)
}

func isHighestTariffRate(rateYen float64, lowestRateYen float64, highestRateYen float64) bool {
	return hasTariffRateSpread(lowestRateYen, highestRateYen) && rateEquals(rateYen, highestRateYen)
}

func positiveFloat(value float64) float64 {
	if value > 0 {
		return value
	}
	return 0
}

func addTariffEnergy(importKWh *float64, exportKWh *float64, importCostYen *float64, exportIncomeYen *float64, signedKWh float64, importRateYen float64, exportRateYen float64) {
	if signedKWh >= 0 {
		*importKWh += signedKWh
		*importCostYen += signedKWh * importRateYen
		return
	}
	exported := -signedKWh
	*exportKWh += exported
	*exportIncomeYen += exported * exportRateYen
}

var ErrCannotDeleteLastTariffPlan = errors.New("cannot delete the last tariff plan")
var ErrTariffPlanCoverageRequired = errors.New("tariff plan coverage is required for current time")

func (r *TariffRepository) tariffPlanAt(ctx context.Context, at time.Time) (domain.TariffPlan, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, plan_name, day_rate_yen, home_rate_yen, night_rate_yen, export_rate_yen, timezone,
		effective_from, effective_to, created_at, updated_at
		FROM tariff_plans
		WHERE effective_from <= ?
			AND (effective_to IS NULL OR effective_to > ?)
		ORDER BY effective_from DESC, id DESC
		LIMIT 1`,
		at.UTC().Format(time.RFC3339Nano),
		at.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return domain.TariffPlan{}, err
	}
	defer rows.Close()
	plans, err := scanTariffPlans(rows)
	if err != nil {
		return domain.TariffPlan{}, err
	}
	if len(plans) == 0 {
		return domain.TariffPlan{}, sql.ErrNoRows
	}
	return plans[0], nil
}

func (r *TariffRepository) listTariffPlansAscending(ctx context.Context) ([]domain.TariffPlan, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, plan_name, day_rate_yen, home_rate_yen, night_rate_yen, export_rate_yen, timezone,
		effective_from, effective_to, created_at, updated_at
		FROM tariff_plans
		ORDER BY effective_from ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTariffPlans(rows)
}

func recalculateTariffPlanEffectiveTo(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT id, effective_from
		FROM tariff_plans
		ORDER BY effective_from ASC, id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type planBoundary struct {
		id            int64
		effectiveFrom string
	}
	plans := []planBoundary{}
	for rows.Next() {
		var plan planBoundary
		if err := rows.Scan(&plan.id, &plan.effectiveFrom); err != nil {
			return err
		}
		plans = append(plans, plan)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	now := time.Now().Format(time.RFC3339Nano)
	for i, plan := range plans {
		var effectiveTo any
		if i+1 < len(plans) {
			effectiveTo = plans[i+1].effectiveFrom
		}
		if _, err := tx.ExecContext(ctx, `UPDATE tariff_plans
			SET effective_to = ?, updated_at = ?
			WHERE id = ?`,
			effectiveTo,
			now,
			plan.id,
		); err != nil {
			return err
		}
	}
	return nil
}

func tariffPlansCoverTime(ctx context.Context, tx *sql.Tx, at time.Time) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*)
		FROM tariff_plans
		WHERE effective_from <= ?
			AND (effective_to IS NULL OR effective_to > ?)`,
		at.UTC().Format(time.RFC3339Nano),
		at.UTC().Format(time.RFC3339Nano),
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *TariffRepository) queryEnergyMeterDeltas(ctx context.Context, from *time.Time, to *time.Time) (*sql.Rows, error) {
	filter := EnergyMeterLogPageFilter{From: from, To: to}
	whereClause, args := energyMeterLogWhere(filter)
	return r.db.QueryContext(ctx, `SELECT measured_at, import_delta_kwh, export_delta_kwh
		FROM energy_meter_logs
		`+whereClause+`
		ORDER BY measured_at ASC, id ASC`, args...)
}

func scanTariffPlans(rows *sql.Rows) ([]domain.TariffPlan, error) {
	plans := []domain.TariffPlan{}
	for rows.Next() {
		var plan domain.TariffPlan
		var effectiveFrom, createdAt, updatedAt string
		var effectiveTo sql.NullString
		if err := rows.Scan(
			&plan.ID,
			&plan.PlanName,
			&plan.DayRateYen,
			&plan.HomeRateYen,
			&plan.NightRateYen,
			&plan.ExportRateYen,
			&plan.Timezone,
			&effectiveFrom,
			&effectiveTo,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		parsedEffectiveFrom, err := parseTime(effectiveFrom)
		if err != nil {
			return nil, err
		}
		plan.EffectiveFrom = parsedEffectiveFrom
		if effectiveTo.Valid {
			parsedEffectiveTo, err := parseTime(effectiveTo.String)
			if err != nil {
				return nil, err
			}
			plan.EffectiveTo = &parsedEffectiveTo
		}
		parsedCreatedAt, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		plan.CreatedAt = parsedCreatedAt
		parsedUpdatedAt, err := parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		plan.UpdatedAt = parsedUpdatedAt
		plans = append(plans, plan)
	}
	return plans, rows.Err()
}

func effectiveTariffPlan(plans []domain.TariffPlan, at time.Time) *domain.TariffPlan {
	for i := len(plans) - 1; i >= 0; i-- {
		plan := plans[i]
		if at.Before(plan.EffectiveFrom) {
			continue
		}
		if plan.EffectiveTo != nil && !at.Before(*plan.EffectiveTo) {
			continue
		}
		return &plans[i]
	}
	return nil
}

func tariffPlanAndRuleAt(plans []domain.TariffPlan, rulesByPlanID map[int64][]domain.TariffPeriodRule, at time.Time) (*domain.TariffPlan, domain.TariffPeriodRule, error) {
	plan := effectiveTariffPlan(plans, at)
	if plan == nil {
		return nil, domain.TariffPeriodRule{}, nil
	}
	location, err := time.LoadLocation(plan.Timezone)
	if err != nil {
		return nil, domain.TariffPeriodRule{}, err
	}
	rule := resolveTariffRule(rulesByPlanID[plan.ID], at.In(location))
	if rule.Period == "" {
		period := tariffPeriod(at.In(location))
		rule = domain.TariffPeriodRule{
			TariffPlanID: plan.ID,
			Period:       period,
			RateYen:      tariffRate(*plan, period),
		}
	}
	return plan, rule, nil
}

func tariffRate(plan domain.TariffPlan, period string) float64 {
	switch period {
	case "day":
		return plan.DayRateYen
	case "home":
		return plan.HomeRateYen
	default:
		return plan.NightRateYen
	}
}

func tariffPeriod(at time.Time) string {
	hour := at.Hour()
	if hour < 7 || hour >= 23 {
		return "night"
	}
	if isELifeHoliday(at) {
		return "home"
	}
	if hour >= 9 && hour < 17 {
		return "day"
	}
	return "home"
}

func isELifeHoliday(at time.Time) bool {
	weekday := at.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return true
	}
	month := at.Month()
	day := at.Day()
	return (month == time.January && (day == 2 || day == 3)) ||
		(month == time.April && day == 30) ||
		(month == time.May && (day == 1 || day == 2)) ||
		(month == time.December && (day == 30 || day == 31))
}

func nullStringValue(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func round4(value float64) float64 {
	return math.Round(value*10000) / 10000
}
