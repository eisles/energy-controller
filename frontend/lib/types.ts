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
  ecoflowDiagnostics?: Record<string, unknown> | null;
  surplusPlan?: SurplusPlan | null;
  nightChargePlan?: NightChargePlan | null;
  delta3AuxPlan?: Delta3AuxPlan | null;
  pro3AcOutputEvent?: Pro3ACOutputEvent | null;
  tariffControl?: TariffControlContext | null;
  controlWriteReadiness?: ControlWriteReadiness | null;
  controlDiagnostics?: ControlDiagnostics | null;
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

export type ControlWriteReadiness = {
  ready: boolean;
  mode: string;
  reasons: string[];
  gates: ControlWriteGates;
};

export type ControlWriteGates = {
  mockMode: boolean;
  simulationMode: boolean;
  enableRealControl: boolean;
  autoControlEnabled: boolean;
  confirmEcoFlowWriteAccepted: boolean;
  realControlTrialConfigured: boolean;
  realControlTrialActive: boolean;
  delta3ReadEnabled: boolean;
  delta3AuxEnabled: boolean;
  delta3ExecuteWrite: boolean;
  delta3AllowPrivateWrite: boolean;
  delta3AllowAutoWrite: boolean;
};

export type ControlDiagnostics = {
  gridState: string;
  summary: string;
  dataFreshness: ControlDataFreshness;
  writeReadiness: ControlDiagnosticsReadiness;
  pro3: ControlDeviceDiagnostics;
  auxiliary: ControlDeviceDiagnostics;
};

export type ControlDataFreshness = {
  updatedAt: string;
  ageSeconds: number;
  stale: boolean;
  hasError: boolean;
  lastError?: string | null;
};

export type ControlDiagnosticsReadiness = {
  ready: boolean;
  mode: string;
  blockedReason?: string;
  blockedReasons: number;
};

export type ControlDeviceDiagnostics = {
  name: string;
  deviceType?: string;
  action: string;
  reason: string;
  controlSource?: string;
  soc?: number | null;
  acInputW?: number | null;
  acOutputW?: number | null;
  targetChargeW?: number | null;
  recommendedAcChargeLimitW?: number | null;
  recommendedBackupReserveSoc?: number | null;
  writeCandidate: boolean;
};

export type Pro3ACOutputEvent = {
  id: number;
  measuredAt: string;
  eventType: string;
  outputPowerOffMemory: boolean;
  gridW: number;
  importW: number;
  exportW: number;
  batterySoc: number;
  batteryInputW: number;
  batteryOutputW: number;
  acChargeLimitW: number;
  bmsMaxCellTempC?: number | null;
  bmsMaxMosTempC?: number | null;
  acOutFreqHz?: number | null;
  acOutDsgPowMaxW?: number | null;
  previousCommandMeasuredAt?: string | null;
  previousCommandKind: string;
  previousCommandSent: boolean;
  previousCommandWouldWrite: boolean;
  previousCommandTargetAcChargeW?: number | null;
  previousCommandTargetReserveSoc?: number | null;
  previousCommandReason: string;
  message: string;
  createdAt: string;
};

export type Delta3Status = {
  available: boolean;
  deviceType?: string;
  soc?: number | null;
  acInW?: number | null;
  acOutW?: number | null;
  acChargeLimitW?: number | null;
  gridBypassDisabled?: boolean | null;
  acOutputEnabled?: boolean | null;
  acOutput1Enabled?: boolean | null;
  acOutput2Enabled?: boolean | null;
  acOutputProtectionChannel?: number | null;
  maxChargeSoc?: number | null;
  minDischargeSoc?: number | null;
  backupReserveSoc?: number | null;
  backupReserveEnabled?: boolean | null;
  touModeEnabled?: boolean | null;
  selfPoweredEnabled?: boolean | null;
  scheduledEnabled?: boolean | null;
  intelligentEnabled?: boolean | null;
  cycleCount?: number | null;
  cycleCountSource?: string;
  cycleCountCandidate?: CycleCountCandidate | null;
  cycleCountCandidates?: CycleCountCandidate[];
  updatedAt?: string;
  lastError?: string | null;
  cached?: boolean;
  telemetryDiagnostics?: PrivateTelemetryDiagnostics | null;
};

export type CycleCountCandidate = {
  value: number;
  source: string;
  cmdFunc: number;
  cmdId: number;
  field: number;
  confidence: string;
  reason: string;
};

export type PrivateTelemetryDiagnostics = {
  decodedMessages: number;
  unsupportedMessages: number;
  replyCount: number;
  inspectErrorCount?: number;
  lastInspectError?: string;
  fieldCount: number;
  fieldSummaryTruncated?: boolean;
  fieldSummaries?: PrivateTelemetryFieldSummary[];
};

