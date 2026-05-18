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

EcoFlow Developer の公式 document page は確認できるが、ブラウザ検索上は JavaScript 必須で本文を直接取得できなかった。

- `https://developer.ecoflow.com/us/document/...`
- `https://developer-eu.ecoflow.com/us/document/...`

そのため、この調査では公式ページ本文を直接 source of truth として確定できていない。実装時は developer account で公式 docs を開き、該当 device document を人間が確認する必要がある。

ただし、複数の公開実装・ドキュメントで以下の共通形が確認できる。

```text
GET /iot-open/sign/device/quota/all?sn=<serial>
PUT /iot-open/sign/device/quota
```

Shelly knowledge base の EcoFlow 連携メモでは、write body の envelope として以下の形が示されている。

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

同じ記事では署名について、request parameters を flatten して sort し、`accessKey` / `nonce` / `timestamp` と合わせて HMAC-SHA256 する、と説明されている。

### 3. DELTA Pro 3 に近い candidate payload

Symcon community の DELTA Pro 3 事例では、以下の candidate params が言及されている。

```json
{
  "params": {
    "cfgPlugInInfoAcInChgPowMax": 400
  }
}
```

同 thread では `cmdId=17`、`cmdFunc=254`、`dirDest=1`、`dirSrc=1`、`dest=2`、`needAck=true` の envelope も DELTA Pro 3 向けとして言及されている。

ただし、thread 内では PHP 実装で signature error が出ており、同じ JSON が Java では動作したという報告に留まる。よって、この情報は「候補」であって「この repo で即実装してよい確定値」ではない。

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
- AC charge power の write param 候補は `cfgPlugInInfoAcInChgPowMax`

### まだ不確実

- DELTA Pro 3 で `cfgPlugInInfoAcInChgPowMax` が常に有効か
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

以下は公式 docs と実機確認なしに実装しない。

- real `PUT /iot-open/sign/device/quota` call
- `cfgPlugInInfoAcInChgPowMax` の実送信
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

## 参考 URL

- EcoFlow Developer document: https://developer.ecoflow.com/us/document
- EcoFlow Developer EU document: https://developer-eu.ecoflow.com/us/document
- Shelly knowledge base EcoFlow integration: https://kb.shelly.cloud/knowledge-base/kbuca-ecoflow-works-with-shelly
- openHAB EcoFlow binding: https://www.openhab.org/addons/bindings/ecoflow/
- Symcon EcoFlow API thread: https://community.symcon.de/t/ecoflow-api/130047?page=2
- EcoFlow API SDK/schema repo: https://github.com/rustyy/ecoflow-api
- EcoFlow DELTA Pro 3 manual mirror: https://www.manualslib.com/guide/3619472/ecoflow-delta-pro-3-manual.html
