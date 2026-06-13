# Energy Controller

Nature Remo E と EcoFlow DELTA Pro 3 を使い、家庭の買電/売電状況、EcoFlow の SOC/入出力、翌日の PV 発電予測から充電方針を決めるローカル運用向けアプリです。

管理画面は Next.js Static Export を Go backend が配信します。backend は SQLite に制御判断、電力量、夜間充電計画、余剰追従コマンド履歴を保存します。

## 安全境界

`.env.example` と Docker Compose の既定値は、必ず mock + simulation + write disabled です。

```env
MOCK_MODE=true
SIMULATION_MODE=true
ENABLE_REAL_CONTROL=false
AUTO_CONTROL_ENABLED=false
CONFIRM_ECOFLOW_WRITE=
REAL_CONTROL_TRIAL_MINUTES=0
REAL_CONTROL_TRIAL_UNTIL=
```

EcoFlow への実書き込みは、以下がすべて揃った場合だけ実行されます。

- `MOCK_MODE=false`
- `SIMULATION_MODE=false`
- `ENABLE_REAL_CONTROL=true`
- `AUTO_CONTROL_ENABLED=true`
- `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`
- `REAL_CONTROL_TRIAL_MINUTES` が正の値、または `REAL_CONTROL_TRIAL_UNTIL` が未来時刻
- `ECOFLOW_ACCESS_KEY` / `ECOFLOW_SECRET_KEY` / `ECOFLOW_DEVICE_SN` が設定済み

実制御でも、最小コマンド間隔、最小差分、連続買電/売電回数、AC充電下限/上限、バックアップリザーブ制御を通して、頻繁な送信を避けます。家庭配線や分電盤に関わる改造は電気工事士に依頼してください。EcoFlow AC 出力を家庭用コンセントや系統へ戻す接続は絶対に行わないでください。

## 構成

- `backend`: Go API server / control loop / SQLite store
- `frontend`: Next.js Static Export dashboard
- `backend/data`: ローカル実行時の SQLite / runtime logs
- Docker Compose: home server / mini PC / Raspberry Pi / NAS 向け起動

主要 API:

- `GET /api/status`: 現在値、余剰追従計画、夜間充電計画
- `GET /api/logs`: 制御ログ
- `GET /api/energy-meter-logs`: Nature Remo E 電力量ログ
- `GET /api/surplus-control-command-logs`: 余剰追従コマンド履歴
- `GET /api/night-charge-plan-logs`: 夜間充電計画履歴
- `GET /api/night-charge-summaries`: 夜間充電の日次検証
- `GET /api/solar-forecast`: PV 発電予測

## 初回セットアップ

```bash
cp .env.example .env
cd frontend
npm install
npm run build
cd ../backend
go test ./...
go build -o bin/server ./cmd/server
```

mock/simulation のまま Docker Compose で起動する場合:

```bash
docker compose up -d --build
```

確認先:

- 管理画面: http://localhost:8080/
- status API: http://localhost:8080/api/status

停止:

```bash
docker compose down
```

## 実稼働用 `.env`

実稼働でローカルの `backend/bin/server` を使う場合は、cwd に左右されないように `DB_PATH` と `FRONTEND_DIR` は絶対パスにしてください。

```env
APP_ENV=local
HTTP_PORT=18085
DB_PATH=/Users/sato/go/src/github.com/eisles/energy-controller/backend/data/energy-real-observe.db
FRONTEND_DIR=/Users/sato/go/src/github.com/eisles/energy-controller/frontend/out

MOCK_MODE=false
SIMULATION_MODE=false
ENABLE_REAL_CONTROL=true
AUTO_CONTROL_ENABLED=true
CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND
REAL_CONTROL_TRIAL_MINUTES=0
REAL_CONTROL_TRIAL_UNTIL=YYYY-MM-DDTHH:MM:SS+09:00

NATURE_MODE=cloud
NATURE_ACCESS_TOKEN=...
NATURE_APPLIANCE_ID=...

ECOFLOW_ACCESS_KEY=...
ECOFLOW_SECRET_KEY=...
ECOFLOW_DEVICE_SN=...
ECOFLOW_BASE_URL=https://api-e.ecoflow.com

POLL_INTERVAL_SEC=30
MIN_COMMAND_INTERVAL_SEC=60
MIN_COMMAND_DIFF_W=100
MIN_CHARGE_W=400
MAX_CHARGE_W=1500
TARGET_EXPORT_BUFFER_W=150

NOTIFICATION_ENABLED=false
NOTIFICATION_PROVIDER=slack
SLACK_WEBHOOK_URL=
MANUAL_CHARGE_ALERT_EXPORT_W=700
MANUAL_CHARGE_ALERT_SOC=95
MANUAL_CHARGE_ALERT_CONSECUTIVE=3
MANUAL_CHARGE_ALERT_COOLDOWN_MINUTES=30
```

