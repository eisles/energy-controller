# Phase 7 EcoFlow 診断情報ログ保存 実装計画

## 目的

DELTA Pro 3 の AC 出力が一時的に 0W または OFF になった場合に、後から原因を追跡できるように、EcoFlow Cloud の read-only quota から温度・警告・異常・保護・AC 出力状態に関連しそうな値を保存する。

今回の目的は「原因特定に使える観測データを残すこと」であり、温度上昇時の自動停止や AC 出力 ON/OFF 制御は実装しない。

## 非目的

- DELTA Pro 3 の AC 出力 ON/OFF write API は追加しない。
- 温度や警告に基づく自動停止制御は追加しない。
- EcoFlow write gate や実制御条件は変更しない。
- EcoFlow の raw quota 全量保存は行わない。秘密情報ではないが、不要なデータ肥大を避ける。

## 現状

- `power_logs` は Grid、Battery SOC、入力W、出力W、AC充電上限、判断理由を保存している。
- `current_status` は dashboard 用の現在値を保存している。
- EcoFlow quota から `powOutSumW` は保存しているため「出力が 0W になった」は追える。
- しかし温度、警告、保護、AC output enabled などの周辺情報は保存していないため、「なぜ 0W になったか」は追えない。

## 変更方針

1. `domain.BatteryStatus` / `domain.Status` / `domain.PowerLog` に `EcoFlowDiagnostics` を追加する。
2. EcoFlow Cloud quota 読み取り時に、キー名から診断系と判断できる値だけを抽出する。
   - `temp`
   - `alarm`
   - `warn`
   - `fault`
   - `error`
   - `protect`
   - `acout`
   - `ac_out`
   - `output`
3. 抽出結果を `current_status.ecoflow_diagnostics_json` と `power_logs.ecoflow_diagnostics_json` に JSON として保存する。
4. `/api/status` と `/api/logs` の JSON にも診断情報を含める。
5. 取得対象キーが存在しない場合は `nil` / `null` のままにし、既存 API 互換性を壊さない。

## 安全境界

- read-only quota の保存のみ行う。
- EcoFlow write command は追加しない。
- AC 出力 ON/OFF 操作は追加しない。
- 温度値や警告値が取得できても、この Phase では制御判断に使わない。
- 既存の実制御 gate、機器マスタ、優先順位制御には影響させない。

## 変更予定ファイル

- `backend/internal/domain/status.go`
  - `EcoFlowDiagnostics` を status / log に追加。
- `backend/internal/ecoflow/quota_adapter.go`
  - quota から診断系キーだけを抽出する helper を追加。
- `backend/internal/mock/status_provider.go`
  - `BatteryStatus` の診断情報を `Status` に受け渡す。
- `backend/internal/store/migrations.go`
  - `current_status` / `power_logs` に `ecoflow_diagnostics_json` を追加。
- `backend/internal/store/status_repository.go`
  - 現在値の保存・取得に診断JSONを追加。
- `backend/internal/store/log_repository.go`
  - ログ保存・一覧取得に診断JSONを追加。
- `backend/cmd/server/main.go`
  - `powerLogFromStatus` に診断情報を渡す。
- テスト
  - quota 抽出テスト。
  - repository 保存・取得テスト。

## 検証

- `cd backend && rtk go test ./...`
- `cd frontend && rtk npm run build`
- `rtk codex review --uncommitted`

## 運用メモ

- 取得できるキー名は EcoFlow 側のレスポンスに依存するため、初回は `/api/status.ecoflowDiagnostics` と `power_logs.ecoflow_diagnostics_json` を見て実データを確認する。
- AC 出力 0W が再発した場合、同時刻の `battery_output_w`、`ecoflow_diagnostics_json`、`surplus_control_command_logs` を突き合わせる。
