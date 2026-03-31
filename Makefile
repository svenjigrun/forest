BIN        := ./bin/forest
CMD        := ./cmd/forest
GOFLAGS    :=
FOREST_DIR ?= ./testforest

.PHONY: build test lint vet clean run reindex embed publish ci

build:
	go build $(GOFLAGS) -o $(BIN) $(CMD)

test:
	go test ./...

lint:
	golangci-lint run

vet:
	go vet ./...

clean:
	rm -rf ./bin

run: build
	$(BIN)

reindex: build
	$(BIN) reindex $(FOREST_DIR)

embed: build
	$(BIN) embed $(FOREST_DIR)

publish: build
	$(BIN) publish --output ./public $(FOREST_DIR)

ci: test vet lint
