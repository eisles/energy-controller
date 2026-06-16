# Battery Cost Daily Breakdown Implementation Plan

作成時刻: 2026-06-17 04:12:41 JST

## Goal

料金概算の「バッテリーあり / バッテリーなし推定」比較に、日別の判断材料を追加する。

日別に次の値を出し、節約効果が小さい理由を確認できるようにする。

- 低単価充電量
- 高単価・中間単価での放電量
- 売電吸収量
- 推定損失量
- 日別のバッテリーあり / なし推定差額

## Non-goals

- EcoFlow への実機 write、制御条件、充放電ロジックは変更しない。
- Nature Remo / EcoFlow の外部 API 呼び出しは追加しない。
- DB migration は追加しない。既存の `power_logs` と料金マスタから集計する。
- 補助バッテリーの未保存入出力を新たに永続化する変更はしない。

## Current State

- `/api/tariff/summary` は `TariffSummary.BatteryComparison` を返している。
- 比較は `power_logs.grid_w`, `battery_input_w`, `battery_output_w` から、`grid_w - battery_input_w + battery_output_w` を「バッテリーなし推定」として近似している。
- 現在は全期間合計だけなので、効果が小さい理由が「低単価で充電できていない」「高単価で放電できていない」「売電吸収が少ない」「損失が大きい」のどれか判別しづらい。
- 既存の未コミット差分として、サイクル候補診断表示の変更が残っている。今回の実装ではその差分を壊さず、料金比較ファイルだけを追加変更する。

## Data/API Contract

`BatteryCostComparison` に日別配列を追加する。

```json
{
  "dailyBreakdown": [
    {
      "date": "2026-06-16",
      "sampleCount": 123,
      "actualNetCostYen": 120.5,
      "estimatedNoBatteryNetCostYen": 132.0,
      "estimatedSavingsYen": 11.5,
      "lowPriceChargeKwh": 1.2,
      "midPriceDischargeKwh": 0.3,
      "highPriceDischargeKwh": 0.8,
      "exportAbsorptionKwh": 0.6,
      "batteryInputKwh": 1.4,
      "batteryOutputKwh": 1.1,
      "estimatedLossKwh": 0.3
    }
  ]
}
```

集計の考え方:

- 日付は料金ルール解決時のローカル日付でまとめる。
- 低単価充電量は、該当区間が最低単価で `battery_input_w > battery_output_w` の正味充電分を加算する。
- 中間・高単価放電量は、該当区間で `battery_output_w > battery_input_w` の正味放電分を加算する。
- 売電吸収量は、実績 `grid_w < 0` かつ `battery_input_w > battery_output_w` の正味充電分を、実際の売電量を上限に加算する。
- 推定損失量は `max(0, batteryInputKwh - batteryOutputKwh)` とする。
- 既存の合計比較値は後方互換のため維持する。

## Files Likely To Change

- `backend/internal/domain/status.go`
  - `BatteryCostComparisonDailyBreakdown` を追加。
  - `BatteryCostComparison` に `DailyBreakdown` を追加。
- `backend/internal/store/tariff_repository.go`
  - `addBatteryCostInterval` で日別内訳も加算する。
  - 料金区分が低単価・高単価・中間単価のどれか判定する helper を追加する。
- `backend/internal/store/repository_test.go`
  - 日別内訳の集計テストを追加する。
- `frontend/lib/types.ts`
  - API 型を追加する。
- `frontend/components/TariffSummaryPanel.tsx`
  - 料金概算パネルに日別内訳テーブルを追加する。

## Safety Boundaries

- 読み取り済みログの集計だけを変更する。
- EcoFlow/Nature への write や追加 read は行わない。
- 制御判断、料金最適化制御、夜間充電制御は変更しない。
- シリアル番号、API key、token は計画・ログ・表示に追加しない。

## Implementation Steps

1. Domain 型に日別内訳を追加する。
2. 既存の `batteryCostComparison` 集計中に、日別 map を持たせる。
3. 各サンプル区間について、既存の合計値に加えて日別値を更新する。
4. 日別 map を日付昇順の slice に変換し、丸め処理を行う。
5. Go テストで次を確認する。
   - 既存の合計比較値が変わらない。
   - 低単価充電、高単価放電、売電吸収、損失が期待どおり加算される。
6. TypeScript 型を追加する。
7. 料金概算 UI に「日別バッテリー効果」テーブルを追加する。
8. backend test、frontend build、実装レビューを実行する。

## Review Points

- 既存 `batteryComparison` JSON 互換性を壊していないか。
- 料金区分判定が固定時刻ではなく料金マスタ解決結果に基づいているか。
- パススルーに近い `battery_input_w` と `battery_output_w` を過大な充電・放電として見せていないか。
- 売電吸収量が実際の売電量を超えないか。
- 推定値であることが UI 上分かるか。

## Verification Commands

```bash
cd backend && GOCACHE=$PWD/.gocache rtk go test ./...
cd frontend && rtk npm run build
rtk codex review --uncommitted
```

可能なら反映後に次も確認する。

```bash
docker compose up -d --build
curl -s http://localhost:18085/api/tariff/summary
```

## Rollback / Operational Notes

- DB migration なしのため、問題があれば追加フィールドと UI 表示を戻すだけでよい。
- 既存の合計比較値を変更しないため、画面上の従来比較は継続して使える。
- 日別内訳は推定であり、補助バッテリーの機器別ログが未完全な間は効果の全量を表さない。
