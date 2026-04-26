.PHONY: build frontend dev-frontend clean run test test-ui help build-all

VERSION ?= 0.4.1
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -ldflags "-X github.com/k8nstantin/OpenPraxis/cmd.Version=$(VERSION) \
	-X github.com/k8nstantin/OpenPraxis/cmd.GitCommit=$(GIT_COMMIT) \
	-X github.com/k8nstantin/OpenPraxis/cmd.BuildDate=$(BUILD_DATE)"

# Svelte v2 dashboard build pipeline. The Go binary embeds dist/ via
# `go:embed all:ui/dashboard/dist`, so the frontend MUST build BEFORE
# `go build` — otherwise we ship the chunk-1 stub instead of the real
# Svelte app.
DASHBOARD_DIR := internal/web/ui/dashboard
DASHBOARD_NM  := $(DASHBOARD_DIR)/node_modules

# Install npm deps if missing or package.json newer. The explicit touch
# on the directory keeps mtime comparisons reliable across filesystems
# where directory mtimes don't bump on every nested write.
$(DASHBOARD_NM): $(DASHBOARD_DIR)/package.json
	@echo "  npm install (dashboard)…"
	cd $(DASHBOARD_DIR) && npm install
	@touch $(DASHBOARD_NM)

# Real Svelte build. Vite handles its own incremental caching, so we
# always invoke it — make doesn't try to track every src/**/*.svelte file.
frontend: $(DASHBOARD_NM)
	@echo "  npm run build (dashboard)…"
	cd $(DASHBOARD_DIR) && npm run build

# Vite HMR dev server for frontend work alongside `make run`. Run in a
# separate terminal — the dev server proxies /api/* to the Go server.
dev-frontend: $(DASHBOARD_NM)
	cd $(DASHBOARD_DIR) && npm run dev

build: frontend
	go mod tidy
	go build $(LDFLAGS) -o openpraxis .
	@if [ "$$(uname)" = "Darwin" ]; then codesign --force --sign - openpraxis && echo "  codesigned: openpraxis (ad-hoc)"; fi

# `clean` removes the binary and the hashed Vite output but keeps the
# committed stub at dist/index.html so go:embed always has something
# to embed even right after `make clean`.
clean:
	rm -f openpraxis
	rm -rf $(DASHBOARD_DIR)/dist/assets

run: build
	./openpraxis serve

test: test-ui
	go test -v ./...

# UI tests — vanilla JS suite (Node-only, no npm deps; see
# dag-renderer-recurring-failures.md for why these exist) PLUS the
# Svelte vitest suite for the dashboard v2.
test-ui: $(DASHBOARD_NM)
	@for f in internal/web/ui/views/__tests__/*.test.js; do \
		echo "  ui (legacy): $$f"; \
		node "$$f" || exit 1; \
	done
	@echo "  ui (svelte): vitest"
	cd $(DASHBOARD_DIR) && npm test

# Cross-compilation. The Svelte bundle is platform-agnostic so it
# builds once and gets embedded into all three Go targets.
build-all: clean frontend
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o openpraxis-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o openpraxis-darwin-amd64 .
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o openpraxis-linux-amd64 .
	@if [ "$$(uname)" = "Darwin" ]; then \
		codesign --force --sign - openpraxis-darwin-arm64 && echo "  codesigned: openpraxis-darwin-arm64"; \
		codesign --force --sign - openpraxis-darwin-amd64 && echo "  codesigned: openpraxis-darwin-amd64"; \
	fi

help:
	@echo "  build         - Build the binary (chains npm install + npm run build)"
	@echo "  clean         - Remove built binaries + dashboard hashed assets"
	@echo "  run           - Build and run the server"
	@echo "  test          - Run all tests (Go + UI)"
	@echo "  test-ui       - Run UI tests only (legacy JS + Svelte vitest)"
	@echo "  frontend      - Build the Svelte dashboard only"
	@echo "  dev-frontend  - Run vite HMR dev server (use alongside 'make run')"
	@echo "  build-all     - Cross-compile for darwin/linux"
