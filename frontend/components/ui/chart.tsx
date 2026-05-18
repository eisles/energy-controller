"use client";

import type { CSSProperties, ReactElement } from "react";
import { ResponsiveContainer, Tooltip } from "recharts";
import { cn } from "@/lib/utils";

export type ChartConfig = Record<
  string,
  {
    label: string;
    color: string;
    unit?: string;
  }
>;

type ChartContainerProps = {
  config: ChartConfig;
  className?: string;
  children: ReactElement;
};

export function ChartContainer({ config, className, children }: ChartContainerProps) {
  const style = Object.fromEntries(
    Object.entries(config).map(([key, item]) => [`--color-${key}`, item.color])
  ) as CSSProperties;

  return (
    <div className={cn("ui-chart", className)} style={style}>
      <ResponsiveContainer width="100%" height="100%">
        {children}
      </ResponsiveContainer>
    </div>
  );
}

export const ChartTooltip = Tooltip;

type TooltipPayload = {
  color?: string;
  dataKey?: string | number;
  name?: string | number;
  value?: string | number | null;
};

type ChartTooltipContentProps = {
  active?: boolean;
  label?: string | number;
  payload?: TooltipPayload[];
  config: ChartConfig;
};

export function ChartTooltipContent({ active, label, payload, config }: ChartTooltipContentProps) {
  if (!active || !payload?.length) {
    return null;
  }

  return (
    <div className="ui-chart-tooltip">
      <div className="ui-chart-tooltip-label">{label}</div>
      <div className="ui-chart-tooltip-list">
        {payload.map((item) => {
          const key = String(item.dataKey ?? item.name ?? "");
          const meta = config[key];
          return (
            <div className="ui-chart-tooltip-row" key={key}>
              <span className="ui-chart-tooltip-dot" style={{ background: item.color ?? meta?.color }} />
              <span>{meta?.label ?? key}</span>
              <strong>{formatValue(item.value, meta?.unit)}</strong>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function formatValue(value: string | number | null | undefined, unit?: string) {
  if (value === null || value === undefined || value === "") {
    return "-";
  }
  if (typeof value === "number") {
    return `${value.toLocaleString()}${unit ? ` ${unit}` : ""}`;
  }
  return `${value}${unit ? ` ${unit}` : ""}`;
}
