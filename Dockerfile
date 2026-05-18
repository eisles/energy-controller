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
EXPOSE 8080
CMD ["/app/energy-controller"]
