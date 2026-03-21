.PHONY: run build test test-race vet lint fmt ci clean

run:
	go run ./cmd/gateway --config config.yaml

build:
	go build -o bin/gateway ./cmd/gateway

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
