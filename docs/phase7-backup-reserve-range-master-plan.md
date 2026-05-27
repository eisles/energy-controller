# Phase 7: 機器マスターによるバックアップリザーブ制御範囲

## Goal

充電機器マスターに、EcoFlow 本体のバックアップリザーブ残量をこのシステムが動かしてよい範囲として `backup_reserve_min_soc` / `backup_reserve_max_soc` を追加する。

これにより、DELTA 3 Plus 補助制御では以下の判断を機器マスターの範囲内で行う。

- 買電中: 放電させたい場合、バックアップリザーブを `backup_reserve_min_soc` まで下げる。
- 売電中: 充電・パススルーへ誘導したい場合、バックアップリザーブを `backup_reserve_max_soc` 以下で上げる。
- どちらの場合も、機器マスターの最小/最大範囲を超える実機write候補を作らない。

## Non-goals

- EcoFlow の新しい実機write API は追加しない。
- `ENABLE_REAL_CONTROL=true` / `SIMULATION_MODE=false` の既存安全ゲートは変更しない。
- DELTA 3 Plus 2台目以降の配分ロジックは変更しない。
- DELTA Pro 3 の夜間充電計画、SwitchBot 制御、通知制御は今回の対象外。
- `.env` の認証情報やシリアル番号は変更しない。

## Current State

- `charging_devices.reserve_soc` は、現在は自動制御が下げてよい最低放電残量として使われている。
- `target_soc` は充電目標として存在するが、バックアップリザーブの上限として明示されていない。
- `Delta3AuxSettings.MinDischargeReserveSoc` は買電時にリザーブを下げる下限として使われる。
- 売電時の `maybeSetDelta3BackupReserve` は、現在残量から段階的にリザーブを上げるが、機器マスター由来の明示的な上限を持たない。

## Data/API Contract

### `charging_devices`

新規カラムを追加する。

| column | type | meaning |
| --- | --- | --- |
| `backup_reserve_min_soc` | INTEGER NOT NULL DEFAULT 0 | このシステムがバックアップリザーブを下げてよい最小残量% |
| `backup_reserve_max_soc` | INTEGER NOT NULL DEFAULT 0 | このシステムがバックアップリザーブを上げてよい最大残量% |

既存データの移行:

- `backup_reserve_min_soc = reserve_soc` when `backup_reserve_min_soc = 0`
- `backup_reserve_max_soc = target_soc` when `backup_reserve_max_soc = 0`
- 正規化では 5-100% に丸める。
- 保存時は `backup_reserve_min_soc` / `backup_reserve_max_soc` の永続値として 0 を許可しない。API クライアントが 0 または未指定で送信した場合は、保存前に `reserve_soc` / `target_soc` から 5-100% の実値へ補完する。
- `backup_reserve_max_soc < backup_reserve_min_soc` の場合は保存時に拒否する。

### Backend Domain/API

`domain.ChargingDevice` と JSON に以下を追加する。

- `backupReserveMinSoc`
- `backupReserveMaxSoc`

既存 API `/api/settings/charging-devices` は同じエンドポイントのまま、追加フィールドを返す。

### Frontend

`ChargingDevice` 型と設定画面に以下を追加する。

- `バックアップリザーブ最小%`
- `バックアップリザーブ最大%`

画面上の意味:

- `最低放電残量`: 実機から取得した機器側の下限値として表示する。機器マスターの制御下限とは混ぜない。
- `最大充電残量`: 実機から取得した機器側の充電上限として表示する。バックアップリザーブ最大値とは別概念。
- `本体リザーブ残量`: 実機から取得した現在のバックアップリザーブ設定。
- `リザーブ制御範囲`: システムが実機へ設定してよい最小-最大。

## Control Contract

`control.Delta3AuxSettings` に以下を追加する。

- `BackupReserveMinSoc`
- `BackupReserveMaxSoc`

後方互換:

