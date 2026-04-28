.PHONY: build frontend dev-frontend clean run test test-ui help build-all types storybook

VERSION ?= 0.4.1
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -ldflags "-X github.com/k8nstantin/OpenPraxis/cmd.Version=$(VERSION) \
	-X github.com/k8nstantin/OpenPraxis/cmd.GitCommit=$(GIT_COMMIT) \
	-X github.com/k8nstantin/OpenPraxis/cmd.BuildDate=$(BUILD_DATE)"

# React v2 dashboard build pipeline. The Go binary embeds dist/ via
# `go:embed all:ui/dashboard/dist`, so the frontend MUST build BEFORE
# `go build` — otherwise we ship the committed stub instead of the real
# React app.
DASHBOARD_DIR := internal/web/ui/dashboard
DASHBOARD_NM  := $(DASHBOARD_DIR)/node_modules

# Portal V2 dashboard (operator redesign on shadcn-admin) — same shape,
# separate tree. Shipped on :9766 alongside Portal A on :8765 via the
# `--portal-v2-port` serve flag. go:embed pulls dist/ into the binary.
DASHBOARDV2_DIR := internal/web/ui/dashboard-v2
DASHBOARDV2_NM  := $(DASHBOARDV2_DIR)/node_modules

# Install npm deps if missing or package.json newer. The explicit touch
# on the directory keeps mtime comparisons reliable across filesystems
# where directory mtimes don't bump on every nested write.
$(DASHBOARD_NM): $(DASHBOARD_DIR)/package.json
	@echo "  npm install (dashboard)…"
	cd $(DASHBOARD_DIR) && npm install
	@touch $(DASHBOARD_NM)

$(DASHBOARDV2_NM): $(DASHBOARDV2_DIR)/package.json
	@echo "  npm install (dashboard-v2)…"
	cd $(DASHBOARDV2_DIR) && npm install
	@touch $(DASHBOARDV2_NM)

# Real React build. Vite handles its own incremental caching, so we
# always invoke it — make doesn't try to track every src/**/*.tsx file.
frontend: $(DASHBOARD_NM) $(DASHBOARDV2_NM)
	@echo "  npm run build (dashboard)…"
	cd $(DASHBOARD_DIR) && npm run build
	@echo "  npm run build (dashboard-v2)…"
	cd $(DASHBOARDV2_DIR) && npm run build

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
	rm -rf $(DASHBOARDV2_DIR)/dist/assets
	rm -rf $(DASHBOARDV2_DIR)/dist/images

run: build
	./openpraxis serve

test: test-ui
	go test -v ./...

# UI tests — vanilla JS suite (Node-only, no npm deps; see
# dag-renderer-recurring-failures.md for why these exist) PLUS the
# React vitest suite for the dashboard v2.
test-ui: $(DASHBOARD_NM)
	@for f in internal/web/ui/views/__tests__/*.test.js; do \
		echo "  ui (legacy): $$f"; \
		node "$$f" || exit 1; \
	done
	@echo "  ui (react): vitest"
	cd $(DASHBOARD_DIR) && npm test

# Cross-compilation. The React bundle is platform-agnostic so it
# builds once and gets embedded into all three Go targets.
build-all: clean frontend
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o openpraxis-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o openpraxis-darwin-amd64 .
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o openpraxis-linux-amd64 .
	@if [ "$$(uname)" = "Darwin" ]; then \
		codesign --force --sign - openpraxis-darwin-arm64 && echo "  codesigned: openpraxis-darwin-arm64"; \
		codesign --force --sign - openpraxis-darwin-amd64 && echo "  codesigned: openpraxis-darwin-amd64"; \
	fi

# Generate TypeScript types from Go structs so the dashboard's typed API
# client and Go's HTTP handlers can never drift. Uses `tygo`; the config
# at tools/tygo/config.yaml maps internal/* packages to a single emitted
# .ts file consumed by src/lib/api/index.ts. CI runs this then a `git
# diff --exit-code` so an out-of-date generated file fails the pipeline.
types:
	@command -v tygo >/dev/null 2>&1 || { echo "tygo not installed — run: go install github.com/gzuidhof/tygo@latest"; exit 1; }
	tygo generate --config tools/tygo/config.yaml
	@echo "  types generated → $(DASHBOARD_DIR)/src/lib/api/generated.ts"

# CI gate: regenerate types and fail if the working tree drifts. Catches
# Go struct changes that weren't paired with a `make types` commit.
types-check: types
	@git diff --exit-code -- $(DASHBOARD_DIR)/src/lib/api/generated.ts \
		|| { echo "ERROR: generated.ts is stale — run 'make types' and commit"; exit 1; }
	@echo "  types-check ok"

# Storybook dev server. Dev-only — Storybook is NOT bundled into the Go
# binary; this is the operator-facing review surface for primitives +
# chrome + cross-cutting components.
storybook: $(DASHBOARD_NM)
	cd $(DASHBOARD_DIR) && npm run storybook

help:
	@echo "  build         - Build the binary (chains npm install + npm run build)"
	@echo "  clean         - Remove built binaries + dashboard hashed assets"
	@echo "  run           - Build and run the server"
	@echo "  test          - Run all tests (Go + UI)"
	@echo "  test-ui       - Run UI tests only (legacy JS + React vitest)"
	@echo "  frontend      - Build the React dashboard only"
	@echo "  dev-frontend  - Run vite HMR dev server (use alongside 'make run')"
	@echo "  storybook     - Run Storybook dev server (primitives review)"
	@echo "  types         - Regenerate TS types from Go structs (tygo)"
	@echo "  build-all     - Cross-compile for darwin/linux"
