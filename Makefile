BIN  := dashboard
GO   := go

.PHONY: all build run test vet

all: build

build:
	$(GO) build -o bin/$(BIN) ./cmd/dashboard

run: build
	./bin/$(BIN)

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...
