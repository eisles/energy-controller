# ポータブルバッテリー有無の料金比較 実装計画

## Goal

Nature Remo E の買電/売電実績と EcoFlow 入出力ログを使い、ポータブルバッテリーがある現在の実績料金と、ポータブルバッテリーが無かった場合の推定料金を同じ料金マスタで比較できるようにする。

この機能は費用評価用の read-only 集計であり、充放電制御や EcoFlow write には使わない。

## Non-goals

- 充放電制御ロジックの変更。
- EcoFlow への追加 write。
- Nature / EcoFlow API の新規外部呼び出し。
- 基本料金、燃料費調整額、再エネ賦課金、割引の厳密な請求再現。
- 機器別ログが空の現状で、DELTA 3 Plus / DELTA 3 Max Plus / RIVER 2 の過去分を正確に復元すること。

## Current State

- `energy_meter_logs` には Nature の累積買電/売電差分が保存されている。
- `/api/tariff/summary` は `energy_meter_logs` を料金マスタで集計し、実績の買電料金・売電収入・差引概算を返している。
- `power_logs` には DELTA Pro 3 の `battery_input_w` / `battery_output_w` があり、バッテリー無し推定の近似に使える。
- `charging_device_power_logs` テーブルは存在するが、現DBではまだ0件のため、補助バッテリーの過去入出力は比較へ反映できない。
- 料金時間帯は `tariff_plans` と `tariff_period_rules` で管理され、カスタムルールが無い場合は既定ルールに fallback する。

## Data / API Contract

`GET /api/tariff/summary` の既存レスポンスに後方互換の追加フィールドを入れる。

```json
{
  "batteryComparison": {
    "available": true,
    "method": "power_logs_no_battery_estimate",
    "sampleCount": 1234,
    "actualImportKwh": 10.0,
    "actualExportKwh": 3.0,
    "actualNetCostYen": 200.0,
    "estimatedNoBatteryImportKwh": 12.0,
    "estimatedNoBatteryExportKwh": 1.0,
    "estimatedNoBatteryNetCostYen": 300.0,
    "estimatedSavingsYen": 100.0,
    "batteryInputKwh": 4.0,
    "batteryOutputKwh": 3.5,
    "note": "..."
  }
}
```

### Estimation Formula

時系列 `power_logs` から隣接サンプル間の秒数 `dt` を計算する。異常に長い欠損区間は過大積算を避けるため上限を設ける。

- 実績の系統電力: `grid_w`
- バッテリー無し推定: `grid_w - battery_input_w + battery_output_w`
- 正なら買電、負なら売電。
- 買電は該当時刻の料金時間帯単価、売電は料金プランの `export_rate_yen` で計算する。

この式は「DELTA Pro 3 の入出力が主な差分である」という近似であり、機器別ログが揃うまでは推定精度を `approximate` として表示する。

## Files Likely To Change

- `backend/internal/domain/status.go`
  - `TariffSummary` に `BatteryComparison` を追加。
  - 比較用 domain struct を追加。
- `backend/internal/store/tariff_repository.go`
  - `EnergyCostSummary` 内で比較集計を追加。
  - `power_logs` からバッテリー無し推定を算出する query/helper を追加。
- `backend/internal/store/repository_test.go`
  - 料金比較の単体テストを追加。
- `frontend/lib/types.ts`
  - API型を追加。
- `frontend/components/TariffSummaryPanel.tsx`
  - 料金概算カードに「バッテリーあり/なし比較」を表示。

## Safety Boundaries

- read-only 集計のみ。
- EcoFlow write path、制御 planner、command executor は変更しない。
- `.env`、認証情報、デバイスSN、APIキーは触らない。
- 推定結果は制御判断に使わず、画面表示とAPIレスポンスに限定する。
- 推定精度と対象データの限界をUIと `note` に明示する。

## Implementation Steps

1. Domain type に `BatteryCostComparison` を追加し、`TariffSummary` へ optional field として接続する。
2. `TariffRepository.EnergyCostSummary` で既存料金集計後に比較集計を呼び出す。
3. 比較集計は `power_logs` の `measured_at, grid_w, battery_input_w, battery_output_w` を時系列で読み、隣接サンプルの `dt` からkWhを計算する。
4. 各サンプル時刻に有効な料金プラン・時間帯を既存 helper と同じ規則で解決する。
5. サンプル欠損が多い場合は `available=false` または `note` で警告する。
6. Frontend type を更新し、`TariffSummaryPanel` に比較カードを追加する。
7. 既存の料金概算テーブル・検索期間フィルタは維持する。

## Review Points

- 既存 `/api/tariff/summary` の後方互換性が維持されていること。
- `from` / `to` フィルタが比較集計にも反映されること。
- 時間帯単価の解決が既存料金概算と同じであること。
- 長いログ欠損で料金が過大計上されないこと。
- UI が推定値を実績値と誤認させないこと。

## Verification Commands

- `cd backend && rtk go test ./internal/store ./internal/api ./cmd/server`
- `cd backend && rtk go test ./...`
- `cd frontend && rtk npm run build`
- `curl -s 'http://localhost:18085/api/tariff/summary'` で `batteryComparison` を確認する。

## Rollback / Operational Notes

- 追加フィールドは optional なので、問題があれば UI 表示と比較集計呼び出しを戻せば既存料金概算へ戻せる。
- 稼働中の実機制御サービスを再起動する場合も、今回の変更は read-only 集計のみで write 挙動は変えない。
