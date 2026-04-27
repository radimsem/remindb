.PHONY: build run test fuzz test-all fmt lint tidy

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" ./...

run:
	go run -ldflags "$(LDFLAGS)" ./cmd/remindb

test:
	go test ./...

fuzz:
	./scripts/fuzz.sh

test-all:
	./scripts/test.sh

fmt:
	gofmt -w .

lint:
	golangci-lint run ./...

tidy:
	go mod tidy
