.PHONY: install ci lint test dev seed wire mock all

# Install every tool the other targets need.
install:
	go install github.com/google/wire/cmd/wire@latest
	go install github.com/vektra/mockery/v2@latest
	go install github.com/bokwoon95/wgo@latest
	@echo "Installing atlas..."
	@OS=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	if [ "$$OS" = "linux" ] || [ "$$OS" = "darwin" ]; then \
		curl -sSf https://atlasgo.sh | sh; \
	else \
		echo ">>> On Windows, download manually:"; \
		echo ">>> curl -L https://release.ariga.io/atlas/atlas-windows-amd64-latest.exe -o \"$$(go env GOPATH)/bin/atlas.exe\""; \
	fi
	go mod download

ci: lint test

lint:
	go vet ./...

test:
	go test -race ./...

dev:
	wgo run ./cmd/server -db ./data.db

seed:
	atlas schema apply \
		--url "sqlite://data.db" \
		--to "file://schema.sql" \
		--dev-url "sqlite://dev?mode=memory" \
		--auto-approve
	go run ./cmd/seed -db ./data.db

db-reset:
	rm -f ./data.db

wire:
	wire ./wire

mock:
	mockery

# Full from-scratch verification — install, regenerate, lint, test.
all: install wire mock ci
