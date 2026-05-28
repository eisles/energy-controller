# Phase 7 PV充電補正係数 実装計画

## 目的

天気予報の日射量から算出したPV発電予測を、そのまま深夜充電計画へ使わない。毎日の予測値と実際にEcoFlowへ充電できた量を比較し、PV発電予測に掛ける補正係数を提案できるようにする。

あわせて、DELTA Pro 3 だけでなく DELTA 3 Plus も機器ごとの日中負荷を持つ前提にし、全機器の必要量、利用可能量、PV割当、深夜充電目標SOCを計算できるようにする。

最初は補正係数の手動適用を基本とし、十分なログが取れてから自動適用できる余地を残す。初期実装では自動適用トグルは出さず、明示操作で保存された補正係数だけを制御計算に使う。

## 背景

現在の深夜充電プランは、Open-Meteoの日射量から推定PV発電量を計算し、EcoFlowの過去出力ログから日中使用量を差し引いて、翌日の深夜充電目標を決めている。

ただし、予測PV発電量の全量がEcoFlow充電へ回るとは限らない。

- 家全体の消費に先に使われる。
- EcoFlowのAC入力上限、最低充電W、満充電、バックアップリザーブに制約される。
- DELTA Pro 3とDELTA 3 Plusの受け入れ可能量が時間帯で変わる。
- 売電していても、制御条件により充電へ取り込めない時間がある。
- 天気予報の日射量と実際の発電量にはズレがある。
- DELTA 3 Plus が日中に約400W程度を担当する前提が、深夜充電プランの不足量に十分反映されていない。

そのため、以下の考え方に変更する。

```text
補正後PV充電見込みkWh = 予測PV発電kWh * PV充電補正係数
```

さらに、補正後PV充電見込みは「全機器向けのPV充電余力」として扱い、優先順位順に各機器へ配分する。

## 非目的

- EcoFlowの安全ゲートを緩めない。
- 実機writeを無条件に増やさない。
- Nature RemoやEcoFlowの認証情報を保存しない。
- 自動適用を初期状態で有効化しない。
- 機器別の充電制御を根本的に作り直さない。
- 機器マスターを使わずに `.env` の固定台数で制御しない。

## 現在の状態

- `nightChargePlan.estimatedPvKwh` は日射量から推定したPV発電量。
- `nightChargePlan.estimatedPvToBatteryKwh` はPVから蓄電へ回る見込み。
- `nightChargePlan.consumptionSource` は `ecoflow-output` の場合、`power_logs.battery_output_w` 由来。
- DELTA 3 Plusの現在値は取得できているが、機器別の日中負荷実績としてはまだ弱い。
- 深夜充電プランには機器別配分の型があるが、画面やJSON上で見えない場合がある。
- DELTA Pro 3 の日中負荷は既存の `power_logs.battery_output_w` 履歴を使える。
- DELTA 3 Plus の日中負荷は実績ログが薄いため、初期実装では機器マスターの想定負荷WとPV有効時間から推定する。

## データ設計

### settings

既存の設定保存に以下を追加する。

| 項目 | 型 | 既定値 | 説明 |
| --- | --- | --- | --- |
| `pv_charge_correction_factor` | REAL | `0.7` | PV発電予測に掛ける手動補正係数 |
| `pv_charge_correction_manual` | INTEGER | `0` | ユーザーが明示保存した補正係数かどうか |
| `pv_charge_correction_updated_at` | TEXT | `NULL` | 補正係数を最後に手動保存した時刻 |
| `pv_charge_correction_min_sample_days` | INTEGER | `7` | 推奨値算出に必要な最小日数 |
| `pv_charge_correction_min_factor` | REAL | `0.2` | 補正係数の下限 |
| `pv_charge_correction_max_factor` | REAL | `0.9` | 補正係数の上限 |

自動適用設定は今回追加しない。将来、自動適用の仕様、上限変化幅、監査ログ、停止条件を別途レビューした後に追加する。

`pv_charge_correction_manual=0` の場合は、`pv_charge_correction_factor=0.7` でも組み込み既定値として扱い、`pvChargeCorrectionSource=default` を返す。UIから明示保存された場合だけ `pv_charge_correction_manual=1` と `pv_charge_correction_updated_at` を更新し、同じ0.7でも `pvChargeCorrectionSource=manual` として扱う。

### night_charge_plan_logs

補正係数は `requiredNightChargeKwh` の算出結果を変えるため、夜間制御判断ログにも入力値を保存する。

