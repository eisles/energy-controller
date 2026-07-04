package store

import (
	"math"
	"testing"
	"time"
)

func TestEstimateEcoFlowLoadSplitsEveningFromShoulder(t *testing.T) {
	location, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, location)
	samples := []ecoflowLoadSample{}
	// 朝shoulder(8時台): 600Wを1時間
	// 日中(10時台): 400Wを1時間
	// 夕方shoulder(18時台): 900Wを1時間
	// 深夜(23時台): 300Wを1時間
	for _, block := range []struct {
		hour    int
		outputW int
	}{
		{8, 600},
		{10, 400},
		{18, 900},
		{23, 300},
	} {
		start := day.Add(time.Duration(block.hour) * time.Hour)
		for minute := 0; minute <= 60; minute++ {
			samples = append(samples, ecoflowLoadSample{
				measuredAt:     start.Add(time.Duration(minute) * time.Minute),
				batteryOutputW: block.outputW,
			})
		}
	}
	now := day.Add(24 * time.Hour)

	estimate := estimateEcoFlowLoadFromSamples(samples, now, 7, location)

	if len(estimate.Daily) != 1 {
		t.Fatalf("Daily days = %d, want 1", len(estimate.Daily))
	}
	daily := estimate.Daily[0]
	if !almostEqualKWh(daily.EveningOutputKWh, 0.9) {
		t.Fatalf("EveningOutputKWh = %f, want 0.9", daily.EveningOutputKWh)
	}
	// shoulderは朝0.6kWh+夕方0.9kWhの合算のまま
	if !almostEqualKWh(daily.ShoulderOutputKWh, 1.5) {
		t.Fatalf("ShoulderOutputKWh = %f, want 1.5", daily.ShoulderOutputKWh)
	}
	if !almostEqualKWh(estimate.AverageEveningOutputKWh, 0.9) {
		t.Fatalf("AverageEveningOutputKWh = %f, want 0.9", estimate.AverageEveningOutputKWh)
	}
}

func almostEqualKWh(got, want float64) bool {
	return math.Abs(got-want) < 0.02
}
