# 売電中の夜間充電機器別復旧候補抑制 実装計画

## 目的

売電中または余剰充電候補が出ているときに、夜間充電の機器別割当が DELTA Pro 3 の復旧候補を再生成して、余剰充電制御を上書きしないようにする。

現在の実稼働確認では、`gridW` がマイナスで売電しており、`surplusPlan` は `AC充電上限を1000Wへ設定` を推奨している。一方で `nightChargePlan` は機器別同期後に `self-powered=on|ac=400|reserve=62` の候補を持ち、`controlDiagnostics.pro3.controlSource` が `night_charge_plan` になっている。これにより、売電吸収したい局面で夜間充電側が制御所有権を持つ。

## 非目的

- 実機 write 条件、`ENABLE_REAL_CONTROL`、`SIMULATION_MODE`、既存の安全ゲートは変更しない。
- EcoFlow private MQTT のペイロード、認証、接続間隔は変更しない。
- DELTA 3 Max Plus / DELTA 3 Plus 2 の優先順位や UI 表示は変更しない。
- Docker 再起動や実稼働反映は、この計画の実装範囲には含めない。

## 現状整理

- `PlanNightCharging` は `GridW > 0` のときだけ料金帯放電を有効にする修正済み。
- `NightChargePlan` のログには `GridW` / `ImportW` / `ExportW` が残るが、`ApplyNightChargeDevicePlans` の入力である `NightChargeDeviceInput` には現在の売買電状態がない。
- `syncPrimaryPro3NightRecoveryPlan` は、機器別計画で `ShouldCharge=false` かつ Pro3 が write target の場合に、`self-powered`、AC充電上限の停止フロア、バックアップリザーブ同期候補を作る。
- そのため、ベースプランが売電中に `observe` を推奨しても、後段の機器別復旧候補が再度 write 候補を作れる。

## 変更方針

1. `NightChargeDeviceWriteGuard` に売電中かどうかを示す読み取り専用の制御コンテキストを追加する。
   - 候補名は `GridW int`、またはより意図が明確な `Exporting bool` とする。
   - 既存テストの初期値で挙動が変わらないよう、ゼロ値は「売電中ではない」と扱う。
2. 呼び出し元で現在の `status.GridW` を guard に渡す。
   - 既に `NightChargePlanLog` に `GridW` は保存されているため、同じ source of truth を使う。
3. `syncPrimaryPro3NightChargePlan` または `syncPrimaryPro3NightRecoveryPlan` で、売電中の Pro3 復旧候補を抑制する。
   - 抑制対象は `RecommendedMode="self-powered"`、AC充電上限を停止フロアへ下げる候補、リザーブ同期候補。
   - 高単価/中間単価の買電中放電 `self-powered-discharge` は `GridW > 0` を前提とするため影響させない。
   - 夜間低単価の通常充電候補 `ShouldCharge=true` は影響させない。
4. 抑制時の `CommandBlockReason` と `ActionSummary` は、余剰充電側が所有すべきことが分かる文言にする。
   - 例: `exporting; surplus plan owns DELTA Pro 3 charging recovery`
5. ユニットテストを追加する。
   - 売電中に Pro3 が目標到達済みでも、機器別復旧候補が `WouldWrite=false` になり、AC充電上限を下げないこと。
   - 非売電時の既存復旧候補テストはそのまま通ること。

## 変更予定ファイル

- `backend/internal/control/night_charge_device_planner.go`
- `backend/internal/control/night_charge_device_planner_test.go`
- 呼び出し元で guard を組み立てている backend ファイル

## 安全境界

- 実機 write の有効化条件は変更しない。
- 変更は planner の候補生成抑制であり、新しい write 経路は追加しない。
- 売電中の Pro3 余剰充電は既存 `surplusPlan` に任せる。
- 買電中の料金帯放電と深夜低単価充電は既存条件を維持する。

## 検証

```bash
cd backend && GOCACHE=$PWD/.gocache rtk go test ./internal/control
cd backend && GOCACHE=$PWD/.gocache rtk go test ./...
rtk git diff --check
rtk codex review --uncommitted
```

## ロールバック

問題があれば、この変更で追加した guard の売電判定と復旧候補抑制テストを戻す。既存の実機 write ゲートや adapter には触れないため、ロールバック範囲は control planner に限定される。
