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

type TariffRepository struct {
	db *sql.DB
}

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
	return summary, nil
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
