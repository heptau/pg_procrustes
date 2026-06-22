BIN    := pg_procrustes
CMD    := ./cmd/pg_procrustes
OUTDIR := build

VERSION ?= $(shell cat VERSION)

LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.DEFAULT_GOAL := help

.PHONY: help build test lint fmt install clean release \
        build-darwin-arm64 build-darwin-amd64 \
        build-linux-amd64 build-linux-arm64 \
        build-windows-amd64

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

lint: ## Run linter (requires golangci-lint)
	golangci-lint run ./...

fmt: ## Format Go source files
	gofmt -w .

# ── cross-compilation ─────────────────────────────────────────────────────────

build-darwin-arm64: ## macOS Apple Silicon  → build/pg_procrustes-darwin-arm64
	@mkdir -p $(OUTDIR)
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o $(OUTDIR)/$(BIN)-darwin-arm64   $(CMD)

build-darwin-amd64: ## macOS Intel          → build/pg_procrustes-darwin-amd64
	@mkdir -p $(OUTDIR)
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o $(OUTDIR)/$(BIN)-darwin-amd64   $(CMD)

build-linux-amd64: ## Linux x86-64          → build/pg_procrustes-linux-amd64
	@mkdir -p $(OUTDIR)
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o $(OUTDIR)/$(BIN)-linux-amd64    $(CMD)

build-linux-arm64: ## Linux ARM64           → build/pg_procrustes-linux-arm64
	@mkdir -p $(OUTDIR)
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o $(OUTDIR)/$(BIN)-linux-arm64    $(CMD)

build-windows-amd64: ## Windows x86-64      → build/pg_procrustes-windows-amd64.exe
	@mkdir -p $(OUTDIR)
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(OUTDIR)/$(BIN)-windows-amd64.exe $(CMD)

release: build-darwin-arm64 build-darwin-amd64 build-linux-amd64 build-linux-arm64 build-windows-amd64 ## Build binaries for all platforms

# ── cleanup ───────────────────────────────────────────────────────────────────

clean: ## Remove build directory
	rm -rf $(OUTDIR)
