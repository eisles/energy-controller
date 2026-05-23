# Phase 7 DELTA_3 系 private MQTT/protobuf 制御検証計画

## 目的

DELTA Pro 3 とは別系統の DELTA_3 / DELTA_3 Plus / DELTA_3 Max Plus 相当機について、EcoFlow public API で取得できない SOC / 入出力 / AC 充電設定が private MQTT/protobuf 経路で取得・制御できるかを検証する。

この計画は本番自動制御の追加ではない。まず検証用 CLI と adapter を作り、既定では read-only / dry-run のみで動かす。実機 write は、明示的な one-shot オプションと安全 gate が揃った場合に限る。

## 背景

`tolwi/hassio-ecoflow-cloud` は DELTA_3 系を public API ではなく EcoFlow app 系の login と MQTT credential 取得、`/app/{userId}/{sn}/thing/property/get` / `set` topic、protobuf payload で扱っている。

重要な切り分け:

- これは LAN 内の完全ローカル API ではない。
- EcoFlow cloud MQTT broker と app/private API に依存する。
- device 側の local MQTT redirect は成立が不安定で、現時点ではこの repo の実装対象にしない。

## 安全境界

- default は `MOCK_MODE=true` / `SIMULATION_MODE=true` / `ENABLE_REAL_CONTROL=false` を維持する。
- 検証 CLI の default は read-only probe のみ。
- write payload の生成と送信は adapter 内に閉じる。
- write は以下がすべて成立する場合だけ許可する。
  - `MOCK_MODE=false`
  - `SIMULATION_MODE=false`
  - `ENABLE_REAL_CONTROL=true`
  - `AUTO_CONTROL_ENABLED=false`
  - `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`
  - CLI に `--execute` が指定されている
  - CLI に `--allow-private-api-write` が指定されている
  - 対象 command が allowlist に入っている
- 自動制御 loop から DELTA_3 系 write は呼ばない。
- SwitchBot 制御とは接続しない。
- EcoFlow AC 出力を家庭用コンセントや系統へ戻す接続は禁止のまま。
- EcoFlow account password / token / device SN は `.env` または CLI option だけから読み、repo に保存しない。

## 検証対象

### read-only で確認する値

`tolwi/hassio-ecoflow-cloud` の DELTA_3 実装を参考に、最低限以下を probe 結果として表示する。

```text
deviceType
deviceSN
online
bms_batt_soc
cms_batt_soc
pow_in_sum_w
pow_out_sum_w
pow_get_ac_in
pow_get_ac_out
pow_get_pv
plug_in_info_ac_in_chg_pow_max
energy_backup_start_soc
energy_backup_en
```

キー名は private protobuf 由来なので domain 層へ漏らさず、`internal/ecoflowdelta3` の adapter 内で typed result へ変換する。

### write dry-run で確認する command

最初に扱う write command は以下に限定する。

```text
set_ac_charge_power
set_backup_reserve_soc
```

DELTA_3 の AC 充電 W は `tolwi/hassio-ecoflow-cloud` 上では 100-1500W、DELTA_3_1500 では 200-1500W として扱われている。検証 CLI では device type ごとの範囲を分け、未知 device type は write 不可にする。

## 実装方針

### 1. private API client

`backend/internal/ecoflowdelta3` を追加する。

責務:

- EcoFlow app login
  - `POST https://{ECOFLOW_PRIVATE_API_HOST}/auth/login`
  - email と base64 password を送る
  - token / userId を取得する
- MQTT credential 取得
  - `GET /iot-auth/app/certification`
  - MQTT URL / port / username / password を取得する
- MQTT connection
  - TLS 必須
  - client id は安定化できる設定を優先する
  - reconnect / retry は CLI 検証では最小限に留める
- topic publish / subscribe
  - get: `/app/{userId}/{sn}/thing/property/get`
  - get_reply: `/app/{userId}/{sn}/thing/property/get_reply`
  - set: `/app/{userId}/{sn}/thing/property/set`
  - set_reply: `/app/{userId}/{sn}/thing/property/set_reply`

注意:

- app/private API は非公式経路なので、public API client と同じ package に混ぜない。
- credential / token を log に出さない。
- MQTT raw payload は debug option がある場合だけ hex dump し、既定では出さない。

### 2. protobuf codec

DELTA_3 系の検証に必要な最小 protobuf codec を追加する。

候補:

- `proto/ef_delta3.proto` を repo 内に追加し、Go code generation する。
- 最小検証だけなら、最初は get snapshot request と read response parser を限定実装する。

