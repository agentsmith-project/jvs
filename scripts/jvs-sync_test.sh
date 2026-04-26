#!/bin/bash
# Test script for jvs-sync.sh

# Don't use set -e as it causes early exit on test failures
# We handle errors manually in each test

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

TESTS_PASSED=0
TESTS_FAILED=0

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $*"
    ((TESTS_PASSED++))
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $*"
    ((TESTS_FAILED++))
}

log_info() {
    echo -e "${YELLOW}[INFO]${NC} $*"
}

# Setup test environment
setup_test_repo() {
    local test_dir="$1"
    mkdir -p "$test_dir"

    # Create minimal JVS structure
    mkdir -p "$test_dir/.jvs/worktrees/main"
    mkdir -p "$test_dir/.jvs/snapshots"
    mkdir -p "$test_dir/.jvs/descriptors"
    mkdir -p "$test_dir/.jvs/audit"
    mkdir -p "$test_dir/.jvs/gc"
    mkdir -p "$test_dir/.jvs/gc/tombstones"
    mkdir -p "$test_dir/.jvs/intents"
    mkdir -p "$test_dir/.jvs/locks/repo.lock"

    # Create format_version
    echo "1" > "$test_dir/.jvs/format_version"

    # Create worktree config
    cat > "$test_dir/.jvs/worktrees/main/config.json" << EOF
{
    "name": "main",
    "path": "main",
    "created_at": "2024-01-01T00:00:00Z"
}
EOF

    # Create some test files in main/
    mkdir -p "$test_dir/main"
    echo "test content" > "$test_dir/main/test.txt"
    echo "more content" > "$test_dir/main/file2.txt"

    # Create test intent file (should be excluded)
    echo "in-flight" > "$test_dir/.jvs/intents/test-intent.json"

    # Create runtime state (should be excluded)
    echo "locked" > "$test_dir/.jvs/locks/repo.lock/holder.json"
    echo "nested-lock" > "$test_dir/.jvs/locks/repo.lock/session.json"
    echo "active gc plan" > "$test_dir/.jvs/gc/active-plan.json"

    # Create committed GC metadata (should be synced)
    echo "committed tombstone" > "$test_dir/.jvs/gc/tombstones/retained.json"
}

test_help_output() {
    log_info "Testing help output..."

    # Use sed to strip escape codes, then grep
    if ./scripts/jvs-sync.sh --help 2>&1 | sed 's/\x1b\[[0-9;]*m//g' | grep -q "JVS Sync Helper"; then
        log_pass "Help command works"
    else
        log_fail "Help command output missing expected text"
    fi
}

test_help_runtime_state_boundary() {
    log_info "Testing help runtime-state boundary..."

    local help
    help=$(./scripts/jvs-sync.sh --help 2>&1 | sed 's/\x1b\[[0-9;]*m//g')

    local missing=0
    for expected in ".jvs/locks/**" ".jvs/intents/**" ".jvs/gc/*.json"; do
        if ! grep -Fq "$expected" <<< "$help"; then
            log_fail "Help output missing runtime-state boundary: $expected"
            missing=1
        fi
    done

    if grep -Fq ".jvs/*.lock" <<< "$help"; then
        log_fail "Help output still documents old .jvs/*.lock lock pattern"
        missing=1
    fi

    if grep -Fq ".jvs/gc/ (portable metadata)" <<< "$help"; then
        log_fail "Help output treats all .jvs/gc/ as portable metadata"
        missing=1
    fi

    if [[ $missing -eq 0 ]]; then
        log_pass "Help output documents the GA runtime-state boundary"
    fi
}

test_dry_run() {
    log_info "Testing dry-run mode..."

    local src_dir
    local dst_dir
    src_dir=$(mktemp -d)
    dst_dir=$(mktemp -d)

    setup_test_repo "$src_dir"

    # Run dry-run backup
    if ./scripts/jvs-sync.sh backup -n "$src_dir" "$dst_dir" &>/dev/null; then
        # Destination should NOT be created in dry-run mode
        if [[ ! -d "$dst_dir/.jvs" ]]; then
            log_pass "Dry-run doesn't create files"
        else
            log_fail "Dry-run created files (shouldn't)"
        fi
    else
        log_fail "Dry-run command failed"
    fi

    rm -rf "$src_dir" "$dst_dir"
}

test_backup_creates_destination() {
    log_info "Testing backup creates destination..."

    local src_dir
    local dst_dir
    src_dir=$(mktemp -d)
    dst_dir=$(mktemp -d)

    setup_test_repo "$src_dir"

    # Run actual backup
    if ./scripts/jvs-sync.sh backup "$src_dir" "$dst_dir" &>/dev/null; then
        if [[ -f "$dst_dir/.jvs/format_version" ]]; then
            log_pass "Backup creates destination files"
        else
            log_fail "Backup didn't create format_version"
        fi
    else
        log_fail "Backup command failed"
    fi

    rm -rf "$src_dir" "$dst_dir"
}

