# Fuzzing Tests for JVS

This directory contains fuzzing tests for JVS critical functions. Fuzzing is a dynamic analysis technique that automatically finds edge cases, panics, and security vulnerabilities by testing code with randomized inputs.

## Overview

Fuzzing tests use Go's built-in fuzzer (introduced in Go 1.18) to generate thousands of random inputs and test how code handles unexpected or malformed data. This is especially valuable for:

- **Parsing functions** (snapshot IDs, descriptors, paths)
- **Validation functions** (names, tags, paths)
- **Data serialization** (JSON marshaling/unmarshaling)

## Running Fuzz Tests

### Quick smoke test (5 seconds per fuzzer)

```bash
# Run a specific fuzz target for 5 seconds
go test -fuzz=FuzzValidateName -fuzztime=5s ./test/fuzz/...

# Run all fuzz tests briefly
go test -fuzz=. -fuzztime=5s ./test/fuzz/...
```

### Standard fuzzing run (1 minute per fuzzer)

```bash
# Run for 1 minute (recommended for CI)
go test -fuzz=FuzzValidateName -fuzztime=1m ./test/fuzz/...

# Run all fuzzers for 1 minute each
go test -fuzz=. -fuzztime=1m ./test/fuzz/...
```

### Extended fuzzing (for deep analysis)

```bash
# Run overnight or for extended periods
go test -fuzz=FuzzValidateName -fuzztime=24h ./test/fuzz/...
```

### With coverage guidance

```bash
# Use corpus from previous runs for better coverage
go test -fuzz=FuzzValidateName -fuzztime=1m ./test/fuzz/... -test.fuzzcachedir=/tmp/jvs-fuzz-cache
```

## Fuzz Targets

| Target | Function Tested | Purpose |
|--------|----------------|---------|
| `FuzzValidateName` | `pathutil.ValidateName` | Worktree name validation |
| `FuzzValidateTag` | `pathutil.ValidateTag` | Tag validation |
| `FuzzParseSnapshotID` | `model.SnapshotID` | Snapshot ID parsing/formatting |
| `FuzzCanonicalMarshal` | `jsonutil.CanonicalMarshal` | JSON canonicalization |
| `FuzzDescriptorJSON` | `model.Descriptor` | Descriptor JSON marshaling/unmarshaling |
| `FuzzSnapshotIDString` | `model.SnapshotID.String()` | Snapshot ID string conversion |
| `FuzzDescriptorMalformedJSON` | `model.Descriptor` | Malformed descriptor handling |
| `FuzzReadyMarkerJSON` | `model.ReadyMarker` | ReadyMarker JSON parsing |
| `FuzzIntentRecordJSON` | `model.IntentRecord` | IntentRecord JSON parsing |
| `FuzzCompressionInfoJSON` | `model.CompressionInfo` | CompressionInfo JSON parsing |
| `FuzzPartialPaths` | Path validation | Partial snapshot path validation |
| `FuzzTagValue` | Tag validation | Tag value validation |
| `FuzzDescriptorChecksum` | Descriptor checksum | Descriptor checksum consistency |

## Understanding Fuzz Test Output

```
fuzz: elapsed: 3s, execs: 808244 (269336/sec), new interesting: 194 (total: 208)
```

- `elapsed`: Time spent fuzzing
- `execs`: Total number of test executions
- `(269336/sec)`: Executions per second (higher = better)
- `new interesting`: New inputs that increased code coverage
- `(total: 208)`: Total size of the corpus (interesting inputs saved)

### If a bug is found

When the fuzzer finds a crash or assertion failure:

```
--- FAIL: FuzzValidateName (0.23s)
    --- FAIL: FuzzValidateName (0.00s)
testing.go:1398: panic: runtime error: index out of range

Failing input written to: /tmp/testFuzz2554378948/seed0
```

The failing input is saved to a file. You can reproduce with:

```bash
go test -v ./test/fuzz/...
```

## Interpreting Results

### PASS - No issues found

```
PASS
ok      github.com/agentsmith-project/jvs/test/fuzz     6.025s
```

