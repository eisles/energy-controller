# Phase 7 DELTA 3 Plus 余剰補助充電制御 実装計画

## 目的

DELTA Pro 3 の余剰追従だけでは吸収しきれない売電がある場合に、DELTA 3 Plus を補助バッテリーとして使い、売電を減らす。

DELTA 3 Plus は実機検証で以下が確認済みである。

- read-only status 取得が可能。
- AC 充電上限 100W / 200W の one-shot write と readback が可能。
- 公式アプリ上では AC 充電の最低値が DELTA Pro 3 より低く、細かい余剰吸収に向いている。

今回の実装では、frontend の簡易計算に留めている補助充電候補を backend の planner / guard / log に移し、DELTA Pro 3 優先後の残余売電に対して DELTA 3 Plus の推奨 AC 充電上限を計算できるようにする。

## 非目的

- DELTA 3 Plus を DELTA Pro 3 より先に充電する制御はしない。
- SwitchBot スマートプラグ連携はこの計画では実装しない。
- DELTA 3 Plus の放電制御、AC output ON/OFF、grid bypass 設定変更はこの計画では実装しない。
- private API の秘密情報、device SN、token をログ・画面・docs に出さない。
- EcoFlow AC 出力を家庭用コンセントや系統へ戻す構成は扱わない。

## 現状

### DELTA Pro 3 側

`backend/internal/control/surplus_planner.go` に DELTA Pro 3 向けの余剰追従 planner がある。

- `IDLE`
- `READY`
- `CHARGING`
- `RECOVERING`
- `PASSTHROUGH`

`backend/internal/control/surplus_executor.go` には guard と executor があり、実機 write は以下が揃う場合だけ候補になる。

- `MOCK_MODE=false`
- `SIMULATION_MODE=false`
- `ENABLE_REAL_CONTROL=true`
- `AUTO_CONTROL_ENABLED=true`
- `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`
- real control trial window が有効
- minimum interval / duplicate suppression / expected current check が通る

### DELTA 3 Plus 側

`backend/internal/api/delta3_status_handler.go` に `GET /api/delta3/status` がある。

現在取得できる主な値:

- SOC
- AC入力
- AC出力
- AC充電上限
- 最大充電SOC
- 最低放電SOC
- backup reserve
- gridBypassDisabled
- acOutputEnabled

`frontend/components/StatusCards.tsx` の `buildAuxiliaryBatteryPlan()` は frontend 内で簡易的に推奨値を計算している。これは実行 guard や永続ログと連携していないため、実制御へ進めるには backend 側へ移す必要がある。

## 制御方針

### 優先順位

1. DELTA Pro 3 の既存余剰追従を優先する。
2. DELTA Pro 3 が追加吸収できない、または追加後も残余売電がある場合だけ DELTA 3 Plus を候補にする。
3. DELTA 3 Plus は 100W 単位の補助吸収先として扱う。
4. 買電に転じた場合は DELTA 3 Plus 補助充電を下げる、または停止する。

### 残余売電の考え方

Nature Remo E の `exportW` は家全体の系統結果であり、DELTA Pro 3 / DELTA 3 Plus の現時点の入出力を含んだ値である。

そのため DELTA 3 Plus の補助判断は、以下を分ける。

#### DELTA Pro 3 が未調整または調整候補あり

DELTA Pro 3 の `SurplusPlan` が AC 充電上限引き上げ候補を持つ場合、DELTA 3 Plus は原則待機する。

理由:

- 2 台を同時に増やすと買電へ振れやすい。
- どちらの write が効いたかログで切り分けにくい。

例外として、DELTA Pro 3 が上限に近い、満充電に近い、または mode / reserve 条件で追加吸収できない場合は DELTA 3 Plus を候補にできる。

#### DELTA Pro 3 が吸収不能または吸収済み

以下のいずれかなら DELTA 3 Plus の補助充電候補を作る。

- DELTA Pro 3 の `batterySoc >= targetSoc`
- DELTA Pro 3 の `acChargeLimitW >= maxChargeW`
- DELTA Pro 3 の `SurplusPlan` が `IDLE` / `PASSTHROUGH` / `RECOVERING` で、かつ DELTA Pro 3 の追加 AC 充電候補がない

以下の場合は DELTA 3 Plus を待機させる。

- 直近 DELTA Pro 3 command が minimum interval 内で、効果反映待ちの可能性がある
- 直近 DELTA Pro 3 command が expected current mismatch で抑制され、現在値の前提が崩れている
- 直近 DELTA Pro 3 command が error retry cooldown 中で、同じ原因が継続している

