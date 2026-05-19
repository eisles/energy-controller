"use client";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { EnergyMeterLog } from "@/lib/types";

type EnergyMeterLogTableProps = {
  logs: EnergyMeterLog[];
  error: string | null;
  page: number;
  pageSize: number;
  total: number;
  from: string;
  to: string;
  isFiltered: boolean;
  onFromChange: (from: string) => void;
  onToChange: (to: string) => void;
  onSearchSubmit: () => void;
  onSearchClear: () => void;
  onPageChange: (page: number) => void;
};

export function EnergyMeterLogTable({
  logs,
  error,
  page,
  pageSize,
  total,
  from,
  to,
  isFiltered,
  onFromChange,
  onToChange,
  onSearchSubmit,
  onSearchClear,
  onPageChange
}: EnergyMeterLogTableProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const startIndex = (page - 1) * pageSize;
  const firstItem = total === 0 ? 0 : startIndex + 1;
  const lastItem = Math.min(startIndex + logs.length, total);
  const hasDraftFilter = Boolean(from || to);
  const renderPager = () => (total > pageSize ? <EnergyMeterPager page={page} totalPages={totalPages} onPageChange={onPageChange} /> : null);

  return (
    <Card className="section">
      <CardHeader>
        <CardTitle>電力量ログ</CardTitle>
        <CardDescription>
          {total === 0 ? `/api/energy-meter/logs?limit=${pageSize}&offset=${startIndex}` : `${firstItem}-${lastItem} / ${total} 件を表示`}
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
                <TableHead>買電累積</TableHead>
                <TableHead>売電累積</TableHead>
                <TableHead>買電差分</TableHead>
                <TableHead>売電差分</TableHead>
                <TableHead>係数</TableHead>
                <TableHead>単位</TableHead>
                <TableHead>E0 updated</TableHead>
                <TableHead>E3 updated</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={9} className="empty-cell">
                    {isFiltered ? "一致する電力量ログはありません。" : "電力量ログはまだありません。"}
                  </TableCell>
                </TableRow>
              ) : (
                logs.map((log) => (
                  <TableRow key={log.id}>
                    <TableCell>{formatDateTime(log.measuredAt)}</TableCell>
                    <TableCell>{kwh(log.importCumulativeKwh)}</TableCell>
                    <TableCell>{kwh(log.exportCumulativeKwh)}</TableCell>
                    <TableCell>{nullableKwh(log.importDeltaKwh)}</TableCell>
                    <TableCell>{nullableKwh(log.exportDeltaKwh)}</TableCell>
                    <TableCell>{log.coefficient}</TableCell>
                    <TableCell>{log.cumulativeUnit}</TableCell>
                    <TableCell>{formatDateTime(log.importValueUpdatedAt)}</TableCell>
                    <TableCell>{formatDateTime(log.exportValueUpdatedAt)}</TableCell>
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

function EnergyMeterPager({
  page,
  totalPages,
  onPageChange
}: {
  page: number;
  totalPages: number;
  onPageChange: (page: number) => void;
}) {
  return (
    <div className="table-pager" aria-label="energy meter table pagination">
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

function kwh(value: number) {
  return `${formatNumber(value)} kWh`;
}

function nullableKwh(value: number | null) {
  return value === null ? "-" : kwh(value);
}

function formatNumber(value: number) {
  return new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 4 }).format(value);
}

function formatDateTime(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString("ja-JP");
}
