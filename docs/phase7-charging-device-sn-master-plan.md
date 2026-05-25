# Phase 7 充電機器マスタ SN 管理 実装計画

## 目的

DELTA Pro 3 と複数台の DELTA 3 Plus を、`.env` の単一シリアル番号ではなく `charging_devices` マスタで識別できるようにする。

現状の `credentialRef` は認証情報の参照名として残し、実機を特定する `deviceSn` と `deviceType` をマスタに追加する。これにより、2台目以降の DELTA 3 Plus を追加して、将来の余剰電力配分や優先順位制御に使える土台を作る。

## 非目的

- EcoFlow / SwitchBot の新しい write API は追加しない。
- 認証情報、パスワード、API token、secret key は DB に保存しない。
- 複数台 DELTA 3 Plus への同時配分制御は今回実装しない。
- SwitchBot スマートプラグ制御は今回実装しない。
- DELTA Pro 3 の既存制御経路は変更しない。

## 現状

- `charging_devices` は以下を管理している。
  - 表示名
  - 種別
  - provider
  - role
  - credentialRef
  - enabled / controlEnabled
  - 優先順位
  - 充電W範囲
  - 容量 / SOC設定
- DELTA 3 Plus の接続先はまだ `.env` の `ECOFLOW_DELTA3_DEVICE_SN` / `ECOFLOW_DELTA3_DEVICE_TYPE` に依存している。
- UI には SN を入力する場所がない。

## 追加するデータ契約

### `charging_devices`

追加カラム:

| column | type | 用途 |
| --- | --- | --- |
| `device_sn` | `TEXT NOT NULL DEFAULT ''` | 実機シリアル番号。token や password ではないが、ログには出さない。 |
| `device_type` | `TEXT NOT NULL DEFAULT ''` | EcoFlow private API の device type。例: `DELTA_3`, `DELTA_3_PLUS`。 |

制約:

- `device_sn` は空文字を許可する。
- 空でない `device_sn` は同一DB内で重複しないように unique index を追加する。
- `device_sn` は UI の編集フォームでは入力できるが、一覧表示では末尾だけ見せる。
- ログやエラー文に `device_sn` を出さない。

### API JSON

`GET/POST /api/settings/charging-devices` に以下を追加する。

```json
{
  "deviceSn": "...",
  "deviceType": "DELTA_3"
}
```

バリデーション:

- EcoFlow 機器は `deviceType` を推奨する。未入力時は kind から既定値を補う。
- `ecoflow_delta3_plus` の既定 `deviceType` は `DELTA_3` とし、実機に合わせて UI で変更可能にする。
- SN は trim し、空白や制御文字を含む値は拒否する。

## DELTA 3 Plus 接続方針

### 読み取り対象

DELTA 3 Plus の read-only status は、以下の条件を満たす最初の1台を優先度順に選ぶ。

1. `enabled = true`
2. `provider = ecoflow`
3. `kind = ecoflow_delta3_plus`
4. `device_sn != ''`
5. `supports_soc_read = true`

対象が見つからない場合は、後方互換として `.env` の `ECOFLOW_DELTA3_DEVICE_SN` を使う。これにより既存運用を急に止めない。

### 補助充電 write 対象

DELTA 3 Plus 補助充電の既存 write 経路は、以下をすべて満たす場合だけ実行候補にする。

1. 上記の読み取り対象のうち、マスタから選択された対象がある。
2. 対象機器の `controlEnabled = true`。
3. 対象機器の `supports_ac_charge_limit = true`。
4. `DELTA3_AUX_ENABLED=true`
5. 既存の write gate をすべて満たす。
   - `MOCK_MODE=false`
   - `SIMULATION_MODE=false`
   - `ENABLE_REAL_CONTROL=true`
   - `AUTO_CONTROL_ENABLED=true`
   - `ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=true`
   - real-control trial window が有効
   - `ECOFLOW_DELTA3_READ_ENABLED=true`
   - `ECOFLOW_DELTA3_EXECUTE=true`
   - `ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=true`
   - `CONFIRM_ECOFLOW_WRITE=I_UNDERSTAND`
   - 最小コマンド間隔 / 差分

`.env` fallback は read-only status の後方互換に限定する。write はマスタに `device_sn` が登録され、かつ `controlEnabled=true` の機器だけを対象にする。これにより、マスタ上で無効化した機器を `.env` の残存SNで誤って制御しない。

`controlEnabled=false` またはマスタ write 対象なしの場合は、read-only 表示は行うが write は抑制して、補助制御ログに理由を残す。

## 実装ステップ

1. DB migration
   - `charging_devices` に `device_sn` / `device_type` を追加する。
   - 空でない `device_sn` の unique index を追加する。
   - seed の DELTA 3 Plus には `.env` 移行前提として `device_sn=''`、`device_type='DELTA_3'` を入れる。

2. Domain / Repository / API
   - `domain.ChargingDevice` に `DeviceSN` / `DeviceType` を追加する。
   - Repository の SELECT / INSERT / UPDATE / scan を更新する。
   - Handler payload と validation を更新する。
   - SN はエラーログに出さない。

3. DELTA 3 Plus target resolver
   - `store` に優先度順で DELTA 3 Plus 対象を選ぶ関数を追加する。
   - `config.Config` をコピーして、選択された `DeviceSN` / `DeviceType` を上書きする helper を追加する。
   - read-only は対象なしなら既存 `.env` 設定に fallback する。write は fallback せず、マスタ対象必須にする。
   - startup 時に作った DELTA 3 reader / writer は master SN 変更を反映できないため、`/api/delta3/status` の各リクエストと control loop の各tickで、選択済み config から reader / writer を作る。
   - status reader の cache は device SN / device type 単位で分け、マスタ編集後に別SNの古い値を返さない。

4. Runtime 接続
   - `/api/delta3/status` はDBがある場合、マスタ選択結果で read-only status を取得する。
   - control loop の DELTA 3 Plus 補助制御も同じ選択結果を使う。
   - write は `controlEnabled=false` なら既存 gate 前に抑制する。

5. Frontend
   - 機器マスタ編集フォームに `deviceSn` / `deviceType` を追加する。
   - 一覧には SN 全体を出さず、末尾だけの masked 表示にする。
   - 説明文を「SNはマスタ、認証情報は `.env`」に変更する。

6. Tests
   - migration / repository tests に新カラムと unique index を追加する。
   - API handler tests に `deviceSn` / `deviceType` を追加する。
   - resolver tests を追加し、マスタ優先・`.env` fallback・controlEnabled 抑制を確認する。
   - `go test ./...` と frontend build を通す。

## 安全境界

- SN は secret ではないが識別子なので、ログには出さない。
- 認証情報は引き続き `.env` のみ。
- 新しい外部API呼び出しは追加しない。
- 既存 DELTA 3 Plus private API client の利用対象SNをマスタから選べるようにするだけ。
- write は既存 gate に `controlEnabled` を追加して安全側に倒す。

## 確認コマンド

```bash
cd backend && rtk go test ./...
cd frontend && rtk npm run build
rtk git diff --check
rtk curl -sS http://localhost:18085/api/settings/charging-devices
```

## 運用メモ

- 既存DBには migration で `device_sn` / `device_type` が追加される。
- 実機のSNは UI の機器マスタ画面で入力する。
- 入力後、DELTA 3 Plus status は優先度の最も高い有効な DELTA 3 Plus を使う。
- 2台目以降の配分制御は、今回のマスタ対応を前提に次タスクで実装する。
