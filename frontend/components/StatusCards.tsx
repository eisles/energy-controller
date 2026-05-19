import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import type { EnergyStatus, NightChargePlan, SurplusPlan } from "@/lib/types";
import type { ReactNode } from "react";

type Metric = {
  label: string;
  value: number | string;
  unit?: string;
  description?: string;
};

type MetricKey = "gridW" | "importW" | "exportW" | "batterySoc" | "targetChargeW" | "state";

const primaryMetrics: MetricKey[] = ["gridW", "importW", "exportW", "batterySoc", "targetChargeW", "state"];

export function StatusCards({ status, fetchError }: { status: EnergyStatus; fetchError: string | null }) {
  const netBatteryW = status.batteryInputW - status.batteryOutputW;
  const metrics: Record<MetricKey, Metric> = {
    gridW: { label: "Grid", value: formatGridFlow(status.gridW) },
    importW: { label: "Import", value: status.importW, unit: "W", description: "現在の買電" },
    exportW: { label: "Export", value: status.exportW, unit: "W", description: "現在の売電" },
    batterySoc: {
      label: "Battery",
      value: formatNetBatteryFlow(netBatteryW),
      description: `SOC ${status.batterySoc}% / 入力 ${status.batteryInputW}W / 出力 ${status.batteryOutputW}W`
    },
    targetChargeW: { label: "Target charge", value: status.targetChargeW, unit: "W", description: "推奨充電W" },
    state: { label: "State", value: status.state || "-" }
  };

  return (
    <>
      {fetchError ? (
        <Alert variant="destructive" className="section">
          <AlertTitle>API read failed</AlertTitle>
          <AlertDescription>{fetchError}</AlertDescription>
        </Alert>
      ) : null}

      <section className="metric-grid section" aria-label="current energy status">
        {primaryMetrics.map((key) => (
          <MetricCard key={key} metric={metrics[key]} />
        ))}
      </section>

      <Card className="decision-panel section">
        <CardHeader>
          <CardDescription>Last decision</CardDescription>
          <CardTitle>{status.lastDecisionReason || "-"}</CardTitle>
        </CardHeader>
        <CardContent className="detail-strip" aria-label="status detail">
          <Detail label="Mode" value={<Badge variant="secondary">{status.mode || "-"}</Badge>} />
          <Detail label="Net battery" value={formatNetBatteryFlow(netBatteryW)} />
          <Detail label="Battery input" value={`${status.batteryInputW} W`} />
          <Detail label="Battery output" value={`${status.batteryOutputW} W`} />
          <Detail label="AC charge limit" value={`${status.acChargeLimitW} W`} />
          <Detail label="Backup reserve" value={nullablePercent(status.backupReserveSoc)} />
          <Detail label="TOU mode" value={nullableOnOff(status.touModeEnabled)} />
          <Detail label="Battery capacity" value={formatBatteryCapacity(status.batteryFullEnergyWh)} />
          <Detail label="Updated" value={formatDateTime(status.updatedAt)} />
        </CardContent>
      </Card>

      <SurplusPlanCard plan={status.surplusPlan} />
      <NightChargePlanCard plan={status.nightChargePlan} />

      {status.lastError ? (
        <Alert variant="destructive" className="section">
          <AlertTitle>Last error</AlertTitle>
          <AlertDescription>{status.lastError}</AlertDescription>
        </Alert>
      ) : null}
    </>
  );
}

