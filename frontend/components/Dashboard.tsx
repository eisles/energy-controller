"use client";

import type { ReactNode } from "react";
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
import { SurplusControlCommandLogTable } from "@/components/SurplusControlCommandLogTable";
import { TariffSummaryPanel } from "@/components/TariffSummaryPanel";
import { Button } from "@/components/ui/button";
import {
  fetchEnergyMeterLogsPage,
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
const dryRunPlanLimit = 10;

type LogSectionKey =
  | "nightDryRun"
  | "surplusCommand"
  | "nightPlan"
  | "nightSummary"
  | "surplusDryRun"
  | "energyMeter"
  | "control";

const initialLogSections: Record<LogSectionKey, boolean> = {
  nightDryRun: false,
  surplusCommand: false,
  nightPlan: false,
  nightSummary: false,
  surplusDryRun: false,
  energyMeter: false,
  control: false
};

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
  const [surplusCommandLogs, setSurplusCommandLogs] = useState<SurplusControlCommandLog[]>([]);
  const [surplusCommandPage, setSurplusCommandPage] = useState(1);
  const [surplusCommandTotal, setSurplusCommandTotal] = useState(0);
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
  const [surplusCommandError, setSurplusCommandError] = useState<string | null>(null);
  const [nightChargeSummaryError, setNightChargeSummaryError] = useState<string | null>(null);
  const [energyMeterError, setEnergyMeterError] = useState<string | null>(null);
  const [tariffError, setTariffError] = useState<string | null>(null);
  const [solarForecastError, setSolarForecastError] = useState<string | null>(null);
  const [solarForecastLoading, setSolarForecastLoading] = useState(false);
  const [openLogSections, setOpenLogSections] = useState<Record<LogSectionKey, boolean>>(initialLogSections);

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

  function toggleLogSection(section: LogSectionKey) {
    setOpenLogSections((current) => ({ ...current, [section]: !current[section] }));
  }

  return (
    <main className="page-shell">
      <Header status={status} />
      <StatusCards
        status={status}
        fetchError={statusError}
        chartSlot={<EnergyCharts logs={chartLogs} rangeLabel={logRange.label} ranges={logRanges} selectedRange={logRange} onRangeChange={setLogRange} />}
      />
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
      <CollapsibleLogSection
        title="夜間制御 dry-run 履歴"
        summary={logSectionSummary(nightDryRunPlanLogs.length, nightDryRunPlanError)}
        open={openLogSections.nightDryRun}
        onToggle={() => toggleLogSection("nightDryRun")}
      >
        <DryRunPlanHistory
          logs={nightDryRunPlanLogs}
          error={nightDryRunPlanError}
          title="夜間制御 dry-run 履歴"
          marker="night dry-run plan:"
          emptyMessage="夜間制御 dry-run 計画はまだ記録されていません。"
        />
      </CollapsibleLogSection>
      <CollapsibleLogSection
        title="余剰追従 実行ログ"
        summary={pagedLogSectionSummary(surplusCommandTotal, surplusCommandPage, surplusControlCommandLogPageSize, surplusCommandError)}
        open={openLogSections.surplusCommand}
        onToggle={() => toggleLogSection("surplusCommand")}
      >
        <SurplusControlCommandLogTable
          logs={surplusCommandLogs}
          error={surplusCommandError}
          page={surplusCommandPage}
          pageSize={surplusControlCommandLogPageSize}
          total={surplusCommandTotal}
          onPageChange={setSurplusCommandPage}
        />
      </CollapsibleLogSection>
      <CollapsibleLogSection
        title="夜間充電計画ログ"
        summary={pagedLogSectionSummary(nightChargePlanTotal, nightChargePlanPage, nightChargePlanLogPageSize, nightChargePlanError)}
        open={openLogSections.nightPlan}
        onToggle={() => toggleLogSection("nightPlan")}
      >
        <NightChargePlanLogTable
          logs={nightChargePlanLogs}
          error={nightChargePlanError}
          page={nightChargePlanPage}
          pageSize={nightChargePlanLogPageSize}
          total={nightChargePlanTotal}
          onPageChange={setNightChargePlanPage}
        />
      </CollapsibleLogSection>
      <CollapsibleLogSection
        title="夜間充電 日次検証ログ"
        summary={pagedLogSectionSummary(nightChargeSummaryTotal, nightChargeSummaryPage, nightChargeSummaryPageSize, nightChargeSummaryError)}
        open={openLogSections.nightSummary}
        onToggle={() => toggleLogSection("nightSummary")}
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
      </CollapsibleLogSection>
      <CollapsibleLogSection
        title="余剰追従 dry-run 履歴"
        summary={logSectionSummary(dryRunPlanLogs.length, dryRunPlanError)}
        open={openLogSections.surplusDryRun}
        onToggle={() => toggleLogSection("surplusDryRun")}
      >
        <DryRunPlanHistory logs={dryRunPlanLogs} error={dryRunPlanError} />
      </CollapsibleLogSection>
      <CollapsibleLogSection
        title="電力量ログ"
        summary={pagedLogSectionSummary(energyMeterTotal, energyMeterPage, energyMeterLogPageSize, energyMeterError)}
        open={openLogSections.energyMeter}
        onToggle={() => toggleLogSection("energyMeter")}
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
      </CollapsibleLogSection>
      <CollapsibleLogSection
        title="制御ログ"
        summary={pagedLogSectionSummary(logTotal, logPage, logPageSize, logsError)}
        open={openLogSections.control}
        onToggle={() => toggleLogSection("control")}
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
      </CollapsibleLogSection>
    </main>
  );
}

function CollapsibleLogSection({
  title,
  summary,
  open,
  onToggle,
  children
}: {
  title: string;
  summary: string;
  open: boolean;
  onToggle: () => void;
  children: ReactNode;
}) {
  return (
    <section className="collapsible-log-section section">
      <div className="collapsible-log-header">
        <div>
          <h2>{title}</h2>
          <p>{summary}</p>
        </div>
        <Button type="button" variant="outline" className="collapsible-log-toggle" aria-expanded={open} onClick={onToggle}>
          <span className="collapsible-log-toggle-icon" aria-hidden="true">
            {open ? "-" : "+"}
          </span>
          {open ? "閉じる" : "開く"}
        </Button>
      </div>
      {open ? <div className="collapsible-log-body">{children}</div> : null}
    </section>
  );
}

function logSectionSummary(count: number, error: string | null) {
  if (error) {
    return `取得エラー: ${error}`;
  }
  return `${count}件表示`;
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
