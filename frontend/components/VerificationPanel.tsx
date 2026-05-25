"use client";

import type { ReactNode } from "react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { decisionReasonLabel, guardReasonLabel, strategyStateLabel, writeCandidateLabel } from "@/lib/display-labels";
import type { Delta3AuxControlCommandLog, Delta3Status, EnergyStatus, NightChargeDailySummary } from "@/lib/types";

type VerificationPanelProps = {
  latestNightSummary: NightChargeDailySummary | null;
  latestNightSummaryError: string | null;
  status: EnergyStatus;
  delta3Status: Delta3Status | null;
  delta3StatusError: string | null;
  recentDelta3AuxLogs: Delta3AuxControlCommandLog[];
  recentDelta3AuxError: string | null;
};

export function VerificationPanel({
  latestNightSummary,
  latestNightSummaryError,
  status,
  delta3Status,
  delta3StatusError,
  recentDelta3AuxLogs,
  recentDelta3AuxError
}: VerificationPanelProps) {
  const aggregate = aggregateDelta3AuxLogs(recentDelta3AuxLogs);
  const auxPlan = status.delta3AuxPlan;
  const latestAuxLog = recentDelta3AuxLogs[0] ?? null;
  const delta3Available = Boolean(delta3Status?.available) && !delta3StatusError;
  const currentDelta3Reason = delta3StatusError || delta3Status?.lastError || auxPlan?.suppressedReason || "";
  const delta3Reason = currentDelta3Reason || (auxPlan ? "" : latestAuxLog?.suppressedReason || "");

  return (
    <div className="verification-panel">
      {latestNightSummaryError ? (
        <Alert variant="destructive">
          <AlertTitle>夜間充電実績の取得に失敗しました</AlertTitle>
          <AlertDescription>{latestNightSummaryError}</AlertDescription>
        </Alert>
      ) : null}
      {recentDelta3AuxError ? (
        <Alert variant="destructive">
          <AlertTitle>DELTA 3 Plus補助ログの取得に失敗しました</AlertTitle>
          <AlertDescription>{recentDelta3AuxError}</AlertDescription>
        </Alert>
      ) : null}
      <Card className="verification-card">
        <CardHeader>
          <div className="panel-title-row">
            <div>
              <CardDescription>最新夜間サマリー</CardDescription>
              <CardTitle>夜間充電 実績検証</CardTitle>
            </div>
            <Badge variant={nightSummaryBadgeVariant(latestNightSummary?.finalResultStatus)}>{statusLabel(latestNightSummary?.finalResultStatus)}</Badge>
          </div>
        </CardHeader>
        <CardContent>
          {latestNightSummary ? (
            <>
              <div className="detail-strip" aria-label="night charge verification">
                <Detail label="夜間日" value={latestNightSummary.summaryDate} />
                <Detail label="目標SOC" value={nullablePercent(latestNightSummary.plannedTargetSoc)} />
                <Detail label="7:00 SOC" value={nullablePercent(latestNightSummary.nightEndSoc)} />
                <Detail label="目標差" value={formatSocGap(latestNightSummary.morningTargetSocGap)} />
                <Detail label="夜間実質充電" value={nullableKwh(latestNightSummary.nightNetBatteryKwh)} />
                <Detail label="必要充電との差" value={formatKwhGap(latestNightSummary.nightRequiredChargeGapKwh)} />
              </div>
              <div className="detail-strip planner-secondary" aria-label="night charge follow-up verification">
                <Detail label="日中充電+売電" value={nullableKwh(latestNightSummary.daytimeChargeAndExportKwh)} />
                <Detail label="日中Battery in" value={nullableKwh(latestNightSummary.daytimeBatteryInputKwh)} />
                <Detail label="日中売電" value={nullableKwh(latestNightSummary.daytimeExportKwh)} />
                <Detail label="07:00判定" value={<Badge variant={nightSummaryBadgeVariant(latestNightSummary.morningStatus)}>{statusLabel(latestNightSummary.morningStatus)}</Badge>} />
                <Detail label="16:00判定" value={<Badge variant={nightSummaryBadgeVariant(latestNightSummary.finalResultStatus)}>{statusLabel(latestNightSummary.finalResultStatus)}</Badge>} />
                <Detail label="Data source" value={latestNightSummary.dataSource || "-"} />
              </div>
              <p className="planner-reason">{latestNightSummary.morningReason || "-"}</p>
              <p className="planner-reason">{latestNightSummary.finalResultReason || "-"}</p>
              <p className="readonly-note">Battery inputは買電由来の充電も混ざり得るため、PV由来とは断定しません。</p>
            </>
          ) : (
            <p className="readonly-note">夜間充電サマリーはまだありません。</p>
          )}
        </CardContent>
      </Card>
      <Card className="verification-card">
        <CardHeader>
          <div className="panel-title-row">
            <div>
              <CardDescription>最新状態と直近25件</CardDescription>
              <CardTitle>DELTA 3 Plus 補助制御 実証</CardTitle>
            </div>
            <Badge variant={delta3Available ? "success" : "secondary"}>{delta3Available ? "接続中" : "取得不可"}</Badge>
          </div>
        </CardHeader>
        <CardContent>
          <div className="detail-strip" aria-label="delta3 auxiliary verification">
            <Detail label="SOC" value={nullablePercent(delta3Status?.soc ?? auxPlan?.delta3Soc ?? latestAuxLog?.delta3Soc)} />
            <Detail label="最大充電SOC" value={nullablePercent(delta3Status?.maxChargeSoc ?? auxPlan?.delta3MaxChargeSoc)} />
            <Detail label="現在AC上限" value={nullableWatt(delta3Status?.acChargeLimitW ?? auxPlan?.currentAcChargeLimitW ?? latestAuxLog?.previousAcChargeLimitW)} />
            <Detail label="推奨AC上限" value={nullableWatt(auxPlan?.recommendedAcChargeLimitW ?? latestAuxLog?.targetAcChargeLimitW)} />
            <Detail label="残余売電" value={nullableWatt(auxPlan?.residualExportW ?? latestAuxLog?.residualExportW)} />
            <Detail label="安全余力" value={nullableWatt(auxPlan?.safetyMarginW)} />
          </div>
          <div className="detail-strip planner-secondary" aria-label="delta3 auxiliary log aggregate">
            <Detail label="状態" value={strategyStateLabel(auxPlan?.strategyState ?? latestAuxLog?.strategyState)} />
            <Detail label="送信判定" value={writeCandidateLabel(auxPlan?.wouldWrite ?? latestAuxLog?.wouldWrite)} />
            <Detail label="ログ件数" value={`${aggregate.total} 件`} />
            <Detail label="送信候補" value={`${aggregate.candidates} 件`} />
            <Detail label="送信済み" value={`${aggregate.sent} 件`} />
            <Detail label="エラー" value={`${aggregate.errors} 件`} />
          </div>
          <p className="planner-reason">抑制: {guardReasonLabel(delta3Reason)}</p>
          <p className="planner-reason">理由: {decisionReasonLabel(auxPlan?.reason ?? latestAuxLog?.decisionReason)}</p>
          <p className="readonly-note">このパネルは確認用です。送信可否は既存の実機制御gateと直近ログの判定結果だけを表示します。</p>
        </CardContent>
      </Card>
    </div>
  );
}

