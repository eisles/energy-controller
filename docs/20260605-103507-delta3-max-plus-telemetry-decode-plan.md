# DELTA 3 Max Plus Telemetry Decode Plan

## Goal

DELTA 3 Max Plus の EcoFlow private MQTT telemetry を、制御判断に使える read-only 状態として安定して取得できるようにする。

現時点では DELTA 3 Max Plus の電源が入っていないため、この計画では実機 write や実値検証は行わない。まずは既存 decoder と status reader の構造を整理し、電源投入後に短時間で raw payload 収集、field mapping、画面確認まで進められる実装計画を定義する。

## Non-Goals

- DELTA 3 Max Plus への新しい write command は追加しない。
- AC1 / AC2 の ON/OFF write command は追加しない。
- AC出力保護チャンネルの write command は追加しない。
- RIVER 2 の telemetry decode は今回の範囲外とする。
- 実機writeの安全ゲート、最小コマンド間隔、重複抑制、確認値は変更しない。
- 電源OFF状態の機器から取れない値を推測で補完しない。

## Current State

- DELTA 3 Max Plus は charging device master に登録済みで、`device_type=DELTA_3_MAX_PLUS`、`status_source=ecoflow_private_mqtt` として扱える。
- write target selection は `DELTA_3_MAX_PLUS` を候補にできるが、実際の制御判断には telemetry の可読性が必要。
- 既存 decoder は `cmdFunc=254 cmdId=21` の display upload から、SOC、AC入力、AC出力、PV入力、AC1、AC2、AC出力保護などの field を読む構造を持つ。
- 現在の `/api/status` の制御診断では、DELTA 3 Max Plus が `EcoFlow private MQTT returned no supported telemetry fields for DELTA_3_MAX_PLUS` となる場合がある。
- 既存の AC group 計画では、AC1/AC2/AC出力保護について以下の field mapping が整理済み。
  - `field=367`: AC1
  - `field=971`: AC2
  - `field=1539`: AC出力保護チャンネル
  - `field=47`: AC1/AC2 全体状態の候補

## Data Needed After Power-On

DELTA 3 Max Plus の電源投入後、以下の read-only raw capture を取得する。

1. 電源ON直後、AC1/AC2 OFF、PV接続ありの状態
2. AC1 ON / AC2 OFF
3. AC1 OFF / AC2 ON
4. AC1 ON / AC2 ON
5. AC出力保護チャンネル AC1
6. AC出力保護チャンネル AC2
7. PV入力がある状態とない状態
8. AC入力がある状態とない状態

取得時は device serial number、認証情報、private MQTT credential をログや計画ファイルへ書かない。raw payload を保存する場合も、機器識別子は既存のマスク処理またはテスト fixture 用の置換値を使う。

## Implementation Plan

### 1. Read-only raw capture support

`backend/cmd/ecoflow-delta3-probe` または既存の private MQTT read path に、DELTA 3 Max Plus の unsupported telemetry を調査しやすい出力を追加する。

- decoded fields count
- unsupported message count
- header `cmdFunc` / `cmdId`
- unknown field number
- wire type
- numeric value preview
- bytes length preview

この出力は read-only に限定し、`--execute` や write gate とは分離する。

### 2. Unsupported telemetry diagnostics

`hasReadablePrivateMQTTTelemetry` が false の場合でも、完全に値が取れないのか、未対応 field だけが多いのかを区別できるようにする。

追加候補:

- `decodedMessages`
- `unsupportedMessages`
- `supportedFieldNames`
- `unknownFieldCount`
- `lastUnsupportedSummary`

API 表示では secrets や raw payload 本体は出さず、運用者が「電源OFF」「payloadは来ているが未対応」「MQTT接続不可」を区別できる粒度に留める。

### 3. DELTA 3 Max Plus profile mapping

既存の `ecoflowprivate.Profile` と `DecodeSnapshot(deviceType, ...)` を使い、`DELTA_3_MAX_PLUS` 固有の mapping を必要最小限だけ追加する。

優先して確定する項目:

- SOC
- AC入力W
- AC出力W
- PV入力W
- AC充電上限W
- 最大充電SOC
- 最低放電SOC
- バックアップリザーブSOC
- Energy Backup ON/OFF
- AC1 ON/OFF
- AC2 ON/OFF
- AC出力保護チャンネル
- grid bypass / pass-through 判定に必要な値

