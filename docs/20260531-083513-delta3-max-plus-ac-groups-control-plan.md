# DELTA 3 Max Plus AC Group Control Plan

## Goal

DELTA 3 Max Plus の EcoFlow private MQTT ステータスを、実機調査で確認した AC1 / AC2 / AC出力保護チャンネルに対応させる。

主目的は、AC2 だけ ON のときに既存ロジックが `AC出力OFF` と誤判定する問題を直し、制御判断・画面・ログで AC1/AC2 を区別できるようにすること。

## Non-goals

- AC1 / AC2 の ON/OFF write command は今回実装しない。
- AC出力保護チャンネルの write command は今回実装しない。
- `ENABLE_REAL_CONTROL=true` の実機書き込み条件や既存の最小コマンド間隔、安全ゲートは変更しない。
- 料金最適化や深夜充電配分の制御方針はこの作業では変更しない。

## Current State

調査結果:

- `cmdFunc=254 cmdId=21 field=367`
  - `4`: AC1 OFF
  - `14`: AC1 ON
- `cmdFunc=254 cmdId=21 field=971`
  - `4`: AC2 OFF
  - `14`: AC2 ON
- `cmdFunc=254 cmdId=21 field=1539`
  - `1`: AC出力保護 = AC1
  - `2`: AC出力保護 = AC2
- `cmdFunc=254 cmdId=21 field=47`
  - `12`: AC1/AC2 とも OFF
  - `14`: AC1/AC2 のどちらか、または両方が ON

既存実装は `field=367` を `ACOutputEnabled` に割り当てているため、AC2 だけ ON の場合に `ACOutputEnabled=false` となる。

## Files Likely To Change

- `backend/internal/ecoflowprivate/status.go`
- `backend/internal/ecoflowprivate/codec.go`
- `backend/internal/ecoflowprivate/codec_test.go`
- `backend/internal/api/delta3_status_handler.go`
- `backend/internal/api/delta3_status_handler_test.go`
- `backend/cmd/server/main.go`
- `frontend/lib/types.ts`
- `frontend/components/StatusCards.tsx`

既存の read-only raw capture 追加差分は残し、今回の実装で必要なテスト補強に利用する。

## Data / API Contract

`ecoflowprivate.Status` に次の任意項目を追加する。

- `ACOutput1Enabled *bool`
- `ACOutput2Enabled *bool`
- `ACOutputProtectionChannel *int`

API response も同じ JSON 名で公開する。

- `acOutput1Enabled`
- `acOutput2Enabled`
- `acOutputProtectionChannel`

既存の `acOutputEnabled` は後方互換のため維持する。DELTA 3 Max Plus で AC1/AC2 のどちらかが取れている場合は、`acOutputEnabled = AC1 || AC2` とする。

## Safety Boundaries

- 今回は read/status と制御判断の誤判定修正が対象。
- 実機 write path は増やさない。
- 未確定の write payload を推測で送信しない。
- 既存の `ENABLE_REAL_CONTROL=true` かつ `SIMULATION_MODE=false` のゲートは維持する。
- 実機 write が必要な将来対応では、別途 raw command capture と unit test を先に追加する。

## Implementation Steps

1. `ecoflowprivate.Status` に AC1/AC2/保護チャンネル項目を追加し、`merge` で保持する。
2. `decodeDisplayUpload` で `field=367` を AC1、`field=971` を AC2、`field=1539` を保護チャンネルとして decode する。
3. decode 後に `ACOutputEnabled` を `ACOutput1Enabled || ACOutput2Enabled` へ正規化する。ただし個別フィールドが取れていない機種では既存値を維持する。
4. API response と device status response に新項目を通す。
5. 制御判断で参照される `ACOutputEnabled` が AC2-only でも true になることをテストで固定する。
6. 画面に DELTA 3 Max Plus の `AC1`、`AC2`、`AC出力保護` を表示する。
7. 既存の単一 AC出力表示は残し、全体状態として `AC1 または AC2` を示す。

## Review Points

- AC2-only の誤判定が直っているか。
- DELTA 3 Plus / DELTA 3 / DELTA Pro 3 の既存単一 AC 出力表示を壊していないか。
- `field=1539` を ON/OFF と混同していないか。
- write command を増やしていないか。
- secrets や device serial number を計画・テスト fixture に入れていないか。

## Verification

- `cd backend && rtk go test ./internal/ecoflowprivate ./internal/api ./internal/control`
- `cd backend && rtk go test ./cmd/server ./cmd/ecoflow-delta3-probe`
- `cd backend && rtk go test ./...`
- `cd frontend && rtk npm run build`
- `rtk git diff --check`

## Rollback / Operational Notes

- この変更はステータス decode と表示/API 契約の追加であり、実機 write は増えない。
- 問題が出た場合は、新規 AC1/AC2/保護チャンネル項目の表示だけを隠しても既存制御は継続できる。
- AC2-only 誤判定を避けるため、`field=971` の decode は残す方が安全。
