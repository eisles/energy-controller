"use client";

import { useEffect, useState } from "react";
import { ControlPanel } from "@/components/ControlPanel";
import { EnergyCharts } from "@/components/EnergyCharts";
import { Header } from "@/components/Header";
import { LogTable } from "@/components/LogTable";
import { StatusCards } from "@/components/StatusCards";
import { fetchLogs, fetchStatus } from "@/lib/api";
import type { EnergyStatus, PowerLog } from "@/lib/types";

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
  const [logs, setLogs] = useState<PowerLog[]>([]);
  const [logRange, setLogRange] = useState<LogRange>(logRanges[1]);
  const [statusError, setStatusError] = useState<string | null>(null);
  const [logsError, setLogsError] = useState<string | null>(null);

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
          setLogs(nextLogs);
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

  return (
    <main className="page-shell">
      <Header status={status} />
      <StatusCards status={status} fetchError={statusError} />
      <EnergyCharts logs={logs} rangeLabel={logRange.label} ranges={logRanges} selectedRange={logRange} onRangeChange={setLogRange} />
      <LogTable logs={logs} error={logsError} />
      <ControlPanel />
    </main>
  );
}

function logQuery(range: LogRange) {
  if (range.hours === null) {
    return { limit: 500 };
  }
  return { since: new Date(Date.now() - range.hours * 60 * 60 * 1000).toISOString() };
}
