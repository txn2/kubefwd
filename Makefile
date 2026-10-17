# kubefwd — local verification matching CI.
#
# Run `make verify` before pushing. It runs the checks the CI pipeline runs
# (formatting, changed-line lint, tests with coverage floors, build, tidy and
# module verification, action-pin validation, gosec, govulncheck,
# osv-scanner and semgrep), so you don't find out from a red PR.
#
# Every tool is installed at its pinned version into .tools/<tool>-<version>/,
# and CI runs these same targets, so local and CI cannot drift onto different
# scanner versions. Bumping a version below installs the new one on next use.

# Tool versions. CI installs these through the targets below; the one pin CI
# repeats is golangci-lint's, in the lint job, and tools-check fails if the two
# disagree.
GOLANGCI_LINT_VERSION ?= v2.12.1
GOSEC_VERSION         ?= v2.28.0
GOVULNCHECK_VERSION   ?= v1.1.4
OSV_SCANNER_VERSION   ?= v2.6.0
GREMLINS_VERSION      ?= v0.6.0
DEADCODE_VERSION      ?= v0.51.0
SEMGREP_VERSION       ?= 1.151.0

# The Go toolchain is pinned by the toolchain directive in go.mod, which CI's
# setup-go reads too. With the default GOTOOLCHAIN=auto, go downloads it.
GO_VERSION_REQUIRED := $(shell sed -n 's/^toolchain go//p' go.mod)

# Coverage floors. COVERAGE_MIN is the total-coverage floor and matches
# codecov.yml's project target; PATCH_COVERAGE_MIN covers lines changed vs
# PATCH_BASE and matches codecov.yml's patch target. Both block.
COVERAGE_MIN       ?= 80
PATCH_COVERAGE_MIN ?= 80
PATCH_BASE         ?= origin/master

# Base for changed-line linting. CI uses the PR's base commit.
LINT_BASE ?= origin/master

# Mutation testing efficacy floor (make mutate, weekly in CI).
MUTATION_EFFICACY_MIN ?= 60

TOOLS_DIR     := $(CURDIR)/.tools
GOLANGCI_LINT := $(TOOLS_DIR)/golangci-lint-$(GOLANGCI_LINT_VERSION)/golangci-lint
GOSEC         := $(TOOLS_DIR)/gosec-$(GOSEC_VERSION)/gosec
GOVULNCHECK   := $(TOOLS_DIR)/govulncheck-$(GOVULNCHECK_VERSION)/govulncheck
OSV_SCANNER   := $(TOOLS_DIR)/osv-scanner-$(OSV_SCANNER_VERSION)/osv-scanner
GREMLINS      := $(TOOLS_DIR)/gremlins-$(GREMLINS_VERSION)/gremlins
DEADCODE      := $(TOOLS_DIR)/deadcode-$(DEADCODE_VERSION)/deadcode
SEMGREP_VENV  := $(TOOLS_DIR)/semgrep-$(SEMGREP_VERSION)
SEMGREP       := $(SEMGREP_VENV)/bin/semgrep

BUILD_DIR := $(CURDIR)/build

GO ?= go

.DEFAULT_GOAL := verify

.PHONY: verify
verify: check-go-version tools-check tidy-check mod-verify fmt-check lint test coverage-check build validate-actions patch-coverage security semgrep semgrep-diff
	@$(MAKE) --no-print-directory write-verify-sentinel
	@echo ""
	@echo "==> verify: all checks passed"

# Record that verify passed against the current working tree. The
# pre-commit review gate (~/.claude/hooks/review-gate.sh) reads this
# sentinel and must find the same tree id it computes, otherwise it
# blocks the commit. The id must match the gate's compute_diff_hash: a
# tree written from the working directory through a throwaway index
# seeded from HEAD, so it covers untracked files and ignores what is
# staged. .claude is gitignored, so writing the sentinel does not alter
# the tree.
.PHONY: write-verify-sentinel
write-verify-sentinel:
	@if git rev-parse --git-dir >/dev/null 2>&1; then \
	  mkdir -p .claude; \
	  dir=$$(mktemp -d); \
	  git read-tree --index-output="$$dir/index" HEAD; \
	  GIT_INDEX_FILE="$$dir/index" git add -A; \
	  GIT_INDEX_FILE="$$dir/index" git write-tree | cut -c1-16 > .claude/.last-verify-passed; \
	  rm -rf "$$dir"; \
	fi

