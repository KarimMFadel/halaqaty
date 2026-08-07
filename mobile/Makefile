# mobile/Makefile — Flutter mobile targets only
#
# Run from this directory:  make <target>
# Run from repo root:       make <target>  (root Makefile delegates here)
#
# Prerequisites:
#   Flutter SDK (stable channel) — https://docs.flutter.dev/get-started/install
#   Run `flutter pub get` once after cloning or updating pubspec.yaml

.DEFAULT_GOAL := help

.PHONY: help deps test test-feature-001-auth-roles-profile test-feature-001-widget test-feature-001-integration analyze build-apk build-ios

# ──────────────────────────────────────────────────────────────────────────────

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ──────────────────────────────────────────────────────────────────────────────
# Setup
# ──────────────────────────────────────────────────────────────────────────────

deps: ## Install Flutter dependencies (run after cloning or pubspec.yaml changes)
	flutter pub get

# ──────────────────────────────────────────────────────────────────────────────
# Tests & Analysis
# ──────────────────────────────────────────────────────────────────────────────

test: ## Run unit and widget tests
	flutter test test

FEATURE_001_WIDGET_DIRS := test/widget/auth test/widget/profile
FEATURE_001_INTEGRATION_FILES := integration_test/auth_journey_test.dart integration_test/profile_flow_test.dart integration_test/role_access_test.dart

test-feature-001-auth-roles-profile: test-feature-001-widget test-feature-001-integration ## Run auth/profile feature tests for feature 001

test-feature-001-widget: ## Run auth/profile widget tests (if present)
	@set -e; \
	found=0; \
	for dir in $(FEATURE_001_WIDGET_DIRS); do \
		if [ -d "$$dir" ]; then \
			found=1; \
			flutter test "$$dir"; \
		fi; \
	done; \
	if [ $$found -eq 0 ]; then \
		echo "ℹ️  No feature 001 widget test directories found"; \
	fi

test-feature-001-integration: ## Run auth/profile integration tests (if present)
	@set -e; \
	found=0; \
	for file in $(FEATURE_001_INTEGRATION_FILES); do \
		if [ -f "$$file" ]; then \
			found=1; \
			flutter test "$$file"; \
		fi; \
	done; \
	if [ $$found -eq 0 ]; then \
		echo "ℹ️  No feature 001 integration test files found"; \
	fi

analyze: ## Run flutter analyze (static analysis — zero issues required before PR)
	flutter analyze

# ──────────────────────────────────────────────────────────────────────────────
# Builds
# ──────────────────────────────────────────────────────────────────────────────

build-apk: ## Build a debug Android APK (for internal testing)
	flutter build apk --debug

build-ios: ## Build an iOS release archive (requires macOS + Xcode)
	flutter build ipa
