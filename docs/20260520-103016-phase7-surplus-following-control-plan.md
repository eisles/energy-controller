# Phase 7 余剰追従 AC 充電制御 実装計画

## 目的

Nature Remo E の売電量と EcoFlow DELTA Pro 3 の入出力状態を使い、売電を極力減らしつつ買電へ振れすぎないように AC 充電上限とバックアップリザーブを調整する。

この計画では、従来の「バッテリー出力 + 最低 AC 充電 400W + 安全マージン」を常に開始条件として使う保守的な制御を見直す。放電中から充電を始める局面と、すでに充電中で売電量を見ながら追従する局面を分ける。

## 安全境界

- デフォルトでは実機 write を行わない。
- 実機 write は以下がすべて成立する場合だけ許可する。
  - `MOCK_MODE=false`
  - `SIMULATION_MODE=false`
  - `ENABLE_REAL_CONTROL=true`
  - `AUTO_CONTROL_ENABLED=true`
  - `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`
- EcoFlow write は adapter 経由に閉じる。
- Nature Remo / EcoFlow の raw response を domain 層へ漏らさない。
- 最小コマンド間隔、最小差分、連続判定、期待値チェックを必須にする。
- 実機制御は dry-run、短時間 supervised run、通常運転の順で段階的に有効化する。

## 現状の課題

現在の余剰充電プランは、放電中から充電を開始する場合の安全条件としては妥当だが、すでに充電中の場合にも同じ条件を使うため、余剰を吸収しきれないことがある。

例:

```text
売電 488W
AC充電上限 500W
バッテリー入力 743W
バッテリー出力 246W
```

この状態では、すでに充電が始まっている。次の調整は「現在の AC 充電上限に、まだ残っている売電量から安全マージンを差し引いて足す」方が実測に合う。

```text
次のAC充電上限 = 現在AC充電上限 + 売電W - 安全マージン
例: 500W + 488W - 150W = 838W -> 800W
```

一方で、放電中から充電を始める場合は、EcoFlow が放電をやめて AC 充電へ寄ることで系統側の見え方が大きく変わる。そのため、開始判定ではバッテリー出力を加味する。

```text
開始に必要な売電 = バッテリー出力W + 最低AC充電W + 安全マージン
例: 360W + 400W + 150W = 910W
```

## 制御状態

外部へ返す `strategyState` は、既存の API / UI / log 互換性を優先して以下を維持する。

```text
IDLE
READY
CHARGING
RECOVERING
PASSTHROUGH
```

この計画で追加する `start` / `tracking` / `import recovery` の区分は、既存 state を置き換えるものではなく、planner 内部の判定理由または command log の補助分類として扱う。もし将来 state 名自体を変更する場合は、domain type、frontend type、既存ログ表示、検索条件、テスト、過去ログの扱いを同じ差分で移行する。

### `IDLE`

売電が小さい、SOC が目標以上、または必要な観測値が不足している状態。

- 実機 write はしない。
- read-only ログだけ残す。

### `READY` / start 判定

放電中または未充電で、売電量が充電開始条件を満たした状態。

開始条件:

```text
exportW >= batteryOutputW + minChargeW + safetyMarginW
```

初回 target:

```text
targetW = exportW - batteryOutputW - safetyMarginW
targetW = clamp(roundDownTo100(targetW), minChargeW, maxChargeW)
```

この局面では、バッテリー出力を差し引く。

### `CHARGING` / tracking 判定

すでに充電中で、売電量を見ながら AC 充電上限を追従させる状態。

充電中の判定候補:

```text
batteryInputW - batteryOutputW >= effectiveChargeThresholdW
または
ACChargeLimitW >= minChargeW かつ BackupReserveSoc > BatterySoc
```

追従 target:

```text
targetW = currentACChargeLimitW + exportW - targetExportBufferW
targetW = clamp(roundDownTo100(targetW), minChargeW, maxChargeW)
targetW = limitStep(currentACChargeLimitW, targetW, maxIncreaseStepW, maxDecreaseStepW)
```

この局面では、バッテリー出力を再度差し引かない。Nature Remo の売電量は、すでに現在の EcoFlow 入出力を含んだ結果だからである。

### `RECOVERING` / import recovery 判定

買電に振れた状態。

復帰 target:

```text
targetW = currentACChargeLimitW - importW - safetyMarginW
targetW = max(roundDownTo100(targetW), minChargeW)
```

AC 充電上限が `minChargeW` でも買電が続く場合:

- バックアップリザーブを通常値へ戻す。
- 必要なら TOU / self-powered mode の復帰を行う。
- 追加 write は cooldown 後に行う。

### `PASSTHROUGH` / small surplus 判定

売電はあるが、`batteryOutputW + minChargeW` を超えるほどではない状態。

例:

```text
売電 300W
バッテリー出力 250W
最低AC充電 400W
```

この場合、通常の AC 充電を始めると買電へ振れやすい。実機挙動として、バックアップリザーブを現在 SOC 付近へ合わせたパススルー状態で細かい余剰を吸収できる可能性がある。

実装は通常充電とは分け、専用条件を置く。

- TOU mode の扱いを明示する。
- BackupReserveSoc を現在 SOC へ合わせる。
- 買電検知時は即座に `RECOVERING` へ移る。
- 連続 write を防ぐ cooldown を長めに取る。

## 設定値

既存設定に加えて、以下を追加する。

```text
effectiveChargeThresholdW = 100
targetExportBufferW = 150
maxIncreaseStepW = 400
maxDecreaseStepW = 600
reserveRaiseStepPercent = 2
defaultReserveSoc = 30
passThroughEnabled = false
passThroughCooldownSec = 300
```

