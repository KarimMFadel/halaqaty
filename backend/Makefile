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

.PHONY: help test test-contract test-integration coverage test-feature-003-coverage \
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

test-contract: ## Run ALL contract tests, unfiltered (the contract gate; focused subsets: test-feature-001-contract)
	go test -tags=contract ./tests/contract/...

coverage: ## Project-wide coverage gate: unit+contract+integration combined profile, >=80% aggregate floor over internal/ (requires DATABASE_URL)
	powershell -NoProfile -Command "if (-not $$env:DATABASE_URL) { Write-Error 'DATABASE_URL is not set'; exit 1 }"
	go test -count=1 -short -tags='contract integration' '-coverpkg=./internal/...' '-coverprofile=coverage.out' ./internal/... ./tests/contract ./tests/integration ./tests/performance
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/check_coverage.ps1 coverage.out

test-feature-003-coverage: coverage ## Feature 003 coverage gate (delegates to the project-wide coverage target)

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

# FOCUSED dev-only subsets — kept for reference/fast iteration while debugging a
# feature. They run ONLY the tests matching the -run filter below, so they are
# NOT coverage or merge gates. Gates: make test, make test-contract,
# make test-integration, make coverage.

test-feature-001-contract: ## FOCUSED dev only — feature 001 contract subset; full gate is `make test-contract`
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

test-feature-001-integration: ## FOCUSED dev only — feature 001 integration subset; full gate is `make test-integration` (needs DATABASE_URL)
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
