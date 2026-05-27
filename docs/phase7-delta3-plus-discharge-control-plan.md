# Phase 7 DELTA 3 Plus 買電時放電制御 実装計画

## Goal

買電中に DELTA 3 Plus がパススルーに近い状態になっている場合、バッテリー残量に余力があれば機器マスタの `reserveSoc` まで放電できるように、補助制御の判定と表示を整理する。

具体的には、`importW > 0` かつ DELTA 3 Plus の残量%が `reserveSoc` より高い場合、AC 充電上限を最小へ下げるだけでなく、放電下限として `backupReserveSoc = reserveSoc` を設定する候補を作る。これにより、バックアップリザーブが現在残量%近辺に残っていてパススルーになる状態を、制御可能な下限残量%まで放電できる状態へ戻す。

この計画では `SOC` は `State of Charge`、つまりバッテリー残量率を意味する。UI 表示では利用者向けに `残量%` / `制御下限残量%` と表記し、内部フィールド名としてだけ `SOC` を使う。

## Non-Goals

- DELTA 3 Plus の AC output ON/OFF は変更しない。
- Grid bypass / EPS / home grid 連携の write API は追加しない。
- 複数 DELTA 3 Plus への同時配分制御は追加しない。
- `.env` の安全 gate を緩めない。
- 実機 write を既定 ON にしない。
- 既存の DELTA Pro 3 余剰追従制御を広範囲に作り替えない。

## Current State

- `charging_devices.reserve_soc` は、DELTA 3 Plus 自動制御が一時的に下げてよい放電下限残量%として扱う。
- `backend/internal/control/delta3_aux_planner.go` には、買電中に `maybeSetDelta3DischargeReserve` を呼ぶ処理がある。
- ただし実機観測では、DELTA 3 Plus が `AC入力 ~= AC出力` のパススルーに見える状態で、制御計画が `no command candidate` になることがある。
- `/api/status` の `delta3AuxPlan` は read-only 表示になっており、write gate の抑制理由と planner の候補理由が UI 上で分かりにくい。
- 現在の実機値では `SOC=88%`、`AC入力` と `AC出力` が近く、買電中なので、放電優先に倒すべき状態である。

## Data And Control Contract

### Inputs

- `domain.Status.ImportW`
- `domain.Status.ExportW`
- DELTA 3 Plus status
  - `SOC`
  - `ACInW`
  - `ACOutW`
  - `ACChargeLimitW`
  - `BackupReserveSoc`
  - `BackupReserveEnabled`
  - `MaxChargeSoc`
- `control.Delta3AuxSettings`
  - `MinChargeW`
  - `MaxChargeW`
  - `SafetyMarginW`
  - `MinCommandDiffW`
  - `StopImportThresholdW`
  - `MinDischargeReserveSoc`

### Planner Output

買電中かつ残量%が放電下限より高い場合:

- `StrategyState = RECOVERING`
- `RecommendedACChargeLimitW = MinChargeW` または安全上限内の回復値
- `ShouldSetBackupReserve = true` when current backup reserve state does not allow discharge down to `MinDischargeReserveSoc`
- `RecommendedBackupReserveSoc = MinDischargeReserveSoc`
- `WouldWrite = true` before write guards
- `Reason` は「買電中のため DELTA 3 Plus を制御下限残量まで放電できるようにリザーブを下げる」趣旨にする

### Guard Output

実writeは既存 gate をすべて通った場合だけ実行する。

- `MOCK_MODE=false`
- `SIMULATION_MODE=false`
- `ENABLE_REAL_CONTROL=true`
- `AUTO_CONTROL_ENABLED=true`
- `CONFIRM_ECOFLOW_WRITE` が期待値
- trial window が有効
- `ECOFLOW_DELTA3_READ_ENABLED=true`
- 機器マスタ上の DELTA 3 Plus `controlEnabled=true`
- `ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=true`
- `ECOFLOW_DELTA3_EXECUTE=true`
- `ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=true`
- minimum command interval / diff / previous error backoff を満たす

## Safety Boundaries

- 既定では mock + simulation + read-only を維持する。
- 追加する write 候補も既存の `EvaluateDelta3AuxCommandGuard` を必ず通す。
- `reserveSoc` 未満へ放電させない。
- `SOC <= reserveSoc` の時は放電用リザーブ変更候補を作らない。
- `BackupReserveSoc` が取得不能な時は、実write候補を作らず、理由をログに残す。
- 405 / AC output OFF の再発を避けるため、AC充電上限は既存の出力考慮安全上限を超えない。

## Implementation Steps

1. `backend/internal/control/delta3_aux_planner.go`
   - 買電中の回復分岐で、パススルー判定とは独立して `SOC > MinDischargeReserveSoc` なら放電リザーブ候補を評価する。
   - `BackupReserveSoc` が取得できる場合のみ、現在値と `MinDischargeReserveSoc` を比較して必要な時だけ `ShouldSetBackupReserve` を立てる。
   - `BackupReserveEnabled=false` かつ `BackupReserveSoc` が下限と一致する場合でも、必要なら有効化候補を出す。
   - 理由文を日本語表示マップに追加する。

2. `backend/internal/control/delta3_aux_planner_test.go`
   - 買電中、残量88%、reserveSoc 20%、backupReserveSoc 88% なら `RecommendedBackupReserveSoc=20` になること。
   - 買電中、backupReserveSoc 0%、disabled の場合は、機器仕様上放電できるなら候補を作るか、現状維持の理由を明確にすること。
   - 残量が reserveSoc 以下なら候補を作らないこと。
   - backup reserve 未取得なら実write候補を作らないこと。

3. `frontend/lib/display-labels.ts`
   - 新しい reason / suppressed reason を日本語化する。

4. 必要に応じて `frontend/components/StatusCards.tsx`
   - `本体リザーブ残量` と `制御下限残量` の差が放電可否に関係することが分かる表示へ微修正する。
   - 既存未コミットのリザーブ表示整理と矛盾しないようにする。

## Review Points

- 買電中に充電を増やす候補を作っていないこと。
- `reserveSoc` より下に放電する候補を作っていないこと。
- `backupReserveSoc` 未取得時に危険な推測 write をしないこと。
- 既存 write gate を迂回していないこと。
- unit test で実機なしに判定できること。
- UI で `本体リザーブ残量` と `制御下限残量` が混同されないこと。

## Verification Commands

```bash
cd backend && rtk go test ./...
cd frontend && rtk npm run build
rtk codex review --uncommitted
```

## Operational Notes

- 実装後にサーバーを再ビルド・再起動しない限り、稼働中の `backend/bin/server` には反映されない。
- 実機 write が有効な環境では、買電中かつ SOC が十分ある時に DELTA 3 Plus の backup reserve を機器マスタの `reserveSoc` へ戻す候補が出る。
- 期待通り放電しない場合は、EcoFlow private API の `backupReserveSoc` がアプリ表示と一致しているか、別フィールドでセルフパワー/バックアップ設定を持っていないかを次の調査対象にする。
