export type EnergyStatus = {
  gridW: number;
  importW: number;
  exportW: number;
  batterySoc: number;
  batteryInputW: number;
  batteryOutputW: number;
  acChargeLimitW: number;
  backupReserveSoc?: number | null;
  energyBackupEnabled?: boolean | null;
  touModeEnabled?: boolean | null;
  selfPoweredEnabled?: boolean | null;
  scheduledEnabled?: boolean | null;
  intelligentEnabled?: boolean | null;
  batteryFullEnergyWh?: number | null;
  surplusPlan?: SurplusPlan | null;
  nightChargePlan?: NightChargePlan | null;
  targetChargeW: number;
  state: string;
  mode: string;
  realControlTrialUntil?: string | null;
  realControlTrialActive: boolean;
  realControlTrialRemainingSeconds: number;
  lastDecisionReason: string;
  lastError: string | null;
  updatedAt: string;
};

export type SurplusPlan = {
  mode: string;
  strategyState: string;
  netBatteryW: number;
  requiredStartExportW: number;
  availableStartMarginW: number;
  recommendedAcChargeLimitW: number;
  recommendedBackupReserveSoc?: number | null;
  shouldRaiseBackupReserve: boolean;
  shouldLowerBackupReserve: boolean;
  shouldAlignBackupReserve: boolean;
  shouldAdjustAcChargeLimit: boolean;
  shouldDisableEnergyModes: boolean;
  shouldEnableTouMode: boolean;
  wouldWrite: boolean;
  actionSummary: string;
  reason: string;
};

export type WeatherForecast = {
  provider: string;
  date: string;
  weatherCode: number;
  shortwaveRadiationMjPerM2: number;
  sunshineDurationHours: number;
  cloudCoverMeanPercent: number;
  precipitationProbabilityMax: number;
  precipitationSumMm: number;
  hourlyShortwaveRadiation?: Array<{
    time: string;
    shortwaveRadiationWPerM2: number;
  }>;
};

export type WeatherLocation = {
  enabled: boolean;
  latitude: number;
  longitude: number;
  timezone: string;
  pvCapacityKw: number;
  pvPerformanceRatio: number;
  dailyBaseLoadKwh: number;
  batteryCapacityKwh: number;
  minimumReserveSoc: number;
};

export type DaytimeConsumptionEstimate = {
  days: number;
  startHour: number;
  endHour: number;
  sampleCount: number;
  averageImportKwh: number;
  averageExportKwh: number;
  averageBatteryChargeKwh: number;
  averageBatteryDischargeKwh: number;
  averageEstimatedLoadKwh: number;
  suggestedDailyBaseLoadKwh: number;
  daily: Array<{
    date: string;
    sampleCount: number;
    importKwh: number;
    exportKwh: number;
    batteryChargeKwh: number;
    batteryDischargeKwh: number;
    estimatedLoadKwh: number;
  }>;
};

export type EcoFlowLoadEstimate = {
  days: number;
  daytimeStartHour: number;
  daytimeEndHour: number;
  nightStartHour: number;
  nightEndHour: number;
  sampleCount: number;
  daytimeSampleDays: number;
  completeDaytimeSampleDays: number;
  averageDaytimeOutputKwh: number;
  averageShoulderOutputKwh: number;
  averageNightOutputKwh: number;
  averageDailyOutputKwh: number;
  averageDaytimeChargeKwh: number;
  suggestedDaytimeBaseLoadKwh: number;
  suggestedOvernightReserveKwh: number;
  note: string;
  daily: Array<{
    date: string;
    sampleCount: number;
    daytimeSampleCount: number;
    daytimeComplete: boolean;
    daytimeOutputKwh: number;
    shoulderOutputKwh: number;
    nightOutputKwh: number;
    dailyOutputKwh: number;
    daytimeChargeKwh: number;
    daytimeNetLoadKwh: number;
  }>;
};

