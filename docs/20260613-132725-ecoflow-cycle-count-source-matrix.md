# EcoFlow cycle count source matrix

作成日時: 2026-06-13 13:27:25 JST
更新日時: 2026-06-13 21:36 JST

## Summary

EcoFlow 各機器のサイクル数取得元を、現時点の read-only API / Developer MQTT quota / private MQTT status の観測結果に基づいて整理する。

結論として、現時点で実装済み・表示可能なのは DELTA Pro 3 の Developer MQTT quota `cycles` key のみ。DELTA 3 Plus 系は private MQTT status は取得できているが、サイクル数として採用できる名前付き key または検証済み field はまだない。

## Coverage matrix

| Device | Device type | Current UI/API cycle count | Developer MQTT quota | Private MQTT status | Current decision |
| --- | --- | --- | --- | --- | --- |
| DELTA Pro 3 | DELTA_PRO3 | 取得済み | 2026-06-13 live watch: `cycles` key confirmed, values observed at 54/60/65 across repeated BMS packets | Cloud status primary. Private status not needed for cycle count | Supported |
| DELTA 3 Plus | DELTA_3_PLUS | 未取得 | 2026-06-13 live watch 60s: no quota messages | Status telemetry available. No verified cycle field | Unsupported until named key or verified field is found |
| DELTA 3 Max Plus | DELTA_3_MAX_PLUS | 未取得 | 2026-06-13 live watch: quota received, only `powGetAcIn`, `powGetAcOutList`, `powInSumW`, `powOutSumW`; no cycle/BMS/SOH diagnostic key | Status telemetry available. No verified cycle field | Unsupported until cycle key/field is identified |
| DELTA 3 Plus 2 | DELTA_3_PLUS | 未取得 | 2026-06-13 live watch 60s: no quota messages | Status telemetry available. No verified cycle field | Unsupported until named key or verified field is found |
| RIVER 2 | RIVER_2 | 未取得 | 2026-06-13 live watch 60s: no quota messages. Device is expected to be powered off | Status timeout | Unsupported until device is online and probe succeeds |

## Evidence table

| Check | Result | Interpretation |
| --- | --- | --- |
| `/api/devices/statuses` | DELTA Pro 3 returns `cycleCount` with `ecoflow_developer_mqtt_quota` | Current display path works for Pro 3 |
| Developer MQTT quota probe for DELTA Pro 3 | `cycles` key returned | Safe to keep `cycles` as accepted named key |
| 2026-06-13 live Developer MQTT quota watch for DELTA Pro 3 | `cycles` key returned repeatedly. Observed values included 54/60/65 from quota packets | `cycles` remains the only accepted cycle source |
| 2026-06-13 live Developer MQTT quota watch for DELTA 3 Plus | 60s watch completed with 0 messages | No Developer MQTT cycle source confirmed |
| 2026-06-13 live Developer MQTT quota watch for DELTA 3 Max Plus | 60s watch received quota messages, but `cycleCandidates` and `diagnosticKeys` were empty. A 25s key-name-only check saw only `powGetAcIn`, `powGetAcOutList`, `powInSumW`, `powOutSumW` | Current Developer MQTT quota stream does not expose a cycle key for this device under the observed conditions |
| 2026-06-13 live Developer MQTT quota watch for DELTA 3 Plus 2 | 60s watch completed with 0 messages | No Developer MQTT cycle source confirmed |
| 2026-06-13 live Developer MQTT quota watch for RIVER 2 | 60s watch completed with 0 messages | Expected while device is powered off |
| Private MQTT telemetry diagnostics for DELTA 3 series | decoded field summaries exist | Useful for investigation, but not enough to accept cycle count |

## Accepted source rules

### Accepted

- Developer MQTT quota key is named and semantically clear:
  - `cycles`
  - `bmsCycles`
  - `bmsBattCycles`
  - `bms_bmsStatus.cycles`
  - `hs_yj751_bms_slave_addr.1.cycles`
- Cloud REST quota exposes a named cycle key.

### Not accepted

- Unknown private MQTT protobuf field numbers.
- Values that merely look plausible, such as small integers near expected cycle counts.
- Values from an offline or timed-out device.
- Any value requiring real-device write to verify.

## Per-device next actions

| Device type | Next action | Acceptance condition |
| --- | --- | --- |
| DELTA_3_PLUS | Run a longer Developer MQTT quota watch while device is online and active | A named accepted cycle key appears in quota payload |
| DELTA_3_MAX_PLUS | Capture quota key names over a longer window and compare against app-visible data if available | A named cycle key appears, or a private field is validated by repeated observations |
| RIVER_2 | Power on device and run the same quota watch | Quota arrives and named cycle key is present |
| DELTA_PRO3 | Keep current implementation | Continue using Developer MQTT quota `cycles` key |

## Future implementation guardrails

- Keep all EcoFlow external details inside adapter packages.
- Do not expose raw EcoFlow response structures to the domain layer.
- Add device-type-specific mapping only after the matrix marks the source as accepted.
- Add unit tests for each accepted key or field.
- Preserve read-only behavior for status acquisition.
- Do not add real-device write for cycle count discovery.
- Do not store or document serial numbers, access keys, secret keys, private app credentials, or tokens.

## Suggested probe workflow

Use the read-only probe and do not print serial numbers:

```bash
cd backend
go run ./cmd/ecoflow-developer-mqtt-probe --sn <DEVICE_SN> --timeout-sec 60
go run ./cmd/ecoflow-developer-mqtt-probe --sn <DEVICE_SN> --watch-sec 300
```

The watch output includes investigation-only `diagnosticKeys` for quota keys containing `bms`, `cyc`, `cycle`, or `soh`. Sensitive-looking paths such as serial numbers, tokens, accounts, emails, and secrets are filtered out. Non-numeric string values are also omitted because path names alone cannot prove they are safe to print. These values are not accepted as `cycleCount` until this matrix marks a source as confirmed.

Record only:

- device display name
- device type
- whether quota arrived
- whether an accepted cycle key appeared
- accepted key name
- relevant diagnostic key names and numeric/bool scalar values
- whether the value was displayed by `/api/devices/statuses`

## Implementation readiness

| Device type | Ready for code implementation? | Reason |
| --- | --- | --- |
| DELTA_PRO3 | Yes | Named `cycles` key confirmed and already implemented |
| DELTA_3_PLUS | No | No named cycle key confirmed |
| DELTA_3_MAX_PLUS | No | Quota was observed but no accepted cycle key confirmed |
| RIVER_2 | No | Device offline during probe |

## Operational note

The DELTA 3 series may still expose the cycle count through private MQTT protobuf fields, but those fields must remain investigation-only until a reliable mapping is established. The current table intentionally prefers missing data over a plausible but unverified number.
