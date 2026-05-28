# Phase 7 DELTA 3 Plus 2 リザーブ反映検知 実装計画

## 目的

DELTA 3 Plus 2 で AC 充電上限は反映されている一方、バックアップリザーブ値が `0% / OFF` のままでパススルーになり、実充電へ移らない状態を検知できるようにする。

この計画では、リザーブ制御コマンドを追加で強めるのではなく、既存の実機writeゲートを維持したまま、送信済みコマンドと現在の実機状態を突き合わせて以下を実現する。

- リザーブ反映待ちを画面と `Delta3AuxPlan` に出す。
- 一定時間後も反映されない場合はリザーブ反映失敗として扱う。
- 反映失敗中は同じリザーブ候補を繰り返し送らない。
- AC充電上限の安全制御や買電時の充電停止は止めない。

## 非目的

- DELTA 3 Plus の新しい private API payload を推測して実機writeすること。
- AC出力ON/OFFの自動復旧writeを追加すること。
- DELTA Pro 3 の制御ロジックを変更すること。
- 機器マスターの優先順位や夜間充電配分を再設計すること。

## 現状

- DELTA 3 Plus 1台目はルータ接続のため制御対象外。
- DELTA 3 Plus 2 が現在の補助バッテリー制御対象。
- 直近では AC充電上限の変更は反映されている。
- 一方で `backupReserveSoc=0`, `backupReserveEnabled=false` が続き、`target_backup_reserve_soc=80` などの送信後も実機状態が変わっていない。
- 現在の `EvaluateDelta3AuxCommandGuard` は、直近ログの fingerprint と最小間隔で抑止するが、「送信済みリザーブが反映されなかった」という意味づけを持っていない。

## 変更対象

想定する主な変更ファイル:

- `backend/internal/domain/status.go`
  - `Delta3AuxPlan` にリザーブ反映状態を追加する。
- `backend/internal/control/delta3_aux_executor.go`
  - 直近送信済みログと現在状態から反映待ち/失敗を判定する。
  - 反映失敗中の同一リザーブ候補を抑止する。
- `backend/internal/control/delta3_aux_planner.go`
  - 必要なら計画理由に反映状態を組み込む。
- `backend/internal/control/delta3_aux_executor_test.go`
  - 反映待ち、反映失敗、AC安全制御優先の単体テストを追加する。
- `frontend/lib/types.ts`
  - 追加フィールドの型定義。
- `frontend/components/StatusCards.tsx`
  - 反映待ち/失敗状態を表示。
- `frontend/lib/display-labels.ts`
  - 日本語表示ラベルを追加。

## データ/API契約

`Delta3AuxPlan` に以下を追加する。

- `backupReserveApplyState`
  - `""`: 判定なし
  - `pending`: 送信済みリザーブ候補の反映待ち
  - `failed`: 送信済みリザーブ候補が一定時間後も反映されていない
  - `applied`: 直近送信値が現在値に反映済み
- `backupReserveApplyReason`
  - 画面表示用の短い理由。
- `lastBackupReserveCommandAt`
  - 直近のリザーブ実送信時刻。
- `lastBackupReserveTargetSoc`
  - 直近のリザーブ目標値。

DBスキーマは変更しない。既存の `delta3_aux_control_command_logs` から判定する。

## 判定ロジック

1. 直近の `delta3_aux_control_command_logs` から、以下を満たすログを取得済みの `Previous` として使う。
   - `command_sent=true`
   - `target_backup_reserve_soc IS NOT NULL`
   - `error_message IS NULL`
2. 現在の実機状態と比較する。
   - リザーブ有効化コマンドは `backupReserveEnabled=true` かつ `backupReserveSoc == target_backup_reserve_soc` なら `applied`
   - リザーブ無効化コマンドは `backupReserveEnabled=false` なら `applied`
   - 送信から `MinCommandInterval` 未満なら `pending`
   - 送信から `MinCommandInterval` 以上経過し、まだ一致しなければ `failed`
3. `failed` かつ次の候補が同じ `target_backup_reserve_soc` の場合は抑止する。
   - `suppressedReason`: `previous backup reserve command was not reflected by device`
4. ただし以下は抑止しない。反映失敗中の同一リザーブ候補は外し、AC充電上限の安全方向だけを送る。
   - AC充電上限だけを下げる安全制御。
   - `SAFE_LIMIT` のAC充電上限調整。
   - 買電中にAC充電上限を下げる候補。

## 安全境界

- 実機write条件は既存どおり維持する。
  - `ENABLE_REAL_CONTROL=true`
  - `SIMULATION_MODE=false`
  - `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`
  - trial window / master control / private API write gate
- 新しい実機write payload は追加しない。
- 反映失敗時は同じリザーブwriteを抑止し、AC充電上限の安全方向だけ許可する。
- 既存のDELTA 3 Plus 1台目制御除外は維持する。

## 実装手順

1. `Delta3AuxPlan` に反映状態フィールドを追加する。
2. `EvaluateDelta3AuxCommandGuard` で直近リザーブ実送信ログと現在状態を比較し、Planへ反映状態を埋める。
3. `delta3AuxCommandSuppressedReason` に、反映失敗中の同一リザーブ候補抑止を追加する。
4. フロントエンドの型と表示を追加する。
5. 単体テストを追加する。
6. backend/frontend の検証を実行する。
7. `codex review --uncommitted` で実装レビューを回す。

## レビューポイント

- 反映失敗判定が「同じリザーブ候補」だけを止めていること。
- AC充電上限を下げる安全制御が止まらないこと。
- 実機writeゲートを緩めていないこと。
- UIで「反映待ち」と「反映失敗」が区別できること。
- DB migration を不要に保てていること。

## 検証コマンド

```bash
cd backend && rtk go test ./...
cd frontend && rtk npm run build
rtk git diff --check
rtk codex review --uncommitted
```

## 運用メモ

実装後、Plus2で `backupReserveSoc=0` が続く場合でも、同じリザーブ設定の連続送信は止まる。画面上は反映失敗として見えるため、次の作業は EcoFlow private API のリザーブpayload再検証になる。
