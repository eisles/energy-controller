# Phase 7 DELTA Pro 3 機器マスター制御 実装計画

## 目的

DELTA Pro 3 の読み取り対象と実制御対象を、`.env` の固定 `ECOFLOW_DEVICE_SN` ではなく充電機器マスターから解決できるようにする。これにより、画面で管理している DELTA Pro 3 の `deviceSn`、有効/無効、制御可否が実際の EcoFlow Cloud 読み取り・書き込み経路にも反映される。

あわせて、画面上部のカードが DELTA Pro 3 1 台に固定された見え方になっている点を解消する。上部は「家全体・制御全体」のサマリーとして表示し、DELTA Pro 3 / DELTA 3 Plus など機器ごとの SOC、入出力、充電上限、接続状態は機器マスター由来の機器別ブロックに表示する。

## 非目的

- 新しい EcoFlow 書き込みコマンドは追加しない。
- `ENABLE_REAL_CONTROL=true`、`SIMULATION_MODE=false`、`AUTO_CONTROL_ENABLED=true`、確認フラグ、最小実行間隔など既存の安全ゲートは緩めない。
- DELTA 3 Plus の private MQTT 認証方式や補助バッテリー制御の判断ロジックは変更しない。
- 機器マスターの CRUD 仕様やデータ移行方針は変更しない。
- API キー、シークレット、デバイス SN などの秘密値をコードやドキュメントに固定しない。

## 現状

- DELTA 3 Plus は充電機器マスターから `Delta3ReadTarget` / `Delta3WriteTarget` を解決している。
- DELTA Pro 3 は `backend/cmd/server/main.go` で `cfg.EcoFlowDeviceSN` を直接使って `ecoflow.SignedClient` / `ecoflow.SignedWriteClient` を作成している。
- 充電機器マスターには `kind=ecoflow_delta_pro3`、`statusSource=ecoflow_cloud`、`deviceSn`、`controlEnabled`、`supportsAcChargeLimit` を保存できる。
- `/api/devices/statuses` は機器一覧を返すが、実ステータス取得は DELTA 3 Plus private MQTT に偏っており、DELTA Pro 3 は機器別ステータスとして十分に表示できていない。
- フロントエンドの上部メトリクスにはバッテリー残量や実質充電が出ており、機器が増えた場合に DELTA Pro 3 固定の情報と全体情報が混在して見える。

## データ/API 契約

DELTA Pro 3 のマスター行は次の条件で EcoFlow Cloud 対象として扱う。

- `provider = "ecoflow"`
- `kind = "ecoflow_delta_pro3"`
- `statusSource = "ecoflow_cloud"`
- `enabled = true`
- `deviceSn` は trim 後に空でない
- 読み取り対象: `supportsSocRead = true`
- 書き込み対象: 上記に加えて `controlEnabled = true` かつ `supportsAcChargeLimit = true`
- 制御ループの読み取り対象は、書き込み対象が存在する場合は必ず同じマスター行に合わせる。書き込み対象がない場合のみ read-only の最優先行を使う。

`.env` の `ECOFLOW_ACCESS_KEY`、`ECOFLOW_SECRET_KEY`、`ECOFLOW_BASE_URL` は EcoFlow Cloud 認証情報として継続利用する。`ECOFLOW_DEVICE_SN` は既存環境との互換用フォールバックとして残すが、サーバーの通常経路は機器マスターを優先する。

`/api/devices/statuses` は全充電機器を返し、取得方式ごとに次のように `status` を埋める。

- DELTA Pro 3: `statusSource=ecoflow_cloud` の場合、EcoFlow Cloud API から機器マスターの `deviceSn` で read-only 状態を取得する。
- DELTA 3 Plus: `statusSource=ecoflow_private_mqtt` の場合、既存の private MQTT 経路で read-only 状態を取得する。
- その他/未対応: 機器情報は返し、`status.available=false` と理由を返す。

フロントエンドは `/api/status` を全体サマリー、`/api/devices/statuses` を機器別ステータスとして扱う。

## 安全境界

