package ecoflowprivate

import (
	"encoding/hex"
	"fmt"
	"math"
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
