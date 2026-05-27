package domain

import "time"

type GridPower struct {
	GridW   int `json:"gridW"`
	ImportW int `json:"importW"`
	ExportW int `json:"exportW"`
}

type BatteryStatus struct {
	Soc                 int            `json:"batterySoc"`
	InputW              int            `json:"batteryInputW"`
	OutputW             int            `json:"batteryOutputW"`
	ACChargeLimitW      int            `json:"acChargeLimitW"`
	BackupReserveSoc    *int           `json:"backupReserveSoc,omitempty"`
	EnergyBackupEnabled *bool          `json:"energyBackupEnabled,omitempty"`
	TOUModeEnabled      *bool          `json:"touModeEnabled,omitempty"`
	SelfPoweredEnabled  *bool          `json:"selfPoweredEnabled,omitempty"`
	ScheduledEnabled    *bool          `json:"scheduledEnabled,omitempty"`
	IntelligentEnabled  *bool          `json:"intelligentEnabled,omitempty"`
	FullEnergyWh        *int           `json:"fullEnergyWh,omitempty"`
	EcoFlowDiagnostics  map[string]any `json:"ecoflowDiagnostics,omitempty"`
	IsOnline            bool           `json:"isOnline"`
}

type ControlDecision struct {
	ShouldCharge  bool
	TargetChargeW int
	Reason        string
}

type Status struct {
	GridW                            int              `json:"gridW"`
	ImportW                          int              `json:"importW"`
	ExportW                          int              `json:"exportW"`
	BatterySoc                       int              `json:"batterySoc"`
	BatteryInputW                    int              `json:"batteryInputW"`
	BatteryOutputW                   int              `json:"batteryOutputW"`
	ACChargeLimitW                   int              `json:"acChargeLimitW"`
	BackupReserveSoc                 *int             `json:"backupReserveSoc,omitempty"`
	EnergyBackupEnabled              *bool            `json:"energyBackupEnabled,omitempty"`
	TOUModeEnabled                   *bool            `json:"touModeEnabled,omitempty"`
	SelfPoweredEnabled               *bool            `json:"selfPoweredEnabled,omitempty"`
	ScheduledEnabled                 *bool            `json:"scheduledEnabled,omitempty"`
	IntelligentEnabled               *bool            `json:"intelligentEnabled,omitempty"`
	BatteryFullEnergyWh              *int             `json:"batteryFullEnergyWh,omitempty"`
	EcoFlowDiagnostics               map[string]any   `json:"ecoflowDiagnostics,omitempty"`
	SurplusPlan                      *SurplusPlan     `json:"surplusPlan,omitempty"`
	NightChargePlan                  *NightChargePlan `json:"nightChargePlan,omitempty"`
	Delta3AuxPlan                    *Delta3AuxPlan   `json:"delta3AuxPlan,omitempty"`
	TargetChargeW                    int              `json:"targetChargeW"`
	State                            string           `json:"state"`
	Mode                             string           `json:"mode"`
	RealControlTrialUntil            *time.Time       `json:"realControlTrialUntil,omitempty"`
	RealControlTrialActive           bool             `json:"realControlTrialActive"`
	RealControlTrialRemainingSeconds int64            `json:"realControlTrialRemainingSeconds"`
	LastDecisionReason               string           `json:"lastDecisionReason"`
	LastError                        *string          `json:"lastError"`
	UpdatedAt                        time.Time        `json:"updatedAt"`
}

type SurplusPlan struct {
	Mode                        string `json:"mode"`
	StrategyState               string `json:"strategyState"`
	NetBatteryW                 int    `json:"netBatteryW"`
	RequiredStartExportW        int    `json:"requiredStartExportW"`
	AvailableStartMarginW       int    `json:"availableStartMarginW"`
	RecommendedACChargeLimitW   int    `json:"recommendedAcChargeLimitW"`
	RecommendedBackupReserveSoc *int   `json:"recommendedBackupReserveSoc,omitempty"`
	ShouldRaiseBackupReserve    bool   `json:"shouldRaiseBackupReserve"`
	ShouldLowerBackupReserve    bool   `json:"shouldLowerBackupReserve"`
	ShouldAlignBackupReserve    bool   `json:"shouldAlignBackupReserve"`
	ShouldAdjustACChargeLimit   bool   `json:"shouldAdjustAcChargeLimit"`
	ShouldDisableEnergyModes    bool   `json:"shouldDisableEnergyModes"`
	ShouldEnableTOUMode         bool   `json:"shouldEnableTouMode"`
	WouldWrite                  bool   `json:"wouldWrite"`
	ActionSummary               string `json:"actionSummary"`
	Reason                      string `json:"reason"`
}

