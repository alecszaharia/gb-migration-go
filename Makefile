SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

BINARY   ?= brizy-migration
BIN_DIR  ?= bin
PKG      ?= ./...
GOFLAGS  ?=
CGO_ENABLED ?= 0
LDFLAGS  ?= -s -w
GOOS     ?= $(shell go env GOOS)
GOARCH   ?= $(shell go env GOARCH)
CGO_ENABLED   ?= $(shell go env CGO_ENABLED)

GB_MIGRATION_TEST_DSN ?=

GO_FILES := $(shell find . -name '*.go' -not -path './bin/*' -not -path './.git/*')

.PHONY: help build build-linux install run test test-integration test-all \
        fmt fmt-check vet lint tidy deps clean baseline

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n\nTargets:\n"} \
	     /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: ## Build binary for the host platform into $(BIN_DIR)
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY) .

build-linux: ## Cross-compile a static linux/amd64 binary
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' \
		-o $(BIN_DIR)/$(BINARY)-linux-amd64 .

install: ## Install binary into $(go env GOPATH)/bin
	go install $(GOFLAGS) -ldflags '$(LDFLAGS)' .

run: build ## Build and run; pass CLI args via ARGS="..."
	$(BIN_DIR)/$(BINARY) $(ARGS)

test: ## Run unit tests (MySQL integration tests are skipped without GB_MIGRATION_TEST_DSN)
	go test $(GOFLAGS) -count=1 -race $(PKG)

test-integration: ## Run MySQL integration tests; requires GB_MIGRATION_TEST_DSN
	@[ -n "$(GB_MIGRATION_TEST_DSN)" ] || { echo "GB_MIGRATION_TEST_DSN is required, e.g. root:nopassword@tcp(127.0.0.1:3306)/"; exit 1; }
	GB_MIGRATION_TEST_DSN='$(GB_MIGRATION_TEST_DSN)' go test $(GOFLAGS) -count=1 -race -v $(PKG)

test-all: test-integration ## Alias: unit + integration tests (needs GB_MIGRATION_TEST_DSN)

baseline: ## Capture golden baseline from the pre-change tool; requires GB_MIGRATION_TEST_DSN
	GB_MIGRATION_TEST_DSN='$(GB_MIGRATION_TEST_DSN)' scripts/capture_baseline.sh

fmt: ## Format Go sources
	gofmt -s -w $(GO_FILES)

fmt-check: ## Fail if any Go source is not gofmt-formatted
	@out="$$(gofmt -s -l $(GO_FILES))"; \
	if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

vet: ## Run go vet
	go vet $(PKG)

lint: fmt-check vet ## Static checks: gofmt + go vet (+ golangci-lint if installed)
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run $(PKG); \
	else echo "golangci-lint not installed; skipped"; fi

tidy: ## Tidy go.mod / go.sum
	go mod tidy

deps: ## Download and verify module dependencies
	go mod download
	go mod verify

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) $(BINARY)
	go clean
