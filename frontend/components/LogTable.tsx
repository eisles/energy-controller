"use client";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { commandResultLabel } from "@/lib/display-labels";
import type { PowerLog } from "@/lib/types";

type LogTableProps = {
  logs: PowerLog[];
  error: string | null;
  page: number;
  pageSize: number;
  total: number;
  search: string;
  from: string;
  to: string;
  isFiltered: boolean;
  onSearchChange: (search: string) => void;
  onFromChange: (from: string) => void;
  onToChange: (to: string) => void;
  onSearchSubmit: () => void;
  onSearchClear: () => void;
  onPageChange: (page: number) => void;
};

export function LogTable({
  logs,
  error,
  page,
  pageSize,
  total,
  search,
  from,
  to,
  isFiltered,
  onSearchChange,
  onFromChange,
  onToChange,
  onSearchSubmit,
  onSearchClear,
  onPageChange
}: LogTableProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const startIndex = (page - 1) * pageSize;
  const firstItem = total === 0 ? 0 : startIndex + 1;
  const lastItem = Math.min(startIndex + logs.length, total);
  const hasDraftFilter = Boolean(search || from || to);
  const renderPager = () => (total > pageSize ? <LogPager page={page} totalPages={totalPages} onPageChange={onPageChange} /> : null);

  return (
    <Card className="section">
      <CardHeader>
        <CardTitle>制御ログ</CardTitle>
        <CardDescription>
          {total === 0 ? `/api/logs?limit=${pageSize}&offset=${startIndex}` : `${firstItem}-${lastItem} / ${total} 件を表示`}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {error ? <p className="inline-error">{error}</p> : null}
        <form
          className="log-table-toolbar"
          onSubmit={(event) => {
            event.preventDefault();
            onSearchSubmit();
          }}
        >
          <label className="log-search">
            <span>ログ検索</span>
            <input
              className="text-input"
              value={search}
              onChange={(event) => onSearchChange(event.target.value)}
              placeholder="mode / reason / error / W数"
            />
          </label>
          <label className="log-period">
            <span>開始</span>
            <input className="text-input" type="datetime-local" value={from} onChange={(event) => onFromChange(event.target.value)} />
          </label>
          <label className="log-period">
            <span>終了</span>
            <input className="text-input" type="datetime-local" value={to} onChange={(event) => onToChange(event.target.value)} />
          </label>
          <div className="log-filter-actions">
            <Button type="submit">検索</Button>
            {hasDraftFilter || isFiltered ? (
              <Button type="button" variant="outline" onClick={onSearchClear}>
                クリア
              </Button>
            ) : null}
          </div>
        </form>
        {renderPager()}
        <div className="table-wrap">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Measured</TableHead>
                <TableHead>Grid</TableHead>
                <TableHead>Import</TableHead>
                <TableHead>Export</TableHead>
                <TableHead>SOC</TableHead>
                <TableHead>Net</TableHead>
                <TableHead>In</TableHead>
                <TableHead>Out</TableHead>
                <TableHead>AC limit</TableHead>
                <TableHead>Target</TableHead>
                <TableHead>Mode</TableHead>
                <TableHead>Command</TableHead>
                <TableHead>Reason</TableHead>
                <TableHead>Error</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={14} className="empty-cell">
                    {isFiltered ? "一致するログはありません。" : "ログはまだありません。"}
                  </TableCell>
                </TableRow>
              ) : (
                logs.map((log) => (
                  <TableRow key={log.id}>
                    <TableCell>{formatDateTime(log.measuredAt)}</TableCell>
                    <TableCell>{watt(log.gridW)}</TableCell>
                    <TableCell>{watt(log.importW)}</TableCell>
                    <TableCell>{watt(log.exportW)}</TableCell>
                    <TableCell>{nullableUnit(log.batterySoc, "%")}</TableCell>
                    <TableCell>{nullableUnit(netBatteryW(log), "W")}</TableCell>
                    <TableCell>{nullableUnit(log.batteryInputW, "W")}</TableCell>
                    <TableCell>{nullableUnit(log.batteryOutputW, "W")}</TableCell>
                    <TableCell>{nullableUnit(log.acChargeLimitW, "W")}</TableCell>
                    <TableCell>{watt(log.targetChargeW)}</TableCell>
                    <TableCell>
                      <Badge variant="secondary">{log.mode || "-"}</Badge>
                    </TableCell>
                    <TableCell>
                      <Badge variant={log.commandSent ? "warning" : "success"}>{commandResultLabel({ commandSent: log.commandSent })}</Badge>
                    </TableCell>
                    <TableCell className="reason-cell">{log.decisionReason || "-"}</TableCell>
                    <TableCell className="reason-cell">{log.errorMessage || "-"}</TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
        {renderPager()}
      </CardContent>
    </Card>
  );
}

function LogPager({
  page,
  totalPages,
  onPageChange
}: {
  page: number;
  totalPages: number;
  onPageChange: (page: number) => void;
}) {
  return (
    <div className="table-pager" aria-label="log table pagination">
      <div className="table-pager-status">
        ページ {page} / {totalPages}
      </div>
      <div className="table-pager-actions">
        <Button variant="outline" onClick={() => onPageChange(1)} disabled={page === 1}>
          最初
        </Button>
        <Button variant="outline" onClick={() => onPageChange(Math.max(1, page - 1))} disabled={page === 1}>
          前へ
        </Button>
        <Button variant="outline" onClick={() => onPageChange(Math.min(totalPages, page + 1))} disabled={page === totalPages}>
          次へ
        </Button>
        <Button variant="outline" onClick={() => onPageChange(totalPages)} disabled={page === totalPages}>
          最後
        </Button>
      </div>
    </div>
  );
}

function watt(value: number) {
  return `${value} W`;
}

function nullableUnit(value: number | null, unit: string) {
  return value === null ? "-" : `${value} ${unit}`;
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