| 項目 | 型 | 説明 |
| --- | --- | --- |
| `pv_charge_correction_factor` | REAL | 判断時点で使った補正係数 |
| `pv_charge_correction_source` | TEXT | `default` / `manual` |
| `corrected_estimated_pv_kwh` | REAL | 補正後PV発電見込み |
| `corrected_estimated_pv_to_battery_kwh` | REAL | 補正後PV充電見込み |
| `total_daytime_required_kwh` | REAL | 全機器の日中必要量 |
| `total_available_kwh` | REAL | 全機器の現在利用可能量 |
| `total_deficit_kwh` | REAL | 全機器の不足量 |

これにより、後から「なぜその深夜充電目標になったか」を補正係数、補正元、PV見込み、全機器不足量と合わせて再現できるようにする。

### 充電機器マスター

既存の機器マスターを深夜計画の入力として使う。初期実装で使う主な項目は以下。

| 項目 | 用途 |
| --- | --- |
| `priority` | PV割当と深夜充電対象の優先順位 |
| `capacity_wh` | 現在SOCから利用可能量と目標SOCを計算する。計算時にkWhへ変換する |
| `backup_reserve_min_soc` | システムが下げてもよいリザーブ下限。日中に使える最低残量 |
| `backup_reserve_max_soc` | システムが上げてもよいリザーブ上限。深夜充電の最大目標 |
| `expected_daytime_load_w` | 新規追加。実績が弱い機器の想定日中負荷。DELTA 3 Plus は初期値400Wを想定 |
| `max_charge_w` | 深夜充電上限や実行可能性の表示に使う |
| `control_enabled` | 実機write候補にするかを判定する |

`expected_daytime_load_w` は `charging_devices` に追加する。既存DBでは `ALTER TABLE charging_devices ADD COLUMN expected_daytime_load_w INTEGER NOT NULL DEFAULT 0` を行い、DELTA 3 Plus の初期値はマスター編集画面から設定する。自動で全機器へ400Wを入れると誤制御になるため、マイグレーションでは既定0Wのままにする。

### 日次評価ログ

新規テーブル `pv_charge_correction_daily_logs` を追加する。

| 項目 | 説明 |
| --- | --- |
| `date` | 評価対象日 |
| `forecast_pv_kwh` | 当日のPV発電予測 |
| `forecast_pv_to_battery_kwh` | 当初のPV充電見込み |
| `actual_battery_input_kwh` | PV有効時間帯の主機と補助機器の実質充電量合計 |
| `actual_export_kwh` | Nature Remo電力量ログがあれば売電実績、なければ `power_logs.export_w` から概算 |
| `actual_capture_factor` | `actual_battery_input_kwh / forecast_pv_kwh` |
| `weather_code` | 予報天気コード |
| `cloud_cover_mean_percent` | 雲量 |
| `sample_quality` | `ok` / `insufficient-data` / `outlier` |
| `created_at`, `updated_at` | 監査用 |

既存ログだけで日次評価を再計算できるようにし、実機writeとは独立させる。同じ評価対象日を再生成しても二重計上しないように、`date` は `UNIQUE` とし、生成処理は `INSERT ... ON CONFLICT(date) DO UPDATE` 相当の upsert で上書きする。これにより `okSampleDays` や `actual_capture_factor` 平均が同じ日で重複しないようにする。

`power_logs.battery_input_w` は主機側の入力であり、DELTA 3 Plus のAC入力は現在の機器ステータスには出ているが永続化されていない。そのため、PV捕捉量を全機器で評価するには、機器別の入出力履歴も保存する必要がある。

新規テーブル `charging_device_power_logs` を追加する。

| 項目 | 説明 |
| --- | --- |
| `device_id` | 充電機器マスターID |
| `measured_at` | 取得時刻 |
| `soc` | 取得できたSOC |
| `ac_input_w` | 機器のAC入力W |
| `ac_output_w` | 機器のAC出力W |
| `ac_charge_limit_w` | AC充電上限 |
| `backup_reserve_soc` | 本体リザーブ残量 |
| `status_source` | 取得経路 |
| `available` | 取得できたか |
| `error_message` | 取得失敗理由 |