test_excludes_intents() {
    log_info "Testing intent file exclusion..."

    local src_dir
    local dst_dir
    src_dir=$(mktemp -d)
    dst_dir=$(mktemp -d)

    setup_test_repo "$src_dir"
    ./scripts/jvs-sync.sh backup "$src_dir" "$dst_dir" &>/dev/null

    # Intents should be excluded
    if [[ ! -f "$dst_dir/.jvs/intents/test-intent.json" ]]; then
        log_pass "Intent files are excluded"
    else
        log_fail "Intent files were synced (should be excluded)"
    fi

    rm -rf "$src_dir" "$dst_dir"
}

test_excludes_locks() {
    log_info "Testing lock file exclusion..."

    local src_dir
    local dst_dir
    src_dir=$(mktemp -d)
    dst_dir=$(mktemp -d)

    setup_test_repo "$src_dir"
    ./scripts/jvs-sync.sh backup "$src_dir" "$dst_dir" &>/dev/null

    # Lock directories should be excluded
    if [[ ! -e "$dst_dir/.jvs/locks/repo.lock/holder.json" && ! -e "$dst_dir/.jvs/locks/repo.lock/session.json" ]]; then
        log_pass "Lock directories are excluded"
    else
        log_fail "Lock directories were synced (should be excluded)"
    fi

    rm -rf "$src_dir" "$dst_dir"
}

test_excludes_active_gc_plans_only() {
    log_info "Testing active GC plan exclusion..."

    local src_dir
    local dst_dir
    src_dir=$(mktemp -d)
    dst_dir=$(mktemp -d)

    setup_test_repo "$src_dir"
    ./scripts/jvs-sync.sh backup "$src_dir" "$dst_dir" &>/dev/null

    if [[ -f "$dst_dir/.jvs/gc/active-plan.json" ]]; then
        log_fail "Active GC plan files were synced (should be excluded)"
    elif [[ ! -f "$dst_dir/.jvs/gc/tombstones/retained.json" ]]; then
        log_fail "Committed GC tombstones were not synced"
    else
        log_pass "Active GC plans are excluded while committed GC metadata is synced"
    fi

    rm -rf "$src_dir" "$dst_dir"
}

test_syncs_payload() {
    log_info "Testing payload file sync..."

    local src_dir
    local dst_dir
    src_dir=$(mktemp -d)
    dst_dir=$(mktemp -d)

    setup_test_repo "$src_dir"
    ./scripts/jvs-sync.sh backup "$src_dir" "$dst_dir" &>/dev/null

    # Payload files should be synced
    if [[ -f "$dst_dir/main/test.txt" ]]; then
        if [[ $(cat "$dst_dir/main/test.txt") == "test content" ]]; then
            log_pass "Payload files are synced correctly"
        else
            log_fail "Payload content doesn't match"
        fi
    else
        log_fail "Payload files not synced"
    fi

    rm -rf "$src_dir" "$dst_dir"
}

test_verify_command() {
    log_info "Testing verify command..."

    local src_dir
    local dst_dir
    src_dir=$(mktemp -d)
    dst_dir=$(mktemp -d)

    setup_test_repo "$src_dir"
    ./scripts/jvs-sync.sh backup "$src_dir" "$dst_dir" &>/dev/null

    # Verify should pass
    if ./scripts/jvs-sync.sh verify "$src_dir" "$dst_dir" &>/dev/null; then
        log_pass "Verify command passes for synced repos"
    else
        log_fail "Verify command failed for synced repos"
    fi

    rm -rf "$src_dir" "$dst_dir"
}

test_invalid_repo_detection() {
    log_info "Testing invalid repository detection..."

    local src_dir
    src_dir=$(mktemp -d)

    # Empty directory should fail
    if ./scripts/jvs-sync.sh backup "$src_dir" "/tmp/dummy" &>/dev/null; then
        log_fail "Backup succeeded for non-repo (should fail)"
    else
        log_pass "Backup fails for non-repo directory"
    fi

    rm -rf "$src_dir"
}

# Run all tests
main() {
    echo "================================"
    echo "JVS Sync Script Tests"
    echo "================================"
    echo ""

    cd "$(dirname "$0")/.."

    test_help_output
    test_help_runtime_state_boundary
    test_dry_run
    test_backup_creates_destination
    test_excludes_intents
    test_excludes_locks
    test_excludes_active_gc_plans_only
    test_syncs_payload
    test_verify_command
    test_invalid_repo_detection

    echo ""
    echo "================================"
    echo "Test Results"
    echo "================================"
    echo "Passed: $TESTS_PASSED"
    echo "Failed: $TESTS_FAILED"
    echo ""

    if [[ $TESTS_FAILED -eq 0 ]]; then
        echo -e "${GREEN}All tests passed!${NC}"
        exit 0
    else
        echo -e "${RED}Some tests failed!${NC}"
        exit 1
    fi
}

main "$@"
