# 料金マスタ・料金判定表示強化 実装計画

## Goal

料金最適化制御を強める前に、オペレーターが画面上で次の2点を確認できるようにする。

- 料金プランごとの平日・休日/祝日の料金時間帯ルールを、編集フォームを開かずに一覧で確認できる。
- 現在の料金判定が、買電/売電状態と合わせて「充電優先・放電優先・深夜待ち」のどれに寄っているか分かる。

## Non-goals

- EcoFlow 実機write、AC出力、バックアップリザーブ、AC充電上限の制御条件は変更しない。
- 料金プラン保存APIの契約は変更しない。
- 祝日カレンダーや電力会社料金体系の自動取得は追加しない。
- 料金単価・時間帯ルールを自動補正する処理は追加しない。

## Current State

- `TariffPlanPanel` は料金プラン一覧と追加/編集フォームを分離済み。
- `TariffPlan` には `periodRules` と `periodRuleSource` があり、既定/カスタムを判別できる。
- `StatusCards` には `TariffControlCard` があり、現在区分・単価・次の低単価・解決元・理由を表示している。
- `EnergyStatus` には `importW` / `exportW` / `targetChargeW` / `tariffControl` が含まれる。

## Data/API Contracts

新しいAPIは追加しない。

利用する既存データ:

- `TariffPlan.periodRules`
- `TariffPlan.periodRuleSource`
- `EnergyStatus.importW`
- `EnergyStatus.exportW`
- `EnergyStatus.targetChargeW`
- `EnergyStatus.tariffControl`

フロントエンド内の表示用ヘルパーだけ追加する。

## UI Changes

### 料金プラン一覧

`TariffPlanPanel` の一覧ドロワーに、選択中または一覧行ごとの料金時間帯を読める表示を追加する。

- 一覧行の「時間帯」は、既定/カスタムと行数だけでなく、平日/休日の概要が分かる文字列にする。
- 一覧の下に「料金時間帯プレビュー」を追加し、現在時刻で有効な料金プランの時間帯を平日・休日/祝日で表示する。
- 明示的な時間帯ルールが空の既定ルール運用では、画面側で既存のデイ/ホーム/ナイト単価から既定時間帯を展開して表示する。
- プレビューは read-only とし、編集は既存どおり「新規追加」「編集」ドロワーでのみ行う。

### 料金判定表示

`StatusCards` の `TariffControlCard` を強化する。

- 現在の買電/売電と料金判定を組み合わせた「制御目安」を表示する。
- 例:
  - 売電中: 余剰吸収のため充電優先
  - 買電中 + 高単価: 充電抑制、可能なら放電優先
  - 買電中 + 低単価: 不足分の充電候補
  - 中間単価: 現在ルールに従う
- `tariff.reason` は引き続き表示し、解決元が料金時間帯マスタか既定ルールかを明示する。
- `tariffControl` が未取得の場合もカードは表示し、料金判定未取得として現在電力と理由を表示する。

## Safety Boundaries

- 実機write条件、`ENABLE_REAL_CONTROL`、`SIMULATION_MODE`、最小送信間隔、ヒステリシスは変更しない。
- DB migration は追加しない。
- 秘密情報、SN、token、password は表示・保存しない。
- 今回は表示強化だけで、制御出力は変えない。

## Implementation Steps

1. `TariffPlanPanel` に料金時間帯の read-only プレビューを追加する。
2. `TariffPlanPanel` に時間帯ルール概要の表示ヘルパーを追加する。
3. `periodRules` が空の場合も、既定ルールを read-only 表示用に展開する。
4. 未来開始のプランではなく、現在時刻で有効な料金プランをプレビュー対象にする。
5. `StatusCards` の `TariffControlCard` を `EnergyStatus` 全体を受け取る形に変更する。
6. `importW` / `exportW` / `targetChargeW` / `tariffControl` から制御目安文言を作る。
7. `tariffControl` 未取得時もカードを表示し、料金判定が未確定であることを明示する。
8. 必要なCSSを `globals.css` に追加する。

## Review Points

- 一覧画面に編集用 input が混ざらないこと。
- 料金時間帯プレビューは read-only であること。
- 表示文言が制御実行済みと誤読されないこと。
- 実機制御ロジックに差分がないこと。

## Verification

- `cd frontend && rtk npm run build`
- `rtk git diff --check`
- ブラウザで以下を確認する。
  - 料金プラン一覧を開いても編集フォームが出ない。
  - 料金時間帯プレビューが平日・休日/祝日で表示される。
  - 料金最適化制御カードに制御目安が表示される。

## Rollback / Operational Notes

- 表示のみの変更なので、問題があれば該当フロントエンド差分を戻せば制御動作は元に戻る。
- 料金判定の文言はあくまで目安であり、実際のwriteは既存の安全ゲートと制御ログで判断する。
