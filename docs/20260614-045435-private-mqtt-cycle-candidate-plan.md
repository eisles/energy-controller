# DELTA 3 系 private MQTT サイクル数候補調査計画

作成日時: 2026-06-14 04:54:35 JST

## Goal

DELTA 3 Max Plus / DELTA 3 Plus / DELTA 3 Plus 2 の private MQTT protobuf telemetry から、サイクル数に関係しそうな raw field を安全に観測できるようにする。

今回の成果は、既存の `ecoflow-delta3-probe` に read-only の候補抽出出力を追加し、実機観測で「候補 field があるか」を確認できる足場を作ることまでとする。

## Non-goals

- 実機 write command は追加しない。
- 未確認の raw field を正式な `cycleCount` として UI/API/制御判断へ採用しない。
- raw payload、シリアル番号、認証情報、token、メールアドレスを保存または表示しない。
- 充放電制御、料金最適化、AC 出力制御は変更しない。
- DB migration は行わない。

## Current State

- DELTA Pro 3 は Developer MQTT quota の named key `cycles` でサイクル数取得済み。
- DELTA 3 Max Plus は Developer MQTT quota message は届くが、直近観測では `powGetAcIn` / `powGetAcOutList` / `powInSumW` / `powOutSumW` のみで、サイクル候補は出ていない。
- DELTA 3 Plus / DELTA 3 Plus 2 は直近 60 秒の Developer MQTT quota watch で quota message が届いていない。
- `backend/cmd/ecoflow-delta3-probe` には `--inspect-fields` があり、private MQTT protobuf の raw field を read-only で観測できる。
- `backend/internal/ecoflowprivate` は raw field summary を保持できるが、サイクル数候補だけを安全に抽出する専用出力はまだない。

## Files Likely To Change

- `backend/internal/ecoflowprivate/inspect.go`
  - `SnapshotField` からサイクル数候補らしい field を抽出する helper と型を追加する。
- `backend/internal/ecoflowprivate/status.go`
  - 必要なら read-only の候補 summary 型を追加する。ただし正式 `CycleCount` には反映しない。
- `backend/cmd/ecoflow-delta3-probe/main.go`
  - `--inspect-cycle-candidates` のような read-only flag を追加し、通常 probe / offline fixture の JSON に候補一覧を出す。
- `backend/internal/ecoflowprivate/inspect_test.go`
  - 候補抽出、範囲制限、正式 `CycleCount` 非反映を検証する。
- `backend/cmd/ecoflow-delta3-probe/main_test.go`
  - probe output に候補一覧が含まれることを検証する。
- `docs/20260613-132725-ecoflow-cycle-count-source-matrix.md`
  - private MQTT 候補調査の手順と注意点を追記する。

## Data/API Contract

`ecoflow-delta3-probe` に read-only 診断出力を追加する。

```json
{
  "mode": "read-only",
  "status": {
    "deviceType": "DELTA_3_MAX_PLUS"
  },
  "cycleFieldCandidates": [
    {
      "messageIndex": 0,
      "cmdFunc": 254,
      "cmdId": 21,
      "field": 123,
      "value": 12,
      "reason": "unknown private MQTT varint field in plausible cycle-count range"
    }
  ],
  "cycleFieldSummary": [
    {
      "cmdFunc": 254,
      "cmdId": 21,
      "field": 123,
      "samples": 3,
      "values": [12],
      "stable": true,
      "presentInAll": true,
      "reason": "stable candidate across all read-only samples; still investigation-only"
    }
  ],
  "write": {
    "wouldSend": false,
    "sent": false,
    "reason": "read-only probe",
    "cycleCandidateRepeats": 3
  }
}
```

Rules:

