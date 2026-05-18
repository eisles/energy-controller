import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import type { EnergyStatus } from "@/lib/types";
import type { ReactNode } from "react";

type Metric = {
  label: string;
  value: number | string;
  unit?: string;
  description?: string;
};

const primaryMetrics: Array<keyof EnergyStatus> = ["gridW", "importW", "exportW", "batterySoc", "targetChargeW", "state"];

export function StatusCards({ status, fetchError }: { status: EnergyStatus; fetchError: string | null }) {
  const metrics: Record<keyof EnergyStatus, Metric> = {
    gridW: { label: "Grid", value: formatGridFlow(status.gridW) },
    importW: { label: "Import", value: status.importW, unit: "W", description: "現在の買電" },
    exportW: { label: "Export", value: status.exportW, unit: "W", description: "現在の売電" },
    batterySoc: {
      label: "Battery",
      value: formatBatteryFlow(status.batteryInputW, status.batteryOutputW),
      description: `SOC ${status.batterySoc}%`
    },
    batteryInputW: { label: "Battery input", value: status.batteryInputW, unit: "W" },
    batteryOutputW: { label: "Battery output", value: status.batteryOutputW, unit: "W" },
    acChargeLimitW: { label: "AC charge limit", value: status.acChargeLimitW, unit: "W" },
    targetChargeW: { label: "Target charge", value: status.targetChargeW, unit: "W", description: "推奨充電W" },
    mode: { label: "Mode", value: status.mode || "-" },
    state: { label: "State", value: status.state || "-" },
    lastDecisionReason: { label: "Last decision", value: status.lastDecisionReason || "-" },
    lastError: { label: "Last error", value: status.lastError || "-" },
    updatedAt: { label: "Updated", value: formatDateTime(status.updatedAt) }
  };

  return (
    <>
      {fetchError ? (
        <Alert variant="destructive" className="section">
          <AlertTitle>API read failed</AlertTitle>
          <AlertDescription>{fetchError}</AlertDescription>
        </Alert>
      ) : null}

      <section className="metric-grid section" aria-label="current energy status">
        {primaryMetrics.map((key) => (
          <MetricCard key={key} metric={metrics[key]} />
        ))}
      </section>

      <Card className="decision-panel section">
        <CardHeader>
          <CardDescription>Last decision</CardDescription>
          <CardTitle>{status.lastDecisionReason || "-"}</CardTitle>
        </CardHeader>
        <CardContent className="detail-strip" aria-label="status detail">
          <Detail label="Mode" value={<Badge variant="secondary">{status.mode || "-"}</Badge>} />
          <Detail label="Battery input" value={`${status.batteryInputW} W`} />
          <Detail label="Battery output" value={`${status.batteryOutputW} W`} />
          <Detail label="AC charge limit" value={`${status.acChargeLimitW} W`} />
          <Detail label="Updated" value={formatDateTime(status.updatedAt)} />
        </CardContent>
      </Card>

      {status.lastError ? (
        <Alert variant="destructive" className="section">
          <AlertTitle>Last error</AlertTitle>
          <AlertDescription>{status.lastError}</AlertDescription>
        </Alert>
      ) : null}
    </>
  );
}

function MetricCard({ metric }: { metric: Metric }) {
  return (
    <Card className="metric-card">
      <CardHeader>
        <CardDescription>{metric.label}</CardDescription>
        <CardTitle className="metric-value">
          {metric.value}
          {metric.unit ? <span className="metric-unit">{metric.unit}</span> : null}
        </CardTitle>
      </CardHeader>
      {metric.description ? <CardContent className="metric-description">{metric.description}</CardContent> : null}
    </Card>
  );
}

function formatGridFlow(value: number) {
  if (value > 0) {
    return `買電 ${value} W`;
  }
  if (value < 0) {
    return `売電 ${Math.abs(value)} W`;
  }
  return "買電/売電 0 W";
}

function formatBatteryFlow(inputW: number, outputW: number) {
  if (inputW > 0 && outputW > 0) {
    return `入出力中 ${inputW} / ${outputW} W`;
  }
  if (inputW > 0) {
    return `充電中 ${inputW} W`;
  }
  if (outputW > 0) {
    return `放電中 ${outputW} W`;
  }
  return "待機中 0 W";
}

function Detail({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="detail-item">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function formatDateTime(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString("ja-JP");
}
