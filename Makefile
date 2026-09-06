BINARY  := bin/api
PKG     := github.com/casbek/api-swiss-ephemeris
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(PKG)/internal/version.Version=$(VERSION)

# cgo is required: the Swiss Ephemeris sources are compiled into the binary.
export CGO_ENABLED = 1

.PHONY: all build run test race bench vet fmt check clean

all: check build

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/api

run:
	go run ./cmd/api

test:
	go test ./... -count=1

race:
	go test ./... -count=1 -race

bench:
	go test ./internal/swe -run='^$$' -bench=. -benchtime=2s

vet:
	go vet ./...

fmt:
	gofmt -w $(shell git ls-files '*.go')

# Everything CI runs.
check: vet test race

clean:
	rm -rf bin