- `MinDischargeReserveSoc` は既存テスト互換のため残す。
- `BackupReserveMinSoc` が未指定なら `MinDischargeReserveSoc` を使う。
- `BackupReserveMaxSoc` が未指定なら、対象機器の `target_soc` を使う。対象機器がない純粋な default settings の場合だけ 90% を既定値にする。

制御ルール:

1. 買電中:
   - `SOC > BackupReserveMinSoc` かつ現在の `BackupReserveSoc` が取得済みなら、`RecommendedBackupReserveSoc = BackupReserveMinSoc` を候補にする。
   - すでに `BackupReserveMinSoc` かつ enabled の場合は候補を作らない。
2. 売電中:
   - `BackupReserveMaxSoc`、実機 `MaxChargeSoc - buffer`、100% の最小値を上限とする。
   - 推奨リザーブは `BackupReserveMinSoc <= target <= BackupReserveMaxSoc` に収める。
   - 現在残量以上に上げる必要がある場合だけ候補を作る。
3. 取得不能:
   - `BackupReserveSoc` が取得不能な場合、推測で実機write候補を作らない。

## Safety Boundaries

- 実機writeの可否は既存の `ENABLE_REAL_CONTROL` / `SIMULATION_MODE` / `controlEnabled` / command interval / fingerprint guard に従う。
- 今回は制御範囲の計算と master 設定追加であり、write gate は緩めない。
- 新規カラムの初期値は既存 `reserve_soc` / `target_soc` から作り、意図しない 100% 固定にはしない。
- 既存の `reserve_soc` は互換用として残し、保存時に `backup_reserve_min_soc` と同期させる。

## Implementation Steps

1. DB migration
   - `charging_devices` に `backup_reserve_min_soc` / `backup_reserve_max_soc` を追加。
   - 既存行へ `reserve_soc` / `target_soc` から初期値を補完。
   - migration test でカラム存在を確認。
2. Domain/repository/API
   - `domain.ChargingDevice` にフィールド追加。
   - `ChargingDeviceRepository` の SELECT/INSERT/UPDATE/scan を更新。
   - 保存時の validation で永続値は 5-100 と min<=max を検証する。
   - 古いクライアント互換のため、保存前に 0/未指定の min/max は `reserve_soc` / `target_soc` から実値へ補完する。
3. Control settings
   - `Delta3AuxSettings` に min/max を追加。
   - `delta3AuxSettingsForDevice` で機器マスターの値を反映。
   - `normalizeDelta3AuxSettings` で 5-100 に丸め、max<min を補正。
4. Planner
   - 買電時は `BackupReserveMinSoc` へ下げる。
   - 売電時は `BackupReserveMaxSoc` を上限にして上げる。
   - 既存理由文と日本語表示を必要に応じて調整。
5. Frontend
   - `ChargingDevice` 型、空フォーム、編集フォーム、保存前 validation を更新。
   - ステータスカードに `リザーブ制御範囲` を表示する。
6. Tests
   - migration test。
   - repository/domain roundtrip。
   - planner unit test: 買電時に min へ下げる、売電時に max を超えない、max<min の補正。
   - frontend build。

## Review Points

- `reserve_soc` と新規 min/max の意味が混ざっていないこと。
- 実機 `backupReserveSoc` が取得不能な場合に推測writeしないこと。
- max を超えてバックアップリザーブを上げないこと。
- 既存 DB でも migration 後に API が落ちないこと。
- 既存 `.env` や secrets に触れていないこと。

## Verification Commands

```bash
cd backend && rtk go test ./...
cd frontend && rtk npm run build
rtk codex review --uncommitted
```

## Rollback / Operational Notes

- 変更は schema additive なので既存行は保持される。
- 実機制御を止める場合は、機器マスターの `controlEnabled=false` または既存 `.env` の安全ゲートで停止する。
- 既存 `reserve_soc` は互換用に残すため、古い画面/API呼び出しが即時に壊れることは避ける。