`REAL_CONTROL_TRIAL_UNTIL` は実書き込み許可の期限です。長期運用では `REAL_CONTROL_TRIAL_MINUTES` よりも、再起動で期限がずれない `REAL_CONTROL_TRIAL_UNTIL` を使ってください。値は運用者が明示的に決め、管理画面のヘッダーと `/api/status` で期限と有効状態を確認してください。

Docker Compose で実稼働する場合も、同じ `.env` の値を使います。Compose 内部では container port は `8080`、host port は `${HTTP_PORT}` です。

### 売電過多の手動充電通知

EcoFlow の AC 充電上限が `MAX_CHARGE_W` に到達している、または SOC が `MANUAL_CHARGE_ALERT_SOC` 以上で、なお `MANUAL_CHARGE_ALERT_EXPORT_W` 以上の売電が続く場合、別バッテリーの手動充電を促す Slack 通知を送れます。通知は read-only alert で、EcoFlow の制御コマンドや write gate には影響しません。

```env
NOTIFICATION_ENABLED=true
NOTIFICATION_PROVIDER=slack
SLACK_WEBHOOK_URL=https://hooks.slack.com/services/...
MANUAL_CHARGE_ALERT_EXPORT_W=700
MANUAL_CHARGE_ALERT_SOC=95
MANUAL_CHARGE_ALERT_CONSECUTIVE=3
MANUAL_CHARGE_ALERT_COOLDOWN_MINUTES=30
```

Slack webhook URL は secret として扱い、repository に保存しないでください。通知は `MANUAL_CHARGE_ALERT_CONSECUTIVE` 回連続で条件を満たした場合だけ送信し、同じ条件では cooldown 中に再送しません。送信成否は SQLite の `notification_logs` に保存されます。

## ローカル実稼働起動

```bash
cd backend
./bin/server
```

起動後に確認します。

```bash
curl -sS http://localhost:18085/api/status | jq '{mode,state,lastDecisionReason,lastError,gridW,importW,exportW,batterySoc,batteryInputW,batteryOutputW,acChargeLimitW,backupReserveSoc,surplusPlan,nightChargePlan}'
curl -sS http://localhost:18085/api/devices/statuses | jq '.[] | {name,kind,statusSource,controlEnabled,status}'
tail -f data/real-control-continuous.log
```

実制御で見るべき点:

- `lastError` が `null`
- `surplusPlan.wouldWrite` / `nightChargePlan.wouldWrite` または command 履歴で、実行条件に応じた write 候補と送信結果を確認する
- `commandSent=true` の履歴がある場合、EcoFlow app と read-only status で反映を確認する
- 買電時は AC 充電を増やしすぎず、売電時だけ余剰吸収候補になる
- `decisionReason` にコマンド抑制、ガード、SOC、PV予測、夜間計画の理由が残る

## 制御方針

余剰追従:

- Nature Remo E の `exportW` と EcoFlow の `batteryOutputW` / `batteryInputW` を見て、売電を減らす方向に AC 充電上限やバックアップリザーブを調整します。
- EcoFlow app 上の AC 充電下限は 400W として扱います。
- 400W 充電が大きすぎる場面では、バックアップリザーブと SOC の関係を使った pass-through 的な挙動も候補にします。
- 買電へ戻った場合は充電を抑制し、既定リザーブへ戻す候補を出します。

夜間充電:

