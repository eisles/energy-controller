"use client";

import { useEffect, useState } from "react";
import { fetchStatus } from "../lib/api";
import type { EnergyStatus } from "../lib/types";

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
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const nextStatus = await fetchStatus();
        if (!cancelled) {
          setStatus(nextStatus);
          setError(null);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "status request failed");
        }
      }
    }

    load();
    const timer = window.setInterval(load, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(timer);
    };
  }, []);

  return (
    <main>
      <header className="header">
        <div>
          <h1 className="title">Energy Controller</h1>
          <p className="subtitle">Nature Remo E と EcoFlow DELTA Pro 3 の mock simulation 管理画面</p>
        </div>
        <div className="badge">MOCK + SIMULATION</div>
      </header>

      <section className="grid" aria-label="current energy status">
        <Metric label="Grid" value={status.gridW} unit="W" />
        <Metric label="Import" value={status.importW} unit="W" />
        <Metric label="Export" value={status.exportW} unit="W" />
        <Metric label="Target charge" value={status.targetChargeW} unit="W" />
        <Metric label="Battery SOC" value={status.batterySoc} unit="%" />
        <Metric label="Battery input" value={status.batteryInputW} unit="W" />
        <Metric label="Battery output" value={status.batteryOutputW} unit="W" />
        <Metric label="AC charge limit" value={status.acChargeLimitW} unit="W" />
        <Metric label="Mode" value={status.mode} />
      </section>

      <section className="section details">
        <div className="panel">
          <span className="panel-label">Last decision</span>
          <p className="reason">{status.lastDecisionReason}</p>
          {error ? <p className="error">{error}</p> : null}
          {status.lastError ? <p className="error">{status.lastError}</p> : null}
        </div>
        <div className="panel">
          <span className="panel-label">Runtime</span>
          <dl className="status-list">
            <div className="status-row">
              <dt>State</dt>
              <dd>{status.state}</dd>
            </div>
            <div className="status-row">
              <dt>Updated</dt>
              <dd>{status.updatedAt ? new Date(status.updatedAt).toLocaleString() : "-"}</dd>
            </div>
          </dl>
        </div>
      </section>
    </main>
  );
}

function Metric({ label, value, unit }: { label: string; value: number | string; unit?: string }) {
  return (
    <div className="card">
      <span>{label}</span>
      <div className="value">
        {value}
        {unit ? <small className="unit">{unit}</small> : null}
      </div>
    </div>
  );
}
