"use client";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { commandResultLabel, guardReasonLabel, strategyStateLabel } from "@/lib/display-labels";
import type { SurplusControlCommandLog } from "@/lib/types";

type Props = {
  logs: SurplusControlCommandLog[];
  error: string | null;
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
};

export function SurplusControlCommandLogTable({ logs, error, page, pageSize, total, onPageChange }: Props) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const startIndex = (page - 1) * pageSize;
  const firstItem = total === 0 ? 0 : startIndex + 1;
  const lastItem = Math.min(startIndex + logs.length, total);

  return (
    <Card className="section">
      <CardHeader>
        <div className="panel-title-row">
          <div>
            <CardDescription>Dry-run command audit</CardDescription>
            <CardTitle>余剰追従コマンド履歴</CardTitle>
          </div>
          <Badge variant="secondary">未送信</Badge>
        </div>
      </CardHeader>
      <CardContent>
        {error ? <p className="inline-error">{error}</p> : null}
        <p className="readonly-note">
          {total === 0 ? `/api/surplus-control/commands?limit=${pageSize}&offset=${startIndex}` : `${firstItem}-${lastItem} / ${total} 件を表示`}
        </p>
        <Pager page={page} totalPages={totalPages} onPageChange={onPageChange} />
        <div className="table-wrap dry-run-table-wrap">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Measured</TableHead>
                <TableHead>状態</TableHead>
                <TableHead>Grid</TableHead>
                <TableHead>Battery</TableHead>
                <TableHead>AC上限</TableHead>
                <TableHead>Reserve</TableHead>
                <TableHead>判定</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={7} className="empty-cell">
                    余剰追従コマンド履歴はまだ記録されていません。
                  </TableCell>
                </TableRow>
              ) : (
                logs.map((log) => (
                  <TableRow key={log.id}>
                    <TableCell>{formatDateTime(log.measuredAt)}</TableCell>
                    <TableCell>
                      <Badge variant={log.wouldWrite ? "warning" : "secondary"}>{strategyStateLabel(log.strategyState)}</Badge>
                      <br />
                      <span className="readonly-note">{commandKindLabel(log.commandKind)}</span>
                    </TableCell>
                    <TableCell>{formatGrid(log)}</TableCell>
                    <TableCell>{formatBattery(log)}</TableCell>
                    <TableCell>{formatChange(log.previousAcChargeLimitW, log.targetAcChargeLimitW, "W")}</TableCell>
                    <TableCell>{formatChange(log.previousBackupReserveSoc, log.targetBackupReserveSoc, "%")}</TableCell>
                    <TableCell className="reason-cell">
                      <Badge variant={log.commandSent ? "warning" : log.dryRun ? "secondary" : "success"}>
                        {commandResultLabel(log)}
                      </Badge>
                      <p>{log.decisionReason || "-"}</p>
                      <p className="readonly-note">{actionFlags(log)}</p>
                      {log.modeGuardReason ? <p className="readonly-note">Mode: {log.modeGuardReason}</p> : null}
                      {log.suppressedReason ? <p className="readonly-note">抑制: {guardReasonLabel(log.suppressedReason)}</p> : null}
                      {log.errorMessage ? <p className="inline-error">{log.errorMessage}</p> : null}
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
    <div className="table-pager" aria-label="surplus control command pagination">
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

function formatGrid(log: SurplusControlCommandLog) {
  if (log.exportW > 0) {
    return `売電 ${log.exportW} W`;
  }
  if (log.importW > 0) {
    return `買電 ${log.importW} W`;
  }
  return "買電/売電 0 W";
}

function formatBattery(log: SurplusControlCommandLog) {
  const netW = log.batteryInputW - log.batteryOutputW;
  if (netW > 0) {
    return `実質充電 ${netW} W`;
  }
  if (netW < 0) {
    return `実質放電 ${Math.abs(netW)} W`;
  }
  return "待機中 0 W";
}

function formatChange(previous: number | null, target: number | null, unit: string) {
  if (target === null || target === undefined) {
    return previous === null || previous === undefined ? "-" : `${previous} ${unit}`;
  }
  if (previous === null || previous === undefined) {
    return `- -> ${target} ${unit}`;
  }
  return `${previous} -> ${target} ${unit}`;
}

function commandKindLabel(value: string) {
  if (value === "ac_charge_limit") {
    return "AC上限";
  }
  if (value === "backup_reserve") {
    return "Reserve";
  }
  if (value === "energy_mode") {
    return "Mode";
  }
  if (value === "mixed") {
    return "複合";
  }
  return "候補なし";
}

function actionFlags(log: SurplusControlCommandLog) {
  const actions: string[] = [];
  if (log.shouldAdjustAcChargeLimit) actions.push("AC");
  if (log.shouldSetBackupReserve) actions.push("Reserve");
  if (log.shouldDisableEnergyModes) actions.push("Modes OFF");
  if (log.shouldEnableTouMode) actions.push("TOU ON");
  return actions.length > 0 ? actions.join(" / ") : "no action";
}

function formatDateTime(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString("ja-JP");
}
