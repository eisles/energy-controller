# Container Timezone tzdata Implementation Plan

## Goal

Fix the runtime error `unknown time zone Asia/Tokyo` that appears in tariff control context when the app runs in the Docker container.

The fix should allow `time.LoadLocation("Asia/Tokyo")` to work reliably in the production container and keep the current safety gates for EcoFlow writes unchanged.

## Non-goals

- Do not change tariff optimization behavior.
- Do not change charging or discharging decisions.
- Do not enable real EcoFlow writes by default.
- Do not modify API credentials, device serial numbers, or `.env` secrets.
- Do not change the existing uncommitted PV forecast history graph feature.

## Current State

- The running container is built from `alpine:3.20`.
- The runtime stage does not install `tzdata`.
- The app stores and validates tariff/weather timezone values such as `Asia/Tokyo`.
- `time.LoadLocation(plan.Timezone)` is used by tariff control and tariff summary code.
- The current `/api/status` reports:
  - `tariff control context failed: unknown time zone Asia/Tokyo`
- Existing unrelated uncommitted changes are present for the PV forecast history graph. They will be preserved and not reverted.

## Files Likely To Change

- `Dockerfile`
  - Install `tzdata` in the runtime image so zoneinfo files are available.
- `backend/cmd/server/main.go`
  - Add a blank import of `time/tzdata` so the Go binary can resolve IANA time zones even if the runtime image or host lacks zoneinfo.

## Data/API Contracts

- No API contract changes.
- No database schema changes.
- No control command payload changes.

## Safety Boundaries

- This is a runtime dependency and timezone resolution fix only.
- Real-device write gates remain unchanged:
  - `ENABLE_REAL_CONTROL=true`
  - `SIMULATION_MODE=false`
  - existing write-target and command interval checks
- No new writes to EcoFlow or Nature APIs are introduced.

## Implementation Steps

1. Update the Docker runtime image to install `tzdata`.
2. Add `import _ "time/tzdata"` to `backend/cmd/server/main.go`.
3. Run Go formatting.
4. Run backend tests.
5. Build/restart Docker Compose.
6. Verify `/api/status` no longer includes `unknown time zone Asia/Tokyo`.

## Review Points

- Ensure the change is limited to timezone availability.
- Ensure default mock/simulation settings in `Dockerfile` remain unchanged.
- Ensure no real-device control path is loosened.
- Ensure the fix works even in minimal runtime environments.

## Verification Commands

```sh
cd backend && rtk go test ./...
docker compose up -d --build
curl -s http://localhost:18085/api/status
```

## Rollback / Operational Notes

- Rollback is safe by reverting the Dockerfile and `main.go` import changes.
- If the container is already running, it must be rebuilt and restarted for the Dockerfile change to take effect.
- The `time/tzdata` import increases binary size slightly but avoids future zoneinfo dependency failures.
