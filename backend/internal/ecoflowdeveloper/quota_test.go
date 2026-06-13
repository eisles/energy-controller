package ecoflowdeveloper

import "testing"

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
