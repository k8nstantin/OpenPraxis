.PHONY: build clean run test help

VERSION ?= 0.4.0
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -ldflags "-X github.com/k8nstantin/OpenPraxis/cmd.Version=$(VERSION) \
	-X github.com/k8nstantin/OpenPraxis/cmd.GitCommit=$(GIT_COMMIT) \
	-X github.com/k8nstantin/OpenPraxis/cmd.BuildDate=$(BUILD_DATE)"

build:
	go mod tidy
	go build $(LDFLAGS) -o openpraxis .
	@if [ "$$(uname)" = "Darwin" ]; then codesign --force --sign - openpraxis && echo "  codesigned: openpraxis (ad-hoc)"; fi

clean:
	rm -f openpraxis

run: build
	./openpraxis serve

test:
	go test -v ./...

# Cross-compilation
build-all: clean
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o openpraxis-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o openpraxis-darwin-amd64 .
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o openpraxis-linux-amd64 .
	@if [ "$$(uname)" = "Darwin" ]; then \
		codesign --force --sign - openpraxis-darwin-arm64 && echo "  codesigned: openpraxis-darwin-arm64"; \
		codesign --force --sign - openpraxis-darwin-amd64 && echo "  codesigned: openpraxis-darwin-amd64"; \
	fi

help:
	@echo "  build     - Build the binary"
	@echo "  clean     - Remove built binaries"
	@echo "  run       - Build and run the server"
	@echo "  test      - Run all tests"
	@echo "  build-all - Cross-compile for darwin/linux"
