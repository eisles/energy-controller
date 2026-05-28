# Phase 7 余剰追従 executor ガード実装計画

## 目的

`PlanSurplusCharging` が出した余剰追従 plan を、server 常駐処理で実行判断できる形へ進める。

今回の範囲は **実機 write 直前までの executor ガード基盤** とする。EcoFlow 実機 write はまだ server / UI へ接続しない。

## 安全境界

- server / UI から EcoFlow 実機 write を呼ばない。
- `ecoflow.SignedWriteClient` を server provider へ接続しない。
- command log は `dry_run=true` / `command_sent=false` のまま保存する。
- 実機 write へ進むには、別差分で `command_sent=true` と write adapter 呼び出しを追加する。
- 既存の one-shot CLI の実機 write 経路は変更しない。

## 実装内容

### 1. config に confirmation を読む

`CONFIRM_ECOFLOW_WRITE` を config へ読み込む。ただし今回の server executor は、この値が一致しても実機 write しない。

用途:

- dry-run command log に「confirmation が不足しているため real write 不可」を残す。
- 次段階で real write を接続するとき、既存 one-shot CLI と同じ guard を server 側にも適用できるようにする。

### 2. executor guard evaluator を追加する

`backend/internal/control` に余剰追従 command guard evaluator を追加する。

入力:

- 現在 status
- `SurplusPlan`
- `MOCK_MODE`
- `SIMULATION_MODE`
- `ENABLE_REAL_CONTROL`
- `AUTO_CONTROL_ENABLED`
- `CONFIRM_ECOFLOW_WRITE`
- `MinCommandInterval`
- 前回 command log

出力:

- `domain.SurplusControlCommandLog`
- `dry_run=true`
- `command_sent=false`
- `would_write`
- `command_kind`
- `command_fingerprint`
- `suppressed_reason`
- `target_ac_charge_limit_w`
- `target_backup_reserve_soc`
- action flags
  - `should_adjust_ac_charge_limit`
  - `should_set_backup_reserve`
  - `should_disable_energy_modes`
  - `should_enable_tou_mode`

### 3. guard 条件

実行候補を `would_write=true` にできるのは、以下がすべて成立する場合のみ。

```text
plan has command candidate
MOCK_MODE=false
SIMULATION_MODE=false
ENABLE_REAL_CONTROL=true
AUTO_CONTROL_ENABLED=true
CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND
前回 command から MinCommandInterval 以上
同一 target command ではない
expected current が現在値と一致
```

今回の実装では `would_write=true` になっても `dry_run=true` / `command_sent=false` として保存する。

### 4. expected current check

expected current は command log の `previous_*` と現在 status の一致で扱う。

- AC 充電上限変更:
  - `previous_ac_charge_limit_w == status.acChargeLimitW`
- backup reserve 変更:
  - `previous_backup_reserve_soc == status.backupReserveSoc`
- mode 変更:
  - TOU / self-powered / scheduled / intelligent の4フラグがすべて取得済み
  - command 候補が前提にする expected 値と現在値が一致
  - 1つでも不明な場合は real write 不可として `mode status unavailable` を抑制理由にする

mode 操作の expected check は、既存 one-shot CLI と同等にする。余剰追従 planner の `ShouldDisableEnergyModes` / `ShouldEnableTOUMode` は mode 操作候補なので、AC / reserve だけで guard OK にしない。

今回の dry-run では、expected current mismatch が起きる構造は限定的だが、次段階の real write 接続前に log 形式と判定を固定する。

### 5. duplicate suppression

同一 command 抑制は target 値だけで判定しない。

以下を含む `command_fingerprint` を作り、前回 command log と比較する。

```text
command_kind
target_ac_charge_limit_w
target_backup_reserve_soc
should_adjust_ac_charge_limit
should_set_backup_reserve
should_disable_energy_modes
should_enable_tou_mode
```

これにより、AC / reserve が同じでも mode 操作だけ必要なケース、または mode 操作のみのケースを区別する。

### 6. repository

`surplus_control_command_logs` の直近 command を取得できるようにする。

用途:

- `MinCommandInterval`
- 同一 command 抑制

### 7. server 接続

`recordStatus` で以下の順に処理する。

1. status / power log / night plan log を保存
2. 直近 surplus command log を読む
3. guard evaluator で今回の command log を作る
4. `surplus_control_command_logs` へ保存
5. current_status を更新

失敗時:

- surplus command log 保存に失敗しても `Warn` に留める。
- current_status 更新は止めない。

## テスト計画

### Unit test

- `MOCK_MODE=true` で抑制されること。
- `SIMULATION_MODE=true` で抑制されること。
- `ENABLE_REAL_CONTROL=false` で抑制されること。
- `AUTO_CONTROL_ENABLED=false` で抑制されること。
- `CONFIRM_ECOFLOW_WRITE` 未設定/不一致で抑制されること。
- interval 未満では抑制されること。
- 同一 target command は抑制されること。
- mode 操作候補では TOU / self-powered / scheduled / intelligent の4フラグがすべて取得できないと抑制されること。
- mode 操作のみの command が、AC / reserve target と独立した fingerprint で扱われること。
- AC / reserve target が同じでも action flags が異なる場合は同一 command とみなさないこと。
- guard が全て通る場合でも `dry_run=true` / `command_sent=false` になること。

### Repository / server test

- 直近 surplus command log を取得できること。
- `recordStatus` が surplus command log を保存すること。
- surplus command log 保存失敗が current_status 更新を止めないこと。

## 完了条件

- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./...`
- `cd frontend && rtk npm run build`
- `rtk git diff --check -- ':!.serena' ':!backend/data'`
- サブエージェントレビューで `APPROVE`
