BINARY := ozy
BENCH_BINARY := ozy-bench
PKG := ./...
GOLANGCI_LINT_VERSION := v2.12.2
GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null || echo $(shell go env GOPATH)/bin/golangci-lint)
OZY_EXAMPLE_CATALOG ?= /tmp/ozy-example-catalog.json
# Prefer the compose v2 plugin; fall back to the standalone docker-compose.
COMPOSE := $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")

.DEFAULT_GOAL := build

.PHONY: build
build: ## Build the ozy binary
	go build -o $(BINARY) ./cmd/ozy

.PHONY: bench-build
bench-build: ## Build the ozy-bench binary
	go build -o $(BENCH_BINARY) ./cmd/ozy-bench

.PHONY: bench
bench: ## Run the benchmark in Docker (all modes, semantic, 5 runs). Set runs: `make bench 10`.
	@BENCH_RUNS=$(or $(filter-out bench,$(MAKECMDGOALS)),$${BENCH_RUNS:-5}) $(COMPOSE) -f bench/docker-compose.yml up --build --exit-code-from bench-runner

# `make bench 10` passes the run count as an extra goal; swallow it as a no-op
# (only when bench is invoked) so make doesn't look for a target named "10".
ifneq ($(filter bench,$(MAKECMDGOALS)),)
$(filter-out bench,$(MAKECMDGOALS)):
	@:
endif

.PHONY: bench-surface
bench-surface: bench-build ## Compute the static surface tier natively — no Docker, no model
	cd bench && ../$(BENCH_BINARY) run --surface-only

.PHONY: bench-clean
bench-clean: ## Prune local bench run artifacts
	rm -rf bench/runs/*/

.PHONY: test
test: ## Run the test suite
	go test $(PKG)

.PHONY: check-real-mcp-examples
check-real-mcp-examples: ## Opt-in check against examples/test_mcp_examples.jsonc
	@if [ "$$OZY_RUN_REAL_MCP_EXAMPLES" != "1" ]; then \
		echo "skipping real MCP example check; set OZY_RUN_REAL_MCP_EXAMPLES=1"; \
		exit 0; \
	fi; \
	$(MAKE) build; \
	OZY_CATALOG="$(OZY_EXAMPLE_CATALOG)" ./$(BINARY) --config examples/test_mcp_examples.jsonc --format json index; \
	OZY_CATALOG="$(OZY_EXAMPLE_CATALOG)" ./$(BINARY) --config examples/test_mcp_examples.jsonc --format json list

.PHONY: lint
lint: ## Run golangci-lint (vet, staticcheck, gosec, formatting, ...)
	@if [ ! -x "$(GOLANGCI_LINT)" ]; then \
		echo "golangci-lint not found; run 'make tools'"; \
		exit 1; \
	fi
	$(GOLANGCI_LINT) run ./...

.PHONY: fmt
fmt: ## Apply formatters (gofmt, goimports)
	$(GOLANGCI_LINT) fmt

.PHONY: install-hooks
install-hooks: ## Use the tracked Git hooks in .githooks
	git config core.hooksPath .githooks

.PHONY: tools
tools: ## Install the pinned golangci-lint
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: tidy
tidy: ## Ensure go.mod/go.sum are tidy
	go mod tidy

.PHONY: clean
clean: ## Remove build artifacts
	rm -f $(BINARY)

.PHONY: help
help: ## Show available targets
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'
