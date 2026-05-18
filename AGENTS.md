# Project Instructions

## Project
Energy Controller for Nature Remo E and EcoFlow DELTA Pro 3.

This project controls charging behavior based on home grid import/export status.

## Stack
- Backend: Go
- Frontend: Next.js Static Export
- Database: SQLite
- Deployment: Docker Compose
- Runtime target: local home server, mini PC, Raspberry Pi, or NAS

## Safety Rules
- Never enable real EcoFlow control by default.
- Default mode must be mock + simulation.
- EcoFlow write commands must only run when `ENABLE_REAL_CONTROL=true` and `SIMULATION_MODE=false`.
- Do not hardcode API tokens, device serial numbers, access keys, or secret keys.
- Use `.env` and provide `.env.example`.
- Log every control decision.
- Do not send frequent control commands.
- Implement hysteresis and minimum command interval.
- Any real-device write path must have unit tests.
- If API values are uncertain, keep them behind adapters and add TODO comments.
- Never connect EcoFlow AC output back into a home outlet or grid circuit.

## Implementation Rules
- Keep external API details inside adapter packages.
- Do not leak Nature Remo or EcoFlow response structures into the domain layer.
- Domain logic must be unit-testable without network access.
- Go tests must pass with mock clients.
- The app must run without real API credentials.
- Start with read-only and simulation mode.
- Add EcoFlow real control only in the final phase.

## Required Commands
- `cd backend && go test ./...`
- `docker compose up -d --build`
- `docker compose down`

## Done Criteria
- Backend starts on port `8080`.
- `GET /api/status` works.
- Frontend is served by Go.
- SQLite DB is created automatically.
- Mock mode shows changing sample values.
- Real EcoFlow control is disabled by default.
- README explains setup and safety warnings.
