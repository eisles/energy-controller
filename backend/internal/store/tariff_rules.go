package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const tariffRateTolerance = 0.001

func (r *TariffRepository) CurrentTariffControlContext(ctx context.Context, at time.Time) (domain.TariffControlContext, error) {
	if at.IsZero() {
		at = time.Now()
	}
	plan, err := r.tariffPlanAt(ctx, at)
	if err != nil {
		return domain.TariffControlContext{}, err
	}
	location, err := time.LoadLocation(plan.Timezone)
	if err != nil {
		return domain.TariffControlContext{}, err
	}
	localAt := at.In(location)
	rules, source, err := r.effectiveTariffRulesForPlan(ctx, plan)
	if err != nil {
		return domain.TariffControlContext{}, err
	}
	rule := resolveTariffRule(rules, localAt)
	if rule.Period == "" {
		rules = defaultTariffPeriodRules(plan)
		source = "default"
		rule = resolveTariffRule(rules, localAt)
	}
	dayType := tariffDayType(localAt)
	lowest, highest := tariffRateBoundsForDayType(rules, dayType)
	nextLow := nextLowPriceStart(localAt, rules)
	var nextLowUTC *time.Time
	if !nextLow.IsZero() {
		utc := nextLow.UTC()
		nextLowUTC = &utc
	}
	hasPriceSpread := !rateEquals(highest, lowest)
	return domain.TariffControlContext{
		PlanName:       plan.PlanName,
		Timezone:       plan.Timezone,
		DayType:        dayType,
		CurrentPeriod:  rule.Period,
		CurrentRateYen: rule.RateYen,
		LowestRateYen:  lowest,
		HighestRateYen: highest,
		IsLowPrice:     hasPriceSpread && rateEquals(rule.RateYen, lowest),
		IsHighPrice:    hasPriceSpread && rateEquals(rule.RateYen, highest),
		NextLowPriceAt: nextLowUTC,
		Source:         source,
		Reason:         tariffContextReason(rule, source),
	}, nil
}

func (r *TariffRepository) effectiveTariffRulesForPlan(ctx context.Context, plan domain.TariffPlan) ([]domain.TariffPeriodRule, string, error) {
	rules, err := r.tariffRulesForPlan(ctx, plan.ID)
	if err != nil {
		return nil, "", err
	}
	if len(rules) > 0 {
		return rules, "custom", nil
	}
	return defaultTariffPeriodRules(plan), "default", nil
}

func (r *TariffRepository) tariffRulesForPlan(ctx context.Context, planID int64) ([]domain.TariffPeriodRule, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, tariff_plan_id, day_type, period, start_minute, end_minute, rate_yen, priority, created_at, updated_at
		FROM tariff_period_rules
		WHERE tariff_plan_id = ?
		ORDER BY day_type ASC, priority DESC, start_minute ASC, id ASC`, planID)
	if err != nil {
		if strings.Contains(err.Error(), "no such table: tariff_period_rules") {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanTariffPeriodRules(rows)
}

func saveTariffPeriodRules(ctx context.Context, tx *sql.Tx, planID int64, rules []domain.TariffPeriodRule, now string) error {
	if planID <= 0 {
		return errors.New("tariff plan id is required")
	}
	if len(rules) > 0 && !tariffRulesCoverAllDays(rules) {
		return errors.New("tariff period rules must cover every minute for weekday and holiday")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tariff_period_rules WHERE tariff_plan_id = ?`, planID); err != nil {
		return err
	}
	for _, rule := range rules {
		if !validTariffPeriodRule(rule) {
			return errors.New("tariff period rule is out of range")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tariff_period_rules (
			tariff_plan_id, day_type, period, start_minute, end_minute, rate_yen, priority, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			planID,
			rule.DayType,
			rule.Period,
			rule.StartMinute,
			rule.EndMinute,
			rule.RateYen,
			rule.Priority,
			now,
			now,
		); err != nil {
			return err
		}
	}
	return nil
}

func validTariffPeriodRule(rule domain.TariffPeriodRule) bool {
	return (rule.DayType == "weekday" || rule.DayType == "holiday") &&
		rule.Period != "" &&
		rule.StartMinute >= 0 &&
		rule.StartMinute <= 1439 &&
		rule.EndMinute >= 1 &&
		rule.EndMinute <= 1440 &&
		rule.RateYen > 0 &&
		rule.RateYen <= 500
}

func tariffRulesCoverAllDays(rules []domain.TariffPeriodRule) bool {
	coverage := map[string][]bool{
		"weekday": make([]bool, 1440),
		"holiday": make([]bool, 1440),
	}
	for _, rule := range rules {
		if !validTariffPeriodRule(rule) {
			return false
		}
		dayCoverage := coverage[rule.DayType]
		if dayCoverage == nil {
			return false
		}
		for minute := range dayCoverage {
			if tariffRuleContainsMinute(rule, minute) {
				dayCoverage[minute] = true
			}
		}
	}
	for _, dayType := range []string{"weekday", "holiday"} {
		for _, covered := range coverage[dayType] {
			if !covered {
				return false
			}
		}
	}
	return true
}