これらは「DELTA Pro 3 が吸収不能」ではなく「DELTA Pro 3 の状態確定待ち」として扱う。2 台を同時に増加 write しないため、DELTA 3 Plus の増加候補は Pro 3 command の settle / verification window が終わってから作る。

### DELTA 3 Plus 推奨 AC 充電上限

設定値:

```text
Delta3AuxEnabled=false
Delta3AuxMinChargeW=100
Delta3AuxMaxChargeW=1500
Delta3AuxSafetyMarginW=50
Delta3AuxMinCommandDiffW=100
Delta3AuxMaxIncreaseStepW=300
Delta3AuxMaxDecreaseStepW=500
Delta3AuxMinCommandIntervalSec=120
Delta3AuxStopImportThresholdW=50
Delta3AuxTargetMaxSocBufferPercent=2
```

推奨値:

```text
currentLimitW = currentDelta3ACChargeLimitW
residualHeadroomW = exportW - Delta3AuxSafetyMarginW
if currentLimitW is unknown:
    no command candidate
if residualHeadroomW < Delta3AuxMinCommandDiffW:
    hold currentLimitW
targetW = currentLimitW + roundDownTo100(residualHeadroomW)
targetW = clamp(targetW, Delta3AuxMinChargeW, Delta3AuxMaxChargeW)
targetW = limitStep(currentDelta3ACChargeLimitW, targetW, Delta3AuxMaxIncreaseStepW, Delta3AuxMaxDecreaseStepW)
```

`exportW` は DELTA 3 Plus の現在の充電負荷を差し引いた後の系統結果なので、次の target は `exportW` から絶対値を作り直さない。既に 700W で充電していて残余売電が 100W ある場合、target は 100W ではなく「700W を維持、または余力が閾値を超えるなら増加」として扱う。これにより、次ポーリングで充電上限を不用意に下げる oscillation を防ぐ。

`Delta3AuxMinChargeW` 未満の残余余力は、最低 AC 充電量へ切り上げない。小さい売電を 100W へ丸めて買電へ振らせないため、増加候補は `Delta3AuxMinCommandDiffW` 以上の残余余力がある場合だけ作る。

停止・抑制:

```text
importW >= Delta3AuxStopImportThresholdW
=> reduceW = roundUpTo100(importW + Delta3AuxSafetyMarginW)
=> targetW = max(Delta3AuxMinChargeW, currentLimitW - reduceW)
```

買電中に no-op を stop として扱わない。前回 command で DELTA 3 Plus が充電中の場合、no-op ではその充電上限が残り、買電を長引かせる可能性があるため、既知の安全最小値 `Delta3AuxMinChargeW` へ下げる。0W command は送らない。最小値でも買電が続く場合は、外部手段または手動停止が必要な状態として command log / dashboard に残す。

### SOC 条件

DELTA 3 Plus の SOC が以下の場合は補助候補から外す。

```text
delta3Soc >= min(delta3MaxChargeSoc, 100) - Delta3AuxTargetMaxSocBufferPercent
```

SOC が取得できない場合も補助 write 候補にしない。

### mode 条件

今回の自動制御 write 対象は DELTA 3 Plus の AC 充電上限だけに限定する。

- `gridBypassDisabled`
- `acOutputEnabled`
- backup reserve
- min discharge SOC
- max charge SOC

これらは read-only 条件としてログに残すが、自動変更しない。

## データ/API 設計

### backend domain

`backend/internal/domain/status.go` に `Delta3AuxPlan` を追加する。

候補フィールド:

```go
type Delta3AuxPlan struct {
    Mode                         string `json:"mode"`
    StrategyState                string `json:"strategyState"`
    RecommendedACChargeLimitW    int    `json:"recommendedAcChargeLimitW"`
    CurrentACChargeLimitW        *int   `json:"currentAcChargeLimitW,omitempty"`
    Delta3Soc                    *int   `json:"delta3Soc,omitempty"`
    Delta3MaxChargeSoc           *int   `json:"delta3MaxChargeSoc,omitempty"`
    ResidualExportW              int    `json:"residualExportW"`
    SafetyMarginW                int    `json:"safetyMarginW"`
    WouldWrite                   bool   `json:"wouldWrite"`
    ShouldAdjustACChargeLimit    bool   `json:"shouldAdjustAcChargeLimit"`
    SuppressedReason             string `json:"suppressedReason,omitempty"`
    Reason                       string `json:"reason"`
}
```

