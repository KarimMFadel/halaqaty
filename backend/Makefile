# backend/Makefile — Go backend targets only
#
# Run from this directory:  make <target>
# Run from repo root:       make <target>  (root Makefile delegates here)
#
# Prerequisites:
#   go 1.22+, golangci-lint v1.64.x, golang-migrate CLI
#   DATABASE_URL env var for migrate-* targets
#   Example: export DATABASE_URL="postgres://user:pass@localhost:5432/halaqaty?sslmode=disable"

.DEFAULT_GOAL := help

.PHONY: help test test-integration test-feature-003-coverage \
        test-feature-001-auth-roles-profile test-feature-001-unit test-feature-001-contract test-feature-001-integration \
        lint build \
        migrate-up migrate-down migrate-fresh migrate-status migrate-create

# ──────────────────────────────────────────────────────────────────────────────

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ──────────────────────────────────────────────────────────────────────────────
# Tests
# ──────────────────────────────────────────────────────────────────────────────

test: ## Run unit tests (no database required)
	go test -short ./...

test-integration: ## Run integration tests (requires DATABASE_URL and a running PostgreSQL)
	@test -n "$$DATABASE_URL" || { echo "❌  DATABASE_URL is not set"; exit 1; }
	go test -tags=integration ./...

test-feature-003-coverage: ## Run Feature 003 unit coverage and enforce the 80% aggregate floor
	go test -short -coverprofile=feature003.coverage.out ./internal/...
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/check_feature_003_coverage.ps1 feature003.coverage.out

FEATURE_001_UNIT_DIRS := ./internal/auth ./internal/profile ./internal/rbac ./internal/middleware
FEATURE_001_CONTRACT_DIRS := ./tests/contract
FEATURE_001_INTEGRATION_DIRS := ./tests/integration
FEATURE_001_CONTRACT_RUN := Auth|Profile|CircleAssignRole|ResponseSafety
FEATURE_001_INTEGRATION_RUN := AuthFlow|ProfileFlow|CircleRoleAccess|RateLimitPolicy|PasswordStorageSafety

test-feature-001-auth-roles-profile: test-feature-001-unit test-feature-001-contract test-feature-001-integration ## Run auth/profile/rbac/middleware tests for feature 001

test-feature-001-unit: ## Run feature 001 unit tests (auth/profile/rbac/middleware)
	@set -e; \
	found=0; \
	for dir in $(FEATURE_001_UNIT_DIRS); do \
		if [ -d "$$dir" ]; then \
			found=1; \
			go test -short "$$dir/..."; \
		fi; \
	done; \
	if [ $$found -eq 0 ]; then \
		echo "ℹ️  No feature 001 unit test packages found"; \
	fi

test-feature-001-contract: ## Run feature 001 contract tests (if present)
	@set -e; \
	found=0; \
	for dir in $(FEATURE_001_CONTRACT_DIRS); do \
		if [ -d "$$dir" ]; then \
			found=1; \
			go test -tags=contract "$$dir/..." -run '$(FEATURE_001_CONTRACT_RUN)'; \
		fi; \
	done; \
	if [ $$found -eq 0 ]; then \
		echo "ℹ️  No feature 001 contract test packages found"; \
	fi

test-feature-001-integration: ## Run feature 001 integration tests (if present)
	@set -e; \
	found=0; \
	for dir in $(FEATURE_001_INTEGRATION_DIRS); do \
		if [ -d "$$dir" ]; then \
			found=1; \
		fi; \
	done; \
	if [ $$found -eq 0 ]; then \
		echo "ℹ️  No feature 001 integration test packages found"; \
		exit 0; \
	fi; \
	test -n "$$DATABASE_URL" || { echo "❌  DATABASE_URL is not set"; exit 1; }; \
	for dir in $(FEATURE_001_INTEGRATION_DIRS); do \
		if [ -d "$$dir" ]; then \
			go test -tags=integration "$$dir/..." -run '$(FEATURE_001_INTEGRATION_RUN)'; \
		fi; \
	done

# ──────────────────────────────────────────────────────────────────────────────
# Lint & Build
# ──────────────────────────────────────────────────────────────────────────────

lint: ## Run golangci-lint (zero violations required before PR)
	golangci-lint run ./...

build: ## Compile the API binary → bin/api
	go build -o bin/api ./cmd/api

# ──────────────────────────────────────────────────────────────────────────────
# Database Migrations  (golang-migrate CLI — https://github.com/golang-migrate/migrate)
# ──────────────────────────────────────────────────────────────────────────────

migrate-up: ## Apply all pending migrations
	@test -n "$$DATABASE_URL" || { echo "❌  DATABASE_URL is not set"; exit 1; }
	migrate -path migrations -database "$$DATABASE_URL" up

migrate-down: ## Roll back all migrations
	@test -n "$$DATABASE_URL" || { echo "❌  DATABASE_URL is not set"; exit 1; }
	migrate -path migrations -database "$$DATABASE_URL" down -all

migrate-fresh: ## ⚠️  Drop schema + recreate + run all migrations (destroys all data)
	@test -n "$$DATABASE_URL" || { echo "❌  DATABASE_URL is not set"; exit 1; }
	migrate -path migrations -database "$$DATABASE_URL" drop -f
	migrate -path migrations -database "$$DATABASE_URL" up

migrate-status: ## Show current migration version and dirty state
	@test -n "$$DATABASE_URL" || { echo "❌  DATABASE_URL is not set"; exit 1; }
	migrate -path migrations -database "$$DATABASE_URL" version

migrate-create: ## Create a new up/down migration pair — usage: make migrate-create NAME=create_foo
	@test -n "$$NAME" || { echo "❌  NAME is required — usage: make migrate-create NAME=create_foo"; exit 1; }
	migrate create -ext sql -dir migrations -seq $$NAME
