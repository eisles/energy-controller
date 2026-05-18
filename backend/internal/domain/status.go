package domain

import "time"

type GridPower struct {
	GridW   int `json:"gridW"`
	ImportW int `json:"importW"`
	ExportW int `json:"exportW"`
}

type BatteryStatus struct {
	Soc      int  `json:"batterySoc"`
	InputW   int  `json:"batteryInputW"`
	OutputW  int  `json:"batteryOutputW"`
	IsOnline bool `json:"isOnline"`
}

type Status struct {
	GridW              int       `json:"gridW"`
	ImportW            int       `json:"importW"`
	ExportW            int       `json:"exportW"`
	BatterySoc         int       `json:"batterySoc"`
	BatteryInputW      int       `json:"batteryInputW"`
	BatteryOutputW     int       `json:"batteryOutputW"`
	TargetChargeW      int       `json:"targetChargeW"`
	State              string    `json:"state"`
	Mode               string    `json:"mode"`
	LastDecisionReason string    `json:"lastDecisionReason"`
	LastError          *string   `json:"lastError"`
	UpdatedAt          time.Time `json:"updatedAt"`
}