.PHONY: check-go-version
check-go-version:
	@have=$$($(GO) env GOVERSION | sed 's/^go//'); \
	want=$(GO_VERSION_REQUIRED); \
	if [ "$$have" != "$$want" ]; then \
	  echo "ERROR: go is $$have, go.mod pins toolchain go$$want (CI uses the same)." >&2; \
	  echo "       Unset GOTOOLCHAIN or set it to auto so go fetches the pinned toolchain." >&2; \
	  exit 1; \
	fi; \
	echo "==> go $$have (matches go.mod toolchain)"

# ---------------------------------------------------------------------------
# Tools
# ---------------------------------------------------------------------------

# Tools build with whatever Go they require (osv-scanner needs a newer Go than
# kubefwd pins), so GOTOOLCHAIN=auto lets go fetch it even where the
# environment sets GOTOOLCHAIN=local.
$(GOLANGCI_LINT):
	@echo "==> installing golangci-lint $(GOLANGCI_LINT_VERSION)"
	@mkdir -p $(@D)
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/main/install.sh \
		| sh -s -- -b $(@D) $(GOLANGCI_LINT_VERSION)

$(GOSEC):
	@echo "==> installing gosec $(GOSEC_VERSION)"
	@GOTOOLCHAIN=auto GOBIN=$(@D) $(GO) install github.com/securego/gosec/v2/cmd/gosec@$(GOSEC_VERSION)

$(GOVULNCHECK):
	@echo "==> installing govulncheck $(GOVULNCHECK_VERSION)"
	@GOTOOLCHAIN=auto GOBIN=$(@D) $(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

$(OSV_SCANNER):
	@echo "==> installing osv-scanner $(OSV_SCANNER_VERSION)"
	@GOTOOLCHAIN=auto GOBIN=$(@D) $(GO) install github.com/google/osv-scanner/v2/cmd/osv-scanner@$(OSV_SCANNER_VERSION)

$(GREMLINS):
	@echo "==> installing gremlins $(GREMLINS_VERSION)"
	@GOTOOLCHAIN=auto GOBIN=$(@D) $(GO) install github.com/go-gremlins/gremlins/cmd/gremlins@$(GREMLINS_VERSION)

$(DEADCODE):
	@echo "==> installing deadcode $(DEADCODE_VERSION)"
	@GOTOOLCHAIN=auto GOBIN=$(@D) $(GO) install golang.org/x/tools/cmd/deadcode@$(DEADCODE_VERSION)

$(SEMGREP):
	@echo "==> installing semgrep $(SEMGREP_VERSION)"
	@python3 -m venv $(SEMGREP_VENV)
	@$(SEMGREP_VENV)/bin/pip install --quiet --disable-pip-version-check semgrep==$(SEMGREP_VERSION)

.PHONY: tools
tools: $(GOLANGCI_LINT) $(GOSEC) $(GOVULNCHECK) $(OSV_SCANNER) $(GREMLINS) $(DEADCODE) $(SEMGREP)

# Confirm each installed tool reports its pinned version, and that CI's one
# repeated pin (golangci-lint in the lint job) matches this Makefile. The
# install paths are versioned, so drift means a corrupt install or a CI pin
# that was bumped in one place only.
.PHONY: tools-check
tools-check: tools
	@echo "==> checking tool versions"
	@fail=0; \
	check() { \
	  if [ "$$2" != "$$3" ]; then echo "ERROR: $$1 is $$2, want $$3" >&2; fail=1; fi; \
	}; \
	modver() { $(GO) version -m "$$1" 2>/dev/null | awk '$$1=="mod" {print $$3; exit}'; }; \
	check golangci-lint "v$$($(GOLANGCI_LINT) version --short 2>/dev/null)" "$(GOLANGCI_LINT_VERSION)"; \
	check gosec "$$(modver $(GOSEC))" "$(GOSEC_VERSION)"; \
	check govulncheck "$$(modver $(GOVULNCHECK))" "$(GOVULNCHECK_VERSION)"; \
	check osv-scanner "$$(modver $(OSV_SCANNER))" "$(OSV_SCANNER_VERSION)"; \
	check gremlins "$$(modver $(GREMLINS))" "$(GREMLINS_VERSION)"; \
	check deadcode "$$(modver $(DEADCODE))" "$(DEADCODE_VERSION)"; \
	check semgrep "$$($(SEMGREP) --version 2>/dev/null | tail -1)" "$(SEMGREP_VERSION)"; \
	ci=$$(sed -n '/golangci-lint-action/,/version:/s/^ *version: *//p' .github/workflows/ci.yml); \
	check "golangci-lint pin in .github/workflows/ci.yml" "$$ci" "$(GOLANGCI_LINT_VERSION)"; \
	exit $$fail

# ---------------------------------------------------------------------------
# Modules
# ---------------------------------------------------------------------------

# Verify go.mod / go.sum are tidy without rewriting in place.
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

# Check that downloaded modules match go.sum.
.PHONY: mod-verify
mod-verify:
	@echo "==> go mod verify"
	@$(GO) mod verify

# ---------------------------------------------------------------------------
# Formatting and lint
# ---------------------------------------------------------------------------

# Apply the formatters configured in .golangci.yml: gofmt, goimports (with
# the kubefwd local prefix) and gofumpt.
.PHONY: fmt
fmt: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) fmt ./...

