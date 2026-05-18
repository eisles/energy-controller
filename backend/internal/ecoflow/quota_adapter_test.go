package ecoflow

import "testing"

func TestBatteryStatusFromQuotas(t *testing.T) {
	status, err := BatteryStatusFromQuotas(map[string]any{
		"cmsBattSoc":              92.87458,
		"powInSumW":               0.0,
		"powOutSumW":              323.0,
		"plugInInfoAcInChgPowMax": 1500.0,
	})
	if err != nil {
		t.Fatalf("BatteryStatusFromQuotas failed: %v", err)
	}
	if status.Soc != 93 || status.InputW != 0 || status.OutputW != 323 || status.ACChargeLimitW != 1500 || !status.IsOnline {
		t.Fatalf("status = %+v, want soc=93 input=0 output=323 acLimit=1500 online=true", status)
	}
}

func TestBatteryStatusFromQuotasRequiresSoc(t *testing.T) {
	if _, err := BatteryStatusFromQuotas(map[string]any{"powInSumW": 100.0}); err == nil {
		t.Fatal("BatteryStatusFromQuotas returned nil error without SOC")
	}
}
