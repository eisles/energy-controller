"use client";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { commandResultLabel, decisionReasonLabel, guardReasonLabel, strategyStateLabel } from "@/lib/display-labels";
import type { Delta3AuxControlCommandLog } from "@/lib/types";

type Props = {
  logs: Delta3AuxControlCommandLog[];
  error: string | null;
  page: number;
  pageSize: number;
  total: number;
  onPageChange: (page: number) => void;
};

export function Delta3AuxControlCommandLogTable({ logs, error, page, pageSize, total, onPageChange }: Props) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const startIndex = (page - 1) * pageSize;
  const firstItem = total === 0 ? 0 : startIndex + 1;
  const lastItem = Math.min(startIndex + logs.length, total);

  return (
    <Card className="section">
      <CardHeader>
        <div className="panel-title-row">
          <div>
            <CardDescription>Auxiliary battery command audit</CardDescription>
            <CardTitle>DELTA 3 Plus 補助充電ログ</CardTitle>
          </div>
          <Badge variant="secondary">guarded</Badge>
        </div>
      </CardHeader>
      <CardContent>
        {error ? <p className="inline-error">{error}</p> : null}
        <p className="readonly-note">
          {total === 0 ? `/api/delta3/aux-commands?limit=${pageSize}&offset=${startIndex}` : `${firstItem}-${lastItem} / ${total} 件を表示`}
        </p>
        <Pager page={page} totalPages={totalPages} onPageChange={onPageChange} />
        <div className="table-wrap dry-run-table-wrap">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Measured</TableHead>
                <TableHead>状態</TableHead>
                <TableHead>Grid</TableHead>
                <TableHead>DELTA 3</TableHead>
                <TableHead>AC上限</TableHead>
                <TableHead>判定</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="empty-cell">
                    DELTA 3 Plus 補助充電ログはまだ記録されていません。
                  </TableCell>
                </TableRow>
              ) : (
                logs.map((log) => (
                  <TableRow key={log.id}>
                    <TableCell>{formatDateTime(log.measuredAt)}</TableCell>
                    <TableCell>
                      <Badge variant={log.wouldWrite ? "warning" : "secondary"}>{strategyStateLabel(log.strategyState)}</Badge>
                    </TableCell>
                    <TableCell>{formatGrid(log)}</TableCell>
                    <TableCell>
                      SOC {nullablePercent(log.delta3Soc)}
                      <br />
                      <span className="readonly-note">残余売電 {log.residualExportW} W</span>
                    </TableCell>
                    <TableCell>{formatChange(log.previousAcChargeLimitW, log.targetAcChargeLimitW, "W")}</TableCell>
                    <TableCell className="reason-cell">
                      <Badge variant={log.commandSent ? "warning" : log.dryRun ? "secondary" : "success"}>
                        {commandResultLabel(log)}
                      </Badge>
                      <p>{decisionReasonLabel(log.decisionReason)}</p>
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
    <div className="table-pager" aria-label="DELTA 3 Plus auxiliary command pagination">
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

function formatGrid(log: Delta3AuxControlCommandLog) {
  if (log.importW > 0) {
    return `買電 ${log.importW} W`;
  }
  if (log.exportW > 0) {
    return `売電 ${log.exportW} W`;
  }
  return `${log.gridW} W`;
}

function formatChange(previous: number | null, target: number | null, unit: string) {
  const previousLabel = previous === null || previous === undefined ? "-" : `${previous} ${unit}`;
  const targetLabel = target === null || target === undefined ? "-" : `${target} ${unit}`;
  return `${previousLabel} -> ${targetLabel}`;
}

function nullablePercent(value: number | null | undefined) {
  return value === null || value === undefined ? "-" : `${value}%`;
}

function formatDateTime(value: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("ja-JP", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit"
  }).format(date);
}
