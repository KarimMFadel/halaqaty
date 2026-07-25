# Root Makefile — aggregate targets + cross-cutting tools
#
# This file delegates stack-specific work to sub-Makefiles:
#   backend/Makefile  ← all Go targets (test, lint, build, migrate-*)
#   mobile/Makefile   ← all Flutter targets (test, analyze, build-apk)
#
# Pattern: running `make <target>` here calls the same target in each sub-project.
# You can also cd into backend/ or mobile/ and run `make <target>` directly for
# a tighter feedback loop when working on one stack.
#
# Cross-cutting targets (Docker Compose, secrets scan, OpenAPI lint) live here only.

.DEFAULT_GOAL := help

# Suppress "Entering/Leaving directory" noise from recursive make calls
MAKE := $(MAKE) --no-print-directory

.PHONY: help \
        test test-integration lint build \
        migrate-up migrate-down migrate-fresh migrate-status migrate-create \
        secrets api-lint \
        dev up down logs

# ──────────────────────────────────────────────────────────────────────────────

help: ## Show all available targets (root + sub-projects)
	@echo ""
	@echo "\033[1m[root]\033[0m  Cross-cutting targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "\033[1m[backend/]\033[0m  Go targets (run from backend/ or via 'make -C backend help'):"
	@$(MAKE) -C backend help 2>/dev/null || echo "  (not scaffolded yet)"
	@echo ""
	@echo "\033[1m[mobile/]\033[0m   Flutter targets (run from mobile/ or via 'make -C mobile help'):"
	@$(MAKE) -C mobile help 2>/dev/null || echo "  (not scaffolded yet)"
	@echo ""

# ──────────────────────────────────────────────────────────────────────────────
# Aggregate: delegates to both sub-projects
# ──────────────────────────────────────────────────────────────────────────────

test: ## Run all unit tests (Go + Flutter)
	$(MAKE) -C backend test
	$(MAKE) -C mobile test

test-integration: ## Run Go integration tests (requires DATABASE_URL)
	$(MAKE) -C backend test-integration

lint: ## Run all linters (golangci-lint + flutter analyze + spectral + gitleaks)
	$(MAKE) -C backend lint
	$(MAKE) -C mobile analyze
	$(MAKE) api-lint
	$(MAKE) secrets

build: ## Build all artifacts (Go binary + Flutter APK)
	$(MAKE) -C backend build
	$(MAKE) -C mobile build-apk

# ──────────────────────────────────────────────────────────────────────────────
# Database migrations — delegated to backend/Makefile
# (sprint acceptance gates reference these from root, so they live here too)
# ──────────────────────────────────────────────────────────────────────────────

migrate-up: ## Apply pending migrations (delegates to backend/Makefile)
	$(MAKE) -C backend migrate-up

migrate-down: ## Roll back all migrations (delegates to backend/Makefile)
	$(MAKE) -C backend migrate-down

migrate-fresh: ## ⚠️  Drop schema + run all migrations (delegates to backend/Makefile)
	$(MAKE) -C backend migrate-fresh

migrate-status: ## Show current migration version (delegates to backend/Makefile)
	$(MAKE) -C backend migrate-status

migrate-create: ## Create a new migration pair — usage: make migrate-create NAME=create_foo
	$(MAKE) -C backend migrate-create NAME=$(NAME)

# ──────────────────────────────────────────────────────────────────────────────
# Cross-cutting: secrets scan + OpenAPI linting
# These don't belong to backend or mobile — they operate on the whole repo
# ──────────────────────────────────────────────────────────────────────────────

secrets: ## Scan for secrets across the full repo with gitleaks
	gitleaks detect --source .

api-lint: ## Lint docs/contracts/openapi.yaml with Spectral (see .spectral.yaml for config)
	spectral lint docs/contracts/openapi.yaml

# ──────────────────────────────────────────────────────────────────────────────
# Docker Compose — spans both stacks; lives at root only
# ──────────────────────────────────────────────────────────────────────────────

up: ## Start all services (PostgreSQL, MinIO, LiveKit, Go API) with Docker Compose
	docker compose up -d

dev: up ## Start dev environment + show service status (alias for 'make up')
	docker compose ps

down: ## Stop all Docker Compose services
	docker compose down

logs: ## Tail logs from all Docker Compose services
	docker compose logs -f

