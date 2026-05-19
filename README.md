# Energy Controller

Nature Remo E と EcoFlow DELTA Pro 3 を使った家庭用エネルギー制御アプリです。

Phase 1 では実機 API 連携と EcoFlow への書き込み制御は実装していません。デフォルトは必ず mock + simulation です。

```env
MOCK_MODE=true
SIMULATION_MODE=true
ENABLE_REAL_CONTROL=false
AUTO_CONTROL_ENABLED=false
```

EcoFlow の実制御は最終フェーズまで有効化しないでください。家庭配線や分電盤に関わる改造は電気工事士に依頼してください。

## 環境設定

`.env.example` には Phase 2 以降で使う制御設定も先に列挙しています。Phase 1 では見本値として保持し、実際の制御判断への反映は後続フェーズで実装します。

主な制御設定:

- `START_EXPORT_THRESHOLD_W`: 充電開始を検討する売電しきい値
- `STOP_EXPORT_THRESHOLD_W`: 充電停止を検討する売電しきい値
- `SAFETY_MARGIN_W`: 売電量から差し引く安全余白
- `MIN_CHARGE_W` / `MAX_CHARGE_W`: 推奨充電 W の下限/上限
- `TARGET_SOC`: 充電対象 SOC 上限
- `MIN_COMMAND_INTERVAL_SEC`: コマンド送信の最小間隔
- `MIN_COMMAND_DIFF_W`: コマンド差分の最小幅

## 構成

- `backend`: Go API server
- `frontend`: Next.js Static Export
- `SQLite`: `/app/data/energy.db`
- `Docker Compose`: local server deployment

Go backend が `frontend/out` の静的ファイルを配信します。

## 起動

```bash
cp .env.example .env
docker compose up -d --build
```

確認先:

- 管理画面: http://localhost:8080/
- status API: http://localhost:8080/api/status

停止:

```bash
docker compose down
```

## ローカル開発

backend test:

```bash
cd backend
go test ./...
```

frontend static export:

```bash
cd frontend
npm install
npm run build
```

backend をローカル実行する場合:

```bash
cd backend
FRONTEND_DIR=../frontend/out DB_PATH=./data/energy.db go run ./cmd/server
```

## Phase 7 dry-run 検証

通常の server / 管理画面経由では、EcoFlow への実 API 書き込みは行いません。Phase 7 の現時点では、server は write 条件が揃った場合でも mock write adapter が would-send として記録するだけです。

dry-run で確認する場合:

```bash
cd backend
HTTP_PORT=18081 \
FRONTEND_DIR=../frontend/out \
DB_PATH=./data/energy-dry-run.db \
MOCK_MODE=false \
SIMULATION_MODE=false \
ENABLE_REAL_CONTROL=true \
AUTO_CONTROL_ENABLED=true \
NATURE_MODE=local \
POLL_INTERVAL_SEC=5 \
go run ./cmd/server
```

確認ポイント:

- `/api/logs?limit=10` の `decisionReason` に `would-send` が残る
- dry-run のため `commandSent` は `false`
- dry-run のため `actualCommandW` は `null`
- interval / diff 抑制時は `decisionReason` に `command suppressed` が残る

## Phase 7 one-shot 実機検証 CLI

EcoFlow への実 API 書き込みは、管理画面や server API ではなく、手動 CLI で 1 command だけ実行します。通常は `--execute` を付けず dry-run してください。

dry-run:

```bash
cd backend
go run ./cmd/ecoflow-write-test --watts 1000 --expected-current-limit 1500
```

バックアップリザーブ%も同時に検証する場合:

```bash
cd backend
go run ./cmd/ecoflow-write-test \
  --watts 1000 \
  --expected-current-limit 1500 \
  --reserve-soc 90 \
  --expected-current-reserve 88
```

実行時は read-only API で現在の AC 充電上限が `--expected-current-limit` と一致し、`--reserve-soc` を指定した場合は現在のバックアップリザーブが `--expected-current-reserve` と一致することを確認してから、1 回だけ設定を送ります。余剰を実際に充電へ回すには、AC充電上限Wだけでなく、バックアップリザーブ%を現在SOCより上へ引き上げる必要がある可能性があります。以下の環境変数が全て揃わない場合は送信しません。

```bash
cd backend
MOCK_MODE=false \
SIMULATION_MODE=false \
ENABLE_REAL_CONTROL=true \
AUTO_CONTROL_ENABLED=false \
CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND \
ECOFLOW_ACCESS_KEY=... \
ECOFLOW_SECRET_KEY=... \
ECOFLOW_DEVICE_SN=... \
ECOFLOW_BASE_URL=https://api-e.ecoflow.com \
go run ./cmd/ecoflow-write-test --execute --watts 1000 --expected-current-limit 1500
```

バックアップリザーブ%も同時に送る場合:

```bash
cd backend
MOCK_MODE=false \
SIMULATION_MODE=false \
ENABLE_REAL_CONTROL=true \
AUTO_CONTROL_ENABLED=false \
CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND \
ECOFLOW_ACCESS_KEY=... \
ECOFLOW_SECRET_KEY=... \
ECOFLOW_DEVICE_SN=... \
ECOFLOW_BASE_URL=https://api-e.ecoflow.com \
go run ./cmd/ecoflow-write-test \
  --execute \
  --watts 1000 \
  --expected-current-limit 1500 \
  --reserve-soc 90 \
  --expected-current-reserve 88
```

Energy strategy modes を全て OFF にする one-shot 検証:

```bash
cd backend
MOCK_MODE=false \
SIMULATION_MODE=false \
ENABLE_REAL_CONTROL=true \
AUTO_CONTROL_ENABLED=false \
CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND \
ECOFLOW_ACCESS_KEY=... \
ECOFLOW_SECRET_KEY=... \
ECOFLOW_DEVICE_SN=... \
ECOFLOW_BASE_URL=https://api-e.ecoflow.com \
go run ./cmd/ecoflow-write-test \
  --execute \
  --disable-energy-modes \
  --expected-tou-mode=true \
  --expected-self-powered-mode=false \
  --expected-scheduled-mode=false \
  --expected-intelligent-schedule-mode=false
```

2026-05-19 の実機確認では、AC充電上限とバックアップリザーブだけでは充電が始まらず、energy strategy modes 全OFF 後に `batteryInputW` が約 1.4kW へ増えました。この操作は時間帯制御への影響が大きいため、当面は one-shot CLI に限定し、自動連続制御や UI/API からは実行しません。

実行後は EcoFlow app または read-only status で反映を確認し、`ENABLE_REAL_CONTROL` と `CONFIRM_ECOFLOW_WRITE` は継続運用用に残さないでください。自動連続制御、UI からの書き込み、server API からの書き込みはまだ有効化していません。

## Phase 1 の実装範囲

- `GET /api/status`
- SQLite 初期化
- `settings` / `current_status` / `power_logs` table
- mock status provider
- Next.js Static Export の最小管理画面
- Docker Compose 起動

自動制御ループ、設定更新 API、server / 管理画面からの EcoFlow 書き込み制御は未実装です。EcoFlow への実 API 書き込みは Phase 7 の one-shot CLI に限定しています。
