# Phase 7 EcoFlow Write API 調査メモ

## 目的

Phase 7 の EcoFlow 実制御に入る前に、DELTA Pro 3 の write path を安全に設計するための調査計画と実行結果を残す。

このメモは実装指示ではなく、Phase 7 実装前の判断材料である。EcoFlow 実 API への書き込みはまだ実装しない。

## 調査計画

1. 既存実装を確認する
   - `backend/internal/ecoflow/signed_client.go`
   - `backend/internal/ecoflow/quota_adapter.go`
   - `implementation-plan.md` の Phase 5 / Phase 7 記述
2. EcoFlow Developer API の公開情報を確認する
   - 公式 developer page は JavaScript app のため、検索結果と URL 到達性を確認する
   - 直接読めない項目は「未確認」として扱う
3. 既存 OSS / integration 実装を確認する
   - openHAB EcoFlow binding
   - EcoFlow API SDK / schema repo
   - Home Assistant / community 実装
4. DELTA Pro 3 に近い write payload の実例を収集する
   - endpoint
   - HTTP method
   - signing 対象
   - command envelope
   - candidate params
5. Phase 7 実装可否を分類する
   - 確定できたこと
   - まだ不確実なこと
   - mock-first で先に実装できること
   - 実機確認なしでは進めないこと

## 調査実行結果

### 1. 既存 repo の状態

現在の実装は read-only である。

- `SignedClient.GetBatteryStatus` は `GET /iot-open/sign/device/quota/all` を呼ぶだけ
- signing は query params + `accessKey` + `nonce` + `timestamp` を HMAC-SHA256 している
- `BatteryStatusFromQuotas` は以下を読む
  - `cmsBattSoc` / `bmsBattSoc`
  - `powInSumW`
  - `powOutSumW`
  - `plugInInfoAcInChgPowMax`
- `ecoflow.Client` interface は現時点で `GetBatteryStatus` のみ

既存コードには write API 呼び出し、EcoFlow command payload、control start/stop API はない。

### 2. 公開 API / endpoint の確認

EcoFlow Developer の DELTA Pro 3 document は JavaScript app として配信される。通常の HTML 取得では `You need to enable JavaScript to run this app.` だけが返るが、配信 bundle 内の `assets/new_md/deltaPro3.md` chunk から本文を確認できた。

公式 document では HTTP communication mode の Set & Get Quota として以下が示されている。

```text
PUT: /iot-open/sign/device/quota: SetCmdRequest
POST: /iot-open/sign/device/quota: GetCmdRequest, GetCmdResponse
```

SetCmdRequest の envelope は以下の形で、AC 充電 W 以外の複数 command でも共通している。

```json
{
  "sn": "<serial>",
  "cmdId": 17,
  "cmdFunc": 254,
  "dirDest": 1,
  "dirSrc": 1,
  "dest": 2,
  "needAck": true,
  "params": {}
}
```

Shelly knowledge base の EcoFlow 連携メモでも、write endpoint と envelope は同じ形で確認できる。同じ記事では署名について、request parameters を flatten して sort し、`accessKey` / `nonce` / `timestamp` と合わせて HMAC-SHA256 する、と説明されている。

### 3. DELTA Pro 3 に近い candidate payload

EcoFlow Developer の DELTA Pro 3 document では、Maximum AC input power for charging の SetCmdRequest として以下の params が示されている。

```json
{
  "params": {
    "cfgPlugInInfoAcInChgPowMax": 3000
  }
}
```

対応する Set Reply example では `cfgPlugInInfoAcInChgPowMax`、`configOk=true`、`actionId=54` が返る形になっている。

Symcon community の DELTA Pro 3 事例でも同じ params が言及されている。ただし community thread は signature error の話も含むため、公式 document の補助情報として扱う。

### 4. 既存 integration から見える制御軸

openHAB EcoFlow binding は Delta 2 / Delta 2 Max 向けに `ac-input#set-charging-power` を writable channel として公開している。これは DELTA Pro 3 の直接証拠ではないが、EcoFlow power station 系で AC charge power target を write target として扱う precedent になる。

Shelly knowledge base の STREAM Ultra 実例では、以下の params が control outputs として説明されている。

- `cfgFeedGridMode`
- `cfgBackupReverseSoc`
- `cfgEnergyStrategyOperateMode`

これは STREAM Ultra 向けであり、DELTA Pro 3 の AC charging power control とは別物として扱う必要がある。

### 5. DELTA Pro 3 製品仕様上の注意

DELTA Pro 3 manual では、物理的な charge speed switch に `ADJ` と `FAST` があり、`ADJ` は EcoFlow app で設定した customized power を使う、と説明されている。

