import type {
  DaytimeConsumptionEstimate,
  EcoFlowLoadEstimate,
  EnergyMeterLogsPage,
  EnergyStatus,
  NightChargePlanLogsPage,
  PowerLog,
  PowerLogsPage,
  SolarForecastSummary,
  TariffPlan,
  TariffSummary,
  WeatherLocation
} from "./types";

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

export async function fetchLogsPage({
  limit,
  offset,
  q,
  from,
  to
}: {
  limit: number;
  offset: number;
  q?: string;
  from?: string;
  to?: string;
}): Promise<PowerLogsPage> {
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset)
  });
  if (q) {
    params.set("q", q);
  }
  if (from) {
    params.set("from", from);
  }
  if (to) {
    params.set("to", to);
  }
  const response = await fetch(`/api/logs?${params.toString()}`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`logs request failed: ${response.status}`);
  }
  return response.json();
}

export async function fetchEnergyMeterLogsPage({
  limit,
  offset,
  from,
  to
}: {
  limit: number;
  offset: number;
  from?: string;
  to?: string;
}): Promise<EnergyMeterLogsPage> {
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset)
  });
  if (from) {
    params.set("from", from);
  }
  if (to) {
    params.set("to", to);
  }
  const response = await fetch(`/api/energy-meter/logs?${params.toString()}`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`energy meter logs request failed: ${response.status}`);
  }
  return response.json();
}

export async function fetchNightChargePlanLogsPage({
  limit,
  offset,
  from,
  to
}: {
  limit: number;
  offset: number;
  from?: string;
  to?: string;
}): Promise<NightChargePlanLogsPage> {
  const params = new URLSearchParams({
    limit: String(limit),
    offset: String(offset)
  });
  if (from) {
    params.set("from", from);
  }
  if (to) {
    params.set("to", to);
  }
  const response = await fetch(`/api/night-charge/plans?${params.toString()}`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`night charge plan logs request failed: ${response.status}`);
  }
  return response.json();
}

export async function fetchTariffSummary({ from, to }: { from?: string; to?: string } = {}): Promise<TariffSummary> {
  const params = new URLSearchParams();
  if (from) {
    params.set("from", from);
  }
  if (to) {
    params.set("to", to);
  }
  const query = params.toString();
  const response = await fetch(`/api/tariff/summary${query ? `?${query}` : ""}`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`tariff summary request failed: ${response.status}`);
  }
  return response.json();
}

export async function fetchTariffPlans(): Promise<TariffPlan[]> {
  const response = await fetch("/api/settings/tariff-plans", { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`tariff plans request failed: ${response.status}`);
  }
  return response.json();
}

export async function saveTariffPlan(plan: TariffPlan): Promise<TariffPlan> {
  const response = await fetch("/api/settings/tariff-plans", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(plan)
  });
  if (!response.ok) {
    throw new Error(`tariff plan update failed: ${response.status}`);
  }
  return response.json();
}

export async function deleteTariffPlan(id: number): Promise<void> {
  const response = await fetch(`/api/settings/tariff-plans/${id}`, {
    method: "DELETE"
  });
  if (!response.ok) {
    throw new Error(`tariff plan delete failed: ${response.status}`);
  }
}

export async function fetchWeatherLocation(): Promise<WeatherLocation> {
  const response = await fetch("/api/settings/weather-location", { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`weather location request failed: ${response.status}`);
  }
  return response.json();
}

export async function updateWeatherLocation(location: WeatherLocation): Promise<WeatherLocation> {
  const response = await fetch("/api/settings/weather-location", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(location)
  });
  if (!response.ok) {
    throw new Error(`weather location update failed: ${response.status}`);
  }
  return response.json();
}

export async function fetchDaytimeConsumptionEstimate(days = 7): Promise<DaytimeConsumptionEstimate> {
  const params = new URLSearchParams({ days: String(days) });
  const response = await fetch(`/api/analytics/daytime-consumption?${params.toString()}`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`daytime consumption estimate failed: ${response.status}`);
  }
  return response.json();
}

export async function fetchEcoFlowLoadEstimate(days = 7): Promise<EcoFlowLoadEstimate> {
  const params = new URLSearchParams({ days: String(days) });
  const response = await fetch(`/api/analytics/ecoflow-load?${params.toString()}`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`EcoFlow load estimate failed: ${response.status}`);
  }
  return response.json();
}

export async function fetchSolarForecast(days = 3): Promise<SolarForecastSummary> {
  const params = new URLSearchParams({ days: String(days) });
  const response = await fetch(`/api/weather/solar-forecast?${params.toString()}`, { cache: "no-store" });
  if (!response.ok) {
    throw new Error(`solar forecast request failed: ${response.status}`);
  }
  return response.json();
}
