.PHONY: run build test test-race vet lint fmt ci clean

VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS   := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(BUILD_DATE)

run:
	go run ./cmd/gateway --config config.yaml

build:
	go build -ldflags "$(LDFLAGS)" -o bin/gateway ./cmd/gateway

test:
	go test ./...

test-race:
	go test -race -coverprofile=coverage.out ./...

vet:
	go vet ./...

lint:
	@test -z "$$(gofmt -l $$(find . -name '*.go' -type f))"

fmt:
	gofmt -w $$(find . -name '*.go' -type f)

ci: lint vet test-race build

clean:
	rm -rf bin coverage.out
