package ecoflowprivate

import (
	"encoding/base64"
	"encoding/hex"
	"math"
	"testing"
)

func TestBuildGetSnapshotPayload(t *testing.T) {
	payload := BuildGetSnapshotPayload(123)
	headers, err := decodeHeaderMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(headers) != 1 {
		t.Fatalf("headers = %d, want 1", len(headers))
	}
	header := headers[0]
	if header.Src != 32 || header.Dest != 32 || header.Seq != 123 || header.From != "HomeAssistant" {
		t.Fatalf("header = %#v", header)
	}
	if len(header.PData) != 0 {
		t.Fatalf("pdata len = %d, want 0", len(header.PData))
	}
}

func TestBuildSetACChargePowerPayloadIncludesCompanionField(t *testing.T) {
	payload, err := BuildSetACChargePowerPayload("SN123", 100, 9)
	if err != nil {
		t.Fatal(err)
	}
	headers, err := decodeHeaderMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	header := headers[0]
	if header.CmdFunc != 254 || header.CmdID != 17 || header.DeviceSN != "SN123" {
		t.Fatalf("header = %#v", header)
	}
	if got := hex.EncodeToString(header.PData); got != "b00364e80700" {
		t.Fatalf("pdata hex = %s, want b00364e80700", got)
	}
}

func TestBuildSetBackupReservePayload(t *testing.T) {
	payload, err := BuildSetBackupReservePayload("SN123", 30, 9)
	if err != nil {
		t.Fatal(err)
	}
	headers, err := decodeHeaderMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(headers[0].PData); got != "da02040801101e" {
		t.Fatalf("pdata hex = %s, want da02040801101e", got)
	}
}

func TestBuildSetGridBypassDisabledPayload(t *testing.T) {
	payload, err := BuildSetGridBypassDisabledPayload("SN123", true, 9)
	if err != nil {
		t.Fatal(err)
	}
	headers, err := decodeHeaderMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(headers[0].PData); got != "d00101" {
		t.Fatalf("pdata hex = %s, want d00101", got)
	}
}

func TestBuildSetMinDischargeSocPayload(t *testing.T) {
	payload, err := BuildSetMinDischargeSocPayload("SN123", 10, 9)
	if err != nil {
		t.Fatal(err)
	}
	headers, err := decodeHeaderMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(headers[0].PData); got != "90020a" {
		t.Fatalf("pdata hex = %s, want 90020a", got)
	}
}

func TestBuildSetMaxChargeSocPayload(t *testing.T) {
	payload, err := BuildSetMaxChargeSocPayload("SN123", 95, 9)
	if err != nil {
		t.Fatal(err)
	}
	headers, err := decodeHeaderMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(headers[0].PData); got != "88025f" {
		t.Fatalf("pdata hex = %s, want 88025f", got)
	}
}

func TestBuildSetEnergyBackupEnabledPayload(t *testing.T) {
	payload, err := BuildSetEnergyBackupEnabledPayload("SN123", false, 25, 9)
	if err != nil {
		t.Fatal(err)
	}
	headers, err := decodeHeaderMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(headers[0].PData); got != "da020408001019" {
		t.Fatalf("pdata hex = %s, want da020408001019", got)
	}
}

