# Phase 7: DELTA Pro 3 AC出力OFFイベント追跡 実装計画

## 目的

DELTA Pro 3 の AC出力がOFFになった場合に、次回から原因追跡できるようにする。

現在は `power_logs.ecoflow_diagnostics_json.outputPowerOffMemory` で「過去にAC出力OFFが発生した可能性」は見えるが、画面上で目立たず、直前コマンド・温度・出力W・AC出力状態を1つのイベントとして追えない。

## 非目的

- DELTA Pro 3 の AC出力を自動でON/OFFする実機writeは追加しない。
- EcoFlow APIの未確定なAC出力制御payloadは実装しない。
- DELTA 3 Plus の補助制御ロジックは変更しない。
- 既存の余剰追従・夜間充電アルゴリズムの判断式は変更しない。

## 現状

- DELTA Pro 3 の診断値は `current_status.ecoflow_diagnostics_json` と `power_logs.ecoflow_diagnostics_json` に保存されている。
- `outputPowerOffMemory=true` は取得できている。
- `surplus_control_command_logs` には直前の余剰追従コマンドが保存されている。
- 通知基盤は `notification_logs` と Slack webhook ベースの `notify` package がある。
- 画面の上部ステータスカードでは、AC出力OFF履歴や直前状況を専用表示していない。

## データ/API契約

### 新規ドメイン

`domain.Pro3ACOutputEvent` を追加する。

保存する主な項目:

- `measured_at`
- `event_type`: 初期値は `ac_output_off_memory`
- `output_power_off_memory`
- `battery_soc`
- `battery_input_w`
- `battery_output_w`
- `ac_charge_limit_w`
- `grid_w`, `import_w`, `export_w`
- `bms_max_cell_temp`, `bms_max_mos_temp`
- `ac_out_freq`
- `ac_out_dsg_pow_max`
- `previous_command_kind`
- `previous_command_sent`
- `previous_command_measured_at`
- `previous_command_target_ac_charge_limit_w`
- `previous_command_target_backup_reserve_soc`
- `previous_command_reason`
- `message`
- `created_at`

### DB

新規テーブル `pro3_ac_output_event_logs` を追加する。

イベントは同じ `event_type + measured_at` で重複保存しない。初期実装では、状態が `outputPowerOffMemory=true` のときに30分以上前の同種イベントがなければ保存する。

### API

`GET /api/pro3/ac-output-events?limit=20&offset=0` を追加する。

レスポンスはページング形式:

```json
{
  "items": [],
  "limit": 20,
  "offset": 0,
  "total": 0
}
```

`GET /api/status` には直近イベントを `pro3AcOutputEvent` として含める。

### 通知

既存の通知基盤を使い、`outputPowerOffMemory=true` を検出したら `notification_logs` に `kind=pro3_ac_output_off` で保存する。

`NOTIFICATION_ENABLED=true` かつ Slack 設定がある場合だけ Slack 送信する。

通知内容:

- AC出力OFF履歴を検知した時刻
- SOC
- AC出力W
- AC充電上限
- 温度
- 直前コマンド種別と理由

通知は cooldown を設け、同じフラグで頻繁に送らない。

## 画面

トップ画面に DELTA Pro 3 の AC出力監視ブロックを追加する。

表示内容:

- 現在の `outputPowerOffMemory`
- SOC
- AC入力/出力
- AC充電上限
- 温度
- 直近イベント時刻
- 直前コマンド
- 直前コマンド理由

OFF履歴がある場合は警告色で表示する。

## 安全境界

- 本実装は read-only 監視と通知のみ。
- EcoFlow write は追加しない。
- 既存の `ENABLE_REAL_CONTROL` / `SIMULATION_MODE` の安全境界は変更しない。
- 通知送信は外部writeだが、既存通知基盤の設定に従う。未設定時はDBログのみ。
- 秘密情報、SN、token はログや通知に出さない。

## 実装手順

1. `domain.Status` に `Pro3ACOutputEvent` を追加する。
2. `domain.Pro3ACOutputEvent` と page 型を追加する。
3. migration に `pro3_ac_output_event_logs` を追加する。
4. repository を追加し、insert / latest / list page / latest within cooldown を実装する。
5. `recordStatus` で `outputPowerOffMemory=true` を検出し、直前 `surplus_control_command_logs` と診断値をまとめてイベント化する。
6. 既存 `notification_logs` を使って `pro3_ac_output_off` 通知を保存・送信する。
7. `GET /api/pro3/ac-output-events` を追加する。
8. `GET /api/status` に直近イベントを含める。
9. frontend types と `StatusCards` に AC出力監視ブロックを追加する。
10. Go unit test を追加する。
11. frontend build で型と表示を検証する。

## レビュー観点

- `outputPowerOffMemory=true` が継続していても通知/イベントが連投されないこと。
- 直前コマンドは `surplus_control_command_logs` から読み、AC出力OFF操作と誤認させない表示にすること。
- `nil` 診断値や古いDBでも起動できること。
- 実機writeを追加していないこと。
- DB migration が既存DBに対して安全に再実行できること。

## 検証コマンド

```bash
cd backend && rtk go test ./...
cd frontend && rtk npm run build
rtk git diff --check
```

必要に応じてローカルサーバを再起動し、以下を確認する。

```bash
rtk proxy curl -fsS http://localhost:18085/api/status | jq '.pro3AcOutputEvent'
rtk proxy curl -fsS 'http://localhost:18085/api/pro3/ac-output-events?limit=5' | jq .
```

## 運用メモ

今回の変更で次回のAC出力OFF発生時は、少なくとも「検知時刻」「温度」「出力W」「SOC」「直前コマンド」を同じイベントとして追える。原因を本体保護・瞬間負荷・直前制御・通信/DB問題に切り分けやすくなる。
