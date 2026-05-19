package domain

import "time"

type GridPower struct {
	GridW   int `json:"gridW"`
	ImportW int `json:"importW"`
	ExportW int `json:"exportW"`
}

type BatteryStatus struct {
	Soc                 int   `json:"batterySoc"`
	InputW              int   `json:"batteryInputW"`
	OutputW             int   `json:"batteryOutputW"`
	ACChargeLimitW      int   `json:"acChargeLimitW"`
	BackupReserveSoc    *int  `json:"backupReserveSoc,omitempty"`
	EnergyBackupEnabled *bool `json:"energyBackupEnabled,omitempty"`
	TOUModeEnabled      *bool `json:"touModeEnabled,omitempty"`
	FullEnergyWh        *int  `json:"fullEnergyWh,omitempty"`
	IsOnline            bool  `json:"isOnline"`
}

type ControlDecision struct {
	ShouldCharge  bool
	TargetChargeW int
	Reason        string
}

type Status struct {
	GridW               int              `json:"gridW"`
	ImportW             int              `json:"importW"`
	ExportW             int              `json:"exportW"`
	BatterySoc          int              `json:"batterySoc"`
	BatteryInputW       int              `json:"batteryInputW"`
	BatteryOutputW      int              `json:"batteryOutputW"`
	ACChargeLimitW      int              `json:"acChargeLimitW"`
	BackupReserveSoc    *int             `json:"backupReserveSoc,omitempty"`
	EnergyBackupEnabled *bool            `json:"energyBackupEnabled,omitempty"`
	TOUModeEnabled      *bool            `json:"touModeEnabled,omitempty"`
	BatteryFullEnergyWh *int             `json:"batteryFullEnergyWh,omitempty"`
	SurplusPlan         *SurplusPlan     `json:"surplusPlan,omitempty"`
	NightChargePlan     *NightChargePlan `json:"nightChargePlan,omitempty"`
	TargetChargeW       int              `json:"targetChargeW"`
	State               string           `json:"state"`
	Mode                string           `json:"mode"`
	LastDecisionReason  string           `json:"lastDecisionReason"`
	LastError           *string          `json:"lastError"`
	UpdatedAt           time.Time        `json:"updatedAt"`
}

type SurplusPlan struct {
	Mode                        string `json:"mode"`
	NetBatteryW                 int    `json:"netBatteryW"`
	RecommendedACChargeLimitW   int    `json:"recommendedAcChargeLimitW"`
	RecommendedBackupReserveSoc *int   `json:"recommendedBackupReserveSoc,omitempty"`
	ShouldRaiseBackupReserve    bool   `json:"shouldRaiseBackupReserve"`
	ShouldAdjustACChargeLimit   bool   `json:"shouldAdjustAcChargeLimit"`
	WouldWrite                  bool   `json:"wouldWrite"`
	Reason                      string `json:"reason"`
}

type WeatherForecast struct {
	Provider                    string  `json:"provider"`
	Date                        string  `json:"date"`
	WeatherCode                 int     `json:"weatherCode"`
	ShortwaveRadiationMJPerM2   float64 `json:"shortwaveRadiationMjPerM2"`
	SunshineDurationHours       float64 `json:"sunshineDurationHours"`
	CloudCoverMeanPercent       int     `json:"cloudCoverMeanPercent"`
	PrecipitationProbabilityMax int     `json:"precipitationProbabilityMax"`
	PrecipitationSumMM          float64 `json:"precipitationSumMm"`
}

type SolarForecastEstimate struct {
	Forecast                    WeatherForecast `json:"forecast"`
	SolarForecastScore          int             `json:"solarForecastScore"`
	SolarRadiationKWhPerM2      float64         `json:"solarRadiationKwhPerM2"`
	EstimatedPVKWh              float64         `json:"estimatedPvKwh"`
	EstimatedDaytimeLoadKWh     float64         `json:"estimatedDaytimeLoadKwh"`
	EstimatedSurplusKWh         float64         `json:"estimatedSurplusKwh"`
	PVCapacityKW                float64         `json:"pvCapacityKw"`
	PVPerformanceRatio          float64         `json:"pvPerformanceRatio"`
	PrecipitationProbabilityMax int             `json:"precipitationProbabilityMax"`
	PrecipitationSumMM          float64         `json:"precipitationSumMm"`
}

