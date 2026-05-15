.PHONY: ci lint test dev seed wire mock

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

wire:
	wire ./wire

mock:
	mockery
