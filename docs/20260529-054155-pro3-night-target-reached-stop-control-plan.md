# DELTA Pro 3 目標到達後の深夜充電停止制御 実装計画

## 目的

深夜充電プランが「現在残量は目標到達済みなので充電不要」と判定しているにもかかわらず、DELTA Pro 3 が TOU モードと高い AC 充電上限のまま充電し続ける状態を修正する。

現在の問題は、`PlanNightCharging` が self-powered へ戻す候補を作っても、複数機器向けの `syncPrimaryPro3NightChargePlan` が DELTA Pro 3 の機器別 `ShouldCharge=false` を見た時点で write 候補をすべて消している点にある。そのため、画面上は「充電不要」と見えるが、実機へ「充電を止める/抑制する」候補が残らない。

## 非目的

- 実機 write の安全ゲートを緩めない。
- `ENABLE_REAL_CONTROL` や `.env` の既定値を実制御側に変更しない。
- DELTA 3 Plus の制御方針や AC 出力 ON/OFF 制御を変更しない。
- 未検証の EcoFlow private MQTT payload を追加しない。
- 深夜充電プラン全体の需要予測ロジックを作り直さない。

## 現状整理

- `applyNightModeRecommendation` は、現在エネルギーが推奨夜間目標を超えている場合に `RecommendedMode="self-powered"` と `ShouldEnableSelfPoweredMode=true` を作れる。
- `applyNightChargeCommandPlan` は self-powered 推奨時に `RecommendedBackupReserveSoc` と `ShouldSetBackupReserve` を作れる。
- その後 `ApplyNightChargeDevicePlans` が機器別配分を行い、DELTA Pro 3 の機器別目標に同期する。
- `syncPrimaryPro3NightChargePlan` は `devicePlan.ShouldCharge=false` の場合、`ShouldEnableSelfPoweredMode`、`ShouldSetACChargeLimit`、`ShouldSetBackupReserve`、`WouldWrite` を false に戻している。
- この上書きにより、TOU モード継続中や AC 充電上限 1500W の状態でも「止めるための write 候補」が消える。

## 変更方針

DELTA Pro 3 の機器別目標に到達済みでも、実機状態が深夜充電停止向けの状態になっていない場合は write 候補を残す。

具体的には以下の状態を「停止・復帰候補が必要」とみなす。

- 現在 TOU モードが有効、または self-powered が明示的に無効。
- 現在の AC 充電上限が機器ごとの安全な最小値より高い。
- 現在のバックアップリザーブが推奨目標 SOC と異なる。

この場合、深夜充電プランは `ShouldChargeTonight=false` のまま、以下の候補を作る。

- `RecommendedMode="self-powered"`
- `ShouldEnableSelfPoweredMode=true` when current self-powered is known false, or current TOU mode is still true
- `RecommendedBackupReserveSoc=<DELTA Pro 3 の機器別推奨目標 SOC>`
- `ShouldSetBackupReserve=true` when current reserve differs
- `RecommendedACChargeLimitW=<device.MinChargeW if set, otherwise settings.MinChargeW>`
- `ShouldSetACChargeLimit=true` only when current AC charge limit is above the stop floor by `MinCommandDiffW` or more

安全ゲート、夜間ウィンドウ、最小コマンド間隔、trial 期限、確認文字列などは既存通り維持する。

## 実装内容

### 1. DELTA Pro 3 実機モードを機器別入力へ渡す

`NightChargeDeviceInput` に以下を追加する。

- `CurrentTOUModeEnabled *bool`
- `CurrentSelfPoweredEnabled *bool`

`backend/cmd/server/main.go` の `nightChargeDeviceInputs` で `DeviceStatusResponse.Status` から値を渡す。

### 2. 目標到達済み分岐を停止候補生成へ変更

`syncPrimaryPro3NightChargePlan` の `!devicePlan.ShouldCharge` 分岐を置き換える。

- まず `ShouldChargeTonight=false` と機器別推奨目標への同期は維持する。
- 機器別 plan に `BlockReason` がある場合は、復帰候補を作らず `WouldWrite=false` のまま block reason を表示する。
- 充電停止候補が必要な場合は、self-powered / reserve / AC charge limit の候補を作る。
- self-powered write は `CurrentSelfPoweredEnabled == false`、または `CurrentTOUModeEnabled == true` の場合に出す。状態が取得できない nil は「OFF」とは扱わず、既存の `SetSelfPoweredMode(true)` payload で TOU を false にできる状態だけ明示的に解消する。
- AC charge limit の停止床は機器マスターの `MinChargeW` を優先し、未設定の場合だけ `settings.MinChargeW` を使う。
- AC charge limit は「現在値が停止床より高い」場合だけ下げる候補にする。現在値が停止床以下の場合は、充電抑制目的で上げる write は行わない。
- 候補がない場合だけ `WouldWrite=false` とし、理由を `night charge settings already match allocated DELTA Pro 3 recovery plan` のようにする。
- 候補がある場合は、既存と同じ順序で以下を判定する。
  - 夜間/リカバリ window 内か。
  - 実制御 gate が開いているか。
  - DELTA Pro 3 固有の block reason がないか。
  - 問題なければ `WouldWrite=true`。

### 3. コマンド実行順序は既存を使う

`ExecuteNightChargeCommand` はすでに以下を実行できる。

- TOU disable
- AC charge limit set
- backup reserve set
- self-powered enable

今回の修正では executor の新規 write API は追加しない。

### 4. テスト追加

`backend/internal/control/night_charge_device_planner_test.go` に以下を追加する。

- DELTA Pro 3 が目標到達済みでも、TOU 有効/self-powered 明示無効/AC 上限高めなら停止候補が残る。
- DELTA Pro 3 が TOU 有効かつ self-powered 有効のように矛盾して読める状態でも、TOU を落とすために self-powered write 候補が残る。
- DELTA Pro 3 が目標到達済みで AC 上限が機器別停止床以下なら、AC 上限を上げる候補は出ない。
- 実機状態が self-powered、reserve、AC 上限とも一致済みなら write 候補は出ない。
- 機器マスターや状態取得で DELTA Pro 3 がブロックされている場合は、復帰 write 候補も出ない。
- 実制御 gate が閉じている場合は候補があっても `WouldWrite=false` で block reason が出る。

必要に応じて `backend/cmd/server/main_test.go` で `nightChargeDeviceInputs` のモード値引き渡しを確認する。

## 安全境界

- real write は引き続き以下を満たす場合のみ。
  - `ENABLE_REAL_CONTROL=true`
  - `SIMULATION_MODE=false`
  - `MOCK_MODE=false`
  - `AUTO_CONTROL_ENABLED=true`
  - real control trial が有効期限内
  - `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`
  - 機器マスターで対象機器が enabled/controlEnabled
- デフォルトは mock/simulation を維持する。
- `.env`、API token、device serial number、secret は変更しない。
- 実機 write をこの作業中に追加実行しない。検証は unit test、build、review で行う。

## 変更予定ファイル

- `backend/internal/control/night_charge_device_planner.go`
- `backend/internal/control/night_charge_device_planner_test.go`
- `backend/cmd/server/main.go`
- 必要に応じて `backend/cmd/server/main_test.go`

## 検証コマンド

- `cd backend && rtk go test ./...`
- `rtk git diff --check`
- `rtk codex review --uncommitted`

## ロールバック

- DB migration は含めない。
- 問題があれば `NightChargeDeviceInput` の追加フィールドと `syncPrimaryPro3NightChargePlan` の目標到達済み分岐変更、関連テストを戻す。