type Delta3AuxPlan struct {
	Mode                        string `json:"mode"`
	StrategyState               string `json:"strategyState"`
	RecommendedACChargeLimitW   int    `json:"recommendedAcChargeLimitW"`
	CurrentACChargeLimitW       *int   `json:"currentAcChargeLimitW,omitempty"`
	RecommendedBackupReserveSoc *int   `json:"recommendedBackupReserveSoc,omitempty"`
	CurrentBackupReserveSoc     *int   `json:"currentBackupReserveSoc,omitempty"`
	Delta3Soc                   *int   `json:"delta3Soc,omitempty"`
	Delta3MaxChargeSoc          *int   `json:"delta3MaxChargeSoc,omitempty"`
	Delta3ACOutputW             *int   `json:"delta3AcOutputW,omitempty"`
	SafeACChargeLimitW          int    `json:"safeAcChargeLimitW"`
	ResidualExportW             int    `json:"residualExportW"`
	SafetyMarginW               int    `json:"safetyMarginW"`
	WouldWrite                  bool   `json:"wouldWrite"`
	ShouldAdjustACChargeLimit   bool   `json:"shouldAdjustAcChargeLimit"`
	ShouldSetBackupReserve      bool   `json:"shouldSetBackupReserve"`
	ShouldDisableBackupReserve  bool   `json:"shouldDisableBackupReserve"`
	SuppressedReason            string `json:"suppressedReason,omitempty"`
	Reason                      string `json:"reason"`
}

type WeatherForecast struct {
	Provider                    string                     `json:"provider"`
	Date                        string                     `json:"date"`
	WeatherCode                 int                        `json:"weatherCode"`
	ShortwaveRadiationMJPerM2   float64                    `json:"shortwaveRadiationMjPerM2"`
	SunshineDurationHours       float64                    `json:"sunshineDurationHours"`
	CloudCoverMeanPercent       int                        `json:"cloudCoverMeanPercent"`
	PrecipitationProbabilityMax int                        `json:"precipitationProbabilityMax"`
	PrecipitationSumMM          float64                    `json:"precipitationSumMm"`
	HourlyShortwaveRadiation    []HourlyShortwaveRadiation `json:"hourlyShortwaveRadiation,omitempty"`
}

type HourlyShortwaveRadiation struct {
	Time                     string  `json:"time"`
	ShortwaveRadiationWPerM2 float64 `json:"shortwaveRadiationWPerM2"`
}