初期値は保守的にする。

- 通常の AC 充電追従は有効。
- パススルー制御は dry-run から開始し、実機ログで確認してから有効化する。

## Planner 実装方針

`backend/internal/control/surplus_planner.go` を以下の責務へ整理する。

1. 入力値を正規化する。
2. 既存の `IDLE` / `READY` / `CHARGING` / `RECOVERING` / `PASSTHROUGH` を維持しつつ、内部判定として start / tracking / import recovery / small surplus を分ける。
3. 推奨 AC 充電上限を計算する。
4. 推奨バックアップリザーブを計算する。
5. 実機 write 可否とは独立した純粋な plan を返す。

write 可否は executor 側で判定する。

## Executor 実装方針

server 常駐の自動制御 executor を追加する。ただし既存の one-shot CLI と同等以上の guard を持たせる。

実行条件:

```text
plan.WouldWriteCandidate=true
AUTO_CONTROL_ENABLED=true
ENABLE_REAL_CONTROL=true
SIMULATION_MODE=false
MOCK_MODE=false
CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND
前回コマンドから minCommandIntervalSec 以上
現在値が expected current と一致
```

実行する command:

- `SetACChargePower`
- `SetBackupReserveSoc`
- EcoFlow energy mode の切替は、対象 quota と実機挙動が明確なものだけに限定する。

注意:

- 既存の単純な `decision_engine` と同時に real write させない。
- 余剰追従 executor を有効にする場合、古い `SetACChargePower` 経路は dry-run または disabled にする。

## ログ設計

`power_logs` だけでは制御意図の追跡が弱いため、制御コマンド履歴を追加する。

候補テーブル:

```sql
CREATE TABLE surplus_control_command_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  measured_at TEXT NOT NULL,
  strategy_state TEXT NOT NULL,
  grid_w INTEGER NOT NULL,
  import_w INTEGER NOT NULL,
  export_w INTEGER NOT NULL,
  battery_soc INTEGER,
  battery_input_w INTEGER,
  battery_output_w INTEGER,
  previous_ac_charge_limit_w INTEGER,
  target_ac_charge_limit_w INTEGER,
  previous_backup_reserve_soc INTEGER,
  target_backup_reserve_soc INTEGER,
  command_sent INTEGER NOT NULL DEFAULT 0,
  dry_run INTEGER NOT NULL DEFAULT 1,
  suppressed_reason TEXT,
  decision_reason TEXT NOT NULL,
  error_message TEXT,
  created_at TEXT NOT NULL
);
```

画面では以下を表示する。

- 現在の制御状態
- 推奨 AC 充電上限
- 前回送信コマンド
- 抑制理由
- 買電復帰時の処理
- 直近の command log

## テスト計画

### Unit test

`backend/internal/control/surplus_planner_test.go` に追加する。

- 放電中は `batteryOutputW + minChargeW + margin` を開始条件にする。
- 充電中は `currentLimit + exportW - buffer` で追従する。
- 充電中は batteryOutputW を二重に差し引かない。
- 買電時は AC 充電上限を下げる。
- 最低 400W 未満へは設定しない。
- 最大 1500W を超えない。
- 1 回の増加幅を `maxIncreaseStepW` 以内に抑える。
- 1 回の減少幅を `maxDecreaseStepW` 以内に抑える。
- SOC 目標到達時はリザーブを通常値へ戻す。
- パススルー候補は通常充電と別 state の `PASSTHROUGH` で返す。
- 実機 write guard は `MOCK_MODE=true` で write candidate を実行不可にする。
- 実機 write guard は `CONFIRM_ECOFLOW_WRITE` 未設定または不一致で write candidate を実行不可にする。

### Integration / dry-run

- `SIMULATION_MODE=true` で plan と command log が記録されること。
- `MOCK_MODE=true` で write が抑制されること。
- `ENABLE_REAL_CONTROL=false` で write が抑制されること。
- `AUTO_CONTROL_ENABLED=false` で write が抑制されること。
- `CONFIRM_ECOFLOW_WRITE` 未設定または `I_UNDERSTAND` 以外で write が抑制されること。
- expected current mismatch で write が抑制されること。

### Supervised real run

最初は 5 分だけ実行する。

観測項目:

- `gridW`
- `exportW`
- `importW`
- `batteryInputW`
- `batteryOutputW`
- `netBatteryW`
- `acChargeLimitW`
- `backupReserveSoc`
- `command_sent`
- `error_message`

合格条件:

- 実行中に大きな買電へ振れない。
- 買電へ振れた場合、次の interval で AC 充電上限を下げる。
- 連続して同じ command を送らない。
- command log から判断理由を追跡できる。

## 実装ステップ

1. `SurplusPlan` に状態別の調整値を追加する。
2. `PlanSurplusCharging` を開始判定と充電中追従に分ける。
3. planner unit test を追加する。
4. command log table / repository を追加する。
5. dry-run executor を追加する。
6. server の自動制御経路に余剰追従 executor を接続する。
7. 実機 write guard と expected current check を one-shot CLI と揃える。
8. frontend に制御状態と command log を表示する。
9. dry-run で build / test を通す。
10. supervised real run を短時間だけ行い、DB ログで検証する。

## 未実装 TODO

- EcoFlow energy mode の read/write quota を実機でさらに確認する。
- パススルー制御の実機挙動を通常充電とは別に評価する。
- バックアップリザーブを上げる幅を `+2%` 固定にするか、目標 SOC まで直接上げるかを実測で決める。
- `targetExportBufferW` を固定値にするか、天気・時間帯・負荷変動で変えるかを検討する。
- 夜間充電計画との優先順位を整理する。
