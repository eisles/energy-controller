# High-price Self-powered Discharge Recovery Implementation Plan

## Goal

高単価時間帯に買電しているとき、DELTA Pro 3 が実際に放電へ向かうように制御を修正する。

直近の実機確認では、バックアップリザーブは 10%、TOU/SelfPowered は OFF になったが、DELTA Pro 3 は `AC入力 > AC出力` のままで充電寄りだった。EcoFlow の挙動として、AC入力が接続されている状態で放電を優先するには、ユーザー指摘の通り `SelfPowered ON` かつ `backupReserveSoc < current SOC` が必要と見なす。

## Non-goals

- 実機 write gate を緩和しない。
- 新しい手動 write API は追加しない。
- Surplus command log schema は変更しない。
- DELTA 3 Max Plus / DELTA 3 Plus 系の制御は変更しない。

## Current State

- `surplusPlan` は高単価買電中に `backupReserveSoc=10` と energy strategy modes OFF を出す。
- 反映後の実機状態は reserve 10 / modes OFF まで到達したが、AC入力が継続し、実質充電寄りになった。
- `nightChargePlan` は SelfPowered write path を既に持つが、高単価時は command plan を全面 return しており、放電回復コマンドを出せない。

## Data And Contracts

- `NightChargePlanInput` に `GridW` を追加し、買電中かどうかを night planner 側で判定できるようにする。
- 高単価かつ買電中、現在 SOC が放電下限より上の場合:
  - `RecommendedMode = "self-powered-discharge"`
  - `RecommendedBackupReserveSoc = minimum reserve`
  - `ShouldEnableSelfPoweredMode = true` when SelfPowered is not already true
  - `ShouldSetBackupReserve = true` when current reserve differs from minimum reserve
  - `ShouldChargeTonight = false`
- このモードでは `nightChargePlan` が control owner になり、同じ周期の `surplusPlan` が energy modes OFF を上書きしないようにする。

## Safety Boundaries

- `MockMode`、`SimulationMode`、`EnableRealControl`、`AutoControl`、`ConfirmEcoFlowWrite`、`RealControlTrialActive` の gate は既存どおり維持する。
- `MinCommandInterval` は既存の NightCharge command guard で維持する。
- 高単価買電中以外の night charge / low-price charge / surplus charge は変更しない。
- reserve は既存の minimum reserve を下回らない。

## Implementation Steps

1. `NightChargePlanInput` に `GridW` を追加し、mock provider から `gridPower.GridW` を渡す。
2. `night_charge_planner.go` に高単価買電 self-powered discharge helper を追加する。
3. `applyNightModeRecommendation` / `applyNightChargeCommandPlan` を修正し、高単価買電中だけ SelfPowered discharge 候補を出す。
4. `nightPlanOwnsEnergyControl` を修正し、この放電回復モードでは `surplusPlan` を同周期で走らせない。
5. 単体テストを追加し、既存テストを調整する。
6. backend tests、diff check、implementation review を実行する。
7. Docker に反映し、`/api/status` と `/api/night-charge/plans` / `/api/surplus-control/commands` で実機挙動を確認する。

## Review Points

- 高単価買電時だけ SelfPowered 放電回復になること。
- 高単価でも売電時や低単価時にはこの制御が出ないこと。
- NightCharge owner 化で surplus 側の energy modes OFF と競合しないこと。
- 実機 write は既存 gate と最小間隔を通った場合だけ発生すること。

## Verification Commands

- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./internal/control`
- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./...`
- `rtk git diff --check`
- `rtk codex review --uncommitted`

