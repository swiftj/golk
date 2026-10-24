SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c

GO          ?= go
GOBIN       ?= $(shell $(GO) env GOPATH)/bin
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DIST        := dist
TARGETS     := darwin/arm64 darwin/amd64 windows/amd64 windows/arm64 linux/amd64 linux/arm64

STATICCHECK_VERSION := 2026.2.2
GOVULNCHECK_VERSION := v1.9.0

.PHONY: all tools generate lint test test-race test-integration build verify-static clean

all: lint test

tools: ## Install pinned dev tools into GOBIN
	$(GO) install honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VERSION)
	$(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	@command -v golangci-lint >/dev/null || { echo "golangci-lint v2 not found (brew install golangci-lint)"; exit 1; }

# Operation tags whose schemas are generated. Extend this list phase by phase.
GEN_TAGS    := Projects,Resources

generate: ## Regenerate lk/internal/gen from docs/legendkeeper/openapi.json
	$(GO) run ./internal/cmd/lkgen -spec docs/legendkeeper/openapi.json -tags $(GEN_TAGS) -out lk/internal/gen/types.go

lint: ## gofmt, go vet, staticcheck, golangci-lint, govulncheck
	@out="$$(gofmt -l $$(git ls-files -co --exclude-standard '*.go'))"; \
	  if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	$(GO) vet ./...
	$(GO) vet -tags integration ./...
	$(GOBIN)/staticcheck ./...
	$(GOBIN)/staticcheck -tags integration ./...
	golangci-lint run ./...
	golangci-lint run --build-tags integration ./...
	$(GOBIN)/govulncheck ./...

test: ## Unit tests, cgo disabled
	CGO_ENABLED=0 $(GO) test -count=1 ./...

test-race: ## Unit tests with the race detector (needs cgo; tests only, never ship)
	CGO_ENABLED=1 $(GO) test -race -count=1 ./...

test-integration: ## Integration tests against the sandbox project
	@: "$${LK_SANDBOX_API_KEY:?LK_SANDBOX_API_KEY must be set}"
	@: "$${LK_SANDBOX_PROJECT_NAME:?LK_SANDBOX_PROJECT_NAME must be set}"
	CGO_ENABLED=0 $(GO) test -tags integration -count=1 -p 1 ./...

build: ## Cross-compile lk-mcp for every target into dist/
	@mkdir -p $(DIST)
	@for t in $(TARGETS); do \
	  os=$${t%/*}; arch=$${t#*/}; ext=""; [ "$$os" = windows ] && ext=".exe"; \
	  out="$(DIST)/lk-mcp-$$os-$$arch$$ext"; \
	  echo "build $$out"; \
	  CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath \
	    -ldflags "-s -w -X main.version=$(VERSION)" -o "$$out" ./cmd/lk-mcp; \
	done

verify-static: ## Check every binary in dist/ is cgo-free (and static on Linux)
	./scripts/verify-static.sh $(DIST)

clean:
	rm -rf $(DIST)
