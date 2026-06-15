package ecoflowprivate

import (
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
)

type SnapshotField struct {
	MessageIndex int    `json:"messageIndex"`
	CmdFunc      int    `json:"cmdFunc"`
	CmdID        int    `json:"cmdId"`
	Field        int    `json:"field"`
	Wire         int    `json:"wire"`
	Value        string `json:"value"`
	RawHex       string `json:"rawHex,omitempty"`
}

type CycleFieldCandidate struct {
	MessageIndex int    `json:"messageIndex"`
	CmdFunc      int    `json:"cmdFunc"`
	CmdID        int    `json:"cmdId"`
	Field        int    `json:"field"`
	Value        int    `json:"value"`
	Reason       string `json:"reason"`
}

type CycleFieldCandidateObservation struct {
	CmdFunc      int    `json:"cmdFunc"`
	CmdID        int    `json:"cmdId"`
	Field        int    `json:"field"`
	Samples      int    `json:"samples"`
	Values       []int  `json:"values"`
	Stable       bool   `json:"stable"`
	PresentInAll bool   `json:"presentInAll"`
	Reason       string `json:"reason"`
}

const maxCycleFieldCandidates = 80

var knownDecodedTelemetryFields = map[[3]int]struct{}{
	{254, 21, 3}:    {},
	{254, 21, 4}:    {},
	{254, 21, 7}:    {},
	{254, 21, 8}:    {},
	{254, 21, 13}:   {},
	{254, 21, 14}:   {},
	{254, 21, 15}:   {},
	{254, 21, 16}:   {},
	{254, 21, 25}:   {},
	{254, 21, 33}:   {},
	{254, 21, 54}:   {},
	{254, 21, 76}:   {},
	{254, 21, 146}:  {},
	{254, 21, 147}:  {},
	{254, 21, 209}:  {},
	{254, 21, 242}:  {},
	{254, 21, 262}:  {},
	{254, 21, 270}:  {},
	{254, 21, 271}:  {},
	{254, 21, 281}:  {},
	{254, 21, 282}:  {},
	{254, 21, 361}:  {},
	{254, 21, 367}:  {},
	{254, 21, 368}:  {},
	{254, 21, 971}:  {},
	{254, 21, 1539}: {},
	{254, 22, 24}:   {},
	{32, 2, 15}:     {},
}

// These fields are still available through --inspect-fields, but repeated live
// DELTA 3 observations showed them behaving like state, config, voltage,
// temperature, timing, or frequency values rather than cycle counters.
var observedNonCycleTelemetryFields = map[[3]int]struct{}{
	{32, 50, 3}:     {},
	{32, 50, 9}:     {},
	{32, 50, 10}:    {},
	{32, 50, 14}:    {},
	{32, 50, 15}:    {},
	{32, 50, 18}:    {},
	{32, 50, 19}:    {},
	{32, 50, 20}:    {},
	{32, 50, 21}:    {},
	{32, 50, 23}:    {},
	{32, 50, 29}:    {},
	{32, 50, 31}:    {},
	{32, 50, 32}:    {},
	{32, 50, 34}:    {},
	{32, 50, 37}:    {},
	{32, 50, 40}:    {},
	{32, 50, 41}:    {},
	{32, 50, 46}:    {},
	{32, 50, 55}:    {},
	{32, 50, 63}:    {},
	{32, 50, 64}:    {},
	{32, 50, 67}:    {},
	{32, 50, 68}:    {},
	{32, 50, 71}:    {},
	{254, 21, 5}:    {},
	{254, 21, 17}:   {},
	{254, 21, 18}:   {},
	{254, 21, 19}:   {},
	{254, 21, 20}:   {},
	{254, 21, 45}:   {},
	{254, 21, 46}:   {},
	{254, 21, 47}:   {},
	{254, 21, 133}:  {},
	{254, 21, 144}:  {},
	{254, 21, 145}:  {},
	{254, 21, 152}:  {},
	{254, 21, 211}:  {},
	{254, 21, 258}:  {},
	{254, 21, 259}:  {},
	{254, 21, 260}:  {},
	{254, 21, 261}:  {},
	{254, 21, 272}:  {},
	{254, 21, 273}:  {},
	{254, 21, 360}:  {},
	{254, 21, 414}:  {},
	{254, 21, 461}:  {},
	{254, 21, 1279}: {},
	{254, 21, 1460}: {},
	{254, 22, 60}:   {},
}

