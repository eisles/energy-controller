"use client";

import type { ReactNode } from "react";
import { useEffect, useState } from "react";
import { CollapsibleSection } from "@/components/CollapsibleSection";
import { ControlPanel } from "@/components/ControlPanel";
import { AuxiliaryBatteryControlCommandLogTable } from "@/components/AuxiliaryBatteryControlCommandLogTable";
import { DryRunPlanHistory } from "@/components/DryRunPlanHistory";
import { EnergyCharts } from "@/components/EnergyCharts";
import { EnergyMeterLogTable } from "@/components/EnergyMeterLogTable";
import { Header } from "@/components/Header";
import { LogTable } from "@/components/LogTable";
import { NightChargePlanLogTable } from "@/components/NightChargePlanLogTable";
import { NightChargeSummaryTable } from "@/components/NightChargeSummaryTable";
import { SolarForecastPanel } from "@/components/SolarForecastPanel";
import {
  NightChargePlanSection,
  Delta3StatusCard,
  StatusCards,
  StatusChartSection,
  StatusDecisionSection,
  SurplusPlanSection,
  type StatusCardSectionKey
} from "@/components/StatusCards";
import { SurplusControlCommandLogTable } from "@/components/SurplusControlCommandLogTable";
import { TariffSummaryPanel } from "@/components/TariffSummaryPanel";
import { Button } from "@/components/ui/button";
import { VerificationPanel } from "@/components/VerificationPanel";
import {
  fetchEnergyMeterLogsPage,
  fetchDelta3Status,
  fetchDeviceStatuses,
  fetchDelta3AuxControlCommandLogsPage,
  fetchLogs,
  fetchLogsPage,
  fetchNightChargePlanLogsPage,
  fetchNightChargeSummariesPage,
  fetchSolarForecast,
  fetchStatus,
  fetchSurplusControlCommandLogsPage,
  fetchTariffSummary
} from "@/lib/api";
import type {
  EnergyMeterLog,
  EnergyStatus,
  DeviceStatus,
  Delta3Status,
  Delta3AuxControlCommandLog,
  NightChargeDailySummary,
  NightChargePlanLog,
  PowerLog,
  SolarForecastSummary,
  SurplusControlCommandLog,
  TariffSummary
} from "@/lib/types";

type LogRange = {
  label: string;
  hours: number | null;
};

const logRanges: LogRange[] = [
  { label: "1時間", hours: 1 },
  { label: "6時間", hours: 6 },
  { label: "24時間", hours: 24 },
  { label: "最新500件", hours: null }
];

const forecastRanges = [
  { label: "3日", days: 3 },
  { label: "7日", days: 7 },
  { label: "16日", days: 16 }
] as const;

const logPageSize = 25;
const energyMeterLogPageSize = 25;
const nightChargePlanLogPageSize = 25;
const nightChargeSummaryPageSize = 25;
const surplusControlCommandLogPageSize = 25;
const delta3AuxControlCommandLogPageSize = 25;
const dryRunPlanLimit = 10;

type DashboardSectionKey =
  | StatusCardSectionKey
  | "settings"
  | "solarForecast"
  | "tariffSummary"
  | "verification"
  | "nightDryRun"
  | "surplusCommand"
  | "delta3AuxCommand"
  | "nightPlanLog"
  | "nightSummary"
  | "surplusDryRun"
  | "energyMeter"
  | "controlLog";

const defaultSectionOrder: DashboardSectionKey[] = [
  "charts",
  "decision",
  "surplusPlan",
  "nightPlan",
  "verification",
  "settings",
  "solarForecast",
  "tariffSummary",
  "nightDryRun",
  "surplusCommand",
  "delta3AuxCommand",
  "nightPlanLog",
  "nightSummary",
  "surplusDryRun",
  "energyMeter",
  "controlLog"
];

const initialDashboardSections: Record<DashboardSectionKey, boolean> = {
  charts: true,
  decision: true,
  surplusPlan: true,
  nightPlan: true,
  verification: true,
  settings: true,
  solarForecast: true,
  tariffSummary: true,
  nightDryRun: false,
  surplusCommand: false,
  delta3AuxCommand: false,
  nightPlanLog: false,
  nightSummary: false,
  surplusDryRun: false,
  energyMeter: false,
  controlLog: false
};

const dashboardSectionStorageKey = "energy-controller.dashboard.sections.v1";
const dashboardSectionOrderStorageKey = "energy-controller.dashboard.sectionOrder.v1";

const initialStatus: EnergyStatus = {
  gridW: 0,
  importW: 0,
  exportW: 0,
  batterySoc: 0,
  batteryInputW: 0,
  batteryOutputW: 0,
  acChargeLimitW: 0,
  targetChargeW: 0,
  state: "loading",
  mode: "mock",
  realControlTrialUntil: null,
  realControlTrialActive: false,
  realControlTrialRemainingSeconds: 0,
  lastDecisionReason: "loading mock simulation status",
  lastError: null,
  updatedAt: ""
};

