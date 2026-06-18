# 買電中 net 充電の放電優先制御 改善計画

## Goal

買電中かつ中/高単価時間帯に DELTA Pro 3 が `AC入力 > AC出力` の net 充電状態になっている場合、料金最適化として放電優先へ寄せる制御判断を改善する。

現在は制御判断として「買電中は充電しない」と出ていても、DELTA Pro 3 の AC充電上限がアプリ下限相当の 400W に残り、実機が `AC入力 823W / AC出力 422W` のように約400W充電寄りになることがある。この状態を明示的に扱い、放電可能な実機リザーブがある場合は self-powered 放電候補へ進める。

## Non-goals

- EcoFlow の未検証 0W AC充電 write は追加しない。
- `ENABLE_REAL_CONTROL=true` / `SIMULATION_MODE=false` / `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND` など既存の実機 write gate は変更しない。
- コマンド最小間隔、重複抑制、実機 trial window は緩めない。
- DELTA 3 Max Plus / DELTA 3 Plus 2 の private MQTT timeout 改善はこの計画に含めない。
- 料金比較 UI や SOC在庫補正の計算は変更しない。

## Current State

- `backend/internal/control/surplus_planner.go`
  - 買電中は `RECOVERING` になり、AC充電上限を `minImportRecoveryChargeW = 400` まで下げる。
  - すでに AC充電上限が 400W の場合は `ShouldAdjustACChargeLimit=false` となり、追加 write 候補が出ない。
  - 中/高単価の買電中は `TariffControlReason` で放電優先を説明するが、self-powered への切り替えはここでは行わない。
- `backend/internal/control/night_charge_planner.go`
  - 中/高単価買電中に `self-powered-discharge` を推奨する経路がある。
  - ただし放電可否の下限に `SolarSettings.MinimumReserveSoc` や計算上の `MinimumReserveSoc` を使うため、現在 SOC が 23% で設定最小リザーブが 30% の場合は候補にならない。
  - 実機の backup reserve が 10% で、実際には 23% から 10% まで放電余地があっても候補から外れる。
- `backend/internal/control/night_charge_executor.go`
  - self-powered mode write は既存の guarded write 経路がある。

## Proposed Behavior

1. 中/高単価の買電中は、まず「実機の backup reserve SOC」を放電下限として評価する。
2. `BatterySoc > BackupReserveSoc` で、かつ backup reserve が設定最小リザーブより低い場合でも、現在の実機設定が示す放電可能範囲として扱う。
3. この条件で `self-powered-discharge` を推奨し、必要なら:
   - backup reserve を現在の実機 reserve へ維持または設定
   - self-powered mode を ON
4. ただし低単価時間帯はこれまで通り充電優先候補を許可する。
5. 計算上の夜間目標や最低在庫 SOC は引き続き `MinimumReserveSoc` を使い、今回の例外は「中/高単価買電中の放電回復」だけに限定する。

## Data/API Contract

- 既存 `NightChargePlan` の `RecommendedMode`, `ShouldEnableSelfPoweredMode`, `ShouldSetBackupReserve`, `RecommendedBackupReserveSoc`, `ActionSummary`, `Reason`, `CommandFingerprint` を使う。
- 新しい API field は追加しない。
- ログには既存の `ActionSummary` / `CommandBlockReason` / `Reason` により、なぜ self-powered 放電候補になったかを残す。

## Safety Boundaries

- 実機 write は既存の `GuardNightChargeCommand` を通す。
- mock/simulation/default では write しない。
- 低単価時間帯には self-powered 放電へ切り替えない。
- SOC が backup reserve 以下なら放電候補にしない。
- backup reserve が不明な場合は従来の計算下限を使う。
- AC出力や AC充電の未検証パラメータは増やさない。

## Implementation Steps

1. `night_charge_planner.go` に中/高単価買電中の放電下限計算を分離する。
2. `tariffNightSelfPoweredDischargeRecovery` が、実機 backup reserve を安全な放電下限として使える場合に self-powered 候補を返すよう修正する。
3. `tariffNightSelfPoweredDischargeReserveSoc` は、通常計画では `MinimumReserveSoc` を維持しつつ、買電中放電回復だけ現在 backup reserve を使えるようにする。
4. `night_charge_planner_test.go` に以下を追加する。
   - SOC が設定最小リザーブ未満でも、現在 backup reserve より上なら中単価買電中に self-powered 放電候補になる。
   - 低単価では self-powered 放電候補にならない。
   - current backup reserve が高い場合に最低リザーブが ratchet しない既存挙動は維持する。
5. 必要に応じて `ActionSummary` / `TariffControlReason` の文言を調整する。

## Review Points

- 実機 write gate を緩めていないこと。
- 低単価 charging と中/高単価 discharging の優先順位が逆転していないこと。
- current backup reserve を通常の夜間充電目標計算に混ぜていないこと。
- 既存の `DoesNotUseCurrentBackupReserveAsMinimumReserve` 系テストを壊していないこと。

## Verification Commands

- `cd backend && mkdir -p .gocache && GOCACHE=$PWD/.gocache rtk go test ./internal/control`
- `cd backend && mkdir -p .gocache && GOCACHE=$PWD/.gocache rtk go test ./...`
- `rtk codex review --uncommitted`

## Operational Notes

実装後に live 確認する場合は、まず `/api/status` の `nightChargePlan.recommendedMode` と `controlDiagnostics` を確認する。Docker rebuild / 実機稼働反映は、ユーザーから明示依頼がある場合のみ行う。
