"use client";

import { useEffect, useMemo, useState } from "react";
import type { MouseEvent } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Form, FormControl, FormDescription, FormItem, FormLabel } from "@/components/ui/form";
import { fetchDaytimeConsumptionEstimate, fetchEcoFlowLoadEstimate, fetchWeatherLocation, updateWeatherLocation } from "@/lib/api";
import type { DaytimeConsumptionEstimate, EcoFlowLoadEstimate, WeatherLocation } from "@/lib/types";

const fallbackLocation: WeatherLocation = {
  enabled: false,
  latitude: 35.681236,
  longitude: 139.767125,
  timezone: "Asia/Tokyo",
  pvCapacityKw: 0,
  pvPerformanceRatio: 0.75,
  dailyBaseLoadKwh: 0,
  batteryCapacityKwh: 4.096,
  minimumReserveSoc: 30
};

export function WeatherLocationPanel() {
  const [open, setOpen] = useState(false);
  const [location, setLocation] = useState<WeatherLocation>(fallbackLocation);
  const [estimate, setEstimate] = useState<DaytimeConsumptionEstimate | null>(null);
  const [ecoFlowEstimate, setEcoFlowEstimate] = useState<EcoFlowLoadEstimate | null>(null);
  const [status, setStatus] = useState<string>("未読込");
  const [saving, setSaving] = useState(false);
  const [estimating, setEstimating] = useState(false);
  const [estimatingEcoFlow, setEstimatingEcoFlow] = useState(false);

  useEffect(() => {
    let cancelled = false;
    fetchWeatherLocation()
      .then((next) => {
        if (cancelled) {
          return;
        }
        setLocation(normalizeLocation(next));
        setStatus(next.enabled ? "有効" : "無効");
      })
      .catch((err) => {
        if (!cancelled) {
          setStatus(err instanceof Error ? err.message : "読み込み失敗");
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function save(enabled: boolean) {
    setSaving(true);
    try {
      const saved = await updateWeatherLocation(normalizeLocation({ ...location, enabled }));
      setLocation(normalizeLocation(saved));
      setStatus("保存済み");
      setOpen(false);
    } catch (err) {
      setStatus(err instanceof Error ? err.message : "保存失敗");
    } finally {
      setSaving(false);
    }
  }

  async function estimateDaytimeLoad() {
    setEstimating(true);
    try {
      const nextEstimate = await fetchDaytimeConsumptionEstimate(7);
      setEstimate(nextEstimate);
      setLocation((current) => ({ ...current, dailyBaseLoadKwh: roundKwh(nextEstimate.suggestedDailyBaseLoadKwh) }));
      setStatus("直近7日から日中消費を推定しました");
    } catch (err) {
      setStatus(err instanceof Error ? err.message : "推定失敗");
    } finally {
      setEstimating(false);
    }
  }

  async function estimateEcoFlowLoad() {
    setEstimatingEcoFlow(true);
    try {
      const nextEstimate = await fetchEcoFlowLoadEstimate(7);
      setEcoFlowEstimate(nextEstimate);
      if (nextEstimate.completeDaytimeSampleDays > 0) {
        setLocation((current) => ({ ...current, dailyBaseLoadKwh: roundKwh(nextEstimate.suggestedDaytimeBaseLoadKwh) }));
        setStatus("EcoFlow出力の完了済み日中データから日中消費を推定しました");
      } else {
        setStatus("EcoFlow出力の日中完了データが不足しています");
      }
    } catch (err) {
      setStatus(err instanceof Error ? err.message : "EcoFlow出力推定失敗");
    } finally {
      setEstimatingEcoFlow(false);
    }
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>天気設定</CardTitle>
          <CardDescription>深夜充電プラン用の設置地点</CardDescription>
        </CardHeader>
        <CardContent>
          <p className="readonly-note">
            {location.enabled ? `${formatCoord(location.latitude)}, ${formatCoord(location.longitude)}` : "未設定"}
          </p>
          <p className="readonly-note">
            PV {formatKwh(location.pvCapacityKw)} kW / 補正 {formatRatio(location.pvPerformanceRatio)} / Battery {formatKwh(location.batteryCapacityKwh)} kWh
          </p>
          <p className="readonly-note">状態: {status}</p>
          <Button type="button" variant="outline" onClick={() => setOpen(true)}>
            地点を編集
          </Button>
        </CardContent>
      </Card>

      {open ? (
        <div className="drawer-backdrop" role="presentation">
          <aside className="settings-drawer" aria-label="weather location settings">
            <div className="drawer-header">
              <div>
                <p className="eyebrow">Weather</p>
                <h2>設置地点</h2>
              </div>
              <Button type="button" variant="outline" onClick={() => setOpen(false)}>
                閉じる
              </Button>
            </div>

            <MapPicker location={location} onChange={setLocation} />

            <Form>
              <div className="drawer-section-title">地点</div>
              <FormItem>
                <FormLabel htmlFor="weather-latitude">緯度</FormLabel>
                <FormControl>
                  <input
                    id="weather-latitude"
                    className="text-input"
                    value={String(location.latitude)}
                    onChange={(event) => setLocation((current) => ({ ...current, latitude: Number(event.target.value) }))}
                  />
                </FormControl>
              </FormItem>
              <FormItem>
                <FormLabel htmlFor="weather-longitude">経度</FormLabel>
                <FormControl>
                  <input
                    id="weather-longitude"
                    className="text-input"
                    value={String(location.longitude)}
                    onChange={(event) => setLocation((current) => ({ ...current, longitude: Number(event.target.value) }))}
                  />
                </FormControl>
              </FormItem>
              <FormItem>
                <FormLabel htmlFor="weather-timezone">Timezone</FormLabel>
                <FormControl>
                  <input
                    id="weather-timezone"
                    className="text-input"
                    value={location.timezone}
                    onChange={(event) => setLocation((current) => ({ ...current, timezone: event.target.value }))}
                  />
                </FormControl>
                <FormDescription>Open-Meteo の予報取得に使います。EcoFlow 制御設定は変更しません。</FormDescription>
              </FormItem>
              <div className="drawer-section-title">太陽光パネル仕様</div>
              <FormItem>
                <FormLabel htmlFor="pv-capacity">パネル容量 kW</FormLabel>
                <FormControl>
                  <input
                    id="pv-capacity"
                    className="text-input"
                    type="number"
                    min="0"
                    step="0.1"
                    value={String(location.pvCapacityKw)}
                    onChange={(event) => setLocation((current) => ({ ...current, pvCapacityKw: Number(event.target.value) }))}
                  />
                </FormControl>
              </FormItem>
              <FormItem>
                <FormLabel htmlFor="pv-performance-ratio">損失補正</FormLabel>
                <FormControl>
                  <input
                    id="pv-performance-ratio"
                    className="text-input"
                    type="number"
                    min="0.1"
                    max="1"
                    step="0.01"
                    value={String(location.pvPerformanceRatio)}
                    onChange={(event) => setLocation((current) => ({ ...current, pvPerformanceRatio: Number(event.target.value) }))}
                  />
                </FormControl>
                <FormDescription>方角、傾斜、温度、パワコン損失などの概算係数です。</FormDescription>
              </FormItem>
              <FormItem>
                <FormLabel htmlFor="daily-base-load">日中消費 kWh</FormLabel>
                <FormControl>
                  <input
                    id="daily-base-load"
                    className="text-input"
                    type="number"
                    min="0"
                    step="0.1"
                    value={String(location.dailyBaseLoadKwh)}
                    onChange={(event) => setLocation((current) => ({ ...current, dailyBaseLoadKwh: Number(event.target.value) }))}
                  />
                </FormControl>
                <FormDescription>EcoFlow出力ログを優先して、特定回路の 09:00-16:00 消費として推定できます。</FormDescription>
              </FormItem>
              <div className="estimate-panel">
                <div>
                  <strong>EcoFlow特定回路推定</strong>
	                  <p>
	                    {ecoFlowEstimate
	                      ? `日中 ${formatKwh(ecoFlowEstimate.suggestedDaytimeBaseLoadKwh)} kWh / 完了日 ${ecoFlowEstimate.completeDaytimeSampleDays}/${ecoFlowEstimate.daytimeSampleDays} / 朝夕 ${formatKwh(ecoFlowEstimate.averageShoulderOutputKwh)} kWh / 1日 ${formatKwh(ecoFlowEstimate.averageDailyOutputKwh)} kWh / samples ${ecoFlowEstimate.sampleCount}`
	                      : "未取得"}
	                  </p>
                </div>
                <Button type="button" variant="outline" onClick={estimateEcoFlowLoad} disabled={estimatingEcoFlow}>
                  {estimatingEcoFlow ? "推定中" : "EcoFlow出力から推定"}
                </Button>
              </div>
              {ecoFlowEstimate ? (
                <div className="estimate-grid" aria-label="EcoFlow load estimate">
	                  <DetailText label="日中出力" value={`${formatKwh(ecoFlowEstimate.averageDaytimeOutputKwh)} kWh`} />
	                  <DetailText label="日中完了日" value={`${ecoFlowEstimate.completeDaytimeSampleDays} / ${ecoFlowEstimate.daytimeSampleDays} 日`} />
	                  <DetailText label="朝夕出力" value={`${formatKwh(ecoFlowEstimate.averageShoulderOutputKwh)} kWh`} />
                  <DetailText label="夜間出力" value={`${formatKwh(ecoFlowEstimate.averageNightOutputKwh)} kWh`} />
                  <DetailText label="1日出力" value={`${formatKwh(ecoFlowEstimate.averageDailyOutputKwh)} kWh`} />
                  <DetailText label="日中充電" value={`${formatKwh(ecoFlowEstimate.averageDaytimeChargeKwh)} kWh`} />
                </div>
              ) : null}
              <div className="estimate-panel">
                <div>
                  <strong>家全体推定</strong>
                  <p>{estimate ? `推定日中消費 ${formatKwh(estimate.suggestedDailyBaseLoadKwh)} kWh / samples ${estimate.sampleCount}` : "Nature買電売電とBattery入出力からの参考値"}</p>
                </div>
                <Button type="button" variant="outline" onClick={estimateDaytimeLoad} disabled={estimating}>
                  {estimating ? "推定中" : "家全体ログから推定"}
                </Button>
              </div>
              {estimate ? (
                <div className="estimate-grid" aria-label="daytime consumption estimate">
                  <DetailText label="買電" value={`${formatKwh(estimate.averageImportKwh)} kWh`} />
                  <DetailText label="売電" value={`${formatKwh(estimate.averageExportKwh)} kWh`} />
                  <DetailText label="充電" value={`${formatKwh(estimate.averageBatteryChargeKwh)} kWh`} />
                  <DetailText label="放電" value={`${formatKwh(estimate.averageBatteryDischargeKwh)} kWh`} />
                </div>
              ) : null}
              <FormItem>
                <FormLabel htmlFor="battery-capacity">蓄電池容量 kWh</FormLabel>
                <FormControl>
                  <input
                    id="battery-capacity"
                    className="text-input"
                    type="number"
                    min="0.1"
                    step="0.1"
                    value={String(location.batteryCapacityKwh)}
                    onChange={(event) => setLocation((current) => ({ ...current, batteryCapacityKwh: Number(event.target.value) }))}
                  />
                </FormControl>
              </FormItem>
              <FormItem>
                <FormLabel htmlFor="minimum-reserve-soc">最低確保SOC %</FormLabel>
                <FormControl>
                  <input
                    id="minimum-reserve-soc"
                    className="text-input"
                    type="number"
                    min="0"
                    max="100"
                    step="1"
                    value={String(location.minimumReserveSoc)}
                    onChange={(event) => setLocation((current) => ({ ...current, minimumReserveSoc: Number(event.target.value) }))}
                  />
                </FormControl>
              </FormItem>
              <div className="drawer-actions">
                <Button type="button" onClick={() => save(true)} disabled={saving}>
                  {saving ? "保存中" : "保存"}
                </Button>
                <Button type="button" variant="secondary" onClick={() => save(false)} disabled={saving}>
                  無効化
                </Button>
              </div>
            </Form>
            <p className="readonly-note">{status}</p>
          </aside>
        </div>
      ) : null}
    </>
  );
}

function MapPicker({ location, onChange }: { location: WeatherLocation; onChange: (location: WeatherLocation) => void }) {
  const zoom = 15;
  const tiles = useMemo(() => mapTiles(location.latitude, location.longitude, zoom), [location.latitude, location.longitude]);

  function handleClick(event: MouseEvent<HTMLDivElement>) {
    const rect = event.currentTarget.getBoundingClientRect();
    const x = event.clientX - rect.left;
    const y = event.clientY - rect.top;
    const next = pixelToLatLon(tiles.originPixelX + x, tiles.originPixelY + y, zoom);
    onChange({ ...location, latitude: roundCoord(next.latitude), longitude: roundCoord(next.longitude), enabled: true });
  }

  return (
    <div className="map-picker" onClick={handleClick} role="button" tabIndex={0} aria-label="map picker">
      <div className="map-tile-layer" style={{ width: tiles.width, height: tiles.height, transform: `translate(${-tiles.offsetX}px, ${-tiles.offsetY}px)` }}>
        {tiles.items.map((tile) => (
          <img
            key={`${tile.x}-${tile.y}`}
            src={`https://tile.openstreetmap.org/${zoom}/${tile.x}/${tile.y}.png`}
            alt=""
            width={256}
            height={256}
            style={{ left: tile.left, top: tile.top }}
          />
        ))}
      </div>
      <div className="map-pin" aria-hidden="true" />
    </div>
  );
}

function normalizeLocation(location: WeatherLocation): WeatherLocation {
  return {
    enabled: location.enabled,
    latitude: Number.isFinite(location.latitude) && location.latitude !== 0 ? roundCoord(location.latitude) : fallbackLocation.latitude,
    longitude: Number.isFinite(location.longitude) && location.longitude !== 0 ? roundCoord(location.longitude) : fallbackLocation.longitude,
    timezone: location.timezone || "Asia/Tokyo",
    pvCapacityKw: finiteOr(location.pvCapacityKw, fallbackLocation.pvCapacityKw),
    pvPerformanceRatio: finiteOr(location.pvPerformanceRatio, fallbackLocation.pvPerformanceRatio),
    dailyBaseLoadKwh: finiteOr(location.dailyBaseLoadKwh, fallbackLocation.dailyBaseLoadKwh),
    batteryCapacityKwh: finiteOr(location.batteryCapacityKwh, fallbackLocation.batteryCapacityKwh),
    minimumReserveSoc: Math.round(finiteOr(location.minimumReserveSoc, fallbackLocation.minimumReserveSoc))
  };
}

function mapTiles(latitude: number, longitude: number, zoom: number) {
  const width = 520;
  const height = 260;
  const center = latLonToPixel(latitude, longitude, zoom);
  const originPixelX = center.x - width / 2;
  const originPixelY = center.y - height / 2;
  const startTileX = Math.floor(originPixelX / 256);
  const startTileY = Math.floor(originPixelY / 256);
  const endTileX = Math.floor((originPixelX + width) / 256);
  const endTileY = Math.floor((originPixelY + height) / 256);
  const maxTile = 2 ** zoom;
  const items = [];
  for (let x = startTileX; x <= endTileX; x += 1) {
    for (let y = startTileY; y <= endTileY; y += 1) {
      items.push({
        x: ((x % maxTile) + maxTile) % maxTile,
        y: Math.max(0, Math.min(maxTile - 1, y)),
        left: x * 256 - startTileX * 256,
        top: y * 256 - startTileY * 256
      });
    }
  }
  return {
    width: (endTileX - startTileX + 1) * 256,
    height: (endTileY - startTileY + 1) * 256,
    offsetX: originPixelX - startTileX * 256,
    offsetY: originPixelY - startTileY * 256,
    originPixelX,
    originPixelY,
    items
  };
}

function latLonToPixel(latitude: number, longitude: number, zoom: number) {
  const sinLat = Math.sin((latitude * Math.PI) / 180);
  const scale = 256 * 2 ** zoom;
  return {
    x: ((longitude + 180) / 360) * scale,
    y: (0.5 - Math.log((1 + sinLat) / (1 - sinLat)) / (4 * Math.PI)) * scale
  };
}

function pixelToLatLon(x: number, y: number, zoom: number) {
  const scale = 256 * 2 ** zoom;
  const longitude = (x / scale) * 360 - 180;
  const n = Math.PI - (2 * Math.PI * y) / scale;
  const latitude = (180 / Math.PI) * Math.atan(0.5 * (Math.exp(n) - Math.exp(-n)));
  return { latitude, longitude };
}

function roundCoord(value: number) {
  return Math.round(value * 1_000_000) / 1_000_000;
}

function roundKwh(value: number) {
  return Math.round(value * 10) / 10;
}

function formatCoord(value: number) {
  return new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 6 }).format(value);
}

function finiteOr(value: number, fallback: number) {
  return Number.isFinite(value) ? value : fallback;
}

function formatKwh(value: number) {
  return new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 2 }).format(value);
}

function formatRatio(value: number) {
  return new Intl.NumberFormat("ja-JP", { maximumFractionDigits: 2 }).format(value);
}

function DetailText({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}