func InspectSnapshotFields(raw []byte) ([]SnapshotField, error) {
	raw = maybeBase64(raw)
	headers, err := decodeHeaderMessage(raw)
	if err != nil {
		return nil, err
	}
	fields := []SnapshotField{}
	for index, header := range headers {
		pdata := header.PData
		if header.EncType == 1 && header.Src != 32 {
			pdata = xorDecode(pdata, header.Seq)
		}
		inspected, err := inspectPayloadFields(index, header, pdata)
		if err != nil {
			return fields, err
		}
		fields = append(fields, inspected...)
	}
	return fields, nil
}

func CycleFieldCandidatesFromFields(fields []SnapshotField) []CycleFieldCandidate {
	return cycleFieldCandidatesFromFields(fields, maxCycleFieldCandidates)
}

func AllCycleFieldCandidatesFromFields(fields []SnapshotField) []CycleFieldCandidate {
	return cycleFieldCandidatesFromFields(fields, 0)
}

func cycleFieldCandidatesFromFields(fields []SnapshotField, limit int) []CycleFieldCandidate {
	candidates := make([]CycleFieldCandidate, 0)
	seen := make(map[[5]int]struct{})
	for _, field := range fields {
		if limit > 0 && len(candidates) >= limit {
			break
		}
		if field.Wire != wireVarint || isKnownDecodedTelemetryField(field.CmdFunc, field.CmdID, field.Field) {
			continue
		}
		value, err := strconv.Atoi(field.Value)
		if err != nil || value < 2 || value > 5000 {
			continue
		}
		key := [5]int{field.MessageIndex, field.CmdFunc, field.CmdID, field.Field, value}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, CycleFieldCandidate{
			MessageIndex: field.MessageIndex,
			CmdFunc:      field.CmdFunc,
			CmdID:        field.CmdID,
			Field:        field.Field,
			Value:        value,
			Reason:       "unknown private MQTT varint field in plausible cycle-count range",
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		a := candidates[i]
		b := candidates[j]
		if a.MessageIndex != b.MessageIndex {
			return a.MessageIndex < b.MessageIndex
		}
		if a.CmdFunc != b.CmdFunc {
			return a.CmdFunc < b.CmdFunc
		}
		if a.CmdID != b.CmdID {
			return a.CmdID < b.CmdID
		}
		if a.Field != b.Field {
			return a.Field < b.Field
		}
		return a.Value < b.Value
	})
	return candidates
}

func CycleFieldCandidatesFromSnapshot(raw []byte) ([]CycleFieldCandidate, error) {
	fields, err := InspectSnapshotFields(raw)
	if err != nil {
		return nil, err
	}
	return CycleFieldCandidatesFromFields(fields), nil
}

