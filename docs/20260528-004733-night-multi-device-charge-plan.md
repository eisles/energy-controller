# 深夜充電 複数デバイス配分制御 実装計画

## Goal

深夜料金時間帯（23:00-07:00）に、翌日の天気予報・PV発電予測・EcoFlow出力ログから必要な蓄電量を見積もり、充電機器マスターに登録された複数デバイスへ目標充電量を配分する。

現在の夜間充電計画は主に DELTA Pro 3 単体を対象にしている。これを、DELTA Pro 3 / DELTA 3 Plus / 将来の補助デバイスを含む「充電機器全体の計画」として扱えるようにする。

## Non-goals

- SwitchBot スマートプラグ制御は実装しない。
- DELTA 3 Plus 2台目を自動制御ONへ勝手に変更しない。
- 新しい外部APIや認証方式は追加しない。
- `.env` の実機制御ゲートを弱めない。
- 実機writeの頻度制限、重複抑止、trial window、`CONFIRM_ECOFLOW_WRITE` を回避しない。
- 夜間以外の余剰追従制御は今回の主対象にしない。

## Current State

- `PlanNightCharging` は翌日のPV予測、日中負荷、朝までの消費を計算し、`NightChargePlan` を作る。
- `NightChargePlan` は DELTA Pro 3 の `BatterySoc` / `BatteryFullEnergyWh` を前提に、単一の `RecommendedNightTargetSoc` を出している。
- `charging_devices` には DELTA Pro 3 と DELTA 3 Plus が登録され、容量・優先順位・制御範囲を持っている。
- DELTA 3 Plus 1台目は private API で読取・一部制御ができる。
- DELTA 3 Plus 2台目は登録済みだが `control_enabled=0` なので自動制御対象外。
- `Delta3AuxPlan` は日中余剰・買電回復用で、夜間の全体充電配分とは分離されている。

## Safety Boundaries

- 既存の実機write条件を維持する。
  - `MOCK_MODE=false`
  - `SIMULATION_MODE=false`
  - `ENABLE_REAL_CONTROL=true`
  - `AUTO_CONTROL_ENABLED=true`
  - `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`
  - trial window active
- 計画対象に含めるのは `enabled=1` の充電機器のみ。
- 実際にwrite候補にするのは `control_enabled=1` かつ対応能力がある機器のみ。
- DELTA 3 Plus は、現時点では既存の `delta3_aux` writeゲートと duplicate/min interval を使う。
- `control_enabled=0` の機器は配分計画には「参考/未制御」として表示しても、write候補にはしない。
- SOC/AC上限/リザーブ値が読めない機器は、write候補から外し、理由を計画に残す。

## Data Model

### Add domain types

`backend/internal/domain/status.go`

- `NightChargeDevicePlan`
  - `deviceId`
  - `name`
  - `kind`
  - `priority`
  - `controlEnabled`
  - `capacityKWh`
  - `currentSoc`
  - `currentEnergyKWh`
  - `reserveSoc`
  - `targetSoc`
  - `minTargetSoc`
  - `maxTargetSoc`
  - `recommendedTargetSoc`
  - `recommendedTargetKWh`
  - `requiredChargeKWh`
  - `recommendedAcChargeLimitW`
  - `shouldCharge`
  - `wouldWrite`
  - `blockReason`
  - `dataSource`

### Extend `NightChargePlan`

- `totalDeviceCapacityKWh`
- `totalCurrentDeviceEnergyKWh`
- `totalRecommendedTargetKWh`
- `totalRequiredDeviceChargeKWh`
- `devicePlans []NightChargeDevicePlan`

既存フィールドは互換性維持のため残す。DELTA Pro 3 単体相当の既存表示・ログは壊さない。

## Planning Algorithm

1. 既存の `PlanNightCharging` で、翌日PV予測・日中負荷・朝まで消費・安全余力を計算する。
2. 充電機器マスターから有効な機器を取得する。
3. 各機器の容量・現在SOCを決定する。
   - DELTA Pro 3: 既存 `status` の SOC / capacity を利用。
   - DELTA 3 Plus: `api.Delta3StatusReader.CurrentDeviceStatuses` の結果を利用。
   - 読み取り不可機器: `blockReason` を付ける。
4. 総必要エネルギーを計算する。
   - ベースは既存 `NightChargePlan.RecommendedNightTargetKWh`
   - 「全機器の最低確保エネルギー + 朝まで消費 + PV不足 + 安全余力」を上限内で配分
5. 優先順位順に充電目標を配分する。
   - まず各機器の `reserveSoc` / `backupReserveMinSoc` まで確保
   - 追加必要分を priority 昇順で `targetSoc` / `backupReserveMaxSoc` まで配分
   - `control_enabled=0` は配分表示のみ、write対象外
6. 夜間時間帯かつ必要量がある場合、各デバイスの推奨充電上限を出す。
   - DELTA Pro 3: 既存 `NightChargePlan` の write経路を使う。
   - DELTA 3 Plus: 既存 `Delta3AuxPlan` と同じ private API writeゲートを使い、夜間用の計画候補として扱う。
   - ただし今回の初期実装では、DELTA 3 Plus の夜間writeは「計画表示 + guarded candidate」までに抑え、既存write executorに無理に統合しない。

## Implementation Steps

1. `domain.Status` / `NightChargePlan` に複数デバイス計画の型とフィールドを追加する。
2. `control` に複数デバイス配分ロジックを追加する。
   - 純粋関数として単体テスト可能にする。
   - 天気・外部APIに依存しない。
3. `cmd/server` で、夜間計画作成後に充電機器マスターとデバイス状態を使って `DevicePlans` を補完する。
4. Pro 3 の既存夜間writeは維持し、全体計画の Pro 3 配分と矛盾しないようにする。
5. DELTA 3 Plus は初期実装では計画表示を優先し、実writeは既存 `Delta3AuxPlan` の安全制御範囲を超えない。
6. frontend の夜間充電カードに「デバイス別 深夜充電計画」を追加する。
7. unit test を追加する。
   - 晴天でPV十分なら追加充電を抑制する。
   - 雨/曇りでPV不足なら複数機器に目標SOCを配分する。
   - `control_enabled=0` はwrite候補にしない。
   - SOC/容量不明はblockReasonを出す。
8. `go test` / frontend build / review loop を実行する。

## Review Points

- 実機writeゲートが弱くなっていないこと。
- `control_enabled=0` の機器にwrite候補を出さないこと。
- 容量不明・SOC不明時に危険な推測でwriteしないこと。
- 既存の DELTA Pro 3 夜間充電動作を壊さないこと。
- frontend 表示が「計画」と「実行済み」を混同しないこと。

## Verification Commands

```sh
cd backend && rtk go test ./...
cd frontend && rtk npm run build
rtk codex review --uncommitted
rtk git diff --check
```

## Operational Notes

- 初期導入後は、23:00-07:00 の `night_charge_plan_logs` と `delta3_aux_control_command_logs` を確認する。
- DELTA 3 Plus のSOCが下限に近い場合は、日中の買電回復制御と夜間充電計画の優先関係を追加で見直す。
- 将来、SwitchBot プラグを使う場合は、今回の `NightChargeDevicePlan` を基礎に `provider=switchbot` の executor を追加する。
