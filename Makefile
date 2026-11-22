SHELL := /bin/bash

# Default package for tests. Override with: make test PKG=./...
PKG ?= ./...

# Formatter orchestrator (see treefmt.toml): gofmt/goimports for Go,
# dprint for markdown, shfmt for bash. Override with: make format TREEFMT=dprint
TREEFMT ?= treefmt

.PHONY: bench-fixtures help
help:
	@echo "Available targets:"
	@echo "  make cover         - Generate coverage report (no fuzz tag)"
	@echo "  make format        - Format everything via treefmt (gofmt, goimports, dprint, shfmt)"
	@echo "  make format-check  - Check formatting without writing"
	@echo "  make generate      - Rebuild wasm and regenerate Go bindings"
	@echo "  make lint          - Run golangci-lint and clang-tidy (wasm/shim.c)"
	@echo "  make test          - Run tests for $(PKG) (no fuzz tag)"
	@echo "  make test-all      - Run tests, Fuzz* seeds, and Linux RSS tests (-tags 'fuzz rss')"
	@echo "  make test-race     - Run tests with race detector (no fuzz tag)"
	@echo "  make fuzz          - Run native Go fuzz mutation (-tags fuzz, no Test*)"
	@echo "  make bench-fixtures - Download M/L benchmark fixtures (network)"
	@echo "  make wasm          - Compile wasm/shim.c → wasm/wuffs.wasm"
	@echo ""
	@echo "Environment variables:"
	@echo "  PKG=<path>         - Override package (default: $(PKG))"
	@echo "  ARGS=<flags>       - Extra flags passed to go test (e.g. -v -count=1)"
	@echo "  FUZZ=<target>      - Fuzz target (default: $(FUZZ))"
	@echo "  FUZZTIME=<d>       - Fuzz duration (default: $(FUZZTIME))"

.PHONY: bench-fixtures
bench-fixtures:
	bash scripts/fetch_bench_fixtures.sh

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

.PHONY: test-all
test-all:
	time go test $(PKG) -tags "fuzz rss" $(ARGS)

.PHONY: test-race
test-race:
	# Run the root wuffs package tests with the race detector, scoped to the
	# package that owns the Decoder concurrency contract.
	time go test -race . $(ARGS)

# Native Go fuzz (mutation). fuzz_test.go is behind //go:build fuzz.
FUZZ ?= FuzzDecodeRGBA
FUZZTIME ?= 30s

.PHONY: fuzz
fuzz:
	time go test -tags fuzz -run=^$$ -fuzz=$(FUZZ) -fuzztime=$(FUZZTIME) -parallel=4 .

# wasm/shim.c includes the Wuffs C release from a sibling checkout.
WUFFS_RELEASE_C ?= ../wuffs-mirror-release-c/release/c/wuffs-v0.4.c

.PHONY: lint
lint:
	# Run golangci-lint across all packages
	golangci-lint run --max-same-issues 0 ./...
	# Run clang-tidy on wasm/shim.c (checks in .clang-tidy) with the pinned
	# wasi-sdk, for the wasm32 target. WUFFS_WASM_LINT parses Wuffs declarations
	# only, so the analyzer never reports inside the Wuffs release.
	@test -f $(WUFFS_RELEASE_C) || { echo "lint: missing $(WUFFS_RELEASE_C); check out wuffs-mirror-release-c beside this repository"; exit 1; }
	@sdk="$$(bash wasm/wasi-sdk.sh)" && set -x && \
		"$$sdk/bin/clang-tidy" --quiet wasm/shim.c -- --target=wasm32-wasip1 \
		--sysroot="$$sdk/share/wasi-sysroot" -I wasm -std=c11 -DWUFFS_WASM_LINT

.PHONY: cover
cover:
	# Generate coverage profile and HTML report
	time go test ./... -coverprofile=coverage.out
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