type SolarForecastEstimate struct {
	Forecast                    WeatherForecast `json:"forecast"`
	SolarForecastScore          int             `json:"solarForecastScore"`
	SolarRadiationKWhPerM2      float64         `json:"solarRadiationKwhPerM2"`
	EstimatedPVKWh              float64         `json:"estimatedPvKwh"`
	DailyEstimatedPVKWh         float64         `json:"dailyEstimatedPvKwh"`
	PVEffectiveStartAt          string          `json:"pvEffectiveStartAt,omitempty"`
	PVEffectiveEndAt            string          `json:"pvEffectiveEndAt,omitempty"`
	PVEffectiveWindowSource     string          `json:"pvEffectiveWindowSource,omitempty"`
	PVEffectiveRadiationWPerM2  float64         `json:"pvEffectiveRadiationWPerM2,omitempty"`
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
	DaytimeSampleDays            int                        `json:"daytimeSampleDays"`
	CompleteDaytimeSampleDays    int                        `json:"completeDaytimeSampleDays"`
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
	Date               string  `json:"date"`
	SampleCount        int     `json:"sampleCount"`
	DaytimeSampleCount int     `json:"daytimeSampleCount"`
	DaytimeComplete    bool    `json:"daytimeComplete"`
	DaytimeOutputKWh   float64 `json:"daytimeOutputKwh"`
	ShoulderOutputKWh  float64 `json:"shoulderOutputKwh"`
	NightOutputKWh     float64 `json:"nightOutputKwh"`
	DailyOutputKWh     float64 `json:"dailyOutputKwh"`
	DaytimeChargeKWh   float64 `json:"daytimeChargeKwh"`
	DaytimeNetLoadKWh  float64 `json:"daytimeNetLoadKwh"`
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
	Mode                        string           `json:"mode"`
	StrategyState               string           `json:"strategyState"`
	SolarForecastScore          int              `json:"solarForecastScore"`
	SolarRadiationKWhPerM2      float64          `json:"solarRadiationKwhPerM2"`
	EstimatedPVKWh              float64          `json:"estimatedPvKwh"`
	DailyEstimatedPVKWh         float64          `json:"dailyEstimatedPvKwh"`
	PVEffectiveStartAt          string           `json:"pvEffectiveStartAt,omitempty"`
	PVEffectiveEndAt            string           `json:"pvEffectiveEndAt,omitempty"`
	PVEffectiveWindowSource     string           `json:"pvEffectiveWindowSource,omitempty"`
	PVEffectiveRadiationWPerM2  float64          `json:"pvEffectiveRadiationWPerM2,omitempty"`
	MorningToPVStartLoadKWh     float64          `json:"morningToPvStartLoadKwh"`
	PVUsableForEcoFlowKWh       float64          `json:"pvUsableForEcoFlowKwh"`
	ForecastDaytimeDeficitKWh   float64          `json:"forecastDaytimeDeficitKwh"`
	EstimatedDaytimeLoadKWh     float64          `json:"estimatedDaytimeLoadKwh"`
	EstimatedMorningLoadKWh     float64          `json:"estimatedMorningLoadKwh"`
	EstimatedSurplusKWh         float64          `json:"estimatedSurplusKwh"`
	EstimatedDeficitKWh         float64          `json:"estimatedDeficitKwh"`
	EstimatedPVToBatteryKWh     float64          `json:"estimatedPvToBatteryKwh"`
	SafetyMarginKWh             float64          `json:"safetyMarginKwh"`
	BatteryCapacityKWh          float64          `json:"batteryCapacityKwh"`
	CurrentBatteryEnergyKWh     float64          `json:"currentBatteryEnergyKwh"`
	BatteryChargeHeadroomKWh    float64          `json:"batteryChargeHeadroomKwh"`
	RecommendedNightTargetKWh   float64          `json:"recommendedNightTargetKwh"`
	MinimumReserveKWh           float64          `json:"minimumReserveKwh"`
	RequiredNightChargeKWh      float64          `json:"requiredNightChargeKwh"`
	BatteryCapacitySource       string           `json:"batteryCapacitySource"`
	ConsumptionSource           string           `json:"consumptionSource"`
	RecommendedMode             string           `json:"recommendedMode"`
	RecommendedACChargeLimitW   int              `json:"recommendedAcChargeLimitW"`
	RecommendedBackupReserveSoc *int             `json:"recommendedBackupReserveSoc,omitempty"`
	RecommendedNightTargetSoc   int              `json:"recommendedNightTargetSoc"`
	MinimumReserveSoc           int              `json:"minimumReserveSoc"`
	ShouldChargeTonight         bool             `json:"shouldChargeTonight"`
	ShouldSetACChargeLimit      bool             `json:"shouldSetAcChargeLimit"`
	ShouldSetBackupReserve      bool             `json:"shouldSetBackupReserve"`
	ShouldDisableEnergyModes    bool             `json:"shouldDisableEnergyModes"`
	ShouldEnableTOUMode         bool             `json:"shouldEnableTouMode"`
	ShouldEnableSelfPoweredMode bool             `json:"shouldEnableSelfPoweredMode"`
	CommandSuppressed           bool             `json:"commandSuppressed"`
	CommandFingerprint          string           `json:"commandFingerprint"`
	CommandBlockReason          string           `json:"commandBlockReason"`
	CommandSent                 bool             `json:"commandSent"`
	CommandError                *string          `json:"commandError,omitempty"`
	WouldWrite                  bool             `json:"wouldWrite"`
	ActionSummary               string           `json:"actionSummary"`
	Reason                      string           `json:"reason"`
	TargetForecast              *WeatherForecast `json:"targetForecast,omitempty"`
}

