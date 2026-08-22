SHELL := /bin/bash

# Default package for tests. Override with: make test PKG=./...
PKG ?= ./...

# Formatter orchestrator (see treefmt.toml): gofmt/goimports for Go,
# dprint for markdown, shfmt for bash. Override with: make format TREEFMT=dprint
TREEFMT ?= treefmt

.PHONY: help
help:
	@echo "Available targets:"
	@echo "  make cover         - Generate coverage report"
	@echo "  make format        - Format everything via treefmt (gofmt, goimports, dprint, shfmt)"
	@echo "  make format-check  - Check formatting without writing"
	@echo "  make generate      - Rebuild wasm and regenerate Go bindings"
	@echo "  make lint          - Run golangci-lint"
	@echo "  make test          - Run tests for $(PKG)"
	@echo "  make test-race     - Run tests with race detector"
	@echo "  make wasm          - Compile wasm/shim.c → wasm/wuffs.wasm"
	@echo ""
	@echo "Environment variables:"
	@echo "  PKG=<path>         - Override package (default: $(PKG))"
	@echo "  ARGS=<flags>       - Extra flags passed to go test (e.g. -v -count=1)"

.PHONY: wasm
wasm:
	bash wasm/build.sh

.PHONY: generate
generate: wasm
	wasm2go -unsafe -pkg wuffswasm -o internal/wuffswasm/wuffs.go wasm/wuffs.wasm

.PHONY: test
test:
	# Extra go test flags can be passed via ARGS: make test ARGS="-count=1 -v"
	time go test $(PKG) $(ARGS)

.PHONY: test-race
test-race:
	# Run the root wuffs package tests with the race detector, scoped to the
	# package that owns the Decoder concurrency contract.
	time go test -race . $(ARGS)

.PHONY: lint
lint:
	# Run golangci-lint across all packages
	golangci-lint run --max-same-issues 0 ./...

.PHONY: cover
cover:
	# Generate coverage profile and HTML report
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out | tail -n 1
	@echo "Coverage report: coverage.html"

.PHONY: format fmt
format fmt:
	# Format everything via treefmt (gofmt+goimports for Go, dprint for markdown, shfmt for bash)
	$(TREEFMT)

.PHONY: format-check fmt-check
format-check fmt-check:
	# Fail if any file needs reformatting
	$(TREEFMT) --fail-on-change