export function Dashboard() {
  const [status, setStatus] = useState<EnergyStatus>(initialStatus);
  const [delta3Status, setDelta3Status] = useState<Delta3Status | null>(null);
  const [deviceStatuses, setDeviceStatuses] = useState<DeviceStatus[]>([]);
  const [chartLogs, setChartLogs] = useState<PowerLog[]>([]);
  const [tableLogs, setTableLogs] = useState<PowerLog[]>([]);
  const [dryRunPlanLogs, setDryRunPlanLogs] = useState<PowerLog[]>([]);
  const [nightDryRunPlanLogs, setNightDryRunPlanLogs] = useState<PowerLog[]>([]);
  const [nightChargePlanLogs, setNightChargePlanLogs] = useState<NightChargePlanLog[]>([]);
  const [nightChargePlanPage, setNightChargePlanPage] = useState(1);
  const [nightChargePlanTotal, setNightChargePlanTotal] = useState(0);
  const [surplusCommandLogs, setSurplusCommandLogs] = useState<SurplusControlCommandLog[]>([]);
  const [surplusCommandPage, setSurplusCommandPage] = useState(1);
  const [surplusCommandTotal, setSurplusCommandTotal] = useState(0);
  const [delta3AuxCommandLogs, setDelta3AuxCommandLogs] = useState<Delta3AuxControlCommandLog[]>([]);
  const [delta3AuxCommandPage, setDelta3AuxCommandPage] = useState(1);
  const [delta3AuxCommandTotal, setDelta3AuxCommandTotal] = useState(0);
  const [recentDelta3AuxCommandLogs, setRecentDelta3AuxCommandLogs] = useState<Delta3AuxControlCommandLog[]>([]);
  const [nightChargeSummaries, setNightChargeSummaries] = useState<NightChargeDailySummary[]>([]);
  const [latestNightChargeSummary, setLatestNightChargeSummary] = useState<NightChargeDailySummary | null>(null);
  const [nightChargeSummaryPage, setNightChargeSummaryPage] = useState(1);
  const [nightChargeSummaryFromInput, setNightChargeSummaryFromInput] = useState("");
  const [nightChargeSummaryToInput, setNightChargeSummaryToInput] = useState("");
  const [appliedNightChargeSummaryFrom, setAppliedNightChargeSummaryFrom] = useState("");
  const [appliedNightChargeSummaryTo, setAppliedNightChargeSummaryTo] = useState("");
  const [nightChargeSummaryTotal, setNightChargeSummaryTotal] = useState(0);
  const [logPage, setLogPage] = useState(1);
  const [logSearchInput, setLogSearchInput] = useState("");
  const [logFromInput, setLogFromInput] = useState("");
  const [logToInput, setLogToInput] = useState("");
  const [appliedLogSearch, setAppliedLogSearch] = useState("");
  const [appliedLogFrom, setAppliedLogFrom] = useState("");
  const [appliedLogTo, setAppliedLogTo] = useState("");
  const [logTotal, setLogTotal] = useState(0);
  const [energyMeterLogs, setEnergyMeterLogs] = useState<EnergyMeterLog[]>([]);
  const [energyMeterPage, setEnergyMeterPage] = useState(1);
  const [energyMeterFromInput, setEnergyMeterFromInput] = useState("");
  const [energyMeterToInput, setEnergyMeterToInput] = useState("");
  const [appliedEnergyMeterFrom, setAppliedEnergyMeterFrom] = useState("");
  const [appliedEnergyMeterTo, setAppliedEnergyMeterTo] = useState("");
  const [energyMeterTotal, setEnergyMeterTotal] = useState(0);
  const [tariffSummary, setTariffSummary] = useState<TariffSummary | null>(null);
  const [solarForecast, setSolarForecast] = useState<SolarForecastSummary | null>(null);
  const [forecastRange, setForecastRange] = useState<{ label: string; days: number }>(forecastRanges[0]);
  const [tariffFromInput, setTariffFromInput] = useState("");
  const [tariffToInput, setTariffToInput] = useState("");
  const [appliedTariffFrom, setAppliedTariffFrom] = useState("");
  const [appliedTariffTo, setAppliedTariffTo] = useState("");
  const [tariffRefreshToken, setTariffRefreshToken] = useState(0);
  const [logRange, setLogRange] = useState<LogRange>(logRanges[1]);
  const [statusError, setStatusError] = useState<string | null>(null);
  const [delta3StatusError, setDelta3StatusError] = useState<string | null>(null);
  const [logsError, setLogsError] = useState<string | null>(null);
  const [dryRunPlanError, setDryRunPlanError] = useState<string | null>(null);
  const [nightDryRunPlanError, setNightDryRunPlanError] = useState<string | null>(null);
  const [nightChargePlanError, setNightChargePlanError] = useState<string | null>(null);
  const [surplusCommandError, setSurplusCommandError] = useState<string | null>(null);
  const [delta3AuxCommandError, setDelta3AuxCommandError] = useState<string | null>(null);
  const [recentDelta3AuxCommandError, setRecentDelta3AuxCommandError] = useState<string | null>(null);
  const [nightChargeSummaryError, setNightChargeSummaryError] = useState<string | null>(null);
  const [latestNightChargeSummaryError, setLatestNightChargeSummaryError] = useState<string | null>(null);
  const [energyMeterError, setEnergyMeterError] = useState<string | null>(null);
  const [tariffError, setTariffError] = useState<string | null>(null);
  const [solarForecastError, setSolarForecastError] = useState<string | null>(null);
  const [solarForecastLoading, setSolarForecastLoading] = useState(false);
  const [openSections, setOpenSections] = useState<Record<DashboardSectionKey, boolean>>(initialDashboardSections);
  const [openSectionsLoaded, setOpenSectionsLoaded] = useState(false);
  const [sectionOrder, setSectionOrder] = useState<DashboardSectionKey[]>(defaultSectionOrder);
  const [sectionOrderLoaded, setSectionOrderLoaded] = useState(false);
  const [draggingSection, setDraggingSection] = useState<DashboardSectionKey | null>(null);

  useEffect(() => {
    try {
      const savedSections = window.localStorage.getItem(dashboardSectionStorageKey);
      if (savedSections) {
        setOpenSections(mergeStoredSections(savedSections));
      }
    } catch {
      // Ignore localStorage failures and keep default visibility.
    } finally {
      setOpenSectionsLoaded(true);
    }
  }, []);

  useEffect(() => {
    if (!openSectionsLoaded) {
      return;
    }
    try {
      window.localStorage.setItem(dashboardSectionStorageKey, JSON.stringify(openSections));
    } catch {
      // Ignore localStorage write failures; the dashboard remains usable.
    }
  }, [openSections, openSectionsLoaded]);

  useEffect(() => {
    try {
      const savedOrder = window.localStorage.getItem(dashboardSectionOrderStorageKey);
      if (savedOrder) {
        setSectionOrder(mergeStoredSectionOrder(savedOrder));
      }
    } catch {
      // Ignore localStorage failures and keep default order.
    } finally {
      setSectionOrderLoaded(true);
    }
  }, []);

  useEffect(() => {
    if (!sectionOrderLoaded) {
      return;
    }
    try {
      window.localStorage.setItem(dashboardSectionOrderStorageKey, JSON.stringify(sectionOrder));
    } catch {
      // Ignore localStorage write failures; the dashboard remains usable.
    }
  }, [sectionOrder, sectionOrderLoaded]);

  useEffect(() => {
    let cancelled = false;

    async function loadStatus() {
      try {
        const nextStatus = await fetchStatus();
        if (!cancelled) {
          setStatus(nextStatus);
          setStatusError(null);
        }
      } catch (err) {
        if (!cancelled) {
          setStatusError(err instanceof Error ? err.message : "status request failed");
        }
      }
    }

    async function loadLogs() {
      try {
        const nextLogs = await fetchLogs(logQuery(logRange));
        if (!cancelled) {
          setChartLogs(nextLogs);
          setLogsError(null);
        }
      } catch (err) {
        if (!cancelled) {
          setLogsError(err instanceof Error ? err.message : "logs request failed");
        }
      }
    }

    loadStatus();
    loadLogs();
    const timer = window.setInterval(() => {
      loadStatus();
      loadLogs();
    }, 10000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [logRange]);

  useEffect(() => {
    let cancelled = false;
    let inFlight = false;

    async function loadDelta3Status() {
      if (inFlight) {
        return;
      }
      inFlight = true;
      try {
        const nextStatuses = await fetchDeviceStatuses();
        if (!cancelled) {
          setDeviceStatuses(nextStatuses);
          setDelta3Status(nextStatuses.find((device) => device.kind === "ecoflow_delta3_plus")?.status ?? null);
          setDelta3StatusError(null);
        }
      } catch (err) {
        try {
          const nextStatus = await fetchDelta3Status();
          if (!cancelled) {
            setDeviceStatuses([]);
            setDelta3Status(nextStatus);
            setDelta3StatusError(null);
          }
        } catch (fallbackErr) {
          if (!cancelled) {
            setDeviceStatuses([]);
            setDelta3Status(null);
            setDelta3StatusError(fallbackErr instanceof Error ? fallbackErr.message : err instanceof Error ? err.message : "DELTA 3 status request failed");
          }
        }
      } finally {
        inFlight = false;
      }
    }

    loadDelta3Status();
    const timer = window.setInterval(loadDelta3Status, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function loadLogPage() {
      try {
        const nextPage = await fetchLogsPage({
          limit: logPageSize,
          offset: (logPage - 1) * logPageSize,
          q: appliedLogSearch.trim() || undefined,
          from: datetimeLocalToISOString(appliedLogFrom),
          to: datetimeLocalToISOString(appliedLogTo)
        });
        if (!cancelled) {
          setTableLogs(nextPage.items);
          setLogTotal(nextPage.total);
          setLogsError(null);
          const totalPages = Math.max(1, Math.ceil(nextPage.total / logPageSize));
          if (logPage > totalPages) {
            setLogPage(totalPages);
          }
        }
      } catch (err) {
        if (!cancelled) {
          setLogsError(err instanceof Error ? err.message : "logs request failed");
        }
      }
    }

    loadLogPage();
    const timer = window.setInterval(loadLogPage, 10000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [logPage, appliedLogSearch, appliedLogFrom, appliedLogTo]);

  useEffect(() => {
    let cancelled = false;

    async function loadDryRunPlans() {
      try {
        const [surplusPage, nightPage] = await Promise.all([
          fetchLogsPage({
            limit: dryRunPlanLimit,
            offset: 0,
            q: "surplus dry-run plan"
          }),
          fetchLogsPage({
            limit: dryRunPlanLimit,
            offset: 0,
            q: "night dry-run plan"
          })
        ]);
        if (!cancelled) {
          setDryRunPlanLogs(surplusPage.items);
          setNightDryRunPlanLogs(nightPage.items);
          setDryRunPlanError(null);
          setNightDryRunPlanError(null);
        }
      } catch (err) {
        if (!cancelled) {
          setDryRunPlanError(err instanceof Error ? err.message : "dry-run plan logs request failed");
          setNightDryRunPlanError(err instanceof Error ? err.message : "night dry-run plan logs request failed");
        }
      }
    }

    loadDryRunPlans();
    const timer = window.setInterval(loadDryRunPlans, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function loadNightChargePlanLogPage() {
      try {
        const nextPage = await fetchNightChargePlanLogsPage({
          limit: nightChargePlanLogPageSize,
          offset: (nightChargePlanPage - 1) * nightChargePlanLogPageSize
        });
        if (!cancelled) {
          setNightChargePlanLogs(nextPage.items);
          setNightChargePlanTotal(nextPage.total);
          setNightChargePlanError(null);
          const totalPages = Math.max(1, Math.ceil(nextPage.total / nightChargePlanLogPageSize));
          if (nightChargePlanPage > totalPages) {
            setNightChargePlanPage(totalPages);
          }
        }
      } catch (err) {
        if (!cancelled) {
          setNightChargePlanError(err instanceof Error ? err.message : "night charge plan logs request failed");
        }
      }
    }

    loadNightChargePlanLogPage();
    const timer = window.setInterval(loadNightChargePlanLogPage, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [nightChargePlanPage]);

  useEffect(() => {
    let cancelled = false;

    async function loadSurplusCommandLogPage() {
      try {
        const nextPage = await fetchSurplusControlCommandLogsPage({
          limit: surplusControlCommandLogPageSize,
          offset: (surplusCommandPage - 1) * surplusControlCommandLogPageSize
        });
        if (!cancelled) {
          setSurplusCommandLogs(nextPage.items);
          setSurplusCommandTotal(nextPage.total);
          setSurplusCommandError(null);
          const totalPages = Math.max(1, Math.ceil(nextPage.total / surplusControlCommandLogPageSize));
          if (surplusCommandPage > totalPages) {
            setSurplusCommandPage(totalPages);
          }
        }
      } catch (err) {
        if (!cancelled) {
          setSurplusCommandError(err instanceof Error ? err.message : "surplus control command logs request failed");
        }
      }
    }

    loadSurplusCommandLogPage();
    const timer = window.setInterval(loadSurplusCommandLogPage, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [surplusCommandPage]);

  useEffect(() => {
    let cancelled = false;

    async function loadDelta3AuxCommandLogPage() {
      try {
        const nextPage = await fetchDelta3AuxControlCommandLogsPage({
          limit: delta3AuxControlCommandLogPageSize,
          offset: (delta3AuxCommandPage - 1) * delta3AuxControlCommandLogPageSize
        });
        if (!cancelled) {
          setDelta3AuxCommandLogs(nextPage.items);
          setDelta3AuxCommandTotal(nextPage.total);
          setDelta3AuxCommandError(null);
          const totalPages = Math.max(1, Math.ceil(nextPage.total / delta3AuxControlCommandLogPageSize));
          if (delta3AuxCommandPage > totalPages) {
            setDelta3AuxCommandPage(totalPages);
          }
        }
      } catch (err) {
        if (!cancelled) {
          setDelta3AuxCommandError(err instanceof Error ? err.message : "delta3 auxiliary command logs request failed");
        }
      }
    }

    loadDelta3AuxCommandLogPage();
    const timer = window.setInterval(loadDelta3AuxCommandLogPage, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [delta3AuxCommandPage]);

  useEffect(() => {
    let cancelled = false;

    async function loadRecentDelta3AuxCommandLogs() {
      try {
        const nextPage = await fetchDelta3AuxControlCommandLogsPage({
          limit: delta3AuxControlCommandLogPageSize,
          offset: 0
        });
        if (!cancelled) {
          setRecentDelta3AuxCommandLogs(nextPage.items);
          setRecentDelta3AuxCommandError(null);
        }
      } catch (err) {
        if (!cancelled) {
          setRecentDelta3AuxCommandError(err instanceof Error ? err.message : "recent delta3 auxiliary command logs request failed");
        }
      }
    }

    loadRecentDelta3AuxCommandLogs();
    const timer = window.setInterval(loadRecentDelta3AuxCommandLogs, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function loadNightChargeSummaryPage() {
      try {
        const nextPage = await fetchNightChargeSummariesPage({
          limit: nightChargeSummaryPageSize,
          offset: (nightChargeSummaryPage - 1) * nightChargeSummaryPageSize,
          from: datetimeLocalToISOString(appliedNightChargeSummaryFrom),
          to: datetimeLocalToISOString(appliedNightChargeSummaryTo)
        });
        if (!cancelled) {
          setNightChargeSummaries(nextPage.items);
          setNightChargeSummaryTotal(nextPage.total);
          setNightChargeSummaryError(null);
          const totalPages = Math.max(1, Math.ceil(nextPage.total / nightChargeSummaryPageSize));
          if (nightChargeSummaryPage > totalPages) {
            setNightChargeSummaryPage(totalPages);
          }
        }
      } catch (err) {
        if (!cancelled) {
          setNightChargeSummaryError(err instanceof Error ? err.message : "night charge summaries request failed");
        }
      }
    }

    loadNightChargeSummaryPage();
    const timer = window.setInterval(loadNightChargeSummaryPage, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [nightChargeSummaryPage, appliedNightChargeSummaryFrom, appliedNightChargeSummaryTo]);

  useEffect(() => {
    let cancelled = false;

    async function loadLatestNightChargeSummary() {
      try {
        const nextPage = await fetchNightChargeSummariesPage({
          limit: 1,
          offset: 0
        });
        if (!cancelled) {
          setLatestNightChargeSummary(nextPage.items[0] ?? null);
          setLatestNightChargeSummaryError(null);
        }
      } catch (err) {
        if (!cancelled) {
          setLatestNightChargeSummary(null);
          setLatestNightChargeSummaryError(err instanceof Error ? err.message : "latest night charge summary request failed");
        }
      }
    }

    loadLatestNightChargeSummary();
    const timer = window.setInterval(loadLatestNightChargeSummary, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function loadEnergyMeterLogPage() {
      try {
        const nextPage = await fetchEnergyMeterLogsPage({
          limit: energyMeterLogPageSize,
          offset: (energyMeterPage - 1) * energyMeterLogPageSize,
          from: datetimeLocalToISOString(appliedEnergyMeterFrom),
          to: datetimeLocalToISOString(appliedEnergyMeterTo)
        });
        if (!cancelled) {
          setEnergyMeterLogs(nextPage.items);
          setEnergyMeterTotal(nextPage.total);
          setEnergyMeterError(null);
          const totalPages = Math.max(1, Math.ceil(nextPage.total / energyMeterLogPageSize));
          if (energyMeterPage > totalPages) {
            setEnergyMeterPage(totalPages);
          }
        }
      } catch (err) {
        if (!cancelled) {
          setEnergyMeterError(err instanceof Error ? err.message : "energy meter logs request failed");
        }
      }
    }

    loadEnergyMeterLogPage();
    const timer = window.setInterval(loadEnergyMeterLogPage, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [energyMeterPage, appliedEnergyMeterFrom, appliedEnergyMeterTo]);

  useEffect(() => {
    let cancelled = false;

    async function loadTariffSummary() {
      try {
        const nextSummary = await fetchTariffSummary({
          from: datetimeLocalToISOString(appliedTariffFrom),
          to: datetimeLocalToISOString(appliedTariffTo)
        });
        if (!cancelled) {
          setTariffSummary(nextSummary);
          setTariffError(null);
        }
      } catch (err) {
        if (!cancelled) {
          setTariffError(err instanceof Error ? err.message : "tariff summary request failed");
        }
      }
    }

    loadTariffSummary();
    const timer = window.setInterval(loadTariffSummary, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [appliedTariffFrom, appliedTariffTo, tariffRefreshToken]);

  useEffect(() => {
    let cancelled = false;

    async function loadSolarForecast() {
      setSolarForecastLoading(true);
      try {
        const nextForecast = await fetchSolarForecast(forecastRange.days);
        if (!cancelled) {
          setSolarForecast(nextForecast);
          setSolarForecastError(null);
        }
      } catch (err) {
        if (!cancelled) {
          setSolarForecastError(err instanceof Error ? err.message : "solar forecast request failed");
        }
      } finally {
        if (!cancelled) {
          setSolarForecastLoading(false);
        }
      }
    }

    loadSolarForecast();
    const timer = window.setInterval(loadSolarForecast, 30 * 60 * 1000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [forecastRange]);

  function submitLogSearch() {
    setAppliedLogSearch(logSearchInput);
    setAppliedLogFrom(logFromInput);
    setAppliedLogTo(logToInput);
    setLogPage(1);
  }

  function clearLogSearch() {
    setLogSearchInput("");
    setLogFromInput("");
    setLogToInput("");
    setAppliedLogSearch("");
    setAppliedLogFrom("");
    setAppliedLogTo("");
    setLogPage(1);
  }

  function submitEnergyMeterSearch() {
    setAppliedEnergyMeterFrom(energyMeterFromInput);
    setAppliedEnergyMeterTo(energyMeterToInput);
    setEnergyMeterPage(1);
  }

  function clearEnergyMeterSearch() {
    setEnergyMeterFromInput("");
    setEnergyMeterToInput("");
    setAppliedEnergyMeterFrom("");
    setAppliedEnergyMeterTo("");
    setEnergyMeterPage(1);
  }

  function submitNightChargeSummarySearch() {
    setAppliedNightChargeSummaryFrom(nightChargeSummaryFromInput);
    setAppliedNightChargeSummaryTo(nightChargeSummaryToInput);
    setNightChargeSummaryPage(1);
  }

  function clearNightChargeSummarySearch() {
    setNightChargeSummaryFromInput("");
    setNightChargeSummaryToInput("");
    setAppliedNightChargeSummaryFrom("");
    setAppliedNightChargeSummaryTo("");
    setNightChargeSummaryPage(1);
  }

  function submitTariffSearch() {
    setAppliedTariffFrom(tariffFromInput);
    setAppliedTariffTo(tariffToInput);
  }

  function clearTariffSearch() {
    setTariffFromInput("");
    setTariffToInput("");
    setAppliedTariffFrom("");
    setAppliedTariffTo("");
  }

  function toggleSection(section: DashboardSectionKey) {
    setOpenSections((current) => ({ ...current, [section]: !current[section] }));
  }

  function moveSection(section: DashboardSectionKey, direction: -1 | 1) {
    setSectionOrder((current) => moveSectionInVisibleOrder(current, section, direction, (candidate) => isDashboardSectionVisible(candidate, status)));
  }

  function moveDraggedSection(targetSection: DashboardSectionKey) {
    if (!draggingSection || draggingSection === targetSection) {
      return;
    }
    setSectionOrder((current) => moveSectionNearTarget(current, draggingSection, targetSection));
  }

  function renderDashboardBlock(section: DashboardSectionKey, headerControls: ReactNode) {
    switch (section) {
      case "charts":
        return (
          <StatusChartSection
            open={openSections.charts}
            onToggle={() => toggleSection("charts")}
            summary={`Grid / Battery / ${status.updatedAt ? `更新 ${formatDateTime(status.updatedAt)}` : "更新待ち"}`}
            headerControls={headerControls}
          >
            <EnergyCharts logs={chartLogs} rangeLabel={logRange.label} ranges={logRanges} selectedRange={logRange} onRangeChange={setLogRange} />
          </StatusChartSection>
        );
      case "decision":
        return <StatusDecisionSection status={status} open={openSections.decision} onToggle={() => toggleSection("decision")} headerControls={headerControls} />;
      case "surplusPlan":
        return status.surplusPlan ? (
          <SurplusPlanSection plan={status.surplusPlan} open={openSections.surplusPlan} onToggle={() => toggleSection("surplusPlan")} headerControls={headerControls} />
        ) : null;
      case "nightPlan":
        return status.nightChargePlan ? (
          <NightChargePlanSection plan={status.nightChargePlan} open={openSections.nightPlan} onToggle={() => toggleSection("nightPlan")} headerControls={headerControls} />
        ) : null;
      case "verification":
        return (
          <CollapsibleSection
            title="実証検証"
            summary={verificationSectionSummary(latestNightChargeSummary, latestNightChargeSummaryError, recentDelta3AuxCommandLogs, recentDelta3AuxCommandError)}
            open={openSections.verification}
            onToggle={() => toggleSection("verification")}
            headerControls={headerControls}
          >
            <VerificationPanel
              latestNightSummary={latestNightChargeSummary}
              latestNightSummaryError={latestNightChargeSummaryError}
              status={status}
              delta3Status={delta3Status}
              delta3StatusError={delta3StatusError}
              recentDelta3AuxLogs={recentDelta3AuxCommandLogs}
              recentDelta3AuxError={recentDelta3AuxCommandError}
            />
          </CollapsibleSection>
        );
      case "settings":
        return (
          <CollapsibleSection
            title="設定"
            summary="自動制御 / 設定値更新 / 手動シミュレーション / 設置場所 / 料金プラン"
            open={openSections.settings}
            onToggle={() => toggleSection("settings")}
            headerControls={headerControls}
          >
            <ControlPanel onTariffPlanSaved={() => setTariffRefreshToken((value) => value + 1)} />
          </CollapsibleSection>
        );
      case "solarForecast":
        return (
          <CollapsibleSection
            title="発電予測"
            summary={solarForecastSummary(solarForecast, forecastRange.label, solarForecastError)}
            open={openSections.solarForecast}
            onToggle={() => toggleSection("solarForecast")}
            headerControls={headerControls}
          >
            <SolarForecastPanel
              summary={solarForecast}
              error={solarForecastError}
              ranges={forecastRanges}
              selectedRange={forecastRange}
              loading={solarForecastLoading}
              onRangeChange={setForecastRange}
            />
          </CollapsibleSection>
        );
      case "tariffSummary":
        return (
          <CollapsibleSection
            title="料金概算"
            summary={tariffSummaryLabel(tariffSummary, tariffError)}
            open={openSections.tariffSummary}
            onToggle={() => toggleSection("tariffSummary")}
            headerControls={headerControls}
          >
            <TariffSummaryPanel
              summary={tariffSummary}
              error={tariffError}
              from={tariffFromInput}
              to={tariffToInput}
              isFiltered={Boolean(appliedTariffFrom || appliedTariffTo)}
              onFromChange={setTariffFromInput}
              onToChange={setTariffToInput}
              onSearchSubmit={submitTariffSearch}
              onSearchClear={clearTariffSearch}
            />
          </CollapsibleSection>
        );
      case "nightDryRun":
        return (
          <CollapsibleSection
            title="夜間制御 dry-run 履歴"
            summary={logSectionSummary(nightDryRunPlanLogs.length, nightDryRunPlanError)}
            open={openSections.nightDryRun}
            onToggle={() => toggleSection("nightDryRun")}
            headerControls={headerControls}
          >
            <DryRunPlanHistory
              logs={nightDryRunPlanLogs}
              error={nightDryRunPlanError}
              title="夜間制御 dry-run 履歴"
              marker="night dry-run plan:"
              emptyMessage="夜間制御 dry-run 計画はまだ記録されていません。"
            />
          </CollapsibleSection>
        );
      case "surplusCommand":
        return (
          <CollapsibleSection
            title="余剰追従 実行ログ"
            summary={pagedLogSectionSummary(surplusCommandTotal, surplusCommandPage, surplusControlCommandLogPageSize, surplusCommandError)}
            open={openSections.surplusCommand}
            onToggle={() => toggleSection("surplusCommand")}
            headerControls={headerControls}
          >
            <SurplusControlCommandLogTable
              logs={surplusCommandLogs}
              error={surplusCommandError}
              page={surplusCommandPage}
              pageSize={surplusControlCommandLogPageSize}
              total={surplusCommandTotal}
              onPageChange={setSurplusCommandPage}
            />
          </CollapsibleSection>
        );
      case "delta3AuxCommand":
        return (
          <CollapsibleSection
            title="DELTA 3 Plus 補助充電ログ"
            summary={pagedLogSectionSummary(delta3AuxCommandTotal, delta3AuxCommandPage, delta3AuxControlCommandLogPageSize, delta3AuxCommandError)}
            open={openSections.delta3AuxCommand}
            onToggle={() => toggleSection("delta3AuxCommand")}
            headerControls={headerControls}
          >
            <AuxiliaryBatteryControlCommandLogTable
              logs={delta3AuxCommandLogs}
              error={delta3AuxCommandError}
              page={delta3AuxCommandPage}
              pageSize={delta3AuxControlCommandLogPageSize}
              total={delta3AuxCommandTotal}
              onPageChange={setDelta3AuxCommandPage}
            />
          </CollapsibleSection>
        );
      case "nightPlanLog":
        return (
          <CollapsibleSection
            title="夜間充電計画ログ"
            summary={pagedLogSectionSummary(nightChargePlanTotal, nightChargePlanPage, nightChargePlanLogPageSize, nightChargePlanError)}
            open={openSections.nightPlanLog}
            onToggle={() => toggleSection("nightPlanLog")}
            headerControls={headerControls}
          >
            <NightChargePlanLogTable
              logs={nightChargePlanLogs}
              error={nightChargePlanError}
              page={nightChargePlanPage}
              pageSize={nightChargePlanLogPageSize}
              total={nightChargePlanTotal}
              onPageChange={setNightChargePlanPage}
            />
          </CollapsibleSection>
        );
      case "nightSummary":
        return (
          <CollapsibleSection
            title="夜間充電 日次検証ログ"
            summary={pagedLogSectionSummary(nightChargeSummaryTotal, nightChargeSummaryPage, nightChargeSummaryPageSize, nightChargeSummaryError)}
            open={openSections.nightSummary}
            onToggle={() => toggleSection("nightSummary")}
            headerControls={headerControls}
          >
            <NightChargeSummaryTable
              summaries={nightChargeSummaries}
              error={nightChargeSummaryError}
              page={nightChargeSummaryPage}
              pageSize={nightChargeSummaryPageSize}
              total={nightChargeSummaryTotal}
              from={nightChargeSummaryFromInput}
              to={nightChargeSummaryToInput}
              isFiltered={Boolean(appliedNightChargeSummaryFrom || appliedNightChargeSummaryTo)}
              onFromChange={setNightChargeSummaryFromInput}
              onToChange={setNightChargeSummaryToInput}
              onSearchSubmit={submitNightChargeSummarySearch}
              onSearchClear={clearNightChargeSummarySearch}
              onPageChange={setNightChargeSummaryPage}
            />
          </CollapsibleSection>
        );
      case "surplusDryRun":
        return (
          <CollapsibleSection
            title="余剰追従 dry-run 履歴"
            summary={logSectionSummary(dryRunPlanLogs.length, dryRunPlanError)}
            open={openSections.surplusDryRun}
            onToggle={() => toggleSection("surplusDryRun")}
            headerControls={headerControls}
          >
            <DryRunPlanHistory logs={dryRunPlanLogs} error={dryRunPlanError} />
          </CollapsibleSection>
        );
      case "energyMeter":
        return (
          <CollapsibleSection
            title="電力量ログ"
            summary={pagedLogSectionSummary(energyMeterTotal, energyMeterPage, energyMeterLogPageSize, energyMeterError)}
            open={openSections.energyMeter}
            onToggle={() => toggleSection("energyMeter")}
            headerControls={headerControls}
          >
            <EnergyMeterLogTable
              logs={energyMeterLogs}
              error={energyMeterError}
              page={energyMeterPage}
              pageSize={energyMeterLogPageSize}
              total={energyMeterTotal}
              from={energyMeterFromInput}
              to={energyMeterToInput}
              isFiltered={Boolean(appliedEnergyMeterFrom || appliedEnergyMeterTo)}
              onFromChange={setEnergyMeterFromInput}
              onToChange={setEnergyMeterToInput}
              onSearchSubmit={submitEnergyMeterSearch}
              onSearchClear={clearEnergyMeterSearch}
              onPageChange={setEnergyMeterPage}
            />
          </CollapsibleSection>
        );
      case "controlLog":
        return (
          <CollapsibleSection
            title="制御ログ"
            summary={pagedLogSectionSummary(logTotal, logPage, logPageSize, logsError)}
            open={openSections.controlLog}
            onToggle={() => toggleSection("controlLog")}
            headerControls={headerControls}
          >
            <LogTable
              logs={tableLogs}
              error={logsError}
              page={logPage}
              pageSize={logPageSize}
              total={logTotal}
              search={logSearchInput}
              from={logFromInput}
              to={logToInput}
              isFiltered={Boolean(appliedLogSearch || appliedLogFrom || appliedLogTo)}
              onSearchChange={setLogSearchInput}
              onFromChange={setLogFromInput}
              onToChange={setLogToInput}
              onSearchSubmit={submitLogSearch}
              onSearchClear={clearLogSearch}
              onPageChange={setLogPage}
            />
          </CollapsibleSection>
        );
    }
  }

  const visibleSectionOrder = sectionOrder.filter((section) => isDashboardSectionVisible(section, status));

  return (
    <main className="page-shell">
      <Header status={status} />
      <StatusCards status={status} fetchError={statusError} />
      <Delta3StatusCard status={delta3Status} deviceStatuses={deviceStatuses} sourceStatus={status} fetchError={delta3StatusError} />
      <section className="sortable-dashboard" aria-label="dashboard blocks">
        {visibleSectionOrder.map((section, index) => {
          const sortControls = (
            <DashboardSortControls
              section={section}
              draggableLabel={dashboardSectionLabel(section)}
              canMoveUp={index > 0}
              canMoveDown={index < visibleSectionOrder.length - 1}
              onMoveUp={() => moveSection(section, -1)}
              onMoveDown={() => moveSection(section, 1)}
              onDragStart={() => setDraggingSection(section)}
            />
          );
          const block = renderDashboardBlock(section, sortControls);
          if (!block) {
            return null;
          }
          return (
            <SortableDashboardBlock
              key={section}
              section={section}
              isDragging={draggingSection === section}
              onDrop={() => moveDraggedSection(section)}
              onDragEnd={() => setDraggingSection(null)}
            >
              {block}
            </SortableDashboardBlock>
          );
        })}
      </section>
    </main>
  );
}

function SortableDashboardBlock({
  section,
  isDragging,
  onDrop,
  onDragEnd,
  children
}: {
  section: DashboardSectionKey;
  isDragging: boolean;
  onDrop: () => void;
  onDragEnd: () => void;
  children: ReactNode;
}) {
  return (
    <div
      className={`sortable-dashboard-block${isDragging ? " is-dragging" : ""}`}
      data-dashboard-block={section}
      onDragOver={(event) => {
        event.preventDefault();
        event.dataTransfer.dropEffect = "move";
      }}
      onDrop={(event) => {
        event.preventDefault();
        onDrop();
        onDragEnd();
      }}
      onDragEnd={onDragEnd}
    >
      {children}
    </div>
  );
}

function DashboardSortControls({
  section,
  draggableLabel,
  canMoveUp,
  canMoveDown,
  onMoveUp,
  onMoveDown,
  onDragStart
}: {
  section: DashboardSectionKey;
  draggableLabel: string;
  canMoveUp: boolean;
  canMoveDown: boolean;
  onMoveUp: () => void;
  onMoveDown: () => void;
  onDragStart: () => void;
}) {
  return (
    <div className="sortable-dashboard-controls" aria-label={`${draggableLabel} の並び替え`}>
      <span
        className="sortable-dashboard-handle"
        draggable
        role="img"
        aria-label={`${draggableLabel} をドラッグして移動`}
        onDragStart={(event) => {
          event.dataTransfer.effectAllowed = "move";
          event.dataTransfer.setData("text/plain", section);
          onDragStart();
        }}
      >
        ::
      </span>
      <Button type="button" variant="outline" className="sortable-dashboard-button" disabled={!canMoveUp} onClick={onMoveUp}>
        上へ
      </Button>
      <Button type="button" variant="outline" className="sortable-dashboard-button" disabled={!canMoveDown} onClick={onMoveDown}>
        下へ
      </Button>
    </div>
  );
}

function logSectionSummary(count: number, error: string | null) {
  if (error) {
    return `取得エラー: ${error}`;
  }
  return `${count}件表示`;
}

function mergeStoredSections(savedSections: string) {
  const parsed = JSON.parse(savedSections);
  if (!parsed || typeof parsed !== "object") {
    return initialDashboardSections;
  }
  return Object.entries(initialDashboardSections).reduce<Record<DashboardSectionKey, boolean>>((nextSections, [section, defaultOpen]) => {
    const storedValue = (parsed as Record<string, unknown>)[section];
    nextSections[section as DashboardSectionKey] = typeof storedValue === "boolean" ? storedValue : defaultOpen;
    return nextSections;
  }, { ...initialDashboardSections });
}

function mergeStoredSectionOrder(savedOrder: string) {
  const parsed = JSON.parse(savedOrder);
  if (!Array.isArray(parsed)) {
    return defaultSectionOrder;
  }
  const validStoredSections = parsed.filter((section): section is DashboardSectionKey => isDashboardSectionKey(section));
  const missingSections = defaultSectionOrder.filter((section) => !validStoredSections.includes(section));
  return [...validStoredSections, ...missingSections];
}

function isDashboardSectionKey(value: unknown): value is DashboardSectionKey {
  return typeof value === "string" && defaultSectionOrder.includes(value as DashboardSectionKey);
}

function moveSectionInVisibleOrder(
  current: DashboardSectionKey[],
  section: DashboardSectionKey,
  direction: -1 | 1,
  isVisible: (section: DashboardSectionKey) => boolean
) {
  const visibleSections = current.filter(isVisible);
  const fromVisibleIndex = visibleSections.indexOf(section);
  const toVisibleIndex = fromVisibleIndex + direction;
  if (fromVisibleIndex < 0 || toVisibleIndex < 0 || toVisibleIndex >= visibleSections.length) {
    return current;
  }
  return moveSectionNearTarget(current, section, visibleSections[toVisibleIndex]);
}

function isDashboardSectionVisible(section: DashboardSectionKey, status: EnergyStatus) {
  if (section === "surplusPlan") {
    return Boolean(status.surplusPlan);
  }
  if (section === "nightPlan") {
    return Boolean(status.nightChargePlan);
  }
  return true;
}

function moveSectionNearTarget(current: DashboardSectionKey[], draggingSection: DashboardSectionKey, targetSection: DashboardSectionKey) {
  const fromIndex = current.indexOf(draggingSection);
  const targetIndex = current.indexOf(targetSection);
  if (fromIndex < 0 || targetIndex < 0 || fromIndex === targetIndex) {
    return current;
  }
  const next = [...current];
  const [item] = next.splice(fromIndex, 1);
  const insertionIndex = fromIndex < targetIndex ? targetIndex : targetIndex;
  next.splice(insertionIndex, 0, item);
  return next;
}

function dashboardSectionLabel(section: DashboardSectionKey) {
  const labels: Record<DashboardSectionKey, string> = {
    charts: "推移グラフ",
    decision: "制御判断",
    surplusPlan: "余剰追従プラン",
    nightPlan: "深夜充電プラン",
    verification: "実証検証",
    settings: "設定",
    solarForecast: "発電予測",
    tariffSummary: "料金概算",
    nightDryRun: "夜間制御 dry-run 履歴",
    surplusCommand: "余剰追従 実行ログ",
    delta3AuxCommand: "DELTA 3 Plus 補助充電ログ",
    nightPlanLog: "夜間充電計画ログ",
    nightSummary: "夜間充電 日次検証ログ",
    surplusDryRun: "余剰追従 dry-run 履歴",
    energyMeter: "電力量ログ",
    controlLog: "制御ログ"
  };
  return labels[section];
}

function verificationSectionSummary(
  latestNightSummary: NightChargeDailySummary | null,
  latestNightSummaryError: string | null,
  recentDelta3AuxLogs: Delta3AuxControlCommandLog[],
  recentDelta3AuxError: string | null
) {
  if (latestNightSummaryError || recentDelta3AuxError) {
    return `取得エラー: ${latestNightSummaryError || recentDelta3AuxError}`;
  }
  const summaryDate = latestNightSummary?.summaryDate ?? "夜間サマリー待ち";
  return `${summaryDate} / DELTA 3 Plus直近${recentDelta3AuxLogs.length}件`;
}

function solarForecastSummary(summary: SolarForecastSummary | null, rangeLabel: string, error: string | null) {
  if (error) {
    return `取得エラー: ${error}`;
  }
  if (!summary) {
    return `${rangeLabel} / 取得待ち`;
  }
  return `${rangeLabel} / ${summary.items.length}日分`;
}

function tariffSummaryLabel(summary: TariffSummary | null, error: string | null) {
  if (error) {
    return `取得エラー: ${error}`;
  }
  if (!summary) {
    return "取得待ち";
  }
  return `${summary.planName} / ${summary.sampleCount}件`;
}

function formatDateTime(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString("ja-JP");
}

function pagedLogSectionSummary(total: number, page: number, pageSize: number, error: string | null) {
  if (error) {
    return `取得エラー: ${error}`;
  }
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  return `${total}件 / ${page}ページ目 of ${totalPages}`;
}

function datetimeLocalToISOString(value: string) {
  if (!value) {
    return undefined;
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return undefined;
  }
  return date.toISOString();
}

function logQuery(range: LogRange) {
  if (range.hours === null) {
    return { limit: 500 };
  }
  return { since: new Date(Date.now() - range.hours * 60 * 60 * 1000).toISOString() };
}
