all: run

STATICCHECK_VERSION ?= v0.7.0
GO_BUILD_TAGS ?= $(shell ./scripts/go-build-tags.sh)
CLOUD_GO_BUILD_TAGS ?= $(shell ./scripts/go-build-tags.sh cloud)
DOCKER_PLATFORM ?= $(shell docker version --format '{{.Server.Os}}/{{.Server.Arch}}' 2>/dev/null || printf 'linux/amd64')
DOCKER_IMAGE ?= ghcr.io/pascalebeier/hitkeep:snapshot
DOCKER_CLOUD_IMAGE ?= hitkeep:cloud-local

help: ## List the supported human and agent entry points
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_.-]+:.*## / {printf "  %-24s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

doctor: ## Check whether Docker and/or native development prerequisites are ready
	@bash ./scripts/dev-doctor.sh

build: frontend-build go-build

go-build:
	@echo "Building Go application..."
	CGO_ENABLED=1 go build -tags "$(GO_BUILD_TAGS)" -ldflags="-w -s -X 'hitkeep/cmd.Version=snapshot'" -o hitkeep ./cmd/hitkeep/main.go

frontend-build: frontend-dashboard-build

frontend-dashboard-build:
	@echo "Building Angular dashboard and tracker snippet..."
	@cd frontend/dashboard && npm ci --no-fund --no-audit && npm run build:prod
	@echo "Copying dashboard to public directory..."
	@cp -r frontend/dashboard/dist/dashboard/browser/* public/

DEV_ARGS ?=
DEV_COMPOSE ?= docker compose -f compose.dev.yaml
DEV_CLOUD_COMPOSE ?= $(DEV_COMPOSE) -f compose.dev-cloud.yaml

dev: ## Start the native backend and frontend with live reload
	@bash ./scripts/dev.sh $(DEV_ARGS)

dev-seed: ## Seed data, then start native development
	@bash ./scripts/dev.sh --seed

dev-cloud: ## Start native development with cloud build tags and defaults
	@bash ./scripts/dev.sh --cloud

dev-cloud-seed: ## Seed data, then start native cloud development
	@bash ./scripts/dev.sh --cloud --seed

dev-backend:
	@echo "Starting Backend with Live Reload..."
	@HITKEEP_JWT_SECRET=$${HITKEEP_JWT_SECRET:-hitkeep-dev-jwt-secret} \
		HITKEEP_PUBLIC_URL=$${HITKEEP_PUBLIC_URL:-http://localhost:4200} \
		HITKEEP_MAIL_DRIVER=$${HITKEEP_MAIL_DRIVER:-smtp} \
		HITKEEP_MAIL_HOST=$${HITKEEP_MAIL_HOST:-localhost} \
		HITKEEP_MAIL_PORT=$${HITKEEP_MAIL_PORT:-1025} \
		HITKEEP_MAIL_ENCRYPTION=$${HITKEEP_MAIL_ENCRYPTION:-none} \
		HITKEEP_MCP_ENABLED=$${HITKEEP_MCP_ENABLED:-true} \
		go tool air -c .air.toml

dev-frontend:
	@echo "Starting Angular with Hot Reload..."
	@cd frontend/dashboard && npm i --no-fund --no-audit && npm start

run: build
	@./hitkeep

clean:
	@echo "Cleaning up..."
	@rm -f ./hitkeep
	@rm -rf frontend/dashboard/dist frontend/dashboard/node_modules

build-docker: ## Build the self-hosted production image locally (never pushes)
	@echo "Building self-hosted image for $(DOCKER_PLATFORM)..."
	docker buildx build . \
		--target local-image \
		--platform $(DOCKER_PLATFORM) \
		--build-arg GO_BUILD_TAGS="$(GO_BUILD_TAGS)" \
		--build-arg HITKEEP_VARIANT=self-hosted \
		--tag $(DOCKER_IMAGE) \
		--load

build-docker-cloud: ## Build a production-style cloud image locally (never pushes)
	@echo "Building local-only cloud image for $(DOCKER_PLATFORM)..."
	docker buildx build . \
		--target local-image \
		--platform $(DOCKER_PLATFORM) \
		--build-arg GO_BUILD_TAGS="$(CLOUD_GO_BUILD_TAGS)" \
		--build-arg HITKEEP_VARIANT=cloud-local \
		--tag $(DOCKER_CLOUD_IMAGE) \
		--load

smoke-docker: build-docker ## Build and health-check the self-hosted production container
	@bash ./scripts/docker-smoke.sh $(DOCKER_IMAGE) self-hosted

smoke-docker-cloud: build-docker-cloud ## Build and health-check the cloud production container
	@bash ./scripts/docker-smoke.sh $(DOCKER_CLOUD_IMAGE) cloud-local --cloud

update-default-spam-filter:
	@./scripts/update-default-spam-filter.sh

dev-cloud-backend:
	@echo "Starting Backend with Live Reload (cloud tags)..."
	@HITKEEP_GO_BUILD_TAGS="$$(./scripts/go-build-tags.sh cloud)" \
		HITKEEP_JWT_SECRET=$${HITKEEP_JWT_SECRET:-hitkeep-dev-jwt-secret} \
		HITKEEP_PUBLIC_URL=$${HITKEEP_PUBLIC_URL:-http://localhost:4200} \
		HITKEEP_MAIL_DRIVER=$${HITKEEP_MAIL_DRIVER:-smtp} \
		HITKEEP_MAIL_HOST=$${HITKEEP_MAIL_HOST:-localhost} \
		HITKEEP_MAIL_PORT=$${HITKEEP_MAIL_PORT:-1025} \
		HITKEEP_MAIL_ENCRYPTION=$${HITKEEP_MAIL_ENCRYPTION:-none} \
		HITKEEP_MCP_ENABLED=$${HITKEEP_MCP_ENABLED:-true} \
		HITKEEP_CLOUD_HOSTED=$${HITKEEP_CLOUD_HOSTED:-true} \
		HITKEEP_CLOUD_SIGNUP_ENABLED=$${HITKEEP_CLOUD_SIGNUP_ENABLED:-true} \
		HITKEEP_CLOUD_JURISDICTION=$${HITKEEP_CLOUD_JURISDICTION:-EU} \
		HITKEEP_CLOUD_REGION=$${HITKEEP_CLOUD_REGION:-eu-central-1} \
		HITKEEP_CLOUD_UPGRADE_URL=$${HITKEEP_CLOUD_UPGRADE_URL:-http://localhost:4200/admin/team} \
		HITKEEP_CLOUD_SUPPORT_URL=$${HITKEEP_CLOUD_SUPPORT_URL:-https://hitkeep.com/support/help/} \
		HITKEEP_CLOUD_CHECKOUT_SUCCESS_URL=$${HITKEEP_CLOUD_CHECKOUT_SUCCESS_URL:-http://localhost:4200/admin/team?checkout=success} \
		HITKEEP_CLOUD_CHECKOUT_CANCEL_URL=$${HITKEEP_CLOUD_CHECKOUT_CANCEL_URL:-http://localhost:4200/admin/team?checkout=cancelled} \
		go tool air -c .air.toml

dev-docker: ## Start the self-hosted Docker development stack
	@echo "Starting Docker development environment..."
	@$(DEV_COMPOSE) up --build backend frontend mailpit

dev-docker-seed: ## Seed data, then start the self-hosted Docker development stack
	@echo "Seeding Docker development data..."
	@$(DEV_COMPOSE) run --rm seed
	@echo "Starting Docker development environment..."
	@$(DEV_COMPOSE) up --build backend frontend mailpit

dev-docker-cloud: ## Start the Docker development stack with cloud build tags and defaults
	@echo "Starting Docker development environment (cloud/billing)..."
	@$(DEV_CLOUD_COMPOSE) up --build backend frontend mailpit

dev-docker-cloud-seed: ## Seed data, then start the Docker cloud development stack
	@echo "Seeding Docker development data..."
	@$(DEV_CLOUD_COMPOSE) run --rm seed
	@echo "Starting Docker development environment (cloud/billing)..."
	@$(DEV_CLOUD_COMPOSE) up --build backend frontend mailpit

dev-docker-down: ## Stop either Docker development stack
	@$(DEV_COMPOSE) down

dev-docker-clean: ## Stop Docker development and delete its local volumes
	@$(DEV_COMPOSE) down --volumes --remove-orphans

staticcheck:
	@echo "Running Staticcheck..."
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION) -tags "$(GO_BUILD_TAGS)" ./...

.PHONY: all help doctor build go-build frontend-build frontend-dashboard-build run clean build-docker build-docker-cloud smoke-docker smoke-docker-cloud update-default-spam-filter dev dev-seed dev-backend dev-frontend dev-cloud dev-cloud-seed dev-cloud-backend dev-docker dev-docker-seed dev-docker-cloud dev-docker-cloud-seed dev-docker-down dev-docker-clean staticcheck
