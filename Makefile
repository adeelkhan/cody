BINARY := cody

.PHONY: build test

build:
	go build -o $(BINARY) ./cmd/cody

test:
	go test ./...
