import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { CollapsibleSection } from "@/components/CollapsibleSection";
import { decisionReasonLabel, decisionSummaryLabel, guardReasonLabel, strategyStateLabel, writeCandidateLabel } from "@/lib/display-labels";
import type { Delta3Status, DeviceStatus, EnergyStatus, NightChargeDevicePlan, NightChargePlan, SurplusPlan, TariffControlContext } from "@/lib/types";
import type { ReactNode } from "react";

type Metric = {
  label: string;
  value: number | string;
  unit?: string;
  sideValue?: string;
  valueClassName?: string;
  description?: string;
};

type MetricKey = "gridW" | "importW" | "exportW" | "targetChargeW" | "state" | "updatedAt";

const primaryMetrics: MetricKey[] = ["gridW", "importW", "exportW", "targetChargeW", "state", "updatedAt"];
const passthroughToleranceW = 100;
const activePowerThresholdW = 30;

type BadgeVariant = "default" | "secondary" | "success" | "warning" | "destructive";

type DeviceFlowState = {
  label: string;
  variant: BadgeVariant;
  netW: number | null;
  summary: string;
};

type DeviceModeStatus = Pick<
  Delta3Status,
  | "touModeEnabled"
  | "selfPoweredEnabled"
  | "scheduledEnabled"
  | "intelligentEnabled"
  | "backupReserveSoc"
  | "backupReserveEnabled"
>;

export type StatusCardSectionKey = "charts" | "decision" | "surplusPlan" | "nightPlan";

type StatusCardsProps = {
  status: EnergyStatus;
  fetchError: string | null;
};

