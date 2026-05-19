"use client";

import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ChartContainer, ChartTooltip, ChartTooltipContent, type ChartConfig } from "@/components/ui/chart";
import type { PowerLog } from "@/lib/types";

const powerChartConfig = {
  gridW: { label: "Grid", color: "#2563eb", unit: "W" },
  importW: { label: "買電", color: "#dc2626", unit: "W" },
  exportW: { label: "売電", color: "#16a34a", unit: "W" },
  targetChargeW: { label: "推奨充電", color: "#7c3aed", unit: "W" }
} satisfies ChartConfig;

const batteryChartConfig = {
  batterySoc: { label: "SOC", color: "#0f766e", unit: "%" },
  netBatteryW: { label: "実質", color: "#db2777", unit: "W" },
  batteryInputW: { label: "入力", color: "#ea580c", unit: "W" },
  batteryOutputW: { label: "出力", color: "#0891b2", unit: "W" }
} satisfies ChartConfig;

type ChartRange = {
  label: string;
  hours: number | null;
};

type EnergyChartsProps = {
  logs: PowerLog[];
  rangeLabel: string;
  ranges: readonly ChartRange[];
  selectedRange: ChartRange;
  onRangeChange: (range: ChartRange) => void;
};

export function EnergyCharts({ logs, rangeLabel, ranges, selectedRange, onRangeChange }: EnergyChartsProps) {
  const chartData = logs
    .slice()
    .reverse()
    .map((log) => ({
      label: formatTime(log.measuredAt),
      measuredAt: log.measuredAt,
      gridW: log.gridW,
      importW: log.importW,
      exportW: log.exportW,
      targetChargeW: log.targetChargeW,
      batterySoc: log.batterySoc,
      batteryInputW: log.batteryInputW,
      batteryOutputW: log.batteryOutputW,
      netBatteryW: netBatteryW(log)
    }));

  if (chartData.length === 0) {
    return (
      <Card className="section">
        <CardHeader>
          <CardTitle>推移グラフ</CardTitle>
          <CardDescription>ログが蓄積されると選択した時間範囲の推移を表示します。</CardDescription>
        </CardHeader>
        <CardContent>
          <RangeSelector ranges={ranges} selectedRange={selectedRange} onRangeChange={onRangeChange} />
        </CardContent>
      </Card>
    );
  }

  return (
    <section className="section" aria-label="energy charts">
      <div className="chart-toolbar">
        <div>
          <h2>推移グラフ</h2>
          <p>{rangeLabel} のログを表示中</p>
        </div>
        <RangeSelector ranges={ranges} selectedRange={selectedRange} onRangeChange={onRangeChange} />
      </div>
      <div className="chart-grid">
        <Card>
          <CardHeader>
            <CardTitle>電力推移</CardTitle>
            <CardDescription>Grid / 買電 / 売電 / 推奨充電W</CardDescription>
          </CardHeader>
          <CardContent>
            <ChartLegend config={powerChartConfig} />
            <ChartContainer config={powerChartConfig}>
              <LineChart data={chartData} margin={{ top: 12, right: 12, left: -12, bottom: 0 }}>
                <CartesianGrid strokeDasharray="3 3" vertical={false} />
                <XAxis dataKey="label" tickLine={false} axisLine={false} minTickGap={28} />
                <YAxis tickLine={false} axisLine={false} width={58} tickFormatter={(value) => `${value}W`} />
                <ChartTooltip content={<ChartTooltipContent config={powerChartConfig} />} />
                <Line dataKey="gridW" type="monotone" stroke="var(--color-gridW)" strokeWidth={2} dot={false} isAnimationActive={false} />
                <Line dataKey="importW" type="monotone" stroke="var(--color-importW)" strokeWidth={2} dot={false} isAnimationActive={false} />
                <Line dataKey="exportW" type="monotone" stroke="var(--color-exportW)" strokeWidth={2} dot={false} isAnimationActive={false} />
                <Line dataKey="targetChargeW" type="monotone" stroke="var(--color-targetChargeW)" strokeWidth={2} dot={false} isAnimationActive={false} />
              </LineChart>
            </ChartContainer>
          </CardContent>
        </Card>

      <Card>
        <CardHeader>
          <CardTitle>バッテリー推移</CardTitle>
          <CardDescription>SOC / 実質充電W / 入力W / 出力W</CardDescription>
        </CardHeader>
        <CardContent>
          <ChartLegend config={batteryChartConfig} />
          <ChartContainer config={batteryChartConfig}>
            <LineChart data={chartData} margin={{ top: 12, right: 12, left: -12, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" vertical={false} />
              <XAxis dataKey="label" tickLine={false} axisLine={false} minTickGap={28} />
              <YAxis yAxisId="watts" tickLine={false} axisLine={false} width={58} tickFormatter={(value) => `${value}W`} />
              <YAxis yAxisId="soc" orientation="right" tickLine={false} axisLine={false} width={42} tickFormatter={(value) => `${value}%`} />
              <ChartTooltip content={<ChartTooltipContent config={batteryChartConfig} />} />
              <Line yAxisId="soc" dataKey="batterySoc" type="monotone" stroke="var(--color-batterySoc)" strokeWidth={2} dot={false} connectNulls isAnimationActive={false} />
              <Line yAxisId="watts" dataKey="netBatteryW" type="monotone" stroke="var(--color-netBatteryW)" strokeWidth={3} dot={false} connectNulls isAnimationActive={false} />
              <Line yAxisId="watts" dataKey="batteryInputW" type="monotone" stroke="var(--color-batteryInputW)" strokeWidth={2} dot={false} connectNulls isAnimationActive={false} />
              <Line yAxisId="watts" dataKey="batteryOutputW" type="monotone" stroke="var(--color-batteryOutputW)" strokeWidth={2} dot={false} connectNulls isAnimationActive={false} />
            </LineChart>
          </ChartContainer>
        </CardContent>
      </Card>
      </div>
    </section>
  );
}

function RangeSelector({ ranges, selectedRange, onRangeChange }: { ranges: readonly ChartRange[]; selectedRange: ChartRange; onRangeChange: (range: ChartRange) => void }) {
  return (
    <div className="range-selector" aria-label="chart time range">
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

function formatTime(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleTimeString("ja-JP", {
    hour: "2-digit",
    minute: "2-digit"
  });
}

function netBatteryW(log: PowerLog) {
  if (log.batteryInputW === null || log.batteryOutputW === null) {
    return null;
  }
  return log.batteryInputW - log.batteryOutputW;
}
