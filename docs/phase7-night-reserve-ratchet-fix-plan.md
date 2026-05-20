# Phase 7 夜間充電バックアップリザーブ自己増殖 修正計画

## 目的

夜間充電計画で、プログラムが設定した `backupReserveSoc` を次回計算の最低確保SOCとして再利用し、推奨SOCが 42% -> 54% -> 66% -> 78% -> 90% -> 100% と自己増殖する問題を修正する。

修正後は、夜間計画の最低確保SOCはユーザー設定の `minimumReserveSoc` または安全なデフォルト値だけを使う。現在の EcoFlow `backupReserveSoc` は、計算結果を実機へ反映する必要があるかを判定する比較値としてだけ使う。

## 背景

2026-05-21 03:38 以降の実機ログで、夜間計画が以下のように段階的にバックアップリザーブを引き上げた。

```text
03:38 reserve=42
03:39 reserve=54
03:40 reserve=66
03:41 reserve=78
03:43 reserve=90
03:44 reserve=100
```

これは、雨予報や日中消費から 100% が必要と判定した結果ではない。直近の正常計算では、推奨夜間SOCは約 51% で、現在SOCが 98% のため追加充電不要だった。

## 原因

`backend/internal/control/night_charge_planner.go` の `PlanNightCharging` は、現在の `BackupReserveSoc` が 30% より大きい場合に、それを `MinimumReserveSoc` として採用している。

```go
minReserveSoc := 30
if input.BackupReserveSoc != nil && *input.BackupReserveSoc > minReserveSoc {
    minReserveSoc = *input.BackupReserveSoc
}
```

そのため、プログラムが前回設定した `backupReserveSoc` が次回の最低確保SOCへ入り、朝まで消費や安全マージンを再加算して、推奨SOCが段階的に上がる。

## 修正方針

1. `BackupReserveSoc` を `MinimumReserveSoc` の算出から外す。
2. `MinimumReserveSoc` は以下だけで決める。
   - デフォルト 30%
   - `WeatherLocation.MinimumReserveSoc`
3. `BackupReserveSoc` は `ShouldSetBackupReserve` の比較値としてだけ使う。
4. 夜間計画が自分で設定した `backupReserveSoc` を次回計算で再度足し込まないことを単体テストで固定する。
5. 既存の self-powered / TOU / energy-strategy-off の mode 推奨と write guard は維持する。

## 実装手順

1. `PlanNightCharging` の `minReserveSoc` 初期化から `input.BackupReserveSoc` の取り込みを削除する。
2. 既存テストの期待値を確認し、必要なら「設定値の minimum reserve が優先される」テストへ寄せる。
3. 新規テストを追加する。
   - `BackupReserveSoc=90`、`SolarSettings.MinimumReserveSoc=30`、十分な蓄電量ありの入力で、推奨夜間SOCが 90% 以上へ引きずられないこと。
   - 前回 reserve を 42% にした次回計算でも、最低確保SOCが 42% へ増えないこと。
4. `go test ./...` を実行する。
5. frontend 型や表示は既存 `minimumReserveSoc` / `recommendedBackupReserveSoc` の意味を維持するため、必要がなければ変更しない。

## 安全境界

- EcoFlow write API の追加・変更はしない。
- 実機制御条件は変更しない。
- `ENABLE_REAL_CONTROL` / `SIMULATION_MODE` / `CONFIRM_ECOFLOW_WRITE` / trial window guard は維持する。
- 修正中は実制御サーバーを停止し、旧ロジックが追加コマンドを送らないようにする。
- 実装後にサーバーを再開する場合は、テストとレビュー完了後に行う。

## 完了条件

- バックアップリザーブ自己増殖を再現する単体テストが失敗から成功へ変わる。
- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./...` が通る。
- 実装計画レビューで重大指摘がない。
- 実装レビューで重大指摘がない。
