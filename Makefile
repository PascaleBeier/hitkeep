# Compatibility aliases only. ./hk is the development workflow authority.
HK := ./hk

all: help

help: ## Show the canonical HitKeep developer CLI
	@$(HK) help

doctor: ## Diagnose native and container prerequisites
	@$(HK) doctor

setup: ## Prepare this worktree
	@$(HK) setup

dev: ## Start native development
	@$(HK) dev --runtime native

dev-seed: ## Seed isolated data and start native development
	@$(HK) dev --runtime native --seed

dev-cloud: ## Start native cloud-parity development
	@$(HK) dev --runtime native --variant cloud

dev-cloud-seed: ## Seed isolated data and start native cloud development
	@$(HK) dev --runtime native --variant cloud --seed

dev-docker: ## Start container-backed development
	@$(HK) dev --runtime container

dev-docker-seed: ## Seed isolated data and start container development
	@$(HK) dev --runtime container --seed

dev-docker-cloud: ## Start container-backed cloud-parity development
	@$(HK) dev --runtime container --variant cloud

dev-docker-cloud-seed: ## Seed isolated data and start container cloud development
	@$(HK) dev --runtime container --variant cloud --seed

dev-docker-down: ## Stop this worktree's container stack
	@$(HK) dev stop --runtime container

dev-docker-clean: dev-docker-down ## Compatibility alias; never deletes worktree data

dev-backend dev-frontend: dev ## Compatibility aliases for the unified native stack

build go-build frontend-build frontend-dashboard-build: ## Build the self-hosted production binary
	@$(HK) build binary

build-docker: ## Build the self-hosted production image locally
	@$(HK) build image

build-docker-cloud: ## Build the local-only cloud production image
	@$(HK) build image --variant cloud

smoke-docker: ## Build and smoke-test the self-hosted production image
	@$(HK) smoke

smoke-docker-cloud: ## Build and smoke-test the local-only cloud image
	@$(HK) smoke --variant cloud

qa: ## Run change-aware QA
	@$(HK) qa

qa-pr: ## Run the CI pull-request contract
	@$(HK) qa pr

qa-full: ## Run exhaustive self-hosted and cloud QA
	@$(HK) qa full

fmt: ## Format repository Go sources
	@$(HK) fmt

fmt-check: ## Check repository Go formatting without writing
	@$(HK) fmt check

fix: ## Apply pinned Go source migrations
	@$(HK) fix

fix-check: ## Check pinned Go source migrations without writing
	@$(HK) fix check

staticcheck: ## Run the canonical Staticcheck gate
	@$(HK) qa changed --gate go-staticcheck

run: dev ## Compatibility alias for development

clean: ## Stop this worktree's container services without deleting files or data
	@$(HK) dev stop --runtime container

update-default-spam-filter:
	@./scripts/update-default-spam-filter.sh

.PHONY: all help doctor setup dev dev-seed dev-cloud dev-cloud-seed dev-docker dev-docker-seed dev-docker-cloud dev-docker-cloud-seed dev-docker-down dev-docker-clean dev-backend dev-frontend build go-build frontend-build frontend-dashboard-build build-docker build-docker-cloud smoke-docker smoke-docker-cloud qa qa-pr qa-full fmt fmt-check fix fix-check staticcheck run clean update-default-spam-filter
