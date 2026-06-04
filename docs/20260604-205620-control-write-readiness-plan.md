# Control Write Readiness Visibility Plan

## Goal

Make it clear why EcoFlow write commands remain `dryRun=true` or `commandSent=false` without weakening any real-device safety gate.

The immediate operational question is: when a control plan says it can recover import/export behavior, which setting or runtime guard prevents an actual write?

## Non-goals

- Do not enable real EcoFlow writes by default.
- Do not change `.env`, secrets, device serial numbers, or Docker runtime credentials.
- Do not bypass `ENABLE_REAL_CONTROL`, `SIMULATION_MODE`, `MOCK_MODE`, `AUTO_CONTROL_ENABLED`, trial window, confirmation text, minimum command interval, duplicate fingerprint, or private MQTT write gates.
- Do not alter the charging strategy itself in this change.
- Do not send any real-device write during implementation or verification.

## Current state

- `/api/status` shows the current grid and high-level control state.
- Surplus and night-charge command logs expose `dryRun`, `wouldWrite`, `commandSent`, `suppressedReason`, and `decisionReason`.
- The dashboard header shows runtime mode, state, and trial window.
- Operators still have to infer whether the active blocker is mock mode, simulation mode, disabled real control, disabled auto control, trial expiry, missing confirmation, or a private MQTT gate.
- Recent local status check showed a surplus-control log with `dryRun=true`, `wouldWrite=false`, and a command candidate that was suppressed as a duplicate. The exact active write-readiness gates are not displayed as a first-class API/UI value.

## Data and API contract

Add a sanitized write-readiness object to `domain.Status` and `/api/status`.

Proposed JSON shape:

```json
{
  "controlWriteReadiness": {
    "ready": false,
    "mode": "dry-run",
    "reasons": [
      "simulation mode keeps device write disabled"
    ],
    "gates": {
      "mockMode": false,
      "simulationMode": true,
      "enableRealControl": true,
      "autoControlEnabled": true,
      "confirmEcoFlowWriteAccepted": true,
      "realControlTrialActive": true,
      "delta3ExecuteWrite": true,
      "delta3AllowPrivateWrite": true,
      "delta3AllowAutoWrite": true,
      "delta3AuxEnabled": true
    }
  }
}
```

Rules:

- Expose only booleans and reason labels.
- Never expose token values, email, password, access key, secret key, device SN, MQTT client id, or raw confirmation text.
- `ready` is true only when Pro 3 cloud write gates are ready: `MOCK_MODE=false`, `SIMULATION_MODE=false`, `ENABLE_REAL_CONTROL=true`, `AUTO_CONTROL_ENABLED=true`, confirmation accepted, and real-control trial active.
- DELTA 3 private MQTT gates are reported separately inside `gates`; they do not make Pro 3 `ready` false, but they explain auxiliary write readiness.

## Files likely to change

- `backend/internal/domain/status.go`
- `backend/internal/api/status_handler.go`
- A small backend helper for readiness calculation, either in `backend/internal/api` or a focused domain/config helper.
- `backend/internal/api/status_handler_test.go` or equivalent API tests.
- `frontend/lib/types.ts`
- `frontend/components/Header.tsx`
- Optionally `frontend/components/StatusCards.tsx` if a compact detail strip is a better fit than header badges alone.

## Safety boundaries

- This is read-only visibility.
- Existing execution code and write clients remain unchanged.
- Defaults remain mock + simulation.
- No new endpoint accepts writes.
- No secrets are returned.
- Verification uses API reads and tests only.

## Implementation steps

1. Add `ControlWriteReadiness` and `ControlWriteGates` structs to the domain status model.
2. Build a helper that derives sanitized readiness from `config.Config` and the existing real-control trial calculation.
3. Attach readiness to `/api/status` in the status handler.
4. Add backend tests for:
   - default safe config reports not ready because mock/simulation/real-control/auto-control gates block writes.
   - real-control-ready config reports ready.
   - missing confirmation or inactive trial reports not ready.
   - DELTA 3 private gates are represented without exposing secrets.
5. Extend frontend types.
6. Add a compact dashboard display:
   - Header badge: `実制御 ready` or `実制御 dry-run`.
   - Detail text for the first blocker reason.
   - Keep copy short and consistent with existing Japanese labels.
7. Verify local API output does not contain secrets.

## Review points

- The readiness logic must not be used to send commands.
- Labels must describe active blockers accurately.
- The API must not leak raw env values.
- The UI must not imply real writes are enabled when a gate is still false.
- Existing dirty worktree changes must not be reverted.

## Verification commands

```bash
cd backend && rtk go test ./...
cd frontend && rtk npm run build
curl -s http://localhost:18085/api/status
```

For the curl verification, inspect only the new sanitized `controlWriteReadiness` fields and confirm no secrets are present.

## Rollback and operational notes

- Rollback is a normal git revert of the changed status/API/UI files.
- If the UI shows `ready=false`, the operator should check the listed blocker before expecting `commandSent=true`.
- If the UI shows `ready=true` but command logs still show `commandSent=false`, the next investigation should focus on command-level suppressions such as duplicate fingerprint, minimum interval, stale mode reflection, or writer errors.
