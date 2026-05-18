FROM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend ./
RUN npm run build

FROM golang:1.22-alpine AS backend
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum* ./
RUN go mod download
COPY backend ./
RUN CGO_ENABLED=0 go build -o /out/energy-controller ./cmd/server

FROM alpine:3.20
WORKDIR /app
RUN addgroup -S app && adduser -S app -G app
COPY --from=backend /out/energy-controller /app/energy-controller
COPY --from=frontend /src/frontend/out /app/frontend/out
RUN mkdir -p /app/data && chown -R app:app /app
USER app
ENV APP_ENV=production
ENV HTTP_PORT=8080
ENV DB_PATH=/app/data/energy.db
ENV FRONTEND_DIR=/app/frontend/out
ENV MOCK_MODE=true
ENV SIMULATION_MODE=true
ENV ENABLE_REAL_CONTROL=false
ENV NATURE_MODE=cloud
ENV NATURE_ACCESS_TOKEN=
ENV NATURE_APPLIANCE_ID=
ENV NATURE_LOCAL_BASE_URL=http://remo-e.local
ENV ECOFLOW_ACCESS_KEY=
ENV ECOFLOW_SECRET_KEY=
ENV ECOFLOW_DEVICE_SN=
ENV ECOFLOW_BASE_URL=https://api-e.ecoflow.com
ENV POLL_INTERVAL_SEC=30
ENV START_EXPORT_THRESHOLD_W=700
ENV STOP_EXPORT_THRESHOLD_W=300
ENV SAFETY_MARGIN_W=150
ENV MIN_CHARGE_W=400
ENV MAX_CHARGE_W=1500
ENV TARGET_SOC=90
ENV MIN_COMMAND_INTERVAL_SEC=60
ENV MIN_COMMAND_DIFF_W=100
ENV REQUIRE_CONSECUTIVE_EXPORT_COUNT=2
ENV REQUIRE_CONSECUTIVE_IMPORT_COUNT=2
EXPOSE 8080
CMD ["/app/energy-controller"]