type SolarForecastSummary struct {
	Days     int                     `json:"days"`
	Location WeatherLocation         `json:"location"`
	Items    []SolarForecastEstimate `json:"items"`
	Note     string                  `json:"note"`
}

type WeatherLocation struct {
	Enabled            bool    `json:"enabled"`
	Latitude           float64 `json:"latitude"`
	Longitude          float64 `json:"longitude"`
	Timezone           string  `json:"timezone"`
	PVCapacityKW       float64 `json:"pvCapacityKw"`
	PVPerformanceRatio float64 `json:"pvPerformanceRatio"`
	DailyBaseLoadKWh   float64 `json:"dailyBaseLoadKwh"`
	BatteryCapacityKWh float64 `json:"batteryCapacityKwh"`
	MinimumReserveSoc  int     `json:"minimumReserveSoc"`
}

type DaytimeConsumptionEstimate struct {
	Days                       int                               `json:"days"`
	StartHour                  int                               `json:"startHour"`
	EndHour                    int                               `json:"endHour"`
	SampleCount                int                               `json:"sampleCount"`
	AverageImportKWh           float64                           `json:"averageImportKwh"`
	AverageExportKWh           float64                           `json:"averageExportKwh"`
	AverageBatteryChargeKWh    float64                           `json:"averageBatteryChargeKwh"`
	AverageBatteryDischargeKWh float64                           `json:"averageBatteryDischargeKwh"`
	AverageEstimatedLoadKWh    float64                           `json:"averageEstimatedLoadKwh"`
	SuggestedDailyBaseLoadKWh  float64                           `json:"suggestedDailyBaseLoadKwh"`
	Daily                      []DailyDaytimeConsumptionEstimate `json:"daily"`
}

type EcoFlowLoadEstimate struct {
	Days                         int                        `json:"days"`
	DaytimeStartHour             int                        `json:"daytimeStartHour"`
	DaytimeEndHour               int                        `json:"daytimeEndHour"`
	NightStartHour               int                        `json:"nightStartHour"`
	NightEndHour                 int                        `json:"nightEndHour"`
	SampleCount                  int                        `json:"sampleCount"`
	AverageDaytimeOutputKWh      float64                    `json:"averageDaytimeOutputKwh"`
	AverageShoulderOutputKWh     float64                    `json:"averageShoulderOutputKwh"`
	AverageNightOutputKWh        float64                    `json:"averageNightOutputKwh"`
	AverageDailyOutputKWh        float64                    `json:"averageDailyOutputKwh"`
	AverageDaytimeChargeKWh      float64                    `json:"averageDaytimeChargeKwh"`
	SuggestedDaytimeBaseLoadKWh  float64                    `json:"suggestedDaytimeBaseLoadKwh"`
	SuggestedOvernightReserveKWh float64                    `json:"suggestedOvernightReserveKwh"`
	Daily                        []DailyEcoFlowLoadEstimate `json:"daily"`
	Note                         string                     `json:"note"`
}

type DailyEcoFlowLoadEstimate struct {
	Date              string  `json:"date"`
	SampleCount       int     `json:"sampleCount"`
	DaytimeOutputKWh  float64 `json:"daytimeOutputKwh"`
	ShoulderOutputKWh float64 `json:"shoulderOutputKwh"`
	NightOutputKWh    float64 `json:"nightOutputKwh"`
	DailyOutputKWh    float64 `json:"dailyOutputKwh"`
	DaytimeChargeKWh  float64 `json:"daytimeChargeKwh"`
	DaytimeNetLoadKWh float64 `json:"daytimeNetLoadKwh"`
}

type DailyDaytimeConsumptionEstimate struct {
	Date                string  `json:"date"`
	SampleCount         int     `json:"sampleCount"`
	ImportKWh           float64 `json:"importKwh"`
	ExportKWh           float64 `json:"exportKwh"`
	BatteryChargeKWh    float64 `json:"batteryChargeKwh"`
	BatteryDischargeKWh float64 `json:"batteryDischargeKwh"`
	EstimatedLoadKWh    float64 `json:"estimatedLoadKwh"`
}