func SummarizeCycleFieldCandidates(samples [][]SnapshotField) []CycleFieldCandidateObservation {
	if len(samples) == 0 {
		return nil
	}
	valuesByKey := make(map[[3]int]map[int]struct{})
	presentByKey := make(map[[3]int]map[int]struct{})
	for sampleIndex, fields := range samples {
		seenInSample := make(map[[3]int]struct{})
		for _, candidate := range CycleFieldCandidatesFromFields(fields) {
			key := [3]int{candidate.CmdFunc, candidate.CmdID, candidate.Field}
			if _, ok := valuesByKey[key]; !ok {
				valuesByKey[key] = make(map[int]struct{})
			}
			valuesByKey[key][candidate.Value] = struct{}{}
			seenInSample[key] = struct{}{}
		}
		for key := range seenInSample {
			if _, ok := presentByKey[key]; !ok {
				presentByKey[key] = make(map[int]struct{})
			}
			presentByKey[key][sampleIndex] = struct{}{}
		}
	}
	observations := make([]CycleFieldCandidateObservation, 0, len(valuesByKey))
	for key, valueSet := range valuesByKey {
		values := make([]int, 0, len(valueSet))
		for value := range valueSet {
			values = append(values, value)
		}
		sort.Ints(values)
		presentSamples := len(presentByKey[key])
		observations = append(observations, CycleFieldCandidateObservation{
			CmdFunc:      key[0],
			CmdID:        key[1],
			Field:        key[2],
			Samples:      presentSamples,
			Values:       values,
			Stable:       len(values) == 1,
			PresentInAll: presentSamples == len(samples),
			Reason:       cycleObservationReason(values, presentSamples, len(samples)),
		})
	}
	sort.SliceStable(observations, func(i, j int) bool {
		a := observations[i]
		b := observations[j]
		if a.PresentInAll != b.PresentInAll {
			return a.PresentInAll
		}
		if a.Stable != b.Stable {
			return a.Stable
		}
		if a.CmdFunc != b.CmdFunc {
			return a.CmdFunc < b.CmdFunc
		}
		if a.CmdID != b.CmdID {
			return a.CmdID < b.CmdID
		}
		return a.Field < b.Field
	})
	return observations
}

func inspectPayloadFields(index int, header delta3Header, raw []byte) ([]SnapshotField, error) {
	fields := []SnapshotField{}
	for len(raw) > 0 {
		field, wire, valueRaw, ok := readTag(raw)
		if !ok {
			return fields, fmt.Errorf("invalid payload tag")
		}
		before := valueRaw
		value, next, ok := inspectValue(wire, valueRaw)
		if !ok {
			return fields, fmt.Errorf("invalid payload value for field %d wire %d", field, wire)
		}
		fields = append(fields, SnapshotField{
			MessageIndex: index,
			CmdFunc:      header.CmdFunc,
			CmdID:        header.CmdID,
			Field:        field,
			Wire:         wire,
			Value:        value,
			RawHex:       hex.EncodeToString(before[:len(before)-len(next)]),
		})
		raw = next
	}
	return fields, nil
}

func cycleObservationReason(values []int, presentSamples int, totalSamples int) string {
	if len(values) == 1 && presentSamples == totalSamples {
		return "stable candidate across all read-only samples; still investigation-only"
	}
	if len(values) == 1 {
		return "stable candidate but missing from some read-only samples; still investigation-only"
	}
	return "changed across read-only samples; unlikely to be a simple cycle count"
}

func inspectValue(wire int, raw []byte) (string, []byte, bool) {
	switch wire {
	case wireVarint:
		value, next, ok := readVarint(raw)
		if !ok {
			return "", nil, false
		}
		return fmt.Sprintf("%d", value), next, true
	case wireBytes:
		value, next, ok := readBytes(raw)
		if !ok {
			return "", nil, false
		}
		return fmt.Sprintf("bytes:%d", len(value)), next, true
	case wireFixed32:
		if len(raw) < 4 {
			return "", nil, false
		}
		bits := uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24
		floatValue := math.Float32frombits(bits)
		return fmt.Sprintf("float32:%g", floatValue), raw[4:], true
	default:
		next, ok := skipValue(wire, raw)
		if !ok {
			return "", nil, false
		}
		return fmt.Sprintf("wire:%d", wire), next, true
	}
}

func isKnownDecodedTelemetryField(cmdFunc int, cmdID int, field int) bool {
	if _, ok := knownDecodedTelemetryFields[[3]int{cmdFunc, cmdID, field}]; ok {
		return true
	}
	if _, ok := observedNonCycleTelemetryFields[[3]int{cmdFunc, cmdID, field}]; ok {
		return true
	}
	if isBMSHeartbeat(cmdFunc, cmdID) {
		switch field {
		case 6, 26, 27, 47:
			return true
		}
	}
	return false
}
