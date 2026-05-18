package ecoflow

import (
	"context"
	"testing"
)

func TestMockWriteClientRecordsCommands(t *testing.T) {
	client := NewMockWriteClient()

	if err := client.SetACChargePower(context.Background(), 1000); err != nil {
		t.Fatalf("SetACChargePower failed: %v", err)
	}
	if err := client.StopOrMinimizeCharging(context.Background()); err != nil {
		t.Fatalf("StopOrMinimizeCharging failed: %v", err)
	}

	commands := client.Snapshot()
	if len(commands) != 2 {
		t.Fatalf("len(commands) = %d, want 2", len(commands))
	}
	if commands[0].Name != "set_ac_charge_power" || commands[0].Watts != 1000 {
		t.Fatalf("first command = %+v, want set_ac_charge_power 1000", commands[0])
	}
	if commands[1].Name != "stop_or_minimize_charging" || commands[1].Watts != 0 {
		t.Fatalf("second command = %+v, want stop_or_minimize_charging 0", commands[1])
	}
}

func TestMockWriteClientRejectsNonPositiveChargePower(t *testing.T) {
	client := NewMockWriteClient()

	if err := client.SetACChargePower(context.Background(), 0); err == nil {
		t.Fatal("SetACChargePower returned nil, want error")
	}
	if commands := client.Snapshot(); len(commands) != 0 {
		t.Fatalf("len(commands) = %d, want 0", len(commands))
	}
}
