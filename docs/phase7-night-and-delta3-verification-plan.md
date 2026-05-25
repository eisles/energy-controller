# Phase 7 夜間充電実績検証と DELTA 3 Plus 補助制御実証 計画

## 目的

夜間充電計画が実際に期待通りだったかを、計画値と実績値の差分で確認できるようにする。あわせて、DELTA 3 Plus 補助充電制御を実運用へ進める前に、現在の状態・直近ログ・安全ゲートを read-only で確認できる画面を追加する。

対象は次の2点に限定する。

1. 夜間充電計画の実績検証
2. DELTA 3 Plus 補助制御の安全な実証

## 非目的

- EcoFlow の新しい実機書き込みコマンドは追加しない。
- `.env` の実制御フラグ、DELTA 3 Plus 書き込みフラグ、試験期限は変更しない。
- DELTA 3 Plus の private API 認証情報、device SN、token はコード・計画・ログに保存しない。
- 夜間充電計画の算出ロジック自体はこのタスクでは変更しない。
- 通知機能や SwitchBot 制御はこのタスクでは実装しない。

## 現状

- `/api/night-charge/summaries` は日次の夜間サマリーを返している。
- 既存サマリーには計画SOC、7:00 SOC、夜間の買電/売電、Battery input/output、翌日日中のBattery input/売電が含まれる。
- ただし「目標SOCとの差」「夜間の実質充電kWh」「必要充電との差」がAPIレスポンスとしては明示されていないため、画面上で計画の当たり外れを判断しにくい。
- `/api/delta3/status` と `/api/delta3/aux-commands` はDELTA 3 Plusの状態と補助制御ログを返している。
- ただし、直近ログから「安全実証として今どういう状態か」をまとめて見る画面がない。

## 安全境界

- 実機書き込み条件は既存のまま維持する。
  - `ENABLE_REAL_CONTROL=true`
  - `SIMULATION_MODE=false`
  - `MOCK_MODE=false`
  - `AUTO_CONTROL_ENABLED=true`
  - `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`
  - 試験期限内
  - DELTA 3 Plus 補助制御は追加で `DELTA3_AUX_ENABLED=true`
  - DELTA 3 Plus は追加で `ECOFLOW_DELTA3_READ_ENABLED=true`
  - DELTA 3 Plus は追加で `ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=true`
  - DELTA 3 Plus は追加で `ECOFLOW_DELTA3_EXECUTE=true`
  - DELTA 3 Plus は追加で `ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=true`
- 今回の実装は、APIレスポンスの追加計算と画面表示の追加が中心。
- 既存DBのスキーマ変更は行わない。保存済みログから都度計算する。
- read-only / dry-run / guarded write の区別を画面で崩さない。

## データ/API契約

### `/api/night-charge/summaries`

既存の `NightChargeDailySummary` に以下を追加する。

- `morningTargetSocGap`
  - `nightEndSoc - plannedTargetSoc`
  - 7:00 SOCが計画より何ポイント上/下かを示す。
- `nightNetBatteryKwh`
  - `nightBatteryInputKwh - nightBatteryOutputKwh`
  - 同時入出力時の実質充電量を示す。
- `nightRequiredChargeGapKwh`
  - `nightNetBatteryKwh - plannedRequiredChargeKwh`
  - 計画上必要だった深夜充電量に対して実績がどれだけ多い/少ないかを示す。
- `daytimeChargeAndExportKwh`
  - `daytimeBatteryInputKwh + daytimeExportKwh`
  - 翌日日中に観測できたBattery inputと売電の合算参考値。
  - Battery inputには買電由来の充電も混ざり得るため、PV由来とは断定しない。

これらは保存せず、レスポンス生成時に既存ログから算出する。

### フロントエンド表示

新しいダッシュボードブロック `実証検証` を追加する。

- 夜間充電実績検証
  - 最新サマリー日
  - 目標SOCと7:00 SOC
  - 目標SOCとの差
  - 夜間実質充電kWh
  - 必要充電との差
  - 翌日日中の充電+売電
  - 07:00判定と16:00判定
- DELTA 3 Plus 補助制御実証
  - 接続状態
  - SOC / 最大充電SOC
  - 現在AC充電上限 / 推奨AC充電上限
  - 残余売電 / 安全余力
  - 直近25件のログ集計
  - 送信候補、送信済み、エラーの有無
  - 現在の抑制理由

## 変更予定ファイル

- `backend/internal/domain/status.go`
  - `NightChargeDailySummary` に検証用の算出フィールドを追加。
- `backend/internal/store/night_charge_summary_repository.go`
  - 既存ログから追加フィールドを算出。
- `backend/internal/store/repository_test.go`
  - 追加フィールドの算出テストを追加。
- `frontend/lib/types.ts`
  - 追加フィールドの型を追加。
- `frontend/components/VerificationPanel.tsx`
  - 夜間充電実績検証とDELTA 3 Plus補助制御実証の表示を追加。
- `frontend/components/Dashboard.tsx`
  - `実証検証` ブロックを追加し、最新サマリー/DELTA 3 Plus状態を渡す。
  - 夜間充電実績検証は、夜間サマリーテーブルのページ状態を再利用せず、専用に `limit=1&offset=0` を取得する。
  - DELTA 3 Plus補助制御の検証集計はログテーブルのページ状態を再利用せず、専用に `limit=25&offset=0` を取得する。

## 実装手順

1. `NightChargeDailySummary` に検証用フィールドを追加する。
2. `buildSummary` で、計画SOCと7:00 SOC、Battery input/output、必要充電量、日中Battery input/売電から追加フィールドを算出する。
3. repository test で、SOC差分・夜間実質充電・必要充電との差・日中充電+売電を検証する。
4. frontend type を更新する。
5. `VerificationPanel` を追加する。
6. `Dashboard` に `verification` セクションを追加し、初期表示は開いた状態にする。
7. 検証用の夜間充電サマリーは、テーブル用の `nightChargeSummaryPage` とは独立した state/effect で常に最新1件を取得する。
8. 検証用のDELTA 3 Plus補助ログは、テーブル用の `delta3AuxCommandPage` とは独立した state/effect で常に最新25件を取得する。
9. build/test を実行する。
10. 実装レビューを回し、指摘があれば修正する。

## レビュー観点

- 追加フィールドが nil 安全に計算されること。
- power log の近似値と energy meter の差分値の既存扱いを壊さないこと。
- DELTA 3 Plusの画面が送信可能と誤解されないこと。
- 実機書き込みgateや `.env` を変更していないこと。
- frontend build が通ること。

## 確認コマンド

```bash
(cd backend && rtk go test ./...)
(cd frontend && rtk npm run build)
rtk git diff --check
rtk codex review --uncommitted
```

必要に応じて稼働中画面/APIも確認する。

```bash
rtk /usr/bin/curl -s 'http://localhost:18085/api/night-charge/summaries?limit=1&offset=0'
rtk /usr/bin/curl -s 'http://localhost:18085/api/delta3/aux-commands?limit=5&offset=0'
```

## ロールバック

- DBスキーマ変更なし。問題があれば該当コミットをrevertする。
- 追加フィールドはレスポンスの付加情報なので、既存フィールドの意味は変えない。
- 新しい画面ブロックは既存の開閉/並び替え機構に載せるため、表示上の問題があっても制御には影響しない。
