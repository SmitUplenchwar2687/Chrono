.PHONY: build test lint coverage bench clean all

BINARY := bin/chrono
MODULE := github.com/SmitUplenchwar2687/Chrono

build:
	go build -o $(BINARY) ./cmd/chrono

test:
	go test ./... -race -count=1

lint:
	golangci-lint run ./...

coverage:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -func=coverage.out
	@rm -f coverage.out

bench:
	go test ./... -bench=. -benchmem -run=^$$

clean:
	rm -rf bin/ coverage.out

all: test build
