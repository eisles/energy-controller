package ecoflowdeveloper

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestCycleStatusFromQuotaPayloadReadsNamedCycleKeys(t *testing.T) {
	status, err := CycleStatusFromQuotaPayload([]byte(`{"params":{"cycles":12,"soc":88}}`))
	if err != nil {
		t.Fatal(err)
	}
	if status.CycleCount == nil || *status.CycleCount != 12 || status.CycleCountSource != CycleCountSource || status.Key != "cycles" {
		t.Fatalf("status = %+v, want cycles=12 from Developer MQTT quota", status)
	}
	if status.QuotaKeyCount != 2 {
		t.Fatalf("QuotaKeyCount = %d, want 2", status.QuotaKeyCount)
	}
	if len(status.CycleCandidates) != 1 || status.CycleCandidates[0].Key != "cycles" {
		t.Fatalf("CycleCandidates = %+v, want cycles candidate", status.CycleCandidates)
	}
}

func TestCycleStatusFromQuotaPayloadReadsNestedCycleKeys(t *testing.T) {
	status, err := CycleStatusFromQuotaPayload([]byte(`{"params":{"bms_bmsStatus":{"cycles":34}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if status.CycleCount == nil || *status.CycleCount != 34 || status.Key != "bms_bmsStatus.cycles" {
		t.Fatalf("status = %+v, want nested bms_bmsStatus cycle count", status)
	}
}

func TestCycleStatusFromQuotaPayloadReadsDirectPayload(t *testing.T) {
	status, err := CycleStatusFromQuotaPayload([]byte(`{"bmsBattCycles":"56"}`))
	if err != nil {
		t.Fatal(err)
	}
	if status.CycleCount == nil || *status.CycleCount != 56 || status.Key != "bmsBattCycles" {
		t.Fatalf("status = %+v, want bmsBattCycles=56", status)
	}
}

func TestCycleStatusFromQuotaPayloadKeepsMissingCycleNil(t *testing.T) {
	status, err := CycleStatusFromQuotaPayload([]byte(`{"params":{"soc":98}}`))
	if err != nil {
		t.Fatal(err)
	}
	if status.CycleCount != nil || status.CycleCountSource != "" || status.Key != "" {
		t.Fatalf("status = %+v, want no cycle count", status)
	}
}

func TestCycleStatusFromQuotaPayloadReportsCycSohAsCandidateOnly(t *testing.T) {
	status, err := CycleStatusFromQuotaPayload([]byte(`{"params":{"bms_bmsStatus":{"cycSoh":98},"bms_slave":{"cycSoh":"7"},"soc":88}}`))
	if err != nil {
		t.Fatal(err)
	}
	if status.CycleCount != nil || status.CycleCountSource != "" || status.Key != "" {
		t.Fatalf("status = %+v, want cycSoh only as candidate", status)
	}
	if len(status.CycleCandidates) != 2 {
		t.Fatalf("CycleCandidates = %+v, want two cycSoh candidates", status.CycleCandidates)
	}
	if status.CycleCandidates[0].Key != "bms_bmsStatus.cycSoh" || status.CycleCandidates[1].Key != "bms_slave.cycSoh" {
		t.Fatalf("CycleCandidates = %+v, want flattened cycSoh paths", status.CycleCandidates)
	}
}

func TestCycleStatusFromQuotaPayloadReportsNestedNamedCycleCandidate(t *testing.T) {
	status, err := CycleStatusFromQuotaPayload([]byte(`{"params":{"bms_bmsStatus":{"cycles":34}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(status.CycleCandidates) != 1 || status.CycleCandidates[0].Key != "bms_bmsStatus.cycles" {
		t.Fatalf("CycleCandidates = %+v, want nested named cycle candidate", status.CycleCandidates)
	}
}

func TestCycleStatusFromQuotaPayloadReportsDiagnosticKeys(t *testing.T) {
	status, err := CycleStatusFromQuotaPayload([]byte(`{
		"params": {
			"soc": 88,
			"cycleSNum": 5,
			"cyclesNum": 6,
			"sn": 123456,
			"device_sn": 123456,
			"deviceSn": 123456,
			"productSn": 123456,
			"bms_bmsStatus": {
				"bmssn": 123456,
				"bmsSn": 123456,
				"soh": 99,
				"cycSoh": 7,
				"bmsNaN": "NaN",
				"bmsInf": "+Inf",
				"bmsLeadingZero": "01",
				"bmsPlus": "+1",
				"bmsPoint": ".5",
				"bmsTrailingPoint": "1.",
				"serial": "hidden",
				"access_key": "hidden",
				"client_id": "hidden",
				"device": "hidden",
				"user_id": "hidden",
				"nested": {"cycleTotal": 4}
			},
			"packSn": 123456,
			"pack": {"cycleHealth": true},
			"other": {"temperature": 33}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if status.CycleCount != nil {
		t.Fatalf("status = %+v, want diagnostic keys not to set cycle count", status)
	}
	got := diagnosticKeyMap(status.DiagnosticKeys)
	for key, want := range map[string]string{
		"bms_bmsStatus.cycSoh":            "7",
		"bms_bmsStatus.bmsLeadingZero":    "1",
		"bms_bmsStatus.bmsPlus":           "1",
		"bms_bmsStatus.bmsPoint":          "0.5",
		"bms_bmsStatus.bmsTrailingPoint":  "1",
		"bms_bmsStatus.nested.cycleTotal": "4",
		"bms_bmsStatus.soh":               "99",
		"cycleSNum":                       "5",
		"cyclesNum":                       "6",
		"pack.cycleHealth":                "true",
	} {
		if fmt.Sprint(got[key]) != want {
			t.Fatalf("DiagnosticKeys[%s] = %#v, want %#v; all=%+v", key, got[key], want, status.DiagnosticKeys)
		}
	}
	for _, forbidden := range []string{
		"soc",
		"bms_bmsStatus.access_key",
		"bms_bmsStatus.bmsInf",
		"bms_bmsStatus.bmsNaN",
		"bms_bmsStatus.bmsSn",
		"bms_bmsStatus.bmssn",
		"bms_bmsStatus.client_id",
		"bms_bmsStatus.device",
		"bms_bmsStatus.serial",
		"bms_bmsStatus.user_id",
		"device_sn",
		"deviceSn",
		"other.temperature",
		"packSn",
		"productSn",
		"sn",
	} {
		if _, ok := got[forbidden]; ok {
			t.Fatalf("DiagnosticKeys contains %s; all=%+v", forbidden, status.DiagnosticKeys)
		}
	}
}

func TestCycleStatusFromQuotaPayloadPrioritizesCycleDiagnosticsBeforeBMSContext(t *testing.T) {
	params := map[string]any{
		"zz_cycleTotal": 12,
	}
	for i := 0; i < maxDiagnosticKeys+5; i++ {
		params[fmt.Sprintf("bms_%03d", i)] = i
	}
	raw, err := json.Marshal(map[string]any{"params": params})
	if err != nil {
		t.Fatal(err)
	}
	status, err := CycleStatusFromQuotaPayload(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.DiagnosticKeys) != maxDiagnosticKeys {
		t.Fatalf("DiagnosticKeys length = %d, want cap %d", len(status.DiagnosticKeys), maxDiagnosticKeys)
	}
	got := diagnosticKeyMap(status.DiagnosticKeys)
	if fmt.Sprint(got["zz_cycleTotal"]) != "12" {
		t.Fatalf("DiagnosticKeys missing priority cycle key; all=%+v", status.DiagnosticKeys)
	}
}

func diagnosticKeyMap(keys []DiagnosticKey) map[string]any {
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		result[key.Key] = key.Value
	}
	return result
}

func TestCycleStatusFromQuotaPayloadRejectsUncertainCycleValues(t *testing.T) {
	for _, raw := range []string{
		`{"params":{"cycles":-1}}`,
		`{"params":{"cycles":12.5}}`,
		`{"params":{"cycles":"unknown"}}`,
	} {
		status, err := CycleStatusFromQuotaPayload([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if status.CycleCount != nil {
			t.Fatalf("status = %+v, want nil for uncertain value in %s", status, raw)
		}
	}
}

func TestQuotaTopic(t *testing.T) {
	got := QuotaTopic("/acct/", "/SN123/")
	if got != "/open/acct/SN123/quota" {
		t.Fatalf("QuotaTopic = %q", got)
	}
}
