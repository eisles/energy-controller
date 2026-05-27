# DELTA Pro 3 買電時リザーブ下げ制御 実装計画

## 目的

買電中に DELTA Pro 3 の残量がシステムの制御下限より十分高い場合、バックアップリザーブを機器マスターの下限まで下げて放電を許可する。

今回の実例では、機器マスターのリザーブ制御範囲が `10-90%`、DELTA Pro 3 の現在残量が `16%`、本体バックアップリザーブが `15%` なので、買電中は `15% -> 10%` のリザーブ下げ候補を出せるようにする。

## 非目的

- EcoFlow の新しい未検証 API を追加しない。
- 実機 write の安全ゲートを緩めない。
- DELTA 3 Plus の補助制御方針は変更しない。
- 夜間充電計画や天気予測ロジックは変更しない。

## 現状

- `PlanSurplusCharging` は買電時に AC 充電上限を下げる。
- バックアップリザーブは `DefaultReserveSoc` より高い場合だけ下げる。
- `DefaultReserveSoc` は「通常復帰値」として扱われており、機器マスターの下限とは別概念。
- そのため `backupReserveSoc=15`、`DefaultReserveSoc=30` のような状態では、リザーブを `10%` へ下げる候補が出ない。

## データ/API 契約

- `charging_devices.reserve_soc` または `backup_reserve_min_soc` を、DELTA Pro 3 の「制御下限残量」として使う。
- `charging_devices.target_soc` または `backup_reserve_max_soc` は本件では変更しない。
- `/api/status.surplusPlan` の既存フィールドを使う。
  - `recommendedBackupReserveSoc`
  - `shouldLowerBackupReserve`
  - `wouldWrite`
  - `reason`
  - `actionSummary`

## 安全境界

- 実機 write は既存の `ENABLE_REAL_CONTROL=true`、`SIMULATION_MODE=false`、`AUTO_CONTROL_ENABLED=true`、`CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`、実制御期限内の条件を維持する。
- 同一指令の重複抑止、最小コマンド間隔、エラー後リトライ抑止は維持する。
- 残量が制御下限以下の場合は、リザーブ下げを行わない。
- 現在の本体バックアップリザーブが制御下限以下の場合は、上げ戻しを行わない。

## 実装方針

1. `SurplusPlanInput` に `MinDischargeReserveSoc` を追加する。
2. 買電時の `RECOVERING` 分岐で、復帰先を次のように決める。
   - `MinDischargeReserveSoc > 0` なら制御下限として優先。
   - 未設定なら従来どおり `DefaultReserveSoc` を使う。
3. 買電中は、次の条件を満たす場合だけバックアップリザーブを下げる。
   - `BackupReserveSoc != nil`
   - `BatterySoc > recoveryReserve`
   - `BackupReserveSoc > recoveryReserve`
4. 実機マスターの DELTA Pro 3 設定を `SurplusPlanInput.MinDischargeReserveSoc` に渡す。
   - `backup_reserve_min_soc` を優先。
   - なければ `reserve_soc` を使う。
5. テストを追加する。
   - `SOC 16% / backupReserve 15% / minReserve 10% / 買電` で `10%` へ下げる。
   - `SOC 10% / backupReserve 15% / minReserve 10%` では下げない。
   - `backupReserve 5% / minReserve 10%` では上げ戻さない。

## 変更予定ファイル

- `backend/internal/control/surplus_planner.go`
- `backend/internal/control/surplus_planner_test.go`
- `backend/cmd/server/main.go`
- `backend/cmd/server/main_test.go`

## レビュー観点

- 買電時にリザーブを上げ戻して放電を妨げないこと。
- SOC が制御下限以下のときにリザーブを下げないこと。
- 既存の売電時充電、パススルー、夜間充電、DELTA 3 Plus 補助制御へ影響しないこと。
- 実機 write の安全ゲートと重複抑止が維持されていること。

## 検証

```sh
cd backend && rtk go test ./...
cd frontend && rtk npm run build
rtk codex review --uncommitted
```

## 運用メモ

実装後、現在値が `SOC 16% / backupReserve 15% / control min 10% / 買電` に近い場合は、`surplus_control_command_logs` に `target_backup_reserve_soc=10` の候補が出る。実送信されるかは既存の安全ゲート、重複抑止、最小間隔に従う。
