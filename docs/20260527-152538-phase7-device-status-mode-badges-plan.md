# Phase 7 充電機器ステータス・モード表示改善 実装計画

## 目的

管理画面の「充電機器ステータス」で、各充電機器が現在 `充電中`、`放電中`、`パススルー`、`待機`、`取得不可` のどれなのかを、数値を読まなくてもすぐ分かるようにする。

あわせて、取得できているモードや設定状態を要約し、DELTA Pro 3 と DELTA 3 Plus がどの状態で動いているかを一覧内で確認しやすくする。

## 非目的

- 実機制御ロジックは変更しない。
- EcoFlow への write API は追加しない。
- 新しい backend API や DB migration は追加しない。
- EcoFlow private MQTT の raw decode 追加は今回の範囲外とする。

## 現状

- `frontend/components/StatusCards.tsx` の `Delta3StatusCard` で機器ごとの SOC、AC入力、AC出力、AC充電上限などを表示している。
- `frontend/lib/types.ts` には機器ごとの `acInW`、`acOutW`、`acChargeLimitW`、`backupReserveSoc`、`backupReserveEnabled`、`gridBypassDisabled`、`acOutputEnabled` が既にある。
- 現在は AC入力/出力の数値を読めば状態を推測できるが、状態名としては表示されていない。

## 方針

frontend 側で既存の read-only 値から表示用ステータスを推定する。

判定方針:

- 取得不可: `device.status.available` が false
- 取得不可: `acInW` または `acOutW` が null/undefined。接続はできていても電力値が欠けている場合は、状態を `待機` に丸めず、電力値未取得として扱う。
- 判定前に `AC出力` は `abs(AC出力)` へ正規化する。EcoFlow private MQTT 由来の値が負値でも、表示・判定では消費/出力の大きさとして扱う。
- パススルー: `AC入力 >= 50W` かつ `abs(AC出力) >= 50W` かつ `abs(AC入力 - abs(AC出力)) <= 100W`
- 充電中: `AC入力 - abs(AC出力) > 100W`
- 放電中: `abs(AC出力) - AC入力 > 100W`
- 待機: 上記以外

表示方針:

- 機器名の右側に状態 Badge を出す。
- 状態の下に `AC入力 / AC出力 / 実質W` の短い要約を出す。
- 設定状態として、`AC出力 ON/OFF`、`バックアップ ON/OFF`、`Grid bypass`、`最大充電SOC`、`最低放電SOC` をモード要約として表示する。
- 状態 Badge の色は `充電中=success`、`放電中=warning`、`パススルー=secondary`、`待機=secondary`、`取得不可=destructive` とする。

## 変更予定ファイル

- `frontend/components/StatusCards.tsx`
  - 充電機器ごとの状態判定 helper を追加する。
  - 機器カードのヘッダーに状態 Badge と短い要約を追加する。
  - detail strip に `状態`、`実質W`、`モード` を追加する。
- `frontend/app/globals.css`
  - 状態サマリー行の最小限のスタイルを追加する。

## 安全境界

- read-only 表示だけを変更する。
- `ENABLE_REAL_CONTROL`、`SIMULATION_MODE`、EcoFlow write gate、機器マスタ、制御ログ保存には触れない。
- 実機への追加 write は行わない。
- `.env`、認証情報、シリアル番号は変更しない。

## レビュー観点

- パススルー判定が同時入出力の誤読を減らすこと。
- 負値の AC 出力が来ても `abs()` で表示・判定できること。
- 取得不可機器で null 値を参照して UI が落ちないこと。
- 既存の detail 情報が消えないこと。
- build が通ること。

## 検証コマンド

```bash
cd frontend && rtk npm run build
rtk codex review --uncommitted
```

## 運用メモ

この変更は表示改善のみ。実機制御が期待通りかは、既存の DELTA 3 Plus 補助充電ログと実機ステータスで別途確認する。
