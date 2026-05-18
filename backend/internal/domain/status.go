package domain

import "time"

type GridPower struct {
	GridW   int `json:"gridW"`
	ImportW int `json:"importW"`
	ExportW int `json:"exportW"`
}

type BatteryStatus struct {
	Soc            int  `json:"batterySoc"`
	InputW         int  `json:"batteryInputW"`
	OutputW        int  `json:"batteryOutputW"`
	ACChargeLimitW int  `json:"acChargeLimitW"`
	IsOnline       bool `json:"isOnline"`
}

type ControlDecision struct {
	ShouldCharge  bool
	TargetChargeW int
	Reason        string
}

type Status struct {
	GridW              int       `json:"gridW"`
	ImportW            int       `json:"importW"`
	ExportW            int       `json:"exportW"`
	BatterySoc         int       `json:"batterySoc"`
	BatteryInputW      int       `json:"batteryInputW"`
	BatteryOutputW     int       `json:"batteryOutputW"`
	ACChargeLimitW     int       `json:"acChargeLimitW"`
	TargetChargeW      int       `json:"targetChargeW"`
	State              string    `json:"state"`
	Mode               string    `json:"mode"`
	LastDecisionReason string    `json:"lastDecisionReason"`
	LastError          *string   `json:"lastError"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type PowerLog struct {
	ID             int64     `json:"id"`
	MeasuredAt     time.Time `json:"measuredAt"`
	GridW          int       `json:"gridW"`
	ImportW        int       `json:"importW"`
	ExportW        int       `json:"exportW"`
	BatterySoc     *int      `json:"batterySoc"`
	BatteryInputW  *int      `json:"batteryInputW"`
	BatteryOutputW *int      `json:"batteryOutputW"`
	ACChargeLimitW *int      `json:"acChargeLimitW"`
	TargetChargeW  int       `json:"targetChargeW"`
	ActualCommandW *int      `json:"actualCommandW"`
	DecisionReason string    `json:"decisionReason"`
	Mode           string    `json:"mode"`
	CommandSent    bool      `json:"commandSent"`
	ErrorMessage   *string   `json:"errorMessage"`
	CreatedAt      time.Time `json:"createdAt"`
}
