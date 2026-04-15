.PHONY: build clean run test help

VERSION ?= 0.1.0
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -ldflags "-X openloom/cmd.Version=$(VERSION) \
	-X openloom/cmd.GitCommit=$(GIT_COMMIT) \
	-X openloom/cmd.BuildDate=$(BUILD_DATE)"

build:
	go mod tidy
	go build $(LDFLAGS) -o openloom .
	@if [ "$$(uname)" = "Darwin" ]; then codesign --force --sign - openloom && echo "  codesigned: openloom (ad-hoc)"; fi

clean:
	rm -f openloom

run: build
	./openloom serve

test:
	go test -v ./...

# Cross-compilation
build-all: clean
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o openloom-darwin-arm64 .
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o openloom-darwin-amd64 .
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o openloom-linux-amd64 .
	@if [ "$$(uname)" = "Darwin" ]; then \
		codesign --force --sign - openloom-darwin-arm64 && echo "  codesigned: openloom-darwin-arm64"; \
		codesign --force --sign - openloom-darwin-amd64 && echo "  codesigned: openloom-darwin-amd64"; \
	fi

help:
	@echo "  build     - Build the binary"
	@echo "  clean     - Remove built binaries"
	@echo "  run       - Build and run the server"
	@echo "  test      - Run all tests"
	@echo "  build-all - Cross-compile for darwin/linux"
