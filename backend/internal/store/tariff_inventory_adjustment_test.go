package store

import (
	"math"
	"testing"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

func TestFinalizeBatteryCostComparisonInventoryDoesNotAdjustUnchangedPeriodSOC(t *testing.T) {
	startSoc := 50
	endSoc := 50
	firstDayValue := 41.85
	firstDayRate := 34.06
	secondDayValue := -31.95
	secondDayRate := 26.0
	comparison := domain.BatteryCostComparison{
		EstimatedSavingsYen: 12.34,
		InventoryStartSoc:   &startSoc,
		InventoryEndSoc:     &endSoc,
		DailyBreakdown: []domain.BatteryCostComparisonDailyBreakdown{
			{InventoryValueYen: &firstDayValue, InventoryValueRateYen: &firstDayRate},
			{InventoryValueYen: &secondDayValue, InventoryValueRateYen: &secondDayRate},
		},
	}

	finalizeBatteryCostComparisonInventory(&comparison, 12_288)

	assertFloatPointerEquals(t, "InventoryDeltaKWh", comparison.InventoryDeltaKWh, 0)
	assertPeriodInventoryValuationUnavailable(t, comparison)
	assertDailyInventoryValuationUnavailable(t, comparison.DailyBreakdown[0])
	assertDailyInventoryValuationUnavailable(t, comparison.DailyBreakdown[1])
}

func TestFinalizeBatteryCostComparisonInventoryKeepsOnlyPeriodDelta(t *testing.T) {
	startSoc := 50
	endSoc := 60
	firstDayValue := 100.0
	firstDayRate := 26.0
	secondDayValue := -80.0
	secondDayRate := 34.06
	comparison := domain.BatteryCostComparison{
		EstimatedSavingsYen: 10.0,
		InventoryStartSoc:   &startSoc,
		InventoryEndSoc:     &endSoc,
		DailyBreakdown: []domain.BatteryCostComparisonDailyBreakdown{
			{InventoryValueYen: &firstDayValue, InventoryValueRateYen: &firstDayRate},
			{InventoryValueYen: &secondDayValue, InventoryValueRateYen: &secondDayRate},
		},
	}

	finalizeBatteryCostComparisonInventory(&comparison, 12_288)

	assertFloatPointerEquals(t, "InventoryDeltaKWh", comparison.InventoryDeltaKWh, 1.2288)
	assertPeriodInventoryValuationUnavailable(t, comparison)
	assertDailyInventoryValuationUnavailable(t, comparison.DailyBreakdown[0])
	assertDailyInventoryValuationUnavailable(t, comparison.DailyBreakdown[1])
}

func TestFinalizeDailyInventoryKeepsDeltaAndClearsValuation(t *testing.T) {
	startSoc := 40
	endSoc := 50
	delta := 99.0
	value := 88.0
	rate := 77.0
	adjusted := 66.0
	day := domain.BatteryCostComparisonDailyBreakdown{
		InventoryStartSoc:           &startSoc,
		InventoryEndSoc:             &endSoc,
		InventoryDeltaKWh:           &delta,
		InventoryValueYen:           &value,
		InventoryValueRateYen:       &rate,
		AdjustedEstimatedSavingsYen: &adjusted,
	}

	finalizeDailyInventory(&day, 12_288)

	assertFloatPointerEquals(t, "daily InventoryDeltaKWh", day.InventoryDeltaKWh, 1.2288)
	assertDailyInventoryValuationUnavailable(t, day)
}

func assertPeriodInventoryValuationUnavailable(t *testing.T, comparison domain.BatteryCostComparison) {
	t.Helper()
	if comparison.InventoryValueYen != nil || comparison.InventoryValueRateYen != nil || comparison.AdjustedEstimatedSavingsYen != nil {
		t.Fatalf(
			"period inventory valuation = value %v rate %v adjusted %v, want all nil",
			comparison.InventoryValueYen,
			comparison.InventoryValueRateYen,
			comparison.AdjustedEstimatedSavingsYen,
		)
	}
}

func assertDailyInventoryValuationUnavailable(t *testing.T, day domain.BatteryCostComparisonDailyBreakdown) {
	t.Helper()
	if day.InventoryValueYen != nil || day.InventoryValueRateYen != nil || day.AdjustedEstimatedSavingsYen != nil {
		t.Fatalf(
			"daily inventory valuation = value %v rate %v adjusted %v, want all nil",
			day.InventoryValueYen,
			day.InventoryValueRateYen,
			day.AdjustedEstimatedSavingsYen,
		)
	}
}

func assertFloatPointerEquals(t *testing.T, name string, actual *float64, expected float64) {
	t.Helper()
	if actual == nil {
		t.Fatalf("%s = nil, want %v", name, expected)
	}
	if math.Abs(*actual-expected) > 0.00001 {
		t.Fatalf("%s = %v, want %v", name, *actual, expected)
	}
}