- 23:00-07:00 を安価な深夜時間帯として扱います。
- 翌日の時間別日射量から PV 有効時間帯と推定 PV 発電量を作り、EcoFlow 特定回路の過去出力平均、朝 07:00 から PV 開始までの消費、制御対象外負荷を加味して推奨 SOC を決めます。
- 翌日の PV で十分充電できる見込みなら深夜充電を抑え、不足見込みなら深夜に必要分だけ充電する方針です。
- TOU / self-powered / backup reserve の切り替えは実機観測に基づくため、必ずログと EcoFlow app で確認してください。

DELTA Pro 3 の SN は充電機器マスタで管理します。`ECOFLOW_ACCESS_KEY` / `ECOFLOW_SECRET_KEY` / `ECOFLOW_BASE_URL` は EcoFlow Cloud 認証情報として使い、通常の読み取り・余剰追従/夜間充電 write 対象は `kind=ecoflow_delta_pro3`、`statusSource=ecoflow_cloud`、`enabled=true`、`deviceSn` 設定済みのマスタ行から解決します。実機 write はさらに `controlEnabled=true` と既存の real-control gate が必要です。`ECOFLOW_DEVICE_SN` は移行期間の読み取りフォールバックと one-shot CLI 検証用に残していますが、自動 write のフォールバックには使いません。

DELTA Pro 3 のサイクル数は、Cloud REST quota に `cycles` 系 key が含まれる場合はその値を使います。Cloud 側に key がない場合だけ、EcoFlow app private 認証から取得した MQTT 証明書で Developer MQTT quota topic (`/open/{certificateAccount}/{sn}/quota`) を read-only subscribe し、`cycles` / `bmsCycles` / `bmsBattCycles` など名前付き key がある場合だけ表示用に補完します。未命名 protobuf field はサイクル数として扱わず、Developer MQTT quota の取得失敗は SOC や充電制御 status には影響させません。

## ロールバック

実制御を止める場合は、`.env` を以下に戻して再起動します。

```env
MOCK_MODE=true
SIMULATION_MODE=true
ENABLE_REAL_CONTROL=false
AUTO_CONTROL_ENABLED=false
CONFIRM_ECOFLOW_WRITE=
REAL_CONTROL_TRIAL_MINUTES=0
REAL_CONTROL_TRIAL_UNTIL=
```

ローカルプロセス停止:

```bash
lsof -tiTCP:18085 -sTCP:LISTEN | xargs kill
```

Docker Compose 停止:

```bash
docker compose down
```

## one-shot 実機検証 CLI

自動制御ではなく 1 command だけ確認したい場合は、`ecoflow-write-test` を使います。通常は `--execute` を付けず dry-run してください。

```bash
cd backend
go run ./cmd/ecoflow-write-test --watts 1000 --expected-current-limit 1500
```

実送信する場合:

```bash
cd backend
MOCK_MODE=false \
SIMULATION_MODE=false \
ENABLE_REAL_CONTROL=true \
AUTO_CONTROL_ENABLED=false \
CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND \
go run ./cmd/ecoflow-write-test --execute --watts 1000 --expected-current-limit 1500
```

バックアップリザーブや energy strategy modes の検証も同 CLI で行えます。実行前後は必ず `/api/status` と EcoFlow app の両方で反映を確認してください。

## DELTA_3 系 private MQTT/protobuf 検証 CLI

DELTA 3 Plus / DELTA 3 Max Plus 相当機で public API から SOC や入出力が取れない場合は、`ecoflow-delta3-probe` で EcoFlow app 系の private MQTT/protobuf 経路を検証できます。これは LAN 内ローカル API ではなく、EcoFlow cloud MQTT broker に接続する検証ツールです。

既定は read-only probe または dry-run です。自動制御 loop からは呼ばれません。

```env
ECOFLOW_DELTA3_READ_ENABLED=false
ECOFLOW_PRIVATE_API_HOST=api.ecoflow.com
ECOFLOW_PRIVATE_EMAIL=...
ECOFLOW_PRIVATE_PASSWORD=...
ECOFLOW_DELTA3_MQTT_CLIENT_ID=
ECOFLOW_DELTA3_TIMEOUT_SEC=20
ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=false
ECOFLOW_DELTA3_EXECUTE=false
ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=false
```

