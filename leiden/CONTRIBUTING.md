# Contributing to leiden

Thanks for considering a contribution. This is a small, focused, zero-dependency
Go library — the bar for inclusion is correctness, idiomatic Go, and not
breaking the public API. The notes below cover the dev loop and the conventions
the project relies on.

## Prerequisites

- **Go 1.22+** (the module declares `go 1.22`; see `go.mod`).
- **git**.

That's it. The library has zero external dependencies and there is no build
system beyond `go test` and `go build`.

## Development loop

```bash
git clone https://github.com/k8nstantin/OpenPraxis.git
cd OpenPraxis/leiden

go vet ./...                  # static analysis — must pass clean
go build ./...                # must produce zero output
go test ./... -count=1        # all tests must pass
go test -race ./... -count=1  # race detector — strongly recommended

go test -run FuzzCompactNetwork -fuzz=FuzzCompactNetwork -fuzztime=30s ./...
go test -bench=. -benchmem -run='^$' ./...
```

`-count=1` disables the test result cache so re-runs actually re-execute.

## Code quality bar

These are non-negotiable. CI enforces them.

- **`go vet ./...` clean.** Zero warnings.
- **`go test ./... -count=1` passes.** No skipped tests, no flaky tests.
- **`go build ./...` produces zero output.** Zero warnings, zero diagnostics.
- **`staticcheck ./...`** should produce no findings if you have it installed.
- **All exported symbols have godoc comments.** Unexported symbols that aren't
  self-explanatory also need a comment explaining *why* — not *what*.
- **No `panic()` reachable from the public API.** Return a typed error
  (`var ErrX = errors.New(...)`) and let the caller decide.
- **No global mutable state.** RNGs are passed explicitly; do not call into the
  `math/rand` package-level functions.

## API design

The public surface is intentionally small. Easy to add, impossible to remove.

- New configuration goes on the `Options` struct, not as variadic options or
  positional arguments. `DefaultOptions()` picks up sensible defaults so adding
  a field is non-breaking.
- Operations that can fail return `(Value, error)`, not `(Value, bool)`.
- Return types are concrete; parameters can be interfaces (the Go proverb).
- Results are owned by the caller — never hand back a slice or map that
  aliases internal state. Copy before returning.
- No `interface{}` or `any` in the public API. Generic types or concrete types
  only.

## Zero external dependencies

This is a feature. Do not add `require` directives. Do not import
`golang.org/x/...`. If you genuinely need a function that lives outside the
standard library, open an issue first to discuss — the answer is almost
always "vendor a 20-line helper" or "rethink the design."

## Testing

- Unit tests for every exported function. Table-driven where it pays for
  itself (`[]struct{ name, input, expected }`).
- Integration tests on real-world benchmark graphs (Zachary's karate club,
  planted partitions). See `integration_test.go`.
- Benchmarks named `BenchmarkX`, exercised at multiple scales.
- Fuzz tests (`FuzzX`) for any function accepting external input.
- Seed-deterministic: use `rand.New(rand.NewSource(42))` (or another fixed
  seed) so CI is reproducible.
- Test helpers that *must* succeed are prefixed `must`:
  `mustBuildNetwork(t, edges)`.

When adding a new exported function, add at least one example
(`ExampleX` in `example_test.go`) so it shows up in godoc and is exercised by
`go test`.

## Branch + PR flow

- One branch per change, one PR per branch. Never push to `main`.
- Branch names: short, kebab-case, descriptive.
  Examples: `fix-refinement-tiebreak`, `add-modularity-helper`.
- One logical change per PR. Refactors, renames, and behavior changes belong in
  separate PRs.
- A green CI run is expected before review.

```bash
git checkout main
git pull
git checkout -b <your-branch>
# ... make changes ...
git commit -m "<imperative subject>"
git push -u origin <your-branch>
gh pr create --title "<title>" --body "<summary>"
```

## Commit style

One-line subject, imperative mood, no trailing period:

```
Add CommunityCount and GroupBy post-processing helpers
Fix deterministic argmax tiebreak in refinement
Reject negative MaxIterations with a typed error
```

If the change needs more context, add a blank line and a short body explaining
*why*. Keep the *what* in the diff.

## Releasing

This module is versioned independently from the surrounding repository. Go
submodule tags include the directory prefix:

```bash
git tag -a leiden/v0.X.Y -m "leiden v0.X.Y"
git push origin leiden/v0.X.Y
```

After tagging:

1. Move the `## Unreleased` section of `CHANGELOG.md` under a new
   `## [X.Y.Z] - YYYY-MM-DD` heading.
2. Create a GitHub Release that points at `leiden/vX.Y.Z` and pastes the
   changelog entry as the release notes.
3. `pkg.go.dev` will pick up the new version automatically within a few
   minutes; you can force a refresh by visiting
   `https://pkg.go.dev/github.com/k8nstantin/OpenPraxis/leiden@vX.Y.Z`.

Semantic versioning is strict: anything that breaks a caller (signature,
behavior, error type) requires a major-version bump.

## Reporting bugs

Please file a GitHub Issue with:

- `go version` output.
- OS and architecture.
- A minimal reproducer — ideally a `go test` snippet that fails on `main`.
- Expected vs. observed output, including any partition / quality values.

Security-relevant findings: email the maintainer rather than opening a public
issue.

## License

By contributing you agree that your changes are licensed under the Apache
License 2.0, matching the rest of the project.
