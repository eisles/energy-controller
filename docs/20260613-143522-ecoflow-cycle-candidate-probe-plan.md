# EcoFlow cycle candidate probe implementation plan

作成日時: 2026-06-13 14:35:22 JST

## Goal

DELTA 3 Plus / DELTA 3 Max Plus / DELTA 3 Plus 2 のサイクル数候補を、Developer MQTT quota の read-only probe で安全に観測できるようにする。

`cycSoh` 系の key は、コミュニティ実装では cycle / SOH 系の診断値として扱われている可能性がある。ただし現時点では、このリポジトリでサイクル数として採用できるほどの実機検証がない。そのため、今回の実装では `cycleCount` には採用せず、`cycleCandidates` として probe 出力にだけ表示する。

## Non-goals

- 実機 write 制御は追加しない。
- DELTA 3 Plus 系の画面/API `cycleCount` 表示はまだ有効化しない。
- `cycSoh` をサイクル数として確定採用しない。
- private MQTT の未命名 protobuf field をサイクル数として採用しない。
- raw payload、シリアル番号、Access Key、Secret Key、メールアドレス、パスワード、token は保存・表示しない。

## Current State

- `backend/internal/ecoflowdeveloper/quota.go` は確定サイクル key として `cycles`, `bmsCycles`, `bmsBattCycles`, `bms_bmsStatus.cycles`, `hs_yj751_bms_slave_addr.1.cycles` だけを採用している。
- `backend/internal/api/delta3_status_handler.go` は Developer MQTT サイクル補完を `ecoflow_delta_pro3` に限定している。
- `backend/cmd/ecoflow-developer-mqtt-probe` の watch mode は quota key 名を表示できるが、cycle-like key と値の候補を分類して表示しない。
- 直近の対応表では DELTA 3 系は「未対応、次に長めの quota watch で key を確認」と整理している。

## Data/API Contract

### Accepted cycle count

既存どおり、以下の名前付き key だけが `cycleCount` として採用される。

- `cycles`
- `bmsCycles`
- `bmsBattCycles`
- `bms_bmsStatus.cycles`
- `hs_yj751_bms_slave_addr.1.cycles`

### Candidate cycle-like values

今回追加する `cycleCandidates` は調査用の read-only 出力であり、制御判断や UI の正式サイクル数には使わない。

候補抽出ルール:

- quota payload の直下、`params`、`param` を対象にする。
- nested object は `a.b.c` の dot path に flatten する。
- key path の最終 segment または full path が以下に該当するものを候補にする。
  - `cycSoh`
  - `cycleSoh`
  - `cycles`
  - `bmsCycles`
  - `bmsBattCycles`
  - `bms_bmsStatus.cycles`
  - `hs_yj751_bms_slave_addr.1.cycles`
- 値は整数・小数・文字列をそのまま JSON 表示できる範囲で保持する。
- object / array は出力しない。
- 候補数が多い場合は上限を設ける。

例:

```json
{
  "cycleCount": null,
  "cycleCandidates": [
    {"key": "bms_bmsStatus.cycSoh", "value": 98},
    {"key": "bms_slave.cycSoh", "value": 100}
  ]
}
```

## Files Likely To Change

- `backend/internal/ecoflowdeveloper/quota.go`
- `backend/internal/ecoflowdeveloper/quota_test.go`
- `backend/cmd/ecoflow-developer-mqtt-probe/main.go`

## Safety Boundaries

- read-only Developer MQTT subscribe のみ。
- `cycleCandidates` は調査出力であり、`cycleCount` には反映しない。
- API / UI / 制御ロジックには正式値として流さない。
- raw payload と secrets は出さない。
- 実機 write topic や REST SET は使わない。

## Implementation Steps

1. `ecoflowdeveloper` adapter に cycle-like candidate 抽出用の型と helper を追加する。
2. `CycleStatusFromQuotaPayload` は既存の確定 `CycleCount` 挙動を維持しつつ、候補一覧を `CycleStatus` に持たせる。
3. `QuotaMessageFromMQTT` で watch message に candidate 情報を含める。
4. `ecoflow-developer-mqtt-probe` の watch mode の JSON 出力に `cycleCandidates` を追加する。通常 mode も確定サイクル数が取得できた場合は同じ field を出せるようにするが、確定サイクル数なしの timeout 挙動は既存どおり維持する。
5. 単体テストを追加する。
   - `cycSoh` が `cycleCount` ではなく `cycleCandidates` にだけ出ること。
   - 確定 key `cycles` は引き続き `cycleCount` として採用されること。
   - nested key が dot path で候補化されること。
6. 実機の長時間 watch は今回のコード変更後の運用確認として実施できるが、結果をドキュメントへ保存する場合も secrets/SN は記載しない。

## Review Points

- `cycSoh` を正式サイクル数として扱っていないこと。
- 既存 DELTA Pro 3 の `cycles` 取得が壊れていないこと。
- 候補抽出が raw payload や secrets を出さないこと。
- `cycleCandidates` が probe 出力に限定され、制御判断に入っていないこと。

## Verification Commands

```sh
cd backend && GOCACHE=$PWD/.gocache rtk go test ./internal/ecoflowdeveloper ./cmd/ecoflow-developer-mqtt-probe
rtk codex review --uncommitted
```

必要に応じて read-only 実機確認:

```sh
cd backend
GOCACHE=$PWD/.gocache rtk go run ./cmd/ecoflow-developer-mqtt-probe --sn '<redacted>' --watch-sec 120
```

## Rollback Notes

この変更は read-only probe 出力の拡張だけなので、問題があれば `cycleCandidates` の型・helper・CLI 出力追加を戻せば既存の `cycleCount` 採用挙動に戻せる。
