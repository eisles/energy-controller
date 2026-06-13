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

const maxCycleCandidates = 20

var CycleCountKeys = []string{
	"cycles",
	"bmsCycles",
	"bmsBattCycles",
	"bms_bmsStatus.cycles",
	"hs_yj751_bms_slave_addr.1.cycles",
}

var cycleCandidateKeys = map[string]struct{}{
	"cycles":                           {},
	"bmscycles":                        {},
	"bmsbattcycles":                    {},
	"bms_bmsstatus.cycles":             {},
	"hs_yj751_bms_slave_addr.1.cycles": {},
	"cycsoh":                           {},
	"cyclesoh":                         {},
}

type CycleCandidate struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type CycleStatus struct {
	CycleCount       *int
	CycleCountSource string
	Key              string
	QuotaKeyCount    int
	CycleCandidates  []CycleCandidate
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
	status := CycleStatus{
		QuotaKeyCount:   len(quotas),
		CycleCandidates: cycleCandidatesFromQuota(quotas),
	}
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

func cycleCandidatesFromQuota(quotas map[string]any) []CycleCandidate {
	flattened := make(map[string]any)
	flattenQuotaValues("", quotas, flattened)
	keys := make([]string, 0, len(flattened))
	for key := range flattened {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	candidates := make([]CycleCandidate, 0)
	for _, key := range keys {
		if len(candidates) >= maxCycleCandidates {
			break
		}
		if !isCycleCandidateKey(key) {
			continue
		}
		value, ok := candidateScalar(flattened[key])
		if !ok {
			continue
		}
		candidates = append(candidates, CycleCandidate{Key: key, Value: value})
	}
	return candidates
}

func flattenQuotaValues(prefix string, value any, out map[string]any) {
	object, ok := value.(map[string]any)
	if !ok {
		if prefix != "" {
			out[prefix] = value
		}
		return
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		next := key
		if prefix != "" {
			next = prefix + "." + key
		}
		flattenQuotaValues(next, object[key], out)
	}
}

func isCycleCandidateKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if _, ok := cycleCandidateKeys[normalized]; ok {
		return true
	}
	segments := strings.Split(normalized, ".")
	last := segments[len(segments)-1]
	_, ok := cycleCandidateKeys[last]
	return ok
}

func candidateScalar(value any) (any, bool) {
	switch typed := value.(type) {
	case json.Number:
		return typed, true
	case string:
		return typed, true
	case float64:
		return typed, true
	case int:
		return typed, true
	case int64:
		return typed, true
	default:
		return nil, false
	}
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
