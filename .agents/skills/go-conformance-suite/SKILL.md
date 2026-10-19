---
name: go-conformance-suite
description: Use this skill when designing or reviewing tests for a Go interface that has, or will have, more than one implementation (drivers, backends, extractors, storage adapters, language plugins), when the user asks where interface tests should live versus implementation tests, when a second implementation of an existing interface is being added, or when someone asks for a "contract test", "conformance suite", "shared test suite", "<pkg>test package" or "fstest-style" tests. Also use it to decide the opposite case, when a package is small enough that local unit tests are enough and a conformance package would be overkill. Not for mocking guidance, table-test style in general, or integration tests against external services.
metadata:
  author: rfizzle
  standard: agentskills.io
---

# Go conformance suites

The stdlib pattern: define an interface once, verify it once with an importable
suite, and let every implementation call that suite from its own tests.
`testing/fstest.TestFS`, `testing/iotest`, and `gocloud.dev/*/drivertest` all
work this way.

## Decide first: does this interface need a suite?

Build a conformance package only when at least one of these holds:

- The interface has two or more implementations now.
- A second implementation is planned and named in the design (not "maybe someday").
- Third parties outside the module are expected to implement it.

Otherwise, **do not** build one. A package with a single implementation and
no external implementers gets ordinary local tests: table-driven `_test.go`
files in the package, exercising the concrete type. Adding a `<pkg>test`
package for one implementation is pure overhead, and it tempts people to
test through the interface when they should test the behavior directly.

When the second implementation arrives, extract the suite then. Moving
existing contract-shaped tests into `<pkg>test` is a mechanical refactor.

## Layout

```
internal/thing/              # interface, shared types, sentinel errors
internal/thing/thingtest/    # conformance suite + fake (non-test package)
internal/thing/pg/           # implementation A; calls thingtest once
internal/thing/mem/          # implementation B; calls thingtest once
testdata/<impl>/...          # per-implementation fixtures and goldens
```

Why a separate non-test package: `_test.go` files cannot be imported, so a
suite shared across packages must live in a normal package that imports
`testing`. Name it `<pkg>test`, next to the interface, so it is discoverable
and cannot import the implementations (that would be a cycle in spirit and
usually in fact).

## What goes where

| Belongs in `<pkg>test` (contract) | Stays in the implementation package |
| --- | --- |
| Every method's documented behavior on valid input | Parsing, tokenizing, path rules specific to this backend |
| Error contract: sentinel errors returned, wrapped correctly, on the documented conditions | Internal helpers and their edge cases |
| Determinism: two calls with the same input give equal results | Performance benchmarks and hot-path tests |
| Context handling: cancelled context returns promptly with an error | Resource cleanup specific to the backend |
| Cross-call invariants (sums agree, list matches get, sorted output is sorted) | Anything that needs the concrete type |
| Golden comparison against fixture files in a language-agnostic format | How goldens were counted (a COUNTING.md next to them) |

If a test only makes sense for one implementation, it is not contract; keep
it local. If a test would fail for a correct second implementation, it is
not contract either.

## The suite's shape

```go
package thingtest

// Fixture names what an implementation supplies to the suite.
type Fixture struct {
    Root      string   // where the implementation should Detect
    Expected  []string // what List must return, sorted
    GoldenDir string   // <name>.json per item, optional
}

// TestThing runs the contract against impl. Implementations call it once
// from their own tests; it must not be skipped or subset.
func TestThing(t *testing.T, impl thing.Thing, fx Fixture) {
    t.Helper()
    t.Run("Detect", func(t *testing.T) { ... })
    t.Run("List", func(t *testing.T) { ... })
    t.Run("Get/Validate", func(t *testing.T) { ... })
    t.Run("Get/Deterministic", func(t *testing.T) { ... })
    t.Run("Get/Unknown", func(t *testing.T) { ... })
    t.Run("Get/Cancelled", func(t *testing.T) { ... })
    t.Run("Invariants", func(t *testing.T) { ... })
    t.Run("Goldens", func(t *testing.T) { ... })
}

// NewFake returns an in-memory Thing built from a map, for consumers that
// need a Thing but not a real backend.
func NewFake(items map[string]thing.Item) thing.Thing
```

Rules that keep the suite honest:

- `t.Helper()` on the entry point and every assertion helper, so failures
  point at the implementation's test file.
- Subtests named after the contract clause, so a failing implementation
  reports which clause it broke.
- Take the interface type, never a concrete type or a constructor. If the
  suite needs to build instances, take a `func() thing.Thing` factory.
- No `t.Skip` inside the suite. An implementation that cannot satisfy a
  clause is not conformant; fix the implementation or narrow the interface.
- Prove the suite against the fake first, then prove it detects failure:
  mutate a golden and an invariant and assert the suite fails with a
  message naming the item, the field and both values.
- The fake lives here too, so consumers (CLI, HTTP layer, scoring code)
  can be tested without any real backend.
- Goldens are hand-verified fixtures, not snapshots. An `-update` flag may
  exist for regenerating after an intentional definition change, but the
  failure message should point to the counting notes before suggesting it.

## The implementation's test

One call, plus local tests:

```go
func TestConformance(t *testing.T) {
    thingtest.TestThing(t, pg.New(dsn(t)), thingtest.Fixture{
        Root:      fixtureRoot(t),
        Expected:  []string{"a", "b", "c"},
        GoldenDir: filepath.Join(fixtureRoot(t), "golden"),
    })
}
```

Everything else in that package's `_test.go` files is backend-specific.

## Gotchas

- Import direction: `<pkg>test` imports `<pkg>` and `testing`, nothing else
  from the module. Guard it with a `go list -deps` test if the module has
  layering rules.
- A suite that grows a `Config` with per-implementation switches is a smell:
  the interface is under-specified. Tighten the interface doc instead.
- Keep fixtures per implementation under `testdata/<impl>/`. The Go tool
  ignores `testdata` directories for `./...` patterns, which is also how a
  fixture module with deliberately bad code stays out of the root lint run.
- Package-level state for the fake's registry trips `gochecknoglobals`;
  build the fake from a constructor.
- A behavior that only some implementations can offer belongs in an
  optional interface (`interface{ Watch(...) }`) with its own suite
  function, invoked only when the implementation asserts to it.
