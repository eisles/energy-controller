export type EnergyStatus = {
  gridW: number;
  importW: number;
  exportW: number;
  batterySoc: number;
  batteryInputW: number;
  batteryOutputW: number;
  targetChargeW: number;
  state: string;
  mode: string;
  lastDecisionReason: string;
  lastError: string | null;
  updatedAt: string;
};
