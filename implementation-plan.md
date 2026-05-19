# Energy Controller 実装計画

## 目的

Nature Remo E からスマートメーターの瞬時電力を取得し、売電しているタイミングで EcoFlow DELTA Pro 3 へ充電する。

最初から実制御はしない。まずは以下を優先する。

1. ログ取得
2. シミュレーション
3. 推奨充電Wの計算
4. Nature Remo E 実API連携
5. EcoFlow 読み取り専用連携
6. 管理画面
7. 最後に EcoFlow 実制御

---

## 重要な前提

Nature Remo E のスマートメーター瞬時電力は以下の考え方で扱う。

```text
gridW > 0  = 買電中
gridW < 0  = 売電中

exportW = max(0, -gridW)
importW = max(0, gridW)
```

EcoFlow への書き込み制御は、必ず以下を満たす場合のみ実行する。

```text
ENABLE_REAL_CONTROL=true
SIMULATION_MODE=false
MOCK_MODE=false
```

デフォルトは必ず以下。

```env
MOCK_MODE=true
SIMULATION_MODE=true
ENABLE_REAL_CONTROL=false
```

---

## 全体アーキテクチャ

```text
Nature Remo E
   ↓
Go backend / control loop
   ↓
EcoFlow DELTA Pro 3

Next.js Static Export 管理画面
   ↓
Go REST API

SQLite
   ↓
settings / current_status / power_logs
```

---

## プロジェクト構造

```text
energy-controller/
├─ AGENTS.md
├─ README.md
├─ implementation-plan.md
├─ .env.example
├─ .gitignore
├─ docker-compose.yml
├─ Dockerfile
├─ Makefile
│
├─ backend/
│  ├─ go.mod
│  ├─ go.sum
│  ├─ cmd/
│  │  └─ server/
│  │     └─ main.go
│  │
│  ├─ internal/
│  │  ├─ app/
│  │  │  ├─ app.go
│  │  │  └─ wire.go
│  │  │
│  │  ├─ config/
│  │  │  └─ config.go
│  │  │
│  │  ├─ domain/
│  │  │  ├─ power.go
│  │  │  ├─ battery.go
│  │  │  ├─ settings.go
│  │  │  └─ decision.go
│  │  │
│  │  ├─ control/
│  │  │  ├─ controller.go
│  │  │  ├─ decision_engine.go
│  │  │  ├─ hysteresis.go
│  │  │  └─ decision_engine_test.go
│  │  │
│  │  ├─ nature/
│  │  │  ├─ client.go
│  │  │  ├─ cloud_client.go
│  │  │  ├─ local_client.go
│  │  │  ├─ mock_client.go
│  │  │  ├─ parser.go
│  │  │  └─ parser_test.go
│  │  │
│  │  ├─ ecoflow/
│  │  │  ├─ client.go
│  │  │  ├─ signed_client.go
│  │  │  ├─ mock_client.go
│  │  │  ├─ quota_adapter.go
│  │  │  └─ quota_adapter_test.go
│  │  │
│  │  ├─ store/
│  │  │  ├─ db.go
│  │  │  ├─ migrations.go
│  │  │  ├─ log_repository.go
│  │  │  ├─ settings_repository.go
│  │  │  └─ status_repository.go
│  │  │
│  │  ├─ api/
│  │  │  ├─ router.go
│  │  │  ├─ status_handler.go
│  │  │  ├─ settings_handler.go
│  │  │  ├─ logs_handler.go
│  │  │  ├─ control_handler.go
│  │  │  └─ middleware.go
│  │  │
│  │  ├─ static/
│  │  │  └─ embed.go
│  │  │
│  │  └─ clock/
│  │     └─ clock.go
│  │
│  └─ data/
│     └─ .gitkeep
│
└─ frontend/
   ├─ package.json
   ├─ next.config.ts
   ├─ tsconfig.json
   ├─ app/
   │  ├─ layout.tsx
   │  ├─ page.tsx
   │  └─ globals.css
   │
   ├─ components/
   │  ├─ StatusCards.tsx
   │  ├─ ControlPanel.tsx
   │  ├─ SettingsForm.tsx
   │  ├─ LogTable.tsx
   │  └─ Header.tsx
   │
   └─ lib/
      ├─ api.ts
      └─ types.ts
```

