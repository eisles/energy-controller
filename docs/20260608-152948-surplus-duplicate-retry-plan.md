# Surplus Duplicate Retry Guard Implementation Plan

## Goal

買電中かつ料金上放電したい状態で、DELTA Pro 3 の余剰制御候補が `duplicate command candidate` として抑制され続ける問題を修正する。

現在の観測では、余剰制御は `backupReserveSoc=10` と energy strategy modes OFF を候補に出しているが、実機状態は `backupReserveSoc=54` のまま残っている。直前候補と fingerprint が同じという理由だけで再送を止めると、実機が目標状態に到達していない場合でも復旧しない。

## Non-goals

- 実機制御ゲートを緩和しない。
- 手動の実機 write API や追加の操作 UI は作らない。
- 夜間充電プラン、料金マスタ、機器優先順位の再設計はしない。
- DELTA 3 Max Plus や補助バッテリー制御へ同じ修正を横展開しない。

## Current State

- `backend/internal/control/surplus_executor.go` の `surplusCommandSuppressedReason` は、前回 write 候補と今回候補の `CommandFingerprint` が一致すると即座に `duplicate command candidate` を返す。
- そのため、前回送信後に実機状態がまだ目標へ反映されていない場合も同じ候補が抑制される。
- 実運用では `previousBackupReserveSoc=54`、`targetBackupReserveSoc=10` のまま同一候補が抑制され続けている。

## Data And Contracts

- 入力は既存の `domain.Status` と `domain.SurplusControlCommandLog` のみを使う。
- 判定対象:
  - `ShouldAdjustACChargeLimit`: 現在の `ACChargeLimitW` が `TargetACChargeLimitW` と一致しているか。
  - `ShouldSetBackupReserve`: 現在の `BackupReserveSoc` が `TargetBackupReserveSoc` と一致しているか。
  - `ShouldDisableEnergyModes`: すべての energy strategy mode が OFF になっているか。
  - `ShouldEnableTOUMode`: TOU が ON になっているか。
- 目標状態に一致していない場合は、同一 fingerprint でも最小コマンド間隔経過後に再送候補として扱えるようにする。

## Safety Boundaries

- `MockMode`、`SimulationMode`、`EnableRealControl`、`AutoControl`、`ConfirmEcoFlowWrite`、`RealControlTrialActive` の既存ゲートは維持する。
- `backup reserve status unavailable` と mode status guard は維持する。
- `settings.MinCommandInterval` 未満の再送は抑制する。
- 前回エラー後の retry interval は維持する。
- 実機に未反映の同一候補だけを再送対象にし、既に目標状態なら duplicate として抑制する。

## Implementation Steps

1. `surplus_executor.go` に、今回候補が現在の実機観測値に反映済みか判定する helper を追加する。
2. 同一 fingerprint の duplicate 判定を変更する。
   - 最小コマンド間隔未満なら従来どおり duplicate 抑制。
   - 間隔経過後、現在状態が目標と一致していれば duplicate 抑制。
   - 間隔経過後、現在状態が目標と不一致なら再送を許可。
3. `surplus_executor_test.go` に以下を追加または修正する。
   - 同一候補かつ間隔未満は抑制される。
   - 同一候補かつ間隔経過後、実機状態が目標未反映なら `WouldWrite=true` になる。
   - 同一候補かつ間隔経過後、実機状態が目標反映済みなら duplicate 抑制される。
4. 既存の実機 write path test が通ることを確認する。

## Review Points

- 同一候補の無制限連打にならないこと。
- 目標未反映時だけ再送すること。
- mode status が不明な場合に再送許可しないこと。
- 既存の安全ゲートと最小コマンド間隔が維持されること。

## Verification Commands

- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./internal/control`
- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./...`
- `rtk git diff --check`

## Operational Notes

実装後に `docker compose up -d --build` で反映すると、現在の実機状態がまだ目標未反映で、かつ既存の実機制御ゲートがすべて有効な場合、コントローラの通常周期で再送が発生する可能性がある。これは手動 write ではなく、既存ゲート内の自動制御として扱う。
