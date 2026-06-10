# High-price Import Discharge Mode Implementation Plan

## Goal

高単価時間帯に買電しているとき、DELTA Pro 3 が放電回復へ向かうように、余剰制御の recovery mode 判定を修正する。

先の duplicate retry 修正により `backupReserveSoc=10` と energy strategy modes OFF の実機 write は再送された。しかし直後の制御周期で TOU を ON に戻す候補が出て、実機状態が再び `backupReserveSoc=54` 付近へ戻った。高単価の買電中は、TOU 復帰よりもリザーブ低下による放電を優先する必要がある。

## Non-goals

- 夜間充電プラン全体は再設計しない。
- 通常の売電時・低単価時の TOU 復帰挙動は変更しない。
- 手動 write API は追加しない。
- 実機制御ゲートは変更しない。

## Current State

- `PlanSurplusCharging` の買電 recovery は `applyRecoveryModePlan` を呼ぶ。
- `applyRecoveryModePlan` は、energy mode が全 OFF かつ TOU が OFF の場合に `ShouldEnableTOUMode=true` を立てる。
- 高単価買電中でもこの TOU 復帰が走るため、放電床へ向けた reserve 低下と競合する。

## Data And Contracts

- `SurplusPlanInput.TariffControl.IsHighPrice` を使って高単価時間帯を判定する。
- 買電 recovery 中かつ高単価時間帯、さらに現在 SOC が放電床より上の場合は、TOU 復帰を出さない。
- TOU が ON の場合は、energy strategy modes OFF の候補を出す。
- 既存の `modeGuardReason` と実機 write gate はそのまま使う。

## Safety Boundaries

- `ENABLE_REAL_CONTROL`、`SIMULATION_MODE=false`、`CONFIRM_ECOFLOW_WRITE`、trial window などの write gate は維持する。
- `MinCommandInterval` は維持する。
- 高単価買電中以外の recovery では既存の TOU 復帰を維持する。
- 実機状態が mode status unavailable の場合は既存 guard で write しない。

## Implementation Steps

1. `surplus_planner.go` に、高単価買電中の放電 recovery 判定 helper を追加する。
2. `applyRecoveryModePlan` を拡張し、高単価買電中は TOU 復帰を抑制し、TOU が ON なら OFF 候補を出す。
3. `surplus_planner_test.go` に、高単価買電中の TOU OFF 候補と TOU 復帰抑制のテストを追加する。
4. backend tests と diff check を実行する。

## Review Points

- 高単価買電中だけ挙動が変わること。
- TOU ON から OFF へ落とす場合も mode status guard が効くこと。
- 通常 recovery の既存テストが壊れないこと。
- duplicate retry 修正との相互作用でコマンド連打にならないこと。

## Verification Commands

- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./internal/control`
- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./...`
- `rtk git diff --check`

## Operational Notes

反映後、現在が高単価買電かつ DELTA Pro 3 の SOC が放電床より上であれば、余剰制御は TOU 復帰ではなく `backupReserveSoc=10` と energy strategy modes OFF を優先する。実機 write は既存の安全ゲートと最小間隔に従う。