---

## 各パッケージの責務

### `backend/internal/domain`

外部APIに依存しない型を置く。

```go
type GridPower struct {
    GridW   int
    ImportW int
    ExportW int
}

type BatteryStatus struct {
    Soc      int
    InputW   int
    OutputW  int
    IsOnline bool
}

type ControlDecision struct {
    ShouldCharge  bool
    TargetChargeW int
    Reason        string
}
```

### `backend/internal/nature`

Nature Remo E APIとの接続を担当する。

```go
type Client interface {
    GetGridPower(ctx context.Context) (domain.GridPower, error)
}
```

実装候補。

```text
cloud_client.go  Cloud API用
local_client.go  Local API用、後回し可
mock_client.go   テスト・開発用
parser.go        EPC/hex変換など
```

### `backend/internal/ecoflow`

EcoFlow APIとの接続を担当する。

```go
type Client interface {
    GetBatteryStatus(ctx context.Context) (domain.BatteryStatus, error)
    SetACChargePower(ctx context.Context, watts int) error
    StopOrMinimizeCharging(ctx context.Context) error
}
```

EcoFlow固有のquota名、署名処理、APIレスポンス差分は `quota_adapter.go` に閉じ込める。

### `backend/internal/control`

制御判断の中核。

```text
Natureから瞬時電力取得
↓
売電量 exportW を計算
↓
EcoFlow SOCを確認
↓
設定値と比較
↓
推奨充電Wを決定
↓
simulationならログだけ
↓
real controlならEcoFlowへ送信
```

---

## `.env.example`

```env
APP_ENV=local
HTTP_PORT=8080
DB_PATH=/app/data/energy.db

# mode
MOCK_MODE=true
SIMULATION_MODE=true
ENABLE_REAL_CONTROL=false

# Nature Remo
NATURE_MODE=cloud
NATURE_ACCESS_TOKEN=
NATURE_APPLIANCE_ID=
NATURE_LOCAL_BASE_URL=http://remo-e.local

# EcoFlow
ECOFLOW_ACCESS_KEY=
ECOFLOW_SECRET_KEY=
ECOFLOW_DEVICE_SN=
ECOFLOW_BASE_URL=https://api-a.ecoflow.com

# control settings
POLL_INTERVAL_SEC=30
START_EXPORT_THRESHOLD_W=700
STOP_EXPORT_THRESHOLD_W=300
SAFETY_MARGIN_W=150
MIN_CHARGE_W=400
MAX_CHARGE_W=1500
TARGET_SOC=90
MIN_COMMAND_INTERVAL_SEC=60
MIN_COMMAND_DIFF_W=100
REQUIRE_CONSECUTIVE_EXPORT_COUNT=2
REQUIRE_CONSECUTIVE_IMPORT_COUNT=2
```

---

## SQLite設計

### `settings`

```sql
CREATE TABLE settings (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  auto_control_enabled INTEGER NOT NULL DEFAULT 0,
  simulation_mode INTEGER NOT NULL DEFAULT 1,
  start_export_threshold_w INTEGER NOT NULL DEFAULT 700,
  stop_export_threshold_w INTEGER NOT NULL DEFAULT 300,
  safety_margin_w INTEGER NOT NULL DEFAULT 150,
  min_charge_w INTEGER NOT NULL DEFAULT 400,
  max_charge_w INTEGER NOT NULL DEFAULT 1500,
  target_soc INTEGER NOT NULL DEFAULT 90,
  min_command_interval_sec INTEGER NOT NULL DEFAULT 60,
  min_command_diff_w INTEGER NOT NULL DEFAULT 100,
  require_consecutive_export_count INTEGER NOT NULL DEFAULT 2,
  require_consecutive_import_count INTEGER NOT NULL DEFAULT 2,
  updated_at TEXT NOT NULL
);
```

### `power_logs`

