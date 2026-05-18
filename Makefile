.PHONY: test build-frontend run docker-up docker-down

test:
	cd backend && go test ./...

build-frontend:
	cd frontend && npm install && npm run build

run:
	cd backend && go run ./cmd/server

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down
