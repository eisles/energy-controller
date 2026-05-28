package ecoflowprivate

import "testing"

func TestBuildTopics(t *testing.T) {
	got := BuildTopics("user-1", "sn-1")
	if got.Get != "/app/user-1/sn-1/thing/property/get" {
		t.Fatalf("Get = %q", got.Get)
	}
	if got.GetReply != "/app/user-1/sn-1/thing/property/get_reply" {
		t.Fatalf("GetReply = %q", got.GetReply)
	}
	if got.Set != "/app/user-1/sn-1/thing/property/set" {
		t.Fatalf("Set = %q", got.Set)
	}
	if got.Data != "/app/device/property/sn-1" {
		t.Fatalf("Data = %q", got.Data)
	}
}