export type PrivateTelemetryFieldSummary = {
  messageIndex: number;
  cmdFunc: number;
  cmdId: number;
  field: number;
  wire: number;
  value: string;
};

export type DeviceStatus = {
  id: number;
  name: string;
  kind: string;
  provider: string;
  role: string;
  credentialRef: string;
  deviceSn: string;
  deviceType: string;
  statusSource: string;
  enabled: boolean;
  priority: number;
  minChargeW: number;
  maxChargeW: number;
  chargeStepW: number;
  capacityWh: number;
  targetSoc: number;
  reserveSoc: number;
  backupReserveMinSoc: number;
  backupReserveMaxSoc: number;
  expectedDaytimeLoadW: number;
  autoRecoverAcOutput: boolean;
  controlEnabled: boolean;
  status: Delta3Status;
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
  tariffPeriod?: string;
  tariffRateYen?: number;
  tariffControlReason?: string;
};

export type Delta3AuxPlan = {
  deviceId?: number;
  deviceName?: string;
  deviceType?: string;
  mode: string;
  strategyState: string;
  recommendedAcChargeLimitW: number;
  currentAcChargeLimitW?: number | null;
  recommendedBackupReserveSoc?: number | null;
  currentBackupReserveSoc?: number | null;
  currentBackupReserveEnabled?: boolean | null;
  backupReserveApplyState?: string;
  backupReserveApplyReason?: string;
  lastBackupReserveCommandAt?: string;
  lastBackupReserveTargetSoc?: number | null;
  delta3Soc?: number | null;
  delta3MaxChargeSoc?: number | null;
  delta3AcOutputW?: number | null;
  delta3AcOutputEnabled?: boolean | null;
  safeAcChargeLimitW: number;
  residualExportW: number;
  safetyMarginW: number;
  wouldWrite: boolean;
  shouldAdjustAcChargeLimit: boolean;
  shouldSetBackupReserve: boolean;
  shouldDisableBackupReserve: boolean;
  suppressedReason?: string;
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
  pvChargeCorrectionFactor: number;
  pvChargeCorrectionManual: boolean;
  pvChargeCorrectionUpdatedAt?: string;
  pvChargeCorrectionMinSampleDays: number;
  pvChargeCorrectionMinFactor: number;
  pvChargeCorrectionMaxFactor: number;
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
  pvChargeCorrectionFactor: number;
  pvChargeCorrectionSource: string;
  correctedEstimatedPvKwh: number;
  correctedEstimatedPvToBatteryKwh: number;
  pvChargeCorrectionRecommendation?: PVChargeCorrectionRecommendation | null;
  safetyMarginKwh: number;
  batteryCapacityKwh: number;
  currentBatteryEnergyKwh: number;
  batteryChargeHeadroomKwh: number;
  totalDeviceCapacityKwh: number;
  totalCurrentDeviceEnergyKwh: number;
  totalRecommendedTargetKwh: number;
  totalRequiredDeviceChargeKwh: number;
  totalDaytimeRequiredKwh: number;
  totalAvailableKwh: number;
  totalDeficitKwh: number;
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
  tariffPeriod?: string;
  tariffRateYen?: number;
  tariffControlReason?: string;
  targetForecast?: WeatherForecast | null;
  devicePlans?: NightChargeDevicePlan[];
};

export type PVChargeCorrectionRecommendation = {
  recommendedFactor: number;
  okSampleDays: number;
  minSampleDays: number;
  applicable: boolean;
  status: string;
};

