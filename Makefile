# InterGold validator — common dev tasks
#
# Run `make help` to see every target. Default target is `ci` (tidy + fmt-check + vet + test).

GO        ?= go
PKG       := ./...
DEMO      := ./cmd/demo
COVERFILE := coverage.out
BIN_DIR   := bin

.DEFAULT_GOAL := ci

## ci: tidy + fmt-check + vet + test (the full reviewer-friendly pipeline)
.PHONY: ci
ci: tidy fmt-check vet test

## all: ci + test-race + bench (full local pre-commit check)
.PHONY: all
all: ci test-race bench

## install-tools: install wire + mockery code generators
.PHONY: install-tools
install-tools:
	$(GO) install github.com/google/wire/cmd/wire@latest
	$(GO) install github.com/vektra/mockery/v2@latest

## generate: regenerate Wire injectors + Mockery mocks
.PHONY: generate
generate:
	wire ./wire
	mockery

## tidy: refresh go.mod / go.sum
.PHONY: tidy
tidy:
	$(GO) mod tidy

## mod-verify: verify dependencies have expected content (supply chain check)
.PHONY: mod-verify
mod-verify:
	$(GO) mod verify

## fmt: rewrite files in place with gofmt
.PHONY: fmt
fmt:
	$(GO) fmt $(PKG)

## fmt-check: fail if any file would be reformatted by gofmt
.PHONY: fmt-check
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt would change these files:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

## vet: run go vet (catches obvious mistakes static analysis can find)
.PHONY: vet
vet:
	$(GO) vet $(PKG)

## build: compile all binaries into ./bin/
.PHONY: build
build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/ $(DEMO)

## test: run unit tests
.PHONY: test
test:
	$(GO) test $(PKG)

## test-v: run unit tests with verbose output
.PHONY: test-v
test-v:
	$(GO) test -v $(PKG)

## test-race: run unit tests under the race detector (slower; catches data races)
.PHONY: test-race
test-race:
	$(GO) test -race $(PKG)

## bench: run benchmarks
.PHONY: bench
bench:
	$(GO) test -bench=. -benchmem -run='^$$' $(PKG)

## cover: run tests and produce an HTML coverage report at coverage.html
.PHONY: cover
cover:
	$(GO) test -coverprofile=$(COVERFILE) $(PKG)
	$(GO) tool cover -html=$(COVERFILE) -o coverage.html
	@echo "Open coverage.html in a browser."

## cover-func: print per-function coverage to stdout (good for CI logs)
.PHONY: cover-func
cover-func:
	$(GO) test -coverprofile=$(COVERFILE) $(PKG)
	$(GO) tool cover -func=$(COVERFILE)

## demo: run the example program (in-memory adapters)
.PHONY: demo
demo:
	$(GO) run $(DEMO)

## seed: create + seed a SQLite database at ./data.db
.PHONY: seed
seed:
	$(GO) run ./cmd/seed -db ./data.db

## demo-sqlite: seed the SQLite database then run the demo against it
.PHONY: demo-sqlite
demo-sqlite: seed
	$(GO) run $(DEMO) -db ./data.db

## server: start the HTTP server on :8080 (in-memory adapters)
.PHONY: server
server:
	$(GO) run ./cmd/server

## server-sqlite: seed then start the HTTP server using GORM/SQLite
.PHONY: server-sqlite
server-sqlite: seed
	$(GO) run ./cmd/server -db ./data.db

## clean: remove build / coverage artifacts, the test cache, and ./data.db
.PHONY: clean
clean:
	$(GO) clean -testcache
	rm -rf $(BIN_DIR) $(COVERFILE) coverage.html data.db

## help: list available targets
.PHONY: help
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed -e 's/## //'
