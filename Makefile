BINARY := ozy
PKG := ./...
GOLANGCI_LINT_VERSION := v2.12.2
GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null || echo $(shell go env GOPATH)/bin/golangci-lint)
OZY_EXAMPLE_CATALOG ?= /tmp/ozy-example-catalog.json

.DEFAULT_GOAL := build

.PHONY: build
build: ## Build the ozy binary
	go build -o $(BINARY) ./cmd/ozy

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
	$(GOLANGCI_LINT) run

.PHONY: fmt
fmt: ## Apply formatters (gofmt, goimports)
	$(GOLANGCI_LINT) fmt

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
