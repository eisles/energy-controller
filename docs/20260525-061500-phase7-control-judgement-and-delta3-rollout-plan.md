# Phase 7 制御判定表示と DELTA 3 Plus 段階実制御 計画

## 目的

買電時に「なぜ AC 充電上限が下がらない/抑制されているように見えるのか」を画面とログで判断できるようにし、制御判断の英語混じり表示を日本語化する。あわせて DELTA 3 Plus 補助制御を read-only から実制御へ進められるかを、安全ゲートを崩さずに判定できる状態にする。

## 現状の調査結果

- `power_logs` の直近では 2026-05-25 00:10 JST 時点で買電 2440W、DELTA Pro 3 の AC 充電上限 1500W、夜間充電計画は `NIGHT_CHARGE_WINDOW`。
- 夜間充電計画は推奨深夜 SOC 48% を満たすため、23:00-07:00 は `nightPlanOwnsEnergyControl` が true になり、余剰追従のコマンド評価はスキップされる。
- そのため `surplus_control_command_logs` は 22:59 付近で止まって見え、画面上は「余剰追従が今なぜ動いていないか」が分かりにくい。
- `lastDecisionReason` は古い汎用 decision engine の `command suppressed by minimum interval or command diff` と、夜間計画/余剰計画の文字列を連結しており、実際には夜間充電計画が制御を所有しているケースでも誤読しやすい。
- DELTA 3 Plus 補助計画は `RECOVERING` で、買電中は 100W へ戻す方針。ただし `ECOFLOW_DELTA3_EXECUTE=false` / `ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=false` 等の安全ゲートが閉じている場合は送信しない。

## 非目的

- `.env` の実機書き込み設定をこのタスクで有効化しない。
- EcoFlow/Nature Remo の認証情報や SN をコード、計画、ログに保存しない。
- 最小送信間隔やヒステリシスを無効化しない。
- 23:00-07:00 の夜間充電方針そのものを変更しない。

## 安全境界

- 実機書き込みは既存条件を維持する。
  - `ENABLE_REAL_CONTROL=true`
  - `SIMULATION_MODE=false`
  - `MOCK_MODE=false`
  - `AUTO_CONTROL_ENABLED=true`
  - `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`
  - 試験期限内
  - DELTA 3 Plus は追加で `ECOFLOW_DELTA3_READ_ENABLED=true`
  - DELTA 3 Plus 自動書き込み重複確認として `ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=true`
  - DELTA 3 Plus 実送信として `ECOFLOW_DELTA3_EXECUTE=true`
  - DELTA 3 Plus private API 書き込み明示許可として `ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=true`
- 既定値は引き続き mock/simulation/read-only。
- 実装レビュー完了までコミットしない。

## 変更方針

### 1. 買電時の AC 充電上限抑制理由を追えるようにする

- 夜間充電計画が制御を所有しているため余剰追従コマンド評価をスキップした場合、`surplus_control_command_logs` に read-only の監査ログを残す。
- ログの `suppressed_reason` は `night charge plan owns control` とし、`decision_reason` には夜間充電計画が優先中であることを入れる。
- このログは `would_write=false` / `command_sent=false` / `dry_run=true` / `error_message` 空とし、`LatestSurplusControlWriteCandidateLog` の対象にならないようにする。
- これにより、余剰追従ログが古い時刻で止まって見える問題を避ける。

### 2. 判定表示を日本語化する

- `frontend/lib/display-labels.ts` に複合判断理由を日本語化する関数を追加する。
- `lastDecisionReason` の表示は raw string ではなく日本語化済み文字列を表示する。
- 余剰追従/DELTA 3 Plus/夜間充電ログの `decisionReason`、`suppressedReason`、`modeGuardReason` も同じ翻訳ヘルパーを通す。
- 追加翻訳対象:
  - `importing from grid, do not charge`
  - `command suppressed by minimum interval or command diff`
  - `surplus dry-run plan:`
  - `night dry-run plan:`
  - `night charge plan owns control`
  - `night charge settings already match plan`
  - `target daytime solar forecast is strong; keep night charging modest`
  - `current SOC is already above the recommended night target`
  - `duplicate night charge command candidate`
  - `night charge command suppressed by minimum interval`
  - `mode status unavailable`
  - `TOU mode is already enabled`
  - `energy modes already disabled`

### 3. DELTA 3 Plus 実制御へ進める判定を見える化する

- DELTA 3 Plus 補助計画の画面表示では、現在の `strategyState`、推奨 AC 充電上限、現在 AC 充電上限、抑制理由を日本語で表示する。
- 実制御ゲートが閉じている場合は「送信候補あり」ではなく、どの gate で止まっているかを明示する。
- このタスクでは gate を変更しない。実制御に進める場合は、ユーザーが `.env` を明示的に変更し、再起動後にログで確認する。

## 変更予定ファイル

- `backend/cmd/server/main.go`
  - 夜間充電計画が制御を所有して余剰追従評価をスキップした場合の監査ログを追加。
- `backend/cmd/server/main_test.go`
  - 夜間充電計画所有時に余剰追従 skip ログが保存されることをテスト。
  - skip ログが `error_message` 空で、write candidate として取得されないことをテスト。
- `frontend/lib/display-labels.ts`
  - 複合判断理由と追加 guard/reason の日本語化。
- `frontend/components/StatusCards.tsx`
  - `lastDecisionReason` 表示を日本語化。
- `frontend/components/*CommandLogTable.tsx`
  - `decisionReason` / `modeGuardReason` の日本語化。

## 実装手順

1. `surplus_control_command_logs` に夜間計画所有時の skip ログを保存する helper を追加する。
2. `recordStatus` で `nightPlanOwnsControl == true` の場合に skip ログを保存する。
3. 既存の write candidate 検索に影響しないことを確認する。
4. `display-labels.ts` に `decisionSummaryLabel` を追加し、複合文字列を安全に置換する。
5. UI の該当箇所を `decisionSummaryLabel` に置き換える。
6. backend/frontend のテストと build を実行する。
7. `rtk codex review --uncommitted` で実装レビューを回し、指摘があれば修正して再レビューする。

## 確認コマンド

```bash
(cd backend && rtk go test ./...)
(cd frontend && rtk npm run build)
rtk git diff --check
rtk codex review --uncommitted
```

必要に応じて、稼働中サーバを再ビルド/再起動して以下も確認する。

```bash
rtk /usr/bin/curl -s http://localhost:18085/api/status
rtk /usr/bin/curl -s 'http://localhost:18085/api/surplus-control/commands?limit=5&offset=0'
rtk /usr/bin/curl -s 'http://localhost:18085/api/delta3/aux-commands?limit=5&offset=0'
```

## ロールバック

- この変更は表示と監査ログ追加が中心のため、実機書き込みの gate は変わらない。
- 問題があれば該当コミットを revert し、DB に残った skip ログは `would_write=false` / `command_sent=false` のため制御判断には使われない。

## レビュー観点

- 夜間計画所有時の skip ログが write candidate として扱われないこと。
- 実機書き込み gate が緩んでいないこと。
- 日本語化で raw 値の意味が変わらないこと。
- frontend build が通ること。
