# EcoFlow Private MQTT 共通 adapter / 機器 profile 化 実装計画

## 目的

EcoFlow private MQTT 連携のファイル名と責務を、DELTA 3 Plus 固有の前提から、EcoFlow private API/MQTT 共通 adapter と機器別 profile に分ける。

これにより、新しい EcoFlow 機種を追加するときに adapter 本体を増やすのではなく、機器 profile と capability/range を追加する構成にする。

## 非目的

- 実機 write の安全条件を緩めない。
- 新しい write command を追加しない。
- `.env` の認証情報名や既存 API レスポンス JSON を破壊的に変更しない。
- DB テーブル名、既存 URL、既存 UI 表示名をこの作業だけで変更しない。
- DELTA Pro 3 の cloud API 取得経路を private MQTT に移行しない。

## 現状

- `backend/internal/ecoflowdelta3` が private MQTT の認証、MQTT transport、topic、codec、write guard、コマンド payload をまとめて持っている。
- 実際にはメーカー共通と思われる責務と、DELTA 3 系の機器 profile/range が同じ package 名に混ざっている。
- 呼び出し側は `backend/internal/api/delta3_status_handler.go` と `backend/cmd/server/main.go`、検証 CLI `backend/cmd/ecoflow-delta3-probe` が中心。
- API パス `/api/delta3/status` や `.env` の `ECOFLOW_DELTA3_*` は既存運用に使われているため、今回は互換維持する。

## 変更方針

### Package 名

- `backend/internal/ecoflowdelta3` を `backend/internal/ecoflowprivate` に rename する。
- Go package 名も `ecoflowprivate` に変更する。
- import 参照を `ecoflowprivate` に更新する。

### 機器 profile

新規に profile 層を package 内に追加する。

- `DeviceProfile`
  - `DeviceType string`
  - `Family string`
  - `MinACChargeW int`
  - `MaxACChargeW int`
  - `SupportedCommands map[string]bool` 相当
  - 将来拡張用に topic/codec 差異を収められる構造
- `ProfileForDeviceType(deviceType string) (DeviceProfile, bool)`
- 既存 `RangeForDeviceType` は互換 wrapper として残すか、内部で `ProfileForDeviceType` を使う。

対象 profile:

- `DELTA_3`
- `DELTA_3_PLUS`
- `DELTA_3_1500`
- `DELTA_3_MAX_PLUS`

### 共通 adapter と profile の分離

- `auth.go`, `mqtt.go`, `topics.go`, `client.go` は EcoFlow private MQTT 共通層として残す。
- `config.go`, `guard.go`, `codec.go` の device type 分岐は profile を参照する形へ寄せる。
- error message のうち「DELTA_3 private write」は、機器固有に見えすぎるため「EcoFlow private write」へ変更する。
- dry-run/probe の文言は CLI 名として `ecoflow-delta3-probe` を残してもよいが、内部説明は EcoFlow private に寄せる。

## Data / API contract

互換維持するもの:

- `/api/delta3/status`
- `/api/delta3/aux-plan`
- `/api/delta3/aux-commands`
- `Delta3StatusResponse`
- `Delta3StatusReader`
- `.env` の `ECOFLOW_DELTA3_*`
- DB テーブル `delta3_aux_control_command_logs`

将来 rename する候補だが今回は対象外:

- API パス名
- DTO 名
- DB テーブル名
- frontend state 名

## 安全境界

- 実機 write の条件は維持する。
  - `ENABLE_REAL_CONTROL=true`
  - `SIMULATION_MODE=false`
  - `MOCK_MODE=false`
  - `AUTO_CONTROL_ENABLED=true`
  - 実制御期限 / trial window が有効
  - `CONFIRM_ECOFLOW_WRITE` 一致
  - `ECOFLOW_DELTA3_READ_ENABLED=true`
  - `ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=true`
  - `ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=true`
  - `ECOFLOW_DELTA3_EXECUTE=true`
  - 機器マスターで対象機器が `enabled=true` / `controlEnabled=true`
  - 既存の最小コマンド間隔、重複抑止、未反映リザーブ抑止
- profile 化で対応機種が増えても、未定義機種は write 不可のままにする。
- 認証情報、SN、token は plan / test / commit message に含めない。
- 実機 write は追加実行しない。テストは mock transport / dry-run を使う。

## 実装手順

1. `backend/internal/ecoflowdelta3` を `backend/internal/ecoflowprivate` に rename。
2. package 宣言と import を更新。
3. `profile.go` を追加し、機器別 AC 充電範囲と family を定義。
4. `RangeForDeviceType` / `ValidateACChargePower` / `ValidateWriteGuards` を profile 経由に変更。
5. error message を EcoFlow private 汎用表現へ寄せる。
6. 既存 tests を package rename に追従し、profile の単体テストを追加。
7. `delta3_status_handler.go`, `server/main.go`, `ecoflow-delta3-probe` の import と型参照を更新。
8. Go test と frontend build で既存挙動を確認。

## レビュー観点

- 既存の read/status/write guard 挙動が変わっていないか。
- 未対応 device type が誤って write 可能になっていないか。
- `.env` や API パスの互換性を壊していないか。
- package rename による import 漏れがないか。
- 新しい profile が将来機種追加の入口として十分か。

## 検証コマンド

- `cd backend && rtk go test ./...`
- `cd frontend && rtk npm run build`
- `rtk git diff --check`
- `rtk codex review --uncommitted`

## ロールバック

- package rename と profile 追加だけなので、問題があれば `ecoflowprivate` package rename 差分を戻す。
- DB migration や実機設定変更は含めないため、運用データの rollback は不要。