DELTA 3 Plus の `deviceSn` と `deviceType` は dashboard の「設定」→「充電機器マスタ」で管理します。`statusSource=ecoflow_private_mqtt` の EcoFlow 機器は、上記の `ECOFLOW_PRIVATE_EMAIL` / `ECOFLOW_PRIVATE_PASSWORD` を共通認証として使い、機器ごとの SN/type はマスタ値を使います。one-shot CLI 検証だけは `ecoflow-delta3-probe --sn ... --device-type ...` で指定できます。

DELTA 3 Plus 補助充電の自動 write は機器マスターを正とします。対象機器を `enabled=true`、`controlEnabled=true`、`supportsAcChargeLimit=true` にしたうえで、既存の real-control gate（`ENABLE_REAL_CONTROL=true`、`SIMULATION_MODE=false`、`AUTO_CONTROL_ENABLED=true`、`CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`、実制御期限内）と DELTA 3 Plus private API 用の追加安全 gate（`ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=true`、`ECOFLOW_DELTA3_EXECUTE=true`、`ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=true`）を満たす場合だけ実行候補になります。SN/type は機器マスター、送信可否はマスターと安全 gate の両方で判定します。

Developer MQTT quota のサイクル数だけを確認する場合は、write 機能を持たない read-only CLI を使います。

```bash
cd backend
go run ./cmd/ecoflow-developer-mqtt-probe --sn <DEVICE_SN>
```

出力には `cycleCount`、`cycleCountSource`、採用 key、受信 key 数だけを含め、SN や認証情報は表示しません。

read-only probe:

```bash
cd backend
go run ./cmd/ecoflow-delta3-probe --timeout 20s
```

AC充電Wの dry-run:

```bash
cd backend
go run ./cmd/ecoflow-delta3-probe --set-ac-charge-w 100
```

grid bypass disabled の dry-run:

```bash
cd backend
go run ./cmd/ecoflow-delta3-probe --grid-bypass-disabled=true
```

残り候補の dry-run:

```bash
cd backend
go run ./cmd/ecoflow-delta3-probe --min-discharge-soc 10
go run ./cmd/ecoflow-delta3-probe --max-charge-soc 95
go run ./cmd/ecoflow-delta3-probe --energy-backup-enabled=false --energy-backup-start-soc 25
```

dashboard で DELTA 3 Plus の read-only 状態を表示する場合は、private API 認証情報に加えて `ECOFLOW_DELTA3_READ_ENABLED=true` を設定します。表示対象は SOC、AC入力/出力、AC充電上限、`gridBypassDisabled`、`acOutputEnabled` です。取得失敗時は dashboard 内で unavailable と表示し、既存 `/api/status` には影響しません。

DELTA Pro 3 の AC 出力が一時的に 0W / OFF になった原因を後から追えるように、EcoFlow Cloud の read-only quota から温度、警告、異常、保護、AC出力状態に関係しそうなキーだけを `ecoflowDiagnostics` として `/api/status` と `/api/logs` に保存します。この診断情報は観測用で、温度や警告に基づく自動停止制御や AC 出力 ON/OFF 操作は行いません。

one-shot CLI の実送信は、自動制御とは別の検証経路のため `--execute` と `--allow-private-api-write` が必要です。既存の自動制御を止めずに DELTA_3 系の one-shot 検証を行う場合は、追加で `--allow-auto-control-overlap` が必要です。

```bash
cd backend
MOCK_MODE=false \
SIMULATION_MODE=false \
ENABLE_REAL_CONTROL=true \
AUTO_CONTROL_ENABLED=false \
CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND \
go run ./cmd/ecoflow-delta3-probe --set-ac-charge-w 100 --execute --allow-private-api-write
```

自動制御を止めずに one-shot 検証する場合:

```bash
cd backend
MOCK_MODE=false \
SIMULATION_MODE=false \
ENABLE_REAL_CONTROL=true \
AUTO_CONTROL_ENABLED=true \
CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND \
go run ./cmd/ecoflow-delta3-probe --set-ac-charge-w 100 --execute --allow-private-api-write --allow-auto-control-overlap
```

この CLI の write 対象は `set_ac_charge_power`、`set_backup_reserve_soc`、`set_grid_bypass_disabled`、`set_min_discharge_soc`、`set_max_charge_soc`、`set_energy_backup_enabled` の one-shot 検証だけです。SwitchBot 制御や既存の余剰追従自動制御には接続しません。
