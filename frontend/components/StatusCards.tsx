import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { CollapsibleSection } from "@/components/CollapsibleSection";
import { decisionReasonLabel, decisionSummaryLabel, guardReasonLabel, strategyStateLabel, writeCandidateLabel } from "@/lib/display-labels";
import type { Delta3Status, DeviceStatus, EnergyStatus, NightChargePlan, SurplusPlan } from "@/lib/types";
import type { ReactNode } from "react";

type Metric = {
  label: string;
  value: number | string;
  unit?: string;
  sideValue?: string;
  valueClassName?: string;
  description?: string;
};

type MetricKey = "gridW" | "importW" | "exportW" | "batterySoc" | "netBatteryW" | "targetChargeW";

const primaryMetrics: MetricKey[] = ["gridW", "importW", "exportW", "batterySoc", "netBatteryW", "targetChargeW"];

export type StatusCardSectionKey = "charts" | "decision" | "surplusPlan" | "nightPlan";

type StatusCardsProps = {
  status: EnergyStatus;
  fetchError: string | null;
};

export function StatusCards({ status, fetchError }: StatusCardsProps) {
  const netBatteryW = status.batteryInputW - status.batteryOutputW;
  const batteryEnergy = batteryEnergySummary(status.batteryFullEnergyWh, status.batterySoc);
  const metrics: Record<MetricKey, Metric> = {
    gridW: { label: "Grid", value: formatGridFlow(status.gridW), valueClassName: gridFlowClassName(status.gridW) },
    importW: { label: "Import", value: status.importW, unit: "W", description: "現在の買電" },
    exportW: { label: "Export", value: status.exportW, unit: "W", description: "現在の売電" },
    batterySoc: {
      label: "バッテリー残量",
      value: status.batterySoc,
      unit: "%",
      sideValue: batteryEnergyLabel(batteryEnergy),
      description: batterySocDescription(batteryEnergy, status.batteryOutputW, status.backupReserveSoc)
    },
    netBatteryW: {
      label: "Net battery",
      value: formatNetBatteryFlow(netBatteryW),
      description: `入力 ${status.batteryInputW}W / 出力 ${status.batteryOutputW}W`
    },
    targetChargeW: {
      label: "充電推奨",
      value: status.targetChargeW,
      unit: "W",
      description: chargeRecommendationDescription(status.exportW, status.targetChargeW)
    }
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

      {status.lastError ? (
        <Alert variant="destructive" className="section">
          <AlertTitle>Last error</AlertTitle>
          <AlertDescription>{status.lastError}</AlertDescription>
        </Alert>
      ) : null}
    </>
  );
}

