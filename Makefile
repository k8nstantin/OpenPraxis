.PHONY: build frontend dev-frontend clean run test test-ui help build-all types types-check storybook build-storybook

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

# Install npm deps if missing or package.json newer. The explicit touch
# on the directory keeps mtime comparisons reliable across filesystems
# where directory mtimes don't bump on every nested write.
$(DASHBOARD_NM): $(DASHBOARD_DIR)/package.json
	@echo "  npm install (dashboard)…"
	cd $(DASHBOARD_DIR) && npm install
	@touch $(DASHBOARD_NM)

# Real React build. Vite handles its own incremental caching, so we
# always invoke it — make doesn't try to track every src/**/*.tsx file.
frontend: $(DASHBOARD_NM)
	@echo "  npm run build (dashboard)…"
	cd $(DASHBOARD_DIR) && npm run build

# Vite HMR dev server for frontend work alongside `make run`. Run in a
# separate terminal — the dev server proxies /api/* to the Go server.
dev-frontend: $(DASHBOARD_NM)
	cd $(DASHBOARD_DIR) && npm run dev

# Regenerate TypeScript types from Go structs via tygo. Output lives in
# internal/web/ui/dashboard/src/lib/types.gen.*.ts and is committed —
# `types-check` re-runs the generator and fails CI if anything drifted.
types:
	@echo "  tygo generate…"
	go run github.com/gzuidhof/tygo@v0.2.16 generate

types-check:
	@echo "  tygo drift check…"
	@go run github.com/gzuidhof/tygo@v0.2.16 generate
	@if [ -n "$$(git status --porcelain $(DASHBOARD_DIR)/src/lib/types.gen.*.ts)" ]; then \
		echo "ERROR: generated TS types drifted from Go. Run 'make types' and commit the diff."; \
		git --no-pager diff $(DASHBOARD_DIR)/src/lib/types.gen.*.ts; \
		exit 1; \
	fi

# Storybook — dev-only component playground. Not embedded in the
# binary; storybook-static/ is gitignored.
storybook: $(DASHBOARD_NM)
	cd $(DASHBOARD_DIR) && npm run storybook

build-storybook: $(DASHBOARD_NM)
	cd $(DASHBOARD_DIR) && npm run build-storybook

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

help:
	@echo "  build         - Build the binary (chains npm install + npm run build)"
	@echo "  clean         - Remove built binaries + dashboard hashed assets"
	@echo "  run           - Build and run the server"
	@echo "  test          - Run all tests (Go + UI)"
	@echo "  test-ui       - Run UI tests only (legacy JS + React vitest)"
	@echo "  frontend      - Build the React dashboard only"
	@echo "  dev-frontend  - Run vite HMR dev server (use alongside 'make run')"
	@echo "  build-all     - Cross-compile for darwin/linux"