```sql
CREATE TABLE power_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  measured_at TEXT NOT NULL,

  grid_w INTEGER NOT NULL,
  import_w INTEGER NOT NULL,
  export_w INTEGER NOT NULL,

  battery_soc INTEGER,
  battery_input_w INTEGER,
  battery_output_w INTEGER,

  target_charge_w INTEGER NOT NULL DEFAULT 0,
  actual_command_w INTEGER,
  decision_reason TEXT NOT NULL,

  mode TEXT NOT NULL,
  command_sent INTEGER NOT NULL DEFAULT 0,
  error_message TEXT,

  created_at TEXT NOT NULL
);
```

### `current_status`

```sql
CREATE TABLE current_status (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  grid_w INTEGER NOT NULL DEFAULT 0,
  import_w INTEGER NOT NULL DEFAULT 0,
  export_w INTEGER NOT NULL DEFAULT 0,

  battery_soc INTEGER,
  battery_input_w INTEGER,
  battery_output_w INTEGER,

  target_charge_w INTEGER NOT NULL DEFAULT 0,
  state TEXT NOT NULL,
  mode TEXT NOT NULL,
  last_decision_reason TEXT NOT NULL,
  last_error TEXT,

  updated_at TEXT NOT NULL
);
```

---

## API設計

### `GET /api/status`

現在状態を返す。

```json
{
  "gridW": -850,
  "importW": 0,
  "exportW": 850,
  "batterySoc": 62,
  "batteryInputW": 500,
  "batteryOutputW": 0,
  "targetChargeW": 700,
  "state": "simulation",
  "mode": "auto",
  "lastDecisionReason": "export power is enough, simulation only",
  "lastError": null,
  "updatedAt": "2026-05-18T07:30:00+09:00"
}
```

### `GET /api/settings`

設定取得。

### `PUT /api/settings`

設定更新。

### `GET /api/logs?limit=100`

ログ取得。

### `POST /api/control/start`

自動制御開始。

### `POST /api/control/stop`

自動制御停止。

### `POST /api/control/simulate`

手動シミュレーション。

```json
{
  "gridW": -1200,
  "batterySoc": 50
}
```

### `POST /api/control/manual-charge`

手動充電。

実行条件。

```text
ENABLE_REAL_CONTROL=true
SIMULATION_MODE=false
auto_control_enabled=false
watts <= MAX_CHARGE_W
```

---

## 制御ロジック

### 基本判断

```go
func Decide(gridW int, battery domain.BatteryStatus, settings domain.Settings, now time.Time) domain.ControlDecision {
    exportW := max(0, -gridW)
    importW := max(0, gridW)

    if battery.Soc >= settings.TargetSoc {
        return Stop("battery soc reached target")
    }

    if importW > 0 {
        return Stop("grid import detected")
    }

    if exportW < settings.StartExportThresholdW {
        return Stop("export power is below start threshold")
    }

    target := exportW - settings.SafetyMarginW
    target = floorTo100W(target)

    if target < settings.MinChargeW {
        return Stop("target charge power is below minimum charge power")
    }

    if target > settings.MaxChargeW {
        target = settings.MaxChargeW
    }

    return Charge(target, "export power is enough")
}
```

### ヒステリシス

```text
売電開始条件:
  exportW >= startExportThresholdW が N回連続

停止条件:
  importW > 0 または exportW < stopExportThresholdW が N回連続

コマンド送信条件:
  前回送信から minCommandIntervalSec 以上
  かつ 前回設定Wとの差が minCommandDiffW 以上
```

---

## フロント画面

### 表示

```text
現在の買電 / 売電
exportW
importW
DELTA Pro 3 SOC
DELTA Pro 3 入力W
DELTA Pro 3 出力W
推奨充電W
現在モード mock / simulation / real
自動制御 ON / OFF
最終判断理由
最終エラー
```

### 操作

```text
自動制御ON/OFF
設定値更新
手動シミュレーション実行
ログ一覧確認
```

---

## Docker構成

### `docker-compose.yml`

```yaml
services:
  app:
    build: .
    container_name: energy-controller
    restart: unless-stopped
    env_file:
      - .env
    ports:
      - "8080:8080"
    volumes:
      - ./data:/app/data
```

