# EcoFlow Private MQTT Auth Throttle Implementation Plan

## Goal

EcoFlow private MQTT read/probe paths should avoid repeated `/auth/login` calls that trigger `code=1001 message=Server is too busy`.

Implement a guarded authentication control layer that:

- Reuses a successful private MQTT session across clients in the same server process.
- Adds a minimum relogin interval after busy login failures.
- Preserves existing real-device write safety gates.
- Keeps credentials and tokens out of logs, plans, and persisted repository files.

## Non-goals

- Do not add new EcoFlow write commands.
- Do not implement DELTA 3 Plus TOU/self-powered mode switching in this change.
- Do not persist EcoFlow tokens to the repository or to SQLite.
- Do not change Nature Remo polling behavior beyond existing uncommitted UI polling changes.

## Current State

- `backend/internal/ecoflowprivate.Client` already reuses a `Session` inside one client instance.
- The server and probe tooling can still create multiple client instances, so repeated login can happen across devices, routes, control-loop reads, and short-lived probe runs.
- `/api/devices/statuses` shows normal private MQTT status reads are working, while raw probe CLI currently fails at private login with `Server is too busy`.
- Existing uncommitted file before this task: `frontend/components/Dashboard.tsx`. It is out of scope for this backend auth-control change.

## Files Likely To Change

- `backend/internal/ecoflowprivate/client.go`
- `backend/internal/ecoflowprivate/auth.go` or a new auth/session cache helper in the same package
- `backend/internal/ecoflowprivate/client_test.go` or a new test file

## Data And API Contracts

- No public API response shape changes are required.
- Internal `Config` may gain optional session-cache/backoff fields only if needed for tests.
- Session cache keys must not include secrets. A key may use normalized API host, email, and MQTT client ID.
- Cached session values must remain in memory by default.

## Safety Boundaries

- No real-device write path is added.
- Existing write guards remain unchanged:
  - `ENABLE_REAL_CONTROL=true`
  - `SIMULATION_MODE=false`
  - private API write flags
  - command confirmation
- Error messages must not include password, token, or MQTT certificate password.
- If a cached session fails with an auth-like MQTT error, invalidate it and retry at most once.
- If login returns busy, cache only the backoff state and error summary, not credentials.

## Implementation Steps

1. Add a package-level in-memory session cache for EcoFlow private auth.
2. Key cache entries by non-secret auth identity.
3. Store:
   - successful session
   - session expiry timestamp
   - last busy/error timestamp and retry-after timestamp
4. Teach `Client.cachedSession` to:
   - use a valid shared session before calling `Login`
   - respect busy backoff before retrying login
   - publish successful sessions back to the shared cache
   - invalidate the shared session when an auth-like MQTT error is detected
5. Keep test clients isolated, either by allowing an injected cache or disabling shared cache for custom test config.
6. Add unit tests for:
   - two clients sharing one successful login
   - busy login backoff preventing repeated login attempts
   - auth failure invalidating shared session and allowing refresh

## Review Points

- No secrets are logged or written.
- Concurrency is protected with mutexes.
- Backoff is scoped to auth identity, not device serial number.
- Existing per-client session reuse tests still pass.
- No write-command behavior changes.

## Verification

- `cd backend && rtk go test ./internal/ecoflowprivate ./internal/api`
- If broad enough after implementation: `cd backend && rtk go test ./...`

## Rollback Notes

The change should be isolated to private MQTT auth caching. If unexpected MQTT client-ID conflicts appear in real operation, disable shared session reuse in config or revert the auth cache while keeping existing per-client session behavior.
