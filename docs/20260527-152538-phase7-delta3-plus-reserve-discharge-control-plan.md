# Phase 7 DELTA 3 Plus リザーブ下げ放電制御 実装計画

## Goal

DELTA 3 Plus が十分な残量を持つのに、買電中でもパススルーまたは微充電に留まる状態を改善する。

充電機器マスタの `reserveSoc` を「自動制御が一時的に下げてよい最低バックアップリザーブ SOC」として扱い、買電時には DELTA 3 Plus の backup reserve をこの値まで下げることで放電へ寄せる。

## Non-goals

- EcoFlow の新しい未検証 API コマンドは追加しない。
- DELTA 3 Plus の AC output ON/OFF 制御は追加しない。
- grid bypass の自動制御は追加しない。
- DELTA Pro 3 の既存余剰追従ロジックは変更しない。
- `.env` の専用ゲートを増やさない。
- 実機制御のデフォルト有効化はしない。

## Current State

- `backend/internal/control/delta3_aux_planner.go` は DELTA 3 Plus の AC充電上限と backup reserve を計画する。
- `backend/internal/control/delta3_aux_executor.go` は既に `SetEnergyBackupEnabled(ctx, enabled, startSoc)` を実行できる。
- `backend/cmd/server/main.go` の `delta3AuxSettingsForDevice` は機器マスタの `minChargeW` / `maxChargeW` を設定へ反映しているが、`reserveSoc` は DELTA 3 Plus 補助制御へ反映していない。
- 買電時の現行計画は AC充電上限を下げ、backup reserve が有効な場合は無効化する方向になっている。
- そのため、アプリ側の backup reserve や実機挙動によりパススルーになるケースでは、十分な SOC があっても放電へ寄せられない。

## Data And API Contract

### Charging Device Master

`charging_devices.reserve_soc` を DELTA 3 Plus 補助制御では以下の意味で扱う。

- 充電時: これを直接の充電目標にはしない。
- 買電回復時: 放電させてもよい最低 backup reserve SOC として使う。
- 例: `reserveSoc=20`、SOC 89% の場合、買電中は backup reserve を 20% に設定して放電余地を作る。

### Delta3AuxSettings

`Delta3AuxSettings` に `MinDischargeReserveSoc` を追加する。

- デフォルト: 20%
- `delta3AuxSettingsForDevice` で `device.ReserveSoc > 0` の場合はその値を使う。
- 5-100% の範囲へ正規化する。EcoFlow private API の `ValidateBackupReserveSoc` が 5% 未満を拒否するため、下限は 5% とする。

### Delta3AuxPlan

既存の `RecommendedBackupReserveSoc` / `ShouldSetBackupReserve` / `ShouldDisableBackupReserve` を使う。

- 買電中で SOC が `MinDischargeReserveSoc` より十分高い場合:
  - `RecommendedBackupReserveSoc = MinDischargeReserveSoc`
  - `ShouldSetBackupReserve = true`
  - `ShouldDisableBackupReserve = false`
- SOC が下限以下の場合:
  - reserve を下げない。
- current backup reserve が取得できない場合:
  - 既存ガードどおり write しない。

## Safety Boundaries

- 実機 write は既存ガードを維持する。
  - `MOCK_MODE=false`
  - `SIMULATION_MODE=false`
  - `ENABLE_REAL_CONTROL=true`
  - `AUTO_CONTROL_ENABLED=true`
  - `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`
  - trial window active
  - `ECOFLOW_DELTA3_READ_ENABLED=true`
  - charging device master `controlEnabled=true`
  - `ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=true`
  - `ECOFLOW_DELTA3_EXECUTE=true`
  - `ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=true`
- AC充電上限は既存の `AC出力W + AC充電W <= maxChargeW` 相当の安全上限を維持する。
- 405 / AC出力OFF の再発を避けるため、AC出力制御や grid bypass 制御は追加しない。
- backup reserve を下げる対象は DELTA 3 Plus 補助制御対象の1台目だけに限定する。
- command interval / duplicate fingerprint は既存ロジックを維持する。

## Implementation Steps

1. `Delta3AuxSettings` に `MinDischargeReserveSoc` を追加する。
2. `DefaultDelta3AuxSettings` と `normalizeDelta3AuxSettings` で 5-100% に正規化する。
3. `backend/cmd/server/main.go` の `delta3AuxSettingsForDevice` で `device.ReserveSoc` を `MinDischargeReserveSoc` に反映する。
4. 買電回復時の backup reserve 制御を変更する。
   - 既存の「有効な reserve を無効化する」だけではなく、SOC が十分なら `MinDischargeReserveSoc` へ設定する。
   - `ShouldSetBackupReserve` を使って `SetEnergyBackupEnabled(true, target)` を実行する。
   - SOC が下限以下の場合は変更しない。
5. 既存の余剰充電時 backup reserve 引き上げ制御は維持する。
6. 単体テストを追加・修正する。
   - 買電中、SOC 89%、reserveSoc 20%、現 reserve 88% なら target 20% へ下げる。
   - 買電中、current reserve が 0 / disabled でも target 20% を設定候補にできる。
   - SOC が reserveSoc 以下なら reserve を下げない。
   - `delta3AuxSettingsForDevice` が機器マスタの `reserveSoc` を反映する。

## Review Points

- backup reserve を下げる条件が買電時に限定されていること。
- current reserve 不明時に write しない既存ガードが残っていること。
- 実機 write ガードを緩めていないこと。
- DELTA Pro 3 の既存制御に影響しないこと。
- 既存の AC出力安全上限制御と競合しないこと。

## Verification

```sh
cd backend && rtk go test ./...
cd frontend && rtk npm run build
```

実機 write は追加検証コマンドでは行わない。ローカルサーバー稼働中であれば `GET /api/status` と `GET /api/delta3/status` で `delta3AuxPlan` の候補値だけ確認する。

## Rollback / Operational Notes

- 問題が出た場合は充電機器マスタの DELTA 3 Plus `controlEnabled=false` で DELTA 3 Plus 補助制御だけ止められる。
- さらに必要なら `.env` の `DELTA3_AUX_ENABLED=false` または `ECOFLOW_DELTA3_EXECUTE=false` で write を止める。
- `reserveSoc` は「普段のアプリ設定」ではなく「自動制御が下げてよい最低SOC」として運用する。