export function Delta3StatusCard({
  status,
  deviceStatuses,
  sourceStatus,
  fetchError
}: {
  status: Delta3Status | null;
  deviceStatuses: DeviceStatus[];
  sourceStatus: EnergyStatus;
  fetchError: string | null;
}) {
  const unavailableReason = fetchError || status?.lastError || "read-only status is not loaded";
  const delta3Statuses = deviceStatuses.filter((device) => device.kind === "ecoflow_delta3_plus");
  const availableCount = delta3Statuses.filter((device) => device.status.available).length;
  const available = !fetchError && (availableCount > 0 || Boolean(status?.available));
  const auxiliaryPlan = sourceStatus.delta3AuxPlan;
  return (
    <Card className="delta3-status-card section">
      <CardHeader>
        <div className="panel-title-row">
          <div>
            <CardDescription>Read-only auxiliary battery</CardDescription>
            <CardTitle>DELTA 3 Plus</CardTitle>
          </div>
          <Badge variant={available ? "success" : "secondary"}>{availableCount > 0 ? `${availableCount}/${delta3Statuses.length} 接続中` : available ? "connected" : "unavailable"}</Badge>
        </div>
      </CardHeader>
      <CardContent>
        {delta3Statuses.length > 0 ? (
          <>
            <div className="delta3-device-status-list">
              {delta3Statuses.map((device) => (
                <div className="delta3-device-status-item" key={device.id || device.credentialRef}>
                  <div className="panel-title-row">
                    <div>
                      <strong>{device.name}</strong>
                      <p className="readonly-note">
                        優先 {device.priority} / {device.credentialRef} / {statusSourceLabel(device.statusSource)} / {maskDeviceSn(device.deviceSn)}
                      </p>
                    </div>
                    <Badge variant={device.status.available ? "success" : "secondary"}>{device.status.available ? "接続中" : "取得不可"}</Badge>
                  </div>
                  {device.status.available ? (
                    <>
                      <div className="detail-strip" aria-label={`${device.name} read-only status`}>
                        <Detail label="SOC" value={nullablePercent(device.status.soc)} />
                        <Detail label="AC入力" value={nullableWatt(device.status.acInW)} />
                        <Detail label="AC出力" value={nullableWatt(device.status.acOutW)} />
                        <Detail label="AC充電上限" value={nullableWatt(device.status.acChargeLimitW)} />
                        <Detail label="Grid bypass disabled" value={nullableOnOff(device.status.gridBypassDisabled)} />
                        <Detail label="AC output" value={nullableOnOff(device.status.acOutputEnabled)} />
                      </div>
                      <div className="detail-strip planner-secondary" aria-label={`${device.name} configuration status`}>
                        <Detail label="最大充電SOC" value={nullablePercent(device.status.maxChargeSoc)} />
                        <Detail label="最低放電SOC" value={nullablePercent(device.status.minDischargeSoc)} />
                        <Detail label="Backup reserve" value={nullablePercent(device.status.backupReserveSoc)} />
                        <Detail label="Device type" value={device.status.deviceType || device.deviceType || "-"} />
                        <Detail label="Updated" value={formatDateTime(device.status.updatedAt || "")} />
                        <Detail label="制御候補" value={device.controlEnabled ? "有効" : "無効"} />
                      </div>
                    </>
                  ) : (
                    <Alert className="delta3-alert">
                      <AlertTitle>{device.name} read-only status</AlertTitle>
                      <AlertDescription>{device.status.lastError || "read-only status is not loaded"}</AlertDescription>
                    </Alert>
                  )}
                </div>
              ))}
            </div>
            <div className="detail-strip planner-secondary" aria-label="DELTA 3 Plus auxiliary battery plan">
              <Detail label="補助計画" value={strategyStateLabel(auxiliaryPlan?.strategyState || "UNAVAILABLE")} />
              <Detail label="推奨AC上限" value={nullableWatt(auxiliaryPlan?.recommendedAcChargeLimitW)} />
              <Detail label="現在AC上限" value={nullableWatt(auxiliaryPlan?.currentAcChargeLimitW)} />
              <Detail label="残余売電" value={nullableWatt(auxiliaryPlan?.residualExportW ?? sourceStatus.exportW)} />
              <Detail label="安全余力" value={nullableWatt(auxiliaryPlan?.safetyMarginW)} />
              <Detail label="実行" value={writeCandidateLabel(auxiliaryPlan?.wouldWrite)} />
              <Detail label="抑制" value={guardReasonLabel(auxiliaryPlan?.suppressedReason)} />
              <Detail label="理由" value={decisionReasonLabel(auxiliaryPlan?.reason)} />
            </div>
          </>
        ) : available && status ? (
          <>
            <div className="detail-strip" aria-label="DELTA 3 Plus read-only status">
              <Detail label="SOC" value={nullablePercent(status.soc)} />
              <Detail label="AC入力" value={nullableWatt(status.acInW)} />
              <Detail label="AC出力" value={nullableWatt(status.acOutW)} />
              <Detail label="AC充電上限" value={nullableWatt(status.acChargeLimitW)} />
              <Detail label="Grid bypass disabled" value={nullableOnOff(status.gridBypassDisabled)} />
              <Detail label="AC output" value={nullableOnOff(status.acOutputEnabled)} />
            </div>
            <div className="detail-strip planner-secondary" aria-label="DELTA 3 Plus configuration status">
              <Detail label="最大充電SOC" value={nullablePercent(status.maxChargeSoc)} />
              <Detail label="最低放電SOC" value={nullablePercent(status.minDischargeSoc)} />
              <Detail label="Backup reserve" value={nullablePercent(status.backupReserveSoc)} />
              <Detail label="Backup reserve enabled" value={nullableOnOff(status.backupReserveEnabled)} />
              <Detail label="Device type" value={status.deviceType || "-"} />
              <Detail label="Updated" value={formatDateTime(status.updatedAt || "")} />
            </div>
            <div className="detail-strip planner-secondary" aria-label="DELTA 3 Plus auxiliary battery plan">
              <Detail label="補助計画" value={strategyStateLabel(auxiliaryPlan?.strategyState || "UNAVAILABLE")} />
              <Detail label="推奨AC上限" value={nullableWatt(auxiliaryPlan?.recommendedAcChargeLimitW)} />
              <Detail label="現在AC上限" value={nullableWatt(auxiliaryPlan?.currentAcChargeLimitW)} />
              <Detail label="残余売電" value={nullableWatt(auxiliaryPlan?.residualExportW ?? sourceStatus.exportW)} />
              <Detail label="安全余力" value={nullableWatt(auxiliaryPlan?.safetyMarginW)} />
              <Detail label="実行" value={writeCandidateLabel(auxiliaryPlan?.wouldWrite)} />
              <Detail label="抑制" value={guardReasonLabel(auxiliaryPlan?.suppressedReason)} />
              <Detail label="理由" value={decisionReasonLabel(auxiliaryPlan?.reason)} />
            </div>
          </>
        ) : (
          <Alert className="delta3-alert">
            <AlertTitle>DELTA 3 Plus read-only status</AlertTitle>
            <AlertDescription>{unavailableReason}</AlertDescription>
          </Alert>
        )}
      </CardContent>
    </Card>
  );
}