func scanTariffPeriodRules(rows *sql.Rows) ([]domain.TariffPeriodRule, error) {
	rules := []domain.TariffPeriodRule{}
	for rows.Next() {
		var rule domain.TariffPeriodRule
		var createdAt, updatedAt string
		if err := rows.Scan(
			&rule.ID,
			&rule.TariffPlanID,
			&rule.DayType,
			&rule.Period,
			&rule.StartMinute,
			&rule.EndMinute,
			&rule.RateYen,
			&rule.Priority,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		parsedCreatedAt, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		rule.CreatedAt = parsedCreatedAt
		parsedUpdatedAt, err := parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		rule.UpdatedAt = parsedUpdatedAt
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

func (r *TariffRepository) attachEffectiveTariffRules(ctx context.Context, plans []domain.TariffPlan) error {
	for i := range plans {
		rules, source, err := r.effectiveTariffRulesForPlan(ctx, plans[i])
		if err != nil {
			return err
		}
		plans[i].PeriodRules = rules
		plans[i].PeriodRuleSource = source
	}
	return nil
}

func defaultTariffPeriodRules(plan domain.TariffPlan) []domain.TariffPeriodRule {
	return []domain.TariffPeriodRule{
		{TariffPlanID: plan.ID, DayType: "weekday", Period: "night", StartMinute: 23 * 60, EndMinute: 7 * 60, RateYen: plan.NightRateYen, Priority: 10},
		{TariffPlanID: plan.ID, DayType: "weekday", Period: "home", StartMinute: 7 * 60, EndMinute: 9 * 60, RateYen: plan.HomeRateYen, Priority: 10},
		{TariffPlanID: plan.ID, DayType: "weekday", Period: "day", StartMinute: 9 * 60, EndMinute: 17 * 60, RateYen: plan.DayRateYen, Priority: 10},
		{TariffPlanID: plan.ID, DayType: "weekday", Period: "home", StartMinute: 17 * 60, EndMinute: 23 * 60, RateYen: plan.HomeRateYen, Priority: 10},
		{TariffPlanID: plan.ID, DayType: "holiday", Period: "night", StartMinute: 23 * 60, EndMinute: 7 * 60, RateYen: plan.NightRateYen, Priority: 10},
		{TariffPlanID: plan.ID, DayType: "holiday", Period: "home", StartMinute: 7 * 60, EndMinute: 23 * 60, RateYen: plan.HomeRateYen, Priority: 10},
	}
}

func resolveTariffRule(rules []domain.TariffPeriodRule, at time.Time) domain.TariffPeriodRule {
	dayType := tariffDayType(at)
	minute := at.Hour()*60 + at.Minute()
	return resolveTariffRuleForDayTypeMinute(rules, dayType, minute)
}

func resolveTariffRuleForDayTypeMinute(rules []domain.TariffPeriodRule, dayType string, minute int) domain.TariffPeriodRule {
	candidates := make([]domain.TariffPeriodRule, 0, len(rules))
	for _, rule := range rules {
		if rule.DayType != dayType {
			continue
		}
		if tariffRuleContainsMinute(rule, minute) {
			candidates = append(candidates, rule)
		}
	}
	if len(candidates) == 0 {
		return domain.TariffPeriodRule{}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Priority == candidates[j].Priority {
			return candidates[i].StartMinute < candidates[j].StartMinute
		}
		return candidates[i].Priority > candidates[j].Priority
	})
	return candidates[0]
}

func tariffRuleContainsMinute(rule domain.TariffPeriodRule, minute int) bool {
	start := clampMinute(rule.StartMinute)
	end := clampMinute(rule.EndMinute)
	if start == end {
		return true
	}
	if start < end {
		return minute >= start && minute < end
	}
	return minute >= start || minute < end
}

func tariffRateBounds(rules []domain.TariffPeriodRule) (float64, float64) {
	var lowest, highest float64
	for _, rule := range rules {
		if rule.RateYen <= 0 {
			continue
		}
		if lowest == 0 || rule.RateYen < lowest {
			lowest = rule.RateYen
		}
		if rule.RateYen > highest {
			highest = rule.RateYen
		}
	}
	return lowest, highest
}

func tariffRateBoundsForDayType(rules []domain.TariffPeriodRule, dayType string) (float64, float64) {
	var lowest, highest float64
	for minute := 0; minute < 1440; minute++ {
		rule := resolveTariffRuleForDayTypeMinute(rules, dayType, minute)
		if rule.RateYen <= 0 {
			continue
		}
		if lowest == 0 || rule.RateYen < lowest {
			lowest = rule.RateYen
		}
		if rule.RateYen > highest {
			highest = rule.RateYen
		}
	}
	if lowest == 0 {
		return tariffRateBounds(rules)
	}
	return lowest, highest
}

func nextLowPriceStart(at time.Time, rules []domain.TariffPeriodRule) time.Time {
	start := at.Truncate(time.Minute).Add(time.Minute)
	end := at.AddDate(0, 0, 2).Truncate(time.Minute).Add(24 * time.Hour)
	for candidate := start; !candidate.After(end); candidate = candidate.Add(time.Minute) {
		if !isLowPriceAt(candidate, rules) {
			continue
		}
		if !isLowPriceAt(candidate.Add(-time.Minute), rules) {
			return candidate
		}
	}
	return time.Time{}
}

func isLowPriceAt(at time.Time, rules []domain.TariffPeriodRule) bool {
	dayType := tariffDayType(at)
	lowest, highest := tariffRateBoundsForDayType(rules, dayType)
	if lowest <= 0 || rateEquals(highest, lowest) {
		return false
	}
	rule := resolveTariffRule(rules, at)
	return rule.RateYen > 0 && rateEquals(rule.RateYen, lowest)
}

func tariffDayType(at time.Time) string {
	if isELifeHoliday(at) {
		return "holiday"
	}
	return "weekday"
}

func tariffContextReason(rule domain.TariffPeriodRule, source string) string {
	if source == "custom" {
		return "tariff period resolved from tariff period rule master: " + rule.Period
	}
	return "tariff period resolved from default tariff rules: " + rule.Period
}

func clampMinute(value int) int {
	if value < 0 {
		return 0
	}
	if value > 1440 {
		return 1440
	}
	return value
}

func rateEquals(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff <= tariffRateTolerance
}
