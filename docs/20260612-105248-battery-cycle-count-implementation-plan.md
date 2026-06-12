# Battery Cycle Count Implementation Plan

## Goal

各 EcoFlow 充電機器のサイクル数を読み取り専用ステータスとして取得し、機器ステータス画面に表示する。

対象は既存の充電機器ステータス取得経路に限定する。

- DELTA Pro 3: EcoFlow Cloud quota に `cycles` 系の値があれば表示し、Cloud にない場合は同じ SN/type を使う read-only private MQTT 補完でサイクル数だけ取得する。
- DELTA 3 Plus / DELTA 3 Max Plus / RIVER 2: EcoFlow private MQTT の telemetry から既知または候補のサイクル数を取得する。
- 取得できない機器は `-` 表示とし、制御判断には使わない。

## Non-Goals

- EcoFlow への新しい write command は追加しない。
- サイクル数を使った充放電制御の変更はしない。
- 未確定の private MQTT field を断定的に寿命診断へ使わない。
- EcoFlow の認証情報、シリアル番号、トークンをログや計画に残さない。

## Current State

`TarasKhust/ecoflow-api-mqtt` の調査では、サイクル数に関して以下のキーが使われている。

- DELTA Pro 3: `cycles` は MQTT-only sensor とされている。
- DELTA 2 系: `bms_bmsStatus.cycles`
- DELTA Pro Ultra: `hs_yj751_bms_slave_addr.1.cycles`

現在の本リポジトリでは以下の状態。

- `backend/internal/domain/status.go`
  - `BatteryStatus` と `Status` に cycle count field はない。
- `backend/internal/ecoflow/quota_adapter.go`
  - Cloud quota から SOC、入出力、充電上限などを取り込む。
  - `cycles` 系は取り込んでいない。
- `backend/internal/ecoflowprivate/status.go`
  - private MQTT の `Status` に cycle count field はない。
- `backend/internal/ecoflowprivate/codec.go`
  - display upload / BMS heartbeat / CMS heartbeat を一部 decode している。
  - raw telemetry summary は diagnostics として出せる。
- `backend/internal/api/delta3_status_handler.go`
  - `Delta3StatusResponse` に cycle count field はない。
  - Cloud / private MQTT の両方から機器ステータスを返す。
- `frontend/lib/types.ts` と `frontend/components/StatusCards.tsx`
  - 機器ステータスに cycle count 表示はない。

## Data/API Contract

Backend API に optional field を追加する。

```json
{
  "cycleCount": 123,
  "cycleCountSource": "ecoflow_cloud_quota|ecoflow_private_mqtt|ecoflow_private_mqtt_candidate"
}
```

ルール:

- `cycleCount` は `number | null`。
- `cycleCountSource` は値が取れたときだけ返す。
- 正式キーで取れた場合は `ecoflow_cloud_quota` または `ecoflow_private_mqtt`。
- field 番号から推定した値は `ecoflow_private_mqtt_candidate` とし、画面上も候補であることを分かる表現にする。

## Implementation Steps

1. Domain/API 型を拡張する。
   - `domain.BatteryStatus` に `CycleCount *int` を追加する。
   - `api.Delta3StatusResponse` に `CycleCount *int` と `CycleCountSource string` を追加する。
   - frontend の `Delta3Status` 型にも同じ field を追加する。

2. EcoFlow Cloud quota adapter を拡張する。
   - `cycles`, `bmsCycles`, `bmsBattCycles`, `bms_bmsStatus.cycles` など既知候補から整数を拾う。
   - 取得できた場合だけ `CycleCount` をセットする。
   - diagnostic quota の対象にも `cycle` / `soh` を含める。

3. EcoFlow private MQTT status を拡張する。
   - `ecoflowprivate.Status` に `CycleCount *int` と `CycleCountSource string` を追加する。
   - merge 処理で維持する。
   - display upload の known field として `cycles` 相当が確認できる場合に decode を追加する。
   - 既知 field が確定できない場合は、telemetry field summary から安全な candidate 抽出 helper を追加する。
   - candidate は整数、現実的な範囲、温度/SOC/電力に見える値を避ける条件を入れる。

4. DELTA Pro 3 の read-only private MQTT 補完を追加する。
   - 既存の Cloud status は維持し、Cloud で `CycleCount` が取れない場合だけ補完を試す。
   - 補完は device master の SN/type と既存 private MQTT 認証を使い、サイクル数以外の Pro 3 status 置換には使わない。
   - 補完が失敗しても Cloud status は返し、サイクル数は `-` のままにする。
   - 既存 cache / timeout / read-only 境界を維持し、頻繁な追加リクエストにならないようにする。

5. API mapping を更新する。
   - Cloud status mapping で `CycleCount` を返す。
   - private MQTT status mapping で `CycleCount` と source を返す。
   - status unavailable 時は返さない。

6. UI に表示する。
   - 機器ステータスの詳細行に `サイクル数` を追加する。
   - source が candidate の場合は `候補` と分かるラベルにする。
   - 値がない場合は `-`。

7. Tests を追加・更新する。
   - Cloud quota adapter: `cycles` を拾えること。
   - private MQTT status merge: cycle count が保持されること。
   - API mapping: Cloud/private の response に cycle count が出ること。
   - DELTA Pro 3 Cloud status に read-only private MQTT cycle 補完が反映されること。
   - frontend build で型崩れがないこと。

## Safety Boundaries

- 読み取り専用の field 追加のみ。
- 実機 write path、充放電制御、command interval、real-control gate は変更しない。
- private MQTT の candidate は制御判断に使わない。
- DELTA Pro 3 の private MQTT 補完はサイクル数の read-only 取得だけに限定し、充放電状態や制御判断の source of truth は既存どおり Cloud status とする。
- raw telemetry に secret や serial を含めない既存方針を維持する。

## Review Points

- サイクル数が取れない場合に UI/API が壊れない。
- 未確定 field を断定表示しない。
- Cloud API と private MQTT の責務が domain 層へ漏れない。
- 既存 status API の後方互換性を壊さない。

## Verification Commands

```bash
(cd backend && GOCACHE=$PWD/.gocache rtk go test ./...)
(cd frontend && rtk npm run build)
rtk git diff --check
rtk codex review --uncommitted
```

Runtime verification:

```bash
curl -s http://localhost:18085/api/devices/statuses
curl -s http://localhost:18085/api/delta3/status
```

## Rollback / Operational Notes

- 追加 field は optional なので、問題があれば UI 表示だけを外しても既存制御には影響しない。
- candidate 判定が不安定なら `cycleCountSource=ecoflow_private_mqtt_candidate` の表示を非表示化し、diagnostics のみに戻す。
- 実機制御の再起動や write command はこの作業範囲外。
