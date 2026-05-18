import type { EnergyStatus, PowerLog } from "./types";

export async function fetchStatus(): Promise<EnergyStatus> {
  const response = await fetch("/api/status", { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`status request failed: ${response.status}`);
  }
  return response.json();
}

export async function fetchLogs(limit = 100): Promise<PowerLog[]> {
  const response = await fetch(`/api/logs?limit=${limit}`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`logs request failed: ${response.status}`);
  }
  return response.json();
}