export type NightChargePlan = {
  mode: string;
  strategyState: string;
  solarForecastScore: number;
  solarRadiationKwhPerM2: number;
  estimatedPvKwh: number;
  dailyEstimatedPvKwh: number;
  pvEffectiveStartAt?: string;
  pvEffectiveEndAt?: string;
  pvEffectiveWindowSource?: string;
  pvEffectiveRadiationWPerM2?: number;
  morningToPvStartLoadKwh: number;
  pvUsableForEcoFlowKwh: number;
  forecastDaytimeDeficitKwh: number;
  estimatedDaytimeLoadKwh: number;
  estimatedMorningLoadKwh: number;
  estimatedSurplusKwh: number;
  estimatedDeficitKwh: number;
  estimatedPvToBatteryKwh: number;
  safetyMarginKwh: number;
  batteryCapacityKwh: number;
  currentBatteryEnergyKwh: number;
  batteryChargeHeadroomKwh: number;
  recommendedNightTargetKwh: number;
  minimumReserveKwh: number;
  requiredNightChargeKwh: number;
  batteryCapacitySource: string;
  consumptionSource: string;
  recommendedMode: string;
  recommendedAcChargeLimitW: number;
  recommendedBackupReserveSoc?: number | null;
  recommendedNightTargetSoc: number;
  minimumReserveSoc: number;
  shouldChargeTonight: boolean;
  shouldSetAcChargeLimit: boolean;
  shouldSetBackupReserve: boolean;
  shouldDisableEnergyModes: boolean;
  shouldEnableTouMode: boolean;
  shouldEnableSelfPoweredMode: boolean;
  commandSuppressed: boolean;
  commandBlockReason: string;
  wouldWrite: boolean;
  actionSummary: string;
  reason: string;
  targetForecast?: WeatherForecast | null;
};

export type SolarForecastEstimate = {
  forecast: WeatherForecast;
  solarForecastScore: number;
  solarRadiationKwhPerM2: number;
  estimatedPvKwh: number;
  dailyEstimatedPvKwh: number;
  pvEffectiveStartAt?: string;
  pvEffectiveEndAt?: string;
  pvEffectiveWindowSource?: string;
  pvEffectiveRadiationWPerM2?: number;
  estimatedDaytimeLoadKwh: number;
  estimatedSurplusKwh: number;
  pvCapacityKw: number;
  pvPerformanceRatio: number;
  precipitationProbabilityMax: number;
  precipitationSumMm: number;
};

export type SolarForecastSummary = {
  days: number;
  location: WeatherLocation;
  items: SolarForecastEstimate[];
  note: string;
};

export type PowerLog = {
  id: number;
  measuredAt: string;
  gridW: number;
  importW: number;
  exportW: number;
  batterySoc: number | null;
  batteryInputW: number | null;
  batteryOutputW: number | null;
  acChargeLimitW: number | null;
  targetChargeW: number;
  actualCommandW: number | null;
  decisionReason: string;
  mode: string;
  commandSent: boolean;
  errorMessage: string | null;
  createdAt: string;
};

export type PowerLogsPage = {
  items: PowerLog[];
  total: number;
  limit: number;
  offset: number;
};

export type NightChargePlanLog = {
  id: number;
  measuredAt: string;
  strategyState: string;
  recommendedMode: string;
  recommendedNightTargetSoc: number;
  recommendedNightTargetKwh: number;
  currentBatteryEnergyKwh: number;
  requiredNightChargeKwh: number;
  dailyEstimatedPvKwh: number;
  pvEffectiveStartAt: string;
  pvEffectiveEndAt: string;
  pvEffectiveWindowSource: string;
  morningToPvStartLoadKwh: number;
  forecastDaytimeDeficitKwh: number;
  batterySoc: number;
  batteryInputW: number;
  batteryOutputW: number;
  gridW: number;
  importW: number;
  exportW: number;
  shouldChargeTonight: boolean;
  wouldWrite: boolean;
  commandBlockReason: string;
  actionSummary: string;
  reason: string;
  targetForecastDate?: string | null;
  createdAt: string;
};

