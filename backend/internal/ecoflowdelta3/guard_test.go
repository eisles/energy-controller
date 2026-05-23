package ecoflowdelta3

import (
	"strings"
	"testing"
)

func TestWriteGuardsRequireEveryPrivateWriteGate(t *testing.T) {
	base := WriteGuards{
		MockMode:             false,
		SimulationMode:       false,
		EnableRealControl:    true,
		ConfirmEcoFlowWrite:  ConfirmWriteValue,
		Execute:              true,
		AllowPrivateAPIWrite: true,
		Command:              "set_ac_charge_power",
		DeviceType:           "DELTA_3",
	}
	tests := []struct {
		name    string
		mutate  func(*WriteGuards)
		wantErr string
	}{
		{name: "execute", mutate: func(g *WriteGuards) { g.Execute = false }, wantErr: "--execute"},
		{name: "private write flag", mutate: func(g *WriteGuards) { g.AllowPrivateAPIWrite = false }, wantErr: "--allow-private-api-write"},
		{name: "mock", mutate: func(g *WriteGuards) { g.MockMode = true }, wantErr: "MOCK_MODE"},
		{name: "simulation", mutate: func(g *WriteGuards) { g.SimulationMode = true }, wantErr: "SIMULATION_MODE"},
		{name: "real control", mutate: func(g *WriteGuards) { g.EnableRealControl = false }, wantErr: "ENABLE_REAL_CONTROL"},
		{name: "auto control", mutate: func(g *WriteGuards) { g.AutoControlEnabled = true }, wantErr: "AUTO_CONTROL_ENABLED"},
		{name: "confirmation", mutate: func(g *WriteGuards) { g.ConfirmEcoFlowWrite = "" }, wantErr: "CONFIRM_ECOFLOW_WRITE"},
		{name: "command", mutate: func(g *WriteGuards) { g.Command = "power_off" }, wantErr: "allowlisted"},
		{name: "device", mutate: func(g *WriteGuards) { g.DeviceType = "UNKNOWN" }, wantErr: "unsupported device type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guards := base
			tt.mutate(&guards)
			err := guards.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestWriteGuardsAllowExplicitAutoControlOverlap(t *testing.T) {
	guards := WriteGuards{
		MockMode:                false,
		SimulationMode:          false,
		EnableRealControl:       true,
		AutoControlEnabled:      true,
		AllowAutoControlOverlap: true,
		ConfirmEcoFlowWrite:     ConfirmWriteValue,
		Execute:                 true,
		AllowPrivateAPIWrite:    true,
		Command:                 "set_ac_charge_power",
		DeviceType:              "DELTA_3",
	}
	if err := guards.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestWriteGuardsAllowGridBypassCommand(t *testing.T) {
	guards := WriteGuards{
		MockMode:             false,
		SimulationMode:       false,
		EnableRealControl:    true,
		ConfirmEcoFlowWrite:  ConfirmWriteValue,
		Execute:              true,
		AllowPrivateAPIWrite: true,
		Command:              "set_grid_bypass_disabled",
		DeviceType:           "DELTA_3",
	}
	if err := guards.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil", err)
	}
}

func TestValidateACChargePowerDeviceRanges(t *testing.T) {
	if err := ValidateACChargePower("DELTA_3", 100); err != nil {
		t.Fatalf("DELTA_3 100W rejected: %v", err)
	}
	if err := ValidateACChargePower("DELTA_3_1500", 100); err == nil || !strings.Contains(err.Error(), "200-1500W") {
		t.Fatalf("DELTA_3_1500 100W error = %v, want range error", err)
	}
}