function maskDeviceSn(value: string) {
  if (!value) {
    return "SN未設定";
  }
  if (value.length <= 8) {
    return value;
  }
  return `${value.slice(0, 4)}...${value.slice(-4)}`;
}

function statusSourceLabel(value: string) {
  const labels: Record<string, string> = {
    ecoflow_cloud: "EcoFlow Cloud API",
    ecoflow_private_mqtt: "EcoFlow private MQTT",
    switchbot_cloud: "SwitchBot Cloud API",
    manual: "手動"
  };
  return labels[value] || value || "未設定";
}

export function StatusDecisionSection({
  status,
  open,
  onToggle,
  headerControls
}: {
  status: EnergyStatus;
  open: boolean;
  onToggle: () => void;
  headerControls?: ReactNode;
}) {
  const netBatteryW = status.batteryInputW - status.batteryOutputW;
  const decisionSummary = decisionSummaryLabel(status.lastDecisionReason);
  return (
    <CollapsibleSection title="制御判断" summary={decisionSummary} open={open} onToggle={onToggle} headerControls={headerControls}>
      <Card className="decision-panel section">
        <CardHeader>
          <CardDescription>Last decision</CardDescription>
          <CardTitle>{decisionSummary}</CardTitle>
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
    </CollapsibleSection>
  );
}

export function StatusChartSection({
  open,
  onToggle,
  children,
  summary,
  headerControls
}: {
  open: boolean;
  onToggle: () => void;
  children: ReactNode;
  summary: string;
  headerControls?: ReactNode;
}) {
  return (
    <CollapsibleSection title="推移グラフ" summary={summary} open={open} onToggle={onToggle} headerControls={headerControls}>
      {children}
    </CollapsibleSection>
  );
}

