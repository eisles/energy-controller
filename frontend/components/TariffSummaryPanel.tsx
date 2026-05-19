"use client";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { TariffSummary } from "@/lib/types";

type TariffSummaryPanelProps = {
  summary: TariffSummary | null;
  error: string | null;
  from: string;
  to: string;
  isFiltered: boolean;
  onFromChange: (from: string) => void;
  onToChange: (to: string) => void;
  onSearchSubmit: () => void;
  onSearchClear: () => void;
};

const periodLabels: Record<string, string> = {
  day: "デイタイム",
  home: "ホームタイム",
  night: "ナイトタイム"
};

export function TariffSummaryPanel({
  summary,
  error,
  from,
  to,
  isFiltered,
  onFromChange,
  onToChange,
  onSearchSubmit,
  onSearchClear
}: TariffSummaryPanelProps) {
  const hasDraftFilter = Boolean(from || to);

  return (
    <Card className="section">
      <CardHeader>
        <CardTitle>料金概算</CardTitle>
        <CardDescription>
          {summary ? `${summary.planName} / ${summary.sampleCount} 件の電力量差分から計算` : "/api/tariff/summary"}
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
        <div className="tariff-summary-grid">
          <div className="tariff-summary-item">
            <span>買電量</span>
            <strong>{summary ? kwh(summary.totalImportKwh) : "-"}</strong>
          </div>
          <div className="tariff-summary-item">
            <span>売電量</span>
            <strong>{summary ? kwh(summary.totalExportKwh) : "-"}</strong>
          </div>
          <div className="tariff-summary-item">
            <span>買電料金概算</span>
            <strong>{summary ? yen(summary.totalImportCostYen) : "-"}</strong>
          </div>
          <div className="tariff-summary-item">
            <span>売電料金概算</span>
            <strong>{summary ? yen(summary.totalExportIncomeYen) : "-"}</strong>
          </div>
          <div className="tariff-summary-item">
            <span>差引概算</span>
            <strong>{summary ? yen(summary.netCostYen) : "-"}</strong>
          </div>
          <div className="tariff-summary-item">
            <span>タイムゾーン</span>
            <strong>{summary?.timezone ?? "-"}</strong>
          </div>
        </div>
        <div className="table-wrap tariff-table-wrap">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>プラン</TableHead>
                <TableHead>時間帯</TableHead>
                <TableHead>単価</TableHead>
                <TableHead>適用開始</TableHead>
                <TableHead>買電量</TableHead>
                <TableHead>買電料金</TableHead>
                <TableHead>売電量</TableHead>
                <TableHead>売電単価</TableHead>
                <TableHead>売電料金</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {!summary || summary.periods.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={9} className="empty-cell">
                    電力量差分がまだありません。
                  </TableCell>
                </TableRow>
              ) : (
                summary.periods.map((period) => (
                  <TableRow key={`${period.planName}-${period.effectiveFrom}-${period.period}`}>
                    <TableCell>{period.planName}</TableCell>
                    <TableCell>{periodLabels[period.period] ?? period.period}</TableCell>
                    <TableCell>{yenPerKwh(period.rateYen)}</TableCell>
                    <TableCell>{formatDateTime(period.effectiveFrom)}</TableCell>
                    <TableCell>{kwh(period.importKwh)}</TableCell>
                    <TableCell>{yen(period.importCostYen)}</TableCell>
                    <TableCell>{kwh(period.exportKwh)}</TableCell>
                    <TableCell>{yenPerKwh(period.exportRateYen)}</TableCell>
                    <TableCell>{yen(period.exportIncomeYen)}</TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
        {summary?.note ? <p className="readonly-note">{summary.note}</p> : null}
      </CardContent>
    </Card>
  );
}

function kwh(value: number) {
  return `${new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 4 }).format(value)} kWh`;
}

function yen(value: number) {
  return `${new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 2 }).format(value)} 円`;
}

function yenPerKwh(value: number) {
  return `${new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 2 }).format(value)} 円/kWh`;
}

function formatDateTime(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString("ja-JP");
}
