# Phase 7 DELTA Pro 3 SOC上下限ステータス表示 実装計画

## 目的

DELTA Pro 3 の EcoFlow Cloud read-only quota に存在する機器側の最大充電残量と最低放電残量を、バックエンドの状態モデルと管理画面へ正しく流す。

あわせて、画面上で「実機側の最低放電残量」と「Energy Controller がバックアップリザーブを動かしてよい制御範囲」が混ざって見える状態を解消する。

## 非目的

- EcoFlow 実機 write API は追加しない。
- 既存の real-control gate、dry-run、minimum interval、hysteresis は変更しない。
- DELTA 3 Plus private MQTT の認証・制御方式は変更しない。
- 充電機器マスタの `reserve_soc` / `target_soc` 削除は今回行わない。

## 現状

- DELTA Pro 3 の `quota/all` には以下が存在することを read-only で確認済み。
  - `cmsMaxChgSoc`
  - `cmsMinDsgSoc`
- `backend/internal/ecoflow/quota_adapter.go` は現在これらを `domain.BatteryStatus` に詰めていない。
- `backend/internal/api/delta3_status_handler.go` の DELTA Pro 3 Cloud status 変換でも `maxChargeSoc` / `minDischargeSoc` が返らない。
- `frontend/components/StatusCards.tsx` の機器ごと表示では、`最低放電残量` に `device.reserveSoc` を表示している箇所があり、実機値とシステム制御下限が混ざって見える。

## データ/API契約

### Backend domain

`domain.BatteryStatus` に read-only の機器側設定として以下を追加する。

- `MaxChargeSoc *int`
- `MinDischargeSoc *int`

### EcoFlow Cloud quota mapping

`BatteryStatusFromQuotas` で以下を読む。

- `cmsMaxChgSoc` -> `MaxChargeSoc`
- `cmsMinDsgSoc` -> `MinDischargeSoc`

キーが存在しない場合は `nil` とし、既存の起動・mock 動作を壊さない。

### API response

DELTA Pro 3 の `DeviceStatusResponse.status` に以下を返す。

- `maxChargeSoc`
- `minDischargeSoc`

DELTA 3 Plus 側の既存レスポンス契約は変更しない。

## Frontend 表示方針

機器ごとのステータス表示では以下の意味に分ける。

- `最大充電残量`: 実機から取得した `device.status.maxChargeSoc`
- `最低放電残量`: 実機から取得した `device.status.minDischargeSoc`
- `リザーブ制御範囲`: 機器マスタの `backupReserveMinSoc` / `backupReserveMaxSoc`
- `本体リザーブ`: 実機のバックアップリザーブ ON/OFF
- `本体リザーブ残量`: 実機のバックアップリザーブ残量

`device.reserveSoc` は互換/保存用の値として残すが、機器側の `最低放電残量` 表示には使わない。

## 安全境界

- 追加するのは read-only quota の変換と表示のみ。
- 実機 write コマンド、write payload、write gate、`.env` の実制御設定は変更しない。
- 秘密情報、SN、APIキーをログ・ドキュメント・テストに固定しない。
- 既存 DB migration の破壊的変更は行わない。

## 実装手順

1. `domain.BatteryStatus` に `MaxChargeSoc` / `MinDischargeSoc` を追加する。
2. `BatteryStatusFromQuotas` で `cmsMaxChgSoc` / `cmsMinDsgSoc` を取り込む。
3. DELTA Pro 3 Cloud status mapping で API レスポンスに反映する。
4. quota adapter の単体テストで新フィールドの mapping を検証する。
5. 機器ステータス表示の `最低放電残量` を `device.status.minDischargeSoc` に修正する。
6. 必要なら表示補助テキストを調整し、システム制御範囲との意味を分離する。

## レビュー観点

- DELTA Pro 3 の実機値と機器マスタ値が表示上混ざっていないこと。
- quota key が欠けても既存動作が壊れないこと。
- DELTA 3 Plus の表示・制御ロジックに不要な変更が入っていないこと。
- read-only 変更に留まり、実機 write の挙動を変えていないこと。

## 検証コマンド

```bash
cd backend && rtk go test ./...
cd frontend && rtk npm run build
rtk codex review --uncommitted
```

必要に応じて、ローカル画面 `http://localhost:18085/` で DELTA Pro 3 の `最大充電残量` / `最低放電残量` が `-` ではなく表示されることを確認する。

## ロールバック

今回の変更は read-only mapping と表示修正のみなので、問題があれば以下を戻す。

- `domain.BatteryStatus` の追加フィールド
- `BatteryStatusFromQuotas` の追加 mapping
- `mapEcoFlowCloudStatus` の追加 mapping
- `StatusCards.tsx` の表示参照先変更
