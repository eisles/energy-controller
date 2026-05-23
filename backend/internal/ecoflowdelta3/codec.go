package ecoflowdelta3

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math"
	"time"
)

type CommandPayload struct {
	Command string `json:"command"`
	Topic   string `json:"topic"`
	Bytes   []byte `json:"-"`
	Hex     string `json:"hex,omitempty"`
}

type delta3Header struct {
	PData    []byte
	Src      int
	Dest     int
	DSrc     int
	DDest    int
	EncType  int
	CmdFunc  int
	CmdID    int
	DataLen  int
	NeedAck  int
	Seq      int
	Product  int
	Version  int
	Payload  int
	From     string
	DeviceSN string
}

func BuildGetSnapshotPayload(seq int) []byte {
	header := delta3Header{
		Src:  32,
		Dest: 32,
		Seq:  seq,
		From: "HomeAssistant",
	}
	return encodeHeaderMessage(header)
}

func BuildSetACChargePowerPayload(deviceSN string, watts int, seq int) ([]byte, error) {
	if watts <= 0 {
		return nil, fmt.Errorf("AC charge power must be positive: %d", watts)
	}
	pdata := append(encodeTag(54, wireVarint), encodeVarint(uint64(watts))...)
	pdata = append(pdata, encodeTag(125, wireVarint)...)
	pdata = append(pdata, 0)
	return encodeSetCommandHeader(deviceSN, seq, pdata), nil
}

func BuildSetBackupReservePayload(deviceSN string, percent int, seq int) ([]byte, error) {
	if err := ValidateBackupReserveSoc(percent); err != nil {
		return nil, err
	}
	inner := append(encodeTag(1, wireVarint), encodeVarint(1)...)
	inner = append(inner, encodeTag(2, wireVarint)...)
	inner = append(inner, encodeVarint(uint64(percent))...)
	pdata := append(encodeTag(43, wireBytes), encodeVarint(uint64(len(inner)))...)
	pdata = append(pdata, inner...)
	return encodeSetCommandHeader(deviceSN, seq, pdata), nil
}

func DecodeSnapshot(deviceType string, deviceSN string, raw []byte) (Status, error) {
	status := Status{DeviceType: deviceType, DeviceSN: deviceSN}
	raw = maybeBase64(raw)
	headers, err := decodeHeaderMessage(raw)
	if err != nil {
		return status, err
	}
	for _, header := range headers {
		if len(header.PData) == 0 {
			continue
		}
		pdata := header.PData
		if header.EncType == 1 && header.Src != 32 {
			pdata = xorDecode(pdata, header.Seq)
		}
		part := decodePayload(header, pdata)
		status.merge(part)
	}
	return status, nil
}

func NextSeq() int {
	return int(time.Now().UnixMilli() % 2147483647)
}

func encodeSetCommandHeader(deviceSN string, seq int, pdata []byte) []byte {
	header := delta3Header{
		PData:    pdata,
		Src:      32,
		Dest:     2,
		DSrc:     1,
		DDest:    1,
		CmdFunc:  254,
		CmdID:    17,
		DataLen:  len(pdata),
		NeedAck:  1,
		Seq:      seq,
		Product:  1,
		Version:  19,
		Payload:  1,
		DeviceSN: deviceSN,
	}
	return encodeHeaderMessage(header)
}

func encodeHeaderMessage(headers ...delta3Header) []byte {
	var out []byte
	for _, header := range headers {
		encoded := encodeHeader(header)
		out = append(out, encodeTag(1, wireBytes)...)
		out = append(out, encodeVarint(uint64(len(encoded)))...)
		out = append(out, encoded...)
	}
	return out
}

func encodeHeader(h delta3Header) []byte {
	var out []byte
	if len(h.PData) > 0 {
		out = appendBytesField(out, 1, h.PData)
	}
	out = appendIntField(out, 2, h.Src)
	out = appendIntField(out, 3, h.Dest)
	out = appendIntField(out, 4, h.DSrc)
	out = appendIntField(out, 5, h.DDest)
	out = appendIntField(out, 6, h.EncType)
	out = appendIntField(out, 8, h.CmdFunc)
	out = appendIntField(out, 9, h.CmdID)
	out = appendIntField(out, 10, h.DataLen)
	out = appendIntField(out, 11, h.NeedAck)
	out = appendIntField(out, 14, h.Seq)
	out = appendIntField(out, 15, h.Product)
	out = appendIntField(out, 16, h.Version)
	out = appendIntField(out, 17, h.Payload)
	if h.From != "" {
		out = appendStringField(out, 23, h.From)
	}
	if h.DeviceSN != "" {
		out = appendStringField(out, 25, h.DeviceSN)
	}
	return out
}