export function StatusCards({ status, fetchError }: StatusCardsProps) {
  const metrics: Record<MetricKey, Metric> = {
    gridW: { label: "Grid", value: formatGridFlow(status.gridW), valueClassName: gridFlowClassName(status.gridW) },
    importW: { label: "Import", value: status.importW, unit: "W", description: "現在の買電" },
    exportW: { label: "Export", value: status.exportW, unit: "W", description: "現在の売電" },
    targetChargeW: {
      label: "充電推奨",
      value: status.targetChargeW,
      unit: "W",
      description: chargeRecommendationDescription(status.exportW, status.targetChargeW)
    },
    state: {
      label: "制御状態",
      value: status.state || "-",
      description: `mode ${status.mode || "-"}`
    },
    updatedAt: {
      label: "更新時刻",
      value: formatDateTime(status.updatedAt),
      description: "全体サマリー"
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

      <Pro3ACOutputMonitor status={status} />
      <TariffControlCard status={status} />
    </>
  );
}

function TariffControlCard({ status }: { status: EnergyStatus }) {
  const tariff = status.tariffControl;
  const guide = tariffControlGuide(status, tariff);
  if (!tariff) {
    return (
      <Card className="section">
        <CardHeader>
          <div className="panel-title-row">
            <div>
              <CardDescription>Tariff optimized control</CardDescription>
              <CardTitle>料金最適化制御</CardTitle>
            </div>
            <Badge variant="secondary">未取得</Badge>
          </div>
        </CardHeader>
        <CardContent>
          <div className="detail-strip planner-secondary" aria-label="tariff optimized control unavailable">
            <Detail label="料金プラン" value="-" />
            <Detail label="現在電力" value={tariffPowerLabel(status)} />
            <Detail label="制御目安" value={guide.label} />
          </div>
          <p className="planner-reason">{guide.reason}</p>
        </CardContent>
      </Card>
    );
  }
  const priceState = tariff.isLowPrice ? "低単価" : tariff.isHighPrice ? "高単価" : "中間単価";
  const priceVariant: BadgeVariant = tariff.isLowPrice ? "success" : tariff.isHighPrice ? "warning" : "secondary";
  return (
    <Card className="section">
      <CardHeader>
        <div className="panel-title-row">
          <div>
            <CardDescription>Tariff optimized control</CardDescription>
            <CardTitle>料金最適化制御</CardTitle>
          </div>
          <Badge variant={priceVariant}>{priceState}</Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div className="detail-strip planner-secondary" aria-label="tariff optimized control">
          <Detail label="料金プラン" value={tariff.planName || "-"} />
          <Detail label="日種別" value={tariff.dayType === "holiday" ? "休日/祝日" : "平日"} />
          <Detail label="現在区分" value={tariff.currentPeriod || "-"} />
          <Detail label="現在単価" value={formatYenRate(tariff.currentRateYen)} />
          <Detail label="最低単価" value={formatYenRate(tariff.lowestRateYen)} />
          <Detail label="最高単価" value={formatYenRate(tariff.highestRateYen)} />
          <Detail label="次の高単価" value={formatDateTime(tariff.nextHighPriceAt || "")} />
          <Detail label="次の低単価" value={formatDateTime(tariff.nextLowPriceAt || "")} />
          <Detail label="解決元" value={tariff.source === "custom" ? "料金時間帯マスタ" : "既定ルール"} />
          <Detail label="現在電力" value={tariffPowerLabel(status)} />
          <Detail label="制御目安" value={guide.label} />
        </div>
        <p className="planner-reason">{guide.reason}</p>
        {tariff.reason ? <p className="planner-reason">{tariff.reason}</p> : null}
      </CardContent>
    </Card>
  );
}

function Pro3ACOutputMonitor({ status }: { status: EnergyStatus }) {
  const event = status.pro3AcOutputEvent?.eventType ? status.pro3AcOutputEvent : null;
  const outputStateRestoreEnabled = status.pro3AcOutputEvent?.outputPowerOffMemory === true || booleanDiagnostic(status, "outputPowerOffMemory") === true;
  if (!outputStateRestoreEnabled && !event) {
    return null;
  }
  return (
    <Alert variant={event ? "destructive" : "default"} className="section">
      <AlertTitle>DELTA Pro 3 AC出力監視</AlertTitle>
      <div className="ui-alert-description">
        <div className="detail-strip planner-secondary" aria-label="DELTA Pro 3 AC output event">
          <Detail label="AC出力停止シグナル" value={event ? "検知" : "未検知"} />
          <Detail label="出力状態復帰設定" value={outputStateRestoreEnabled ? "有効" : "無効"} />
          <Detail label="確認時刻" value={formatDateTime(event?.measuredAt || status.updatedAt)} />
          <Detail label="SOC" value={nullablePercent(event?.batterySoc ?? status.batterySoc)} />
          <Detail label="AC入力" value={nullableWatt(event?.batteryInputW ?? status.batteryInputW)} />
          <Detail label="AC出力" value={nullableWatt(event?.batteryOutputW ?? status.batteryOutputW)} />
          <Detail label="AC充電上限" value={nullableWatt(event?.acChargeLimitW ?? status.acChargeLimitW)} />
          <Detail label="セル温度" value={nullableTemperature(event?.bmsMaxCellTempC ?? numericDiagnostic(status, "bmsMaxCellTemp"))} />
          <Detail label="MOS温度" value={nullableTemperature(event?.bmsMaxMosTempC ?? numericDiagnostic(status, "bmsMaxMosTemp"))} />
          <Detail label="AC周波数" value={nullableHertz(event?.acOutFreqHz ?? numericDiagnostic(status, "acOutFreq"))} />
          <Detail label="出力上限" value={nullableWatt(event?.acOutDsgPowMaxW ?? numericDiagnostic(status, "plugInInfoAcOutDsgPowMax"))} />
          <Detail label="直前コマンド" value={previousPro3CommandLabel(event)} />
          <Detail label="直前理由" value={event?.previousCommandReason ? decisionReasonLabel(event.previousCommandReason) : "-"} />
        </div>
        {event?.message ? <p className="planner-reason">{event.message}</p> : null}
      </div>
    </Alert>
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
  const sortedDeviceStatuses = [...deviceStatuses].sort((a, b) => a.priority - b.priority || a.id - b.id);
  const availableCount = sortedDeviceStatuses.filter((device) => device.status.available).length;
  const liveAvailableCount = sortedDeviceStatuses.filter((device) => device.status.available && !device.status.cached).length;
  const cachedAvailableCount = sortedDeviceStatuses.filter((device) => device.status.available && device.status.cached).length;
  const available = !fetchError && (availableCount > 0 || Boolean(status?.available));
  const auxiliaryPlan = sourceStatus.delta3AuxPlan;
  const statusSummaryLabel =
    sortedDeviceStatuses.length > 0
      ? cachedAvailableCount > 0
        ? `${liveAvailableCount}/${sortedDeviceStatuses.length} 接続中 / ${cachedAvailableCount} キャッシュ`
        : `${availableCount}/${sortedDeviceStatuses.length} 接続中`
      : available
        ? status?.cached
          ? "cached"
          : "connected"
        : "unavailable";
  const statusSummaryVariant: BadgeVariant = cachedAvailableCount > 0 || status?.cached ? "warning" : available ? "success" : "secondary";
  return (
    <Card className="delta3-status-card section">
      <CardHeader>
        <div className="panel-title-row">
          <div>
            <CardDescription>Read-only device status</CardDescription>
            <CardTitle>充電機器ステータス</CardTitle>
          </div>
          <Badge variant={statusSummaryVariant}>{statusSummaryLabel}</Badge>
        </div>
      </CardHeader>
      <CardContent>
        {sortedDeviceStatuses.length > 0 ? (
          <>
            <div className="delta3-device-status-list">
              {sortedDeviceStatuses.map((device) => {
                const flowState = deviceFlowState(device);
                const connectionBadge = deviceConnectionBadge(device.status);
                return (
                  <div className="delta3-device-status-item" key={device.id || device.credentialRef}>
                    <div className="panel-title-row">
                      <div>
                        <strong>{device.name}</strong>
                        <p className="device-flow-summary">{flowState.summary}</p>
                        <p className="readonly-note">
                          {deviceKindLabel(device.kind)} / 優先 {device.priority} / {device.credentialRef} / {statusSourceLabel(device.statusSource)} / {maskDeviceSn(device.deviceSn)}
                        </p>
                      </div>
                      <div className="device-status-badges">
                        <Badge variant={flowState.variant}>{flowState.label}</Badge>
                        <Badge variant={connectionBadge.variant}>{connectionBadge.label}</Badge>
                      </div>
                    </div>
                    {device.status.available ? (
                      <>
                        <div className="detail-strip" aria-label={`${device.name} read-only status`}>
                          <Detail label="状態" value={<Badge variant={flowState.variant}>{flowState.label}</Badge>} />
                          <Detail label="取得状態" value={<Badge variant={connectionBadge.variant}>{connectionBadge.label}</Badge>} />
                          <Detail label="残量" value={deviceSocLabel(device)} />
                          <Detail label="AC入力" value={nullableWatt(device.status.acInW)} />
                          <Detail label="AC出力" value={nullablePositiveWatt(device.status.acOutW)} />
                          <Detail label="実質W" value={formatDeviceNetFlow(flowState.netW)} />
                          <Detail label="AC充電上限" value={nullableWatt(device.status.acChargeLimitW)} />
                          <Detail label="サイクル数" value={cycleCountLabel(device.status)} />
                          <Detail label="サイクル候補" value={cycleCountCandidateLabel(device.status)} />
                          <Detail label="候補一覧" value={cycleCountCandidatesLabel(device.status)} />
                          <Detail label="容量" value={formatWhCapacity(device.capacityWh)} />
                          <Detail label="設定範囲" value={`${device.minChargeW}-${device.maxChargeW} W`} />
                        </div>
                        <div className="detail-strip planner-secondary" aria-label={`${device.name} configuration status`}>
                          <Detail label="運転モード" value={operationModeLabel(device.status)} />
                          <Detail label="リザーブ関連モード" value={reserveRelatedModeLabel(device.status)} />
                          <Detail label="最大充電残量" value={nullablePercent(device.status.maxChargeSoc)} />
                          <Detail label="最低放電残量" value={nullablePercent(device.status.minDischargeSoc)} />
                          <Detail label="リザーブ制御範囲" value={formatReserveRange(device.backupReserveMinSoc, device.backupReserveMaxSoc)} />
                          <Detail label="バックアップリザーブ残量" value={nullablePercent(device.status.backupReserveSoc)} />
                          <Detail label="Energy Backup" value={nullableOnOff(device.status.backupReserveEnabled)} />
                          <Detail label="グリッドバイパス無効化" value={nullableOnOff(device.status.gridBypassDisabled)} />
                          <Detail label="AC出力" value={nullableOnOff(device.status.acOutputEnabled)} />
                          {hasSplitACOutput(device.status) ? (
                            <>
                              <Detail label="AC1" value={nullableOnOff(device.status.acOutput1Enabled)} />
                              <Detail label="AC2" value={nullableOnOff(device.status.acOutput2Enabled)} />
                              <Detail label="AC出力保護" value={acOutputProtectionChannelLabel(device.status.acOutputProtectionChannel)} />
                            </>
                          ) : null}
                          <Detail label="Device type" value={device.status.deviceType || device.deviceType || "-"} />
                          <Detail label="Updated" value={formatDateTime(device.status.updatedAt || "")} />
                          {device.status.cached ? <Detail label="更新エラー" value={device.status.lastError || "-"} /> : null}
                          <Detail label="制御候補" value={device.controlEnabled ? "有効" : "無効"} />
                          <Detail label="AC出力OFF復旧" value={device.autoRecoverAcOutput ? "許可" : "禁止"} />
                        </div>
                      </>
                    ) : (
                      <Alert className="delta3-alert">
                        <AlertTitle>{device.name} read-only status</AlertTitle>
                        <div className="ui-alert-description">
                          <p>{device.status.lastError || "read-only status is not loaded"}</p>
                          <TelemetryDiagnostics status={device.status} />
                        </div>
                      </Alert>
                    )}
                  </div>
                );
              })}
            </div>
            <div className="detail-strip planner-secondary" aria-label="auxiliary battery plan">
              <Detail label="対象機器" value={auxiliaryTargetLabel(auxiliaryPlan)} />
	              <Detail label="機器別計画" value={auxiliaryDevicePlansLabel(auxiliaryPlan)} />
              <Detail label="補助計画" value={strategyStateLabel(auxiliaryPlan?.strategyState || "UNAVAILABLE")} />
              <Detail label="推奨AC上限" value={nullableWatt(auxiliaryPlan?.recommendedAcChargeLimitW)} />
              <Detail label="現在AC上限" value={nullableWatt(auxiliaryPlan?.currentAcChargeLimitW)} />
              <Detail label="AC出力考慮上限" value={nullableWatt(auxiliaryPlan?.safeAcChargeLimitW)} />
              <Detail label="補助機器出力" value={nullableWatt(auxiliaryPlan?.delta3AcOutputW)} />
              <Detail label="補助機器AC出力" value={nullableOnOff(auxiliaryPlan?.delta3AcOutputEnabled)} />
              <Detail label="推奨リザーブ残量" value={nullablePercent(auxiliaryPlan?.recommendedBackupReserveSoc)} />
              <Detail label="現在リザーブ残量" value={nullablePercent(auxiliaryPlan?.currentBackupReserveSoc)} />
              <Detail label="リザーブ反映" value={backupReserveApplyLabel(auxiliaryPlan)} />
              <Detail label="残余売電" value={nullableWatt(auxiliaryPlan?.residualExportW ?? sourceStatus.exportW)} />
              <Detail label="安全余力" value={nullableWatt(auxiliaryPlan?.safetyMarginW)} />
              <Detail label="リザーブ解除" value={writeCandidateLabel(auxiliaryPlan?.shouldDisableBackupReserve)} />
              <Detail label="実行" value={writeCandidateLabel(auxiliaryPlan?.wouldWrite)} />
              <Detail label="抑制" value={guardReasonLabel(auxiliaryPlan?.suppressedReason)} />
              <Detail label="理由" value={decisionReasonLabel(auxiliaryPlan?.reason)} />
            </div>
          </>
        ) : available && status ? (
          <>
            <div className="detail-strip" aria-label="DELTA 3 Plus read-only status">
              <Detail label="残量" value={nullablePercent(status.soc)} />
              <Detail label="AC入力" value={nullableWatt(status.acInW)} />
              <Detail label="AC出力" value={nullablePositiveWatt(status.acOutW)} />
              <Detail label="AC充電上限" value={nullableWatt(status.acChargeLimitW)} />
              <Detail label="サイクル数" value={cycleCountLabel(status)} />
              <Detail label="サイクル候補" value={cycleCountCandidateLabel(status)} />
              <Detail label="候補一覧" value={cycleCountCandidatesLabel(status)} />
              <Detail label="グリッドバイパス無効化" value={nullableOnOff(status.gridBypassDisabled)} />
              <Detail label="AC output" value={nullableOnOff(status.acOutputEnabled)} />
              {hasSplitACOutput(status) ? (
                <>
                  <Detail label="AC1" value={nullableOnOff(status.acOutput1Enabled)} />
                  <Detail label="AC2" value={nullableOnOff(status.acOutput2Enabled)} />
                  <Detail label="AC出力保護" value={acOutputProtectionChannelLabel(status.acOutputProtectionChannel)} />
                </>
              ) : null}
            </div>
            <div className="detail-strip planner-secondary" aria-label="DELTA 3 Plus configuration status">
              <Detail label="運転モード" value={operationModeLabel(status)} />
              <Detail label="リザーブ関連モード" value={reserveRelatedModeLabel(status)} />
              <Detail label="最大充電残量" value={nullablePercent(status.maxChargeSoc)} />
              <Detail label="最低放電残量" value={nullablePercent(status.minDischargeSoc)} />
              <Detail label="バックアップリザーブ残量" value={nullablePercent(status.backupReserveSoc)} />
              <Detail label="Energy Backup" value={nullableOnOff(status.backupReserveEnabled)} />
              <Detail label="Device type" value={status.deviceType || "-"} />
              <Detail label="Updated" value={formatDateTime(status.updatedAt || "")} />
            </div>
            <div className="detail-strip planner-secondary" aria-label="auxiliary battery plan">
              <Detail label="対象機器" value={auxiliaryTargetLabel(auxiliaryPlan)} />
	              <Detail label="機器別計画" value={auxiliaryDevicePlansLabel(auxiliaryPlan)} />
              <Detail label="補助計画" value={strategyStateLabel(auxiliaryPlan?.strategyState || "UNAVAILABLE")} />
              <Detail label="推奨AC上限" value={nullableWatt(auxiliaryPlan?.recommendedAcChargeLimitW)} />
              <Detail label="現在AC上限" value={nullableWatt(auxiliaryPlan?.currentAcChargeLimitW)} />
              <Detail label="AC出力考慮上限" value={nullableWatt(auxiliaryPlan?.safeAcChargeLimitW)} />
              <Detail label="補助機器出力" value={nullableWatt(auxiliaryPlan?.delta3AcOutputW)} />
              <Detail label="補助機器AC出力" value={nullableOnOff(auxiliaryPlan?.delta3AcOutputEnabled)} />
              <Detail label="推奨リザーブ残量" value={nullablePercent(auxiliaryPlan?.recommendedBackupReserveSoc)} />
              <Detail label="現在リザーブ残量" value={nullablePercent(auxiliaryPlan?.currentBackupReserveSoc)} />
              <Detail label="リザーブ反映" value={backupReserveApplyLabel(auxiliaryPlan)} />
              <Detail label="残余売電" value={nullableWatt(auxiliaryPlan?.residualExportW ?? sourceStatus.exportW)} />
              <Detail label="安全余力" value={nullableWatt(auxiliaryPlan?.safetyMarginW)} />
              <Detail label="リザーブ解除" value={writeCandidateLabel(auxiliaryPlan?.shouldDisableBackupReserve)} />
              <Detail label="実行" value={writeCandidateLabel(auxiliaryPlan?.wouldWrite)} />
              <Detail label="抑制" value={guardReasonLabel(auxiliaryPlan?.suppressedReason)} />
              <Detail label="理由" value={decisionReasonLabel(auxiliaryPlan?.reason)} />
            </div>
          </>
        ) : (
          <Alert className="delta3-alert">
            <AlertTitle>DELTA 3 Plus read-only status</AlertTitle>
            <div className="ui-alert-description">
              <p>{unavailableReason}</p>
              {status ? <TelemetryDiagnostics status={status} /> : null}
            </div>
          </Alert>
        )}
      </CardContent>
    </Card>
  );
}

function TelemetryDiagnostics({ status }: { status: Delta3Status }) {
  const diagnostics = status.telemetryDiagnostics;
  if (!diagnostics) {
    return null;
  }
  const fields = diagnostics.fieldSummaries?.slice(0, 8) ?? [];
  return (
    <div className="telemetry-diagnostics" aria-label="private MQTT telemetry diagnostics">
      <p>
        telemetry: replies {diagnostics.replyCount} / decoded {diagnostics.decodedMessages} / unsupported {diagnostics.unsupportedMessages} / fields{" "}
        {diagnostics.fieldCount}
        {diagnostics.inspectErrorCount ? ` / inspect errors ${diagnostics.inspectErrorCount}` : ""}
        {diagnostics.fieldSummaryTruncated ? " / truncated" : ""}
      </p>
      {diagnostics.lastInspectError ? <p>inspect: {diagnostics.lastInspectError}</p> : null}
      {fields.length > 0 ? (
        <div className="telemetry-field-list">
          {fields.map((field, index) => (
            <span key={`${field.messageIndex}-${field.cmdFunc}-${field.cmdId}-${field.field}-${index}`}>
              {`m${field.messageIndex} ${field.cmdFunc}/${field.cmdId} f${field.field} w${field.wire}=${field.value}`}
            </span>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function deviceKindLabel(value: string) {
  const labels: Record<string, string> = {
    ecoflow_delta_pro3: "DELTA Pro 3",
    ecoflow_delta3_plus: "DELTA 3 Plus",
    switchbot_plug: "SwitchBot plug",
    manual: "手動機器"
  };
  return labels[value] || value || "未設定";
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

function deviceSocLabel(device: DeviceStatus) {
  const percent = nullablePercent(device.status.soc);
  const soc = device.status.soc;
  if (soc === null || soc === undefined || device.capacityWh <= 0) {
    return percent;
  }
  const currentKwh = (device.capacityWh * soc) / 100 / 1000;
  return `${percent} / 約 ${formatDecimal(currentKwh)} kWh`;
}

function formatWhCapacity(value: number) {
  if (!Number.isFinite(value) || value <= 0) {
    return "-";
  }
  return `${formatDecimal(value / 1000)} kWh`;
}

function cycleCountLabel(status: Delta3Status) {
  if (status.cycleCount === null || status.cycleCount === undefined) {
    return "-";
  }
  const source = cycleCountSourceLabel(status.cycleCountSource || "");
  return source ? `${status.cycleCount} 回 / ${source}` : `${status.cycleCount} 回`;
}

function cycleCountCandidateLabel(status: Delta3Status) {
	const candidate = status.cycleCountCandidate;
	if (!candidate) {
		return "-";
	}
	const source = cycleCountSourceLabel(candidate.source);
	return `${candidate.value} 回候補 / ${source} ${candidate.cmdFunc}/${candidate.cmdId}/${candidate.field}`;
}

function cycleCountCandidatesLabel(status: Delta3Status) {
	const candidates = status.cycleCountCandidates?.length
		? status.cycleCountCandidates
		: status.cycleCountCandidate
			? [status.cycleCountCandidate]
			: [];
	if (candidates.length === 0) {
		return "-";
	}
	const source = cycleCountSourceLabel(candidates[0].source);
	const values = candidates.map((candidate) => `${candidate.value}回 ${candidate.cmdFunc}/${candidate.cmdId}/${candidate.field}`);
	return source ? `${source}: ${values.join(" / ")}` : values.join(" / ");
}

function cycleCountSourceLabel(value: string) {
  const labels: Record<string, string> = {
    ecoflow_cloud_quota: "Cloud quota",
    ecoflow_developer_mqtt_quota: "Developer MQTT quota",
    ecoflow_private_mqtt_candidate: "private MQTT"
  };
  return labels[value] || value;
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
          <Detail label="運転モード" value={operationModeLabel(status)} />
          <Detail label="リザーブ関連モード" value={reserveRelatedModeLabel(status)} />
          <Detail label="バックアップリザーブ残量" value={nullablePercent(status.backupReserveSoc)} />
          <Detail label="Energy Backup" value={nullableOnOff(status.energyBackupEnabled)} />
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
            <Detail label="推奨深夜残量" value={`${plan.recommendedNightTargetSoc}%`} />
            <Detail label="最低確保残量" value={`${plan.minimumReserveSoc}%`} />
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
            <Detail label="補正後PV" value={`${formatDecimal(plan.correctedEstimatedPvKwh || plan.estimatedPvKwh)} kWh`} />
            <Detail label="補正後PV充電" value={`${formatDecimal(plan.correctedEstimatedPvToBatteryKwh || plan.estimatedPvToBatteryKwh)} kWh`} />
            <Detail label="PV充電補正" value={`${formatDecimal(plan.pvChargeCorrectionFactor || 0)} / ${pvCorrectionSourceLabel(plan.pvChargeCorrectionSource)}`} />
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
          <div className="detail-strip planner-secondary" aria-label="night charge device totals">
            <Detail label="機器合計容量" value={`${formatDecimal(plan.totalDeviceCapacityKwh || 0)} kWh`} />
            <Detail label="機器現在残量" value={`${formatDecimal(plan.totalCurrentDeviceEnergyKwh || 0)} kWh`} />
            <Detail label="機器推奨残量" value={`${formatDecimal(plan.totalRecommendedTargetKwh || 0)} kWh`} />
            <Detail label="機器必要充電" value={`${formatDecimal(plan.totalRequiredDeviceChargeKwh || 0)} kWh`} />
            <Detail label="日中総必要量" value={`${formatDecimal(plan.totalDaytimeRequiredKwh || 0)} kWh`} />
            <Detail label="全機器利用可能" value={`${formatDecimal(plan.totalAvailableKwh || 0)} kWh`} />
            <Detail label="全機器不足量" value={`${formatDecimal(plan.totalDeficitKwh || 0)} kWh`} />
          </div>
          {plan.devicePlans?.length ? (
            <div className="delta3-device-status-list" aria-label="night charge device plans">
              {plan.devicePlans.map((devicePlan) => (
                <NightChargeDevicePlanItem key={devicePlan.deviceId || `${devicePlan.name}-${devicePlan.priority}`} devicePlan={devicePlan} />
              ))}
            </div>
          ) : null}
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

function NightChargeDevicePlanItem({ devicePlan }: { devicePlan: NightChargeDevicePlan }) {
  return (
    <div className="delta3-device-status-item">
      <div className="panel-title-row">
        <div>
          <strong>{devicePlan.name || devicePlan.kind || `機器 ${devicePlan.deviceId}`}</strong>
          <p className="device-flow-summary">{nightChargeDevicePlanSummary(devicePlan)}</p>
        </div>
        <div className="device-status-badges">
          <Badge variant={devicePlan.shouldCharge ? "warning" : "secondary"}>{devicePlan.shouldCharge ? "充電候補" : "充電不要"}</Badge>
          <Badge variant={devicePlan.wouldWrite ? "warning" : "secondary"}>{writeCandidateLabel(devicePlan.wouldWrite)}</Badge>
        </div>
      </div>
      <div className="detail-strip planner-secondary" aria-label={`${devicePlan.name} night charge device plan`}>
        <Detail label="優先" value={devicePlan.priority} />
        <Detail label="現在残量" value={nullablePercent(devicePlan.currentSoc)} />
        <Detail label="現在kWh" value={`${formatDecimal(devicePlan.currentEnergyKwh)} kWh`} />
        <Detail label="日中必要" value={`${formatDecimal(devicePlan.daytimeRequiredKwh || 0)} kWh`} />
        <Detail label="利用可能" value={`${formatDecimal(devicePlan.availableKwh || 0)} kWh`} />
        <Detail label="PV割当" value={`${formatDecimal(devicePlan.pvAllocatedKwh || 0)} kWh`} />
        <Detail label="7時-PV開始" value={`${formatDecimal(devicePlan.morningPrePvRequiredKwh || 0)} kWh`} />
        <Detail label="推奨目標" value={`${devicePlan.recommendedTargetSoc}%`} />
        <Detail label="推奨kWh" value={`${formatDecimal(devicePlan.recommendedTargetKwh)} kWh`} />
        <Detail label="必要充電" value={`${formatDecimal(devicePlan.requiredChargeKwh)} kWh`} />
        <Detail label="リザーブ制御範囲" value={formatReserveRange(devicePlan.minTargetSoc, devicePlan.maxTargetSoc)} />
        <Detail label="候補AC上限" value={devicePlan.recommendedAcChargeLimitW > 0 ? `${devicePlan.recommendedAcChargeLimitW} W` : "-"} />
        <Detail label="Device type" value={devicePlan.deviceType || "-"} />
        <Detail label="書込対象" value={devicePlan.writeTarget ? "対象" : "対象外"} />
        <Detail label="制御候補" value={devicePlan.controlEnabled ? "有効" : "無効"} />
        <Detail label="取得元" value={statusSourceLabel(devicePlan.dataSource)} />
      </div>
      {devicePlan.reason ? <p className="planner-reason">{decisionSummaryLabel(devicePlan.reason)}</p> : null}
      {devicePlan.blockReason ? <p className="planner-reason">抑制: {nightChargeDeviceBlockReasonLabel(devicePlan.blockReason)}</p> : null}
    </div>
  );
}

function logRangeSummary(status: EnergyStatus) {
  return status.updatedAt ? `更新 ${formatDateTime(status.updatedAt)}` : "更新待ち";
}

function surplusPlanSummary(plan: SurplusPlan) {
  return `${strategyStateLabel(plan.strategyState)} / ${surplusActionLabel(plan)}`;
}

function nightPlanSummary(plan: NightChargePlan) {
  return `${strategyStateLabel(plan.strategyState)} / 推奨深夜残量 ${plan.recommendedNightTargetSoc}% / PV ${formatDecimal(plan.dailyEstimatedPvKwh || plan.estimatedPvKwh)} kWh`;
}

function nightChargeDevicePlanSummary(devicePlan: NightChargeDevicePlan) {
  if (devicePlan.blockReason) {
    return `抑制: ${nightChargeDeviceBlockReasonLabel(devicePlan.blockReason)}`;
  }
  if (devicePlan.shouldCharge) {
    return `目標 ${devicePlan.recommendedTargetSoc}% / 必要 ${formatDecimal(devicePlan.requiredChargeKwh)} kWh`;
  }
  return `目標 ${devicePlan.recommendedTargetSoc}% / 充電不要`;
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

function tariffPowerLabel(status: EnergyStatus) {
  if (status.exportW > 0) {
    return `売電 ${status.exportW} W`;
  }
  if (status.importW > 0) {
    return `買電 ${status.importW} W`;
  }
  return "買電/売電 0 W";
}

function tariffControlGuide(status: EnergyStatus, tariff?: TariffControlContext | null) {
  if (!tariff) {
    return {
      label: "料金判定未取得",
      reason: "料金判定がステータスに含まれていないため、料金時間帯マスタに基づく制御目安は未確定です。既存の制御判断ログを優先します。"
    };
  }
  if (status.exportW > 0 && status.targetChargeW > 0) {
    return {
      label: "余剰充電候補",
      reason: `売電 ${status.exportW} W、充電推奨 ${status.targetChargeW} W。余剰吸収のため充電を優先する目安です。`
    };
  }
  if (status.exportW > 0) {
    return {
      label: "余剰あり",
      reason: `売電 ${status.exportW} W。最低開始Wや安全余白に届かない場合は、既存制御ゲートで充電を抑制します。`
    };
  }
  if (tariff.isHighPrice && status.importW > 0) {
    return {
      label: "放電優先/充電抑制",
      reason: `高単価時間帯に買電 ${status.importW} W。十分な残量があれば放電優先、充電は抑制する目安です。`
    };
  }
  if (tariff.isHighPrice) {
    return {
      label: "高単価待機",
      reason: "高単価時間帯です。売電や買電が小さい場合は、既存制御ログの判断を優先します。"
    };
  }
  if (tariff.isLowPrice && status.importW > 0) {
    return {
      label: "不足分充電候補",
      reason: `低単価時間帯に買電 ${status.importW} W。翌日日中不足やSOC下限を見て、必要分だけ充電する候補です。`
    };
  }
  if (tariff.isLowPrice) {
    return {
      label: "低単価充電候補",
      reason: "低単価時間帯です。深夜充電計画と各機器の制御範囲に従って必要分だけ充電する候補です。"
    };
  }
  const tariffDecisionReason =
    status.nightChargePlan?.tariffControlReason || status.surplusPlan?.tariffControlReason || "";
  if (status.importW > 0 && tariffDecisionReason.includes("preserve battery")) {
    return {
      label: "高単価まで温存",
      reason: tariffDecisionReason
    };
  }
  if (status.importW > 0 && tariffDecisionReason.includes("prioritize battery discharge")) {
    return {
      label: "放電優先",
      reason: tariffDecisionReason
    };
  }
  if (status.importW > 0 && tariffDecisionReason.includes("prefer battery discharge")) {
    return {
      label: "放電優先",
      reason: tariffDecisionReason
    };
  }
  if (status.importW > 0) {
    return {
      label: "料金境界待ち",
      reason: tariffDecisionReason || "次の高単価・低単価境界を確認できないため、能動放電を開始しません。"
    };
  }
  return {
    label: "現行ルール維持",
    reason: tariffDecisionReason || "中間単価時間帯です。売電/買電、SOC、既存の最小送信間隔と安全ゲートを優先します。"
  };
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
    surplusActions.push(`リザーブを現在残量の${reserveLabel}へ合わせる`);
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

function hasSplitACOutput(status: Delta3Status) {
  return status.acOutput1Enabled !== null && status.acOutput1Enabled !== undefined ||
    status.acOutput2Enabled !== null && status.acOutput2Enabled !== undefined ||
    status.acOutputProtectionChannel !== null && status.acOutputProtectionChannel !== undefined;
}

function acOutputProtectionChannelLabel(value: number | null | undefined) {
  if (value === 1) {
    return "AC1";
  }
  if (value === 2) {
    return "AC2";
  }
  return "-";
}

function backupReserveApplyLabel(plan: EnergyStatus["delta3AuxPlan"] | null | undefined) {
  if (!plan?.backupReserveApplyState) {
    return "-";
  }
  const stateLabels: Record<string, string> = {
    pending: "反映待ち",
    failed: "反映失敗",
    applied: "反映済み",
    stale: "古い履歴",
    ignored: "反映不要"
  };
  const state = stateLabels[plan.backupReserveApplyState] ?? plan.backupReserveApplyState;
  const target = plan.lastBackupReserveTargetSoc === null || plan.lastBackupReserveTargetSoc === undefined ? "" : ` / 目標 ${plan.lastBackupReserveTargetSoc}%`;
  return `${state}${target}`;
}

function auxiliaryTargetLabel(plan: EnergyStatus["delta3AuxPlan"] | null | undefined) {
  if (!plan) {
    return "-";
  }
  if (plan.deviceName) {
    return plan.deviceName;
  }
  if (plan.deviceType) {
    return plan.deviceType;
  }
  return plan.deviceId ? `#${plan.deviceId}` : "-";
}

function auxiliaryDevicePlansLabel(plan: EnergyStatus["delta3AuxPlan"] | null | undefined) {
	const plans = plan?.devicePlans;
	if (!plans?.length) return auxiliaryTargetLabel(plan);
	return plans.map((item) => `${auxiliaryTargetLabel(item)}: ${strategyStateLabel(item.strategyState)}`).join(" / ");
}

function nullableWatt(value: number | null | undefined) {
  return value === null || value === undefined ? "-" : `${value} W`;
}

function nullableTemperature(value: number | null | undefined) {
  return value === null || value === undefined ? "-" : `${formatDecimal(value)} ℃`;
}

function nullableHertz(value: number | null | undefined) {
  return value === null || value === undefined ? "-" : `${formatDecimal(value)} Hz`;
}

function numericDiagnostic(status: EnergyStatus, key: string) {
  const value = status.ecoflowDiagnostics?.[key];
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string") {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : null;
  }
  return null;
}

function booleanDiagnostic(status: EnergyStatus, key: string) {
  const value = status.ecoflowDiagnostics?.[key];
  if (typeof value === "boolean") {
    return value;
  }
  if (typeof value === "number" && Number.isFinite(value)) {
    return value !== 0;
  }
  if (typeof value === "string") {
    const normalized = value.trim().toLowerCase();
    if (normalized === "true" || normalized === "1") {
      return true;
    }
    if (normalized === "false" || normalized === "0") {
      return false;
    }
  }
  return null;
}

function previousPro3CommandLabel(event: EnergyStatus["pro3AcOutputEvent"] | null | undefined) {
  if (!event?.previousCommandKind) {
    return "なし";
  }
  const sent = event.previousCommandSent ? "送信あり" : "未送信";
  const targetAc = event.previousCommandTargetAcChargeW === null || event.previousCommandTargetAcChargeW === undefined ? "" : ` / AC ${event.previousCommandTargetAcChargeW}W`;
  const targetReserve =
    event.previousCommandTargetReserveSoc === null || event.previousCommandTargetReserveSoc === undefined ? "" : ` / リザーブ ${event.previousCommandTargetReserveSoc}%`;
  return `${event.previousCommandKind} / ${sent}${targetAc}${targetReserve}`;
}

function nullablePositiveWatt(value: number | null | undefined) {
  return value === null || value === undefined ? "-" : `${Math.abs(value)} W`;
}

function deviceConnectionBadge(status: Delta3Status): { label: string; variant: BadgeVariant } {
  if (status.cached) {
    return { label: "キャッシュ", variant: "warning" };
  }
  if (status.available) {
    return { label: "接続中", variant: "success" };
  }
  return { label: "取得不可", variant: "secondary" };
}

function deviceFlowState(device: DeviceStatus): DeviceFlowState {
  if (!device.status.available) {
    return {
      label: "取得不可",
      variant: "destructive",
      netW: null,
      summary: device.status.lastError || "ステータスを取得できていません"
    };
  }
  if (device.status.acInW === null || device.status.acInW === undefined || device.status.acOutW === null || device.status.acOutW === undefined) {
    return {
      label: "取得不可",
      variant: "destructive",
      netW: null,
      summary: "AC入力またはAC出力の値を取得できていません"
    };
  }

  const inputW = device.status.acInW;
  const outputW = Math.abs(device.status.acOutW);
  const netW = inputW - outputW;
  const powerSummary = `AC入力 ${inputW} W / AC出力 ${outputW} W / ${formatDeviceNetFlow(netW)}`;

  if (inputW >= activePowerThresholdW && outputW >= activePowerThresholdW && Math.abs(netW) <= passthroughToleranceW) {
    return { label: "パススルー", variant: "secondary", netW, summary: powerSummary };
  }
  if (netW > passthroughToleranceW) {
    return { label: "充電中", variant: "success", netW, summary: powerSummary };
  }
  if (netW < -passthroughToleranceW) {
    return { label: "放電中", variant: "warning", netW, summary: powerSummary };
  }
  return { label: "待機", variant: "secondary", netW, summary: powerSummary };
}

function formatDeviceNetFlow(value: number | null) {
  if (value === null) {
    return "-";
  }
  if (value > 0) {
    return `実質充電 ${value} W`;
  }
  if (value < 0) {
    return `実質放電 ${Math.abs(value)} W`;
  }
  return "差分 0 W";
}

function operationModeLabel(status: DeviceModeStatus) {
  if (status.touModeEnabled === true) {
    return "TOU";
  }
  if (status.selfPoweredEnabled === true) {
    return "セルフパワー";
  }
  if (status.scheduledEnabled === true) {
    return "スケジュール";
  }
  if (status.intelligentEnabled === true) {
    return "インテリジェント";
  }
  if (
    status.touModeEnabled === false ||
    status.selfPoweredEnabled === false ||
    status.scheduledEnabled === false ||
    status.intelligentEnabled === false
  ) {
    return "その他";
  }
  return "-";
}

function reserveRelatedModeLabel(status: DeviceModeStatus) {
  if (status.backupReserveSoc === null || status.backupReserveSoc === undefined) {
    return "-";
  }
  if (status.touModeEnabled === true) {
    return "TOU設定";
  }
  if (status.selfPoweredEnabled === true) {
    return "セルフパワー設定";
  }
  if (status.touModeEnabled === false || status.selfPoweredEnabled === false) {
    return "該当なし";
  }
  return "未確認";
}

function formatReserveRange(minSoc: number | null | undefined, maxSoc: number | null | undefined) {
  if (minSoc === null || minSoc === undefined || maxSoc === null || maxSoc === undefined) {
    return "-";
  }
  return `${minSoc}-${maxSoc}%`;
}

function nightChargeDeviceBlockReasonLabel(value: string) {
  if (value === "device capacity is unavailable") {
    return "機器容量を取得できません";
  }
  if (value === "device SOC is unavailable") {
    return "機器残量を取得できません";
  }
  if (value === "device status is unavailable") {
    return "機器ステータスを取得できません";
  }
  if (value === "device control is disabled") {
    return "機器マスタで制御対象外です";
  }
  if (value === "device AC charge control is unavailable") {
    return "AC充電上限の制御に未対応です";
  }
  if (value === "DELTA 3 series device is not the write target") {
    return "DELTA 3 系の書き込み対象ではありません";
  }
  if (value === "DELTA Pro 3 is not the write target") {
    return "DELTA Pro 3 の書き込み対象ではありません";
  }
  if (value === "outside night charge window") {
    return "深夜充電時間外です";
  }
  if (value === "mock mode keeps device write disabled") {
    return "mock mode のため機器への書き込みは無効です";
  }
  if (value === "simulation mode keeps device write disabled") {
    return "simulation mode のため機器への書き込みは無効です";
  }
  if (value === "ENABLE_REAL_CONTROL=false keeps device write disabled") {
    return "ENABLE_REAL_CONTROL=false のため機器への書き込みは無効です";
  }
  if (value === "auto control disabled keeps device write disabled") {
    return "自動制御OFFのため機器への書き込みは無効です";
  }
  if (value === "CONFIRM_ECOFLOW_WRITE is not I_UNDERSTAND") {
    return "実機書き込み確認が未設定です";
  }
  if (value === "real control trial window inactive") {
    return "実制御の有効期限外です";
  }
  return value || "-";
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

function pvCorrectionSourceLabel(value: string) {
  if (value === "manual") {
    return "手動";
  }
  if (value === "default") {
    return "既定";
  }
  if (value === "recommended") {
    return "推奨";
  }
  return value || "-";
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

function formatYenRate(value: number | null | undefined) {
  if (value === null || value === undefined || Number.isNaN(value)) {
    return "-";
  }
  return `${value.toFixed(2)} 円/kWh`;
}
