package ecoflow

import "testing"

func TestSignIncludesSortedParamsAndAuthFields(t *testing.T) {
	got := sign(map[string]string{"sn": "device-sn"}, "access-key", "secret-key", "123456", "1700000000000")
	want := "5230696a09e852661a82b1c3d3169e1ea981fa7c2f9998a73f2b9460e3e1aee3"
	if got != want {
		t.Fatalf("sign = %s, want %s", got, want)
	}
}
