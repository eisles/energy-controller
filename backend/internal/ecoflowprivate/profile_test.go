package ecoflowprivate

import "testing"

func TestProfileForDeviceTypeNormalizesKnownEcoFlowPrivateProfiles(t *testing.T) {
	profile, ok := ProfileForDeviceType(" delta_3_plus ")
	if !ok {
		t.Fatal("ProfileForDeviceType returned ok=false")
	}
	if profile.DeviceType != "DELTA_3_PLUS" || profile.Family != "delta3_series" {
		t.Fatalf("profile = %+v, want normalized DELTA_3_PLUS delta3_series", profile)
	}
	if profile.MinACChargeW != 100 || profile.MaxACChargeW != 1500 {
		t.Fatalf("AC charge range = %d-%d, want 100-1500", profile.MinACChargeW, profile.MaxACChargeW)
	}
	if !profile.SupportedCommand["set_energy_backup_enabled"] {
		t.Fatalf("SupportedCommand missing set_energy_backup_enabled: %+v", profile.SupportedCommand)
	}
}

func TestProfileForDeviceTypeKeepsUnsupportedDevicesUnavailable(t *testing.T) {
	if _, ok := ProfileForDeviceType("UNKNOWN_MODEL"); ok {
		t.Fatal("ProfileForDeviceType returned ok=true for unsupported model")
	}
	if _, ok := RangeForDeviceType("UNKNOWN_MODEL"); ok {
		t.Fatal("RangeForDeviceType returned ok=true for unsupported model")
	}
}