- 候補は investigation-only とし、`status.cycleCount` へは入れない。
- 値は protobuf inspector が既に安全化した scalar 表現のみを使う。
- 候補抽出は `wire=0` varint の非負整数を中心にし、異常に大きい値、bool 相当だけの値、既知の SOC / 温度 / 電力 field は除外する。
- `cmdFunc/cmdId/field/wire/value` のみを出し、SN や raw payload は出さない。
- 最大件数を設ける。
- raw field 番号は private API 仕様として未確定なので、候補のまま扱う。
- `--inspect-cycle-candidates-repeat` で複数回 read-only 観測し、`cycleFieldSummary` に安定性を出す。
- `--raw-output-dir` と repeat を併用した場合は、全サンプルの raw を sample 番号付きファイル名で保存する。

## Candidate Heuristic

候補として出す条件:

- `wire=0` で `value` が 0 以上の整数として parse できる。
- 値が実用的なサイクル数範囲に収まる。初期値は `2 <= value <= 5000`。
- `cmdFunc/cmdId/field` が既に decode 済みの SOC、電力、温度、残時間、ON/OFF 制御 field と明確に分かっているものは除外する。
- 同一 `(cmdFunc, cmdId, field, wire, value)` は重複排除する。

候補として出さない条件:

- `wire=5` float32、`wire=2` bytes、parse 不能な値。
- 値が `0` または `1` の bool 的な field。
- 現在既知の `SOC` / `AC入力` / `AC出力` / `PV入力` / `AC充電上限` / `backupReserve` / `AC output` 系 field。

## Safety Boundaries

- この変更は read-only probe と docs のみ。
- `--execute` や write guard の挙動は変更しない。
- 本番制御ループ、charging device master、Docker 設定は変更しない。
- 実機観測は必要な場合も private MQTT get/read のみで、set publish は行わない。
- 候補値は正式な `cycleCount` ではないことを docs と JSON 名で明確にする。

## Implementation Steps

1. `ecoflowprivate` に `CycleFieldCandidate` 型と `CycleFieldCandidatesFromFields` / `CycleFieldCandidatesFromSnapshot` helper を追加する。
2. 既知 field 除外表を helper 内に閉じ込める。
3. `ecoflow-delta3-probe` に `--inspect-cycle-candidates` flag を追加する。
4. `--inspect-cycle-candidates-repeat` / `--inspect-cycle-candidates-interval` を追加し、複数回観測の安定性 summary を出す。
5. read-only probe / offline fixture の JSON に `cycleFieldCandidates` を追加する。
6. 単体テストで以下を検証する。
   - 小さな非負整数 field は候補になる。
   - bool 値、既知 SOC/電力/温度/control field、float/bytes は候補にならない。
   - 複数サンプルで安定候補と変動候補を区別できる。
   - 候補が出ても `CycleCount` は nil のまま。
7. 対応表 docs に、private MQTT 候補調査は採用前の観測段階であることを追記する。
8. 必要なら DELTA 3 Max Plus / DELTA 3 Plus 系に対して read-only probe を短時間実行し、候補 field を確認する。

## Review Points

- raw payload や serial-like value を出す経路がないか。
- 候補 field を正式 `cycleCount` と誤解させる名前や表示になっていないか。
- `--execute` 系 write path に影響していないか。
- repeat と `--raw-output-dir` を併用した場合、raw 保存対象と JSON summary がずれていないか。
- 既知 field 除外が広すぎて調査候補を潰していないか。
- テストが安全境界と非反映を十分に固定しているか。

## Verification Commands

```sh
cd backend && GOCACHE=$PWD/.gocache rtk go test ./internal/ecoflowprivate ./cmd/ecoflow-delta3-probe
cd backend && GOCACHE=$PWD/.gocache rtk go test ./...
rtk codex review --uncommitted
```

Optional read-only live check:

```sh
cd backend && GOCACHE=$PWD/.gocache rtk go run ./cmd/ecoflow-delta3-probe --sn '<redacted>' --device-type DELTA_3_MAX_PLUS --inspect-cycle-candidates --inspect-cycle-candidates-repeat 3 --inspect-cycle-candidates-interval 3s
```

## Rollback And Operational Notes

問題があれば、追加した候補抽出 helper、probe JSON フィールド、docs 追記を戻すだけで既存挙動に戻る。

この変更はサイクル数を確定実装するものではなく、次の観測と対応表更新のための安全な調査用 tooling である。