func appendIntField(out []byte, field int, value int) []byte {
	if value == 0 {
		return out
	}
	out = append(out, encodeTag(field, wireVarint)...)
	return append(out, encodeVarint(uint64(value))...)
}

func appendBytesField(out []byte, field int, value []byte) []byte {
	out = append(out, encodeTag(field, wireBytes)...)
	out = append(out, encodeVarint(uint64(len(value)))...)
	return append(out, value...)
}

func appendStringField(out []byte, field int, value string) []byte {
	return appendBytesField(out, field, []byte(value))
}

func decodeHeaderMessage(raw []byte) ([]delta3Header, error) {
	var headers []delta3Header
	for len(raw) > 0 {
		field, wire, rest, ok := readTag(raw)
		if !ok {
			return nil, fmt.Errorf("invalid protobuf tag")
		}
		raw = rest
		if field != 1 || wire != wireBytes {
			var skipped bool
			raw, skipped = skipValue(wire, raw)
			if !skipped {
				return nil, fmt.Errorf("invalid top-level header value")
			}
			continue
		}
		var length uint64
		var okLen bool
		length, raw, okLen = readVarint(raw)
		if !okLen || length > uint64(len(raw)) {
			return nil, fmt.Errorf("invalid header length")
		}
		n := int(length)
		header, err := decodeHeader(raw[:n])
		if err != nil {
			return nil, err
		}
		headers = append(headers, header)
		raw = raw[n:]
	}
	if len(headers) == 0 {
		return nil, fmt.Errorf("DELTA_3 payload does not include headers")
	}
	return headers, nil
}

func decodeHeader(raw []byte) (delta3Header, error) {
	var h delta3Header
	for len(raw) > 0 {
		field, wire, rest, ok := readTag(raw)
		if !ok {
			return h, fmt.Errorf("invalid header tag")
		}
		raw = rest
		switch field {
		case 1:
			value, next, ok := readBytes(raw)
			if !ok {
				return h, fmt.Errorf("invalid pdata")
			}
			h.PData = value
			raw = next
		case 2, 3, 4, 5, 6, 8, 9, 10, 11, 14, 15, 16, 17:
			value, next, ok := readVarint(raw)
			if !ok {
				return h, fmt.Errorf("invalid header varint")
			}
			assignHeaderInt(&h, field, int(value))
			raw = next
		case 23, 25:
			value, next, ok := readBytes(raw)
			if !ok {
				return h, fmt.Errorf("invalid header string")
			}
			if field == 23 {
				h.From = string(value)
			} else {
				h.DeviceSN = string(value)
			}
			raw = next
		default:
			var skipped bool
			raw, skipped = skipValue(wire, raw)
			if !skipped {
				return h, fmt.Errorf("invalid header unknown field")
			}
		}
	}
	return h, nil
}

func assignHeaderInt(h *delta3Header, field int, value int) {
	switch field {
	case 2:
		h.Src = value
	case 3:
		h.Dest = value
	case 4:
		h.DSrc = value
	case 5:
		h.DDest = value
	case 6:
		h.EncType = value
	case 8:
		h.CmdFunc = value
	case 9:
		h.CmdID = value
	case 10:
		h.DataLen = value
	case 11:
		h.NeedAck = value
	case 14:
		h.Seq = value
	case 15:
		h.Product = value
	case 16:
		h.Version = value
	case 17:
		h.Payload = value
	}
}

func decodePayload(header delta3Header, raw []byte) Status {
	switch {
	case header.CmdFunc == 254 && header.CmdID == 21:
		return decodeDisplayUpload(raw)
	case header.CmdFunc == 254 && header.CmdID == 22:
		return decodeRuntimeUpload(raw)
	case header.CmdFunc == 254 && header.CmdID == 18:
		status := decodeSetReply(raw)
		status.LastSetReplySeq = &header.Seq
		return status
	case header.CmdFunc == 32 && header.CmdID == 2:
		return decodeCMSHeartbeat(raw)
	case isBMSHeartbeat(header.CmdFunc, header.CmdID):
		return decodeBMSHeartbeat(raw)
	default:
		return Status{UnsupportedMessages: 1}
	}
}

