import { Badge } from "@/components/ui/badge";
import type { EnergyStatus } from "@/lib/types";

export function Header({ status }: { status: EnergyStatus }) {
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
      </div>
    </header>
  );
}