export function NightChargePlanSection({
  plan,
  open,
  onToggle,
  headerControls
}: {
  plan?: NightChargePlan | null;
  open: boolean;
  onToggle: () => void;
  headerControls?: ReactNode;
}) {
  if (!plan) {
    return null;
  }

  return (
    <CollapsibleSection title="深夜充電プラン" summary={nightPlanSummary(plan)} open={open} onToggle={onToggle} headerControls={headerControls}>
      <Card className="planner-panel section">
        <CardHeader>
          <div className="panel-title-row">
            <div>
              <CardDescription>Weather forecast planner</CardDescription>
              <CardTitle>深夜充電プラン</CardTitle>
            </div>
            <Badge variant={plan.wouldWrite ? "warning" : "secondary"}>{writeCandidateLabel(plan.wouldWrite)}</Badge>
          </div>
        </CardHeader>
        <CardContent>
          <div className="detail-strip" aria-label="night charging planner detail">
            <Detail label="状態" value={strategyStateLabel(plan.strategyState)} />
            <Detail label="PV期待度" value={`${plan.solarForecastScore}/100`} />
            <Detail label="推奨mode" value={modeLabel(plan.recommendedMode)} />
            <Detail label="推奨深夜SOC" value={`${plan.recommendedNightTargetSoc}%`} />
            <Detail label="最低確保SOC" value={`${plan.minimumReserveSoc}%`} />
            <Detail label="今夜充電" value={yesNo(plan.shouldChargeTonight)} />
            <Detail label="対象日" value={plan.targetForecast?.date || "-"} />
            <Detail label="日射量" value={plan.targetForecast ? `${formatDecimal(plan.targetForecast.shortwaveRadiationMjPerM2)} MJ/m2` : "-"} />
          </div>
          <div className="detail-strip planner-secondary" aria-label="weather forecast detail">
            <Detail label="推定PV発電" value={`${formatDecimal(plan.dailyEstimatedPvKwh || plan.estimatedPvKwh)} kWh`} />
            <Detail label="PV有効時間" value={formatTimeWindow(plan.pvEffectiveStartAt, plan.pvEffectiveEndAt)} />
            <Detail label="PV時間ソース" value={plan.pvEffectiveWindowSource || "-"} />
            <Detail label="PVしきい値" value={plan.pvEffectiveRadiationWPerM2 ? `${formatDecimal(plan.pvEffectiveRadiationWPerM2)} W/m2` : "-"} />
            <Detail label="推定日中消費" value={`${formatDecimal(plan.estimatedDaytimeLoadKwh)} kWh`} />
            <Detail label="朝まで消費" value={`${formatDecimal(plan.estimatedMorningLoadKwh)} kWh`} />
            <Detail label="7時-PV開始" value={`${formatDecimal(plan.morningToPvStartLoadKwh)} kWh`} />
            <Detail label="PV不足見込" value={`${formatDecimal(plan.forecastDaytimeDeficitKwh)} kWh`} />
            <Detail label="推定余剰" value={`${formatDecimal(plan.estimatedSurplusKwh)} kWh`} />
            <Detail label="推定不足" value={`${formatDecimal(plan.estimatedDeficitKwh)} kWh`} />
            <Detail label="PV充電見込" value={`${formatDecimal(plan.estimatedPvToBatteryKwh)} kWh`} />
            <Detail label="安全余力" value={`${formatDecimal(plan.safetyMarginKwh)} kWh`} />
            <Detail label="充電余地" value={`${formatDecimal(plan.batteryChargeHeadroomKwh)} kWh`} />
            <Detail label="容量ソース" value={capacitySourceLabel(plan.batteryCapacitySource)} />
            <Detail label="消費ソース" value={consumptionSourceLabel(plan.consumptionSource)} />
            <Detail label="日射量" value={`${formatDecimal(plan.solarRadiationKwhPerM2)} kWh/m2`} />
          </div>
          <div className="detail-strip planner-secondary" aria-label="night charge energy detail">
            <Detail label="現在残量" value={`${formatDecimal(plan.currentBatteryEnergyKwh)} kWh`} />
            <Detail label="推奨残量" value={`${formatDecimal(plan.recommendedNightTargetKwh)} kWh`} />
            <Detail label="最低確保" value={`${formatDecimal(plan.minimumReserveKwh)} kWh`} />
            <Detail label="深夜必要量" value={`${formatDecimal(plan.requiredNightChargeKwh)} kWh`} />
            <Detail label="容量" value={`${formatDecimal(plan.batteryCapacityKwh)} kWh`} />
          </div>
          <div className="detail-strip planner-secondary" aria-label="night charge command guard detail">
            <Detail label="候補AC上限" value={plan.recommendedAcChargeLimitW > 0 ? `${plan.recommendedAcChargeLimitW} W` : "-"} />
            <Detail label="候補リザーブ" value={nullablePercent(plan.recommendedBackupReserveSoc)} />
            <Detail label="AC上限変更" value={yesNo(plan.shouldSetAcChargeLimit)} />
            <Detail label="リザーブ変更" value={yesNo(plan.shouldSetBackupReserve)} />
            <Detail label="TOU解除" value={yesNo(plan.shouldDisableEnergyModes)} />
            <Detail label="TOU維持" value={yesNo(plan.shouldEnableTouMode)} />
            <Detail label="Self-powered" value={yesNo(plan.shouldEnableSelfPoweredMode)} />
            <Detail label="抑制" value={yesNo(plan.commandSuppressed)} />
          </div>
          <div className="detail-strip planner-secondary" aria-label="weather forecast detail">
            <Detail label="日照" value={plan.targetForecast ? `${formatDecimal(plan.targetForecast.sunshineDurationHours)} h` : "-"} />
            <Detail label="雲量" value={plan.targetForecast ? `${plan.targetForecast.cloudCoverMeanPercent}%` : "-"} />
            <Detail label="降水確率" value={plan.targetForecast ? `${plan.targetForecast.precipitationProbabilityMax}%` : "-"} />
            <Detail label="降水量" value={plan.targetForecast ? `${formatDecimal(plan.targetForecast.precipitationSumMm)} mm` : "-"} />
            <Detail label="取得元" value={plan.targetForecast?.provider || "-"} />
            <Detail label="天気コード" value={plan.targetForecast ? `${plan.targetForecast.weatherCode}` : "-"} />
          </div>
          {plan.actionSummary ? <p className="planner-reason">未送信計画: {decisionSummaryLabel(plan.actionSummary)}</p> : null}
          {plan.commandBlockReason ? <p className="planner-reason">抑制: {guardReasonLabel(plan.commandBlockReason)}</p> : null}
          <p className="planner-reason">{decisionSummaryLabel(plan.reason)}</p>
        </CardContent>
      </Card>
    </CollapsibleSection>
  );
}

