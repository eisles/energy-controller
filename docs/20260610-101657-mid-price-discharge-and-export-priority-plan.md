# Mid-price Import Discharge And Export Priority Implementation Plan

## Goal

中間単価でも買電中は、DELTA Pro 3 が十分な残量を持つ場合にバッテリー放電を優先するかを制御として実装する。

あわせて、売電時に `DELTA Pro 3` / `DELTA 3 Max Plus` / `DELTA 3 Plus 2` のどれへ優先充電するかを、既存の優先順位制御に沿って整理し、テストと診断理由で固定する。

## Current State

- `night_charge_planner.go` は `TariffControl.IsHighPrice` のときだけ `self-powered-discharge` を候補にする。
- 中間単価は `mid-price period; observe unless surplus or forecast target requires action` になり、買電中でも放電優先にならない。
- `surplus_planner.go` も高単価買電中だけ TOU 復帰を抑制するため、中間単価買電中に TOU 側へ戻す余地がある。
- 売電時の補助バッテリー制御は、既に `DELTA Pro 3` の surplus candidate を待つ `WAIT_PRO3` と、優先度が高い DELTA 3 系 write target の bypass を持っている。

## Target Behavior

### 買電中

- 料金マスターで現在単価が低単価ではない場合、つまり中間単価または高単価の買電中は、バッテリー SOC が放電下限より上なら `self-powered-discharge` を優先する。
- `RecommendedMode = "self-powered-discharge"` にし、SelfPowered ON と backup reserve を放電下限へ下げる候補を出す。
- 低単価中は従来どおり深夜充電・不足補充の制御を許可する。
- 料金マスターが取得できない場合は、既存の時刻ベース・安全側挙動を維持し、無条件に放電へ切り替えない。

### 売電中

- 原則の優先順は `DELTA Pro 3` を先頭にする。Pro3 が余剰吸収候補を持つ間、DELTA 3 系は `WAIT_PRO3` として待つ。
- 機器マスターの優先度で DELTA 3 系 write target が Pro3 より高い場合だけ、その write target は Pro3 待ちを bypass できる。
- DELTA 3 系の中では、現在の write target 解決結果に従う。現状の想定は `DELTA 3 Max Plus` が write target なら Max Plus、そうでなければ `DELTA 3 Plus 2` が対象になる。
- この優先順を理由文とテストで明示し、今後の機器追加で意図せず順序が変わらないようにする。

## Non-goals

- 実機 write gate を緩和しない。
- 新しい手動 write API は追加しない。
- Nature / EcoFlow の認証・通信間隔は変更しない。
- 料金マスター UI は変更しない。
- DELTA 3 Max Plus の AC1/AC2 グループ制御はこの計画では変更しない。

## Safety Boundaries

- `MockMode`、`SimulationMode`、`EnableRealControl`、`AutoControl`、`ConfirmEcoFlowWrite`、`RealControlTrialActive` の既存 gate を維持する。
- 最小コマンド間隔と command fingerprint による重複抑制を維持する。
- 料金コンテキストが不明な場合は中間単価とは見なさない。
- reserve は既存の minimum reserve / min discharge reserve を下回らない。
- 同一周期で night plan と surplus plan が競合しないよう、`self-powered-discharge` は night plan owner のままにする。

## Implementation Steps

1. `night_charge_planner.go` の高単価専用 helper を、低単価以外の料金帯を判定する helper へ置き換える。
2. 中間単価買電中にも `self-powered-discharge` 候補を出し、ActionSummary と TariffControlReason を更新する。
3. `surplus_planner.go` の高単価買電 recovery 判定を、低単価以外の買電 recovery 判定へ拡張し、中間単価で TOU 復帰しないようにする。
4. `auxiliary_battery_planner.go` の `WAIT_PRO3` 理由を、優先順が分かる文言に更新する。
5. `night_charge_planner_test.go` に中間単価買電で SelfPowered discharge になるテスト、低単価でこの制御に入らないテストを追加する。
6. `surplus_planner_test.go` に中間単価買電で TOU 復帰を抑制するテストを追加する。
7. `auxiliary_battery_planner_test.go` に売電時の Pro3 優先待ちと、優先度 bypass の意図が分かる assertion を追加する。
8. backend tests、diff check、plan / implementation review を実行する。

## Review Points

- 中間単価買電中だけでなく高単価買電中も従来どおり放電優先になること。
- 低単価中は深夜充電・不足補充の候補が抑制されないこと。
- 料金コンテキストなしでは無条件に放電優先へ切り替わらないこと。
- 売電時の優先充電順が `Pro3 first` として説明・テストされること。
- 実機 write は既存 gate と最小間隔を通った場合だけ発生すること。

## Verification Commands

- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./internal/control`
- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./cmd/server`
- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./...`
- `rtk git diff --check`
- `rtk codex review --uncommitted`
