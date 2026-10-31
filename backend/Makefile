.PHONY: run build test test-live vet fmt seed tidy

run:
	go run ./cmd/api

build:
	go build -trimpath -ldflags="-s -w" -o bin/api ./cmd/api
	go build -trimpath -ldflags="-s -w" -o bin/seed ./cmd/seed

test:
	go test ./...

# Hits the real TikTok / J&T endpoints (needs internet).
test-live:
	LIVE_TESTS=1 go test ./internal/service -run Live -v

vet:
	go vet ./...

fmt:
	gofmt -l -w .

seed:
	go run ./cmd/seed

tidy:
	go mod tidy
