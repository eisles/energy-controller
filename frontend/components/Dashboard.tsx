"use client";

import { useEffect, useState } from "react";
import { ControlPanel } from "@/components/ControlPanel";
import { DryRunPlanHistory } from "@/components/DryRunPlanHistory";
import { EnergyCharts } from "@/components/EnergyCharts";
import { EnergyMeterLogTable } from "@/components/EnergyMeterLogTable";
import { Header } from "@/components/Header";
import { LogTable } from "@/components/LogTable";
import { NightChargePlanLogTable } from "@/components/NightChargePlanLogTable";
import { NightChargeSummaryTable } from "@/components/NightChargeSummaryTable";
import { SolarForecastPanel } from "@/components/SolarForecastPanel";
import { StatusCards } from "@/components/StatusCards";
import { TariffSummaryPanel } from "@/components/TariffSummaryPanel";
import {
  fetchEnergyMeterLogsPage,
  fetchLogs,
  fetchLogsPage,
  fetchNightChargePlanLogsPage,
  fetchNightChargeSummariesPage,
  fetchSolarForecast,
  fetchStatus,
  fetchTariffSummary
} from "@/lib/api";
import type { EnergyMeterLog, EnergyStatus, NightChargeDailySummary, NightChargePlanLog, PowerLog, SolarForecastSummary, TariffSummary } from "@/lib/types";

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
const dryRunPlanLimit = 10;

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
  lastDecisionReason: "loading mock simulation status",
  lastError: null,
  updatedAt: ""
};

export function Dashboard() {
  const [status, setStatus] = useState<EnergyStatus>(initialStatus);
  const [chartLogs, setChartLogs] = useState<PowerLog[]>([]);
  const [tableLogs, setTableLogs] = useState<PowerLog[]>([]);
  const [dryRunPlanLogs, setDryRunPlanLogs] = useState<PowerLog[]>([]);
  const [nightDryRunPlanLogs, setNightDryRunPlanLogs] = useState<PowerLog[]>([]);
  const [nightChargePlanLogs, setNightChargePlanLogs] = useState<NightChargePlanLog[]>([]);
  const [nightChargePlanPage, setNightChargePlanPage] = useState(1);
  const [nightChargePlanTotal, setNightChargePlanTotal] = useState(0);
  const [nightChargeSummaries, setNightChargeSummaries] = useState<NightChargeDailySummary[]>([]);
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
  const [logsError, setLogsError] = useState<string | null>(null);
  const [dryRunPlanError, setDryRunPlanError] = useState<string | null>(null);
  const [nightDryRunPlanError, setNightDryRunPlanError] = useState<string | null>(null);
  const [nightChargePlanError, setNightChargePlanError] = useState<string | null>(null);
  const [nightChargeSummaryError, setNightChargeSummaryError] = useState<string | null>(null);
  const [energyMeterError, setEnergyMeterError] = useState<string | null>(null);
  const [tariffError, setTariffError] = useState<string | null>(null);
  const [solarForecastError, setSolarForecastError] = useState<string | null>(null);
  const [solarForecastLoading, setSolarForecastLoading] = useState(false);

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
    }, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, [logRange]);

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
    const timer = window.setInterval(loadLogPage, 5000);
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

  return (
    <main className="page-shell">
      <Header status={status} />
      <StatusCards status={status} fetchError={statusError} />
      <DryRunPlanHistory
        logs={nightDryRunPlanLogs}
        error={nightDryRunPlanError}
        title="夜間制御 dry-run 履歴"
        marker="night dry-run plan:"
        emptyMessage="夜間制御 dry-run 計画はまだ記録されていません。"
      />
      <NightChargePlanLogTable
        logs={nightChargePlanLogs}
        error={nightChargePlanError}
        page={nightChargePlanPage}
        pageSize={nightChargePlanLogPageSize}
        total={nightChargePlanTotal}
        onPageChange={setNightChargePlanPage}
      />
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
      <DryRunPlanHistory logs={dryRunPlanLogs} error={dryRunPlanError} />
      <EnergyCharts logs={chartLogs} rangeLabel={logRange.label} ranges={logRanges} selectedRange={logRange} onRangeChange={setLogRange} />
      <ControlPanel onTariffPlanSaved={() => setTariffRefreshToken((value) => value + 1)} />
      <SolarForecastPanel
        summary={solarForecast}
        error={solarForecastError}
        ranges={forecastRanges}
        selectedRange={forecastRange}
        loading={solarForecastLoading}
        onRangeChange={setForecastRange}
      />
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
    </main>
  );
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
