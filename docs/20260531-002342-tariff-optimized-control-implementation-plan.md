# Tariff Optimized Control Implementation Plan

## Goal

料金時間帯を固定時刻ではなく料金マスタから解決し、平日/休日/祝日で異なる単価を考慮して、次の制御を行う。

- 高単価時間帯は、残量に余裕がある場合に充電を抑制し、放電しやすい設定へ戻す。
- 低単価時間帯は、翌日のPV予測と日中必要量を見て、不足分だけ深夜充電する。
- 売電中は従来どおり余剰吸収を優先する。ただし買電が発生したら料金帯に応じて充電を抑える。

## Non-Goals

- 実機write gateを緩めない。
- EcoFlow/Natureの認証情報やデバイスSNを計画・ログに出さない。
- 祝日APIのネットワーク依存は追加しない。初期実装は既存の簡易祝日判定を使い、後で祝日マスタ/APIへ差し替え可能にする。
- 電力会社ごとの複雑な燃料調整費/再エネ賦課金の完全再現は対象外。

## Current State

- `tariff_plans` は `day_rate_yen`、`home_rate_yen`、`night_rate_yen`、`export_rate_yen` を持つ。
- 現在の料金区分は `backend/internal/store/tariff_repository.go` の `tariffPeriod` で固定判定している。
  - night: 23:00-7:00
  - day: 平日 9:00-17:00
  - home: 上記以外、休日/祝日の日中
- 制御ロジックは主に売電/買電ベースで、料金帯を直接の制御入力として扱っていない。
- 既存の未コミット差分として、DELTA 3 Max Plus制御と機器ステータスUI調整がある。今回の実装ではこれを戻さない。

## Proposed Data Contract

### Backend Domain

新しい料金時間帯ルールを追加する。

- `TariffPeriodRule`
  - `id`
  - `tariffPlanId`
  - `dayType`: `weekday` / `holiday`
  - `period`: 表示用の区分名。例: `day` / `home` / `night`
  - `startMinute`: 0-1439
  - `endMinute`: 1-1440、日跨ぎは `startMinute > endMinute` で表現
  - `rateYen`
  - `priority`

既存互換のため、ルール未設定時は現在の `day/home/night` から既定ルールを生成する。

### Control Input

`TariffControlContext` を追加する。

- `CurrentPeriod`
- `CurrentRateYen`
- `LowestRateYen`
- `HighestRateYen`
- `IsLowPrice`
- `IsHighPrice`
- `HighPriceUntil`
- `DayType`

制御側は「23時まで」などの固定時刻ではなく、料金マスタで次に低単価へ切り替わる時刻を参照する。

## Control Behavior

### High Price Period

条件:

- 現在料金が高単価帯。
- 現在SOCが制御下限SOCより十分高い。
- PV予測/深夜充電計画から、次の低単価帯まで放電しても安全余力が残る。

動作:

- AC充電上限を最小値へ下げる。
- バックアップリザーブを制御下限へ下げる。
- TOU/セルフパワー等のモードは既存の安全ルールに従う。
- 売電中なら余剰吸収を優先し、買電中なら放電優先にする。

### Low Price Period

条件:

- 現在料金が低単価帯。
- 翌日PV予測と機器ごとの日中必要量から不足がある。

動作:

- 深夜充電計画の目標SOCへ充電する。
- 既存の最小コマンド間隔、差分しきい値、実機write gateを維持する。

### Mid Price Period

条件:

- 最高単価でも最低単価でもない。

動作:

- 売電があれば余剰吸収。
- 買電なら無理な充電を抑制。
- 高単価帯ほど積極放電しない。

## Files Likely To Change

- Backend
  - `backend/internal/domain/status.go`
  - `backend/internal/store/migrations.go`
  - `backend/internal/store/tariff_repository.go`
  - `backend/internal/api/tariff_plans_handler.go`
  - `backend/internal/control/surplus_planner.go`
  - `backend/internal/control/night_charge_planner.go`
  - `backend/cmd/server/main.go`
- Frontend
  - `frontend/lib/types.ts`
  - `frontend/lib/api.ts`
  - `frontend/components/TariffPlanPanel.tsx`
  - `frontend/components/StatusCards.tsx`
- Tests
  - `backend/internal/store/*tariff*_test.go`
  - `backend/internal/control/*tariff*_test.go`
  - `backend/cmd/server/main_test.go`

## Implementation Steps

1. Add tariff period rule domain structs and SQLite migration.
2. Extend tariff repository to list/save period rules with backward-compatible default generation.
3. Replace fixed `tariffPeriod` usage with rule-based period resolution.
4. Add `TariffControlContext` generation for the current status timestamp.
5. Feed the context into surplus and night-charge planners.
6. Add high-price recovery behavior to avoid charging when buying power during high-price periods and sufficient battery energy exists.
7. Add low-price night-charge behavior that continues to use existing forecast/device target calculations.
8. Expose the active tariff period and optimization reason in status JSON.
9. Update the tariff settings UI to show/edit weekday and holiday time bands.
10. Add tests for weekday, holiday, cross-midnight periods, high-price discharge, and low-price night charge.

## Safety Boundaries

- Real EcoFlow writes remain behind `ENABLE_REAL_CONTROL=true`, `SIMULATION_MODE=false`, auto-control, confirmation, trial window, and private-write gates.
- The implementation changes decision inputs and candidates, not the write safety gates.
- Any new write path must be covered by unit tests.
- If tariff rules are invalid or absent, fall back to existing behavior and log/read-only display the fallback reason.

## Review Points

- Migration is backward-compatible for existing SQLite DBs.
- Rule resolution handles:
  - weekday vs holiday
  - cross-midnight periods
  - overlapping periods by priority
  - missing rules by fallback
- High-price discharge does not drop below device/system reserve lower bounds.
- Existing surplus absorption still wins during real export.
- Frontend does not require credentials or reveal secrets.

## Verification Commands

- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./...`
- `cd frontend && rtk npm run build`
- `HTTP_PORT=8080 rtk docker compose up -d --build`
- `rtk curl -s http://localhost:8080/api/status`
- `rtk docker compose down`

## Rollback / Operational Notes

- If tariff rule resolution fails, the repository should use the old 3-period default mapping.
- Operators can disable practical impact by leaving rates equal or using the existing default tariff periods.
- Do not apply migrations manually to production without backing up SQLite.
