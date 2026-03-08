.PHONY: up down test test-integration lint demo-happy-path demo-all

up:
	docker compose up --build

down:
	docker compose down -v

test:
	cd backend && go test ./... -v

test-integration:
	cd backend && go test ./tests/ -v -tags=integration

lint:
	cd backend && go vet ./...

demo-happy-path:
	./scripts/demo-happy-path.sh

demo-all:
	./scripts/demo-all-scenarios.sh