The code handled all random inputs correctly (no panics, no assertion failures).

### High execution rate

- **Good**: >200,000 exec/sec indicates simple, fast validation
- **Expected**: 100,000-500,000 exec/sec for simple validation
- **Lower**: Complex operations may run 10,000-100,000 exec/sec

### "New interesting" count

- Indicates the fuzzer found inputs that exercise new code paths
- Higher numbers suggest better coverage exploration
- Stabilizes over time as the fuzzer exhausts new paths

## Seed Corpus

Each fuzz target includes a seed corpus of known edge cases:

```go
f.Add("")                           // empty string
f.Add("valid-name-123")             // valid name
f.Add("..")                          // path traversal
f.Add("../escape")                   // path traversal attempt
f.Add("name/with/slash")             // invalid separator
```

The fuzzer uses these as a starting point and mutates them to find new interesting inputs.

## Best Practices

### Before Committing

Run each fuzz target for at least 10 seconds:

```bash
go test -fuzz=. -fuzztime=10s ./test/fuzz/...
```

### In CI/CD

For continuous integration, use shorter runs:

```bash
# Quick check (30 seconds per fuzzer)
go test -fuzz=. -fuzztime=30s ./test/fuzz/...
```

### After Security Changes

When modifying validation, parsing, or serialization code:

```bash
# Extended run to find edge cases
go test -fuzz=. -fuzztime=5m ./test/fuzz/...
```

### Saving Interesting Inputs

To preserve fuzz corpus for future runs:

```bash
# Save corpus to testdata/fuzz
go test -fuzz=FuzzValidateName -fuzztime=1m ./test/fuzz/... -test.fuzzcachedir=testdata/fuzz/FuzzValidateName
```

## Adding New Fuzz Targets

When adding a new fuzz target:

1. **Name it appropriately**: `Fuzz<FunctionName>`
2. **Add seed corpus**: Include known edge cases
3. **Document what it tests**: Explain the function and purpose
4. **Test locally**: Run for at least 1 minute before committing

```go
// FuzzMyFunction tests my function with random inputs.
func FuzzMyFunction(f *testing.F) {
    // Add seed corpus
    f.Add("valid input")
    f.Add("invalid input")

    f.Fuzz(func(t *testing.T, input string) {
        // Call the function being tested
        result := MyFunction(input)

        // Assert invariants that should always be true
        if result != nil && !isValid(result) {
            t.Errorf("invalid result: %v", result)
        }
    })
}
```

## Coverage Reports

To see which code paths the fuzzer is exploring:

```bash
# Run with coverage
go test -coverprofile=fuzz_coverage.out -fuzz=FuzzValidateName -fuzztime=10s ./test/fuzz/...

# View coverage
go tool cover -html=fuzz_coverage.out
```

## Q1 Roadmap Item: "Add dynamic analysis (fuzzing)"

This fuzzing suite satisfies the Q1 roadmap requirement for adding dynamic analysis. The implementation includes:

- ✅ Fuzzing for snapshot ID parsing
- ✅ Fuzzing for descriptor parsing
- ✅ Fuzzing for path validation
- ✅ Fuzzing for JSON serialization
- ✅ Documentation and CI integration ready

## Resources

- [Go Fuzzing Tutorial](https://go.dev/doc/tutorial/fuzz)
- [Go Fuzzing Blog Post](https://go.dev/security/fuzz/)
- [CNCF Security Tools](https://www.cncf.org/projects-security/)

## Troubleshooting

### Fuzzer is slow

- Reduce the input data size (if using large inputs)
- Check for expensive operations in the fuzz function
- Use `-fuzztime` to limit execution time

### Out of memory

- Fuzzers can consume significant memory with large corpora
- Use `-fuzztime` with shorter duration
- Clear fuzz cache: `rm -rf testdata/fuzz`

### Flaky tests

- Ensure fuzz function is deterministic (same input → same output)
- Avoid using global state or time-based randomness in fuzz function
- Use `t.Skip()` for truly random tests that shouldn't be fuzzed