主機の `power_logs.battery_input_w` は後方互換のため残すが、補正係数の `actual_battery_input_kwh` は、原則として機器別の実質充電量を使う。DELTA 3 Plus のようにAC入力とAC出力が同時に出る機器では、`charging_device_power_logs.ac_input_w` をそのまま加算しない。基本式は `max(0, ac_input_w - abs(ac_output_w))` とし、SOC増分からより確実に蓄電量を推定できる場合はSOC増分を優先する。`ac_input_w` と `abs(ac_output_w)` が近いパススルー状態は、バッテリー充電実績ではなく負荷供給として扱い、PV捕捉量から除外する。

移行直後など補助機器履歴が不足する日、またはパススルーが多く実質充電量を判定できない日は `sample_quality=insufficient-data` にし、主機だけの値や生のAC入力値で全機器向け係数を出さない。

実充電量の集計では、深夜充電や買電によるAC充電をPV捕捉量に混ぜない。対象は原則として当日のPV有効時間帯に限定し、以下のどちらかを満たすサンプルだけを採用する。

- `export_w > 0` で売電が発生している。
- `import_w == 0` かつ PV有効時間帯内で、深夜充電コマンドや買電回復コマンドの影響がない。

夜間充電ウィンドウ、買電中の充電、直近に充電制御コマンドが実行された区間は `sample_quality=insufficient-data` または除外として扱う。これにより `actual_capture_factor` が深夜電力や買電充電で過大評価されることを防ぐ。

## 補正係数の算出

1. 直近N日から `sample_quality=ok` の日を抽出する。
2. PV有効時間帯かつ非買電/売電寄りのサンプルから `actual_capture_factor` を計算する。
3. 夜間充電、買電充電、制御コマンド直後の区間を除外する。
4. 0未満、1超過、予測PVが極端に少ない日を除外する。
5. 中央値または外れ値除外平均を推奨値にする。
6. `min_factor` から `max_factor` の範囲に丸める。

初期実装では、単純で説明しやすい以下を使う。ただし `sample_quality=ok` の日数が `pv_charge_correction_min_sample_days` 未満の場合は、推奨補正係数を適用不可として返し、UIの「推奨値を適用」ボタンも無効にする。

```text
if okSampleDays < minSampleDays:
  recommendation.status = "insufficient-samples"
  recommendation.applicable = false
else:
  推奨補正係数 = clamp(直近OK日の actual_capture_factor 平均, min_factor, max_factor)
  recommendation.applicable = true
```

## 機器別日中負荷とPV配分

深夜充電プランでは、全体の不足量だけでなく機器別に以下を計算する。

### 機器ごとの日中必要量

| 機器 | 算出方法 |
| --- | --- |
| DELTA Pro 3 | 既存どおり `power_logs.battery_output_w` の履歴から推定する |
| DELTA 3 Plus | 初期実装では機器マスターの `expected_daytime_load_w` とPV有効時間から推定する。例: `400W * 7h = 2.8kWh` |

将来、DELTA 3 Plus の実績ログが安定したら、機器ごとのAC出力ログを優先し、マスター値はフォールバックにする。

### 現在利用可能量

```text
利用可能kWh = (capacity_wh / 1000) * max(0, (current_soc - backup_reserve_min_soc) / 100)
```

`backup_reserve_min_soc` は機器本体の最低放電残量ではなく、このシステムが放電方向に制御してよい下限として扱う。`backup_reserve_max_soc` は深夜充電で引き上げてもよい上限として扱う。

### 不足量とPV割当

```text
日中不足kWh = max(0, 日中必要量kWh - 現在利用可能量kWh)
補正後PV充電見込みkWh = estimatedPvKwh * pvChargeCorrectionFactor
```

既存の `morningToPvStartLoadKwh` は、PV有効時間が始まる前に必要な電力量としてPV割当前に確保する。朝7時以降からPV開始までの負荷は、日中のPVでは後から補えないため、補正後PV充電見込みから差し引かない。

さらに既存 planner が確保している `estimatedMorningLoadKwh` と `safetyMarginKwh` も、PV割当前に必ず深夜充電側へ積む。これは深夜中から7時までの残り負荷と安全マージンであり、翌日のPVで相殺してはいけない。

```text
深夜残り確保kWh = estimatedMorningLoadKwh + safetyMarginKwh
朝PV開始前不足kWh = max(0, morningToPvStartLoadKwh - 現在利用可能量kWh)
PVで相殺可能な不足kWh = max(0, 日中不足kWh - 朝PV開始前不足kWh)
深夜で必ず確保するkWh = 深夜残り確保kWh + 朝PV開始前不足kWh
```