export function SurplusPlanSection({
  plan,
  open,
  onToggle,
  headerControls
}: {
  plan?: SurplusPlan | null;
  open: boolean;
  onToggle: () => void;
  headerControls?: ReactNode;
}) {
  if (!plan) {
    return null;
  }

  return (
    <CollapsibleSection title="余剰追従プラン" summary={surplusPlanSummary(plan)} open={open} onToggle={onToggle} headerControls={headerControls}>
      <Card className="planner-panel section">
        <CardHeader>
          <div className="panel-title-row">
            <div>
              <CardDescription>Read-only planner</CardDescription>
              <CardTitle>余剰追従プラン</CardTitle>
            </div>
            <Badge variant={plan.wouldWrite ? "warning" : "secondary"}>{writeCandidateLabel(plan.wouldWrite)}</Badge>
          </div>
        </CardHeader>
        <CardContent>
          <div className="detail-strip" aria-label="surplus planner detail">
            <Detail label="状態" value={strategyStateLabel(plan.strategyState)} />
            <Detail label="Net battery" value={formatNetBatteryFlow(plan.netBatteryW)} />
            <Detail label="開始必要売電" value={`${plan.requiredStartExportW} W`} />
            <Detail label="開始余力" value={formatSignedW(plan.availableStartMarginW)} />
            <Detail label="推奨AC充電" value={`${plan.recommendedAcChargeLimitW} W`} />
            <Detail label="推奨リザーブ" value={nullablePercent(plan.recommendedBackupReserveSoc)} />
            <Detail label="AC調整" value={yesNo(plan.shouldAdjustAcChargeLimit)} />
            <Detail label="リザーブ引上げ" value={yesNo(plan.shouldRaiseBackupReserve)} />
            <Detail label="リザーブ戻し" value={yesNo(plan.shouldLowerBackupReserve)} />
            <Detail label="リザーブ同期" value={yesNo(plan.shouldAlignBackupReserve)} />
            <Detail label="Modes OFF" value={yesNo(plan.shouldDisableEnergyModes)} />
            <Detail label="TOU ON" value={yesNo(plan.shouldEnableTouMode)} />
          </div>
          <p className="planner-reason">{surplusActionLabel(plan)}</p>
          <p className="planner-reason">{decisionSummaryLabel(plan.reason)}</p>
        </CardContent>
      </Card>
    </CollapsibleSection>
  );
}

