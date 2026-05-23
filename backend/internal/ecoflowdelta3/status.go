package ecoflowdelta3

type Status struct {
	DeviceType                string `json:"deviceType"`
	DeviceSN                  string `json:"deviceSN"`
	BMSBatterySoc             *int   `json:"bmsBatterySoc,omitempty"`
	CMSBatterySoc             *int   `json:"cmsBatterySoc,omitempty"`
	InputW                    *int   `json:"inputW,omitempty"`
	OutputW                   *int   `json:"outputW,omitempty"`
	ACInW                     *int   `json:"acInW,omitempty"`
	ACOutW                    *int   `json:"acOutW,omitempty"`
	PVInW                     *int   `json:"pvInW,omitempty"`
	ACChargeLimitW            *int   `json:"acChargeLimitW,omitempty"`
	BackupReserveSoc          *int   `json:"backupReserveSoc,omitempty"`
	BackupReserveEnabled      *bool  `json:"backupReserveEnabled,omitempty"`
	ChargingState             *int   `json:"chargingState,omitempty"`
	DecodedMessages           int    `json:"decodedMessages"`
	UnsupportedMessages       int    `json:"unsupportedMessages"`
	LastSetReplyConfigOK      *bool  `json:"lastSetReplyConfigOk,omitempty"`
	LastSetReplyACChargeLimit *int   `json:"lastSetReplyAcChargeLimitW,omitempty"`
	LastSetReplySeq           *int   `json:"lastSetReplySeq,omitempty"`
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
	if other.BackupReserveSoc != nil {
		s.BackupReserveSoc = other.BackupReserveSoc
	}
	if other.BackupReserveEnabled != nil {
		s.BackupReserveEnabled = other.BackupReserveEnabled
	}
	if other.ChargingState != nil {
		s.ChargingState = other.ChargingState
	}
	if other.LastSetReplyConfigOK != nil {
		s.LastSetReplyConfigOK = other.LastSetReplyConfigOK
	}
	if other.LastSetReplyACChargeLimit != nil {
		s.LastSetReplyACChargeLimit = other.LastSetReplyACChargeLimit
	}
	if other.LastSetReplySeq != nil {
		s.LastSetReplySeq = other.LastSetReplySeq
	}
	s.DecodedMessages += other.DecodedMessages
	s.UnsupportedMessages += other.UnsupportedMessages
}
