# cycleCountCandidate 診断表示 実装計画

## Goal

private MQTT で観測できているサイクル数らしきフィールドを、正式な `cycleCount` とは分離した `cycleCountCandidate` として API と画面に読み取り専用表示する。

## Non-goals

- `cycleCountCandidate` を正式な `cycleCount` として採用しない。
- 制御判断、充放電優先度、料金最適化には使わない。
- 実機 write、EcoFlow 設定変更、追加の MQTT subscribe / command は行わない。
- シリアル番号、認証情報、トークンをログや画面に追加しない。

## Current State

- `Delta3StatusResponse` は正式値として `cycleCount` / `cycleCountSource` を返している。
- private MQTT の生フィールド診断は `telemetryDiagnostics.fieldSummaries` として返している。
- private MQTT のサイクル候補抽出ロジックは `ecoflowprivate.CycleFieldCandidatesFromFields` にある。
- ただし UI では候補値が `-` のままで、どの候補フィールドを見ているか分かりにくい。

## API Contract

`Delta3StatusResponse` に以下を追加する。

```json
{
  "cycleCountCandidate": {
    "value": 79,
    "source": "ecoflow_private_mqtt_candidate",
    "cmdFunc": 254,
    "cmdId": 21,
    "field": 427,
    "confidence": "candidate",
    "reason": "private MQTT candidate; not accepted cycleCount"
  }
}
```

候補は `cycleCount` が未取得の機器だけに出す。`cycleCount` が Developer MQTT quota などで正式取得できている場合は、混乱を避けるため候補表示は出さない。

## Candidate Selection

既存の `telemetryDiagnostics.fieldSummaries` 相当の情報から `CycleFieldCandidatesFromFields` を使い、機種別に観測済みフィールドだけを選ぶ。

- `DELTA_3_PLUS`: `254/21/427` を優先し、なければ `254/22/280`
- `DELTA_3_MAX_PLUS`: `32/50/85` を優先し、なければ `32/50/86`
- 値は `0 < value <= 5000` の整数だけを候補にする
- 未知機種や未知フィールドは候補表示しない

## Frontend

- `Delta3Status` に `cycleCountCandidate` 型を追加する。
- 機器ステータス行に `サイクル候補` を追加する。
- 表示例: `79 回候補 / private MQTT 254/21/427`
- 正式値ではないことが分かるように `候補` を含める。

## Safety Boundaries

- 読み取り済み telemetry から候補を整形するだけで、実機 write は追加しない。
- 候補値を制御ロジックに渡さない。
- 候補値が見つからない場合は `-` 表示にする。
- private MQTT のフィールド名は未確定扱いとし、正式サイクル数とは別フィールドで維持する。

## Implementation Steps

1. `backend/internal/api/ecoflow_device_status_handler.go` に `CycleCountCandidateResponse` を追加する。
2. private MQTT status の `FieldSummaries` から候補を選ぶ helper を追加する。
3. `mapDelta3Status` で正式 `CycleCount` がない場合だけ候補を設定する。
4. API unit test で DELTA 3 Plus / DELTA 3 Max Plus / 正式 cycleCount ありの場合を確認する。
5. `frontend/lib/types.ts` に型を追加する。
6. `frontend/components/StatusCards.tsx` に `サイクル候補` 表示と formatter を追加する。

## Verification

- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./internal/api ./internal/ecoflowprivate`
- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./...`
- `cd frontend && rtk npm run build`
- `rtk codex review --uncommitted`

## Rollback

追加フィールドと UI 表示を戻せば既存の正式 `cycleCount` 表示に戻る。DB migration や実機設定変更はない。
