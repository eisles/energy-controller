package ecoflow

import "fmt"

const (
	candidateACChargePowerParam = "cfgPlugInInfoAcInChgPowMax"
	candidateBackupReserveParam = "cfgBackupReverseSoc"
	candidateTOUModeParam       = "cfgEnergyStrategyOperateMode.operateTouModeOpen"
	candidateEnergyModeParam    = "cfgEnergyStrategyOperateMode"
)

type commandPayload struct {
	SN      string         `json:"sn"`
	CmdID   int            `json:"cmdId"`
	CmdFunc int            `json:"cmdFunc"`
	DirDest int            `json:"dirDest"`
	DirSrc  int            `json:"dirSrc"`
	Dest    int            `json:"dest"`
	NeedAck bool           `json:"needAck"`
	Params  map[string]any `json:"params"`
}

func buildSetACChargePowerPayload(deviceSN string, watts int) (commandPayload, error) {
	if deviceSN == "" {
		return commandPayload{}, fmt.Errorf("EcoFlow device SN is empty")
	}
	if watts <= 0 {
		return commandPayload{}, fmt.Errorf("EcoFlow AC charge power must be positive: %d", watts)
	}
	return newCommandPayload(deviceSN, map[string]any{
		// Confirmed in EcoFlow DELTA Pro 3 Developer docs. Keep this builder
		// disconnected from real PUT until a one-command device validation passes.
		candidateACChargePowerParam: watts,
	}), nil
}

func buildSetBackupReservePayload(deviceSN string, percent int) (commandPayload, error) {
	if deviceSN == "" {
		return commandPayload{}, fmt.Errorf("EcoFlow device SN is empty")
	}
	if percent < 0 || percent > 100 {
		return commandPayload{}, fmt.Errorf("EcoFlow backup reserve SOC must be 0-100: %d", percent)
	}
	return newCommandPayload(deviceSN, map[string]any{
		// Candidate inferred from EcoFlow quota naming and adjacent EcoFlow docs.
		// Keep use behind the same real-control guards and validate with one-shot only.
		candidateBackupReserveParam: percent,
	}), nil
}

func buildSetTOUModePayload(deviceSN string, enabled bool) (commandPayload, error) {
	if deviceSN == "" {
		return commandPayload{}, fmt.Errorf("EcoFlow device SN is empty")
	}
	return newCommandPayload(deviceSN, map[string]any{
		// Candidate inferred from observed DELTA Pro 3 read quota naming and
		// EcoFlow energy strategy control examples. Validate with one-shot only.
		candidateEnergyModeParam: map[string]any{
			"operateTouModeOpen":                 enabled,
			"operateSelfPoweredOpen":             false,
			"operateScheduledOpen":               false,
			"operateIntelligentScheduleModeOpen": false,
		},
	}), nil
}

func newCommandPayload(deviceSN string, params map[string]any) commandPayload {
	return commandPayload{
		SN:      deviceSN,
		CmdID:   17,
		CmdFunc: 254,
		DirDest: 1,
		DirSrc:  1,
		Dest:    2,
		NeedAck: true,
		Params:  params,
	}
}
