package ecoflowdeveloper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/eisles/energy-controller/backend/internal/ecoflowprivate"
)

const CycleCountSource = "ecoflow_developer_mqtt_quota"

var CycleCountKeys = []string{
	"cycles",
	"bmsCycles",
	"bmsBattCycles",
	"bms_bmsStatus.cycles",
	"hs_yj751_bms_slave_addr.1.cycles",
}

type CycleStatus struct {
	CycleCount       *int
	CycleCountSource string
	Key              string
	QuotaKeyCount    int
}

type QuotaMessage struct {
	TopicKind    string
	PayloadBytes int
	CycleStatus  CycleStatus
	KeyNames     []string
	ParseError   string
}

func QuotaMessageFromMQTT(msg ecoflowprivate.MQTTMessage, isQuota bool) QuotaMessage {
	kind := "status"
	if isQuota {
		kind = "quota"
	}
	result := QuotaMessage{
		TopicKind:    kind,
		PayloadBytes: len(msg.Payload),
	}
	if !isQuota {
		return result
	}
	status, err := CycleStatusFromQuotaPayload(msg.Payload)
	if err != nil {
		result.ParseError = err.Error()
		return result
	}
	result.CycleStatus = status
	result.KeyNames = QuotaKeyNamesFromPayload(msg.Payload)
	return result
}

func CycleStatusFromQuotaPayload(raw []byte) (CycleStatus, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return CycleStatus{}, fmt.Errorf("decode EcoFlow Developer MQTT quota payload: %w", err)
	}
	quotas := quotaMapFromPayload(payload)
	status := CycleStatus{QuotaKeyCount: len(quotas)}
	for _, key := range CycleCountKeys {
		rawValue, ok := quotaValue(quotas, key)
		if !ok {
			continue
		}
		value, ok := nonNegativeInt(rawValue)
		if !ok {
			continue
		}
		status.CycleCount = &value
		status.CycleCountSource = CycleCountSource
		status.Key = key
		return status, nil
	}
	return status, nil
}

func QuotaKeyNamesFromPayload(raw []byte) []string {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return nil
	}
	quotas := quotaMapFromPayload(payload)
	keys := make([]string, 0, len(quotas))
	for key := range quotas {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func quotaMapFromPayload(payload map[string]any) map[string]any {
	for _, wrapper := range []string{"params", "param"} {
		if wrapped, ok := payload[wrapper].(map[string]any); ok {
			return wrapped
		}
	}
	return payload
}

func quotaValue(quotas map[string]any, key string) (any, bool) {
	if value, ok := quotas[key]; ok {
		return value, true
	}
	current := any(quotas)
	for _, segment := range strings.Split(key, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func nonNegativeInt(value any) (int, bool) {
	var number float64
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		number = parsed
	case float64:
		number = typed
	case int:
		number = float64(typed)
	case int64:
		number = float64(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		number = float64(parsed)
	default:
		return 0, false
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || number < 0 || number > 100000 || math.Trunc(number) != number {
		return 0, false
	}
	return int(number), true
}