### 4. UI and diagnostics

充電機器ステータスと制御診断で、DELTA 3 Max Plus の取得状態を明確にする。

- `取得不可` の場合は理由を短く表示する。
- `電源OFF相当`、`MQTT接続不可`、`unsupported telemetry` を分ける。
- AC1/AC2/AC出力保護が取れた場合は、単一の `AC出力` 表示だけに潰さず併記する。
- 制御候補に入らない場合は、`no command candidate` だけでなく、telemetry不足なのか、priorityなのか、controlEnabledなのかを表示する。

### 5. Control integration after read verification

telemetry が安定して読めることを確認してから、既存の補助充電制御へ接続する。

- 売電時の充電候補に入れる。
- DELTA 3 Plus 2 と DELTA 3 Max Plus の priority を明示する。
- 1台 write target の既存方針を維持する。
- 複数台同時配分は別計画に分ける。

## Files Likely To Change

- `backend/internal/ecoflowprivate/codec.go`
- `backend/internal/ecoflowprivate/status.go`
- `backend/internal/ecoflowprivate/profile.go`
- `backend/internal/ecoflowprivate/codec_test.go`
- `backend/cmd/ecoflow-delta3-probe/main.go`
- `backend/internal/api/delta3_status_handler.go`
- `backend/internal/api/delta3_status_handler_test.go`
- `backend/internal/domain/status.go`
- `frontend/lib/types.ts`
- `frontend/lib/display-labels.ts`
- `frontend/components/Dashboard.tsx`
- `frontend/components/StatusCards.tsx`

## Safety Boundaries

- 計画段階では実機 write を行わない。
- 実装段階でも最初は read-only decoder と diagnostics に限定する。
- 未確認 payload を write command として送信しない。
- `ENABLE_REAL_CONTROL`、`SIMULATION_MODE`、`MOCK_MODE`、`AUTO_CONTROL_ENABLED`、確認値、private MQTT write gate の既存判定は維持する。read-only telemetry 実装では、これらの値を実制御向けに変更しない。
- device serial number、API token、access key、secret key、private MQTT credential は計画、テスト、ログに残さない。

## Review Points

- 電源OFFの機器を unsupported telemetry と誤判定し続けないか。
- payload は来ているが未対応 field だけ、という状態を診断できるか。
- DELTA 3 Plus 2 の既存 decode と制御表示を壊していないか。
- AC2-only 状態を AC出力OFF と誤判定しないか。
- RIVER 2 が誤って write target にならないか。
- raw capture が secrets や serial number を漏らさないか。

## Verification Plan

電源OFFでも実行できる確認:

- `cd backend && rtk go test ./internal/ecoflowprivate ./internal/api`
- `cd backend && rtk go test ./cmd/ecoflow-delta3-probe`
- `cd frontend && rtk npm run build`
- `rtk git diff --check`

電源投入後に実行する確認:

- `cd backend && rtk go run ./cmd/ecoflow-delta3-probe --device-type DELTA_3_MAX_PLUS`
- `curl -fsS http://localhost:${HTTP_PORT:-8080}/api/devices/statuses`
- `curl -fsS http://localhost:${HTTP_PORT:-8080}/api/status`
- ブラウザで `充電機器ステータス` と `制御診断` を確認する。

実運用環境が `HTTP_PORT=18085` の場合は `http://localhost:18085/...` で同じ endpoint を確認する。

## Power-On Checklist

DELTA 3 Max Plus の電源を入れたら、次の順で確認する。

1. `/api/devices/statuses` で DELTA 3 Max Plus が `available=true` になるか。
2. SOC / AC入力W / AC出力W / PV入力W のどれかが取れるか。
3. AC1/AC2 の ON/OFF 操作をアプリ側で行い、read-only 値だけを比較する。
4. AC出力保護チャンネルを AC1/AC2 に切り替え、`acOutputProtectionChannel` の値を確認する。
5. 取得値が安定してから、controlEnabled と priority を運用設定として調整する。

## Deferred Work

- AC1 / AC2 ON/OFF の write command 実装
- AC出力保護チャンネル write command 実装
- DELTA 3 Plus 2 と DELTA 3 Max Plus への同時余剰配分
- RIVER 2 telemetry decode
- 長期ログから機種別 field mapping を自動提案する仕組み
