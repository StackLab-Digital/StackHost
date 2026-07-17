SHELL := /bin/sh
APP := stackhost
setup:
	go mod download
	cd web && npm install
dev:
	go run ./cmd/stackhost
test:
	go test ./...
lint:
	gofmt -w cmd internal 2>/dev/null || true
	go vet ./...
	cd web && npm run typecheck && npm run lint
build:
	go build -o bin/$(APP) ./cmd/stackhost
	cd web && npm run build