### `Dockerfile`

```dockerfile
# frontend build
FROM node:22-alpine AS frontend-builder
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# backend build
FROM golang:1.23-alpine AS backend-builder
WORKDIR /src/backend
RUN apk add --no-cache gcc musl-dev sqlite-dev
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend-builder /src/frontend/out ./internal/static/out
RUN go build -o /out/server ./cmd/server

# runtime
FROM alpine:3.20
WORKDIR /app
RUN apk add --no-cache ca-certificates tzdata sqlite-libs
COPY --from=backend-builder /out/server /app/server
ENV TZ=Asia/Tokyo
EXPOSE 8080
CMD ["/app/server"]
```

---

## Makefile

```makefile
.PHONY: dev test build docker-up docker-down frontend-build

dev:
	cd backend && go run ./cmd/server

test:
	cd backend && go test ./...

frontend-build:
	cd frontend && npm ci && npm run build

build: frontend-build
	cd backend && go build -o ../bin/server ./cmd/server

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down
```

---

## 実装フェーズ

### Phase 1: 雛形作成

目的。

```text
Go API + Next.js Static Export + SQLite + Docker Composeの土台を作る
```

完了条件。

```text
docker compose up -d --build
http://localhost:8080 で管理画面表示
http://localhost:8080/api/status がJSONを返す
cd backend && go test ./... が通る
```

Codexへの指示。

```text
このリポジトリに、Go backend + Next.js Static Export frontend + SQLite + Docker Compose の雛形を作ってください。

要件:
- backend は Go
- frontend は Next.js static export
- Go backend が frontend/out の静的ファイルを配信する
- DB は SQLite
- mock mode をデフォルトにする
- .env.example を作る
- README に起動手順を書く
- GET /api/status を実装する
- go test ./... が通るようにする
- docker compose up -d --build で起動できるようにする
```

### Phase 2: 制御ロジックのみ実装

```text
制御ロジックを実装してください。

要件:
- gridW > 0 は買電
- gridW < 0 は売電
- exportW = max(0, -gridW)
- importW = max(0, gridW)
- SOCがtargetSoc以上なら充電しない
- exportWがstartExportThresholdW未満なら充電しない
- targetChargeW = exportW - safetyMarginW
- minChargeW / maxChargeW で丸める
- 100W単位に丸める
- ヒステリシスを入れる
- minCommandIntervalSecを考慮する
- ユニットテストを厚めに書く
- まだ実機APIは呼ばない
```

### Phase 3: SQLiteログ保存

```text
SQLiteのマイグレーションとログ保存を実装してください。

要件:
- settings
- power_logs
- current_status
- 起動時に自動マイグレーションする
- 制御ループごとにpower_logsへ保存する
- /api/logs?limit=100 を実装する
- /api/status は current_status を返す
```

### Phase 4: Nature Remo E Cloud API連携

```text
Nature Remo E Cloud API client を実装してください。

要件:
- NATURE_ACCESS_TOKEN を .env から読む
- /1/echonetlite/appliances からスマートメーターを探す
- EPC 0xE7 measured_instantaneous を取得する
- 正値は買電、負値は売電として GridPower に変換する
- updated_at が古い場合は warning として status に出す
- APIエラー時はアプリを落とさず lastError に保存する
- mock client は残す
- parser のユニットテストを書く
```

### Phase 5: EcoFlow読み取り専用連携

```text
EcoFlow DELTA Pro 3 の読み取り専用 client を実装してください。

要件:
- ECOFLOW_ACCESS_KEY
- ECOFLOW_SECRET_KEY
- ECOFLOW_DEVICE_SN
- ECOFLOW_BASE_URL
を .env から読む

- まずは読み取り専用
- SOC、入力W、出力W、現在のAC充電設定を取得する設計にする
- API固有のquota名は quota_adapter.go に閉じ込める
- API項目名が不明なものは TODO とログで明示する
- mock client は残す
- 実APIが失敗してもアプリを落とさない
```

### Phase 6: 管理画面

