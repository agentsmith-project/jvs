GOLANGCI_LINT_VERSION ?= v1.64.8
GOLANGCI_LINT_PACKAGE := github.com/golangci/golangci-lint/cmd/golangci-lint
GOSEC_VERSION ?= v2.26.1
GOSEC_PACKAGE := github.com/securego/gosec/v2/cmd/gosec
STATICCHECK_VERSION ?= v0.7.0
STATICCHECK_PACKAGE := honnef.co/go/tools/cmd/staticcheck
FUZZTIME ?= 100x
FUZZMINIMIZETIME ?= 0
FUZZPARALLEL ?= 1
override release_fuzz_checked_shell = $(strip $(shell $1)$(if $(filter-out 0,$(.SHELLSTATUS)),$(error $2),))
override RELEASE_FUZZ_PACKAGE_PATTERN := ./test/fuzz/...
override RELEASE_FUZZ_PACKAGES = $(call release_fuzz_checked_shell,go list $(RELEASE_FUZZ_PACKAGE_PATTERN),release fuzz package discovery failed)
# Release fuzz exclusions must be edited here so skipped targets are reviewable.
override RELEASE_FUZZ_EXCLUDE_TARGETS :=
override RELEASE_FUZZ_ALL_TARGETS = $(call release_fuzz_checked_shell,set -eu; tmp=$$(mktemp); trap 'rm -f "$$tmp"' EXIT HUP INT TERM; for pkg in $(RELEASE_FUZZ_PACKAGES); do go test -list '^Fuzz' "$$pkg" >"$$tmp"; sed -n "s|^\(Fuzz[A-Za-z0-9_]*\)$$|$$pkg:\1|p" "$$tmp"; done,release fuzz target discovery failed)
override RELEASE_FUZZ_TARGETS = $(filter-out $(RELEASE_FUZZ_EXCLUDE_TARGETS),$(RELEASE_FUZZ_ALL_TARGETS))
STORY_E2E_RUN_PATTERN := ^(TestStoryE2EGate_|TestStoryLocal_|TestStoryJSON_|TestStory_PublicTransferJSON|TestStoryRepoClone|TestStorySeparated)

.PHONY: build test library fuzz-tests tools lint conformance regression contract-check docs-contract ci-contract verify security sec fuzz fuzz-list test-race test-cover test-all integration release-build release-binary-smoke release-gate story-local story-json story-e2e story-juicefs-local release-gate-juicefs clean

build:
	go build -o bin/jvs ./cmd/jvs

release-build:
	mkdir -p bin
	GOOS=linux GOARCH=amd64 go build -o bin/jvs-linux-amd64 ./cmd/jvs
	GOOS=linux GOARCH=arm64 go build -o bin/jvs-linux-arm64 ./cmd/jvs
	GOOS=darwin GOARCH=amd64 go build -o bin/jvs-darwin-amd64 ./cmd/jvs
	GOOS=darwin GOARCH=arm64 go build -o bin/jvs-darwin-arm64 ./cmd/jvs
	GOOS=windows GOARCH=amd64 go build -o bin/jvs-windows-amd64.exe ./cmd/jvs

release-binary-smoke: release-build
	@test -x "$(CURDIR)/bin/jvs-linux-amd64"
	PATH="$(CURDIR)/bin:$$PATH" JVS_BINARY_UNDER_TEST="$(CURDIR)/bin/jvs-linux-amd64" go test -tags conformance -count=1 -v -run '^TestStorySeparatedRestore' ./test/conformance/...

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

story-local: build
	PATH="$(CURDIR)/bin:$$PATH" go test -tags conformance -count=1 -v -run '^TestStoryLocal_' ./test/conformance/...

story-json: build
	PATH="$(CURDIR)/bin:$$PATH" go test -tags conformance -count=1 -v -run '^TestStoryJSON_' ./test/conformance/...

