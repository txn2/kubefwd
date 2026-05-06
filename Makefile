# kubefwd — local verification matching CI.
#
# Run `make verify` before pushing. It runs the same checks the CI
# pipeline does (lint, test, build, tidy, action-pin validation,
# patch-coverage report) so you don't find out from a red PR.

# Versions — keep in sync with .github/workflows/ci.yml.
GOLANGCI_LINT_VERSION ?= v2.12.1
GO_VERSION_REQUIRED   ?= 1.26

# Local cache for tooling we install on demand.
TOOLS_BIN := $(CURDIR)/.tools/bin
GOLANGCI_LINT := $(TOOLS_BIN)/golangci-lint

# Patch coverage threshold — matches codecov.yml `patch.default.target`.
PATCH_COVERAGE_TARGET ?= 50
PATCH_BASE ?= origin/master

GO ?= go

.DEFAULT_GOAL := verify

.PHONY: verify
verify: check-go-version tidy-check lint test build validate-actions patch-coverage
	@echo ""
	@echo "==> verify: all checks passed"

.PHONY: check-go-version
check-go-version:
	@have=$$($(GO) env GOVERSION | sed 's/^go//'); \
	want=$(GO_VERSION_REQUIRED); \
	case "$$have" in \
	  $$want|$$want.*) echo "==> go $$have (matches $$want.x)";; \
	  *) echo "ERROR: local go is $$have, CI uses $$want.x. Install matching toolchain." >&2; exit 1;; \
	esac

# Verify go.mod / go.sum are tidy without rewriting in place — same
# guarantee CI implicitly relies on when it runs `go mod download`.
.PHONY: tidy-check
tidy-check:
	@echo "==> go mod tidy (check)"
	@tmp=$$(mktemp -d); cp go.mod go.sum "$$tmp/"; \
	$(GO) mod tidy; \
	if ! diff -q "$$tmp/go.mod" go.mod >/dev/null || ! diff -q "$$tmp/go.sum" go.sum >/dev/null; then \
	  echo "ERROR: go.mod/go.sum are not tidy. Run 'go mod tidy' and commit." >&2; \
	  diff -u "$$tmp/go.mod" go.mod || true; \
	  diff -u "$$tmp/go.sum" go.sum || true; \
	  cp "$$tmp/go.mod" "$$tmp/go.sum" .; \
	  rm -rf "$$tmp"; \
	  exit 1; \
	fi; \
	rm -rf "$$tmp"

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: build
build:
	@echo "==> go build"
	$(GO) build ./...

.PHONY: test
test:
	@echo "==> go test (race + coverage, matches CI)"
	$(GO) test -race -coverprofile=coverage.txt -covermode=atomic ./...

# Install the exact golangci-lint version CI uses, into a local cache.
# Avoids the trap where a system-wide linter built against an older Go
# fails on a go.mod targeting a newer toolchain (the bug this Makefile
# was created to prevent).
$(GOLANGCI_LINT):
	@echo "==> installing golangci-lint $(GOLANGCI_LINT_VERSION)"
	@mkdir -p $(TOOLS_BIN)
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/main/install.sh \
		| sh -s -- -b $(TOOLS_BIN) $(GOLANGCI_LINT_VERSION)

.PHONY: lint
lint: $(GOLANGCI_LINT)
	@echo "==> golangci-lint $(GOLANGCI_LINT_VERSION)"
	@have=$$($(GOLANGCI_LINT) --version | awk '{print $$4}'); \
	want=$$(echo $(GOLANGCI_LINT_VERSION) | sed 's/^v//'); \
	if [ "$$have" != "$$want" ]; then \
	  echo "ERROR: golangci-lint version drift: have $$have, want $$want." >&2; \
	  echo "Delete $(TOOLS_BIN)/golangci-lint and re-run." >&2; \
	  exit 1; \
	fi
	$(GOLANGCI_LINT) run --timeout=5m

.PHONY: validate-actions
validate-actions:
	@echo "==> validating GitHub Action SHA pins"
	./scripts/validate-action-shas.sh

# Approximates codecov's patch-coverage check locally. Coverage profile
# from `make test` is intersected with lines added vs $(PATCH_BASE).
# Reports as informational by default — matches codecov.yml — but you
# can flip it to blocking with `make patch-coverage STRICT=1`.
.PHONY: patch-coverage
patch-coverage: coverage.txt
	@echo "==> patch coverage vs $(PATCH_BASE) (target $(PATCH_COVERAGE_TARGET)%)"
	@./scripts/patch-coverage.sh $(PATCH_BASE) coverage.txt $(PATCH_COVERAGE_TARGET) $${STRICT:-0}

coverage.txt:
	@$(MAKE) test

.PHONY: clean
clean:
	rm -rf coverage.txt $(TOOLS_BIN) .tools

.PHONY: help
help:
	@echo "Targets:"
	@echo "  verify           lint + test + build + tidy-check + action pins + patch coverage"
	@echo "  lint             golangci-lint (auto-installs $(GOLANGCI_LINT_VERSION) to .tools/)"
	@echo "  test             go test -race with coverage profile"
	@echo "  build            go build ./..."
	@echo "  tidy             go mod tidy"
	@echo "  tidy-check       fail if go.mod/go.sum are not tidy"
	@echo "  validate-actions check GitHub Action SHA pins"
	@echo "  patch-coverage   coverage of lines changed vs $(PATCH_BASE)"
	@echo "                   STRICT=1 to fail below $(PATCH_COVERAGE_TARGET)%"
	@echo "  clean            remove coverage.txt and tool cache"
