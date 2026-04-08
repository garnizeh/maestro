BINARY     := maestro
MODULE     := github.com/rodrigo-baliza/maestro
CMD        := ./cmd/maestro

# Version info injected at build time via ldflags
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE       ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GO_VERSION ?= $(shell go version | cut -d' ' -f3)

LDFLAGS := -X $(MODULE)/internal/cli.Version=$(VERSION) \
           -X $(MODULE)/internal/cli.Commit=$(COMMIT) \
           -X $(MODULE)/internal/cli.BuildDate=$(DATE) \
           -X $(MODULE)/internal/cli.GoVersion=$(GO_VERSION)

STATIC_LDFLAGS := $(LDFLAGS) -w -s -extldflags "-static"

.PHONY: help build build-static install test test-integration test-e2e \
        lint vuln fmt clean generate completions ci-local

help: ## Display available targets
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} \
	  /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: ## Build the maestro binary
	go build -ldflags "$(LDFLAGS)" -o ./bin/$(BINARY) $(CMD)

build-static: ## Build a fully static binary (CGO_ENABLED=0)
	CGO_ENABLED=0 go build -ldflags "$(STATIC_LDFLAGS)" -o ./bin/$(BINARY)-static $(CMD)

install: build ## Install maestro to $GOPATH/bin
	cp ./bin/$(BINARY) $(shell go env GOPATH)/bin/$(BINARY)

test: ## Run unit tests with race detector
	go test -race -count=1 -cover ./internal/...

test-integration: ## Run integration tests (requires OCI runtime)
	go test -race -count=1 -tags integration ./test/integration/...

test-e2e: ## Run end-to-end tests (requires full environment)
	go test -race -count=1 -tags e2e ./test/e2e/...

lint: ## Run golangci-lint
	golangci-lint run ./...

vuln: ## Run govulncheck
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

fmt: ## Format code with gofmt
	gofmt -w -s .
	go fix ./...

clean: ## Remove build artifacts
	rm -rf ./bin/
	go clean -cache -testcache

generate: ## Run go generate
	go generate ./...

ci-local: ## Run the same checks as CI (lint → vuln → test → build → smoke). Must pass before opening a PR.
	@echo "==> lint config verify"
	golangci-lint config verify
	@echo "==> lint"
	$(MAKE) lint
	@echo "==> vuln"
	$(MAKE) vuln
	@echo "==> test"
	$(MAKE) test
	@echo "==> build"
	$(MAKE) build
	@echo "==> build-static"
	$(MAKE) build-static
	@echo "==> smoke"
	./bin/$(BINARY) version
	@echo "==> OK — all CI checks passed locally"

completions: build ## Generate shell completions to ./completions/
	mkdir -p completions
	./bin/$(BINARY) generate completions bash  > completions/$(BINARY).bash
	./bin/$(BINARY) generate completions zsh   > completions/_$(BINARY)
	./bin/$(BINARY) generate completions fish  > completions/$(BINARY).fish