func TestDecodeSnapshotDisplayUpload(t *testing.T) {
	display := []byte{}
	display = appendFloatField(display, 3, 123.4)
	display = appendFloatField(display, 4, 456.2)
	display = appendIntField(display, 7, 1)
	display = appendIntField(display, 8, 30)
	display = appendIntField(display, 13, 14)
	display = appendIntField(display, 25, 1)
	display = appendIntField(display, 33, 4)
	display = appendFloatField(display, 54, 400)
	display = appendIntField(display, 76, 1)
	display = appendIntField(display, 146, 1)
	display = appendIntField(display, 147, 1)
	display = appendIntField(display, 209, 100)
	display = appendFloatField(display, 242, 71.6)
	display = appendFloatField(display, 262, 72.2)
	display = appendIntField(display, 270, 95)
	display = appendIntField(display, 271, 10)
	display = appendIntField(display, 281, 2)
	display = appendIntField(display, 282, 3)
	display = appendFloatField(display, 361, 88.8)
	display = appendIntField(display, 367, 14)
	display = appendFloatField(display, 368, 401.1)
	display = appendIntField(display, 971, 4)
	display = appendIntField(display, 1539, 1)
	payload := encodeHeaderMessage(delta3Header{PData: display, Src: 2, CmdFunc: 254, CmdID: 21, Seq: 1})

	status, err := DecodeSnapshot("DELTA_3", "SN123", payload)
	if err != nil {
		t.Fatal(err)
	}
	assertIntPtr(t, "InputW", status.InputW, 123)
	assertIntPtr(t, "OutputW", status.OutputW, 456)
	assertIntPtr(t, "ACInW", status.ACInW, 400)
	assertIntPtr(t, "ACOutW", status.ACOutW, 401)
	assertIntPtr(t, "PVInW", status.PVInW, 89)
	assertIntPtr(t, "ACChargeLimitW", status.ACChargeLimitW, 100)
	assertIntPtr(t, "BMSBatterySoc", status.BMSBatterySoc, 72)
	assertIntPtr(t, "CMSBatterySoc", status.CMSBatterySoc, 72)
	assertIntPtr(t, "BackupReserveSoc", status.BackupReserveSoc, 30)
	assertIntPtr(t, "MaxChargeSoc", status.MaxChargeSoc, 95)
	assertIntPtr(t, "MinDischargeSoc", status.MinDischargeSoc, 10)
	assertIntPtr(t, "ACOutputProtectionChannel", status.ACOutputProtectionChannel, 1)
	assertIntPtr(t, "ChargingState", status.ChargingState, 2)
	assertIntPtr(t, "BMSChargingState", status.BMSChargingState, 2)
	assertIntPtr(t, "CMSChargingState", status.CMSChargingState, 3)
	if status.BackupReserveEnabled == nil || !*status.BackupReserveEnabled {
		t.Fatalf("BackupReserveEnabled = %v, want true", status.BackupReserveEnabled)
	}
	if status.GridBypassDisabled == nil || !*status.GridBypassDisabled {
		t.Fatalf("GridBypassDisabled = %v, want true", status.GridBypassDisabled)
	}
	if status.ACOutputEnabled == nil || !*status.ACOutputEnabled {
		t.Fatalf("ACOutputEnabled = %v, want true", status.ACOutputEnabled)
	}
	assertBoolPtr(t, "ACOutput1Enabled", status.ACOutput1Enabled, true)
	assertBoolPtr(t, "ACOutput2Enabled", status.ACOutput2Enabled, false)
	if status.DCOutputEnabled == nil || *status.DCOutputEnabled {
		t.Fatalf("DCOutputEnabled = %v, want false", status.DCOutputEnabled)
	}
	if status.USBOutputEnabled == nil || !*status.USBOutputEnabled {
		t.Fatalf("USBOutputEnabled = %v, want true", status.USBOutputEnabled)
	}
	if status.XBoostEnabled == nil || !*status.XBoostEnabled {
		t.Fatalf("XBoostEnabled = %v, want true", status.XBoostEnabled)
	}
	if status.OutputPowerOffMemory == nil || !*status.OutputPowerOffMemory {
		t.Fatalf("OutputPowerOffMemory = %v, want true", status.OutputPowerOffMemory)
	}
	if status.DecodedMessages != 1 {
		t.Fatalf("DecodedMessages = %d, want 1", status.DecodedMessages)
	}
}

func TestDecodeSnapshotDelta3MaxPlusACGroups(t *testing.T) {
	tests := []struct {
		name        string
		ac1         int
		ac2         int
		protection  int
		wantOverall bool
		wantAC1     bool
		wantAC2     bool
	}{
		{name: "all off", ac1: 4, ac2: 4, protection: 2, wantOverall: false, wantAC1: false, wantAC2: false},
		{name: "ac1 on", ac1: 14, ac2: 4, protection: 1, wantOverall: true, wantAC1: true, wantAC2: false},
		{name: "ac2 on", ac1: 4, ac2: 14, protection: 2, wantOverall: true, wantAC1: false, wantAC2: true},
		{name: "both on", ac1: 14, ac2: 14, protection: 1, wantOverall: true, wantAC1: true, wantAC2: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			display := []byte{}
			display = appendIntField(display, 367, tt.ac1)
			display = appendIntField(display, 971, tt.ac2)
			display = appendIntField(display, 1539, tt.protection)
			payload := encodeHeaderMessage(delta3Header{PData: display, Src: 2, CmdFunc: 254, CmdID: 21, Seq: 1})

			status, err := DecodeSnapshot("DELTA_3_MAX_PLUS", "SN123", payload)
			if err != nil {
				t.Fatal(err)
			}
			assertBoolPtr(t, "ACOutputEnabled", status.ACOutputEnabled, tt.wantOverall)
			assertBoolPtr(t, "ACOutput1Enabled", status.ACOutput1Enabled, tt.wantAC1)
			assertBoolPtr(t, "ACOutput2Enabled", status.ACOutput2Enabled, tt.wantAC2)
			assertIntPtr(t, "ACOutputProtectionChannel", status.ACOutputProtectionChannel, tt.protection)
		})
	}
}

func TestDecodeSnapshotFallsBackToGenericACOutput(t *testing.T) {
	display := appendIntField(nil, 76, 1)
	payload := encodeHeaderMessage(delta3Header{PData: display, Src: 2, CmdFunc: 254, CmdID: 21, Seq: 1})

	status, err := DecodeSnapshot("DELTA_3", "SN123", payload)
	if err != nil {
		t.Fatal(err)
	}
	assertBoolPtr(t, "ACOutputEnabled", status.ACOutputEnabled, true)
	if status.ACOutput1Enabled != nil || status.ACOutput2Enabled != nil {
		t.Fatalf("AC group fields = %v/%v, want nil without split AC telemetry", status.ACOutput1Enabled, status.ACOutput2Enabled)
	}
}

