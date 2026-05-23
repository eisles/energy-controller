package ecoflowdelta3

type Status struct {
	DeviceType                       string `json:"deviceType"`
	DeviceSN                         string `json:"deviceSN"`
	BMSBatterySoc                    *int   `json:"bmsBatterySoc,omitempty"`
	CMSBatterySoc                    *int   `json:"cmsBatterySoc,omitempty"`
	InputW                           *int   `json:"inputW,omitempty"`
	OutputW                          *int   `json:"outputW,omitempty"`
	ACInW                            *int   `json:"acInW,omitempty"`
	ACOutW                           *int   `json:"acOutW,omitempty"`
	PVInW                            *int   `json:"pvInW,omitempty"`
	ACChargeLimitW                   *int   `json:"acChargeLimitW,omitempty"`
	MaxChargeSoc                     *int   `json:"maxChargeSoc,omitempty"`
	MinDischargeSoc                  *int   `json:"minDischargeSoc,omitempty"`
	BackupReserveSoc                 *int   `json:"backupReserveSoc,omitempty"`
	BackupReserveEnabled             *bool  `json:"backupReserveEnabled,omitempty"`
	GridBypassDisabled               *bool  `json:"gridBypassDisabled,omitempty"`
	ACOutputEnabled                  *bool  `json:"acOutputEnabled,omitempty"`
	DCOutputEnabled                  *bool  `json:"dcOutputEnabled,omitempty"`
	USBOutputEnabled                 *bool  `json:"usbOutputEnabled,omitempty"`
	XBoostEnabled                    *bool  `json:"xboostEnabled,omitempty"`
	OutputPowerOffMemory             *bool  `json:"outputPowerOffMemory,omitempty"`
	ChargingState                    *int   `json:"chargingState,omitempty"`
	BMSChargingState                 *int   `json:"bmsChargingState,omitempty"`
	CMSChargingState                 *int   `json:"cmsChargingState,omitempty"`
	PCSWorkMode                      *int   `json:"pcsWorkMode,omitempty"`
	DecodedMessages                  int    `json:"decodedMessages"`
	UnsupportedMessages              int    `json:"unsupportedMessages"`
	LastSetReplyConfigOK             *bool  `json:"lastSetReplyConfigOk,omitempty"`
	LastSetReplyACChargeLimit        *int   `json:"lastSetReplyAcChargeLimitW,omitempty"`
	LastSetReplyBackupReserveSoc     *int   `json:"lastSetReplyBackupReserveSoc,omitempty"`
	LastSetReplyBackupReserveEnabled *bool  `json:"lastSetReplyBackupReserveEnabled,omitempty"`
	LastSetReplySeq                  *int   `json:"lastSetReplySeq,omitempty"`
}

func (s *Status) merge(other Status) {
	if other.BMSBatterySoc != nil {
		s.BMSBatterySoc = other.BMSBatterySoc
	}
	if other.CMSBatterySoc != nil {
		s.CMSBatterySoc = other.CMSBatterySoc
	}
	if other.InputW != nil {
		s.InputW = other.InputW
	}
	if other.OutputW != nil {
		s.OutputW = other.OutputW
	}
	if other.ACInW != nil {
		s.ACInW = other.ACInW
	}
	if other.ACOutW != nil {
		s.ACOutW = other.ACOutW
	}
	if other.PVInW != nil {
		s.PVInW = other.PVInW
	}
	if other.ACChargeLimitW != nil {
		s.ACChargeLimitW = other.ACChargeLimitW
	}
	if other.MaxChargeSoc != nil {
		s.MaxChargeSoc = other.MaxChargeSoc
	}
	if other.MinDischargeSoc != nil {
		s.MinDischargeSoc = other.MinDischargeSoc
	}
	if other.BackupReserveSoc != nil {
		s.BackupReserveSoc = other.BackupReserveSoc
	}
	if other.BackupReserveEnabled != nil {
		s.BackupReserveEnabled = other.BackupReserveEnabled
	}
	if other.GridBypassDisabled != nil {
		s.GridBypassDisabled = other.GridBypassDisabled
	}
	if other.ACOutputEnabled != nil {
		s.ACOutputEnabled = other.ACOutputEnabled
	}
	if other.DCOutputEnabled != nil {
		s.DCOutputEnabled = other.DCOutputEnabled
	}
	if other.USBOutputEnabled != nil {
		s.USBOutputEnabled = other.USBOutputEnabled
	}
	if other.XBoostEnabled != nil {
		s.XBoostEnabled = other.XBoostEnabled
	}
	if other.OutputPowerOffMemory != nil {
		s.OutputPowerOffMemory = other.OutputPowerOffMemory
	}
	if other.ChargingState != nil {
		s.ChargingState = other.ChargingState
	}
	if other.BMSChargingState != nil {
		s.BMSChargingState = other.BMSChargingState
	}
	if other.CMSChargingState != nil {
		s.CMSChargingState = other.CMSChargingState
	}
	if other.PCSWorkMode != nil {
		s.PCSWorkMode = other.PCSWorkMode
	}
	if other.LastSetReplyConfigOK != nil {
		s.LastSetReplyConfigOK = other.LastSetReplyConfigOK
	}
	if other.LastSetReplyACChargeLimit != nil {
		s.LastSetReplyACChargeLimit = other.LastSetReplyACChargeLimit
	}
	if other.LastSetReplyBackupReserveSoc != nil {
		s.LastSetReplyBackupReserveSoc = other.LastSetReplyBackupReserveSoc
	}
	if other.LastSetReplyBackupReserveEnabled != nil {
		s.LastSetReplyBackupReserveEnabled = other.LastSetReplyBackupReserveEnabled
	}
	if other.LastSetReplySeq != nil {
		s.LastSetReplySeq = other.LastSetReplySeq
	}
	s.DecodedMessages += other.DecodedMessages
	s.UnsupportedMessages += other.UnsupportedMessages
}