type NightChargePlanLog struct {
	ID                        int64     `json:"id"`
	MeasuredAt                time.Time `json:"measuredAt"`
	StrategyState             string    `json:"strategyState"`
	RecommendedMode           string    `json:"recommendedMode"`
	RecommendedNightTargetSoc int       `json:"recommendedNightTargetSoc"`
	RecommendedNightTargetKWh float64   `json:"recommendedNightTargetKwh"`
	CurrentBatteryEnergyKWh   float64   `json:"currentBatteryEnergyKwh"`
	RequiredNightChargeKWh    float64   `json:"requiredNightChargeKwh"`
	DailyEstimatedPVKWh       float64   `json:"dailyEstimatedPvKwh"`
	PVEffectiveStartAt        string    `json:"pvEffectiveStartAt"`
	PVEffectiveEndAt          string    `json:"pvEffectiveEndAt"`
	PVEffectiveWindowSource   string    `json:"pvEffectiveWindowSource"`
	MorningToPVStartLoadKWh   float64   `json:"morningToPvStartLoadKwh"`
	ForecastDaytimeDeficitKWh float64   `json:"forecastDaytimeDeficitKwh"`
	BatterySoc                int       `json:"batterySoc"`
	BatteryInputW             int       `json:"batteryInputW"`
	BatteryOutputW            int       `json:"batteryOutputW"`
	GridW                     int       `json:"gridW"`
	ImportW                   int       `json:"importW"`
	ExportW                   int       `json:"exportW"`
	ShouldChargeTonight       bool      `json:"shouldChargeTonight"`
	WouldWrite                bool      `json:"wouldWrite"`
	CommandFingerprint        string    `json:"commandFingerprint"`
	CommandSent               bool      `json:"commandSent"`
	CommandError              *string   `json:"commandError,omitempty"`
	CommandBlockReason        string    `json:"commandBlockReason"`
	ActionSummary             string    `json:"actionSummary"`
	Reason                    string    `json:"reason"`
	TargetForecastDate        *string   `json:"targetForecastDate,omitempty"`
	CreatedAt                 time.Time `json:"createdAt"`
}

type NightChargeDailySummary struct {
	SummaryDate               string     `json:"summaryDate"`
	PlanCreatedAt             *time.Time `json:"planCreatedAt,omitempty"`
	TargetForecastDate        *string    `json:"targetForecastDate,omitempty"`
	PlannedTargetSoc          *int       `json:"plannedTargetSoc,omitempty"`
	PlannedTargetKWh          *float64   `json:"plannedTargetKwh,omitempty"`
	PlannedRequiredChargeKWh  *float64   `json:"plannedRequiredChargeKwh,omitempty"`
	PlannedMode               string     `json:"plannedMode"`
	NightStartSoc             *int       `json:"nightStartSoc,omitempty"`
	NightEndSoc               *int       `json:"nightEndSoc,omitempty"`
	NightSocDelta             *int       `json:"nightSocDelta,omitempty"`
	MinNightSoc               *int       `json:"minNightSoc,omitempty"`
	MaxNightSoc               *int       `json:"maxNightSoc,omitempty"`
	NightImportKWh            *float64   `json:"nightImportKwh,omitempty"`
	NightExportKWh            *float64   `json:"nightExportKwh,omitempty"`
	NightBatteryInputKWh      *float64   `json:"nightBatteryInputKwh,omitempty"`
	NightBatteryOutputKWh     *float64   `json:"nightBatteryOutputKwh,omitempty"`
	DaytimeBatteryInputKWh    *float64   `json:"daytimeBatteryInputKwh,omitempty"`
	DaytimeExportKWh          *float64   `json:"daytimeExportKwh,omitempty"`
	MorningTargetSocGap       *int       `json:"morningTargetSocGap,omitempty"`
	NightNetBatteryKWh        *float64   `json:"nightNetBatteryKwh,omitempty"`
	NightRequiredChargeGapKWh *float64   `json:"nightRequiredChargeGapKwh,omitempty"`
	DaytimeChargeAndExportKWh *float64   `json:"daytimeChargeAndExportKwh,omitempty"`
	MorningStatus             string     `json:"morningStatus"`
	MorningReason             string     `json:"morningReason"`
	FinalResultStatus         string     `json:"finalResultStatus"`
	FinalResultReason         string     `json:"finalResultReason"`
	DataSource                string     `json:"dataSource"`
}

type PowerLog struct {
	ID                 int64          `json:"id"`
	MeasuredAt         time.Time      `json:"measuredAt"`
	GridW              int            `json:"gridW"`
	ImportW            int            `json:"importW"`
	ExportW            int            `json:"exportW"`
	BatterySoc         *int           `json:"batterySoc"`
	BatteryInputW      *int           `json:"batteryInputW"`
	BatteryOutputW     *int           `json:"batteryOutputW"`
	ACChargeLimitW     *int           `json:"acChargeLimitW"`
	TargetChargeW      int            `json:"targetChargeW"`
	ActualCommandW     *int           `json:"actualCommandW"`
	DecisionReason     string         `json:"decisionReason"`
	Mode               string         `json:"mode"`
	CommandSent        bool           `json:"commandSent"`
	ErrorMessage       *string        `json:"errorMessage"`
	EcoFlowDiagnostics map[string]any `json:"ecoflowDiagnostics,omitempty"`
	CreatedAt          time.Time      `json:"createdAt"`
}