補正後PV充電見込みは、全機器用の充電余力として扱う。推定余剰が `1.2kWh` の場合も、DELTA Pro 3 だけでなく全機器で共有する値として扱う。

1. 機器マスターの優先順位順に不足量を並べる。
2. 深夜残り負荷と安全マージンを先に深夜充電必要量へ積む。
3. 各機器の朝PV開始前不足を先に深夜充電必要量へ積む。
4. 補正後PV充電見込みを、PVで相殺可能な不足へ割り当てる。
5. 深夜残り確保、朝PV開始前不足、PV割当後も残る不足を深夜充電必要量にする。
6. 深夜充電必要量から機器ごとの目標SOCを計算し、`backup_reserve_max_soc` を超えないよう丸める。

### 機器別深夜充電目標

| 機器 | 制御方針 |
| --- | --- |
| DELTA Pro 3 | 既存のTOU/セルフパワー/バックアップリザーブ制御を使う |
| DELTA 3 Plus | AC充電上限とバックアップリザーブ範囲で目標SOCまで充電する |

実機writeは既存の安全ゲート、最小コマンド間隔、`ENABLE_REAL_CONTROL=true`、`SIMULATION_MODE=false` を通った場合だけ候補にする。

## 深夜充電プランへの反映

`nightChargePlan` のPV見込みに以下を追加する。

- `pvChargeCorrectionFactor`
- `correctedEstimatedPvKwh`
- `correctedEstimatedPvToBatteryKwh`
- `pvChargeCorrectionSource`
  - `manual`
  - `default`
- `pvChargeCorrectionRecommendation`
  - `recommendedFactor`
  - `okSampleDays`
  - `minSampleDays`
  - `applicable`
  - `status`
- `totalDaytimeRequiredKwh`
- `totalAvailableKwh`
- `totalDeficitKwh`
- 既存の JSON `requiredNightChargeKwh` を補正後の深夜必要充電量として更新する。Go構造体名は既存どおり `RequiredNightChargeKWh` を使う。別名の `nightRequiredChargeKwh` は追加しない。
- 既存の `devicePlans[]` を拡張する
  - `deviceId`
  - `name`
  - `priority`
  - `currentSoc`
  - `daytimeRequiredKwh`
  - `availableKwh`
  - `pvAllocatedKwh`
  - PV割当後に残る機器別の深夜必要充電量は、既存 JSON `requiredChargeKwh` を更新する。Go構造体名は既存どおり `RequiredChargeKWh` を使う。別名の `nightRequiredKwh` は追加しない。
  - 補正後の機器別目標は既存 JSON `recommendedTargetSoc` / `recommendedTargetKwh` を更新する。`targetSoc` は機器マスターの設定値として残し、補正後目標として上書きしない。
  - `reason`

深夜充電計算では、従来の `estimatedPvKwh` を表示用に残し、制御判断には補正後の値を使う。

```text
制御用PV見込み = estimatedPvKwh * pvChargeCorrectionFactor
```

## API

### GET `/api/status`

既存の `nightChargePlan` に補正情報を追加する。

### GET `/api/pv-charge-correction`

以下を返す。

- 現在設定値
- 推奨補正係数
- サンプル日数
- 日次評価ログ
- 補正前PV予測と補正後PV充電見込み
- 機器別の日中必要量、PV割当、既存 `requiredChargeKwh` に反映された深夜必要量

### POST `/api/pv-charge-correction`

手動で設定値を更新する。

初期実装では、UIから明示的に保存した場合のみ更新する。自動適用は設定項目もUIも追加しない。

POSTされた補正係数は `pv_charge_correction_min_factor` から `pv_charge_correction_max_factor` の範囲で検証する。範囲外、NaN、無限大、数値でない入力は保存せず `400 Bad Request` を返す。保存に成功した場合だけ `pv_charge_correction_manual=1` と `pv_charge_correction_updated_at` を更新し、変更履歴ログを残す。

## UI

設定ブロックに「PV充電補正」を追加する。

表示内容:

- 現在の補正係数
- 推奨補正係数
- 直近サンプル日数
- 補正前PV予測
- 補正後PV充電見込み
- 実充電量平均
- 「推奨値を適用」ボタン
- 全体の日中総必要量
- 全機器の利用可能量
- 全機器不足量
- 既存 `requiredNightChargeKwh` に反映された深夜必要充電量
- 機器別の現在SOC、日中必要量、利用可能量、PV割当、既存 `requiredChargeKwh` に反映された深夜必要量、推奨目標SOC、不足理由

