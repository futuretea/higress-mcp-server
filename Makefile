BINARY_NAME ?= higress-mcp-server
GOLANGCI_LINT ?= golangci-lint
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --verify --quiet HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/futuretea/higress-mcp-server/pkg/core/version.Version=$(VERSION) \
	-X github.com/futuretea/higress-mcp-server/pkg/core/version.Commit=$(COMMIT) \
	-X github.com/futuretea/higress-mcp-server/pkg/core/version.Date=$(DATE)



.PHONY: build test test-race test-template-init lint format tidy ci e2e clean docker coverage

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/mcp-server

test:
	go test ./...

test-race:
	go test -race ./...

test-template-init:
	@if grep -Fq 'example.invalid/mcp-template-module-placeholder' go.mod; then \
		./scripts/test-init-template.sh; \
	else \
		echo "template initialization checks skipped in an initialized repository"; \
	fi

lint:
	go vet ./...
	test -z "$$(gofmt -l cmd internal pkg)"
	@if command -v "$(GOLANGCI_LINT)" >/dev/null 2>&1; then \
		$(GOLANGCI_LINT) run ./...; \
	else \
		echo "$(GOLANGCI_LINT) not installed, skipping"; \
	fi

format:
	gofmt -w $$(find cmd internal pkg -name '*.go')

tidy:
	go mod tidy

coverage:
	go test ./pkg/... -coverprofile=coverage.out
	go tool cover -func=coverage.out | tail -1

ci: lint test-race build

e2e: build
	HIGRESS_E2E=1 HIGRESS_E2E_BINARY="$(CURDIR)/bin/$(BINARY_NAME)" go test ./test/e2e -count=1 -v -timeout 12m

DOCKER_TAG ?= dev

docker:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(DATE) \
		-t ghcr.io/futuretea/higress-mcp-server:$(DOCKER_TAG) \
		-t ghcr.io/futuretea/higress-mcp-server:$(VERSION) .

clean:
	rm -rf bin coverage.out coverage.txt