実装する decode:

- `Delta3HeaderMessage`
- Display property upload
- Runtime property upload
- CMS heartbeat
- BMS heartbeat
- Set reply

完全 decode を目指さず、検証対象キーだけ typed result へ抽出する。未知 field は `unknownFieldsCount` などの診断値に留める。

### 3. 検証 CLI

`backend/cmd/ecoflow-delta3-probe` を追加する。

read-only 例:

```bash
go run ./cmd/ecoflow-delta3-probe \
  --sn "$ECOFLOW_DELTA3_DEVICE_SN" \
  --device-type DELTA_3 \
  --timeout 20s
```

dry-run write 例。`--execute` を付けない場合は MQTT set topic に publish しない:

```bash
go run ./cmd/ecoflow-delta3-probe \
  --sn "$ECOFLOW_DELTA3_DEVICE_SN" \
  --device-type DELTA_3 \
  --set-ac-charge-w 100
```

real one-shot 例:

```bash
go run ./cmd/ecoflow-delta3-probe \
  --sn "$ECOFLOW_DELTA3_DEVICE_SN" \
  --device-type DELTA_3 \
  --set-ac-charge-w 100 \
  --execute \
  --allow-private-api-write
```

CLI output は JSON を基本にする。

```json
{
  "mode": "read-only",
  "deviceType": "DELTA_3",
  "soc": 72,
  "inputW": 0,
  "outputW": 410,
  "acInW": 0,
  "acOutW": 410,
  "pvInW": 0,
  "acChargeLimitW": 100,
  "backupReserveSoc": 30,
  "write": {
    "wouldSend": false,
    "sent": false,
    "reason": "read-only probe"
  }
}
```

### 4. 設定

`.env.example` には秘密値を空で追加する。

```env
ECOFLOW_PRIVATE_API_HOST=api.ecoflow.com
ECOFLOW_PRIVATE_EMAIL=
ECOFLOW_PRIVATE_PASSWORD=
ECOFLOW_DELTA3_DEVICE_SN=
ECOFLOW_DELTA3_DEVICE_TYPE=DELTA_3
ECOFLOW_DELTA3_MQTT_CLIENT_ID=
```

既存 public API の `ECOFLOW_ACCESS_KEY` / `ECOFLOW_SECRET_KEY` とは用途が異なるため、名前を分ける。

### 5. テスト

必須 unit test:

- login request payload が password を base64 化する
- token / userId / MQTT credential の parse
- topic builder
- get snapshot request の protobuf envelope
- read response fixture から SOC / inputW / outputW / acChargeLimitW を抽出
- AC charge W の device type 別 validation
- write guard
  - default は送信不可
  - `--execute` なしは送信不可
  - `--allow-private-api-write` なしは送信不可
  - `CONFIRM_ECOFLOW_WRITE` なしは送信不可
  - unknown device type は送信不可
- dry-run は MQTT set topic publish を呼ばない

network を使う test は追加しない。実機検証は手動 CLI だけで行う。

## 実装ステップ

1. `docs/phase7-delta3-control-verification-plan.md` を保存し、計画レビューを通す。
2. `backend/internal/ecoflowdelta3` に config / auth / topic / guard / DTO を追加する。
3. protobuf codec の最小実装を追加する。
4. `backend/cmd/ecoflow-delta3-probe` を追加する。
5. `.env.example` と README に検証手順と注意を追加する。
6. `cd backend && go test ./...` を通す。
7. `codex review --uncommitted` を回し、指摘がなくなるまで修正する。

## 完了条件

- default 起動や既存自動制御では DELTA_3 write が絶対に走らない。
- CLI の read-only probe が network credential 不足時に安全に失敗する。
- CLI の dry-run write が payload summary だけを出し、MQTT set publish を呼ばない。
- CLI の real write は二重の CLI option と既存 write gate 相当の env がないと失敗する。
- DELTA_3 系の SOC / 入出力 / AC充電W / backup reserve の取得可否を JSON で確認できる。
- unit test が network なしで通る。
- `cd backend && go test ./...` が通る。

## 未解決事項

- DELTA_3 Plus と DELTA_3 Max Plus が EcoFlow private API 上でどの `deviceType` として返るかは実機 probe まで未確定。
- protobuf schema は community 実装由来になるため、公式互換性は保証されない。
- EcoFlow app / firmware 更新で private API や MQTT payload が変わる可能性がある。
- local MQTT broker への完全移行は今回対象外。