story-e2e: build
	PATH="$(CURDIR)/bin:$$PATH" go test -tags conformance -count=1 -v -run '$(STORY_E2E_RUN_PATTERN)' ./test/conformance/...

story-juicefs-local: $(if $(filter 1,$(JVS_STORY_JUICEFS_LOCAL) $(JVS_JUICEFS_E2E)),build)
	@set -eu; \
	if [ "$${JVS_STORY_JUICEFS_LOCAL:-}" != "1" ] && [ "$${JVS_JUICEFS_E2E:-}" != "1" ]; then \
		echo "SKIP story-juicefs-local: set JVS_STORY_JUICEFS_LOCAL=1 or JVS_JUICEFS_E2E=1 to run the real local JuiceFS profile."; \
		exit 0; \
	fi; \
	run_pattern='^(TestStoryJuiceFSLocal_|TestJuiceFSUserStory_)'; \
	list_file="$$(mktemp)"; \
	trap 'rm -f "$$list_file"' EXIT HUP INT TERM; \
	if ! JVS_JUICEFS_E2E=1 PATH="$(CURDIR)/bin:$$PATH" go test -tags 'conformance juicefs_e2e' -list "$$run_pattern" ./test/conformance/... >"$$list_file"; then \
		echo "Failed to list local JuiceFS story tests; see go test output above." >&2; \
		exit 1; \
	fi; \
	tests="$$(sed -n '/^TestStoryJuiceFSLocal_/p; /^TestJuiceFSUserStory_/p' "$$list_file")"; \
	if [ -z "$$tests" ]; then \
		echo "No local JuiceFS story tests registered; JuiceFS story implementation is owned separately." >&2; \
		exit 1; \
	fi; \
	JVS_JUICEFS_E2E=1 PATH="$(CURDIR)/bin:$$PATH" go test -tags 'conformance juicefs_e2e' -count=1 -v -run "$$run_pattern" ./test/conformance/...

release-gate-juicefs:
	$(MAKE) JVS_STORY_JUICEFS_LOCAL=1 JVS_JUICEFS_E2E=1 JVS_JUICEFS_E2E_REQUIRED=1 story-juicefs-local

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
	@set -eu; \
	echo "Running security scans..."; \
	echo "Installing gosec $(GOSEC_VERSION)"; \
	go install $(GOSEC_PACKAGE)@$(GOSEC_VERSION); \
	"$$(go env GOPATH)/bin/gosec" -verbose=text -fmt=json -out gosec-report.json ./... || true; \
	echo "Installing staticcheck $(STATICCHECK_VERSION)"; \
	go install $(STATICCHECK_PACKAGE)@$(STATICCHECK_VERSION); \
	"$$(go env GOPATH)/bin/staticcheck" ./... || true; \
	echo "Security scan complete. See gosec-report.json for details."

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
		go test "$$pkg" -run='^$$' -fuzz="^$${target}$$" -fuzztime=$(FUZZTIME) -fuzzminimizetime=$(FUZZMINIMIZETIME) -parallel=$(FUZZPARALLEL) -test.fuzzcachedir="$$fuzz_cache" || exit 1; \
	done
	@echo "All fuzzing tests passed."

test-race:
	go test -race -count=1 ./internal/... ./pkg/...

test-cover:
	go test -coverprofile=coverage.out -covermode=atomic ./internal/... ./pkg/...
	@go tool cover -func=coverage.out | awk '/^total:/ { gsub(/%/, "", $$3); if ($$3+0 < 60) { printf "FAIL: coverage %.1f%% < 60%% threshold\n", $$3; exit 1 } else { printf "OK: coverage %.1f%% >= 60%% threshold\n", $$3 } }'

test-all: test conformance regression fuzz

integration: build conformance

release-gate: tools docs-contract ci-contract test-race test-cover lint build release-build conformance release-binary-smoke library regression fuzz-tests fuzz
	@echo "RELEASE GATE PASSED"

clean:
	rm -rf bin/
	rm -f coverage.out gosec-report.json
