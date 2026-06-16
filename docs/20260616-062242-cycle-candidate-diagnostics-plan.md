# サイクル候補診断表示 実装計画

## 目的

DELTA 3 Plus 系の private MQTT サイクル候補について、現在採用された 1 件だけでなく、比較対象となる候補フィールドを画面/APIで同時に確認できるようにする。

特に以下を比較できる状態にする。

- `254/21/427`
- `254/22/280`
- DELTA 3 Max Plus 用の `32/50/85` など既存候補

## 非目的

- 候補値を正式なサイクル数として採用しない。
- 実機 write、制御判断、充電/放電ロジックは変更しない。
- EcoFlow 認証情報やシリアル番号を新規に保存・表示しない。

## 現状

- Backend は `cycleCountCandidate` として優先候補 1 件だけを返している。
- private MQTT の内部 status には `CycleFieldCandidates` があり、切り詰め前の候補を参照できる。
- Frontend は「サイクル候補」欄で 1 件だけ表示している。
- DELTA 3 Plus と DELTA 3 Plus 2 で、候補として採用される field が異なる可能性がある。

## 変更対象

- `backend/internal/api/ecoflow_device_status_handler.go`
  - `Delta3StatusResponse` に `cycleCountCandidates` を追加する。
  - preferred field 全件について、存在する候補を配列で返す。
  - 既存 `cycleCountCandidate` は互換性のため維持する。
- `backend/internal/api/ecoflow_device_status_handler_test.go`
  - DELTA 3 Plus の複数候補が返ることをテストする。
  - 正式 `cycleCount` がある場合は候補を出さない既存安全動作を確認する。
- `frontend/lib/types.ts`
  - `cycleCountCandidates?: CycleCountCandidate[]` を追加する。
- `frontend/components/StatusCards.tsx`
  - 採用候補表示に加え、候補一覧を短く表示する。
  - 画面の行が肥大化しないよう、候補は `値回 254/21/427` のような短い表記にする。

## API契約

既存:

```json
{
  "cycleCountCandidate": {
    "value": 79,
    "source": "ecoflow_private_mqtt_candidate",
    "cmdFunc": 254,
    "cmdId": 21,
    "field": 427
  }
}
```

追加:

```json
{
  "cycleCountCandidates": [
    {
      "value": 79,
      "source": "ecoflow_private_mqtt_candidate",
      "cmdFunc": 254,
      "cmdId": 21,
      "field": 427
    },
    {
      "value": 0,
      "source": "ecoflow_private_mqtt_candidate",
      "cmdFunc": 254,
      "cmdId": 22,
      "field": 280
    }
  ]
}
```

`cycleCountCandidate` は従来どおり最優先の 1 件を返す。

## 安全境界

- read-only 診断表示のみ。
- 実機 write command、制御条件、夜間充電・余剰充電の判断には使わない。
- `candidate` の confidence/reason を維持し、正式値と誤認させない。

## 実装手順

1. Backend response 型に `CycleCountCandidates` を追加する。
2. preferred field に一致する全候補を返す helper を追加する。
3. 既存の単一候補 helper は配列 helper の先頭を返す形に整理する。
4. Backend unit test を追加・更新する。
5. Frontend 型を更新する。
6. 機器ステータスの詳細欄に候補一覧を追加する。
7. Backend test、Frontend build、implementation review を実行する。

## 確認コマンド

```bash
cd backend && GOCACHE=$PWD/.gocache rtk go test ./...
cd frontend && rtk npm run build
rtk codex review --uncommitted
```

## 運用メモ

反映後は DELTA 3 Plus / DELTA 3 Plus 2 の `254/21/427` と `254/22/280` を数日比較し、SOC変化・充放電量・使用実態と整合するかを見る。
