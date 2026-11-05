BIN    := pg_procrustes
CMD    := ./cmd/pg_procrustes
OUTDIR := build

# Version baked into local builds comes from the VERSION file. `make release`
# / `make prepare-release` ask for the NEW version interactively (or take
# VERSION=X.Y.Z from the command line / environment).
APP_VERSION := $(shell cat VERSION)

LDFLAGS := -ldflags "-s -w -X main.version=$(APP_VERSION)"

.DEFAULT_GOAL := help

.PHONY: builder-templates builder-examples help build test test-update bench lint fmt install clean \
        build-darwin-arm64 build-darwin-amd64 \
        build-linux-amd64 build-linux-arm64 \
        build-windows-amd64 build-windows-arm64 \
        build-all release-local prepare-release release llms

help: ## Show this help
	@echo "pg_procrustes — PostgreSQL SQL formatter"
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ { printf "  %-24s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# ── local build ───────────────────────────────────────────────────────────────

build: builder-templates ## Build binary for the current platform → build/pg_procrustes
	@mkdir -p $(OUTDIR)
	go build $(LDFLAGS) -o $(OUTDIR)/$(BIN) $(CMD)

# The binary finds installed templates in ../share/pg_procrustes/templates (and templates/ next
# to it), so `template: name` works wherever PREFIX points.
PREFIX ?= $(shell go env GOPATH)

install: ## Install the binary and templates/ under PREFIX (default $GOPATH): bin/ and share/pg_procrustes/templates/
	@mkdir -p $(PREFIX)/bin $(PREFIX)/share/pg_procrustes/templates
	go build $(LDFLAGS) -o $(PREFIX)/bin/$(BIN) $(CMD)
	cp templates/*.yaml $(PREFIX)/share/pg_procrustes/templates/

# ── tests / quality ───────────────────────────────────────────────────────────

test: ## Run all tests
	go test ./... -count=1

test-update: ## Regenerate golden want.sql files (run after intentional formatter changes)
	go test ./formatter/... -run TestGolden -update

bench: ## Run benchmarks
	go test ./formatter/... -run '^$$' -bench=. -benchmem

GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null || echo $$(go env GOPATH)/bin/golangci-lint)

lint: ## Run linter (installs golangci-lint into $GOPATH/bin if missing)
	@test -x "$(GOLANGCI_LINT)" || { echo "==> Installing golangci-lint..."; go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest; }
	$(GOLANGCI_LINT) run ./...

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

build-all: builder-templates build-darwin-arm64 build-darwin-amd64 \
           build-linux-amd64 build-linux-arm64 \
           build-windows-amd64 build-windows-arm64 ## Build raw binaries for all platforms → build/

# ── release ───────────────────────────────────────────────────────────────────

prepare-release: ## Ask for the new VERSION, test, roll CHANGELOG, bump VERSION, commit and push (no tag)
	@VERSION="$(VERSION)" scripts/prepare_release.sh

release-local: ## Test, build current platform archive, verify — no git, no push
	@VERSION="$(APP_VERSION)" scripts/release.sh --local

release: clean ## One step: ask for VERSION, test, bump + CHANGELOG, commit, tag, push (CI builds and publishes)
	@VERSION="$(VERSION)" scripts/prepare_release.sh
	@SKIP_TESTS=1 scripts/release.sh

builder-templates: ## Regenerate docs/config-builder-templates.js from templates/*.yaml (add or remove a file there)
	@go run scripts/gen_builder_templates.go

builder-examples: builder-templates ## Regenerate docs/config-builder-examples.js with the real formatter
	@go run scripts/gen_builder_examples.go

llms: ## Regenerate docs/llms-full.txt from README.md and .pg_procrustes.yaml
	@scripts/gen_llms_full.sh

# ── cleanup ───────────────────────────────────────────────────────────────────

clean: ## Remove build and dist directories
	rm -rf $(OUTDIR) dist