func decodeDisplayUpload(raw []byte) Status {
	var s Status
	for len(raw) > 0 {
		field, wire, rest, ok := readTag(raw)
		if !ok {
			s.UnsupportedMessages++
			return s
		}
		raw = rest
		switch field {
		case 3:
			raw = assignFloat(raw, wire, &s.InputW)
		case 4:
			raw = assignFloat(raw, wire, &s.OutputW)
		case 7:
			raw = assignBoolVarint(raw, wire, &s.BackupReserveEnabled)
		case 8:
			raw = assignIntVarint(raw, wire, &s.BackupReserveSoc)
		case 13, 14, 15, 16:
			raw = assignFlowEnabled(raw, wire, &s.USBOutputEnabled)
		case 25:
			raw = assignBoolVarint(raw, wire, &s.XBoostEnabled)
		case 33:
			raw = assignFlowEnabled(raw, wire, &s.DCOutputEnabled)
		case 54:
			raw = assignFloat(raw, wire, &s.ACInW)
		case 76:
			raw = assignBoolVarint(raw, wire, &s.ACOutputEnabled)
		case 146:
			raw = assignBoolVarint(raw, wire, &s.GridBypassDisabled)
		case 147:
			raw = assignBoolVarint(raw, wire, &s.OutputPowerOffMemory)
		case 209:
			raw = assignIntVarint(raw, wire, &s.ACChargeLimitW)
		case 242:
			raw = assignFloat(raw, wire, &s.BMSBatterySoc)
		case 262:
			raw = assignFloat(raw, wire, &s.CMSBatterySoc)
		case 270:
			raw = assignIntVarint(raw, wire, &s.MaxChargeSoc)
		case 271:
			raw = assignIntVarint(raw, wire, &s.MinDischargeSoc)
		case 281:
			raw = assignIntVarint(raw, wire, &s.ChargingState)
			s.BMSChargingState = s.ChargingState
		case 282:
			raw = assignIntVarint(raw, wire, &s.CMSChargingState)
		case 361:
			raw = assignFloat(raw, wire, &s.PVInW)
		case 367:
			raw = assignFlowEnabled(raw, wire, &s.ACOutputEnabled)
		case 368:
			raw = assignFloat(raw, wire, &s.ACOutW)
		default:
			var skipped bool
			raw, skipped = skipValue(wire, raw)
			if !skipped {
				s.UnsupportedMessages++
				return s
			}
		}
	}
	s.DecodedMessages = 1
	return s
}

func decodeRuntimeUpload(raw []byte) Status {
	var s Status
	for len(raw) > 0 {
		field, wire, rest, ok := readTag(raw)
		if !ok {
			s.UnsupportedMessages++
			return s
		}
		raw = rest
		switch field {
		case 24:
			raw = assignIntVarint(raw, wire, &s.PCSWorkMode)
		default:
			var skipped bool
			raw, skipped = skipValue(wire, raw)
			if !skipped {
				s.UnsupportedMessages++
				return s
			}
		}
	}
	s.DecodedMessages = 1
	return s
}

func decodeCMSHeartbeat(raw []byte) Status {
	var s Status
	for len(raw) > 0 {
		field, wire, rest, ok := readTag(raw)
		if !ok {
			s.UnsupportedMessages++
			return s
		}
		raw = rest
		switch field {
		case 15:
			raw = assignFloat(raw, wire, &s.CMSBatterySoc)
		default:
			var skipped bool
			raw, skipped = skipValue(wire, raw)
			if !skipped {
				s.UnsupportedMessages++
				return s
			}
		}
	}
	s.DecodedMessages = 1
	return s
}

func decodeBMSHeartbeat(raw []byte) Status {
	var s Status
	for len(raw) > 0 {
		field, wire, rest, ok := readTag(raw)
		if !ok {
			s.UnsupportedMessages++
			return s
		}
		raw = rest
		switch field {
		case 6:
			raw = assignIntVarint(raw, wire, &s.BMSBatterySoc)
		case 26:
			raw = assignIntVarint(raw, wire, &s.InputW)
		case 27:
			raw = assignIntVarint(raw, wire, &s.OutputW)
		case 47:
			raw = assignIntVarint(raw, wire, &s.ChargingState)
		default:
			var skipped bool
			raw, skipped = skipValue(wire, raw)
			if !skipped {
				s.UnsupportedMessages++
				return s
			}
		}
	}
	s.DecodedMessages = 1
	return s
}

func decodeSetReply(raw []byte) Status {
	var s Status
	for len(raw) > 0 {
		field, wire, rest, ok := readTag(raw)
		if !ok {
			s.UnsupportedMessages++
			return s
		}
		raw = rest
		switch field {
		case 2:
			raw = assignBoolVarint(raw, wire, &s.LastSetReplyConfigOK)
		case 54:
			raw = assignIntVarint(raw, wire, &s.LastSetReplyACChargeLimit)
		case 43:
			value, next, ok := readBytes(raw)
			if !ok {
				s.UnsupportedMessages++
				return s
			}
			backup := decodeEnergyBackup(value)
			if backup.BackupReserveEnabled != nil {
				s.LastSetReplyBackupReserveEnabled = backup.BackupReserveEnabled
			}
			if backup.BackupReserveSoc != nil {
				s.LastSetReplyBackupReserveSoc = backup.BackupReserveSoc
			}
			raw = next
		default:
			var skipped bool
			raw, skipped = skipValue(wire, raw)
			if !skipped {
				s.UnsupportedMessages++
				return s
			}
		}
	}
	s.DecodedMessages = 1
	return s
}

