package domain

import "time"

type ChargingDevice struct {
	ID                    int64     `json:"id,omitempty"`
	Name                  string    `json:"name"`
	Kind                  string    `json:"kind"`
	Provider              string    `json:"provider"`
	Role                  string    `json:"role"`
	CredentialRef         string    `json:"credentialRef"`
	DeviceSN              string    `json:"deviceSn"`
	DeviceType            string    `json:"deviceType"`
	StatusSource          string    `json:"statusSource"`
	Enabled               bool      `json:"enabled"`
	ControlEnabled        bool      `json:"controlEnabled"`
	Priority              int       `json:"priority"`
	MinChargeW            int       `json:"minChargeW"`
	MaxChargeW            int       `json:"maxChargeW"`
	ChargeStepW           int       `json:"chargeStepW"`
	CapacityWh            int       `json:"capacityWh"`
	TargetSoc             int       `json:"targetSoc"`
	ReserveSoc            int       `json:"reserveSoc"`
	SupportsSocRead       bool      `json:"supportsSocRead"`
	SupportsACChargeLimit bool      `json:"supportsAcChargeLimit"`
	SupportsOnOff         bool      `json:"supportsOnOff"`
	Notes                 string    `json:"notes"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}
