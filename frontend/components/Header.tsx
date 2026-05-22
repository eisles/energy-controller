import { Badge } from "@/components/ui/badge";
import type { EnergyStatus } from "@/lib/types";

export function Header({ status }: { status: EnergyStatus }) {
  const trial = realControlTrialBadge(status);

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
