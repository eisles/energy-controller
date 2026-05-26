# Phase 7 充電機器優先順位制御 実装計画

## 目的

充電機器マスタの `priority` を制御順の正とし、売電吸収時に優先順位が高い機器を先に充電候補として扱う。

現在は DELTA Pro 3 の余剰追従を先に評価し、その後 DELTA 3 Plus 補助充電を評価している。これを、マスタ上で DELTA 3 Plus の `priority` が DELTA Pro 3 より小さい場合は、DELTA Pro 3 の送信候補を抑制し、DELTA 3 Plus 側が Pro 3 待ちをしないようにする。

## 非目的

- 新しい実機 API 操作を追加しない。
- EcoFlow の安全ゲートを弱めない。
- 複数 DELTA 3 Plus を同時に配分制御する実装は今回含めない。
- 売電量を全機器へ最適配分する本格 allocator は今回含めない。

## 現状

- 機器マスタには `priority`、`controlEnabled`、`supportsAcChargeLimit` がある。
- DELTA Pro 3 write target は `EcoFlowCloudWriteTarget` で取得され、`priority ASC, id ASC` で選ばれる。
- DELTA 3 Plus write target は `Delta3WriteTarget` で取得され、同じく `priority ASC, id ASC` で選ばれる。
- DELTA 3 Plus 補助制御は、DELTA Pro 3 にまだ充電候補がある場合 `WAIT_PRO3` で待つ。

## 変更方針

1. サーバー側で DELTA Pro 3 と DELTA 3 Plus の write target を取得し、`priority` を比較する。
2. DELTA 3 Plus の `priority` が DELTA Pro 3 より小さい場合だけ、DELTA 3 Plus を高優先とみなす。
3. ただし DELTA 3 Plus 側が実際に制御可能な状態でない場合は、DELTA Pro 3 を抑制しない。
   - `DELTA3_AUX_ENABLED=true`
   - `ECOFLOW_DELTA3_READ_ENABLED=true`
   - 機器マスタで DELTA 3 Plus write target が存在する
   - DELTA 3 Plus の状態が取得でき、満充電付近ではなく、AC 充電上限の変更候補がある
   - DELTA 3 Plus private API 用の追加安全 gate が有効
   - DELTA 3 Plus の command guard で重複候補、最小送信間隔、直前エラーバックオフに該当しない
4. DELTA 3 Plus が高優先かつ制御可能な場合:
   - DELTA Pro 3 の余剰追従 command guard は、DELTA 3 Plus に実際に送信可能な変更候補がある時だけ「高優先機器が先」として送信を抑制する。
   - DELTA 3 Plus planner は Pro 3 待ちをスキップする。
5. 同順位の場合は現状維持として DELTA Pro 3 優先にする。

## 変更予定ファイル

- `backend/cmd/server/main.go`
  - 優先順位解決 helper を追加。
  - DELTA Pro 3 command guard に高優先機器名を渡す。
  - DELTA 3 Plus planner に Pro 3 待ちスキップを渡す。
- `backend/internal/control/surplus_executor.go`
  - 高優先機器がある場合の suppression reason を追加。
- `backend/internal/control/delta3_aux_planner.go`
  - `IgnorePro3Wait` を追加し、優先機器側が Pro 3 待ちしないようにする。
- `backend/internal/control/*_test.go`
  - 優先順位による Pro 3 抑制、DELTA 3 Plus の Pro 3 待ちスキップを検証する。
- `frontend/lib/display-labels.ts`
  - 新しい suppression reason の日本語表示を追加する。

## 安全境界

- 実機 write は既存条件を維持する。
  - `ENABLE_REAL_CONTROL=true`
  - `SIMULATION_MODE=false`
  - `AUTO_CONTROL_ENABLED=true`
  - `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`
  - 実制御期限内
  - `ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=true`
  - `ECOFLOW_DELTA3_EXECUTE=true`
  - `ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=true`
- DELTA 3 Plus は、補助制御と読取が有効な場合だけ Pro 3 より優先できる。
- `DELTA3_AUX_ENABLED=false` や `ECOFLOW_DELTA3_READ_ENABLED=false` の状態で Pro 3 を止めない。
- DELTA 3 Plus が満充電付近、状態未取得、差分不足などで充電候補を持たない場合も Pro 3 を止めない。
- DELTA 3 Plus 側が最小送信間隔や直前エラーバックオフで送信できない場合も Pro 3 を止めない。

## 検証

- `cd backend && rtk go test ./...`
- `cd frontend && rtk npm run build`
- `rtk codex review --uncommitted`
- 必要に応じて backend binary を再ビルドして `localhost:18085` の `/api/status` と制御ログを確認する。

## 運用メモ

- `priority` は小さい数値ほど高優先。
- 現在のマスタで DELTA Pro 3 の優先順位が DELTA 3 Plus より高ければ、従来通り Pro 3 優先で動く。
- DELTA 3 Plus を先に使いたい場合は、機器マスタで DELTA 3 Plus の `priority` を DELTA Pro 3 より小さくする。
