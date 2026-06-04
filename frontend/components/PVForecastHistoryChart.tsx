"use client";

import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart";
import type { PVForecastHistoryItem } from "@/lib/types";

const pvHistoryChartConfig = {
  estimatedPvKwh: { label: "発電予測", color: "#0f766e", unit: "kWh" },
  correctedEstimatedPvKwh: { label: "補正後予測", color: "#2563eb", unit: "kWh" },
  correctedEstimatedPvToBatteryKwh: { label: "充電想定", color: "#7c3aed", unit: "kWh" },
  forecastDaytimeDeficitKwh: { label: "不足見込", color: "#dc2626", unit: "kWh" }
} satisfies ChartConfig;

const nightDecisionChartConfig = {
  recommendedNightTargetSoc: { label: "目標残量", color: "#0891b2", unit: "%" },
  requiredNightChargeKwh: { label: "必要充電", color: "#ea580c", unit: "kWh" }
} satisfies ChartConfig;

type PVForecastHistoryRange = {
  label: string;
  days: number;
};

type PVForecastHistoryChartProps = {
  items: PVForecastHistoryItem[];
  error: string | null;
  ranges: readonly PVForecastHistoryRange[];
  selectedRange: PVForecastHistoryRange;
  onRangeChange: (range: PVForecastHistoryRange) => void;
};

export function PVForecastHistoryChart({ items, error, ranges, selectedRange, onRangeChange }: PVForecastHistoryChartProps) {
  const chartData = items.map((item) => ({
    ...item,
    label: formatDateLabel(item.forecastDate)
  }));
  const latest = items.at(-1);

  return (
    <section className="section" aria-label="pv forecast history charts">
      <div className="chart-toolbar">
        <div>
          <h2>発電予測履歴</h2>
          <p>{latest ? `${items.length}日分 / 最新 ${latest.forecastDate}` : `${selectedRange.label} / 履歴待ち`}</p>
        </div>
        <RangeSelector ranges={ranges} selectedRange={selectedRange} onRangeChange={onRangeChange} />
      </div>
      {error ? <p className="inline-error">{error}</p> : null}
      {latest ? (
        <div className="estimate-grid pv-history-summary" aria-label="pv forecast history summary">
          <DetailText label="最新補正後予測" value={`${formatDecimal(latest.correctedEstimatedPvKwh)} kWh`} />
          <DetailText label="充電想定" value={`${formatDecimal(latest.correctedEstimatedPvToBatteryKwh)} kWh`} />
          <DetailText label="不足見込" value={`${formatDecimal(latest.forecastDaytimeDeficitKwh)} kWh`} />
          <DetailText label="目標残量" value={`${latest.recommendedNightTargetSoc}%`} />
        </div>
      ) : null}
      {chartData.length === 0 ? (
        <Card>
          <CardHeader>
            <CardTitle>履歴なし</CardTitle>
            <CardDescription>夜間計画ログが蓄積されると表示されます。</CardDescription>
          </CardHeader>
        </Card>
      ) : (
        <div className="chart-grid">
          <Card>
            <CardHeader>
              <CardTitle>発電予測 kWh</CardTitle>
              <CardDescription>予測 / 補正後 / 充電想定 / 不足見込</CardDescription>
            </CardHeader>
            <CardContent>
              <ChartLegend config={pvHistoryChartConfig} />
              <ChartContainer config={pvHistoryChartConfig}>
                <LineChart data={chartData} margin={{ top: 12, right: 12, left: -12, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" vertical={false} />
                  <XAxis dataKey="label" tickLine={false} axisLine={false} minTickGap={24} />
                  <YAxis tickLine={false} axisLine={false} width={58} tickFormatter={(value) => `${value}kWh`} />
                  <ChartTooltip content={<ChartTooltipContent config={pvHistoryChartConfig} />} />
                  <Line dataKey="estimatedPvKwh" type="monotone" stroke="var(--color-estimatedPvKwh)" strokeWidth={2} dot={false} isAnimationActive={false} />
                  <Line dataKey="correctedEstimatedPvKwh" type="monotone" stroke="var(--color-correctedEstimatedPvKwh)" strokeWidth={2} dot={false} isAnimationActive={false} />
                  <Line dataKey="correctedEstimatedPvToBatteryKwh" type="monotone" stroke="var(--color-correctedEstimatedPvToBatteryKwh)" strokeWidth={2} dot={false} isAnimationActive={false} />
                  <Line dataKey="forecastDaytimeDeficitKwh" type="monotone" stroke="var(--color-forecastDaytimeDeficitKwh)" strokeWidth={2} dot={false} isAnimationActive={false} />
                </LineChart>
              </ChartContainer>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>深夜充電判断</CardTitle>
              <CardDescription>推奨目標残量 / 必要深夜充電量</CardDescription>
            </CardHeader>
            <CardContent>
              <ChartLegend config={nightDecisionChartConfig} />
              <ChartContainer config={nightDecisionChartConfig}>
                <LineChart data={chartData} margin={{ top: 12, right: 12, left: -12, bottom: 0 }}>
                  <CartesianGrid strokeDasharray="3 3" vertical={false} />
                  <XAxis dataKey="label" tickLine={false} axisLine={false} minTickGap={24} />
                  <YAxis yAxisId="soc" tickLine={false} axisLine={false} width={48} tickFormatter={(value) => `${value}%`} />
                  <YAxis yAxisId="kwh" orientation="right" tickLine={false} axisLine={false} width={58} tickFormatter={(value) => `${value}kWh`} />
                  <ChartTooltip content={<ChartTooltipContent config={nightDecisionChartConfig} />} />
                  <Line yAxisId="soc" dataKey="recommendedNightTargetSoc" type="monotone" stroke="var(--color-recommendedNightTargetSoc)" strokeWidth={2} dot={false} isAnimationActive={false} />
                  <Line yAxisId="kwh" dataKey="requiredNightChargeKwh" type="monotone" stroke="var(--color-requiredNightChargeKwh)" strokeWidth={2} dot={false} isAnimationActive={false} />
                </LineChart>
              </ChartContainer>
            </CardContent>
          </Card>
        </div>
      )}
    </section>
  );
}

function RangeSelector({
  ranges,
  selectedRange,
  onRangeChange
}: {
  ranges: readonly PVForecastHistoryRange[];
  selectedRange: PVForecastHistoryRange;
  onRangeChange: (range: PVForecastHistoryRange) => void;
}) {
  return (
    <div className="range-selector" aria-label="pv forecast history range">
      {ranges.map((range) => (
        <Button
          key={range.label}
          type="button"
          variant={range.label === selectedRange.label ? "default" : "outline"}
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

function DetailText({ label, value }: { label: string; value: string }) {
  return (
    <div className="estimate-item">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function formatDateLabel(value: string) {
  if (!value) {
    return "-";
  }
  const date = new Date(`${value}T00:00:00`);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleDateString("ja-JP", {
    month: "numeric",
    day: "numeric"
  });
}

function formatDecimal(value: number) {
  if (!Number.isFinite(value)) {
    return "0.0";
  }
  return value.toFixed(1);
}
