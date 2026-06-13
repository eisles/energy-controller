# EcoFlow DELTA 3 系サイクル数フィールド特定計画

## Goal

DELTA 3 Plus、DELTA 3 Max Plus、DELTA 3 Plus 2 のサイクル数取得に向けて、EcoFlow Developer MQTT quota と private MQTT telemetry からサイクル数に関係しそうなキーだけを安全に観測できるようにする。

今回の成果は「フィールド特定用の読み取り probe」と「観測結果を機種別対応表へ反映するための足場」までとする。未確認フィールドを正式な `cycleCount` として UI や制御判断へ採用しない。

## Non-goals

- EcoFlow 実機への write command は追加しない。
- `cycSoh` など意味が未確定の値を正式なサイクル数として採用しない。
- 生の MQTT payload、シリアル番号、認証情報、トークンをログ、docs、画面へ出さない。
- 充放電制御、料金最適化、AC 出力制御は変更しない。

## Current State

- DELTA Pro 3 は Developer MQTT quota の `cycles` から `cycleCount=65` を取得できている。
- DELTA 3 Max Plus は Developer MQTT quota message を受信できているが、現在の `cycleCandidates` では候補キーが見つかっていない。
- DELTA 3 Plus と DELTA 3 Plus 2 は直近の短時間 watch で Developer MQTT quota message を受信できていない。
- `backend/internal/ecoflowdeveloper/quota.go` は確定キーと候補キーを抽出できるが、候補の範囲が `cycle` / `cycSoh` 系に狭い。
- `backend/cmd/ecoflow-developer-mqtt-probe` は `keyNames` を出せるが、キー一覧が多い場合にサイクル調査に必要な関連キーだけを見やすく整理できない。

## Files Likely To Change

- `backend/internal/ecoflowdeveloper/quota.go`
  - quota payload から `bms` / `cyc` / `cycle` / `soh` を含む関連キーを抽出する read-only helper を追加する。
  - 値は scalar のみを対象にし、payload 全体や credentials は出さない。
- `backend/internal/ecoflowdeveloper/quota_test.go`
  - 関連キー抽出、上限、確定 `cycleCount` への非反映を検証する。
- `backend/cmd/ecoflow-developer-mqtt-probe/main.go`
  - watch output に `diagnosticKeys` または同等の安全な関連キー一覧を追加する。
  - 既存の通常 mode / watch mode の read-only 表示を維持する。
- `backend/cmd/ecoflow-developer-mqtt-probe/main_test.go`
  - JSON 出力に関連キー一覧が含まれることを検証する。
- `docs/20260613-132725-ecoflow-cycle-count-source-matrix.md`
  - 今回の調査方法と、実機確認で確定しない限り DELTA 3 系は未対応のまま扱う旨を追記する。

## Data And API Contracts

Probe の watch message に、次のような読み取り専用診断フィールドを追加する。

```json
{
  "mode": "read-only-watch",
  "event": "message",
  "topicKind": "quota",
  "quotaKeyCount": 42,
  "cycleCandidates": [],
  "diagnosticKeys": [
    {"key": "bms_bmsStatus.soh", "value": 99},
    {"key": "bms_slave.cycSoh", "value": 7}
  ],
  "write": {
    "wouldSend": false,
    "sent": false,
    "reason": "read-only Developer MQTT quota/status subscribe"
  }
}
```

Rules:

- `diagnosticKeys` は `bms` / `cyc` / `cycle` / `soh` を含む flatten path のみ。
- 値は数値または bool の scalar のみ。非数値 string は、パス名だけでは秘密情報でないと保証できないため出力しない。
- 最大件数を設け、巨大 payload をそのまま表示しない。
- `cycleCount` は既存の確定キーに一致した場合だけ設定する。
- `diagnosticKeys` は UI の正式サイクル数や制御判断には使わない。

## Safety Boundaries

- 変更は read-only probe と docs に限定する。
- EcoFlow write path、制御判断、DB migration、Docker runtime flag は変更しない。
- `.env`、SN、Access Key、Secret Key、private email/password は出力しない。
- 実機確認コマンドを実行する場合も `--watch-sec` の読み取り購読だけにする。

## Implementation Steps

1. `quota.go` に安全な関連キー抽出型と helper を追加する。
2. `QuotaMessage` と probe watch output に関連キー一覧を追加する。
3. `cycleCandidates` と同様、関連キーは調査用の出力だけに留める。
4. 単体テストで以下を検証する。
   - `bms` / `cyc` / `cycle` / `soh` を含む nested key が flatten される。
   - 関連しない key は出ない。
   - object/array payload は値として出さない。
   - 関連キーがあっても確定 `cycleCount` は設定されない。
5. 既存の対応表 docs に、次の調査手順と注意点を追記する。
6. 必要なら実機読み取り probe を短時間実行し、DELTA 3 Max Plus の関連キーが出るか確認する。

## Review Points

- 未確定値が正式 `cycleCount` に混入していないか。
- 生 payload、SN、credential を出す経路がないか。
- probe 追加が制御処理や通常 API を壊していないか。
- 出力名が用途を誤解させないか。

## Verification Commands

```sh
cd backend && GOCACHE=$PWD/.gocache rtk go test ./internal/ecoflowdeveloper ./cmd/ecoflow-developer-mqtt-probe
cd backend && GOCACHE=$PWD/.gocache rtk go test ./...
rtk codex review --uncommitted
```

Optional read-only live check:

```sh
cd backend && GOCACHE=$PWD/.gocache rtk go run ./cmd/ecoflow-developer-mqtt-probe --sn '<redacted>' --watch-sec 120
```

## Rollback And Operational Notes

問題があれば、追加した関連キー抽出 helper、probe JSON フィールド、対応表追記を戻せば、既存の `cycleCount` 取得挙動に戻る。

この変更は読み取り専用であり、実機制御の開始、停止、充電量、放電量には影響しない。
