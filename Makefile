GOCMD=go
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
# Every check runs in one environment, whether a person types the target or a
# gate runs it: no provider route or terminal palette inherited from the shell,
# no executable Git fsmonitor hook, and the tool caches under TMPDIR.
HERMETIC_ENV=env -u NO_COLOR GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=core.fsmonitor GIT_CONFIG_VALUE_0= TMPDIR=$${TMPDIR:-/tmp} XDG_CACHE_HOME=$${TMPDIR:-/tmp}/llm-cache GOLANGCI_LINT_CACHE=$${TMPDIR:-/tmp}/llm-golangci-lint
# The build tags the other test tiers live behind. test reads none of them, so
# a tagged file is skipped by default; vet reads them so a file that stopped
# compiling fails in the gate rather than in the one job that selects it.
TIER_TAGS=contract,integration
# gofmt ships with the toolchain but is not always on PATH; fall back to GOROOT.
GOFMT ?= $(shell command -v gofmt 2>/dev/null || echo "$$(go env GOROOT)/bin/gofmt")
GOIMPORTS=goimports
GOLANGCI_LINT=golangci-lint
PROJECT_GOFILES=$(shell find . -type f -name '*.go' -not -path "./vendor/*" -not -path "./.*")
# The gate only reads, and what it reads is what a commit would carry, so it
# asks git rather than the filesystem.
TRACKED_GOFILES=$(shell git ls-files '*.go')
PROJECT_PACKAGES=$(shell go list ./...)

.PHONY: all fmt fmt-check vet lint test integration help

all: help

## Check:
fmt: ## Rewrite every Go file with gofmt and goimports
	@$(GOFMT) -e -s -w $(PROJECT_GOFILES)
	@if command -v $(GOIMPORTS) >/dev/null 2>&1; then \
		$(GOIMPORTS) -e -format-only -w -d $(PROJECT_GOFILES); \
	else \
		echo "goimports not found, skipping (install: go install golang.org/x/tools/cmd/goimports@latest)"; \
	fi

# Both checks are hard: this refuses to run when a formatter is missing,
# because a gate that skips itself reports the same green as one that passed.
fmt-check: ## Fail if any tracked Go file is not gofmt- and goimports-clean
	@command -v $(GOFMT) >/dev/null 2>&1 || { echo "gofmt not found at $(GOFMT)"; exit 1; }
	@drift="$$($(GOFMT) -e -s -l $(TRACKED_GOFILES))"; \
	if [ -n "$$drift" ]; then echo "Not gofmt-clean. Run make fmt:"; echo "$$drift"; exit 1; fi
	@command -v $(GOIMPORTS) >/dev/null 2>&1 || { echo "goimports not found (install: go install golang.org/x/tools/cmd/goimports@latest)"; exit 1; }
	@drift="$$($(GOIMPORTS) -e -format-only -l $(TRACKED_GOFILES))"; \
	if [ -n "$$drift" ]; then echo "Imports are not grouped as goimports leaves them. Run make fmt:"; echo "$$drift"; exit 1; fi

vet: ## Run go vet over every tier's files
	@$(HERMETIC_ENV) $(GOVET) -mod=readonly -tags $(TIER_TAGS) $(PROJECT_PACKAGES)

lint: ## Run golangci-lint
	@$(HERMETIC_ENV) $(GOLANGCI_LINT) run --modules-download-mode=readonly

## Test:
# Cacheable on purpose: nothing here adds -count=1, and a test must not chdir.
test: ## Run the hermetic tier: no listener, clipboard, daemon or network
	@$(HERMETIC_ENV) $(GOTEST) -mod=readonly $(PROJECT_PACKAGES)

integration: ## Run the tests behind the integration tag (may need the network or credentials)
	@$(HERMETIC_ENV) $(GOTEST) -mod=readonly -tags=integration -count=1 -v $(PROJECT_PACKAGES)

## Help:
help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?##/ {printf "  %-14s%s\n", $$1, $$2}' $(MAKEFILE_LIST)
