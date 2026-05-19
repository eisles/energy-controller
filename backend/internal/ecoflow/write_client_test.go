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
	if err := client.SetBackupReserveSoc(context.Background(), 82); err != nil {
		t.Fatalf("SetBackupReserveSoc failed: %v", err)
	}
	if err := client.SetTOUMode(context.Background(), false); err != nil {
		t.Fatalf("SetTOUMode failed: %v", err)
	}
	if err := client.StopOrMinimizeCharging(context.Background()); err != nil {
		t.Fatalf("StopOrMinimizeCharging failed: %v", err)
	}

	commands := client.Snapshot()
	if len(commands) != 4 {
		t.Fatalf("len(commands) = %d, want 4", len(commands))
	}
	if commands[0].Name != "set_ac_charge_power" || commands[0].Watts != 1000 {
		t.Fatalf("first command = %+v, want set_ac_charge_power 1000", commands[0])
	}
	if commands[1].Name != "set_backup_reserve_soc" || commands[1].Watts != 82 {
		t.Fatalf("second command = %+v, want set_backup_reserve_soc 82", commands[1])
	}
	if commands[2].Name != "set_tou_mode" || commands[2].Watts != 0 {
		t.Fatalf("third command = %+v, want set_tou_mode 0", commands[2])
	}
	if commands[3].Name != "stop_or_minimize_charging" || commands[3].Watts != 0 {
		t.Fatalf("fourth command = %+v, want stop_or_minimize_charging 0", commands[3])
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

func TestMockWriteClientRejectsInvalidBackupReserveSoc(t *testing.T) {
	client := NewMockWriteClient()

	if err := client.SetBackupReserveSoc(context.Background(), 101); err == nil {
		t.Fatal("SetBackupReserveSoc returned nil, want error")
	}
	if commands := client.Snapshot(); len(commands) != 0 {
		t.Fatalf("len(commands) = %d, want 0", len(commands))
	}
}
