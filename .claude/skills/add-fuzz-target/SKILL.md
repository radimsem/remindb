---
name: add-fuzz-target
description: Use when adding a new `Fuzz*` function in remindb — symptoms include "fuzz this function", "find panics in X", "add property-based testing", "harden against malformed input", or any task that creates a `FuzzXxx(f *testing.F)` in a `*_test.go`. Also use when extending an existing fuzz target's seed corpus with new shape coverage.
---

# Add a fuzz target

remindb fuzzes parser, query, transformer, compiler, and temperature code. The fuzz harness is whatever Go's `testing.F` gives you — there's no project-specific framework — but the *seed-corpus discipline* is project convention worth getting right. `scripts/fuzz.sh` auto-discovers any `Fuzz*` function via `go test -list='^Fuzz'`, so naming your function `FuzzXxx` is the only registration needed.

## Where it lands

Two files at most.

| File | What changes |
|---|---|
| `pkg/<package>/fuzz_test.go` | New file or extend existing — `FuzzXxx(f *testing.F)` |
| `pkg/<package>/testdata/fuzz/<FuzzXxx>/` | Auto-managed by Go fuzz; commit any minimization corpus crashes find here |

If `pkg/<package>/fuzz_test.go` already exists (it does for `parser`, `query`, `transformer`, `compiler`, `temperature`), append; don't make a second file.

## The function shape

Mirror `pkg/parser/fuzz_test.go` and `pkg/temperature/fuzz_test.go`. The shape is uniform:

```go
func FuzzExample(f *testing.F) {
    // Seed corpus — see "Seed selection" below.
    f.Add(input1, input2)
    // ... more f.Add lines, each one shape ...

    f.Fuzz(func(t *testing.T, input1 T1, input2 T2) {
        result, err := YourFunc(input1, input2)

        // Invariants — see "Invariant assertions" below.
        if err != nil {
            return        // errors are fine; panics are not
        }
        if !invariantHolds(result) {
            t.Errorf("invariant violated: ...")
        }
    })
}
```

Two rules:

- **Function name `FuzzXxx`.** `scripts/fuzz.sh` greps for `^Fuzz` in `go test -list` output. Anything else is invisible.
- **One target per logical surface.** Don't multiplex two unrelated functions into one fuzz target — Go's fuzzer mutates the input tuple as a unit, so combined targets dilute coverage.

## Seed selection — the discipline

A fuzz seed says "this is a *shape* worth starting from." The fuzzer mutates from there. The point isn't to enumerate all valid inputs; it's to give the mutator a head start on every category of structural variation. Aim for one seed per shape:

| Shape | Why it matters | Example |
|---|---|---|
| Happy path | Baseline — proves the harness wires up correctly | `f.Add("doc.md", []byte("# Hello"))` |
| Empty input | Off-by-one and bounds-check guard | `f.Add("file.md", []byte{})` |
| Malformed | Decoder error path | `f.Add("file.json", []byte("{unclosed"))` |
| UTF-8 boundary | Mid-byte slicing, multi-byte boundary | `f.Add("file.md", []byte{0xe3})` |
| Structural extreme | Deep nesting, large counts | `f.Add("file.json", []byte("{\"a\":{\"b\":{\"c\":\"deep\"}}}"))` |
| Numeric extreme (numeric inputs) | Inf, NaN, MaxFloat, MaxInt, negative | `f.Add(math.MaxFloat64, 1.0)` |
| Boundary value (parameterized) | The threshold/cap/limit value itself | `f.Add(1, math.MaxInt)` (budget exactly at limit) |

`pkg/temperature/fuzz_test.go` is the cleanest template for numeric fuzz; `pkg/parser/fuzz_test.go` is the template for byte-sequence fuzz. Both spend ~10 seed lines each — that's the right density.

## Invariant assertions

The default invariant is **must not panic**. The runtime catches panics and reports them as crashes, so you don't write that one — it's free.

Beyond no-panic, write *property* assertions, not example-based ones. The mutator generates novel inputs you can't predict:

- **Type-level invariants:** `len(out) <= len(in)`, `result >= 0`, `tokensUsed <= budget`.
- **Round-trip invariants:** `Decode(Encode(x)) == x`.
- **Conditional invariants:** "if input is valid UTF-8, error must be nil". `pkg/parser/fuzz_test.go` does this for `ErrInvalidUTF8`.
- **Domain invariants:** "result is non-NaN when both inputs are non-NaN" (see `FuzzDecayFactor` and `FuzzScore` in `pkg/temperature/fuzz_test.go`).

A common pattern: when an input could be invalid, *gate* the assertion on input validity:

```go
if budget >= 0 && tok1 >= 0 && tok2 >= 0 {
    if result.TokensUsed > budget {
        t.Errorf("TokensUsed = %d exceeds budget %d", result.TokensUsed, budget)
    }
}
```

This avoids spurious failures from negative-int inputs you've already documented as out-of-domain.

## Running it locally

```bash
# Run all fuzz targets for 30 seconds each
scripts/fuzz.sh 30s

# Or one target for longer
go test -run='^$' -fuzz='^FuzzExample$' -fuzztime=2m ./pkg/<package>/
```

A new target should run for at least 30s without crashing before you commit. If a crash surfaces, the crashing seed is saved under `pkg/<package>/testdata/fuzz/FuzzExample/<hash>` — commit that file along with the fix so future runs replay the regression.

## Quick reference

```
1. pkg/<package>/fuzz_test.go    (FuzzXxx with seed corpus + Fuzz callback)
2. scripts/fuzz.sh 30s            (must finish without crashes)
3. If a crash file appears in testdata/fuzz/: fix root cause, commit the seed
```

## Common mistakes

- **Asserting on a specific output value.** The fuzzer's input is unpredictable; specific outputs can't hold. Use property assertions: bounds, types, conditional invariants.
- **Returning early on `err != nil` without checking what kind of error.** If the function should return a *specific* sentinel for a given input class (like `ErrInvalidUTF8` for invalid UTF-8 in parser), assert that. Otherwise generic `return` on error is correct.
- **Missing the `f.Add(...)` seed for the empty/zero case.** The Go fuzzer mutates from seeds — without an empty seed, it may take a long time to discover the empty input naturally.
- **Long-running invariant checks inside the fuzz body.** Each iteration runs thousands of times per second; an O(n²) assertion will throttle the fuzzer and reduce coverage. Keep invariant checks O(n) or O(1).
- **Not committing crash seeds from `testdata/fuzz/`.** The seed file is the regression test for the bug you just fixed. Without it, the same bug can re-emerge silently.
- **Naming the function `FuzzTestXxx` or `FuzzCheck`.** `scripts/fuzz.sh` greps `^Fuzz` so any prefix-only-`Fuzz` name works, but stylistic convention here is `FuzzXxx` matching the function-under-test.

## Cross-references

- `.claude/rules/go-concise.md` — error sentinel patterns, naming
- `.claude/skills/add-parser/SKILL.md` — for the special case of seeding `FuzzParseBytes` with a new format (you don't add a new fuzz target, you extend an existing one)
- `scripts/fuzz.sh` — the auto-discovery loop; read it once if you're curious how `go test -list='^Fuzz'` finds your target
