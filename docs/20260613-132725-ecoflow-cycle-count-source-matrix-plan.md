# EcoFlow cycle count source matrix implementation plan

作成日時: 2026-06-13 13:27:25 JST

## Goal

DELTA 3 Plus / DELTA 3 Max Plus / DELTA 3 Plus 2 のサイクル数が画面で `-` のままになっているため、機種ごとにどの read-only source からサイクル数を取得できるかを整理した対応表を作成する。

この対応表は、次に全機種対応を実装するときの判断材料にする。推測値や未命名 protobuf field をサイクル数として採用しない。

## Non-goals

- この計画では実機 write 制御を追加しない。
- この計画では DELTA 3 Plus 系のサイクル数表示実装を追加しない。
- シリアル番号、API key、secret、メールアドレス、password をドキュメントへ記載しない。
- private MQTT の未命名 protobuf field を根拠なしに cycle と断定しない。

## Current state

直近の read-only 確認結果:

| Device | Device type | Current UI/API cycle status | Developer MQTT quota probe | Private MQTT status |
| --- | --- | --- | --- | --- |
| DELTA Pro 3 | DELTA_PRO3 | 取得済み。Developer MQTT quota 由来 | `cycles` key で取得済み | Cloud status が主 |
| DELTA 3 Plus | DELTA_3_PLUS | 未取得 | 短時間 probe では quota 未受信 | status telemetry は取得済み。ただし cycle の名前付き field は未確認 |
| DELTA 3 Max Plus | DELTA_3_MAX_PLUS | 未取得 | quota は来るが cycle key 未検出 | status telemetry は取得済み。ただし cycle の名前付き field は未確認 |
| DELTA 3 Plus 2 | DELTA_3_PLUS | 未取得 | 短時間 probe では quota 未受信 | status telemetry は取得済み。ただし cycle の名前付き field は未確認 |
| RIVER 2 | RIVER_2 | 未取得 | 電源OFF想定。quota 未受信 | status timeout |

## Data/API contracts

### Developer MQTT quota

- Source: EcoFlow official Developer MQTT certification and `/open/{certificateAccount}/{deviceSN}/quota`
- Current accepted cycle keys:
  - `cycles`
  - `bmsCycles`
  - `bmsBattCycles`
  - `bms_bmsStatus.cycles`
  - `hs_yj751_bms_slave_addr.1.cycles`
- Only named keys are acceptable.
- If quota arrives without cycle key, the compatibility matrix should record it as `quota-received-no-cycle-key`.
- If quota does not arrive within probe timeout, record it as `quota-timeout`.

### Private MQTT status

- Source: EcoFlow private MQTT protobuf status telemetry.
- Current status endpoint can expose decoded field diagnostics for DELTA 3 series.
- Unknown numeric fields must be listed as `candidate/unverified` only when there is explicit evidence.
- A field can become accepted only after one of these is true:
  - A known public mapping identifies the field as cycle count for that device type.
  - Repeated app-side observations prove the field changes consistently with EcoFlow app cycle count.
  - A named key becomes available through Developer MQTT quota.

## Files likely to change

- Add `docs/20260613-132725-ecoflow-cycle-count-source-matrix.md`
- Add `docs/20260613-132725-ecoflow-cycle-count-source-matrix.html`

No backend/frontend code change is planned for this step.

## Implementation steps

1. Create the compatibility matrix document in Markdown.
2. Include:
   - current device coverage table
   - evidence and probe result table
   - source acceptance rule
   - per-device next action
   - implementation guardrails for future code changes
3. Render the same content as standalone HTML for easy viewing.
4. Keep all identifiers generic: device names and device types are allowed; serial numbers and credentials are not.

## Safety boundaries

- Read-only only.
- No real-device write.
- No control behavior changes.
- No credentials or serial numbers in docs.
- Do not treat uncertain private MQTT fields as production cycle count.

## Review points

- The plan and matrix must separate confirmed data from unverified candidates.
- The matrix must not leak secrets or serial numbers.
- The matrix must not imply DELTA 3 Plus cycle count is implemented.
- The matrix must provide a clear next step for each unsupported device.

## Verification commands

```bash
rtk git status --short --untracked-files=all
rtk sed -n '1,220p' docs/20260613-132725-ecoflow-cycle-count-source-matrix-plan.md
rtk sed -n '1,260p' docs/20260613-132725-ecoflow-cycle-count-source-matrix.md
rtk codex review --uncommitted
```

No Go or frontend build is required because this step only adds documentation.

## Rollback notes

The change is documentation-only. Rollback is removing the added matrix Markdown/HTML files and the plan files.
