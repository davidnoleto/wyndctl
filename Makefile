BINARY     := wyndctl
BIN_DIR    := bin
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_SHA    := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
GIT_DIRTY  := $(shell git diff --quiet 2>/dev/null || echo dirty)
VERSION    := $(shell git describe --tags --always 2>/dev/null || echo dev)
MAX_AGE    := 90

LDFLAGS := -s -w \
  -X main.version=$(VERSION) \
  -X main.buildTime=$(BUILD_TIME) \
  -X main.gitSHA=$(GIT_SHA) \
  -X main.gitDirty=$(GIT_DIRTY) \
  -X main.maxAgeDays=$(MAX_AGE)

.PHONY: build install test test-short lint build-all clean

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) .

install:
	go install -ldflags "$(LDFLAGS)" .

test:
	go test ./... -v -race

test-short:
	go test ./... -short

lint:
	golangci-lint run ./...

build-all:
	GOOS=linux  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-linux-amd64 .
	GOOS=linux  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY)-darwin-arm64 .

clean:
	rm -rf $(BIN_DIR)
