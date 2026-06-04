# PV Forecast History Graph Implementation Plan

## Goal

過去に保存されている深夜充電計画ログから、発電予測の推移をグラフで確認できるようにする。

表示対象は `night_charge_plan_logs` に保存済みの予測値とし、日別に集約して以下を見られるようにする。

- 発電予測 kWh
- 補正後発電予測 kWh
- バッテリー充電に使える想定 kWh
- 日中不足見込み kWh
- 推奨深夜目標 SOC
- 必要深夜充電量 kWh

## Non-goals

- EcoFlow / Nature Remo への実機 write は追加しない。
- 天気予報 API への追加取得や過去予報の再取得はしない。
- `pv_charge_correction_daily_logs` の実績集計自動生成は今回の対象外。
- 料金最適化ロジック、充放電制御ロジックは変更しない。

## Current State

- `night_charge_plan_logs` には `target_forecast_date`、`daily_estimated_pv_kwh`、`corrected_estimated_pv_kwh`、`corrected_estimated_pv_to_battery_kwh`、`forecast_daytime_deficit_kwh` などが保存されている。
- 既存 API `GET /api/night-charge/plans` はページングされた生ログを返す。
- フロントエンドは `Recharts` と `ChartContainer` を使ったグラフ実装が `frontend/components/EnergyCharts.tsx` にある。
- ダッシュボードには `発電予測` セクションと `夜間計画・結果` セクションがある。

## Data/API Contract

新規に読み取り専用 API を追加する。

`GET /api/night-charge/forecast-history?days=30`

レスポンス:

```json
{
  "items": [
    {
      "forecastDate": "2026-06-02",
      "firstMeasuredAt": "2026-06-01T12:00:05+09:00",
      "lastMeasuredAt": "2026-06-02T06:50:42+09:00",
      "sampleCount": 4280,
      "estimatedPvKwh": 2.754,
      "correctedEstimatedPvKwh": 1.928,
      "correctedEstimatedPvToBatteryKwh": 1.928,
      "forecastDaytimeDeficitKwh": 1.762,
      "recommendedNightTargetSoc": 59,
      "requiredNightChargeKwh": 0,
      "shouldChargeTonight": false
    }
  ]
}
```

集約方針:

- `target_forecast_date` がある行を対象にする。
- 日付ごとに最新 `measured_at` のログを代表値として使う。
- `sample_count`、`first_measured_at`、`last_measured_at` は同日ログから計算する。
- `days` は 1-90 に丸める。既定は 30。
- DBに履歴がない場合は `items: []` を返す。

## Files Likely To Change

- `backend/internal/domain/status.go`
  - 発電予測履歴のレスポンス型を追加。
- `backend/internal/store/night_charge_plan_repository.go`
  - 日別代表ログを返す読み取りメソッドを追加。
- `backend/internal/api/night_charge_forecast_history_handler.go`
  - 新規 API handler を追加。
- `backend/internal/api/router.go`
  - 新規 route を登録。
- `backend/internal/api/*_test.go` / `backend/internal/store/*_test.go`
  - handler / repository のテストを追加。
- `frontend/lib/types.ts`
  - 発電予測履歴型を追加。
- `frontend/lib/api.ts`
  - fetch 関数を追加。
- `frontend/components/PVForecastHistoryChart.tsx`
  - グラフ UI を追加。
- `frontend/components/Dashboard.tsx`
  - セクション状態、取得処理、表示を追加。
- 必要なら `frontend/app/globals.css`
  - 既存 chart layout で不足する最小スタイルのみ追加。

## UI Plan

`発電予測` の近くに `発電予測履歴` セクションを追加する。

表示内容:

- レンジボタン: `14日` / `30日` / `90日`
- 折れ線グラフ 1: 予測 kWh
  - 発電予測
  - 補正後予測
  - バッテリー充電想定
  - 日中不足見込み
- 折れ線グラフ 2: 充電判断
  - 推奨深夜目標 SOC
  - 必要深夜充電量 kWh
- サマリー:
  - 対象日数
  - 最新対象日
  - 最新の補正後予測
  - 最新の不足見込み

既存 UI と同じ `Card` / `ChartContainer` / `range-selector` を使う。

## Safety Boundaries

- 読み取り専用。実機制御、設定更新、DB write は追加しない。
- 外部 API は呼ばず、SQLite の既存ログだけを読む。
- API token、SN、secret は扱わない。
- 429対策や制御周期は変更しない。

## Implementation Steps

1. domain に `PVForecastHistoryItem` と `PVForecastHistoryResponse` を追加する。
2. `NightChargePlanRepository` に `ListPVForecastHistory(ctx, days)` を追加する。
3. 新規 handler `nightChargeForecastHistoryHandler` を追加する。
4. router に `GET /api/night-charge/forecast-history` を登録する。
5. Go handler/repository テストを追加する。
6. frontend 型と `fetchPVForecastHistory(days)` を追加する。
7. `PVForecastHistoryChart` を作り、既存 Recharts パターンで表示する。
8. `Dashboard` に state/effect/section を追加する。
9. frontend build と backend test を通す。
10. `rtk codex review --uncommitted` で実装レビューを行い、指摘があれば修正する。

## Verification Commands

```bash
cd backend && GOCACHE=$PWD/.gocache rtk go test ./...
cd frontend && rtk npm run build
rtk git diff --check
rtk codex review --uncommitted
```

## Rollback / Operational Notes

- 新規 API と UI は読み取り専用なので、問題があれば route 登録と Dashboard セクション追加を戻せば制御への影響はない。
- 既存の `night_charge_plan_logs` を直接読むため、発電予測履歴はログ保存開始日以降のみ表示される。
- `pv_charge_correction_daily_logs` が空でも、このグラフは `night_charge_plan_logs` だけで表示できる。
