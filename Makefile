.PHONY: build test library lint conformance regression contract-check ci-contract verify security sec fuzz test-race test-cover test-all integration release-gate clean

build:
	go build -o bin/jvs ./cmd/jvs

test:
	go test ./internal/... ./pkg/... ./test/library/...

library:
	go test -count=1 ./test/library/...

conformance:
	go test -tags conformance -count=1 -v ./test/conformance/...

regression:
	go test -tags conformance -count=1 -v ./test/regression/...

contract-check: build
	go test -count=1 ./internal/repo ./internal/snapshot ./internal/restore ./internal/worktree ./internal/gc ./internal/doctor ./internal/verify
	go test -tags conformance -count=1 -run 'TestContract_' -v ./test/conformance/...

ci-contract:
	go test -count=1 ./test/ci/...

lint:
	golangci-lint run ./...

verify: test lint

security: sec

sec:
	@echo "Running security scans..."
	go install github.com/securecodewarrior/gosec/v2@latest || true
	gosec -verbose=text -fmt=json -out gosec-report.json ./... || true
	go install honnef.co/go/tools/cmd/staticcheck@latest || true
	staticcheck ./... || true
	@echo "Security scan complete. See gosec-report.json for details."

fuzz:
	@echo "Running fuzzing tests (10 seconds each)..."
	@for target in FuzzValidateName FuzzValidateTag FuzzParseSnapshotID FuzzCanonicalMarshal FuzzDescriptorJSON FuzzSnapshotIDString FuzzPartialPaths; do \
		echo "Fuzzing $$target..."; \
		go test -fuzz="$$target" -fuzztime=10s ./test/fuzz/... || exit 1; \
	done
	@echo "All fuzzing tests passed."

test-race:
	go test -race -count=1 ./internal/... ./pkg/...

test-cover:
	go test -coverprofile=coverage.out -covermode=atomic ./internal/... ./pkg/...
	@go tool cover -func=coverage.out | awk '/^total:/ { gsub(/%/, "", $$3); if ($$3+0 < 60) { printf "FAIL: coverage %.1f%% < 60%% threshold\n", $$3; exit 1 } else { printf "OK: coverage %.1f%% >= 60%% threshold\n", $$3 } }'

test-all: test conformance regression fuzz

integration: build conformance

release-gate: ci-contract test-race test-cover lint build conformance library regression fuzz
	@echo "RELEASE GATE PASSED"

clean:
	rm -rf bin/
	rm -f coverage.out gosec-report.json
