.PHONY: build run test fuzz test-all fmt lint tidy

build:
	go build ./...

run:
	go run ./cmd/remindb

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
