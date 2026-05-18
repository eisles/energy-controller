"use client";

import { useEffect, useState } from "react";
import { ControlPanel } from "@/components/ControlPanel";
import { Header } from "@/components/Header";
import { LogTable } from "@/components/LogTable";
import { StatusCards } from "@/components/StatusCards";
import { fetchLogs, fetchStatus } from "@/lib/api";
import type { EnergyStatus, PowerLog } from "@/lib/types";

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
        const nextLogs = await fetchLogs(100);
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
  }, []);

  return (
    <main className="page-shell">
      <Header status={status} />
      <StatusCards status={status} fetchError={statusError} />
      <LogTable logs={logs} error={logsError} />
      <ControlPanel />
    </main>
  );
}