`domain.Status` に `Delta3AuxPlan *Delta3AuxPlan` を追加する。

### backend planner

`backend/internal/control/delta3_aux_planner.go` を追加する。

入力:

- `domain.Status`
- `Delta3StatusResponse` 相当の read-only status を domain 向けに変換した値
- 余剰追従 settings
- DELTA 3 Plus 補助 settings
- 直近 DELTA Pro 3 command log
- 直近 DELTA 3 Plus command log

出力:

- `domain.Delta3AuxPlan`

planner は pure function にし、network / DB / env を参照しない。

### backend guard / executor

`backend/internal/control/delta3_aux_executor.go` を追加する。

実機 write 候補条件:

```text
Delta3AuxEnabled=true
MOCK_MODE=false
SIMULATION_MODE=false
ENABLE_REAL_CONTROL=true
AUTO_CONTROL_ENABLED=true
CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND
real control trial window active
ECOFLOW_DELTA3_READ_ENABLED=true
ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=true
ECOFLOW_DELTA3_EXECUTE=true
ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=true
plan has command candidate
前回 DELTA 3 Plus command から minimum interval 以上
target と current の差分が Delta3AuxMinCommandDiffW 以上
同一 fingerprint ではない
```

`ECOFLOW_DELTA3_EXECUTE` と `ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE` は、既存 `ecoflowdelta3.WriteGuards` の `Execute` / `AllowPrivateAPIWrite` に対応する server-side 設定として追加する。どちらかが false の場合、planner は `WouldWrite=false` または executor の suppression として扱い、private API write を送信しない。

実行 command:

- `SetACChargePower(ctx, watts int)` のみ。

今回の計画では backup reserve / mode / output / grid bypass の write は追加しない。

### command log

新規テーブル候補:

```sql
CREATE TABLE delta3_aux_control_command_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  measured_at TEXT NOT NULL,
  strategy_state TEXT NOT NULL,
  grid_w INTEGER NOT NULL,
  import_w INTEGER NOT NULL,
  export_w INTEGER NOT NULL,
  residual_export_w INTEGER NOT NULL,
  delta3_soc INTEGER,
  previous_ac_charge_limit_w INTEGER,
  target_ac_charge_limit_w INTEGER,
  would_write INTEGER NOT NULL,
  dry_run INTEGER NOT NULL,
  command_sent INTEGER NOT NULL,
  command_fingerprint TEXT NOT NULL,
  suppressed_reason TEXT,
  decision_reason TEXT NOT NULL,
  error_message TEXT,
  created_at TEXT NOT NULL
);
```

API:

```text
GET /api/delta3/aux-plan
GET /api/delta3/aux-commands?limit=25&offset=0
```

`/api/status` にも `delta3AuxPlan` を含める。ただし DELTA 3 Plus read が失敗した場合は status 全体を失敗させず、`delta3AuxPlan.strategyState=UNAVAILABLE` とする。

### frontend

`frontend/components/StatusCards.tsx` の `buildAuxiliaryBatteryPlan()` を削除または fallback のみにし、backend の `delta3AuxPlan` を表示する。

表示項目:

- 補助計画 state
- 推奨 AC 充電上限
- 残余売電
- 安全余力
- 実行可否
- 抑制理由
- 理由

ログ:

- `DELTA 3 Plus 補助充電ログ` をログブロック群に追加する。
- API paging で `limit` / `offset` を使う。
- 既存の開閉・並び替え・localStorage ルールに従う。

## 実装ステップ

### 1. 設定追加

`backend/internal/config/config.go` と `.env.example` に DELTA 3 Plus 補助制御設定を追加する。

既定値は安全側にする。

```env
DELTA3_AUX_ENABLED=false
DELTA3_AUX_MIN_CHARGE_W=100
DELTA3_AUX_MAX_CHARGE_W=1500
DELTA3_AUX_SAFETY_MARGIN_W=50
DELTA3_AUX_MIN_COMMAND_DIFF_W=100
DELTA3_AUX_MAX_INCREASE_STEP_W=300
DELTA3_AUX_MAX_DECREASE_STEP_W=500
DELTA3_AUX_MIN_COMMAND_INTERVAL_SEC=120
DELTA3_AUX_STOP_IMPORT_THRESHOLD_W=50
DELTA3_AUX_TARGET_MAX_SOC_BUFFER_PERCENT=2
ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=false
ECOFLOW_DELTA3_EXECUTE=false
ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=false
```

### 2. domain / planner

- `Delta3AuxPlanInput`
- `Delta3AuxSettings`
- `PlanDelta3AuxCharging`

