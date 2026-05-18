import type { EnergyStatus, PowerLog } from "./types";

export async function fetchStatus(): Promise<EnergyStatus> {
  const response = await fetch("/api/status", { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`status request failed: ${response.status}`);
  }
  return response.json();
}

export async function fetchLogs({ limit, since }: { limit?: number; since?: string } = {}): Promise<PowerLog[]> {
  const params = new URLSearchParams();
  if (since) {
    params.set("since", since);
  }
  if (limit !== undefined) {
    params.set("limit", String(limit));
  } else if (!since) {
    params.set("limit", "100");
  }
  const response = await fetch(`/api/logs?${params.toString()}`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`logs request failed: ${response.status}`);
  }
  return response.json();
}
