package control

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const Pro3ACOutputOffEventType = "ac_output_off_memory"

func BuildPro3ACOutputEvent(status domain.Status, previous *domain.SurplusControlCommandLog, now time.Time) (*domain.Pro3ACOutputEvent, bool) {
	outputPowerOffMemory, ok := boolDiagnostic(status.EcoFlowDiagnostics, "outputPowerOffMemory")
	if !ok || !outputPowerOffMemory {
		return nil, false
	}
	if now.IsZero() {
		now = time.Now()
	}
	measuredAt := status.UpdatedAt
	if measuredAt.IsZero() {
		measuredAt = now
	}
	event := &domain.Pro3ACOutputEvent{
		MeasuredAt:           measuredAt,
		EventType:            Pro3ACOutputOffEventType,
		OutputPowerOffMemory: outputPowerOffMemory,
		GridW:                status.GridW,
		ImportW:              status.ImportW,
		ExportW:              status.ExportW,
		BatterySoc:           status.BatterySoc,
		BatteryInputW:        status.BatteryInputW,
		BatteryOutputW:       status.BatteryOutputW,
		ACChargeLimitW:       status.ACChargeLimitW,
		BMSMaxCellTempC:      firstFloatDiagnostic(status.EcoFlowDiagnostics, "bmsMaxCellTemp", "bmsMasterTemp"),
		BMSMaxMosTempC:       firstFloatDiagnostic(status.EcoFlowDiagnostics, "bmsMaxMosTemp", "bmsMosTemp"),
		ACOutFreqHz:          firstFloatDiagnostic(status.EcoFlowDiagnostics, "acOutFreq"),
		ACOutDsgPowMaxW:      intDiagnostic(status.EcoFlowDiagnostics, "plugInInfoAcOutDsgPowMax"),
		CreatedAt:            now,
	}
	if previous != nil {
		measuredAt := previous.MeasuredAt
		event.PreviousCommandMeasuredAt = &measuredAt
		event.PreviousCommandKind = previous.CommandKind
		event.PreviousCommandSent = previous.CommandSent
		event.PreviousCommandWouldWrite = previous.WouldWrite
		event.PreviousCommandTargetACChargeW = previous.TargetACChargeLimitW
		event.PreviousCommandTargetReserveSoc = previous.TargetBackupReserveSoc
		event.PreviousCommandReason = previous.DecisionReason
	}
	event.Message = Pro3ACOutputEventMessage(*event)
	return event, true
}

func Pro3ACOutputEventMessage(event domain.Pro3ACOutputEvent) string {
	parts := []string{
		"DELTA Pro 3 のAC出力OFF履歴を検知しました",
		fmt.Sprintf("SOC %d%%", event.BatterySoc),
		fmt.Sprintf("AC出力 %dW", event.BatteryOutputW),
		fmt.Sprintf("AC入力 %dW", event.BatteryInputW),
		fmt.Sprintf("AC充電上限 %dW", event.ACChargeLimitW),
	}
	if event.BMSMaxCellTempC != nil {
		parts = append(parts, fmt.Sprintf("セル温度 %.1f℃", *event.BMSMaxCellTempC))
	}
	if event.BMSMaxMosTempC != nil {
		parts = append(parts, fmt.Sprintf("MOS温度 %.1f℃", *event.BMSMaxMosTempC))
	}
	if event.PreviousCommandKind != "" {
		parts = append(parts, fmt.Sprintf("直前制御 %s sent=%t wouldWrite=%t", event.PreviousCommandKind, event.PreviousCommandSent, event.PreviousCommandWouldWrite))
	}
	return strings.Join(parts, " / ")
}

func boolDiagnostic(values map[string]any, key string) (bool, bool) {
	value, ok := values[key]
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case float64:
		return typed != 0, true
	case int:
		return typed != 0, true
	case string:
		parsed, err := strconv.ParseBool(typed)
		if err == nil {
			return parsed, true
		}
		if typed == "1" {
			return true, true
		}
		if typed == "0" {
			return false, true
		}
	}
	return false, false
}

func floatDiagnostic(values map[string]any, key string) *float64 {
	value, ok := values[key]
	if !ok {
		return nil
	}
	var converted float64
	switch typed := value.(type) {
	case float64:
		converted = typed
	case float32:
		converted = float64(typed)
	case int:
		converted = float64(typed)
	case int64:
		converted = float64(typed)
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		if err != nil {
			return nil
		}
		converted = parsed
	default:
		return nil
	}
	if math.IsNaN(converted) || math.IsInf(converted, 0) {
		return nil
	}
	return &converted
}

func firstFloatDiagnostic(values map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		value := floatDiagnostic(values, key)
		if value != nil {
			return value
		}
	}
	return nil
}

func intDiagnostic(values map[string]any, key string) *int {
	value := floatDiagnostic(values, key)
	if value == nil {
		return nil
	}
	converted := int(math.Round(*value))
	return &converted
}
