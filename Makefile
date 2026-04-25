all: run

STATICCHECK_VERSION ?= v0.7.0

build: frontend-build go-build

go-build:
	@echo "Building Go application..."
	CGO_ENABLED=1 go build -ldflags="-w -s -X 'hitkeep/cmd.Version=snapshot'" -o hitkeep ./cmd/hitkeep/main.go

frontend-build: frontend-dashboard-build

frontend-dashboard-build:
	@echo "Building Angular dashboard and tracker snippet..."
	@cd frontend/dashboard && npm ci --no-fund --no-audit && npm run build:prod
	@echo "Copying dashboard to public directory..."
	@cp -r frontend/dashboard/dist/dashboard/browser/* public/

DEV_ARGS ?=

dev:
	@bash ./scripts/dev.sh $(DEV_ARGS)

dev-seed:
	@bash ./scripts/dev.sh --seed

dev-backend:
	@echo "Starting Backend with Live Reload..."
	@HITKEEP_JWT_SECRET=$${HITKEEP_JWT_SECRET:-hitkeep-dev-jwt-secret} \
		HITKEEP_PUBLIC_URL=$${HITKEEP_PUBLIC_URL:-http://localhost:4200} \
		HITKEEP_MAIL_DRIVER=$${HITKEEP_MAIL_DRIVER:-smtp} \
		HITKEEP_MAIL_HOST=$${HITKEEP_MAIL_HOST:-localhost} \
		HITKEEP_MAIL_PORT=$${HITKEEP_MAIL_PORT:-1025} \
		HITKEEP_MAIL_ENCRYPTION=$${HITKEEP_MAIL_ENCRYPTION:-none} \
		HITKEEP_MCP_ENABLED=$${HITKEEP_MCP_ENABLED:-true} \
		air

dev-frontend:
	@echo "Starting Angular with Hot Reload..."
	@cd frontend/dashboard && npm i --no-fund --no-audit && npm start

run: build
	@./hitkeep

clean:
	@echo "Cleaning up..."
	@rm -f ./hitkeep
	@rm -rf frontend/dashboard/dist frontend/dashboard/node_modules

build-docker:
	@echo "Building binary for local platform..."
	CGO_ENABLED=1 go build -ldflags="-w -s -X 'hitkeep/cmd.Version=snapshot'" -o hitkeep-linux-amd64 ./cmd/hitkeep/main.go
	docker buildx build . \
		--platform linux/amd64 \
		--tag ghcr.io/pascalebeier/hitkeep:snapshot \
		--load

build-cloud:
	@./build-cloud.sh arm64

build-cloud-deploy:
	@./build-cloud.sh arm64 --deploy

update-default-spam-filter:
	@./scripts/update-default-spam-filter.sh

dev-cloud:
	@echo "Starting development environment (cloud/billing)..."
	@if ! command -v air > /dev/null; then \
		echo "Air is not installed. Installing..."; \
		go install github.com/air-verse/air@latest; \
	fi
	@make -j2 dev-cloud-backend dev-frontend

dev-cloud-backend:
	@echo "Starting Backend with Live Reload (billing tags)..."
	@HITKEEP_JWT_SECRET=$${HITKEEP_JWT_SECRET:-hitkeep-dev-jwt-secret} \
		HITKEEP_PUBLIC_URL=$${HITKEEP_PUBLIC_URL:-http://localhost:4200} \
		air -c .air-cloud.toml

staticcheck:
	@echo "Running Staticcheck..."
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) ./...

.PHONY: all build go-build frontend-build frontend-dashboard-build run clean update-default-spam-filter dev dev-seed dev-backend dev-frontend dev-cloud dev-cloud-backend staticcheck