```text
Next.jsの管理画面を実装してください。

表示:
- 現在の買電/売電W
- exportW
- importW
- DELTA Pro 3 SOC
- 入力W
- 出力W
- 推奨充電W
- mode
- simulation / real control 状態
- 最終判断理由
- 最終エラー
- 直近ログ

操作:
- 自動制御ON/OFF
- 設定値更新
- 手動シミュレーション実行

注意:
- Next.jsはStatic Export
- APIはGo backendを呼ぶ
- UIキットは shadcn/ui を使う
- shadcn/ui の Card / Button / Table / Badge / Alert / Form 系コンポーネントをベースにする
- 書き込み API が未実装の操作は disabled または read-only 表示に留める
```

### Phase 7: EcoFlow実制御

```text
EcoFlowへの実制御を追加してください。

重要:
- ENABLE_REAL_CONTROL=true かつ SIMULATION_MODE=false のときだけ実行
- デフォルトでは絶対に実行しない
- mock modeでは実行しない
- 自動制御OFFのときは実行しない
- 前回設定との差が MIN_COMMAND_DIFF_W 未満なら送信しない
- MIN_COMMAND_INTERVAL_SEC 未満なら送信しない
- 送信したコマンドは power_logs に保存する
- API失敗時は連続リトライしない
- 買電になったら StopOrMinimizeCharging を呼ぶ
- 実制御部分のユニットテストを追加する
```

#### Phase 7 詳細計画

Phase 7 は EcoFlow への書き込み制御を初めて追加する段階なので、実装は以下の順で進める。実機 write path は最後まで既定 OFF とし、read-only / simulation の既存挙動を壊さない。

##### 7-1. 実制御ガードを先に固定する

EcoFlow への write command は、必ず以下をすべて満たす場合だけ送信する。

```text
ENABLE_REAL_CONTROL=true
SIMULATION_MODE=false
MOCK_MODE=false
auto_control_enabled=true
```

上記のいずれかが false の場合は、実 API へ書き込まず、判断結果と送信しなかった理由を `power_logs` と current status に残す。

実装条件:

- `controller` / write adapter 境界の手前で guard する
- adapter 側にも二重 guard を置き、呼び出しミスでも送信できないようにする
- unit test で `ENABLE_REAL_CONTROL=false`、`SIMULATION_MODE=true`、`MOCK_MODE=true`、`auto_control_enabled=false` の各ケースを確認する
- default `.env.example` は `MOCK_MODE=true` / `SIMULATION_MODE=true` / `ENABLE_REAL_CONTROL=false` のまま変更しない

##### 7-2. EcoFlow write adapter を read model から分離する

EcoFlow API の command / quota name / params は不確実性が高いため、domain や controller に直接漏らさない。

実装条件:

- `backend/internal/ecoflow` に write 専用 adapter を置く
- domain / control 層は `SetACChargePower(ctx, watts)` と `StopOrMinimizeCharging(ctx)` の抽象だけを見る
- EcoFlow 固有の endpoint、署名、payload、quota name は adapter 内に閉じる
- 不明な API 値や推定値は TODO comment と structured log に残す
- 実 API 失敗時はアプリを落とさず、`lastError` と `power_logs.error_message` に残す

##### 7-3. command 抑制を write path で必ず適用する

実制御では頻繁なコマンド送信を避ける。判断エンジンが target を出しても、write path は以下を満たすときだけ送信する。

```text
abs(targetW - lastCommandW) >= MIN_COMMAND_DIFF_W
now - lastCommandAt >= MIN_COMMAND_INTERVAL_SEC
```

実装条件:

- 最後に送った command W / timestamp を保存する
- 抑制された場合も `command_sent=false` と理由を log に残す
- command 抑制は simulation mode でも同じ判断ログを出せるようにする
- unit test で差分不足、interval 不足、両方満たすケースを確認する

##### 7-4. StopOrMinimizeCharging の安全動作を先に実装する

買電中、売電不足、SOC 上限到達、API 不確実の場合は、充電強化ではなく停止または最小化を優先する。

実装条件:

