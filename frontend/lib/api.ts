import type { EnergyStatus } from "./types";

export async function fetchStatus(): Promise<EnergyStatus> {
  const response = await fetch("/api/status", { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`status request failed: ${response.status}`);
  }
  return response.json();
}