function logRangeSummary(status: EnergyStatus) {
  return status.updatedAt ? `更新 ${formatDateTime(status.updatedAt)}` : "更新待ち";
}

function surplusPlanSummary(plan: SurplusPlan) {
  return `${strategyStateLabel(plan.strategyState)} / ${surplusActionLabel(plan)}`;
}

function nightPlanSummary(plan: NightChargePlan) {
  return `${strategyStateLabel(plan.strategyState)} / 推奨深夜SOC ${plan.recommendedNightTargetSoc}% / PV ${formatDecimal(plan.dailyEstimatedPvKwh || plan.estimatedPvKwh)} kWh`;
}

function MetricCard({ metric }: { metric: Metric }) {
  return (
    <Card className="metric-card">
      <CardHeader>
        <CardDescription>{metric.label}</CardDescription>
        <CardTitle className="metric-value">
          <span className={metric.valueClassName}>
            {metric.value}
            {metric.unit ? <span className="metric-unit">{metric.unit}</span> : null}
          </span>
          {metric.sideValue ? <span className="metric-side-value">{metric.sideValue}</span> : null}
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

function gridFlowClassName(value: number) {
  if (value > 0) {
    return "metric-grid-import";
  }
  if (value < 0) {
    return "metric-grid-export";
  }
  return undefined;
}

function formatTimeWindow(start?: string, end?: string) {
  if (!start || !end) {
    return "-";
  }
  return `${formatTimeOnly(start)}-${formatTimeOnly(end)}`;
}

function formatTimeOnly(value: string) {
  const parts = value.split("T");
  return parts[1]?.slice(0, 5) || value;
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

function formatSignedW(value: number) {
  if (value > 0) {
    return `+${value} W`;
  }
  return `${value} W`;
}

function chargeRecommendationDescription(exportW: number, targetChargeW: number) {
  if (exportW > 0 && targetChargeW > 0) {
    return `余剰あり: ${targetChargeW}W充電推奨 / 未送信`;
  }
  if (targetChargeW > 0) {
    return `${targetChargeW}W充電推奨 / 未送信`;
  }
  if (exportW > 0) {
    return "余剰あり / 充電推奨なし";
  }
  return "充電推奨なし";
}

function surplusActionLabel(plan: SurplusPlan) {
  if (plan.actionSummary) {
    return `未送信計画: ${plan.actionSummary}`;
  }
  const reserveLabel =
    plan.recommendedBackupReserveSoc !== null && plan.recommendedBackupReserveSoc !== undefined
      ? `${plan.recommendedBackupReserveSoc}%`
      : null;
  const surplusActions: string[] = [];
  if (plan.shouldRaiseBackupReserve && reserveLabel) {
    surplusActions.push(`リザーブを${reserveLabel}へ引き上げ`);
  }
  if (plan.shouldAlignBackupReserve && reserveLabel) {
    surplusActions.push(`リザーブを現在SOCの${reserveLabel}へ合わせる`);
  }
  if (plan.shouldAdjustAcChargeLimit && plan.recommendedAcChargeLimitW > 0) {
    surplusActions.push(`AC充電を${plan.recommendedAcChargeLimitW}Wへ調整`);
  }
  if (plan.shouldDisableEnergyModes) {
    surplusActions.push("動作モードを全OFFに");
  }
  if (plan.shouldEnableTouMode) {
    surplusActions.push("TOUをONに戻す");
  }
  if (surplusActions.length > 0) {
    return `売電抑制: 充電開始には${surplusActions.join("、")}する推奨です。未送信です。`;
  }
  if (plan.shouldRaiseBackupReserve && reserveLabel) {
    return `売電抑制: 充電開始にはリザーブを${reserveLabel}へ引き上げる推奨です。未送信です。`;
  }
  if (plan.shouldLowerBackupReserve && reserveLabel) {
    return `買電抑制: AC充電を下げ、リザーブをデフォルトの${reserveLabel}へ戻す推奨です。未送信です。`;
  }
  if (plan.recommendedAcChargeLimitW === 0 && plan.shouldAdjustAcChargeLimit) {
    return "買電抑制: AC充電を0Wへ下げる推奨です。未送信です。";
  }
  if (plan.recommendedAcChargeLimitW > 0 && plan.shouldAdjustAcChargeLimit) {
    return `売電抑制: ${plan.recommendedAcChargeLimitW}W充電に回す推奨です。未送信です。`;
  }
  if (plan.recommendedAcChargeLimitW > 0) {
    return `売電抑制: ${plan.recommendedAcChargeLimitW}W充電候補です。未送信です。`;
  }
  return "売電抑制: 現在は追加充電の推奨はありません。";
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

function nullableWatt(value: number | null | undefined) {
  return value === null || value === undefined ? "-" : `${value} W`;
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

function batteryEnergySummary(fullEnergyWh: number | null | undefined, batterySoc: number) {
  if (fullEnergyWh === null || fullEnergyWh === undefined || fullEnergyWh <= 0) {
    return null;
  }
  const remainingWh = (fullEnergyWh * batterySoc) / 100;
  return {
    remainingWh,
    fullEnergyWh
  };
}

function batterySocDescription(
  energy: { remainingWh: number; fullEnergyWh: number } | null,
  batteryOutputW: number,
  backupReserveSoc: number | null | undefined
) {
  const parts: string[] = [];
  if (energy) {
    parts.push(formatRemainingRuntime(energy.remainingWh, batteryOutputW));
  }
  if (backupReserveSoc !== null && backupReserveSoc !== undefined) {
    parts.push(`確保 ${backupReserveSoc}%`);
  }
  return parts.length > 0 ? parts.join(" / ") : "容量未取得";
}

function batteryEnergyLabel(energy: { remainingWh: number; fullEnergyWh: number } | null) {
  if (!energy) {
    return undefined;
  }
  return `約 ${formatDecimal(energy.remainingWh / 1000)} / ${formatDecimal(energy.fullEnergyWh / 1000)} kWh`;
}

function formatRemainingRuntime(remainingWh: number, batteryOutputW: number) {
  if (batteryOutputW <= 0) {
    return "現在出力なし";
  }
  const hours = remainingWh / batteryOutputW;
  if (hours >= 24) {
    return `現在出力なら約 ${formatDecimal(hours / 24)}日`;
  }
  return `現在出力なら約 ${formatDecimal(hours)}時間`;
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

function consumptionSourceLabel(value: string) {
  if (value === "ecoflow-output") {
    return "EcoFlow出力";
  }
  if (value === "manual") {
    return "手動設定";
  }
  if (value === "fallback") {
    return "保守";
  }
  return "-";
}

function modeLabel(value: string) {
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