type SurplusControlCommandLog struct {
	ID                        int64     `json:"id"`
	MeasuredAt                time.Time `json:"measuredAt"`
	StrategyState             string    `json:"strategyState"`
	CommandKind               string    `json:"commandKind"`
	CommandFingerprint        string    `json:"commandFingerprint"`
	GridW                     int       `json:"gridW"`
	ImportW                   int       `json:"importW"`
	ExportW                   int       `json:"exportW"`
	BatterySoc                int       `json:"batterySoc"`
	BatteryInputW             int       `json:"batteryInputW"`
	BatteryOutputW            int       `json:"batteryOutputW"`
	PreviousACChargeLimitW    *int      `json:"previousAcChargeLimitW"`
	TargetACChargeLimitW      *int      `json:"targetAcChargeLimitW"`
	PreviousBackupReserveSoc  *int      `json:"previousBackupReserveSoc"`
	TargetBackupReserveSoc    *int      `json:"targetBackupReserveSoc"`
	CommandSent               bool      `json:"commandSent"`
	DryRun                    bool      `json:"dryRun"`
	WouldWrite                bool      `json:"wouldWrite"`
	ShouldAdjustACChargeLimit bool      `json:"shouldAdjustAcChargeLimit"`
	ShouldSetBackupReserve    bool      `json:"shouldSetBackupReserve"`
	ShouldDisableEnergyModes  bool      `json:"shouldDisableEnergyModes"`
	ShouldEnableTOUMode       bool      `json:"shouldEnableTouMode"`
	ModeGuardReason           string    `json:"modeGuardReason"`
	SuppressedReason          string    `json:"suppressedReason"`
	DecisionReason            string    `json:"decisionReason"`
	ErrorMessage              *string   `json:"errorMessage"`
	CreatedAt                 time.Time `json:"createdAt"`
}

type Delta3AuxControlCommandLog struct {
	ID                         int64     `json:"id"`
	MeasuredAt                 time.Time `json:"measuredAt"`
	StrategyState              string    `json:"strategyState"`
	CommandFingerprint         string    `json:"commandFingerprint"`
	GridW                      int       `json:"gridW"`
	ImportW                    int       `json:"importW"`
	ExportW                    int       `json:"exportW"`
	ResidualExportW            int       `json:"residualExportW"`
	Delta3Soc                  *int      `json:"delta3Soc"`
	PreviousACChargeLimitW     *int      `json:"previousAcChargeLimitW"`
	TargetACChargeLimitW       *int      `json:"targetAcChargeLimitW"`
	PreviousBackupReserveSoc   *int      `json:"previousBackupReserveSoc"`
	TargetBackupReserveSoc     *int      `json:"targetBackupReserveSoc"`
	CommandSent                bool      `json:"commandSent"`
	DryRun                     bool      `json:"dryRun"`
	WouldWrite                 bool      `json:"wouldWrite"`
	ShouldAdjustACChargeLimit  bool      `json:"shouldAdjustAcChargeLimit"`
	ShouldSetBackupReserve     bool      `json:"shouldSetBackupReserve"`
	ShouldDisableBackupReserve bool      `json:"shouldDisableBackupReserve"`
	SuppressedReason           string    `json:"suppressedReason"`
	DecisionReason             string    `json:"decisionReason"`
	ErrorMessage               *string   `json:"errorMessage"`
	CreatedAt                  time.Time `json:"createdAt"`
}

type NotificationLog struct {
	ID              int64     `json:"id"`
	MeasuredAt      time.Time `json:"measuredAt"`
	Kind            string    `json:"kind"`
	Fingerprint     string    `json:"fingerprint"`
	Severity        string    `json:"severity"`
	Message         string    `json:"message"`
	Reason          string    `json:"reason"`
	ExportW         int       `json:"exportW"`
	BatterySoc      int       `json:"batterySoc"`
	ACChargeLimitW  int       `json:"acChargeLimitW"`
	Sent            bool      `json:"sent"`
	ErrorMessage    *string   `json:"errorMessage"`
	ConsecutiveHits int       `json:"consecutiveHits"`
	CreatedAt       time.Time `json:"createdAt"`
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
