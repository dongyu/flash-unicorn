SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

GO ?= go
LOCAL_BIN := $(CURDIR)/bin
export PATH := $(LOCAL_BIN):$(PATH)

SQLC_VERSION          := v1.31.1
MOCKGEN_VERSION       := v0.6.0
AIR_VERSION           := v1.67.4
GOLANGCI_LINT_VERSION := v2.15.0
MIGRATE_VERSION       := v4.20.1
GOVULNCHECK_VERSION   := v1.10.0

VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -s -w \
	-X unicorn/internal/platform/buildinfo.Version=$(VERSION) \
	-X unicorn/internal/platform/buildinfo.Commit=$(COMMIT) \
	-X unicorn/internal/platform/buildinfo.Date=$(BUILD_DATE)

MIGRATE_SCRIPT := ./scripts/migrate.sh
RACE_FLAG      := $(shell command -v gcc >/dev/null 2>&1 && echo -race)

.PHONY: help init tools build gen db-check migrate-up migrate-down migrate-reset migrate-status migrate-create dev run test test-int lint vuln tidy docker-build

help: ## Show available targets
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'

init: ## Rename template module & app name (usage: make init module=github.com/acme/app name=app)
	@test -n "$(module)" || (echo "usage: make init module=<go-module-path> [name=<app-name>]" && exit 1)
	./scripts/init-project.sh "$(module)" "$(name)"

tools: ## Install pinned local dev tools into ./bin
	@mkdir -p $(LOCAL_BIN)
	GOBIN=$(LOCAL_BIN) $(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
	GOBIN=$(LOCAL_BIN) $(GO) install go.uber.org/mock/mockgen@$(MOCKGEN_VERSION)
	GOBIN=$(LOCAL_BIN) $(GO) install github.com/air-verse/air@$(AIR_VERSION)
	GOBIN=$(LOCAL_BIN) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	GOBIN=$(LOCAL_BIN) $(GO) install -tags "pgx5 sqlite" github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)

build: ## Build server binary into ./bin/server with embedded version metadata
	@mkdir -p $(LOCAL_BIN)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $(LOCAL_BIN)/server ./cmd/server

gen: ## Run sqlc and GoMock code generation
	sqlc generate
	$(GO) generate ./internal/domain/... ./internal/usecase/...

db-check: ## Verify postgres and sqlite migrations stay in lockstep
	@pg_files=$$(ls -1 db/postgres/migrations/*.sql 2>/dev/null | xargs -n1 basename | sort); \
	sq_files=$$(ls -1 db/sqlite/migrations/*.sql 2>/dev/null | xargs -n1 basename | sort); \
	if [ "$$pg_files" != "$$sq_files" ]; then \
		echo "ERROR: postgres and sqlite migration filenames differ!"; \
		diff -u <(echo "$$pg_files") <(echo "$$sq_files"); \
		exit 1; \
	fi
	@echo "OK: postgres and sqlite migrations match."

migrate-up: ## Run all pending migrations (app via golang-migrate + River internal tables)
	$(MIGRATE_SCRIPT) up

migrate-down: ## Roll back 1 application migration step
	$(MIGRATE_SCRIPT) down 1

migrate-reset: ## Drop and re-apply all migrations (app + River; blocked in production without --force)
	$(MIGRATE_SCRIPT) reset

migrate-status: ## Show application and River migration status
	$(MIGRATE_SCRIPT) status

migrate-create: ## Create matching postgres & sqlite migration files (usage: make migrate-create name=add_users)
	@test -n "$(name)" || (echo "usage: make migrate-create name=<migration_name>" && exit 1)
	migrate create -ext sql -dir db/postgres/migrations -seq $(name)
	migrate create -ext sql -dir db/sqlite/migrations -seq $(name)

dev: ## Start server with Air hot reload
	air

run: ## Run server directly
	$(GO) run ./cmd/server

test: ## Run unit and zero-Docker SQLite integration tests
	$(GO) test $(RACE_FLAG) ./...

test-int: ## Run full integration tests including PostgreSQL contract suite
	$(GO) test $(RACE_FLAG) -tags=integration ./...

lint: ## Run golangci-lint (includes Clean Architecture depguard rules)
	golangci-lint run ./...

vuln: ## Scan dependencies for known vulnerabilities
	GOFLAGS=-mod=mod $(GO) run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

tidy: ## Verify go.mod/go.sum need no tidy-up
	$(GO) mod tidy
	@git diff --exit-code -- go.mod go.sum

docker-build: ## Build production multi-stage Docker image
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) --build-arg BUILD_DATE=$(BUILD_DATE) -t unicorn:latest .