このため、AC charge power write を実装しても、実機側の物理 switch / app mode / firmware state によって反映条件が変わる可能性がある。

## 現時点の結論

### 確度が高い

- read API は現在の実装どおり `GET /iot-open/sign/device/quota/all` でよい
- write endpoint 候補は `PUT /iot-open/sign/device/quota`
- write envelope 候補は `cmdId=17` / `cmdFunc=254` / `dirDest=1` / `dirSrc=1` / `dest=2` / `needAck=true`
- AC charge power の read quota は `plugInInfoAcInChgPowMax`
- AC charge power の write param は EcoFlow Developer DELTA Pro 3 document 上では `cfgPlugInInfoAcInChgPowMax`

### まだ不確実

- stop/minimize charging に使うべき正式 param
- backup reserve の write param が DELTA Pro 3 で `cfgBackupReverseSoc` 相当か
- request body を含む PUT signing の正確な flatten ルール
- EU / US host の選択を device/account から自動判定できるか
- API server-side rate limit
- EcoFlow app mode / physical charge speed switch / firmware state による write 反映条件

## Phase 7 実装方針

### 先に実装してよい

実 API へ送らない範囲で、以下は実装してよい。

1. write adapter interface

```go
type WriteClient interface {
    SetACChargePower(ctx context.Context, watts int) error
    StopOrMinimizeCharging(ctx context.Context) error
}
```

2. mock write client
   - 送信予定 payload を記録するだけ
   - network access なし

3. guard logic
   - `ENABLE_REAL_CONTROL=true`
   - `SIMULATION_MODE=false`
   - `MOCK_MODE=false`
   - `auto_control_enabled=true`

4. command suppression
   - `MIN_COMMAND_INTERVAL_SEC`
   - `MIN_COMMAND_DIFF_W`

5. logging
   - `command_sent=false`
   - `actual_command_w`
   - guard reason
   - would-send payload summary

### まだ実装しない

以下は実機確認なしに実装しない。

- real `PUT /iot-open/sign/device/quota` call
- backup reserve write
- stop/minimize の real payload
- auto continuous real control

## 実機確認前チェックリスト

- developer account で DELTA Pro 3 device document を開き、write command の正式 payload を確認する
- app で charge speed switch / customized charging power の反映条件を確認する
- `ENABLE_REAL_CONTROL=false` で would-send log だけ出ることを確認する
- `SIMULATION_MODE=false` でも `ENABLE_REAL_CONTROL=false` なら送信されないことを確認する
- real write は 1 command 限定で開始する
- 実行後、`ENABLE_REAL_CONTROL=false` に戻す

## 追加確認: AC 充電 W payload builder

2026-05-18 時点で `https://developer-eu.ecoflow.com/us/document/deltaPro3` の配信 bundle を確認し、DELTA Pro 3 の Maximum AC input power for charging が公式 document に載っていることを確認した。

```text
PUT /iot-open/sign/device/quota

cmdId=17
cmdFunc=254
dirDest=1
dirSrc=1
dest=2
needAck=true
params.cfgPlugInInfoAcInChgPowMax=<watts>
```

このため、Phase 7 の次段階では以下だけを実装した。

- `backend/internal/ecoflow` に AC charge power payload builder を追加
- JSON envelope と `params.cfgPlugInInfoAcInChgPowMax` を unit test で固定
- real `PUT` 呼び出し、署名付き body 送信、実機 write adapter への接続は未実装のまま維持

この builder は公式 document の payload 形に基づく。ただし実機送信に進む前に、対象アカウント / 対象 device / physical charge speed switch / firmware state で 1 command 限定の実機検証が必要である。

## 追加確認: PUT signing request builder

2026-05-18 時点で、実送信しない範囲で `PUT /iot-open/sign/device/quota` の request builder と signing params flatten を追加した。

実装した範囲:

- command payload を JSON body に marshal する
- body fields を署名用 params に flatten する
- nested params は `params.cfgPlugInInfoAcInChgPowMax=1000` のような dotted key にする
- `accessKey` / `nonce` / `timestamp` / `sign` header を request に設定する
- unit test で署名対象 params、JSON body、HTTP method、headers を固定する

この段階でまだ実装していなかった範囲:

- `httpClient.Do` による real PUT 送信
- `WriteClient.SetACChargePower` から signed PUT request builder への接続
- real response parsing
- command_sent=true の実機ログ

この段階では EcoFlow への実 API 書き込みは発生していない。その後、`SignedWriteClient` と one-shot CLI の範囲に限って real PUT 送信経路を追加した。server / 管理画面 / backend API からの実機書き込みは引き続き未接続である。

## 追加確認: one-shot 実機検証 CLI

