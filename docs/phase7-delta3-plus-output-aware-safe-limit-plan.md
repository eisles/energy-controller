# Phase 7 DELTA 3 Plus AC出力考慮安全上限制御 実装計画

## 目的

DELTA 3 Plus がエラーコード 405 を表示して AC 出力 OFF になる事象を受け、補助充電制御で `AC充電上限 + AC出力W <= 安全上限` を守る。

今回の変更は、DELTA 3 Plus への充電指示をより保守的にするためのものであり、AC 出力 ON/OFF の自動復帰は実装しない。

## 非目的

- AC 出力を自動で ON に戻す制御は実装しない。
- EcoFlow private MQTT の未解析エラーコード取得は今回の範囲外とする。
- DELTA Pro 3 の余剰追従制御は変更しない。
- 実機制御 gate の既定値や安全条件を緩めない。

## 現状

- DELTA 3 Plus 補助制御は AC 充電上限とバックアップリザーブを調整している。
- AC 出力 ON/OFF を直接切り替える書き込み処理はない。
- 405 発生時、液晶には 405 のみ表示される。
- 既存の `Delta3AuxSettings.MaxChargeW` は DELTA 3 Plus の充電上限として使われている。
- 機器マスタには `minChargeW` / `maxChargeW` があるため、制御対象ごとの安全上限として利用できる。

## 方針

DELTA 3 Plus の推奨 AC 充電上限を以下で制限する。

```text
safeAcChargeLimitW = max(minChargeW, roundDownTo100(maxChargeW - abs(acOutW)))
recommendedAcChargeLimitW <= safeAcChargeLimitW
```

これにより、例えば `maxChargeW=1500W`、`AC出力=410W` の場合は、推奨 AC 充電上限を `1000W` までに制限する。

## データ/API契約

`delta3AuxPlan` に以下を追加する。

- `delta3AcOutputW`
  - DELTA 3 Plus の現在 AC 出力W。
  - 不明または 0 以下なら省略可能。
- `safeAcChargeLimitW`
  - AC 出力を考慮した推奨上限の上限値。

既存の `recommendedAcChargeLimitW` は、この安全上限以下に丸めた値として返す。

## 実装手順

1. `backend/internal/domain/status.go`
   - `Delta3AuxPlan` に `Delta3ACOutputW` と `SafeACChargeLimitW` を追加する。

2. `backend/internal/control/delta3_aux_planner.go`
   - `abs(acOutW)` から AC 出力負荷を算出する。
   - `safeAcChargeLimitW` を計算する helper を追加する。
   - READY / RECOVERING / 現在値過大の各ケースで推奨 AC 充電上限を安全上限以下に制限する。
   - 現在 AC 充電上限が安全上限を超えている場合は `SAFE_LIMIT` として下げる候補を出す。

3. `backend/cmd/server/main.go`
   - DELTA 3 Plus の制御設定に機器マスタの `minChargeW` / `maxChargeW` を反映する。

4. `frontend/lib/types.ts`
   - `Delta3AuxPlan` 型に追加フィールドを反映する。

5. `frontend/components/StatusCards.tsx`
   - 補助計画に `AC出力考慮上限` と `DELTA 3 Plus出力` を表示する。

6. `frontend/lib/display-labels.ts`
   - `SAFE_LIMIT` と新しい理由文を日本語表示にする。

7. テスト
   - AC 出力がある状態で現在 AC 充電上限が安全上限を超えると `SAFE_LIMIT` で下げること。
   - 安全上限に達しているパススルー時は、AC 充電上限を上げず、必要ならバックアップリザーブだけを候補にすること。

## 安全境界

- 実機 write は既存 gate を通す。
- `ENABLE_REAL_CONTROL=true`、`SIMULATION_MODE=false`、`MOCK_MODE=false`、確認文字列、試験期限、DELTA 3 private write 許可が揃わない限り送信しない。
- AC 出力 ON/OFF の自動操作は行わない。
- 405 検知時の自動復帰は行わない。

## レビュー観点

- `AC充電上限 + AC出力W <= maxChargeW` が planner の候補値で守られていること。
- `ACOutW` が負値でも正しく絶対値で扱われること。
- 機器マスタの `maxChargeW` が反映されること。
- 実機 write gate が緩んでいないこと。
- UI は read-only 表示に留まること。

## 検証コマンド

```bash
rtk go test ./...    # backend directory
rtk npm run build    # frontend directory
```

## 運用メモ

405 が再発する場合は、DELTA 3 Plus の自動制御は無効のままにし、次フェーズでエラーコード/保護状態の raw 診断ログ保存を検討する。