- 実機書き込みは既存の `ecoflow.WriteGuards` を必ず通す。
- 機器マスターで `controlEnabled=false` の DELTA Pro 3 は書き込み対象にしない。
- `deviceSn` が空白だけのマスター行は対象にせず、`.env` の SN にフォールバックして書き込まない。
- 複数の DELTA Pro 3 行がある場合、制御判断に使う読み取り値とコマンド送信先を別々の機器にしない。
- 機器マスターの対象が見つからない場合、書き込みは安全側に倒して実行しない。
- 読み取りは運用互換のため、機器マスターに対象がない場合のみ `ECOFLOW_DEVICE_SN` へフォールバックする。
- フォールバックや対象なしはログ/エラー文で理由が分かるようにするが、SN はログに出さない。
- 機器別ステータスは read-only 表示に限り、画面から新しい実機制御ボタンは追加しない。

## 実装手順

1. `ChargingDeviceRepository` に DELTA Pro 3 用の読み取り/書き込みターゲット解決メソッドを追加する。
   - `EcoFlowCloudReadTarget(ctx)`
   - `EcoFlowCloudWriteTarget(ctx)`
   - `EcoFlowCloudReadTarget(ctx)` は write target が存在する場合に同じ行を返し、read/write の対象ずれを防ぐ。
2. リポジトリテストを追加し、読み取りは `controlEnabled=false` でも選ばれ、書き込みは `controlEnabled=true` の行だけ選ばれることを確認する。
3. `backend/cmd/server/main.go` に DELTA Pro 3 用のマスター解決ラッパーを追加する。
   - 読み取りは機器マスター優先、なければ `.env` SN へフォールバック。
   - 書き込みは機器マスターの `controlEnabled=true` 対象のみ許可。
4. サーバー側の DELTA Pro 3 書き込みラッパーの unit test を追加する。
   - `controlEnabled=true` のマスター対象がない場合、`.env` の `ECOFLOW_DEVICE_SN` へフォールバックせずエラーにする。
   - `controlEnabled=true` の対象がある場合だけ、その `deviceSn` で署名付き writer を組み立てる。
   - 既存の `ecoflow.WriteGuards` が適用されることを確認する。
5. `newStatusProvider` と `newSurplusWriteClient` の DELTA Pro 3 クライアント生成をマスター解決経由へ差し替える。
6. `/api/devices/statuses` で DELTA Pro 3 の read-only ステータスも返せるようにする。
7. フロントエンドの上部カードを全体サマリーに整理し、機器別ステータスカードを DELTA Pro 3 / DELTA 3 Plus を含む汎用表示へ変更する。
   - 全体サマリー: Grid、買電、売電、制御推奨、状態、更新時刻など家全体/制御全体の情報。
   - 機器別カード: 機器名、種別、接続状態、SOC、AC入力/出力、AC充電上限、容量、制御候補、取得方式。
8. README / `.env.example` の説明を、DELTA Pro 3 の SN は機器マスター管理が主であることに更新する。

## レビューポイント

- 書き込みの安全ゲートが削除・緩和されていないこと。
- `controlEnabled=false` の DELTA Pro 3 へ実機書き込みしないこと。
- DELTA Pro 3 の write target が存在する場合、制御ループの read target が同じ行に揃うこと。
- DELTA Pro 3 の server-side write wrapper に unit test があり、`.env` SN への write fallback がないこと。
- DELTA 3 Plus の private MQTT 経路を壊していないこと。
- `/api/status` と `/api/devices/statuses` の役割が分離されていること。
- 上部カードが DELTA Pro 3 固定の情報に見えないこと。
- `.env` から秘密情報や SN をコードに固定していないこと。
- 既存の mock/simulation default が維持されていること。

## 確認コマンド

```sh
cd backend && rtk go test ./...
```

フロントエンド変更を含むため:

```sh
cd frontend && rtk npm run build
```

必要に応じて API 確認:

```sh
curl -s http://localhost:18085/api/settings/charging-devices | jq .
curl -s http://localhost:18085/api/devices/statuses | jq .
curl -s http://localhost:18085/api/status | jq .
```

## 運用メモ

- 既存 DB では DELTA Pro 3 の `controlEnabled` が無効の可能性がある。実制御する場合は、画面の機器マスターで DELTA Pro 3 の SN と制御有効を確認してから起動する。
- `ECOFLOW_DEVICE_SN` は移行期間の読み取りフォールバックとして残るが、今後の設定の正は機器マスターとする。