type NightChargePlan struct {
	Mode                      string           `json:"mode"`
	SolarForecastScore        int              `json:"solarForecastScore"`
	SolarRadiationKWhPerM2    float64          `json:"solarRadiationKwhPerM2"`
	EstimatedPVKWh            float64          `json:"estimatedPvKwh"`
	EstimatedDaytimeLoadKWh   float64          `json:"estimatedDaytimeLoadKwh"`
	EstimatedSurplusKWh       float64          `json:"estimatedSurplusKwh"`
	BatteryChargeHeadroomKWh  float64          `json:"batteryChargeHeadroomKwh"`
	BatteryCapacitySource     string           `json:"batteryCapacitySource"`
	RecommendedNightTargetSoc int              `json:"recommendedNightTargetSoc"`
	MinimumReserveSoc         int              `json:"minimumReserveSoc"`
	ShouldChargeTonight       bool             `json:"shouldChargeTonight"`
	WouldWrite                bool             `json:"wouldWrite"`
	Reason                    string           `json:"reason"`
	TargetForecast            *WeatherForecast `json:"targetForecast,omitempty"`
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

type EnergyMeterLog struct {
	ID                   int64     `json:"id"`
	MeasuredAt           time.Time `json:"measuredAt"`
	ImportCumulativeKWh  float64   `json:"importCumulativeKwh"`
	ExportCumulativeKWh  float64   `json:"exportCumulativeKwh"`
	ImportDeltaKWh       *float64  `json:"importDeltaKwh"`
	ExportDeltaKWh       *float64  `json:"exportDeltaKwh"`
	Coefficient          int       `json:"coefficient"`
	CumulativeUnit       float64   `json:"cumulativeUnit"`
	RawImportCumulative  string    `json:"rawImportCumulative"`
	RawExportCumulative  string    `json:"rawExportCumulative"`
	ImportValueUpdatedAt time.Time `json:"importValueUpdatedAt"`
	ExportValueUpdatedAt time.Time `json:"exportValueUpdatedAt"`
	CreatedAt            time.Time `json:"createdAt"`
}

type EnergyMeterReading struct {
	MeasuredAt           time.Time
	ImportCumulativeKWh  float64
	ExportCumulativeKWh  float64
	Coefficient          int
	CumulativeUnit       float64
	RawImportCumulative  string
	RawExportCumulative  string
	ImportValueUpdatedAt time.Time
	ExportValueUpdatedAt time.Time
}

type TariffSettings struct {
	PlanName      string     `json:"planName"`
	DayRateYen    float64    `json:"dayRateYen"`
	HomeRateYen   float64    `json:"homeRateYen"`
	NightRateYen  float64    `json:"nightRateYen"`
	ExportRateYen float64    `json:"exportRateYen"`
	Timezone      string     `json:"timezone"`
	EffectiveFrom time.Time  `json:"effectiveFrom"`
	EffectiveTo   *time.Time `json:"effectiveTo,omitempty"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type TariffPlan struct {
	ID            int64      `json:"id"`
	PlanName      string     `json:"planName"`
	DayRateYen    float64    `json:"dayRateYen"`
	HomeRateYen   float64    `json:"homeRateYen"`
	NightRateYen  float64    `json:"nightRateYen"`
	ExportRateYen float64    `json:"exportRateYen"`
	Timezone      string     `json:"timezone"`
	EffectiveFrom time.Time  `json:"effectiveFrom"`
	EffectiveTo   *time.Time `json:"effectiveTo,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type TariffPeriodSummary struct {
	PlanName        string     `json:"planName"`
	Period          string     `json:"period"`
	ImportKWh       float64    `json:"importKwh"`
	ImportCostYen   float64    `json:"importCostYen"`
	ExportKWh       float64    `json:"exportKwh"`
	ExportIncomeYen float64    `json:"exportIncomeYen"`
	RateYen         float64    `json:"rateYen"`
	ExportRateYen   float64    `json:"exportRateYen"`
	EffectiveFrom   time.Time  `json:"effectiveFrom"`
	EffectiveTo     *time.Time `json:"effectiveTo,omitempty"`
}

type TariffSummary struct {
	PlanName             string                `json:"planName"`
	Timezone             string                `json:"timezone"`
	From                 *time.Time            `json:"from,omitempty"`
	To                   *time.Time            `json:"to,omitempty"`
	SampleCount          int                   `json:"sampleCount"`
	TotalImportKWh       float64               `json:"totalImportKwh"`
	TotalExportKWh       float64               `json:"totalExportKwh"`
	TotalImportCostYen   float64               `json:"totalImportCostYen"`
	TotalExportIncomeYen float64               `json:"totalExportIncomeYen"`
	NetCostYen           float64               `json:"netCostYen"`
	Periods              []TariffPeriodSummary `json:"periods"`
	Note                 string                `json:"note"`
}