# Fail if any file in the repo is not formatted. Unlike lint, this covers the
# whole tree: formatting is mechanical and the tree is clean.
.PHONY: fmt-check
fmt-check: $(GOLANGCI_LINT)
	@echo "==> formatting (gofmt, goimports, gofumpt)"
	@out=$$($(GOLANGCI_LINT) fmt --diff ./... 2>&1); \
	if [ -n "$$out" ]; then \
	  echo "$$out"; \
	  echo "ERROR: files are not formatted. Run 'make fmt'." >&2; \
	  exit 1; \
	fi

# Lint only the lines changed since the merge base with $(LINT_BASE),
# matching CI's --new-from-rev. Pre-existing issues elsewhere do not fail
# the run, which lets strict linters apply to new code while the backlog
# is fixed separately (`make lint-full` shows it).
#
# The patch (scripts/changed-patch.sh) includes uncommitted and untracked
# .go files, so lint runs on what you are about to commit, not only on
# what is committed.
#
# The cache is cleaned first. A warm cache can return an earlier
# filtered result for an unchanged package and report 0 issues on lines
# that CI, which always runs cold, rejects.
.PHONY: lint
lint: $(GOLANGCI_LINT)
	@echo "==> golangci-lint $(GOLANGCI_LINT_VERSION) (changed lines)"
	@patch=$$(mktemp -t kubefwd-lint-patch.XXXXXX); \
	trap 'rm -f "$$patch"' EXIT; \
	./scripts/changed-patch.sh $(LINT_BASE) > "$$patch" || exit 1; \
	if [ ! -s "$$patch" ]; then \
	  echo "No Go changes; nothing to lint."; \
	  exit 0; \
	fi; \
	$(GOLANGCI_LINT) cache clean; \
	$(GOLANGCI_LINT) run --new-from-patch="$$patch" ./...

# Lint the whole repository, including pre-existing issues. Not part of
# verify, and CI does not enforce it. Use it for cleanup passes.
.PHONY: lint-full
lint-full: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

# ---------------------------------------------------------------------------
# Tests and coverage
# ---------------------------------------------------------------------------

.PHONY: test
test:
	@echo "==> go test (race + coverage, matches CI)"
	$(GO) test -race -coverprofile=coverage.txt -covermode=atomic ./...

coverage.txt:
	@$(MAKE) --no-print-directory test

