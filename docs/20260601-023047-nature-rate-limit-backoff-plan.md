# Nature Remo 429 backoff and cache implementation plan

## Goal

Keep grid import/export control responsive while reducing Nature Cloud HTTP 429 risk.

The control loop should still be able to run at a short practical interval, but Nature Cloud reads must not be issued more often than necessary. When Nature Cloud returns HTTP 429, the system should back off and reuse the last successful smart-meter payload for a bounded period instead of treating grid power as 0 W.

## Non-goals

- Do not change EcoFlow real-write safety gates.
- Do not enable real control by default.
- Do not add or expose Nature/EcoFlow credentials.
- Do not change charge/discharge planner thresholds except for using cached Nature values when rate limited.
- Do not replace Nature Cloud with local Nature Remo access in this change.

## Current state

- `backend/internal/nature/cloud_client.go`
  - `CurrentGridPower` calls `fetchAppliances`.
  - `CurrentEnergyMeterReading` calls `fetchAppliances`.
  - In one `recordStatus` cycle this can create two Nature Cloud requests.
- `backend/internal/mock/status_provider.go`
  - On any Nature grid read error, `currentGridPower` returns 0 W and sets `LastError`.
  - That makes control judgement unstable during 429.
- `backend/cmd/server/main.go`
  - `recordEnergyMeterReading` runs after each control decision.
  - Because the Nature client does not cache the appliance payload, cumulative energy reads can add API pressure.

## Design

### Nature Cloud response cache

Add an in-memory cache inside `nature.CloudClient`.

- Cache the whole `/1/echonetlite/appliances` response.
- Default cache TTL: 10 seconds.
- Allow override through `CloudConfig.CacheTTL`.
- Rate-limit fallback only uses cached payloads within a bounded max age.
- A single control cycle can parse both instantaneous power and cumulative energy from the same cached payload.

### HTTP 429 backoff

When Nature Cloud returns HTTP 429:

- Set `rateLimitUntil`.
- Use `Retry-After` seconds when present.
- Otherwise use a default 60 second backoff.
- If a cached payload exists, return that cached payload and expose a warning.
- If no recent cached payload exists, return an error and keep backing off until the retry time.

This preserves recent grid direction for short outages without pretending the live read succeeded.

### Warning propagation

Add an optional warning interface from the Nature reader to the status provider.

- `StatusProvider.currentGridPower` should include `LastGridReadWarning` in `LastError`.
- When cached data is used due to 429, the UI/API can show that Nature Cloud is rate limited while still displaying the last known import/export.
- Existing stale check remains: if Nature's property `updated_at` is older than the status provider's stale threshold, it is still reported as stale.

### Safety boundaries

- Real EcoFlow write gates remain unchanged.
- 429 cached values should not bypass command hysteresis or minimum command intervals.
- If cached data is too old, existing stale warning remains visible.
- If no cached data exists after 429, keep existing fail-safe behavior: grid power becomes 0 W and `LastError` is set.

## Files likely to change

- `backend/internal/nature/cloud_client.go`
- `backend/internal/nature/cloud_client_test.go`
- `backend/internal/mock/status_provider.go`
- `backend/internal/mock/status_provider_test.go`

## Data and API contracts

- No database migration.
- No public API shape change is required.
- `lastError` may now contain a warning such as Nature Cloud rate-limited and cached response used.
- Existing `gridW`, `importW`, and `exportW` remain numeric fields.

## Implementation steps

1. Add cache and backoff fields to `nature.CloudClient`.
2. Add `CloudConfig.CacheTTL`, `CloudConfig.RateLimitBackoff`, and optional clock injection for deterministic tests.
3. Update `fetchAppliances` to:
   - return fresh cache when valid,
   - avoid network calls during active backoff when cache exists,
   - set backoff on HTTP 429,
   - update cache on successful response.
4. Add `LastGridReadWarning() *string` on `CloudClient`.
5. Update `mock.StatusProvider.currentGridPower` to merge optional reader warnings into `LastError`.
6. Add unit tests:
   - grid and cumulative reads in quick succession reuse one HTTP response,
   - HTTP 429 with cache returns cached grid power and warning,
   - HTTP 429 without cache returns an error,
   - status provider reports reader warning without zeroing grid power.

## Review points

- The cache must not hide non-429 errors when no cached payload exists.
- Backoff must not spin requests while rate limited.
- Warning text must not include secrets or tokens.
- Tests must not depend on real Nature Cloud.

## Verification commands

```sh
cd backend && GOCACHE=$PWD/.gocache rtk go test ./internal/nature ./internal/mock
cd backend && GOCACHE=$PWD/.gocache rtk go test ./...
rtk codex review --uncommitted
```

## Operational notes

- This change does not require increasing `POLL_INTERVAL_SEC` just to avoid double Nature Cloud calls.
- If 429 continues after this change, the next operational step is to increase the cache TTL/backoff or increase the control loop interval from the environment to reduce Nature API frequency, not to weaken charge/discharge hysteresis.
- The running local server must be rebuilt/restarted before the change affects `http://localhost:18085`.
