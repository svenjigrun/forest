BIN      := ./bin/forest
CMD      := ./cmd/forest
GOFLAGS  :=

.PHONY: build test lint vet clean run ci

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

ci: test vet lint