# Fail if total statement coverage is below COVERAGE_MIN.
.PHONY: coverage-check
coverage-check: coverage.txt
	@total=$$($(GO) tool cover -func=coverage.txt | awk '/^total:/ {gsub(/%/, "", $$3); print $$3}'); \
	echo "==> total coverage $$total% (floor $(COVERAGE_MIN)%)"; \
	if awk -v t="$$total" -v m="$(COVERAGE_MIN)" 'BEGIN { exit !(t + 0 < m + 0) }'; then \
	  echo "ERROR: total coverage $$total% is below the $(COVERAGE_MIN)% floor." >&2; \
	  exit 1; \
	fi

# Coverage of the lines changed vs $(PATCH_BASE), approximating codecov's
# patch status. Blocking by default; STRICT=0 reports without failing.
.PHONY: patch-coverage
patch-coverage: coverage.txt
	@echo "==> patch coverage vs $(PATCH_BASE) (target $(PATCH_COVERAGE_MIN)%)"
	@./scripts/patch-coverage.sh $(PATCH_BASE) coverage.txt $(PATCH_COVERAGE_MIN) $${STRICT:-1}

# Mutation testing over the whole module. Too slow for every PR; CI runs it
# weekly (.github/workflows/mutation.yml). gremlins takes a directory, not a
# package pattern: `./pkg/...` matches nothing and passes with no results.
.PHONY: mutate
mutate: $(GREMLINS)
	@echo "==> mutation testing (efficacy floor $(MUTATION_EFFICACY_MIN)%)"
	$(GREMLINS) unleash --workers 4 --timeout-coefficient 3 --threshold-efficacy $(MUTATION_EFFICACY_MIN) .

# Report functions unreachable from main or tests. Informational: it does
# not fail, since exported API kept for library users shows up here too.
.PHONY: dead-code
dead-code: $(DEADCODE)
	@echo "==> dead code (informational)"
	@out=$$($(DEADCODE) -test ./... 2>&1); \
	if [ -n "$$out" ]; then echo "$$out"; else echo "No dead code found."; fi

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

.PHONY: build
build:
	@echo "==> go build"
	$(GO) build ./...

.PHONY: validate-actions
validate-actions:
	@echo "==> validating GitHub Action SHA pins"
	./scripts/validate-action-shas.sh

# ---------------------------------------------------------------------------
# Security
# ---------------------------------------------------------------------------

# gosec, govulncheck and osv-scanner. CI's security job runs this target.
.PHONY: security
security: gosec govulncheck osv

.PHONY: gosec
gosec: $(GOSEC)
	@echo "==> gosec"
	@$(GOSEC) -quiet ./...

# govulncheck exits 3 when our code calls a vulnerable symbol, whether or not
# a fix exists, and has no way to accept a finding. scripts/govulncheck-gate.py
# judges the report against .govulncheck-allow.txt instead, where an accepted
# advisory carries its reason and fails the gate once a fix ships or it stops
# being reported. Both modules are scanned into one report: the root module,
# and test/integration with its build tag.
GOVULN_REPORT := $(BUILD_DIR)/govulncheck-report.json
.PHONY: govulncheck
govulncheck: $(GOVULNCHECK)
	@echo "==> govulncheck (root module and test/integration)"
	@mkdir -p $(BUILD_DIR)
	@$(GOVULNCHECK) -format json ./... > $(GOVULN_REPORT); \
	status=$$?; \
	if [ $$status -ne 0 ] && [ $$status -ne 3 ]; then \
	  echo "ERROR: govulncheck failed to run on the root module (exit $$status)" >&2; \
	  rm -f $(GOVULN_REPORT); exit 1; \
	fi; \
	(cd test/integration && $(GOVULNCHECK) -tags=integration -format json ./...) >> $(GOVULN_REPORT); \
	status=$$?; \
	if [ $$status -ne 0 ] && [ $$status -ne 3 ]; then \
	  echo "ERROR: govulncheck failed to run on test/integration (exit $$status)" >&2; \
	  rm -f $(GOVULN_REPORT); exit 1; \
	fi; \
	python3 scripts/govulncheck-gate.py $(GOVULN_REPORT) .govulncheck-allow.txt; \
	status=$$?; \
	rm -f $(GOVULN_REPORT); \
	exit $$status