func decodeEnergyBackup(raw []byte) Status {
	var s Status
	for len(raw) > 0 {
		field, wire, rest, ok := readTag(raw)
		if !ok {
			s.UnsupportedMessages++
			return s
		}
		raw = rest
		switch field {
		case 1:
			raw = assignBoolVarint(raw, wire, &s.BackupReserveEnabled)
		case 2:
			raw = assignIntVarint(raw, wire, &s.BackupReserveSoc)
		default:
			var skipped bool
			raw, skipped = skipValue(wire, raw)
			if !skipped {
				s.UnsupportedMessages++
				return s
			}
		}
	}
	return s
}

func assignFloat(raw []byte, wire int, target **int) []byte {
	if wire != wireFixed32 || len(raw) < 4 {
		next, skipped := skipValue(wire, raw)
		if !skipped {
			return nil
		}
		return next
	}
	bits := uint32(raw[0]) | uint32(raw[1])<<8 | uint32(raw[2])<<16 | uint32(raw[3])<<24
	value := int(math.Round(float64(math.Float32frombits(bits))))
	*target = &value
	return raw[4:]
}

func assignIntVarint(raw []byte, wire int, target **int) []byte {
	if wire != wireVarint {
		next, skipped := skipValue(wire, raw)
		if !skipped {
			return nil
		}
		return next
	}
	value, next, ok := readVarint(raw)
	if !ok {
		return nil
	}
	intValue := int(value)
	*target = &intValue
	return next
}

func assignBoolVarint(raw []byte, wire int, target **bool) []byte {
	if wire != wireVarint {
		next, skipped := skipValue(wire, raw)
		if !skipped {
			return nil
		}
		return next
	}
	value, next, ok := readVarint(raw)
	if !ok {
		return nil
	}
	boolValue := value != 0
	*target = &boolValue
	return next
}

func assignFlowEnabled(raw []byte, wire int, target **bool) []byte {
	if wire != wireVarint {
		next, skipped := skipValue(wire, raw)
		if !skipped {
			return nil
		}
		return next
	}
	value, next, ok := readVarint(raw)
	if !ok {
		return nil
	}
	if value == 14 || value == 4 {
		boolValue := value == 14
		*target = &boolValue
	}
	return next
}

func isBMSHeartbeat(cmdFunc int, cmdID int) bool {
	switch [2]int{cmdFunc, cmdID} {
	case [2]int{3, 1}, [2]int{3, 2}, [2]int{3, 30}, [2]int{3, 50},
		[2]int{32, 1}, [2]int{32, 3}, [2]int{32, 50}, [2]int{32, 51}, [2]int{32, 52},
		[2]int{254, 24}, [2]int{254, 25}, [2]int{254, 26}, [2]int{254, 27}, [2]int{254, 28}, [2]int{254, 29}, [2]int{254, 30}:
		return true
	default:
		return false
	}
}

func xorDecode(raw []byte, seq int) []byte {
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = byte(int(b) ^ seq)
	}
	return out
}

func maybeBase64(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(trimmed)))
	n, err := base64.StdEncoding.Decode(decoded, trimmed)
	if err != nil {
		return raw
	}
	return decoded[:n]
}

const (
	wireVarint  = 0
	wireFixed32 = 5
	wireBytes   = 2
)

func encodeTag(field int, wire int) []byte {
	return encodeVarint(uint64(field<<3 | wire))
}

func encodeVarint(value uint64) []byte {
	out := []byte{}
	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			out = append(out, b|0x80)
			continue
		}
		return append(out, b)
	}
}

func readTag(raw []byte) (field int, wire int, rest []byte, ok bool) {
	value, rest, ok := readVarint(raw)
	if !ok {
		return 0, 0, nil, false
	}
	return int(value >> 3), int(value & 0x7), rest, true
}

func readVarint(raw []byte) (uint64, []byte, bool) {
	var value uint64
	for i := 0; i < len(raw) && i < 10; i++ {
		b := raw[i]
		value |= uint64(b&0x7f) << (7 * i)
		if b < 0x80 {
			return value, raw[i+1:], true
		}
	}
	return 0, nil, false
}

func readBytes(raw []byte) ([]byte, []byte, bool) {
	length, rest, ok := readVarint(raw)
	if !ok || length > uint64(len(rest)) {
		return nil, nil, false
	}
	n := int(length)
	return rest[:n], rest[n:], true
}

func skipValue(wire int, raw []byte) ([]byte, bool) {
	switch wire {
	case wireVarint:
		_, rest, ok := readVarint(raw)
		return rest, ok
	case wireBytes:
		_, rest, ok := readBytes(raw)
		return rest, ok
	case wireFixed32:
		if len(raw) < 4 {
			return nil, false
		}
		return raw[4:], true
	default:
		return nil, false
	}
}
