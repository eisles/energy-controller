# EcoFlow device status handler ファイル名汎用化計画

## Goal

`backend/internal/api/delta3_status_handler.go` / `delta3_status_handler_test.go` のファイル名を、現在の役割に合う汎用名へ変更する。

推奨変更:

- `backend/internal/api/delta3_status_handler.go` -> `backend/internal/api/ecoflow_device_status_handler.go`
- `backend/internal/api/delta3_status_handler_test.go` -> `backend/internal/api/ecoflow_device_status_handler_test.go`

計画レビューで見つかった `cycleCountCandidate` の信頼性問題も同じ handler 周辺の未コミット差分に含まれるため、あわせて修正する。private MQTT の候補判定は、表示用に切り詰めた `fieldSummaries` ではなく、全フィールドから抽出した候補一覧を優先して使う。

## Non-goals

- `Delta3StatusResponse`、`Delta3StatusReader`、`Delta3StatusCard` などの公開型・関数・API path は今回 rename しない。
- `/api/delta3/status` の互換性は変更しない。
- EcoFlow 実機 write、制御判断、private MQTT 通信、認証情報の扱いは変更しない。
- 既存の historical docs 全体を一括更新しない。
- `cycleCountCandidate` は正式な cycle count としては扱わず、従来どおり候補表示に留める。

## Current State

- 対象ファイルは当初 DELTA 3 系中心だったが、現在は以下を扱っている。
  - DELTA 3 / DELTA 3 Plus
  - DELTA 3 Max Plus
  - DELTA Pro 3 の cloud / Developer MQTT cycle 補完
  - RIVER 2 の unsupported diagnostics
- 前回作業の `cycleCountCandidate` 実装が未コミットのまま同ファイルに入っている。
- 計画レビューで、`cycleCountCandidate` が切り詰め済みの `FieldSummaries` だけを走査しており、80 件より後ろに候補 field があると欠落する問題が見つかった。
- Go は同一 package 内ではファイル名が API に影響しないため、ファイル名 rename は低リスク。

## Scope

1. Go ファイル名を `ecoflow_device_status_handler*` へ変更する。
2. `ecoflowprivate.Status` に、全 private MQTT field から抽出した cycle field candidate を保持し、API handler はこれを `fieldSummaries` より優先して使う。
3. 未コミットの直近計画ファイル `20260614-072550-cycle-count-candidate-diagnostics-plan.*` にある旧ファイル名参照だけ、新ファイル名へ更新する。
4. 既存の古い計画 docs は履歴として残し、今回の機械的一括置換対象にしない。

## Safety Boundaries

- 実機 write path は変更しない。
- 制御ロジック、充放電ロジック、料金最適化ロジックは変更しない。
- シリアル番号、API token、認証情報は触らない。
- `cycleCountCandidate` は診断表示用の候補であり、制御判断や実機 write には使わない。

## Implementation Steps

1. `git status --short` で未コミット差分を確認する。
2. private MQTT adapter の `Status` に cycle field candidate 一覧を保持する。
3. `appendTelemetryFieldSummaries` で切り詰め前の全 field から candidate を抽出する。
4. API handler の `privateCycleCountCandidate` は candidate 一覧を優先し、既存 fake / fallback のため `FieldSummaries` 走査も残す。
5. truncation されても candidate が落ちない unit test を追加する。
6. `mv` で Go ファイル名を変更する。
7. 直近の未コミット plan Markdown / HTML 内の `delta3_status_handler.go` / `_test.go` 参照を新ファイル名に更新する。
8. `rg` で旧ファイル名の残存参照を確認する。
   - historical docs の残存は許容する。
   - current uncommitted plan と実コード内に残っていないことを確認する。

## Verification

- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./internal/api`
- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./...`
- `rtk codex review --uncommitted`

Frontend は今回変更しないが、前回の未コミット frontend 差分が残っているため、必要に応じて `cd frontend && rtk npm run build` も確認する。

## Rollback

問題があればファイル名を元へ戻し、直近 plan docs の参照だけ元に戻す。Go の package 名や public API は変更しないため、rollback はファイル名レベルで完結する。
