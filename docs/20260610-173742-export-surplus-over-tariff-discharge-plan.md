# Export Surplus Over Tariff Discharge Plan

## Goal

売電中は高単価時間帯であっても DELTA Pro 3 の余剰吸収充電を優先し、night charge 側の「高単価なので self-powered 放電へ戻す」候補が surplus charge と競合しないようにする。

## Non-goals

- 実機 write gate、最小コマンド間隔、実制御フラグの緩和はしない。
- 充電優先順位マスタや UI は変更しない。
- DELTA 3 Max Plus / DELTA 3 Plus 2 の write payload は変更しない。
- 料金マスタや料金時間帯 UI は変更しない。

## Current State

- `surplus_planner.go` は売電中に Pro3 の AC 充電上限を上げる候補を出す。
- `night_charge_planner.go` は高単価/中間単価の買電中に self-powered 放電候補を出す。
- ただし night charge 側の候補が control ownership を持つと、売電中でも AC 充電上限を 400W へ戻す候補が出て、surplus plan の `1200W` などの余剰吸収候補と競合する。

## Data And Contracts

- 入力は既存の `NightChargePlanInput.GridW` と `TariffControlContext` を使う。
- `GridW > 0` のときだけ料金優先放電を許可する。
- `GridW < 0` のときは、料金が高単価でも night charge 側は放電優先 write 候補を所有しない。
- `SurplusPlan` の既存出力 contract は維持する。

## Safety Boundaries

- `ENABLE_REAL_CONTROL=true`、`SIMULATION_MODE=false`、`AUTO_CONTROL_ENABLED=true` など既存 write gate は変更しない。
- コマンド間隔、差分閾値、duplicate suppression は変更しない。
- 新しい実機 write 種別は追加しない。
- 変更は planner の候補条件と単体テストに限定する。

## Implementation Steps

1. `night_charge_planner.go` に料金優先放電を許可する入力条件 helper を追加する。
2. `tariffNightSelfPoweredDischargeRecovery` と `applyNightChargeCommandPlan` の料金優先分岐を、その helper 経由にする。
3. 売電中は `self-powered=on|ac=400|reserve=...` の night charge command candidate が出ないテストを追加する。
4. 買電中の高単価/中間単価では従来どおり self-powered 放電候補が出るテストを維持・追加する。
5. 低単価/中立料金の既存テストを壊さない。

## Review Points

- 売電中に night charge 側が surplus charge を上書きしないこと。
- 買電中の料金優先放電が維持されること。
- 単一単価/低単価では放電優先にならないこと。
- 実機 write gate に変更がないこと。

## Verification Commands

- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./internal/control`
- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./cmd/server`
- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./...`
- `rtk git diff --check`
- `rtk codex review --uncommitted`

## Rollback Notes

問題があれば `night_charge_planner.go` の helper 適用と追加テストを戻す。DB migration や設定変更は行わないため rollback はコード差分の revert で完結する。
