# Energy Controller

Nature Remo E と EcoFlow DELTA Pro 3 を使った家庭用エネルギー制御アプリです。

Phase 1 では実機 API 連携と EcoFlow への書き込み制御は実装していません。デフォルトは必ず mock + simulation です。

```env
MOCK_MODE=true
SIMULATION_MODE=true
ENABLE_REAL_CONTROL=false
```

EcoFlow の実制御は最終フェーズまで有効化しないでください。家庭配線や分電盤に関わる改造は電気工事士に依頼してください。

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

## Phase 1 の実装範囲

- `GET /api/status`
- SQLite 初期化
- `settings` / `current_status` / `power_logs` table
- mock status provider
- Next.js Static Export の最小管理画面
- Docker Compose 起動

実機連携、制御ループ、ログ保存 API、設定更新 API、EcoFlow 書き込み制御は未実装です。
