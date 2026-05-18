package ecoflow

import "fmt"

const candidateACChargePowerParam = "cfgPlugInInfoAcInChgPowMax"

type commandPayload struct {
	SN      string         `json:"sn"`
	CmdID   int            `json:"cmdId"`
	CmdFunc int            `json:"cmdFunc"`
	DirDest int            `json:"dirDest"`
	DirSrc  int            `json:"dirSrc"`
	Dest    int            `json:"dest"`
	NeedAck bool           `json:"needAck"`
	Params  map[string]int `json:"params"`
}

func buildSetACChargePowerPayload(deviceSN string, watts int) (commandPayload, error) {
	if deviceSN == "" {
		return commandPayload{}, fmt.Errorf("EcoFlow device SN is empty")
	}
	if watts <= 0 {
		return commandPayload{}, fmt.Errorf("EcoFlow AC charge power must be positive: %d", watts)
	}
	return newCommandPayload(deviceSN, map[string]int{
		// Confirmed in EcoFlow DELTA Pro 3 Developer docs. Keep this builder
		// disconnected from real PUT until a one-command device validation passes.
		candidateACChargePowerParam: watts,
	}), nil
}

func newCommandPayload(deviceSN string, params map[string]int) commandPayload {
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
