# DELTA 3 Plus バックアップリザーブ未反映リカバリ実装計画

## 目的

DELTA 3 Plus 2 の買電時制御で、バックアップリザーブ変更が実機に反映されないまま同じ指示を抑止し続ける状態を解消する。

現状は、買電時に「放電させる」意図で `backup reserve 20%` を設定しようとしているが、実機の読み取り値は `backupReserveEnabled=false` / `backupReserveSoc=0` のままになっている。そのため、直近コマンドが未反映と判定され、以後の制御候補が `previous backup reserve command was not reflected by device` で止まり続けている。

## 非目的

- 実機 write の安全ゲートを緩めない。
- DELTA 3 Plus 1 台目を制御対象に戻さない。
- AC 出力 ON/OFF の新規 write payload は追加しない。
- 未検証の private MQTT command を増やさない。
- `.env` の既定値を実制御有効側へ変更しない。

## 現状

- `PlanDelta3AuxCharging` は買電時に `maybeSetDelta3DischargeReserve` を呼び、`BackupReserveMinSoc` へバックアップリザーブを設定する。
- executor は `ShouldSetBackupReserve` を `SetEnergyBackupEnabled(true, target)` として送信する。
- 実機ステータスでは `backupReserveEnabled=false` / `backupReserveSoc=0` のままなので、`annotateDelta3AuxBackupReserveApplyState` が failed/stale と判定する。
- guard は同じ target の再送を抑止し、制御が停止したように見える。

## 変更方針

買電時の放電リカバリでは、バックアップリザーブを「有効化」しない。

理由:

- バックアップリザーブ有効化は、機種やモードによってパススルー/充電寄りに解釈される可能性がある。
- 買電を減らす目的では、少なくとも現在の `backupReserveEnabled=false` は放電を妨げる状態ではない。
- 反映されない `energy_backup_enabled=true` を繰り返し候補にすると、実際に有効な AC 充電上限調整や今後の制御をブロックする。

## 実装内容

### 1. 買電時のバックアップリザーブ有効化を抑止

`maybeSetDelta3DischargeReserve` を修正する。

- `backupReserveEnabled=false` の場合は、放電阻害要因ではないため `ShouldSetBackupReserve=false` のままにする。
- `backupReserveEnabled=true` かつ `backupReserveSoc > BackupReserveMinSoc` の場合だけ、下げる/解除する候補を出す。
- `backupReserveSoc=0` で disabled の状態を「未反映で再試行すべき状態」と扱わない。

### 2. 未反映判定の扱いを改善

`annotateDelta3AuxBackupReserveApplyState` を修正する。

- 前回 `ShouldSetBackupReserve` が「買電リカバリで master min へ下げる」目的だった場合に限り、`backupReserveEnabled=false` / `backupReserveSoc=0` が返っている状態を failed ではなく `ignored` として扱う。
- `ignored` は「放電リカバリ目的の energy backup reserve command は実機が無視した、またはこのモードでは不要だった可能性がある」という診断状態に限定する。
- 余剰充電時の reserve 引き上げ command は従来通り failed/stale として扱い、未反映抑止を維持する。
- `delta3AuxBackupReserveCommandUnreflected` は failed/stale のみ対象のまま維持するが、`ignored` になった command と同じ reserve target の再送は、買電リカバリ文脈では planner 側で出さない。

### 3. reason を明確化

買電時にバックアップリザーブ操作をしない場合、plan reason を以下の方向に変える。

- `backup reserve is already disabled; use AC charge limit reduction and existing discharge behavior`
- 日本語 UI 側の翻訳対象があれば、表示上は「本体リザーブ無効のため、追加のリザーブ操作は行わない」と分かるようにする。

### 4. テスト追加

backend unit test を追加/更新する。

- backup reserve disabled + reserve 0 の状態では買電時に `ShouldSetBackupReserve=false` になる。
- backup reserve enabled + reserve 80 の状態では買電時に master min へ下げる候補が出る。
- 前回の買電リカバリ reserve command 後に disabled + 0 が返った場合、apply state は `ignored` になり、未反映抑止で制御全体を止めない。
- 前回の余剰吸収 reserve 引き上げ command 後に disabled + 0 が返った場合は、従来通り failed/stale になり、再送抑止が効く。

## 安全境界

- 実機 write 条件は既存通り維持する。
  - `ENABLE_REAL_CONTROL=true`
  - `SIMULATION_MODE=false`
  - `MOCK_MODE=false`
  - `AUTO_CONTROL_ENABLED=true`
  - 実制御期限が有効
  - `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`
  - `ECOFLOW_DELTA3_READ_ENABLED=true`
  - `ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=true`
  - `ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=true`
  - `ECOFLOW_DELTA3_EXECUTE=true`
  - 機器マスターで対象機器が enabled/controlEnabled
- この修正では新規 write command を追加しない。
- 既存の最小コマンド間隔、重複抑止、AC 充電上限の安全計算は維持する。
- 実機 write は追加実行しない。検証は unit test / build / review で行う。

## 変更予定ファイル

- `backend/internal/control/auxiliary_battery_planner.go`
- `backend/internal/control/auxiliary_battery_executor.go`
- `backend/internal/control/auxiliary_battery_planner_test.go`
- `backend/internal/control/auxiliary_battery_executor_test.go`
- 必要に応じて frontend の表示文言

## 検証コマンド

- `cd backend && rtk go test ./...`
- `cd frontend && rtk npm run build`
- `rtk git diff --check`
- `rtk codex review --uncommitted`

## ロールバック

- planner/executor の条件分岐とテスト追加のみなので、問題があれば本差分を戻す。
- DB migration や実機設定変更は含めないため、運用データの rollback は不要。
