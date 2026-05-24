"use client";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { decisionSummaryLabel, guardReasonLabel, strategyStateLabel, writeCandidateLabel } from "@/lib/display-labels";
import type { NightChargePlanLog } from "@/lib/types";

type NightChargePlanLogTableProps = {
  logs: NightChargePlanLog[];
  error: string | null;
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
};

export function NightChargePlanLogTable({ logs, error, page, pageSize, total, onPageChange }: NightChargePlanLogTableProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const startIndex = (page - 1) * pageSize;
  const firstItem = total === 0 ? 0 : startIndex + 1;
  const lastItem = Math.min(startIndex + logs.length, total);

  return (
    <Card className="section">
      <CardHeader>
        <div className="panel-title-row">
          <div>
            <CardDescription>Read-only audit</CardDescription>
            <CardTitle>夜間計画・結果</CardTitle>
          </div>
          <Badge variant="secondary">未送信</Badge>
        </div>
      </CardHeader>
      <CardContent>
        {error ? <p className="inline-error">{error}</p> : null}
        <p className="readonly-note">
          {total === 0 ? `/api/night-charge/plans?limit=${pageSize}&offset=${startIndex}` : `${firstItem}-${lastItem} / ${total} 件を表示`}
        </p>
        <Pager page={page} totalPages={totalPages} onPageChange={onPageChange} />
        <div className="table-wrap dry-run-table-wrap">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Measured</TableHead>
                <TableHead>状態</TableHead>
                <TableHead>Mode</TableHead>
                <TableHead>SOC</TableHead>
                <TableHead>kWh</TableHead>
                <TableHead>PV予測</TableHead>
                <TableHead>Battery</TableHead>
                <TableHead>Grid</TableHead>
                <TableHead>判定</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={9} className="empty-cell">
                    夜間計画ログはまだ記録されていません。
                  </TableCell>
                </TableRow>
              ) : (
                logs.map((log) => (
                  <TableRow key={log.id}>
                    <TableCell>{formatDateTime(log.measuredAt)}</TableCell>
                    <TableCell>
                      <Badge variant="secondary">{strategyStateLabel(log.strategyState)}</Badge>
                    </TableCell>
                    <TableCell>{modeLabel(log.recommendedMode)}</TableCell>
                    <TableCell>{`${log.batterySoc}% -> ${log.recommendedNightTargetSoc}%`}</TableCell>
                    <TableCell>
                      {`${formatDecimal(log.currentBatteryEnergyKwh)} -> ${formatDecimal(log.recommendedNightTargetKwh)} kWh`}
                      <br />
                      <span className="readonly-note">必要 {formatDecimal(log.requiredNightChargeKwh)} kWh</span>
                    </TableCell>
                    <TableCell>
                      {formatDecimal(log.dailyEstimatedPvKwh)} kWh
                      <br />
                      <span className="readonly-note">{formatTimeWindow(log.pvEffectiveStartAt, log.pvEffectiveEndAt)}</span>
                      {log.forecastDaytimeDeficitKwh > 0 ? (
                        <>
                          <br />
                          <span className="readonly-note">不足 {formatDecimal(log.forecastDaytimeDeficitKwh)} kWh</span>
                        </>
                      ) : null}
                    </TableCell>
                    <TableCell>{formatBattery(log)}</TableCell>
                    <TableCell>{formatGrid(log)}</TableCell>
                    <TableCell className="reason-cell">
                      <Badge variant={log.wouldWrite ? "warning" : log.shouldChargeTonight ? "secondary" : "success"}>
                        {log.wouldWrite ? writeCandidateLabel(log.wouldWrite) : log.shouldChargeTonight ? "充電必要" : "問題なし"}
                      </Badge>
                      <p>{decisionSummaryLabel(log.actionSummary || log.reason)}</p>
                      {log.commandBlockReason ? <p className="readonly-note">抑制: {guardReasonLabel(log.commandBlockReason)}</p> : null}
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
        <Pager page={page} totalPages={totalPages} onPageChange={onPageChange} />
      </CardContent>
    </Card>
  );
}

function Pager({ page, totalPages, onPageChange }: { page: number; totalPages: number; onPageChange: (page: number) => void }) {
  return (
    <div className="table-pager" aria-label="night charge plan pagination">
      <div className="table-pager-status">
        ページ {page} / {totalPages}
      </div>
      <div className="table-pager-actions">
      <Button type="button" variant="outline" disabled={page <= 1} onClick={() => onPageChange(1)}>
        最初
      </Button>
      <Button type="button" variant="outline" disabled={page <= 1} onClick={() => onPageChange(page - 1)}>
        前へ
      </Button>
      <Button type="button" variant="outline" disabled={page >= totalPages} onClick={() => onPageChange(page + 1)}>
        次へ
      </Button>
      <Button type="button" variant="outline" disabled={page >= totalPages} onClick={() => onPageChange(totalPages)}>
        最後
      </Button>
      </div>
    </div>
  );
}

function formatGrid(log: NightChargePlanLog) {
  if (log.exportW > 0) {
    return `売電 ${log.exportW} W`;
  }
  if (log.importW > 0) {
    return `買電 ${log.importW} W`;
  }
  return "買電/売電 0 W";
}

function formatBattery(log: NightChargePlanLog) {
  const netW = log.batteryInputW - log.batteryOutputW;
  if (netW > 0) {
    return `実質充電 ${netW} W`;
  }
  if (netW < 0) {
    return `実質放電 ${Math.abs(netW)} W`;
  }
  return "待機中 0 W";
}

function modeLabel(value: string) {
  if (value === "tou") {
    return "TOU";
  }
  if (value === "self-powered") {
    return "Self-powered";
  }
  if (value === "energy-strategy-off") {
    return "Mode OFF";
  }
  if (value === "observe") {
    return "観測";
  }
  return value || "-";
}

function formatDateTime(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString("ja-JP");
}

function formatTimeWindow(start?: string, end?: string) {
  if (!start || !end) {
    return "-";
  }
  return `${formatTimeOnly(start)}-${formatTimeOnly(end)}`;
}

function formatTimeOnly(value: string) {
  const parts = value.split("T");
  return parts[1]?.slice(0, 5) || value;
}

function formatDecimal(value: number) {
  return new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 1 }).format(value);
}