function aggregateDelta3AuxLogs(logs: Delta3AuxControlCommandLog[]) {
  return logs.reduce(
    (summary, log) => {
      summary.total += 1;
      if (log.wouldWrite) {
        summary.candidates += 1;
      }
      if (log.commandSent) {
        summary.sent += 1;
      }
      if (log.errorMessage) {
        summary.errors += 1;
      }
      return summary;
    },
    { total: 0, candidates: 0, sent: 0, errors: 0 }
  );
}

function Detail({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="detail-item">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function nullablePercent(value: number | null | undefined) {
  return typeof value === "number" ? `${value}%` : "-";
}

function nullableWatt(value: number | null | undefined) {
  return typeof value === "number" ? `${value} W` : "-";
}

function nullableKwh(value: number | null | undefined) {
  return typeof value === "number" ? `${formatDecimal(value)} kWh` : "-";
}

function formatSocGap(value: number | null | undefined) {
  if (typeof value !== "number") {
    return "-";
  }
  return `${value >= 0 ? "+" : ""}${value} pt`;
}

function formatKwhGap(value: number | null | undefined) {
  if (typeof value !== "number") {
    return "-";
  }
  return `${value >= 0 ? "+" : ""}${formatDecimal(value)} kWh`;
}

function formatDecimal(value: number) {
  return new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 3 }).format(value);
}

function statusLabel(status: string | null | undefined) {
  const labels: Record<string, string> = {
    pending: "未確定",
    ok: "OK",
    undercharged: "不足",
    overcharged: "過充電",
    "insufficient-data": "データ不足"
  };
  return status ? labels[status] || status : "取得待ち";
}

function nightSummaryBadgeVariant(status: string | null | undefined) {
  if (status === "ok") {
    return "success";
  }
  if (status === "undercharged" || status === "overcharged") {
    return "warning";
  }
  return "secondary";
}