export type NightChargePlanLogsPage = {
  items: NightChargePlanLog[];
  total: number;
  limit: number;
  offset: number;
};

export type SurplusControlCommandLog = {
  id: number;
  measuredAt: string;
  strategyState: string;
  commandKind: string;
  commandFingerprint: string;
  gridW: number;
  importW: number;
  exportW: number;
  batterySoc: number;
  batteryInputW: number;
  batteryOutputW: number;
  previousAcChargeLimitW: number | null;
  targetAcChargeLimitW: number | null;
  previousBackupReserveSoc: number | null;
  targetBackupReserveSoc: number | null;
  commandSent: boolean;
  dryRun: boolean;
  wouldWrite: boolean;
  shouldAdjustAcChargeLimit: boolean;
  shouldSetBackupReserve: boolean;
  shouldDisableEnergyModes: boolean;
  shouldEnableTouMode: boolean;
  modeGuardReason: string;
  suppressedReason: string;
  decisionReason: string;
  errorMessage: string | null;
  createdAt: string;
};

export type SurplusControlCommandLogsPage = {
  items: SurplusControlCommandLog[];
  total: number;
  limit: number;
  offset: number;
};

export type NightChargeDailySummary = {
  summaryDate: string;
  planCreatedAt?: string | null;
  targetForecastDate?: string | null;
  plannedTargetSoc?: number | null;
  plannedTargetKwh?: number | null;
  plannedRequiredChargeKwh?: number | null;
  plannedMode: string;
  nightStartSoc?: number | null;
  nightEndSoc?: number | null;
  nightSocDelta?: number | null;
  minNightSoc?: number | null;
  maxNightSoc?: number | null;
  nightImportKwh?: number | null;
  nightExportKwh?: number | null;
  nightBatteryInputKwh?: number | null;
  nightBatteryOutputKwh?: number | null;
  daytimeBatteryInputKwh?: number | null;
  daytimeExportKwh?: number | null;
  morningStatus: string;
  morningReason: string;
  finalResultStatus: string;
  finalResultReason: string;
  dataSource: string;
};

export type NightChargeDailySummariesPage = {
  items: NightChargeDailySummary[];
  total: number;
  limit: number;
  offset: number;
};

export type EnergyMeterLog = {
  id: number;
  measuredAt: string;
  importCumulativeKwh: number;
  exportCumulativeKwh: number;
  importDeltaKwh: number | null;
  exportDeltaKwh: number | null;
  coefficient: number;
  cumulativeUnit: number;
  rawImportCumulative: string;
  rawExportCumulative: string;
  importValueUpdatedAt: string;
  exportValueUpdatedAt: string;
  createdAt: string;
};

export type EnergyMeterLogsPage = {
  items: EnergyMeterLog[];
  total: number;
  limit: number;
  offset: number;
};

export type TariffPeriodSummary = {
  planName: string;
  period: string;
  importKwh: number;
  importCostYen: number;
  exportKwh: number;
  exportIncomeYen: number;
  rateYen: number;
  exportRateYen: number;
  effectiveFrom: string;
  effectiveTo?: string;
};

export type TariffSummary = {
  planName: string;
  timezone: string;
  from?: string;
  to?: string;
  sampleCount: number;
  totalImportKwh: number;
  totalExportKwh: number;
  totalImportCostYen: number;
  totalExportIncomeYen: number;
  netCostYen: number;
  periods: TariffPeriodSummary[];
  note: string;
};

export type TariffPlan = {
  id?: number;
  planName: string;
  dayRateYen: number;
  homeRateYen: number;
  nightRateYen: number;
  exportRateYen: number;
  timezone: string;
  effectiveFrom: string;
  effectiveTo?: string;
  createdAt?: string;
  updatedAt?: string;
};
