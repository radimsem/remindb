.PHONY: build run test fmt lint tidy

build:
	go build ./...

run:
	go run ./cmd/remindb

test:
	go test ./...

fmt:
	gofmt -w .

lint:
	golangci-lint run ./...

tidy:
	go mod tidy