export type NightChargeDevicePlan = {
  deviceId: number;
  name: string;
  kind: string;
  deviceType?: string;
  priority: number;
  controlEnabled: boolean;
  writeTarget: boolean;
  capacityKwh: number;
  currentSoc?: number | null;
  currentEnergyKwh: number;
  daytimeRequiredKwh: number;
  availableKwh: number;
  pvAllocatedKwh: number;
  morningPrePvRequiredKwh: number;
  reserveSoc: number;
  targetSoc: number;
  minTargetSoc: number;
  maxTargetSoc: number;
  recommendedTargetSoc: number;
  recommendedTargetKwh: number;
  requiredChargeKwh: number;
  recommendedAcChargeLimitW: number;
  shouldCharge: boolean;
  wouldWrite: boolean;
  blockReason: string;
  reason: string;
  dataSource: string;
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
  pvChargeCorrectionFactor: number;
  pvChargeCorrectionSource: string;
  correctedEstimatedPvKwh: number;
  correctedEstimatedPvToBatteryKwh: number;
  totalDaytimeRequiredKwh: number;
  totalAvailableKwh: number;
  totalDeficitKwh: number;
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

export type PVForecastHistoryItem = {
  forecastDate: string;
  firstMeasuredAt: string;
  lastMeasuredAt: string;
  sampleCount: number;
  estimatedPvKwh: number;
  correctedEstimatedPvKwh: number;
  correctedEstimatedPvToBatteryKwh: number;
  forecastDaytimeDeficitKwh: number;
  recommendedNightTargetSoc: number;
  recommendedNightTargetKwh: number;
  requiredNightChargeKwh: number;
  totalDaytimeRequiredKwh: number;
  totalAvailableKwh: number;
  totalDeficitKwh: number;
  pvChargeCorrectionFactor: number;
  pvChargeCorrectionSource: string;
  shouldChargeTonight: boolean;
};

export type PVForecastHistoryResponse = {
  items: PVForecastHistoryItem[];
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

export type Delta3AuxControlCommandLog = {
  id: number;
  measuredAt: string;
  strategyState: string;
  commandFingerprint: string;
  gridW: number;
  importW: number;
  exportW: number;
  residualExportW: number;
  delta3Soc: number | null;
  previousAcChargeLimitW: number | null;
  targetAcChargeLimitW: number | null;
  previousBackupReserveSoc: number | null;
  targetBackupReserveSoc: number | null;
  commandSent: boolean;
  dryRun: boolean;
  wouldWrite: boolean;
  shouldAdjustAcChargeLimit: boolean;
  shouldSetBackupReserve: boolean;
  shouldDisableBackupReserve: boolean;
  suppressedReason: string;
  decisionReason: string;
  errorMessage: string | null;
  createdAt: string;
};

export type Delta3AuxControlCommandLogsPage = {
  items: Delta3AuxControlCommandLog[];
  total: number;
  limit: number;
  offset: number;
};

export type ChargingDevice = {
  id?: number;
  name: string;
  kind: string;
  provider: string;
  role: string;
  credentialRef: string;
  deviceSn: string;
  deviceType: string;
  statusSource: string;
  enabled: boolean;
  controlEnabled: boolean;
  priority: number;
  minChargeW: number;
  maxChargeW: number;
  chargeStepW: number;
  capacityWh: number;
  targetSoc: number;
  reserveSoc: number;
  backupReserveMinSoc: number;
  backupReserveMaxSoc: number;
  expectedDaytimeLoadW: number;
  autoRecoverAcOutput: boolean;
  supportsSocRead: boolean;
  supportsAcChargeLimit: boolean;
  supportsOnOff: boolean;
  notes: string;
  createdAt?: string;
  updatedAt?: string;
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
  morningTargetSocGap?: number | null;
  nightNetBatteryKwh?: number | null;
  nightRequiredChargeGapKwh?: number | null;
  daytimeChargeAndExportKwh?: number | null;
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
  batteryComparison?: BatteryCostComparison;
  note: string;
};

export type BatteryCostComparison = {
  available: boolean;
  method: string;
  quality: string;
  sampleCount: number;
  skippedSampleCount: number;
  maxSampleIntervalSeconds: number;
  actualImportKwh: number;
  actualExportKwh: number;
  actualImportCostYen: number;
  actualExportIncomeYen: number;
  actualNetCostYen: number;
  estimatedNoBatteryImportKwh: number;
  estimatedNoBatteryExportKwh: number;
  estimatedNoBatteryImportCostYen: number;
  estimatedNoBatteryExportIncomeYen: number;
  estimatedNoBatteryNetCostYen: number;
  estimatedSavingsYen: number;
  batteryInputKwh: number;
  batteryOutputKwh: number;
  dailyBreakdown?: BatteryCostComparisonDailyBreakdown[];
  note: string;
};

export type BatteryCostComparisonDailyBreakdown = {
  date: string;
  sampleCount: number;
  actualNetCostYen: number;
  estimatedNoBatteryNetCostYen: number;
  estimatedSavingsYen: number;
  lowPriceChargeKwh: number;
  midPriceDischargeKwh: number;
  highPriceDischargeKwh: number;
  exportAbsorptionKwh: number;
  batteryInputKwh: number;
  batteryOutputKwh: number;
  estimatedLossKwh: number;
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
  periodRules?: TariffPeriodRule[];
  periodRuleSource?: "default" | "custom";
};

export type TariffPeriodRule = {
  id?: number;
  tariffPlanId?: number;
  dayType: "weekday" | "holiday";
  period: string;
  startMinute: number;
  endMinute: number;
  rateYen: number;
  priority: number;
  createdAt?: string;
  updatedAt?: string;
};

export type TariffControlContext = {
  planName: string;
  timezone: string;
  dayType: string;
  currentPeriod: string;
  currentRateYen: number;
  lowestRateYen: number;
  highestRateYen: number;
  isLowPrice: boolean;
  isHighPrice: boolean;
  nextLowPriceAt?: string | null;
  source: string;
  reason: string;
};
