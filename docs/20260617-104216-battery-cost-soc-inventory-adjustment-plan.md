# Battery Cost SOC Inventory Adjustment Plan

作成日時: 2026-06-17 10:42:16 JST

## 背景

現在の料金比較は、`power_logs.grid_w` と `battery_input_w` / `battery_output_w` から、バッテリーあり実績とバッテリーなし推定を比較している。

日別内訳では `低単価充電量`、`高/中単価放電量`、`売電吸収量`、`推定損失` を出しているが、`推定損失 = batteryInputKWh - batteryOutputKWh` だけでは、日をまたいで残ったSOCを損失のように見せる場合がある。逆に、前日までに貯めた電力を当日に放電した場合は、当日の効果を過大評価する場合がある。

## 目的

バッテリーあり/なしの料金比較に、SOC増減による在庫価値を追加し、次の2つを分けて見られるようにする。

- 生の削減額: 現行の `estimatedSavingsYen`。その期間内の買電/売電だけで比較する。
- 在庫調整後削減額: SOC増減を翌日以降に使える残量価値、または過去から使った残量価値として補正した比較。

## 非目的

- EcoFlow実機制御の変更はしない。
- real write条件、`ENABLE_REAL_CONTROL`、`SIMULATION_MODE`、最小コマンド間隔、ヒステリシスは変更しない。
- 補助バッテリーの完全な在庫評価は今回の必須範囲にしない。まずは既存 `power_logs.battery_soc` と DELTA Pro 3 容量で精度を上げる。
- Nature/EcoFlow APIの追加呼び出しはしない。

## 実装方針

### 1. APIモデル拡張

`domain.BatteryCostComparison` に次の任意的な数値を追加する。

- `inventoryStartSoc`
- `inventoryEndSoc`
- `inventoryDeltaKWh`
- `inventoryValueYen`
- `inventoryValueRateYen`
- `adjustedEstimatedSavingsYen`

`domain.BatteryCostComparisonDailyBreakdown` に次を追加する。

- `inventoryStartSoc`
- `inventoryEndSoc`
- `inventoryDeltaKWh`
- `inventoryValueYen`
- `inventoryValueRateYen`
- `adjustedEstimatedSavingsYen`

既存 `estimatedSavingsYen` は互換性のため意味を変えない。

### 2. SOC取得

`TariffRepository.queryPowerLogBatterySamples` で `battery_soc` を追加取得する。

`batteryCostSample` には nullable SOC を保持する。SOCがNULL、容量が0、またはSOC値が不正な場合は在庫補正を無効にして、生の比較だけ返す。

### 3. 容量取得

在庫価値計算に使う容量は `charging_devices.capacity_wh` から取得する。

優先候補:

1. `kind = 'ecoflow_delta_pro3'` か `device_type = 'DELTA_PRO3'` の有効機器
2. 上記がなければ、`capacity_wh > 0` の最大値

取得できない場合は在庫補正をスキップする。

### 4. 在庫価値の計算

SOC増減kWh:

```text
inventoryDeltaKWh = capacityKWh * (endSoc - startSoc) / 100
```

在庫価値単価:

```text
inventoryValueRateYen = 対象日の料金マスタの最高買電単価
```

理由:

- 残ったSOCは、次に高い時間帯の買電を避けられる可能性が高い。
- 前日以前から使ったSOCは、過去に貯めた価値を当日に消費したと見なせる。
- 売電単価ではなく買電回避価値として扱う方が、ユーザーの「高単価時間帯に放電したい」という運用意図に近い。

日別:

```text
inventoryValueYen = inventoryDeltaKWh * inventoryValueRateYen
adjustedEstimatedSavingsYen = estimatedSavingsYen + inventoryValueYen
```

期間合計:

```text
adjustedEstimatedSavingsYen = estimatedSavingsYen + sum(daily.inventoryValueYen)
```

### 5. 推定損失の扱い

既存の `estimatedLossKWh` は互換性のため残すが、画面では「推定損失/未回収」として、在庫補正と並べて表示する。日またぎ残量を厳密な損失として断定しない。

### 6. UI

`TariffSummaryPanel` の料金比較に次を追加する。

- サマリーカード: `在庫調整後` と `SOC在庫補正`
- 日別テーブル列: `SOC`, `在庫補正`, `調整後削減額`
- 注記: 在庫補正は `power_logs.battery_soc` と機器容量からの近似であることを表示する。

既存の一覧/編集UIには影響させない。

### 7. テスト

追加・更新するテスト:

- SOCが上がった日は `inventoryDeltaKWh > 0`、`inventoryValueYen > 0`、`adjustedEstimatedSavingsYen` が生の削減額より大きくなる。
- SOCが下がった日は `inventoryDeltaKWh < 0`、`inventoryValueYen < 0`、過去在庫消費として調整される。
- SOCまたは容量が不足している場合も既存の比較結果は返る。

### 8. 確認

最低限:

```bash
cd backend && rtk go test ./...
cd frontend && rtk npm run build
```

実稼働反映が必要な場合:

```bash
docker compose up -d --build
curl -s http://localhost:18085/api/tariff/summary
```

## 安全境界

この実装は料金比較の集計と画面表示だけを変更する。EcoFlow/Natureへのwrite、制御判断、制御対象機器、実機制御フラグは変更しない。
