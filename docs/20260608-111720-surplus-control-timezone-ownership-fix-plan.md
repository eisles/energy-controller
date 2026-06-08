# Surplus Control Timezone Ownership Fix Plan

## Goal

Fix the DELTA Pro 3 surplus charging control so that daytime surplus export is not blocked by the night charge planner because of UTC hour interpretation.

The observed failure is:

- `/api/status` reports exporting power and a DELTA Pro 3 surplus charge candidate.
- `/api/surplus-control/commands` records `suppressedReason = "night charge plan owns control"`.
- `nightPlanOwnsEnergyControl()` checks `measuredAt.Hour()` directly. Runtime `measuredAt` is UTC, so 11:00 JST appears as 02:00 and is treated as night.

## Non-Goals

- Do not add new EcoFlow write commands.
- Do not change real-control default safety gates.
- Do not change EcoFlow credentials, serial numbers, or private MQTT behavior.
- Do not change DELTA 3 Max Plus telemetry decoding in this task.
- Do not commit unless explicitly requested.

## Current State

Relevant files:

- `backend/cmd/server/main.go`
  - `applyNightChargePlanControl()` returns whether night charging owns energy control.
  - `nightPlanOwnsEnergyControl()` currently decides ownership from `measuredAt.Hour()`.
  - `recordStatus()` skips surplus command evaluation when night plan owns control.
- `backend/cmd/server/main_test.go`
  - Existing tests cover surplus/night interaction and write-client behavior.

Current unsafe behavior:

- When `status.UpdatedAt` is UTC during daytime JST, `nightPlanOwnsEnergyControl()` can return true for `NIGHT_RECOVER`.
- That writes only a skip log and prevents `EvaluateSurplusCommandGuard()` / `ExecuteSurplusCommand()` from running.

## Desired Behavior

Night charge plan owns energy control only when one of these is true:

- Strategy state is `NIGHT_CHARGE_WINDOW`.
- Strategy state is `NIGHT_RECOVER` and the active tariff context says the current period is low-price.
- Strategy state is `NIGHT_RECOVER`, no tariff context is available, and the measured time is in the configured local timezone night window.

For daytime high-price/mid-price periods:

- `NIGHT_RECOVER` must not block surplus charging.
- Existing surplus command guard still decides whether to write.
- Existing minimum interval, duplicate command, real-control, simulation, mock, and confirmation gates remain unchanged.

## Data/API Contracts

No public API schema change is required.

Existing log behavior should improve:

- `/api/surplus-control/commands` should no longer show `night charge plan owns control` for daytime JST surplus candidates.
- If a write is still blocked, the log should show the actual surplus guard reason, such as minimum interval, duplicate command, mode guard, or higher priority device.

## Safety Boundaries

- This change only fixes ownership routing between existing planners.
- It does not bypass `ENABLE_REAL_CONTROL`, `SIMULATION_MODE`, `MOCK_MODE`, `AUTO_CONTROL_ENABLED`, `CONFIRM_ECOFLOW_WRITE`, trial window, duplicate command, or minimum interval gates.
- It does not introduce any unconditional write.
- Tests must use mock/stub write clients only.

## Implementation Steps

1. Update `nightPlanOwnsEnergyControl()` to accept tariff context and configured timezone.
2. Use tariff low-price state as the primary signal when available.
3. Fall back to configured local timezone conversion before checking the legacy `23:00-07:00` window.
4. Update `applyNightChargePlanControl()` caller to pass the needed context.
5. Add unit tests for:
   - `NIGHT_RECOVER` at 02:00 UTC / 11:00 JST with high-price tariff does not own control.
   - `NIGHT_RECOVER` during low-price tariff owns control.
   - fallback timezone conversion treats UTC daytime correctly when tariff context is absent.
   - `NIGHT_CHARGE_WINDOW` still owns control.

## Review Points

- Confirm no new write path was added.
- Confirm existing safety gates remain downstream of the ownership fix.
- Confirm tests prove the UTC/JST regression.
- Confirm no serials, keys, or tokens are added to code or docs.

## Verification Commands

- `cd backend && rtk go test ./cmd/server ./internal/control`
- `cd backend && rtk go test ./...`
- `rtk git diff --check`
- `rtk codex review --uncommitted`

## Operational Notes

After deployment, verify:

- `/api/status` shows current export/import and the surplus plan.
- `/api/surplus-control/commands?limit=10` no longer records `night charge plan owns control` during daytime JST export.
- If real-control gates are enabled in the local environment, any actual write remains subject to the existing guard chain.