- `gridW > 0` の買電状態では charging command を送らない
- `exportW < STOP_EXPORT_THRESHOLD_W` では充電を弱める
- EcoFlow API の stop 相当が不確実な場合は、AC charge W を最小値へ下げる実装に留める
- stop / minimize の実 API payload が確定するまで、adapter 内に TODO を残す
- unit test で買電、売電不足、SOC 上限到達時に charge-up command が送られないことを確認する

##### 7-5. 監査ログと UI 表示を先に使える状態にする

実コマンドを送る前に、何を送る予定だったか、なぜ送ったか、またはなぜ送らなかったかを追跡できるようにする。

実装条件:

- `power_logs.actual_command_w` に送信した W を保存する
- `power_logs.command_sent` を正しく保存する
- `decision_reason` に guard / hysteresis / interval / API error の理由を含める
- `decision_reason` と `surplusPlan.actionSummary` に dry-run の予定アクションを残し、実送信前に買電時の戻し/売電時の充電開始条件を確認できるようにする
- current status の `lastDecisionReason` / `lastError` に UI で判断できる情報を残す
- Phase 6 dashboard には書き込み操作を追加しない。必要なら read-only 表示だけを拡張する

##### 7-6. 実機確認は短時間・手動で開始する

最初の実機確認では、自動連続制御をいきなり有効化しない。

手順:

1. `.env` を実機用に設定する。ただし `ENABLE_REAL_CONTROL=false` のまま起動する
2. read-only で `batterySoc` / `batteryInputW` / `batteryOutputW` / `acChargeLimitW` が取れることを確認する
3. `SIMULATION_MODE=false` にしても `ENABLE_REAL_CONTROL=false` の間は write されないことを log で確認する
4. 短時間だけ `ENABLE_REAL_CONTROL=true` にして、1 command のみ送信される条件を作る
5. `power_logs.command_sent=true`、`actual_command_w`、EcoFlow 側の状態変化を確認する
6. 確認後は `ENABLE_REAL_CONTROL=false` に戻す

ロールバック:

- `.env` の `ENABLE_REAL_CONTROL=false` に戻す
- 必要なら `SIMULATION_MODE=true` / `MOCK_MODE=true` に戻す
- Docker Compose の場合は `docker compose down` 後に `.env` を確認して再起動する

##### Phase 7 完了条件

- default 起動では実コマンドが送信されない
- guard 条件を満たさない全ケースの unit test がある
- write adapter の実 API 呼び出しは adapter package 内に閉じている
- command interval / diff 抑制の unit test がある
- 実コマンド送信時は `power_logs` に `command_sent=true` と `actual_command_w` が残る
- API 失敗時にアプリが落ちず、連続リトライしない
- README に実制御の危険性、設定条件、ロールバック手順を追記する
- `cd backend && go test ./...` が通る
- `docker compose up -d --build` で default mock/simulation 起動できる

#### EcoFlow DELTA Pro 3 制御方針メモ

Phase 5 の read-only API 確認では、DELTA Pro 3 の充電挙動に関係しそうな quota として以下を確認した。

```text
cmsBattSoc
bmsBattSoc
powInSumW
powOutSumW
plugInInfoAcInChgPowMax
energyBackupStartSoc
backupReverseSoc
energyBackupEn
energyStrategyOperateMode.operateTouModeOpen
energyStrategyOperateMode.operateSelfPoweredOpen
energyStrategyOperateMode.operateScheduledOpen
energyStrategyOperateMode.operateIntelligentScheduleModeOpen
```

観測結果:

- TOU mode のときは `energyStrategyOperateMode.operateTouModeOpen=true`
- self-powered mode のときは `energyStrategyOperateMode.operateSelfPoweredOpen=true`
- TOU / self-powered を OFF にすると、上記 mode flag はすべて `false`
- 2026-05-19 の one-shot 実機確認で、AC 充電上限 1500W / backup reserve 85% だけでは `batteryInputW=0` のままだった
- 同日、`cfgEnergyStrategyOperateMode` の nested payload で全 energy strategy mode flag を false にすると `touModeEnabled=false` になり、`batteryInputW` が約 1.4kW へ増えて実充電が始まることを確認した
- dotted key 形式 `cfgEnergyStrategyOperateMode.operateTouModeOpen=false` は EcoFlow API で `code=1008` となり拒否された
- backup reserve を 60% に設定すると `energyBackupStartSoc=60` / `backupReverseSoc=60` に反映される
- self-powered mode では `energyBackupEn=true`、TOU / self-powered OFF では `energyBackupEn=false` になることを確認した
- AC 充電上限 W は `plugInInfoAcInChgPowMax` に見える
- 実入力 W は `powInSumW`、実出力 W は `powOutSumW` に見える

