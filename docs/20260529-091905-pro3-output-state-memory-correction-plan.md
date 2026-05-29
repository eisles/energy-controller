# DELTA Pro 3 出力状態復帰設定の誤検知修正計画

## Goal

`outputPowerOffMemory` を「AC出力OFF履歴」として扱っている誤判定を修正する。

この値は、ユーザー確認により「出力ポートのON/OFF状態を記憶し、電源復帰時に同じ状態へ戻す設定」と考えるのが妥当である。したがって、AC出力OFFイベント、通知、画面上の警告として扱わない。

## Non-goals

- EcoFlow 実機への新しい write 制御は追加しない。
- 既存DBテーブルの破壊的マイグレーションは行わない。
- 過去に保存済みの `pro3_ac_output_event_logs` を削除・書き換えない。
- DELTA 3 Plus 補助充電制御ロジックは今回の対象外。

## Current State

- `backend/internal/control/pro3_ac_output_event.go` が `outputPowerOffMemory=true` のみで `ac_output_off_memory` イベントを生成している。
- 通知文言は「DELTA Pro 3 のAC出力OFF履歴を検知しました」になっている。
- フロントエンド `StatusCards.tsx` は `outputPowerOffMemory=true` を destructive alert として表示し、「AC出力OFF履歴」と表示している。
- 実ログでは `outputPowerOffMemory=true` の状態でも `battery_output_w` と `acOutFreq` が取得できており、AC出力は継続している。

## Data/API Contract

既存API互換を優先する。

- `Pro3ACOutputEvent.outputPowerOffMemory` は残すが、意味は「出力状態復帰設定」として扱う。
- `event_type=ac_output_off_memory` は過去互換のため残す。ただし新規生成条件は変更する。
- 実AC出力OFFの判定は、可能な範囲で以下の信号を組み合わせる。
  - `battery_output_w <= 0`
  - `acOutFreq == 0` または未取得
  - 将来 `acOutputEnabled=false` が Pro 3 で取得できる場合はそれを優先
- `outputPowerOffMemory=true` だけではイベントを生成しない。

## Files Likely To Change

- `backend/internal/control/pro3_ac_output_event.go`
- `backend/internal/control/pro3_ac_output_event_test.go`
- `backend/internal/notify/pro3_ac_output_alert.go`
- `backend/cmd/server/main_test.go`
- `frontend/components/StatusCards.tsx`
- `frontend/lib/types.ts`

## Implementation Steps

1. `BuildPro3ACOutputEvent` の生成条件を修正する。
   - `outputPowerOffMemory=true` のみでは `ok=false`。
   - `battery_output_w <= 0` かつ AC周波数が 0/未取得のような実OFF相当だけイベント化する。
2. イベントメッセージを「AC出力OFF履歴」から「AC出力OFF相当を検知」に寄せる。
3. 通知文言を同様に修正する。
4. フロントエンドの上部アラートを修正する。
   - `outputPowerOffMemory` 単体では destructive alert を出さない。
   - 表示する場合は「出力状態復帰設定: 有効」として通常情報扱いにする。
   - 実OFFイベントがある場合だけ警告表示する。
5. テストを更新する。
   - `outputPowerOffMemory=true` かつ AC出力ありではイベント生成しない。
   - AC出力0/周波数0ではイベント生成する。
   - 通知文言がOFF履歴ではなく実OFF相当になる。

## Safety Boundaries

- 実機 write は行わない。
- `.env` や秘密情報は変更しない。
- 既存ログの削除やDB改変は行わない。
- 既存の安全ゲート、制御コマンド間隔、EcoFlow write path には触れない。

## Review Points

- `outputPowerOffMemory` を再び警告条件として使っていないこと。
- 実OFF検知が過剰に発火しないこと。
- 既存API利用側を壊さないこと。
- 表示文言が「履歴」と誤解させないこと。

## Verification Commands

- `cd backend && rtk go test ./...`
- `cd frontend && rtk npm run build`
- `rtk curl -s http://localhost:18085/api/status | rtk jq '.pro3AcOutputEvent, .ecoflowDiagnostics.outputPowerOffMemory'`

## Rollback Notes

問題があれば、本修正差分を revert すれば旧表示・旧イベント生成に戻せる。DBには破壊的変更を入れないため、ロールバック時のデータ移行は不要。
