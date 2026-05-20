package control

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const defaultPVEffectiveRadiationWPerM2 = 200.0

type PVForecastEstimate struct {
	DailyEstimatedPVKWh        float64
	PVEffectiveStartAt         string
	PVEffectiveEndAt           string
	PVEffectiveWindowSource    string
	PVEffectiveRadiationWPerM2 float64
	SolarRadiationKWhPerM2     float64
}

func EstimatePVForecast(forecast domain.WeatherForecast, location domain.WeatherLocation) PVForecastEstimate {
	ratio := location.PVPerformanceRatio
	if ratio <= 0 {
		ratio = 0.75
	}
	estimate := PVForecastEstimate{
		PVEffectiveRadiationWPerM2: defaultPVEffectiveRadiationWPerM2,
	}
	if len(forecast.HourlyShortwaveRadiation) > 0 {
		for _, hour := range forecast.HourlyShortwaveRadiation {
			estimate.SolarRadiationKWhPerM2 += hour.ShortwaveRadiationWPerM2 / 1000
		}
		segment := strongestPVEffectiveSegment(forecast.HourlyShortwaveRadiation, defaultPVEffectiveRadiationWPerM2)
		estimate.PVEffectiveStartAt = segment.start
		estimate.PVEffectiveEndAt = segment.end
		if segment.start == "" {
			estimate.PVEffectiveStartAt = fallbackPVEffectiveTime(forecast.Date, 9)
			estimate.PVEffectiveEndAt = fallbackPVEffectiveTime(forecast.Date, 16)
			estimate.PVEffectiveWindowSource = "hourly-radiation-no-effective-window"
		} else {
			estimate.PVEffectiveWindowSource = "hourly-radiation"
		}
	} else {
		estimate.SolarRadiationKWhPerM2 = forecast.ShortwaveRadiationMJPerM2 / 3.6
		estimate.PVEffectiveStartAt = fallbackPVEffectiveTime(forecast.Date, 9)
		estimate.PVEffectiveEndAt = fallbackPVEffectiveTime(forecast.Date, 16)
		estimate.PVEffectiveWindowSource = "fallback"
	}
	estimate.DailyEstimatedPVKWh = estimate.SolarRadiationKWhPerM2 * location.PVCapacityKW * ratio
	return estimate
}

type pvEffectiveSegment struct {
	start     string
	end       string
	radiation float64
	lastIndex int
	gap       int
}

func strongestPVEffectiveSegment(hours []domain.HourlyShortwaveRadiation, threshold float64) pvEffectiveSegment {
	var current pvEffectiveSegment
	var best pvEffectiveSegment
	for index, hour := range hours {
		if hour.ShortwaveRadiationWPerM2 >= threshold {
			if current.start == "" {
				current = pvEffectiveSegment{start: hour.Time, end: hour.Time, radiation: hour.ShortwaveRadiationWPerM2, lastIndex: index}
			} else {
				current.end = hour.Time
				current.radiation += hour.ShortwaveRadiationWPerM2
				current.lastIndex = index
				current.gap = 0
			}
			if current.radiation > best.radiation {
				best = current
			}
			continue
		}
		if current.start == "" {
			continue
		}
		if index-current.lastIndex <= 1 && current.gap == 0 {
			current.gap = 1
			continue
		}
		current = pvEffectiveSegment{}
	}
	return best
}

func fallbackPVEffectiveTime(date string, hour int) string {
	if date == "" {
		return ""
	}
	return fmt.Sprintf("%sT%02d:00", date, hour)
}

func expectedMorningToPVStartLoadKWh(input NightChargePlanInput, pvEffectiveStartAt string) float64 {
	if input.EcoFlowLoadEstimate == nil || input.EcoFlowLoadEstimate.SampleCount <= 0 || pvEffectiveStartAt == "" {
		return 0
	}
	startHour, ok := hourFromForecastTime(pvEffectiveStartAt)
	if !ok || startHour <= 7 {
		return 0
	}
	hours := startHour - 7
	if hours > 6 {
		hours = 6
	}
	hourlyLoadKWh := 0.0
	if input.EcoFlowLoadEstimate.AverageShoulderOutputKWh > 0 {
		hourlyLoadKWh = input.EcoFlowLoadEstimate.AverageShoulderOutputKWh / 9
	} else if input.EcoFlowLoadEstimate.AverageDailyOutputKWh > 0 {
		hourlyLoadKWh = input.EcoFlowLoadEstimate.AverageDailyOutputKWh / 24
	} else if input.EcoFlowLoadEstimate.AverageDaytimeOutputKWh > 0 {
		daytimeHours := input.EcoFlowLoadEstimate.DaytimeEndHour - input.EcoFlowLoadEstimate.DaytimeStartHour
		if daytimeHours > 0 {
			hourlyLoadKWh = input.EcoFlowLoadEstimate.AverageDaytimeOutputKWh / float64(daytimeHours)
		}
	}
	if hourlyLoadKWh <= 0 {
		return 0
	}
	return hourlyLoadKWh * float64(hours)
}

func hourFromForecastTime(value string) (int, bool) {
	parts := strings.Split(value, "T")
	if len(parts) != 2 || len(parts[1]) < 2 {
		return 0, false
	}
	hour, err := strconv.Atoi(parts[1][:2])
	if err != nil {
		return 0, false
	}
	return hour, true
}
