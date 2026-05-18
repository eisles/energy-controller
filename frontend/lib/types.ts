export type EnergyStatus = {
  gridW: number;
  importW: number;
  exportW: number;
  batterySoc: number;
  batteryInputW: number;
  batteryOutputW: number;
  acChargeLimitW: number;
  targetChargeW: number;
  state: string;
  mode: string;
  lastDecisionReason: string;
  lastError: string | null;
  updatedAt: string;
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
