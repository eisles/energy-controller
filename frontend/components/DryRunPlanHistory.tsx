"use client";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { PowerLog } from "@/lib/types";

type DryRunPlanHistoryProps = {
  logs: PowerLog[];
  error: string | null;
};

const dryRunMarker = "surplus dry-run plan:";

export function DryRunPlanHistory({ logs, error }: DryRunPlanHistoryProps) {
  return (
    <Card className="section">
      <CardHeader>
        <div className="panel-title-row">
          <div>
            <CardDescription>Read-only audit</CardDescription>
            <CardTitle>余剰追従 dry-run 履歴</CardTitle>
          </div>
          <Badge variant="secondary">no write</Badge>
        </div>
      </CardHeader>
      <CardContent>
        {error ? <p className="inline-error">{error}</p> : null}
        <div className="table-wrap dry-run-table-wrap">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Measured</TableHead>
                <TableHead>Grid</TableHead>
                <TableHead>Battery</TableHead>
                <TableHead>計画</TableHead>
                <TableHead>Command</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="empty-cell">
                    dry-run 計画はまだ記録されていません。
                  </TableCell>
                </TableRow>
              ) : (
                logs.map((log) => (
                  <TableRow key={log.id}>
                    <TableCell>{formatDateTime(log.measuredAt)}</TableCell>
                    <TableCell>{formatGrid(log)}</TableCell>
                    <TableCell>{formatBattery(log)}</TableCell>
                    <TableCell className="reason-cell">{extractDryRunPlan(log.decisionReason)}</TableCell>
                    <TableCell>
                      <Badge variant={log.commandSent ? "warning" : "success"}>{log.commandSent ? "sent" : "not sent"}</Badge>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  );
}

function extractDryRunPlan(reason: string) {
  const markerIndex = reason.indexOf(dryRunMarker);
  if (markerIndex < 0) {
    return reason || "-";
  }
  const plan = reason.slice(markerIndex + dryRunMarker.length).split("; EcoFlow")[0].trim();
  return plan || "-";
}

function formatGrid(log: PowerLog) {
  if (log.exportW > 0) {
    return `売電 ${log.exportW} W`;
  }
  if (log.importW > 0) {
    return `買電 ${log.importW} W`;
  }
  return "買電/売電 0 W";
}

function formatBattery(log: PowerLog) {
  const soc = log.batterySoc === null ? "-" : `${log.batterySoc}%`;
  const netW = netBatteryW(log);
  if (netW === null) {
    return soc;
  }
  if (netW > 0) {
    return `${soc} / 実質充電 ${netW} W`;
  }
  if (netW < 0) {
    return `${soc} / 実質放電 ${Math.abs(netW)} W`;
  }
  return `${soc} / 待機中 0 W`;
}

function netBatteryW(log: PowerLog) {
  if (log.batteryInputW === null || log.batteryOutputW === null) {
    return null;
  }
  return log.batteryInputW - log.batteryOutputW;
}

function formatDateTime(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString("ja-JP");
}
