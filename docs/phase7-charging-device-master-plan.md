# Phase 7 充電機器マスタ管理 実装計画

## 目的

DELTA Pro 3、DELTA 3 Plus、将来の SwitchBot スマートプラグなどを、台数固定の `.env` ではなく充電機器マスタとして管理できるようにする。

この計画では、まず安全に運用できる土台として、DB に充電機器マスタを作成し、管理 API と read-only dashboard 上の編集画面を追加する。既存の DELTA Pro 3 / DELTA 3 Plus 制御ロジックは今回の実装ではマスタ参照へ切り替えない。

## 非目標

- EcoFlow / SwitchBot への新しい実機 write API は追加しない。
- 既存の DELTA Pro 3 余剰追従制御、夜間充電制御、DELTA 3 Plus 補助制御の実行条件は変更しない。
- private API 認証情報、token、device SN、SwitchBot token は DB やコードに保存しない。
- 複数機器への余剰電力配分アルゴリズムは今回実装しない。次フェーズの入力データとしてマスタを整備する。

## 現状

- DELTA Pro 3 は既存の `ECOFLOW_DEVICE_SN` などの環境変数を前提に制御される。
- DELTA 3 Plus は `ECOFLOW_DELTA3_*` と `DELTA3_AUX_*` の環境変数を前提に、単体の補助バッテリーとして扱われる。
- もう一台 DELTA 3 Plus を扱うには、台数固定の環境変数追加より、機器マスタで能力値と優先順位を持つ方が拡張しやすい。

## データ設計

新規テーブル `charging_devices` を追加する。

| カラム | 型 | 内容 |
| --- | --- | --- |
| `id` | INTEGER PK | 内部ID |
| `name` | TEXT | 表示名 |
| `kind` | TEXT | `ecoflow_delta_pro3` / `ecoflow_delta3_plus` / `switchbot_plug` / `manual` |
| `provider` | TEXT | `ecoflow` / `switchbot` / `manual` |
| `role` | TEXT | `primary` / `auxiliary` / `manual_auxiliary` |
| `credential_ref` | TEXT | 認証情報参照名。SN や token は保存しない |
| `enabled` | INTEGER | 管理対象として有効 |
| `control_enabled` | INTEGER | 将来の自動制御候補。初期値 false |
| `priority` | INTEGER | 余剰割当の優先順位 |
| `min_charge_w` | INTEGER | 最小充電W |
| `max_charge_w` | INTEGER | 最大充電W |
| `charge_step_w` | INTEGER | 充電Wの刻み |
| `capacity_wh` | INTEGER | 容量Wh |
| `target_soc` | INTEGER | 目標SOC |
| `reserve_soc` | INTEGER | 最低確保SOC |
| `supports_soc_read` | INTEGER | SOC読み取り可否 |
| `supports_ac_charge_limit` | INTEGER | AC充電上限設定可否 |
| `supports_on_off` | INTEGER | ON/OFF制御可否 |
| `notes` | TEXT | 運用メモ |
| `created_at` / `updated_at` | TEXT | 監査用時刻 |

### 初期データ

DB が空の場合だけ、以下を seed する。

- DELTA Pro 3
  - kind: `ecoflow_delta_pro3`
  - provider: `ecoflow`
  - role: `primary`
  - credential_ref: `ecoflow_pro3_primary`
  - min/max/step: 400 / 1500 / 100
  - control_enabled: false
- DELTA 3 Plus
  - kind: `ecoflow_delta3_plus`
  - provider: `ecoflow`
  - role: `auxiliary`
  - credential_ref: `ecoflow_delta3_primary`
  - min/max/step: 100 / 1500 / 100
  - control_enabled: false
- 手動補助バッテリー枠
  - kind: `manual`
  - provider: `manual`
  - role: `manual_auxiliary`
  - credential_ref: `manual_auxiliary`
  - control_enabled: false

## API 設計

新規 API を追加する。

- `GET /api/settings/charging-devices`
  - 充電機器マスタ一覧を `priority ASC, id ASC` で返す。
- `POST /api/settings/charging-devices`
  - 充電機器を作成または更新する。
  - `id` がある場合は更新、ない場合は作成。
- `DELETE /api/settings/charging-devices/{id}`
  - マスタ行を削除する。
  - 削除しても過去ログや制御ログは消さない。

バリデーション:

- `name`, `kind`, `provider`, `role`, `credential_ref` は空不可。
- `min_charge_w >= 0`
- `max_charge_w >= min_charge_w`
- `charge_step_w >= 1`
- `capacity_wh >= 0`
- `target_soc`, `reserve_soc` は 0-100。
- `priority >= 1`
- `kind/provider/role` は許可リスト制。

## Frontend 設計

設定ブロックに `充電機器マスタ` カードを追加する。

表示:

- 有効台数、制御候補台数、優先順位。
- 各機器の kind、provider、role、min/max/step、capacity、SOC目標、対応機能。
- `credential_ref` は表示してよいが、SN や token は入力させない。

編集:

- 右ドロワーで作成・編集・削除する。
- `control_enabled` は表示・編集できるが、実機 write には直結しない旨を画面上に明示する。

## 安全境界

- 既存の実機制御 gate は維持する。
- 今回追加する API は DB マスタ更新のみで、EcoFlow / SwitchBot への外部 API 呼び出しを行わない。
- 初期値では `control_enabled=false` とし、マスタ追加だけで制御が増えないようにする。
- device SN、token、password、access key は保存・表示しない。
- 将来の制御実装では、`credential_ref` から環境変数または安全な secret store を解決する。

## 実装手順

1. `domain` に `ChargingDevice` 型を追加する。
2. `store` に `charging_devices` migration、seed、repository を追加する。
3. `api` に `charging_devices_handler.go` と router 登録を追加する。
4. backend repository / handler / migration tests を追加する。
5. frontend に `ChargingDevice` 型、API helper、`ChargingDevicePanel` を追加する。
6. `ControlPanel` の設定ブロックにカードを追加する。
7. build/test を実行する。
8. 実装レビューで指摘がなくなるまで修正する。

## 確認コマンド

```sh
cd backend && rtk go test ./...
cd frontend && rtk npm run build
rtk git diff --check
rtk codex review --uncommitted
```

## 運用メモ

- 既存環境変数は当面そのまま残す。
- `charging_devices` は複数台制御のための source of truth 候補だが、今回の実装では制御ロジックからは参照しない。
- 次フェーズで余剰配分 planner を作る場合、`enabled=true` かつ `control_enabled=true` の機器だけを候補にする。