# osv-scanner judges go.mod and the docs site's Python requirements against
# the GitHub advisory database. Accepted advisories live in osv-scanner.toml
# with an ignoreUntil date; an entry that no longer matches a finding fails
# the gate, so the file cannot hold a stale acceptance.
#
# test/integration/go.mod is not scanned here. Every file in that module is
# behind the integration build tag, which osv-scanner's call analysis can't
# apply, so it would report every advisory in the module's dependency graph
# whether or not the tests reach it. govulncheck scans that module with the
# tag and real call analysis instead (see the govulncheck target).
OSV_MANIFESTS := -L go.mod -L .github/requirements-docs.txt
OSV_LOG := $(BUILD_DIR)/osv-scanner.log
.PHONY: osv
osv: $(OSV_SCANNER)
	@echo "==> osv-scanner"
	@mkdir -p $(BUILD_DIR)
	@$(OSV_SCANNER) scan source --config osv-scanner.toml $(OSV_MANIFESTS) > $(OSV_LOG) 2>&1; \
	status=$$?; \
	cat $(OSV_LOG); \
	if grep -q 'has unused ignores' $(OSV_LOG); then \
	  echo "ERROR: osv-scanner.toml accepts an advisory no longer reported; delete that entry." >&2; \
	  status=1; \
	fi; \
	rm -f $(OSV_LOG); \
	exit $$status

# Semgrep's Go ruleset over the whole tree.
.PHONY: semgrep
semgrep: $(SEMGREP)
	@echo "==> semgrep (p/golang)"
	@$(SEMGREP) scan --config p/golang --error --quiet --metrics=off .

# Rules in .semgrep-diff/ that only apply to changed lines. See
# scripts/semgrep-diff.py for why they are diff-scoped.
.PHONY: semgrep-diff
semgrep-diff: $(SEMGREP)
	@echo "==> semgrep (.semgrep-diff/, changed lines)"
	@SEMGREP=$(SEMGREP) python3 scripts/semgrep-diff.py .semgrep-diff/

# ---------------------------------------------------------------------------

.PHONY: clean
clean:
	rm -rf coverage.txt $(BUILD_DIR) $(TOOLS_DIR)

.PHONY: help
help:
	@echo "Targets:"
	@echo "  verify           everything CI checks (run before pushing)"
	@echo "  fmt              apply gofmt, goimports and gofumpt"
	@echo "  fmt-check        fail if any file is not formatted"
	@echo "  lint             golangci-lint on lines changed vs $(LINT_BASE)"
	@echo "  lint-full        golangci-lint on the whole repo (not part of verify)"
	@echo "  test             go test -race with coverage profile"
	@echo "  coverage-check   fail if total coverage is below $(COVERAGE_MIN)%"
	@echo "  patch-coverage   fail if changed-line coverage is below $(PATCH_COVERAGE_MIN)% (STRICT=0 to report only)"
	@echo "  security         gosec + govulncheck (allow-list gate) + osv-scanner"
	@echo "  semgrep          semgrep p/golang over the whole tree"
	@echo "  semgrep-diff     .semgrep-diff/ rules on changed lines"
	@echo "  mutate           gremlins mutation testing, $(MUTATION_EFFICACY_MIN)% efficacy floor (slow)"
	@echo "  dead-code        report unreachable functions (informational)"
	@echo "  build            go build ./..."
	@echo "  tidy             go mod tidy"
	@echo "  tidy-check       fail if go.mod/go.sum are not tidy"
	@echo "  mod-verify       go mod verify"
	@echo "  validate-actions check GitHub Action SHA pins"
	@echo "  tools            install every pinned tool into .tools/"
	@echo "  tools-check      fail if an installed tool or CI pin drifts from this Makefile"
	@echo "  clean            remove coverage.txt, build/ and the tool cache"
