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
  batteryFullEnergyWh?: number | null;
  surplusPlan?: SurplusPlan | null;
  nightChargePlan?: NightChargePlan | null;
  targetChargeW: number;
  state: string;
  mode: string;
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
  estimatedDaytimeLoadKwh: number;
  estimatedSurplusKwh: number;
  estimatedDeficitKwh: number;
  estimatedPvToBatteryKwh: number;
  batteryCapacityKwh: number;
  currentBatteryEnergyKwh: number;
  batteryChargeHeadroomKwh: number;
  recommendedNightTargetKwh: number;
  minimumReserveKwh: number;
  requiredNightChargeKwh: number;
  batteryCapacitySource: string;
  recommendedNightTargetSoc: number;
  minimumReserveSoc: number;
  shouldChargeTonight: boolean;
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
