"use client";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { NightChargeDailySummary } from "@/lib/types";

type NightChargeSummaryTableProps = {
  summaries: NightChargeDailySummary[];
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

export function NightChargeSummaryTable({
  summaries,
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
}: NightChargeSummaryTableProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const startIndex = (page - 1) * pageSize;
  const firstItem = total === 0 ? 0 : startIndex + 1;
  const lastItem = Math.min(startIndex + summaries.length, total);
  const hasDraftFilter = Boolean(from || to);
  const renderPager = () => <Pager page={page} totalPages={totalPages} onPageChange={onPageChange} />;

  return (
    <Card className="section">
      <CardHeader>
        <div className="panel-title-row">
          <div>
            <CardDescription>Read-only audit</CardDescription>
            <CardTitle>夜間サマリー</CardTitle>
          </div>
          <Badge variant="secondary">未送信</Badge>
        </div>
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
        <p className="readonly-note">
          {total === 0 ? `/api/night-charge/summaries?limit=${pageSize}&offset=${startIndex}` : `${firstItem}-${lastItem} / ${total} 件を表示`}
        </p>
        {renderPager()}
        <div className="table-wrap dry-run-table-wrap">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>夜間日</TableHead>
                <TableHead>計画</TableHead>
                <TableHead>SOC</TableHead>
                <TableHead>深夜kWh</TableHead>
                <TableHead>翌日日中</TableHead>
                <TableHead>07:00判定</TableHead>
                <TableHead>16:00判定</TableHead>
                <TableHead>理由</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {summaries.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} className="empty-cell">
                    {isFiltered ? "一致する夜間サマリーはありません。" : "夜間サマリーはまだありません。"}
                  </TableCell>
                </TableRow>
              ) : (
                summaries.map((summary) => (
                  <TableRow key={summary.summaryDate}>
                    <TableCell>
                      <strong>{summary.summaryDate}</strong>
                      <br />
                      <span className="readonly-note">{formatDateTime(summary.planCreatedAt)}</span>
                    </TableCell>
                    <TableCell>
                      <Badge variant="secondary">{modeLabel(summary.plannedMode)}</Badge>
                      <br />
                      <span className="readonly-note">
                        目標 {nullablePercent(summary.plannedTargetSoc)} / {nullableKwh(summary.plannedTargetKwh)}
                      </span>
                      <br />
                      <span className="readonly-note">必要 {nullableKwh(summary.plannedRequiredChargeKwh)}</span>
                    </TableCell>
                    <TableCell>
                      {nullablePercent(summary.nightStartSoc)} -&gt; {nullablePercent(summary.nightEndSoc)}
                      <br />
                      <span className="readonly-note">
                        差分 {formatSocDelta(summary.nightSocDelta)} / min-max {nullablePercent(summary.minNightSoc)}-{nullablePercent(summary.maxNightSoc)}
                      </span>
                    </TableCell>
                    <TableCell>
                      買電 {nullableKwh(summary.nightImportKwh)} / 売電 {nullableKwh(summary.nightExportKwh)}
                      <br />
                      <span className="readonly-note">
                        Battery in {nullableKwh(summary.nightBatteryInputKwh)} / out {nullableKwh(summary.nightBatteryOutputKwh)}
                      </span>
                    </TableCell>
                    <TableCell>
                      Battery in {nullableKwh(summary.daytimeBatteryInputKwh)}
                      <br />
                      <span className="readonly-note">売電 {nullableKwh(summary.daytimeExportKwh)}</span>
                      <br />
                      <span className="readonly-note">{dataSourceLabel(summary.dataSource)}</span>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={summary.morningStatus} />
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={summary.finalResultStatus} />
                    </TableCell>
                    <TableCell className="reason-cell">
                      <p>{summary.morningReason || "-"}</p>
                      <p className="readonly-note">{summary.finalResultReason || "-"}</p>
                    </TableCell>
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

function Pager({ page, totalPages, onPageChange }: { page: number; totalPages: number; onPageChange: (page: number) => void }) {
  return (
    <div className="table-pager" aria-label="night charge summary pagination">
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

function StatusBadge({ status }: { status: string }) {
  return <Badge variant={statusVariant(status)}>{statusLabel(status)}</Badge>;
}

function statusVariant(status: string): "secondary" | "success" | "warning" {
  if (status === "ok") {
    return "success";
  }
  if (status === "undercharged" || status === "overcharged") {
    return "warning";
  }
  return "secondary";
}

function statusLabel(status: string) {
  if (status === "ok") {
    return "ok";
  }
  if (status === "pending") {
    return "pending";
  }
  if (status === "undercharged") {
    return "不足";
  }
  if (status === "overcharged") {
    return "過充電";
  }
  if (status === "insufficient-data") {
    return "データ不足";
  }
  return status || "-";
}

function modeLabel(value?: string | null) {
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

function dataSourceLabel(value: string) {
  if (value === "energy-meter+power-log") {
    return "電力量ログ + 制御ログ";
  }
  if (value === "power-log") {
    return "制御ログ近似";
  }
  return value || "-";
}

function nullablePercent(value?: number | null) {
  return value === null || value === undefined ? "-" : `${value}%`;
}

function formatSocDelta(value?: number | null) {
  if (value === null || value === undefined) {
    return "-";
  }
  return value > 0 ? `+${value}%` : `${value}%`;
}

function nullableKwh(value?: number | null) {
  return value === null || value === undefined ? "-" : `${formatDecimal(value)} kWh`;
}

function formatDecimal(value: number) {
  return new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 2 }).format(value);
}

function formatDateTime(value?: string | null) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString("ja-JP");
}