func TestDecodeSnapshotRuntimeUpload(t *testing.T) {
	runtime := appendIntField(nil, 24, 7)
	payload := encodeHeaderMessage(delta3Header{PData: runtime, Src: 2, CmdFunc: 254, CmdID: 22, Seq: 1})

	status, err := DecodeSnapshot("DELTA_3", "SN123", payload)
	if err != nil {
		t.Fatal(err)
	}
	assertIntPtr(t, "PCSWorkMode", status.PCSWorkMode, 7)
	if status.DecodedMessages != 1 {
		t.Fatalf("DecodedMessages = %d, want 1", status.DecodedMessages)
	}
}

func TestDecodeSnapshotSkipsKnownFloatWithUnexpectedWireType(t *testing.T) {
	display := []byte{}
	display = appendIntField(display, 3, 999)
	display = appendFloatField(display, 4, 456.2)
	payload := encodeHeaderMessage(delta3Header{PData: display, Src: 2, CmdFunc: 254, CmdID: 21, Seq: 1})

	status, err := DecodeSnapshot("DELTA_3", "SN123", payload)
	if err != nil {
		t.Fatal(err)
	}
	if status.InputW != nil {
		t.Fatalf("InputW = %d, want nil for unexpected wire type", *status.InputW)
	}
	assertIntPtr(t, "OutputW", status.OutputW, 456)
}

func TestDecodeSnapshotSetReply(t *testing.T) {
	reply := []byte{}
	reply = appendIntField(reply, 2, 1)
	backup := appendIntField(nil, 1, 1)
	backup = appendIntField(backup, 2, 30)
	reply = appendBytesField(reply, 43, backup)
	reply = appendIntField(reply, 54, 100)
	payload := encodeHeaderMessage(delta3Header{PData: reply, Src: 2, CmdFunc: 254, CmdID: 18, Seq: 123})
	status, err := DecodeSnapshot("DELTA_3", "SN123", payload)
	if err != nil {
		t.Fatal(err)
	}
	if status.LastSetReplyConfigOK == nil || !*status.LastSetReplyConfigOK {
		t.Fatalf("LastSetReplyConfigOK = %v, want true", status.LastSetReplyConfigOK)
	}
	assertIntPtr(t, "LastSetReplyACChargeLimit", status.LastSetReplyACChargeLimit, 100)
	assertIntPtr(t, "LastSetReplyBackupReserveSoc", status.LastSetReplyBackupReserveSoc, 30)
	if status.LastSetReplyBackupReserveEnabled == nil || !*status.LastSetReplyBackupReserveEnabled {
		t.Fatalf("LastSetReplyBackupReserveEnabled = %v, want true", status.LastSetReplyBackupReserveEnabled)
	}
	assertIntPtr(t, "LastSetReplySeq", status.LastSetReplySeq, 123)
}

func TestDecodeSnapshotRejectsOversizedByteLength(t *testing.T) {
	raw := append(encodeTag(1, wireBytes), encodeVarint(^uint64(0))...)
	if _, err := DecodeSnapshot("DELTA_3", "SN123", raw); err == nil {
		t.Fatal("DecodeSnapshot error = nil, want oversized length error")
	}
}

func TestDecodeSnapshotAcceptsBase64WithTrailingWhitespace(t *testing.T) {
	payload := BuildGetSnapshotPayload(1)
	encoded := []byte(base64.StdEncoding.EncodeToString(payload) + "\n")
	status, err := DecodeSnapshot("DELTA_3", "SN123", encoded)
	if err != nil {
		t.Fatal(err)
	}
	if status.DeviceSN != "SN123" {
		t.Fatalf("DeviceSN = %q, want SN123", status.DeviceSN)
	}
}

func appendFloatField(out []byte, field int, value float32) []byte {
	out = append(out, encodeTag(field, wireFixed32)...)
	bits := math.Float32bits(value)
	return append(out, byte(bits), byte(bits>>8), byte(bits>>16), byte(bits>>24))
}

func assertIntPtr(t *testing.T, name string, got *int, want int) {
	t.Helper()
	if got == nil || *got != want {
		if got == nil {
			t.Fatalf("%s = nil, want %d", name, want)
		}
		t.Fatalf("%s = %d, want %d", name, *got, want)
	}
}

func assertBoolPtr(t *testing.T, name string, got *bool, want bool) {
	t.Helper()
	if got == nil || *got != want {
		if got == nil {
			t.Fatalf("%s = nil, want %t", name, want)
		}
		t.Fatalf("%s = %t, want %t", name, *got, want)
	}
}