現時点の仮説:

```text
充電したい:
  - energy strategy mode が ON の場合は、nested `cfgEnergyStrategyOperateMode` で TOU/self-powered/scheduled/intelligent flags を false にする
  - backup reserve % を現在 SOC より高め、または target SOC 付近へ上げる
  - AC charge power W を売電余剰に合わせて設定する

充電を弱めたい:
  - AC charge power W を下げる

充電しない / 放電寄りにしたい:
  - backup reserve % を下げる
  - 必要なら AC charge power W を最小化する
```

ただし、上記は read-only quota とアプリ操作後の観測に基づく仮説であり、書き込み API の command / quota name / params は Phase 7 で別途確定する。

Phase 7 で実制御を入れる場合の優先制御軸:

1. `cfgEnergyStrategyOperateMode` 相当の energy strategy mode flags 全OFF
2. `plugInInfoAcInChgPowMax` 相当の AC 充電 W
3. `energyBackupStartSoc` / `backupReverseSoc` 相当の backup reserve %

安全境界:

- Phase 5 / Phase 6 では EcoFlow 書き込み制御を実装しない
- Phase 7 でも `ENABLE_REAL_CONTROL=true` かつ `SIMULATION_MODE=false` かつ `MOCK_MODE=false` のときだけ送信する
- backup reserve % と AC charge power W は最後に送った値を保存し、差分が小さいときは送らない
- energy strategy mode flags 全OFF は実充電開始に必要だが、当面は one-shot CLI に限定し、自動連続制御へは未接続のままにする
- 買電状態、売電不足、API 不確実、quota 不確実のときは送信せず `lastError` / log に残す
- 最初の実制御は手動実行または短時間限定で確認し、自動連続制御は最後に有効化する

---

## 最初にCodexへ投げるプロンプト

```text
このリポジトリで Energy Controller を作ります。

AGENTS.md と implementation-plan.md の方針に従って、
まず Phase 1 を実装してください。

構成:
- backend: Go
- frontend: Next.js Static Export
- DB: SQLite
- deploy: Docker Compose

今回やること:
1. Go APIサーバーの雛形
2. Next.js Static Export の雛形
3. Goサーバーからfrontend/outを配信
4. SQLite初期化
5. mock statusを返す GET /api/status
6. .env.example
7. Dockerfile
8. docker-compose.yml
9. README
10. go test ./... が通る状態

重要:
- 実機API連携はまだ実装しない
- EcoFlowへの書き込み制御は絶対に入れない
- mock + simulation をデフォルトにする
```

---

## 推奨作業順

```text
1. GitHubに空リポジトリを作る
2. ローカルPCにclone
3. AGENTS.mdを配置
4. implementation-plan.mdを配置
5. Codex CLIを起動
6. 最初のプロンプトでPhase 1を実装
7. docker compose up -d --build を確認
8. Phase 2以降を順番に依頼
9. Nature Remo Eだけ実API接続
10. EcoFlowは読み取り専用接続
11. 数日〜1週間ログ確認
12. 最後にEcoFlow実制御を有効化
```

---

## 注意事項

- いきなり EcoFlow 実制御は入れない。
- `ENABLE_REAL_CONTROL=true` は最後まで設定しない。
- Nature Remo の値が古い場合は制御判断に使わない。
- APIエラー時は充電開始ではなく安全側に倒す。
- 充電電力は売電量ぴったりではなく、必ず `SAFETY_MARGIN_W` を差し引く。
- 家庭配線や分電盤に関わる改造は電気工事士に依頼する。
