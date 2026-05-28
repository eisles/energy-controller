# Phase 7 DELTA 3 Plus 補助バッテリー検証・表示・制御計画

## 目的

DELTA Pro 3 の余剰追従だけでは吸収しきれない売電がある場合に、DELTA 3 Plus を補助バッテリーとして使えるかを段階的に検証する。最初の実装範囲は、DELTA 3 Plus private MQTT/protobuf 経路の残り候補を one-shot で安全に検証できるようにし、管理画面には read-only 状態を表示するところまでとする。

補助バッテリーの自動制御は、実機挙動とログを見ながら別フェーズで有効化する。今回の計画では、制御ロジックの入力・判定・安全境界を文書化し、実装時に write が既定で走らない構成を維持する。

## 安全境界

- 既定値では DELTA 3 Plus への write は実行しない。
- write は既存 DELTA_3 CLI と同じ gate を使い、`--execute`、`--allow-private-api-write`、`ENABLE_REAL_CONTROL=true`、`SIMULATION_MODE=false`、`MOCK_MODE=false`、`CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND` をすべて満たす時だけ許可する。
- `AUTO_CONTROL_ENABLED=true` 中の one-shot 検証は、既存どおり `ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=true` または `--allow-auto-control-overlap` を要求する。
- 検証 write は 1 回 1 コマンドだけに制限する。
- dashboard 追加は read-only API のみを使い、画面操作から DELTA 3 Plus write API を呼ばない。
- DELTA Pro 3 と DELTA 3 Plus の制御を同時に増やして競合させない。補助バッテリー自動制御は計画化に留める。

## 実装ステップ

### 1. 残り候補の one-shot 検証 CLI

`backend/cmd/ecoflow-delta3-probe` に次の dry-run / execute 候補を追加する。

| CLI | protobuf field | 目的 | 検証範囲 |
| --- | --- | --- | --- |
| `--min-discharge-soc <percent>` | `cms_min_dsg_soc` | 最低放電 SOC の設定可否を確認する | 範囲は 0-30% とし、範囲外は送信前に拒否 |
| `--max-charge-soc <percent>` | `cms_max_chg_soc` | 最大充電 SOC の設定可否を確認する | 範囲は 50-100% とし、範囲外は送信前に拒否 |
| `--energy-backup-enabled=true|false` + `--energy-backup-start-soc <percent>` | `cfg_energy_backup.energy_backup_en` / `energy_backup_start_soc` | backup reserve 機能の ON/OFF と開始 SOC の組み合わせを確認する | 開始 SOC は 5-100% を要求し、暗黙値で既存設定を壊さない |

検証手順は各候補で共通にする。

1. read-only probe で変更前状態を記録する。
2. dry-run で topic と payload hex を確認する。
3. 必要な write gate を満たした one-shot execute を 1 つだけ送る。
4. read-only probe で `maxChargeSoc`、`minDischargeSoc`、`backupReserveEnabled`、`backupReserveSoc` の readback を確認する。
5. 検証値が運用値と違う場合は、同じ gate で元に戻す。

### 2. DELTA 3 Plus read-only dashboard

管理画面に DELTA 3 Plus の状態カードを追加する。取得は backend の read-only endpoint 経由にして、frontend から private MQTT 認証情報を扱わない。

表示項目:

- SOC
- AC入力
- AC出力
- AC充電上限
- `gridBypassDisabled`
- `acOutputEnabled`

backend 方針:

- `GET /api/delta3/status` を追加する。
- `ECOFLOW_DELTA3_READ_ENABLED=false` を既定値にし、未設定・認証情報不足・取得失敗時も main status を壊さず `{ available: false, lastError: ... }` を返す。
- response には秘密情報や device SN を表示しない。必要なら device type と取得時刻だけを出す。
- read-only timeout は既存 private probe timeout を使い、dashboard refresh が control loop を詰まらせないようにする。

frontend 方針:

- `fetchDelta3Status()` と `Delta3Status` 型を追加する。
- 6 つの主要カードの直下、推移グラフより前に「DELTA 3 Plus」カードを置く。
- `available=false` の場合は read-only 未設定または取得失敗として Badge/Alert で表示し、画面全体のエラーにはしない。

### 3. 補助バッテリー制御計画

DELTA 3 Plus を補助バッテリーとして使う自動制御は、今回の実装では write しない計画表示・設計整理までに留める。後続実装では次の入力を使う。

入力:

- Nature Remo E の `exportW` / `importW`
- DELTA Pro 3 の `batterySoc`、`batteryInputW`、`batteryOutputW`、`acChargeLimitW`
- DELTA 3 Plus の SOC、AC入力、AC出力、AC充電上限、`gridBypassDisabled`、`acOutputEnabled`
- 手動充電通知の状態
- 将来の SwitchBot スマートプラグ状態

判定:

- DELTA Pro 3 が満充電に近い、AC充電上限に張り付いている、または余剰追従プランが追加吸収できない場合だけ DELTA 3 Plus を候補にする。
- DELTA 3 Plus の SOC が最大充電 SOC に近い場合は補助充電候補から外す。
- 売電が DELTA 3 Plus の最低 AC 充電量と安全マージンを上回る時だけ補助充電を検討する。
- 買電に転じた場合は補助充電を停止または抑制する。
- DELTA Pro 3 の制御と競合しないよう、最小コマンド間隔・ヒステリシス・連続判定回数を別に持つ。

出力:

- 初期実装は dashboard に「補助バッテリー計画」として read-only 推奨だけを表示する。
- 実 write を入れる場合は別計画で、SwitchBot ON/OFF または DELTA 3 Plus AC充電設定のどちらを使うかを明確に分ける。

## 完了条件

- `ecoflow-delta3-probe` が `cms_min_dsg_soc`、`cms_max_chg_soc`、`energy_backup_en` の dry-run payload を生成できる。
- 各 write 候補は allowlist と既存 gate を通らない限り execute できない。
- dashboard に DELTA 3 Plus read-only 状態が表示される。
- DELTA 3 Plus read-only 取得が失敗しても `/api/status` と既存 dashboard は動く。
- 補助バッテリー制御方針がこの文書に残り、今回の実装では自動 write を追加しない。
- `cd backend && go test ./...` が通る。
- `cd frontend && npm run build` が通る。

## 未実装 TODO

- DELTA 3 Plus 補助充電の自動 write。
- SwitchBot スマートプラグ連携。
- DELTA Pro 3 と DELTA 3 Plus の同時制御に対する統合スケジューラ。
- 補助バッテリー制御ログの永続化。