2026-05-18 時点で、実機へ送る経路を server / 管理画面から分離し、手動 CLI の 1 command 限定に閉じる。

実装した範囲:

- `backend/cmd/ecoflow-write-test` を追加する
- `--execute` なしでは dry-run message のみ出し、read / write request を送らない
- `--execute` ありでも、以下の条件が全て揃わない限り request を送らない
  - `MOCK_MODE=false`
  - `SIMULATION_MODE=false`
  - `ENABLE_REAL_CONTROL=true`
  - `AUTO_CONTROL_ENABLED=false`
  - `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`
  - EcoFlow access key / secret key / device SN が空でない
- write 前に read-only `GetBatteryStatus` を実行し、現在の `ACChargeLimitW` が `--expected-current-limit` と一致しない場合は write しない
- 条件一致時にだけ `SetACChargePower(ctx, watts)` を 1 回呼ぶ
- `SignedWriteClient` guard に manual one-shot フラグを追加し、自動制御 disabled のまま手動 1 回実行を許可する

まだ実装していない範囲:

- server `cmd/server` への real write adapter 注入
- frontend / backend API からの実機書き込み
- 自動連続制御での real write

実際の変更設定を送るタイミングは、この CLI 差分のレビューと commit 後に、直前の状態確認と明示承認を行った後とする。

## 実機確認: AC 充電上限 1500W -> 1000W one-shot

2026-05-18 に、one-shot CLI を使って DELTA Pro 3 の AC 充電上限を 1500W から 1000W へ 1 回だけ変更した。

実行前確認:

- 実機 read-only status で `ACChargeLimitW=1500` を確認した
- 確認時の状態は `mode=ecoflow-read` / `state=simulation`
- `ENABLE_REAL_CONTROL=false` の read-only server で確認し、server / UI / backend API からの write は使っていない

実行:

```bash
cd backend
MOCK_MODE=false \
SIMULATION_MODE=false \
ENABLE_REAL_CONTROL=true \
AUTO_CONTROL_ENABLED=false \
CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND \
go run ./cmd/ecoflow-write-test --execute --watts 1000 --expected-current-limit 1500
```

結果:

- CLI は write 前に現在値 `1500W` を確認し、`1000W` の command を 1 回だけ送信した
- CLI 出力は `sent one EcoFlow AC charge power command: 1000W (previous limit confirmed: 1500W)`
- 実行後の read-only status で `ACChargeLimitW=1000` を確認した
- 実行後確認時の `lastError` は空だった

この検証後も、以下は未接続のまま維持する。

- server `cmd/server` への real write adapter 注入
- frontend / backend API からの実機書き込み
- 自動連続制御での real write
- `StopOrMinimizeCharging` の real payload

## 追加確認: SignedWriteClient の単体実装

2026-05-18 時点で、`SignedWriteClient` を `backend/internal/ecoflow` に追加した。

実装した範囲:

- `SetACChargePower(ctx, watts)` で AC 充電 W payload を組み立てる
- `newSignedPUTRequest` を使って signed PUT request を作る
- response body の `code` / `message` を parse し、`code != "0"` を error にする
- adapter 内で二重 guard を行う
  - `MOCK_MODE=false`
  - `SIMULATION_MODE=false`
  - `ENABLE_REAL_CONTROL=true`
  - `AUTO_CONTROL_ENABLED=true` または manual one-shot が明示されている
  - EcoFlow access key / secret key / device SN が空でない
- `httptest` で guard NG 時は 0 request、guard OK 時は 1 request、API error 時は error になることを確認する

まだ実装していない範囲:

- `main.go` から `SignedWriteClient` を注入すること
- `StopOrMinimizeCharging` の real payload
- 自動連続制御で real write adapter を使うこと

実機へ 1500W -> 1000W の変更 command は、one-shot CLI のレビューと commit 後に、直前の read-only 確認と明示承認を経て 1 回だけ実行済みである。

## 参考 URL

- EcoFlow Developer DELTA Pro 3 document: https://developer-eu.ecoflow.com/us/document/deltaPro3
- Shelly knowledge base EcoFlow integration: https://kb.shelly.cloud/knowledge-base/kbuca-ecoflow-works-with-shelly
- openHAB EcoFlow binding: https://www.openhab.org/addons/bindings/ecoflow/
- Symcon EcoFlow API thread: https://community.symcon.de/t/ecoflow-api/130047?page=2
- FHEM EcoFlow API thread: https://forum.fhem.de/index.php?topic=140806.105
- EcoFlow API SDK/schema repo: https://github.com/rustyy/ecoflow-api
- EcoFlow DELTA Pro 3 manual mirror: https://www.manualslib.com/guide/3619472/ecoflow-delta-pro-3-manual.html
