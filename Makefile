BIN    := pg_procrustes
CMD    := ./cmd/pg_procrustes
OUTDIR := build

VERSION ?= $(shell cat VERSION)

LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.DEFAULT_GOAL := help

.PHONY: help build test test-update bench lint fmt install clean \
        build-darwin-arm64 build-darwin-amd64 \
        build-linux-amd64 build-linux-arm64 \
        build-windows-amd64 build-windows-arm64 \
        build-all release-local release

help: ## Show this help
	@echo "pg_procrustes — PostgreSQL SQL formatter"
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ { printf "  %-24s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ── local build ───────────────────────────────────────────────────────────────

build: ## Build binary for the current platform → build/pg_procrustes
	@mkdir -p $(OUTDIR)
	go build $(LDFLAGS) -o $(OUTDIR)/$(BIN) $(CMD)

install: ## Install binary to $GOPATH/bin
	go install $(LDFLAGS) $(CMD)

# ── tests / quality ───────────────────────────────────────────────────────────

test: ## Run all tests
	go test ./... -count=1

test-update: ## Regenerate golden want.sql files (run after intentional formatter changes)
	go test ./internal/formatter/... -run TestGolden -update

bench: ## Run benchmarks
	go test ./internal/formatter/... -run '^$$' -bench=. -benchmem

lint: ## Run linter (requires golangci-lint)
	golangci-lint run ./...

fmt: ## Format Go source files
	gofmt -w .

# ── cross-compilation ─────────────────────────────────────────────────────────

build-darwin-arm64: ## macOS Apple Silicon  → build/pg_procrustes-darwin-arm64
	@mkdir -p $(OUTDIR)
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o $(OUTDIR)/$(BIN)-darwin-arm64      $(CMD)

build-darwin-amd64: ## macOS Intel          → build/pg_procrustes-darwin-amd64
	@mkdir -p $(OUTDIR)
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o $(OUTDIR)/$(BIN)-darwin-amd64      $(CMD)

build-linux-amd64: ## Linux x86-64          → build/pg_procrustes-linux-amd64
	@mkdir -p $(OUTDIR)
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o $(OUTDIR)/$(BIN)-linux-amd64       $(CMD)

build-linux-arm64: ## Linux ARM64           → build/pg_procrustes-linux-arm64
	@mkdir -p $(OUTDIR)
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o $(OUTDIR)/$(BIN)-linux-arm64       $(CMD)

build-windows-amd64: ## Windows x86-64      → build/pg_procrustes-windows-amd64.exe
	@mkdir -p $(OUTDIR)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(OUTDIR)/$(BIN)-windows-amd64.exe $(CMD)

build-windows-arm64: ## Windows ARM64       → build/pg_procrustes-windows-arm64.exe
	@mkdir -p $(OUTDIR)
	GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o $(OUTDIR)/$(BIN)-windows-arm64.exe $(CMD)

build-all: build-darwin-arm64 build-darwin-amd64 \
           build-linux-amd64 build-linux-arm64 \
           build-windows-amd64 build-windows-arm64 ## Build raw binaries for all platforms → build/

# ── release ───────────────────────────────────────────────────────────────────

release-local: ## Test, build current platform archive, verify — no git, no push
	@scripts/release.sh --local

release: ## Tag and push — GitHub Actions builds all platforms, releases, updates tap
	@scripts/release.sh --github

# ── cleanup ───────────────────────────────────────────────────────────────────

clean: ## Remove build and dist directories
	rm -rf $(OUTDIR) dist
