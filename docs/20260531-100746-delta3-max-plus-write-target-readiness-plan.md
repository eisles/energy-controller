# DELTA 3 Max Plus Write Target Readiness Plan

## Goal

DELTA 3 Max Plus を既存の EcoFlow private MQTT 補助バッテリー制御の対象として安全に扱えるようにする。

具体的には、充電機器マスタで `DELTA_3_MAX_PLUS` を `controlEnabled=true` にした場合、夜間充電/料金最適化の機器別計画で「どの DELTA 3 系デバイスが実際の write target か」を明示し、write target ではない DELTA 3 系デバイスを誤って write 候補として表示しない。

## Non-goals

- AC1 / AC2 / AC出力保護チャンネルの ON/OFF write command は追加しない。
- 未確認の EcoFlow private MQTT payload を推測で送信しない。
- 実機 write gate、最小コマンド間隔、trial window、`CONFIRM_ECOFLOW_WRITE` 条件は変更しない。
- 充電機器マスタの DB schema は変更しない。
- `.env` に新しい SN や認証情報を追加しない。

## Current State

- `DELTA_3_MAX_PLUS` は `ecoflow_delta3_plus` 系として機器マスタ、status reader、private MQTT profile に登録済み。
- `ChargingDeviceRepository.Delta3WriteTarget` は `enabled=true`、`controlEnabled=true`、`supports_ac_charge_limit=true`、`status_source='ecoflow_private_mqtt'`、優先度順で DELTA 3 系 write target を1台選ぶ。
- `nightChargeDeviceInputs` は DELTA Pro 3 の write target は反映しているが、DELTA 3 系の write target は `WriteTarget` に反映していない。
- そのため、複数の DELTA 3 系デバイスがある場合、機器別計画で実際には write target ではない機器も write 候補のように見える可能性がある。

## Files Likely To Change

- `backend/cmd/server/main.go`
- `backend/cmd/server/main_test.go`
- `backend/internal/control/night_charge_device_planner.go`
- `backend/internal/control/night_charge_device_planner_test.go`
- `backend/internal/domain/status.go`
- `frontend/lib/types.ts`
- `frontend/components/StatusCards.tsx`

## Data / API Contracts

`NightChargeDevicePlan` に表示用の任意/後方互換項目を追加する。

- `deviceType`
- `writeTarget`

既存の `deviceId`、`kind`、`controlEnabled`、`blockReason` は維持する。

`writeTarget=false` の DELTA 3 系デバイスは、実機 write 候補から除外する。優先度を上げて Max Plus が `Delta3WriteTarget` になった場合だけ、Max Plus が write 候補として表示される。

## Safety Boundaries

- 実機 write の種類は増やさない。既存の AC充電上限/バックアップリザーブ write 経路だけを使う。
- `ENABLE_REAL_CONTROL=true`、`SIMULATION_MODE=false`、`MOCK_MODE=false`、`AUTO_CONTROL_ENABLED=true`、trial window、`CONFIRM_ECOFLOW_WRITE`、`ECOFLOW_DELTA3_*` gate は維持する。
- 複数 DELTA 3 系が登録されている場合も、write target は1台だけに限定する。
- UI は「write target / 対象外」を表示するだけで、直接実機 write を起動しない。
- 認証情報、token、device SN は plan / UI / logs に追加表示しない。

## Implementation Steps

1. `NightChargeDeviceInput` と `NightChargeDevicePlan` に `DeviceType` と `WriteTarget` を追加する。
2. `nightChargeDeviceInputs` で `Delta3WriteTarget` を解決し、DELTA 3 系デバイスの `WriteTarget` を設定する。
3. `nightChargeDeviceAllocationBlockReason` で、`ecoflow_delta3_plus` かつ `WriteTarget=false` の機器を `"DELTA 3 series device is not the write target"` として抑制する。
4. `initialNightChargeDevicePlan` で `deviceType` / `writeTarget` を plan に反映する。
5. 既存の Max Plus target resolver テストに加え、機器別夜間充電計画で Max Plus が write target のときだけ `WouldWrite=true` になり、別の DELTA 3 系が target のときは抑制されるテストを追加する。
6. 画面の機器別深夜充電プランに `Device type` と `write target` を表示する。
7. block reason の日本語表示に DELTA 3 系 write target ではない場合の文言を追加する。

## Review Points

- DELTA 3 Max Plus を対象にできる条件が、機器マスタの優先度/制御候補と一致しているか。
- 非 target の DELTA 3 系デバイスが write 候補に見えないか。
- 既存 DELTA Pro 3 / DELTA 3 Plus / RIVER 2 の表示や抑制理由を壊していないか。
- 実機 write gate や payload が増えていないか。
- secrets や SN が表示・保存されていないか。

## Verification

- `cd backend && rtk go test ./cmd/server ./internal/control ./internal/store ./internal/api`
- `cd frontend && rtk npm run build`
- `rtk git diff --check`
- `rtk codex review --uncommitted`

## Rollback / Operational Notes

- 変更は計画表示と write target 判定の明確化が中心で、write command の種類は増えない。
- 問題が出た場合は `writeTarget` 表示と DELTA 3 系 target 抑制を戻せば、既存の単一 DELTA 3 系制御に戻せる。
- 実運用では、充電機器マスタで DELTA 3 Max Plus の優先度を DELTA 3 Plus 2 より高くし、`controlEnabled=true`、`supportsAcChargeLimit=true`、`deviceType=DELTA_3_MAX_PLUS` にすることで Max Plus が write target になる。
