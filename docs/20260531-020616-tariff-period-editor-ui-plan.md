# 料金時間帯編集UI 実装計画

## 目的

料金最適化制御で参照する平日・休日別の料金時間帯を、画面から確認・編集・保存できるようにする。

現状はバックエンド/API/ステータスに `periodRules` が追加済みだが、フロントエンドの料金プラン編集画面はデイ・ホーム・ナイト・売電の単価だけを保存している。このため、休日料金や祝日扱い、時間帯の変更をUIから管理できない。

## 非対象

- 新しい料金計算ロジックの追加
- 祝日カレンダーの詳細編集
- 実機EcoFlow write条件の変更
- 自動制御の強化
- Nature APIやEcoFlow APIへの通信変更

## 現状

- `backend/internal/domain/status.go`
  - `TariffPlan.periodRules` と `TariffPeriodRule` が定義済み。
- `backend/internal/api/tariff_plans_handler.go`
  - `periodRules` を受け取り、非空の場合は平日・休日それぞれ24時間をカバーすることを検証する。
  - `periodRules` 未送信時は既存ルールを保持する。
  - `periodRules: []` は明示的なカスタムルール削除として扱う。
- `frontend/lib/types.ts`
  - `TariffPeriodRule` 型が定義済み。
- `frontend/components/TariffPlanPanel.tsx`
  - 単価履歴の表示・編集・削除はある。
  - `periodRules` の表示・編集・送信はない。

## 変更方針

`TariffPlanPanel` に料金時間帯エディタを追加する。編集対象は選択中の料金プランに紐づく `periodRules` とし、保存時に単価と一緒に送信する。

UIは次の構成にする。

- 現在の料金プラン概要
  - 既存の単価表示に加えて、時間帯ルールが「既定ルール」か「カスタム」かを表示する。
- 編集ドロワー
  - 既存の「単価」セクションは維持する。
  - 「料金時間帯」セクションを追加する。
  - 平日・休日を分けて、行形式で時間帯を編集する。
  - 各行は `区分 / 開始 / 終了 / 単価 / 優先度 / 削除` を持つ。
  - 行追加、既定ルールへ戻す、単価から既定ルールを再生成する操作を置く。
- 履歴テーブル
  - 各料金プランの時間帯ルール数と、既定/カスタムの区別を表示する。

## データ契約

バックエンドの料金プランGETは、現在は保存済みカスタムルールがない場合も既定ルールを合成して `periodRules` に入れて返す。そのため、UIが「既定ルール」か「カスタム」かを判別できるよう、`TariffPlan` に読み取り専用の `periodRuleSource` を追加する。

```ts
type TariffPlan = {
  periodRules?: TariffPeriodRule[];
  periodRuleSource?: "default" | "custom";
};
```

`periodRuleSource` はGET応答と保存後応答で返す。POST payloadでは無視してよい。UIでは `periodRuleSource === "custom"` の場合だけカスタム扱いにし、「既定ルールへ戻す」は `periodRules: []` を明示送信して保存済みカスタムルールを削除する。

フロントエンドは保存時に次の `TariffPeriodRule[]` を `periodRules` として送る。

```ts
type TariffPeriodRule = {
  dayType: "weekday" | "holiday";
  period: string;
  startMinute: number;
  endMinute: number;
  rateYen: number;
  priority: number;
};
```

時刻入力は `HH:MM` とし、内部では `0..1440` の分に変換する。

日またぎの時間帯は、バックエンド仕様に合わせて `startMinute > endMinute` を許可する。例: `23:00-07:00` は `1380 -> 420`。

## 入力検証

保存前にフロントで次を検証する。

- `dayType` は `weekday` または `holiday`
- `period` は空にしない
- `startMinute` は `0..1439`
- `endMinute` は `1..1440`
- `rateYen` は `0 < rate <= 500`
- カスタムルールが1件以上ある場合、平日・休日それぞれが24時間をカバーする

バックエンド検証は最終防衛線として維持する。

## 既定ルール

UIで生成する既定ルールは現在のバックエンド既定と合わせる。

平日:

- ナイト `23:00-07:00`
- ホーム `07:00-09:00`
- デイ `09:00-17:00`
- ホーム `17:00-23:00`

休日:

- ナイト `23:00-07:00`
- ホーム `07:00-23:00`

単価はフォーム上の `nightRateYen` / `homeRateYen` / `dayRateYen` を使う。

## 安全境界

- この変更はUI/API payloadの編集のみで、実機write pathは変更しない。
- 既存の `ENABLE_REAL_CONTROL`、`SIMULATION_MODE`、`AutoControl`、最小コマンド間隔、write guard は変更しない。
- 秘密情報、デバイスシリアル、APIキーは扱わない。
- カスタムルールが不完全な場合は保存させず、バックエンドにも送らない。

## 実装手順

1. `TariffPlanPanel.tsx` に `periodRules` 用の state を追加する。
2. `TariffPlan` / API応答に `periodRuleSource` を追加する。
3. `editPlan` で既存プランの `periodRules` と `periodRuleSource` をフォームに読み込む。
4. `resetFormToCurrentPlan` で現在プランのルールも復元する。
5. 既定ルール生成 helper を追加する。
6. `HH:MM` と minute の変換 helper を追加する。
7. 料金時間帯行の追加・削除・更新UIを追加する。
8. 保存時にフロント検証を行い、`periodRules` を `saveTariffPlan` に渡す。
9. 履歴テーブルに時間帯ルール状態を表示する。
10. 必要なCSSを既存クラス体系に沿って追加する。
11. `frontend` の build を通す。
12. 影響範囲がAPI payload中心のため、既存backend testも実行する。

## レビュー観点

- `periodRules` 未送信と明示送信の意味を壊していないか。
- 既定ルールとカスタムルールの判別が `periodRuleSource` に基づいているか。
- 編集中プランのカスタムルールが、単価保存で消えないか。
- 平日・休日の24時間カバー検証がUIとAPIで一致しているか。
- `23:00-07:00` のような日またぎ時間帯を扱えるか。
- 実機制御の安全条件に変更が入っていないか。
- モバイル/狭幅でも編集行が破綻しないか。

## 検証コマンド

```bash
cd frontend && rtk npm run build
cd backend && rtk go test ./...
rtk git diff --check
rtk codex review --uncommitted
```

## ロールバック

問題があれば、フロントの `periodRules` 送信を一時的に外すことで既存の単価編集のみの動作に戻せる。バックエンドは `periodRules` 未送信時に既存ルールを保持するため、UIだけを戻しても保存済みカスタムルールは消えない。
