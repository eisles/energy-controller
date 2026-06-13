# EcoFlow Developer MQTT サイクル数取得 実装計画

## Goal

EcoFlow のサイクル数を、未命名の private MQTT protobuf field ではなく、Developer MQTT quota の名前付き key から read-only で取得して画面に表示する。

対象はまず DELTA Pro 3 と、将来同じ Developer MQTT quota で `cycles` 系 key が得られる EcoFlow 機器。DELTA 3 Plus / DELTA 3 Max Plus の既存 private MQTT による SOC、AC 入出力、AC1/AC2、充電上限などの取得と制御は維持する。

## Non-Goals

- 実機 write コマンドは追加しない。
- private MQTT の `field=37` など未命名 raw field をサイクル数として再採用しない。
- Developer MQTT の set topic 実装は行わない。
- Home Assistant 連携や TarasKhust 実装の全面移植は行わない。

## Current State

- EcoFlow Cloud REST `GET /iot-open/sign/device/quota/all` は DELTA Pro 3 の SOC、入出力、SOH、設定値を返すが、現在確認した直接レスポンスには `cycles` が含まれていない。
- `backend/internal/ecoflow/quota_adapter.go` は Cloud quota に `cycles` 系 key が存在する場合だけ `cycleCount` を採用できる。
- 直前コミットで、EcoFlow app private MQTT の未命名 protobuf `field=37` はサイクル数として扱わないよう修正済み。
- TarasKhust/ecoflow-api-mqtt の `const.py` では DELTA Pro 3 の `bmsCycles` が `key: "cycles"`、`mqtt_only: True` と定義されている。
- 同実装の MQTT topic は `/open/{certificateAccount}/{sn}/quota` で、payload は JSON の直下、`params`、または `param` に quota data が入る形を扱っている。

## Data/API Contract

新しい read-only adapter は Developer MQTT quota payload から次の key を探す。

- `cycles`
- `bmsCycles`
- `bmsBattCycles`
- `bms_bmsStatus.cycles`
- `hs_yj751_bms_slave_addr.1.cycles`

取得できた場合:

```json
{
  "cycleCount": 123,
  "cycleCountSource": "ecoflow_developer_mqtt_quota"
}
```

取得できない場合:

```json
{
  "cycleCount": null,
  "cycleCountSource": ""
}
```

Developer MQTT quota の raw payload は通常保存しない。デバッグ CLI では secrets と full SN を出さずに key 一覧と採用値だけを出せるようにする。

## Files Likely To Change

- `backend/internal/ecoflowdeveloper/`
  - Developer MQTT quota の topic、payload parser、read-only client を追加する。
- `backend/internal/api/delta3_status_handler.go`
  - Cloud status / private status に対して、名前付き Developer MQTT quota のサイクル数だけを補完する。
- `backend/internal/api/delta3_status_handler_test.go`
  - Cloud に cycle がない場合、Developer MQTT quota で補完すること。
  - Developer MQTT quota に cycle がない場合、nil のままにすること。
- `backend/internal/domain/status.go` / `backend/internal/ecoflowprivate/status.go`
  - 既存 field の扱いを必要に応じて整理する。
- `backend/cmd/ecoflow-developer-mqtt-probe/`
  - read-only probe CLI を追加する。実装確認と運用調査用。
- `frontend/components/StatusCards.tsx`
  - 既存のサイクル数表示は維持し、取得元ラベルに Developer MQTT quota を追加する。
- `README.md` / `.env.example`
  - Developer MQTT quota read-only の設定と注意点を追記する。

## Safety Boundaries

- read-only の MQTT subscribe のみ行う。
- `/open/{certificateAccount}/{sn}/set` は実装しない。
- 既存の `ENABLE_REAL_CONTROL`、`SIMULATION_MODE`、`CONFIRM_ECOFLOW_WRITE` に関係する write 経路は触らない。
- `.env` の credential、token、password、full serial number はログ、計画、テスト、画面に出さない。
- Developer MQTT quota が失敗しても、既存の SOC / AC 制御 status は壊さず `cycleCount` を nil のままにする。
- MQTT 接続回数を増やしすぎないよう、既存 status cache / timeout 方針に合わせる。

## Implementation Steps

1. Developer MQTT quota adapter を追加する。
   - 既存 `ecoflowprivate.AuthClient` / `MQTTInfo` / Paho transport を再利用できる範囲で使う。
   - topic は `/open/{certificateAccount}/{sn}/quota`。
   - payload は JSON として decode し、`params`、`param`、直下 data の順で quota map を取り出す。
2. quota parser を単体テストする。
   - `{"params":{"cycles":12}}`
   - `{"cycles":12}`
   - `{"params":{"bms_bmsStatus.cycles":34}}`
   - 文字列数値、float 数値、欠損、異常値を確認する。
3. read-only probe CLI を追加する。
   - `--sn`、`--timeout` を受け取り、write は持たない。
   - 出力は `cycleCount`、`cycleCountSource`、受信 key 数、採用 key、error のみにする。
4. API status 補完を追加する。
   - Cloud status が `cycleCount == nil` のときだけ Developer MQTT quota を試す。
   - private MQTT status の SOC などは既存どおり。
   - 失敗時は lastError を status 全体の failure にせず、サイクル数未取得として扱う。
5. Frontend の source label に `ecoflow_developer_mqtt_quota` を追加する。
6. README / `.env.example` を更新する。

## Review Points

- 未命名 protobuf field が cycleCount に戻っていないこと。
- Developer MQTT quota の失敗が制御判断や status 全体を壊さないこと。
- write topic を publish するコードが入っていないこと。
- secrets や full SN がログや画面に出ないこと。
- テストが network なしで通ること。

## Verification Commands

```sh
cd backend && GOCACHE=$PWD/.gocache rtk go test ./...
cd frontend && rtk npm run build
rtk git diff --check
rtk codex review --uncommitted
```

必要に応じて read-only 実機 probe:

```sh
cd backend && GOCACHE=$PWD/.gocache rtk go run ./cmd/ecoflow-developer-mqtt-probe --sn '<redacted>' --timeout 20s
```

## Rollback / Operational Notes

- 問題があれば Developer MQTT quota 補完呼び出しを外せば、既存の Cloud REST / private MQTT status に戻せる。
- サイクル数は制御判断に使わず表示専用にする。
- 実機確認は read-only probe と `/api/devices/statuses` の表示確認に限定する。

