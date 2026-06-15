# 機器別ステータス取得タイムアウト対策 実装計画

## Goal

`GET /api/devices/statuses` が EcoFlow private MQTT の応答待ちで 20 秒以上詰まり、充電機器ステータス画面が `unavailable` になる問題を改善する。

推奨方針:

- 機器ごとの status 読み取りを短い timeout で区切る。
- 1 台の private MQTT 応答遅延で `/api/devices/statuses` 全体を止めない。
- 直近の成功値がある場合は、更新失敗時でも cached status として返す。
- 失敗した機器だけ `lastError` に理由を出す。

## Non-goals

- 実機 write、充電/放電判断、料金最適化判断は変更しない。
- EcoFlow private MQTT の protocol や command payload は変更しない。
- device master、シリアル番号、認証情報、`.env` は変更しない。
- `/api/status` の制御判断 API path は変更しない。

## Current State

- `/api/status` は正常に 200 を返している。
- `/api/devices/statuses` は live 確認で 20 秒 timeout した。
- backend log には `failed to read DELTA_3 status`、`EcoFlow MQTT request timed out waiting for reply`、`context canceled` が出ている。
- `Delta3StatusReader.CurrentDeviceStatuses` は機器を順番に読み取る。
- `currentStatusForConfig` は cache mutex を保持したまま `readDelta3Status` を呼ぶため、遅い network read が他の status read も巻き込みやすい。

## Files Likely To Change

- `backend/internal/api/ecoflow_device_status_handler.go`
- `backend/internal/api/ecoflow_device_status_handler_test.go`
- `frontend/lib/types.ts`
- `frontend/components/StatusCards.tsx`

## Data/API Contract

- `GET /api/devices/statuses` の response shape は変えない。
- 既存の `Delta3StatusResponse.cached` を、直近値または cache hit 表示に使う。
- 更新失敗時に直近成功値を返す場合、`available=true` のまま `cached=true` にする。
- stale fallback の場合は `lastError` に更新失敗理由を保持し、画面で「キャッシュ」扱いを表示する。
- 直近値がない機器は従来どおり `available=false` と `lastError` を返す。

## Safety Boundaries

- read-only status API の応答性改善だけを行う。
- EcoFlow write command path は触らない。
- real control gate、minimum command interval、hysteresis は変更しない。
- 追加ログに secret、token、device SN は出さない。

## Implementation Steps

1. `CurrentDeviceStatuses` を機器ごとに並列取得する。
   - response order は入力 devices の順序を維持する。
   - 各 goroutine は機器単位の context timeout を持つ。
2. 機器別 timeout を helper で決める。
   - 既定は短めの 3 秒程度。
   - `cfg.Delta3Timeout` がそれより短い場合は既存設定を尊重する。
   - Pro 3 の cloud status と Developer MQTT cycle 補完も、同じ機器単位 context 内で完了させる。
3. `currentStatusForConfig` の mutex 範囲を cache / client 取得と cache 更新だけに縮める。
   - network probe 中は mutex を保持しない。
4. cache が期限切れでも、更新が timeout / cancellation / transient error で失敗した場合は、直近成功 response があれば cached として返す。
   - 直近値がない場合は従来どおり unavailable。
5. frontend の `Delta3Status` 型に `cached` を追加し、機器ステータス一覧で cached status を「キャッシュ」として表示する。
   - `available=true` だけで「接続中」と誤認しないようにする。
   - cached status に `lastError` がある場合は detail に更新失敗理由を出す。
6. unit test を追加する。
   - 遅い private MQTT 機器があっても `CurrentDeviceStatuses` が短時間で返る。
   - timeout 後に直近成功値を cached として返す。
   - response order が維持される。

## Review Points

- network read 中に `Delta3StatusReader.mu` を保持しないこと。
- stale cache fallback が control write 判断へ流れないこと。
- stale cache fallback が UI で live status に見えないこと。
- `/api/status` の既存 cache 挙動を壊さないこと。
- context timeout により goroutine leak が起きないこと。

## Verification Commands

- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./internal/api`
- `cd backend && GOCACHE=$PWD/.gocache rtk go test ./...`
- `cd frontend && rtk npm run build`
- `rtk codex review --uncommitted`

## Runtime Verification

実装後、必要に応じて以下を確認する。

- `docker compose up -d --build`
- `curl -s -i --max-time 10 http://127.0.0.1:18085/api/status`
- `curl -s -i --max-time 10 http://127.0.0.1:18085/api/devices/statuses`
- 画面の充電機器ステータスで、取得できた機器が表示され、失敗機器だけ `取得不可` になること。

## Rollback

問題があれば、`CurrentDeviceStatuses` の並列化と stale cache fallback を戻す。API response shape と DB schema は変えないため、rollback は backend code のみで完結する。
