# Phase 7 バックアップリザーブ表示整理 実装計画

## 目的

充電機器ステータスと制御判断で表示している `バックアップ` / `Backup reserve` を、EcoFlow 本体の状態と Energy Controller の制御設定が混ざらない表記に整理する。

現状の `OFF / 30%` のような表示は、バックアップリザーブ機能が OFF なのか、30% が本体設定値なのか、制御下限なのかが分かりにくい。これを明示的に分ける。

## 非目的

- DELTA Pro 3 / DELTA 3 Plus の制御ロジックは変更しない。
- EcoFlow への write API 呼び出しは追加しない。
- 機器マスタの保存形式や API 契約は変更しない。
- 実機のバックアップリザーブ値を推測・補正しない。

## 現状

- `frontend/components/StatusCards.tsx` の充電機器ステータスでは、`backupReserveEnabled` と `backupReserveSoc` / `reserveSoc` が同じ detail に混ざっている。
- DELTA Pro 3 の制御判断でも英語の `Backup reserve` 表記が残っている。
- DELTA 3 Plus の旧 fallback 表示でも `推奨バックアップ` / `現在バックアップ` が残っている。

## 表示方針

### 充電機器ごとの表示

- `本体リザーブ`: `backupReserveEnabled` の ON/OFF。
- `本体リザーブSOC`: 実機から取得した `backupReserveSoc`。
- `制御下限SOC`: 機器マスタの `reserveSoc`。DELTA 3 Plus 補助制御では「自動制御が一時的に下げてよい最低バックアップリザーブ SOC」として使う。

### モード要約

- `リザーブ OFF / 30%` のような混在表記はやめる。
- `本体リザーブ OFF` と `制御下限SOC 30%` を別項目として並べる。

### DELTA Pro 3 / DELTA 3 Plus 補助計画

- 英語の `Backup reserve` は日本語化する。
- `バックアップ` だけの表記は `リザーブSOC` または `本体リザーブSOC` に寄せる。
- `リザーブ解除` のような制御候補名は維持してよいが、対象が backup reserve であることが分かる文脈に置く。

## 変更ファイル

- `frontend/components/StatusCards.tsx`
  - 表示ラベルを整理する。
  - `backupReserveSummary` のような ON/OFF と SOC を混ぜる helper は使わず、状態と数値を別 detail に分ける。

## 安全境界

- read-only dashboard の表示だけを変更する。
- backend API、DB、制御 planner、executor、実機 write path は変更しない。
- `.env` や認証情報、シリアル番号は触らない。

## 実装手順

1. 充電機器一覧の detail strip を `本体リザーブ`、`本体リザーブSOC`、`制御下限SOC` に分割する。
2. `deviceModeSummary` も同じ語彙に揃える。
3. DELTA 3 Plus 旧表示 fallback と制御判断の `Backup reserve` を日本語にする。
4. 不要になった混在表示 helper を削除する。
5. frontend build で型と static export を確認する。

## レビュー観点

- `OFF / 30%` のように ON/OFF と別由来の SOC が混ざる表示が残っていないこと。
- DELTA Pro 3 と DELTA 3 Plus のどちらでも意味が通るラベルであること。
- 表示のみの変更で、制御や API の挙動を変えていないこと。

## 確認コマンド

- `cd frontend && rtk npm run build`
- `rtk codex review --uncommitted`

## 運用メモ

表示上の `制御下限SOC` は、実機本体のバックアップリザーブ状態ではなく Energy Controller の機器マスタ設定である。ユーザーが実機アプリ上の値と比較する時に混同しないよう、今後も本体状態と制御設定は分けて表示する。