自動適用は今回表示しない。将来追加する場合は、別計画で安全条件と停止条件をレビューする。

## 安全境界

- 実機write条件は変更しない。
- 補正係数は深夜充電の目標計算にのみ影響する。
- 補正係数の変更履歴をログに残す。
- 初期値は保守的に `0.7` とし、予測PV全量を充電可能とは扱わない。
- サンプル不足時は推奨値を適用しない。
- DELTA 3 Plus の想定負荷Wは実績不足時のフォールバックであり、実績ログが取れたら実績優先に切り替えられる構造にする。
- 実機write候補は機器別目標を計算したあとも、既存の制御ゲート、コマンド間隔、405/AC出力OFF抑止条件を維持する。

## 実装ステップ

1. `domain` にPV充電補正設定、推奨値、日次ログ型を追加する。
2. SQLiteマイグレーションで設定項目と日次ログテーブルを追加する。
3. `charging_device_power_logs` を追加し、DELTA Pro 3 / DELTA 3 Plus の機器別SOC、AC入力、AC出力、AC充電上限を保存する。
4. `night_charge_plan_logs` に補正係数、補正元、補正後PV見込み、全機器必要量/利用可能量/不足量を保存する。
5. `power_logs`、`charging_device_power_logs`、既存の夜間充電プランログから日次評価ログを生成する repository/service を追加する。
6. `charging_devices` に `expected_daytime_load_w` を追加し、domain/repository/API/UIで編集・表示できるようにする。
7. 機器マスターから優先順位、`capacity_wh`、`backup_reserve_min_soc`、`backup_reserve_max_soc`、想定日中負荷Wを読み込み、機器別日中必要量を計算する。
8. DELTA Pro 3は既存の `power_logs.battery_output_w` 履歴、DELTA 3 Plusは初期実装では機器マスターの想定負荷Wを使う。
9. 深夜残り負荷、安全マージン、朝PV開始前不足をPV割当前に深夜充電必要量として確保する。
10. 補正後PV充電見込みを全機器に優先順位順で配分し、既存の `devicePlans` に機器別のPV割当、既存 `requiredChargeKwh` で表す深夜必要量、目標SOCを追加する。
11. 深夜充電プランに補正係数と機器別計画を入力し、制御用PV見込みを補正後に差し替える。
12. `/api/pv-charge-correction` のGET/POSTを追加する。
13. frontendの型、APIクライアント、設定UI、深夜充電プラン表示を更新する。
14. 単体テストを追加する。
15. build/test/reviewを実施する。

## レビューポイント

- 予測PV全量を充電可能として扱っていないか。
- サンプル不足時に誤った推奨値を出していないか。
- 深夜充電や買電充電がPV捕捉量として混ざっていないか。
- DELTA 3 Plus の実質充電量が補正係数の実績値から抜けていないか。
- DELTA 3 Plus のパススルーAC入力をバッテリー充電実績として加算していないか。
- 自動適用が初期実装に紛れ込んでいないか。
- 実機writeゲートが緩んでいないか。
- UIの「推奨値」と「現在設定値」が混ざって見えないか。
- DELTA Pro 3 / DELTA 3 Plus の制御対象が機器マスター前提から逸脱していないか。
- 推定PV余剰をDELTA Pro 3専用として扱っていないか。
- DELTA 3 Plus の400W級日中負荷が不足量に反映されているか。
- 深夜残り負荷、安全マージン、朝PV開始前不足がPV割当前に確保されているか。
- 目標SOCが `backup_reserve_min_soc` と `backup_reserve_max_soc` の範囲内に丸められているか。
- 既存の `devicePlans` を拡張しており、別名フィールドでUI表示が分裂していないか。
- 夜間制御ログに補正係数、補正元、補正後PV見込みが保存され、後から再現できるか。

## 確認コマンド

```sh
cd backend && rtk go test ./...
cd frontend && rtk npm run build
rtk git diff --check
rtk codex review --uncommitted
```

## 運用メモ

補正係数は、最初は手動で様子を見る。最低7日分のログが取れ、推奨値が安定してから自動適用を検討する。自動適用を追加する場合は、急激な変動を避けるため1日あたりの変更幅を制限し、別途レビューを通す。

DELTA 3 Plus の想定日中負荷は、初期値400Wを目安にする。ただし実際の接続機器や稼働状態で変わるため、機器マスターで変更できるようにし、将来は機器別AC出力ログの実績を優先できるようにする。
