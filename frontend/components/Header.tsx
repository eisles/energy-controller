import { Badge } from "@/components/ui/badge";
import { controlWriteReadinessReasonLabel } from "@/lib/display-labels";
import type { EnergyStatus } from "@/lib/types";

export function Header({ status }: { status: EnergyStatus }) {
  const trial = realControlTrialBadge(status);
  const writeReadiness = controlWriteReadinessBadge(status);

  return (
    <header className="app-header">
      <div>
        <p className="eyebrow">Read-only dashboard</p>
        <h1 className="title">Energy Controller</h1>
        <p className="subtitle">Nature Remo E と EcoFlow DELTA Pro 3 の状態監視</p>
      </div>
      <div className="header-badges" aria-label="runtime mode">
        <Badge variant="secondary">{status.mode || "unknown"}</Badge>
        <Badge variant={status.state === "error" ? "destructive" : "success"}>{status.state || "unknown"}</Badge>
        <Badge variant={trial.variant}>{trial.label}</Badge>
        <Badge variant={writeReadiness.variant}>{writeReadiness.label}</Badge>
        {writeReadiness.reason ? <span className="header-readiness-reason">{writeReadiness.reason}</span> : null}
      </div>
    </header>
  );
}

function realControlTrialBadge(status: EnergyStatus) {
  if (!status.realControlTrialUntil) {
    return { label: "実制御期限 未設定", variant: "secondary" as const };
  }
  const until = new Date(status.realControlTrialUntil);
  const formatted = Number.isNaN(until.getTime())
    ? status.realControlTrialUntil
    : new Intl.DateTimeFormat("ja-JP", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit"
      }).format(until);
  return {
    label: status.realControlTrialActive ? `実制御期限 ${formatted}` : `実制御期限切れ ${formatted}`,
    variant: status.realControlTrialActive ? ("success" as const) : ("destructive" as const)
  };
}

function controlWriteReadinessBadge(status: EnergyStatus) {
  const readiness = status.controlWriteReadiness;
  if (!readiness) {
    return { label: "実制御判定 未取得", variant: "secondary" as const, reason: "" };
  }
  const reason = readiness.ready ? "" : controlWriteReadinessReasonLabel(readiness.reasons[0]);
  return {
    label: readiness.ready ? "実制御 ready" : "実制御 dry-run",
    variant: readiness.ready ? ("success" as const) : ("secondary" as const),
    reason
  };
}
