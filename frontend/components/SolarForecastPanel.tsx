"use client";

import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { SolarForecastSummary } from "@/lib/types";

type ForecastRange = {
  label: string;
  days: number;
};

type SolarForecastPanelProps = {
  summary: SolarForecastSummary | null;
  error: string | null;
  ranges: readonly ForecastRange[];
  selectedRange: ForecastRange;
  loading: boolean;
  onRangeChange: (range: ForecastRange) => void;
};

const solarForecastChartConfig = {
  estimatedPvKwh: { label: "推定PV発電", color: "#1f6f5b", unit: "kWh" },
  estimatedSurplusKwh: { label: "推定余剰", color: "#2563eb", unit: "kWh" },
  precipitationSumMm: { label: "降水量", color: "#64748b", unit: "mm" }
} satisfies ChartConfig;

export function SolarForecastPanel({ summary, error, ranges, selectedRange, loading, onRangeChange }: SolarForecastPanelProps) {
  const chartData =
    summary?.items.map((item) => ({
      label: formatDateLabel(item.forecast.date),
      date: item.forecast.date,
      estimatedPvKwh: roundOne(item.estimatedPvKwh),
      estimatedSurplusKwh: roundOne(item.estimatedSurplusKwh),
      precipitationSumMm: roundOne(item.precipitationSumMm)
    })) ?? [];
  const firstDate = summary?.items[0]?.forecast.date;
  const lastDate = summary?.items[summary.items.length - 1]?.forecast.date;

  return (
    <Card className="section">
      <CardHeader>
        <div className="panel-title-row forecast-panel-title-row">
          <div>
            <CardTitle>発電予測</CardTitle>
            <CardDescription>
              {summary
                ? `${summary.days}日分 / ${summary.items.length}件 / ${formatDateRange(firstDate, lastDate)} / PV ${formatKwh(summary.location.pvCapacityKw)} kW / 補正 ${formatRatio(summary.location.pvPerformanceRatio)}`
                : "/api/weather/solar-forecast"}
            </CardDescription>
          </div>
          <ForecastRangeSelector ranges={ranges} selectedRange={selectedRange} onRangeChange={onRangeChange} />
        </div>
      </CardHeader>
      <CardContent>
        {error ? <p className="inline-error">{error}</p> : null}
        <p className="readonly-note">
          選択中: {selectedRange.label}
          {loading ? " / 読み込み中" : ""}
        </p>
        {chartData.length > 0 ? (
          <>
            <ChartLegend config={solarForecastChartConfig} />
            <ChartContainer config={solarForecastChartConfig}>
              <LineChart data={chartData} margin={{ top: 12, right: 12, left: -12, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" vertical={false} />
                <XAxis dataKey="label" tickLine={false} axisLine={false} minTickGap={18} />
                <YAxis yAxisId="energy" tickLine={false} axisLine={false} width={58} tickFormatter={(value) => `${value}kWh`} />
                <YAxis yAxisId="rain" orientation="right" tickLine={false} axisLine={false} width={42} tickFormatter={(value) => `${value}mm`} />
                <ChartTooltip content={<ChartTooltipContent config={solarForecastChartConfig} />} />
                <Line yAxisId="energy" dataKey="estimatedPvKwh" type="monotone" stroke="var(--color-estimatedPvKwh)" strokeWidth={3} dot={false} isAnimationActive={false} />
                <Line yAxisId="energy" dataKey="estimatedSurplusKwh" type="monotone" stroke="var(--color-estimatedSurplusKwh)" strokeWidth={2} dot={false} isAnimationActive={false} />
                <Line yAxisId="rain" dataKey="precipitationSumMm" type="monotone" stroke="var(--color-precipitationSumMm)" strokeWidth={2} dot={false} isAnimationActive={false} />
              </LineChart>
            </ChartContainer>
            <div className="table-wrap forecast-table-wrap">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>日付</TableHead>
                    <TableHead>推定PV</TableHead>
                    <TableHead>推定余剰</TableHead>
                    <TableHead>日射量</TableHead>
                    <TableHead>日照</TableHead>
                    <TableHead>雲量</TableHead>
                    <TableHead>降水</TableHead>
                    <TableHead>期待度</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {summary?.items.map((item) => (
                    <TableRow key={item.forecast.date}>
                      <TableCell>{formatDate(item.forecast.date)}</TableCell>
                      <TableCell>{formatKwh(item.estimatedPvKwh)} kWh</TableCell>
                      <TableCell>{formatKwh(item.estimatedSurplusKwh)} kWh</TableCell>
                      <TableCell>{formatKwh(item.solarRadiationKwhPerM2)} kWh/m2</TableCell>
                      <TableCell>{formatKwh(item.forecast.sunshineDurationHours)} h</TableCell>
                      <TableCell>{item.forecast.cloudCoverMeanPercent}%</TableCell>
                      <TableCell>{item.precipitationProbabilityMax}% / {formatKwh(item.precipitationSumMm)} mm</TableCell>
                      <TableCell>{item.solarForecastScore}/100</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
            {summary?.note ? <p className="readonly-note">{summary.note}</p> : null}
          </>
        ) : error ? null : (
          <p className="readonly-note">天気予報設定を保存すると発電予測を表示します。</p>
        )}
      </CardContent>
    </Card>
  );
}

function ForecastRangeSelector({ ranges, selectedRange, onRangeChange }: { ranges: readonly ForecastRange[]; selectedRange: ForecastRange; onRangeChange: (range: ForecastRange) => void }) {
  return (
    <div className="range-selector" aria-label="solar forecast days">
      {ranges.map((range) => (
        <Button
          key={range.days}
          type="button"
          variant={range.days === selectedRange.days ? "default" : "outline"}
          onClick={() => onRangeChange(range)}
        >
          {range.label}
        </Button>
      ))}
    </div>
  );
}

function ChartLegend({ config }: { config: ChartConfig }) {
  return (
    <div className="chart-legend">
      {Object.entries(config).map(([key, item]) => (
        <span key={key} className="chart-legend-item">
          <span className="chart-legend-dot" style={{ background: item.color }} />
          {item.label}
        </span>
      ))}
    </div>
  );
}

function formatDate(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(`${value}T00:00:00`).toLocaleDateString("ja-JP", {
    month: "2-digit",
    day: "2-digit",
    weekday: "short"
  });
}

function formatDateLabel(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(`${value}T00:00:00`).toLocaleDateString("ja-JP", {
    month: "2-digit",
    day: "2-digit"
  });
}

function formatDateRange(first?: string, last?: string) {
  if (!first || !last) {
    return "-";
  }
  if (first === last) {
    return formatDate(first);
  }
  return `${formatDate(first)} - ${formatDate(last)}`;
}

function formatKwh(value: number) {
  return new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 1 }).format(value);
}

function formatRatio(value: number) {
  return new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 2 }).format(value);
}

function roundOne(value: number) {
  return Math.round(value * 10) / 10;
}
