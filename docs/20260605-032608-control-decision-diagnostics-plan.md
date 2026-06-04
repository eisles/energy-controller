# Control Decision Diagnostics Implementation Plan

## Goal

Add a read-only control diagnostics view so the dashboard can answer:

- Is live data acquisition healthy right now?
- If the home is importing or exporting, which battery should charge, discharge, hold, or be skipped?
- Why is a charge/discharge write not being attempted?

The feature should make the current control state easier to inspect before adjusting real-device control.

## Non-Goals

- Do not add new EcoFlow write commands.
- Do not loosen existing real-control gates, command intervals, retry suppression, or safety thresholds.
- Do not expose API tokens, private MQTT credentials, device serial numbers beyond the existing device status surface, or confirmation strings.
- Do not redesign the whole dashboard layout.

## Current State

- `GET /api/status` returns grid import/export, DELTA Pro 3 status, surplus plan, night charge plan, DELTA 3 auxiliary plan, tariff context, and control write readiness.
- `GET /api/devices/statuses` returns the configured charging-device master rows with read-only device telemetry where supported.
- The dashboard already polls `/api/status` every 10 seconds and `/api/devices/statuses` every 30 seconds.
- Existing UI can show raw status and logs, but the user still has to infer why a device is charging, discharging, pass-through, waiting, or unavailable.

## Data/API Contract

Extend `domain.Status` and `/api/status` with a new optional `controlDiagnostics` object:

- `gridState`: `importing`, `exporting`, or `neutral`
- `dataFreshness`: status updated time, age seconds, whether the latest status is stale, and whether `lastError` exists
- `writeReadiness`: ready flag and first sanitized blocker copied from existing readiness
- `pro3`: DELTA Pro 3 action, reason, SOC, AC input/output, target charge W, and whether a write candidate exists
- `auxiliary`: DELTA 3 auxiliary action, reason, device name/type, SOC, AC input/output, recommended AC charge limit, recommended backup reserve, and whether a write candidate exists
- `summary`: short Japanese-neutral internal string for display fallback

The diagnostics are derived from existing domain status values only. They do not call external APIs and do not persist new data.

## Frontend Contract

Add a compact "制御診断" section near the top of the dashboard using existing data from `/api/status`:

- Overall grid state and data freshness
- Write readiness state
- DELTA Pro 3 action and reason
- Auxiliary battery action and reason
- Warnings when data is stale, status has an error, or write readiness is blocked

Use existing label helpers for known reason translation and add only small additional labels for diagnostic actions.

## Safety Boundaries

- Read-only only: no write APIs, no new command send paths, no real-device state changes.
- Keep existing `ControlWriteReadiness` as the source of gate state.
- Do not include secret-bearing config values in the diagnostics payload.
- Treat missing telemetry as `unavailable` and show it as a blocker instead of guessing.
- If grid/Nature data is stale or errored, diagnostics must say the control basis is uncertain.

## Implementation Steps

1. Add `ControlDiagnostics` domain structs to `backend/internal/domain/status.go`.
2. Build diagnostics in `backend/internal/api/status_handler.go` after real-control readiness is calculated.
3. Add unit tests for importing, exporting, stale/error status, and sanitized readiness blocker behavior.
4. Extend frontend TypeScript types.
5. Add display-label helpers for diagnostic actions/grid states.
6. Add a compact `ControlDiagnosticsCard` in the dashboard, placed after the top status cards and before device status.
7. Add minimal CSS for the compact diagnostic rows.

## Review Points

- Diagnostics must not leak secrets.
- Diagnostics must not imply a write was sent; use "候補" and "理由" wording.
- Stale/error handling must be explicit.
- Device-specific unavailable cases must stay readable.
- UI should remain compact and not increase the device status row height.

## Verification Commands

- `cd backend && rtk go test ./internal/api -run TestStatusHandler`
- `cd backend && rtk go test ./...`
- `cd frontend && rtk npm run build`
- `git diff --check`

When practical after build:

- `docker compose up -d --build`
- `curl -s http://localhost:${HTTP_PORT:-8080}/api/status` or the local `.env` port such as `18085`
- Open `http://localhost:${HTTP_PORT:-8080}/` or the local `.env` port in the in-app browser and confirm the diagnostics card is visible.

## Rollback / Operational Notes

- The change is read-only and can be reverted by removing the diagnostics fields and dashboard card.
- Existing `/api/status` consumers should continue to work because the new field is optional and additive.
- If diagnostics are misleading in operation, hide the card first rather than changing real-control thresholds.