function NightChargePlanCard({ plan }: { plan?: NightChargePlan | null }) {
  if (!plan) {
    return null;
  }

  return (
    <Card className="planner-panel section">
      <CardHeader>
        <div className="panel-title-row">
          <div>
            <CardDescription>Weather forecast planner</CardDescription>
            <CardTitle>深夜充電プラン</CardTitle>
          </div>
          <Badge variant={plan.wouldWrite ? "warning" : "secondary"}>{plan.wouldWrite ? "would write" : "read-only"}</Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div className="detail-strip" aria-label="night charging planner detail">
          <Detail label="PV期待度" value={`${plan.solarForecastScore}/100`} />
          <Detail label="推奨深夜SOC" value={`${plan.recommendedNightTargetSoc}%`} />
          <Detail label="最低確保SOC" value={`${plan.minimumReserveSoc}%`} />
          <Detail label="今夜充電" value={yesNo(plan.shouldChargeTonight)} />
          <Detail label="対象日" value={plan.targetForecast?.date || "-"} />
          <Detail label="日射量" value={plan.targetForecast ? `${formatDecimal(plan.targetForecast.shortwaveRadiationMjPerM2)} MJ/m2` : "-"} />
        </div>
        <div className="detail-strip planner-secondary" aria-label="weather forecast detail">
          <Detail label="推定PV発電" value={`${formatDecimal(plan.estimatedPvKwh)} kWh`} />
          <Detail label="推定日中消費" value={`${formatDecimal(plan.estimatedDaytimeLoadKwh)} kWh`} />
          <Detail label="推定余剰" value={`${formatDecimal(plan.estimatedSurplusKwh)} kWh`} />
          <Detail label="充電余地" value={`${formatDecimal(plan.batteryChargeHeadroomKwh)} kWh`} />
          <Detail label="容量ソース" value={capacitySourceLabel(plan.batteryCapacitySource)} />
          <Detail label="日射量" value={`${formatDecimal(plan.solarRadiationKwhPerM2)} kWh/m2`} />
        </div>
        <div className="detail-strip planner-secondary" aria-label="weather forecast detail">
          <Detail label="日照" value={plan.targetForecast ? `${formatDecimal(plan.targetForecast.sunshineDurationHours)} h` : "-"} />
          <Detail label="雲量" value={plan.targetForecast ? `${plan.targetForecast.cloudCoverMeanPercent}%` : "-"} />
          <Detail label="降水確率" value={plan.targetForecast ? `${plan.targetForecast.precipitationProbabilityMax}%` : "-"} />
          <Detail label="降水量" value={plan.targetForecast ? `${formatDecimal(plan.targetForecast.precipitationSumMm)} mm` : "-"} />
          <Detail label="取得元" value={plan.targetForecast?.provider || "-"} />
          <Detail label="天気コード" value={plan.targetForecast ? `${plan.targetForecast.weatherCode}` : "-"} />
        </div>
        <p className="planner-reason">{plan.reason || "-"}</p>
      </CardContent>
    </Card>
  );
}

function SurplusPlanCard({ plan }: { plan?: SurplusPlan | null }) {
  if (!plan) {
    return null;
  }

  return (
    <Card className="planner-panel section">
      <CardHeader>
        <div className="panel-title-row">
          <div>
            <CardDescription>Read-only planner</CardDescription>
            <CardTitle>余剰追従プラン</CardTitle>
          </div>
          <Badge variant={plan.wouldWrite ? "warning" : "secondary"}>{plan.wouldWrite ? "would write" : "read-only"}</Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div className="detail-strip" aria-label="surplus planner detail">
          <Detail label="Net battery" value={formatNetBatteryFlow(plan.netBatteryW)} />
          <Detail label="推奨AC充電" value={`${plan.recommendedAcChargeLimitW} W`} />
          <Detail label="推奨リザーブ" value={nullablePercent(plan.recommendedBackupReserveSoc)} />
          <Detail label="AC調整" value={yesNo(plan.shouldAdjustAcChargeLimit)} />
          <Detail label="リザーブ引上げ" value={yesNo(plan.shouldRaiseBackupReserve)} />
        </div>
        <p className="planner-reason">{plan.reason || "-"}</p>
      </CardContent>
    </Card>
  );
}

function MetricCard({ metric }: { metric: Metric }) {
  return (
    <Card className="metric-card">
      <CardHeader>
        <CardDescription>{metric.label}</CardDescription>
        <CardTitle className="metric-value">
          {metric.value}
          {metric.unit ? <span className="metric-unit">{metric.unit}</span> : null}
        </CardTitle>
      </CardHeader>
      {metric.description ? <CardContent className="metric-description">{metric.description}</CardContent> : null}
    </Card>
  );
}

function formatGridFlow(value: number) {
  if (value > 0) {
    return `買電 ${value} W`;
  }
  if (value < 0) {
    return `売電 ${Math.abs(value)} W`;
  }
  return "買電/売電 0 W";
}

function formatNetBatteryFlow(netBatteryW: number) {
  if (netBatteryW > 0) {
    return `実質充電 ${netBatteryW} W`;
  }
  if (netBatteryW < 0) {
    return `実質放電 ${Math.abs(netBatteryW)} W`;
  }
  return "待機中 0 W";
}

function nullablePercent(value: number | null | undefined) {
  return value === null || value === undefined ? "-" : `${value}%`;
}

function nullableOnOff(value: boolean | null | undefined) {
  if (value === null || value === undefined) {
    return "-";
  }
  return value ? "ON" : "OFF";
}

function yesNo(value: boolean) {
  return value ? "あり" : "なし";
}

function formatDecimal(value: number) {
  return new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 1 }).format(value);
}

function formatBatteryCapacity(value: number | null | undefined) {
  if (value === null || value === undefined) {
    return "-";
  }
  return `${formatDecimal(value / 1000)} kWh`;
}

function capacitySourceLabel(value: string) {
  if (value === "device") {
    return "実機取得";
  }
  if (value === "manual") {
    return "手動設定";
  }
  return "-";
}

function Detail({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="detail-item">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function formatDateTime(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString("ja-JP");
}
