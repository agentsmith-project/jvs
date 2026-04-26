GOLANGCI_LINT_VERSION ?= v1.64.8
GOLANGCI_LINT_PACKAGE := github.com/golangci/golangci-lint/cmd/golangci-lint
FUZZTIME ?= 10s
FUZZPARALLEL ?= 1
override release_fuzz_checked_shell = $(strip $(shell $1)$(if $(filter-out 0,$(.SHELLSTATUS)),$(error $2),))
override RELEASE_FUZZ_PACKAGE_PATTERN := ./test/fuzz/...
override RELEASE_FUZZ_PACKAGES = $(call release_fuzz_checked_shell,go list $(RELEASE_FUZZ_PACKAGE_PATTERN),release fuzz package discovery failed)
# Release fuzz exclusions must be edited here so skipped targets are reviewable.
override RELEASE_FUZZ_EXCLUDE_TARGETS :=
override RELEASE_FUZZ_ALL_TARGETS = $(call release_fuzz_checked_shell,set -eu; tmp=$$(mktemp); trap 'rm -f "$$tmp"' EXIT HUP INT TERM; for pkg in $(RELEASE_FUZZ_PACKAGES); do go test -list '^Fuzz' "$$pkg" >"$$tmp"; sed -n "s|^\(Fuzz[A-Za-z0-9_]*\)$$|$$pkg:\1|p" "$$tmp"; done,release fuzz target discovery failed)
override RELEASE_FUZZ_TARGETS = $(filter-out $(RELEASE_FUZZ_EXCLUDE_TARGETS),$(RELEASE_FUZZ_ALL_TARGETS))

.PHONY: build test library fuzz-tests tools lint conformance regression contract-check docs-contract ci-contract verify security sec fuzz fuzz-list test-race test-cover test-all integration release-gate clean

build:
	go build -o bin/jvs ./cmd/jvs

tools:
	@set -eu; \
	gopath_lint="$$(go env GOPATH)/bin/golangci-lint"; \
	path_lint="$$(command -v golangci-lint 2>/dev/null || true)"; \
	for lint_bin in "$$path_lint" "$$gopath_lint"; do \
		if [ -n "$$lint_bin" ] && [ -x "$$lint_bin" ] && "$$lint_bin" --version | grep -q "version $(GOLANGCI_LINT_VERSION)"; then \
			echo "golangci-lint $(GOLANGCI_LINT_VERSION) available at $$lint_bin"; \
			exit 0; \
		fi; \
	done; \
	echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)"; \
	go install $(GOLANGCI_LINT_PACKAGE)@$(GOLANGCI_LINT_VERSION)

test:
	go test ./internal/... ./pkg/... ./test/library/... ./test/fuzz/...

library:
	go test -count=1 ./test/library/...

fuzz-tests:
	go test -count=1 ./test/fuzz/...

conformance: build
	PATH="$(CURDIR)/bin:$$PATH" go test -tags conformance -count=1 -v ./test/conformance/...

regression: build
	PATH="$(CURDIR)/bin:$$PATH" go test -tags conformance -count=1 -v ./test/regression/...

contract-check: build
	go test -count=1 ./internal/repo ./internal/snapshot ./internal/restore ./internal/worktree ./internal/gc ./internal/doctor ./internal/verify
	go test -tags conformance -count=1 -run 'TestContract_' -v ./test/conformance/...

docs-contract:
	go test -tags conformance -count=1 -run 'TestDocs_|TestConformancePublicProfileUsesStableCommands' -v ./test/conformance/...

ci-contract:
	go test -count=1 ./test/ci/...

lint:
	@set -eu; \
	gopath_lint="$$(go env GOPATH)/bin/golangci-lint"; \
	path_lint="$$(command -v golangci-lint 2>/dev/null || true)"; \
	lint_bin=""; \
	for candidate in "$$path_lint" "$$gopath_lint"; do \
		if [ -n "$$candidate" ] && [ -x "$$candidate" ] && "$$candidate" --version | grep -q "version $(GOLANGCI_LINT_VERSION)"; then \
			lint_bin="$$candidate"; \
			break; \
		fi; \
	done; \
	if [ -z "$$lint_bin" ]; then \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)"; \
		go install $(GOLANGCI_LINT_PACKAGE)@$(GOLANGCI_LINT_VERSION); \
		lint_bin="$$gopath_lint"; \
	fi; \
	if [ ! -x "$$lint_bin" ]; then \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) was not installed at $$lint_bin" >&2; \
		exit 127; \
	fi; \
	"$$lint_bin" run ./...

verify: test lint

security: sec

sec:
	@echo "Running security scans..."
	go install github.com/securecodewarrior/gosec/v2@latest || true
	gosec -verbose=text -fmt=json -out gosec-report.json ./... || true
	go install honnef.co/go/tools/cmd/staticcheck@latest || true
	staticcheck ./... || true
	@echo "Security scan complete. See gosec-report.json for details."

fuzz-list:
	@set -eu; \
	targets="$(RELEASE_FUZZ_TARGETS)"; \
	if [ -z "$$targets" ]; then \
		echo "No release-blocking fuzz targets found." >&2; \
		exit 1; \
	fi; \
	printf '%s\n' $$targets

fuzz:
	@set -eu; \
	targets="$(RELEASE_FUZZ_TARGETS)"; \
	if [ -z "$$targets" ]; then \
		echo "No release-blocking fuzz targets found." >&2; \
		exit 1; \
	fi; \
	fuzz_cache="$$(mktemp -d)"; \
	trap 'rm -rf "$$fuzz_cache"' EXIT HUP INT TERM; \
	echo "Running fuzzing tests ($(FUZZTIME) each)..."; \
	for entry in $$targets; do \
		pkg="$${entry%:*}"; \
		target="$${entry##*:}"; \
		if [ "$$pkg" = "$$entry" ] || [ -z "$$target" ]; then \
			echo "Invalid release fuzz target entry: $$entry" >&2; \
			exit 1; \
		fi; \
		echo "Fuzzing $$pkg $$target..."; \
		go test "$$pkg" -run='^$$' -fuzz="^$${target}$$" -fuzztime=$(FUZZTIME) -parallel=$(FUZZPARALLEL) -test.fuzzcachedir="$$fuzz_cache" || exit 1; \
	done
	@echo "All fuzzing tests passed."

test-race:
	go test -race -count=1 ./internal/... ./pkg/...

test-cover:
	go test -coverprofile=coverage.out -covermode=atomic ./internal/... ./pkg/...
	@go tool cover -func=coverage.out | awk '/^total:/ { gsub(/%/, "", $$3); if ($$3+0 < 60) { printf "FAIL: coverage %.1f%% < 60%% threshold\n", $$3; exit 1 } else { printf "OK: coverage %.1f%% >= 60%% threshold\n", $$3 } }'

test-all: test conformance regression fuzz

integration: build conformance

release-gate: tools docs-contract ci-contract test-race test-cover lint build conformance library regression fuzz-tests fuzz
	@echo "RELEASE GATE PASSED"

clean:
	rm -rf bin/
	rm -f coverage.out gosec-report.json