を追加する。

単体テスト:

- DELTA 3 Plus 未取得なら `UNAVAILABLE`
- SOC が最大充電 SOC 付近なら `FULL`
- 買電中なら `RECOVERING`
- DELTA Pro 3 が追加吸収候補を持つなら `WAIT_PRO3`
- DELTA Pro 3 が満充電 / 上限 / 吸収不能なら `READY`
- target は 100W 単位、min/max、step limit、安全余力を満たす
- 既に補助充電中の残余売電では current limit を維持し、絶対 target を残余売電だけで作り直さない
- 買電中は no-op ではなく安全最小値への減少候補を作る
- `Delta3AuxEnabled=false` では `WouldWrite=false`

### 3. repository / migration / API

- `current_status` に `delta3_aux_plan_json` を追加する。
- `StatusRepository.CurrentStatus` / `UpdateCurrentStatus` で `delta3AuxPlan` を保存・復元する。
- `delta3_aux_control_command_logs` migration を追加する。
- repository を追加する。
- `GET /api/delta3/aux-plan` を追加する。
- `GET /api/delta3/aux-commands` を追加する。
- `/api/status` に `delta3AuxPlan` を含める。

### 4. guard / executor

- DELTA 3 Plus 専用 guard evaluator を追加する。
- `ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL` を必須 gate にする。
- 実 writer は `ecoflowdelta3.Client` に `SetACChargePower` 相当の interface を切って注入する。
- writer 未設定時は dry-run log のみ。
- command error はログに残し、status 保存は止めない。

### 5. server 接続

`recordStatus` の流れに DELTA 3 Plus 補助 plan / command log を追加する。

順序:

1. Nature / DELTA Pro 3 status を取得。
2. DELTA Pro 3 surplus plan を作る。
3. DELTA 3 Plus read-only status を cache 経由で取得。
4. DELTA 3 Plus aux plan を作る。
5. guard evaluator で command log を作る。
6. gate が通る場合だけ `SetACChargePower` を呼ぶ。
7. command log を保存する。
8. current_status を保存する。

DELTA 3 Plus read に失敗しても既存 status / DELTA Pro 3 control を止めない。

### 6. frontend

- `EnergyStatus` 型に `delta3AuxPlan` を追加する。
- DELTA 3 Plus card の補助計画表示を backend plan に置き換える。
- `DELTA 3 Plus 補助充電ログ` を画面下部のログブロックへ追加する。
- ログは API paging にする。

### 7. README / 運用手順

README に以下を追記する。

- DELTA 3 Plus 補助充電の目的
- 既定では disabled
- dry-run から始める手順
- 実制御に必要な env
- trial window の使い方
- 買電へ振れたときの停止・確認手順

## レビュー観点

- 既定値で実機 write が走らないこと。
- DELTA Pro 3 と DELTA 3 Plus が同時に増加 write しないこと。
- DELTA 3 Plus の private API 認証を frontend に出さないこと。
- DELTA 3 Plus status 取得失敗で `/api/status` が壊れないこと。
- command interval / duplicate suppression / fingerprint が DELTA Pro 3 と独立していること。
- 100W 未満の売電で DELTA 3 Plus write 候補を作らないこと。
- 0W write を暗黙に追加しないこと。
- unit test が network なしで通ること。

## 検証コマンド

```bash
cd backend && rtk go test ./...
cd frontend && rtk npm run build
rtk docker compose up -d --build
rtk curl -fsS http://localhost:${HTTP_PORT:-8080}/api/status
rtk docker compose down
rtk git diff --check
rtk codex review --uncommitted
```

実機 dry-run / supervised run は実装後に別途、短時間で行う。

## ロールバック / 運用メモ

- `DELTA3_AUX_ENABLED=false` で補助充電 planner / write 候補を停止できる。
- `ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=false` で server からの DELTA 3 Plus write を止められる。
- `AUTO_CONTROL_ENABLED=false` で DELTA Pro 3 / DELTA 3 Plus の自動 write 全体を止められる。
- `SIMULATION_MODE=true` または `ENABLE_REAL_CONTROL=false` で実機 write を止められる。
- command log は削除せず、検証材料として残す。

## 未実装 TODO

- SwitchBot スマートプラグによる DELTA 3 Plus AC 給電 ON/OFF。
- DELTA 3 Plus の AC output / grid bypass / backup reserve 自動操作。
- DELTA Pro 3 と DELTA 3 Plus を統合した複数バッテリー optimizer。
- DELTA 3 Plus の充電効率を実測ログから学習する補正。
