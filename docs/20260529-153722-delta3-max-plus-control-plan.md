# DELTA 3 Max Plus 制御対象化 実装計画

## Goal

DELTA 3 Max Plus を EcoFlow private MQTT の補助充電機器として、既存の機器マスターと安全ゲートに従って制御対象にできるようにする。

まずは安全な第一段階として、既存の「補助充電制御は1台を選択して制御する」構造を維持し、DELTA 3 Plus だけでなく DELTA 3 Max Plus も候補に入るようにする。複数台へ同時に売電を配分する制御は今回の範囲外とする。

## Non-Goals

- 複数補助バッテリーへの同時AC充電W配分は実装しない。
- AC出力ON/OFFの実writeは実装しない。
- RIVER 2 の private MQTT デコード対応は実装しない。
- `.env` に新しい機器SNや認証情報を追加しない。機器マスターを正とする。
- 実機writeの安全ゲートを緩めない。

## Current State

- DELTA 3 Max Plus は機器マスターに登録済み。
  - `kind=ecoflow_delta3_plus`
  - `device_type=DELTA_3_MAX_PLUS`
  - `status_source=ecoflow_private_mqtt`
  - `control_enabled=0`
- 実機検証では以下が成功済み。
  - AC充電上限 `200W -> 300W -> 200W`
  - 最大充電残量 `100% -> 95% -> 100%`
  - 最低放電残量 `0% -> 5% -> 0%`
  - バックアップリザーブ `50% / OFF -> 55% / ON -> 50% / OFF`
  - gridBypassDisabled `false -> true -> false`
- `ChargingDeviceRepository.Delta3WriteTarget` は `kind='ecoflow_delta3_plus'` かつ `control_enabled=1` の最優先1台を返す。
- `Delta3StatusReader.CurrentDeviceStatuses` は private MQTT 機器を一覧取得できるが、成功応答もキャッシュされるため、実機検証直後に画面で古い値が見える場合がある。

## Data/API Contracts

### Charging Device Master

DELTA 3 Max Plus は既存の `charging_devices` を使う。

- `kind`: `ecoflow_delta3_plus`
- `device_type`: `DELTA_3_MAX_PLUS`
- `status_source`: `ecoflow_private_mqtt`
- `supports_soc_read`: `true`
- `supports_ac_charge_limit`: `true`
- `control_enabled`: UIまたはDB設定で制御対象化

### Control Target Selection

第一段階では、補助充電制御のwrite targetは1台のみとする。

- `enabled=1`
- `provider='ecoflow'`
- private MQTT対象
- `supports_soc_read=1`
- `control_enabled=1`
- `supports_ac_charge_limit=1`
- `priority ASC, id ASC` の先頭

DELTA 3 Plus 2 と DELTA 3 Max Plus の両方を `control_enabled=1` にした場合、優先順位が小さい方だけが補助充電制御の対象になる。

## Safety Boundaries

- 実機writeは既存どおり以下を満たす場合だけ行う。
  - `ENABLE_REAL_CONTROL=true`
  - `SIMULATION_MODE=false`
  - `MOCK_MODE=false`
  - `AUTO_CONTROL_ENABLED=true`
  - 既存の auto-control/private MQTT write overlap 許可条件を満たす
  - `CONFIRM_ECOFLOW_WRITE` が既存の確認値
  - private MQTT write許可が有効
- コマンド間隔、差分しきい値、ヒステリシスは既存の `Delta3AuxSettings` を使う。
- DELTA 3 Max Plus のAC充電下限は `200W` として扱う。
- 画面やログでは「DELTA 3 Plus」固定表記を避け、選択された補助充電機器名を表示できるようにする。

## Files Likely to Change

- `backend/internal/store/charging_device_repository.go`
  - private MQTT補助機器の対象判定を `ecoflow_delta3_plus` 固定から汎用化する。
- `backend/internal/store/charging_device_repository_test.go`
  - DELTA 3 Max Plus がread/write target候補になるテストを追加する。
- `backend/internal/api/delta3_status_handler.go`
  - private MQTT対象判定とキャッシュTTLの扱いを見直す。
- `backend/internal/api/delta3_status_handler_test.go`
  - DELTA 3 Max Plus の一覧取得・キャッシュ動作を確認する。
- `backend/cmd/server/main.go`
  - 補助充電計画に対象機器名を入れられるようにする。
- `backend/internal/domain/status.go`
  - `Delta3AuxPlan` に device id/name/type を追加する。
- `backend/internal/control/auxiliary_battery_planner.go`
  - 文言を機器固定から補助充電機器へ寄せる。制御ロジック自体は既存を維持する。
- `frontend/components/StatusCards.tsx`
  - 補助計画表示に対象機器名を表示する。
- `frontend/lib/types.ts`
  - `Delta3AuxPlan` の型追加。

## Implementation Steps

1. 対象判定の共通化
   - `isEcoFlowPrivateMQTTAuxiliaryDevice` のようなヘルパーを repository/api で使える形にする、または同等の条件を各層で揃える。
   - 対象は `kind='ecoflow_delta3_plus'` かつ `status_source='ecoflow_private_mqtt'` を維持する。
   - `device_type=DELTA_3_MAX_PLUS` を明示的に許可する。

2. Write target selectionの確認
   - DELTA 3 Max Plus が `control_enabled=1` なら `Delta3WriteTarget` に選ばれることをテストする。
   - 複数候補がある場合は `priority ASC, id ASC` で1台だけ選ばれることをテストする。

3. 補助計画へ対象機器情報を追加
   - `Delta3AuxPlan` に `deviceId`, `deviceName`, `deviceType` を追加する。
   - `applyDelta3AuxControl` で実際に選ばれた write target を設定する。
   - 対象がない場合は未設定のままとする。

4. 固定表示の修正
   - 画面の補助計画に対象機器名を表示する。
   - 「DELTA 3 Plus」固定の説明文は、必要な箇所だけ「補助充電機器」に寄せる。

5. キャッシュ見直し
   - 実機検証や画面確認で古い値が目立つため、private MQTTの成功キャッシュTTLを短縮する。
   - まずは実装影響を限定し、成功TTLを現在より短くするだけに留める。
   - read失敗時のbusy backoffは維持する。

6. ローカルDB設定
   - コード実装後、必要なら DELTA 3 Max Plus の `control_enabled` と `priority` を運用設定として更新する。
   - これはコードコミット対象ではなく、運用DB変更として扱う。

## Review Points

- DELTA 3 Max Plus を許可しても RIVER 2 がwrite targetにならないこと。
- `control_enabled=false` の機器にwriteしないこと。
- 実機writeガードが変わっていないこと。
- 既存のDELTA 3 Plus 2制御が壊れないこと。
- UIで対象機器が分かること。
- キャッシュ短縮がEcoFlow private APIの過剰アクセスにつながらないこと。

## Verification Commands

- `cd backend && rtk go test ./...`
- `cd frontend && rtk npm run build`
- `docker compose up -d --build`
- `curl -fsS http://localhost:8080/api/devices/statuses`
- `curl -fsS http://localhost:8080/api/status`
- `docker compose down`

ローカル実機確認環境では `HTTP_PORT=18085` で起動しているため、同じAPIを `http://localhost:18085/...` でも確認する。

## Operational Notes

- 実装直後は DELTA 3 Max Plus を `control_enabled=false` のままにし、read-only表示と選択ロジックのテストを先に確認する。
- 制御対象化する場合は、DELTA 3 Plus 2 と DELTA 3 Max Plus の `priority` を明示的に決める。
- 複数台同時配分は次フェーズで実装する。
