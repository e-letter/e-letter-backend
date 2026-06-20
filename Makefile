# Backend E-Letter Makefile

.PHONY: build tidy run test

build:
	go build ./...

tidy:
	go mod tidy

run:
	go run ./cmd/api

test:
	go test ./...
