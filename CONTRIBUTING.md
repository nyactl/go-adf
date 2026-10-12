# Contributing

Thanks for helping improve go-adf. Bug reports, fixes and focused feature proposals are welcome.

## Before you start

- For anything bigger than a small fix, open an issue first to agree on the approach.
- Security problems go through private reporting, see [SECURITY.md](SECURITY.md).
- By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## Development

Requires Go 1.23 or newer.

```sh
make test   # unit tests, examples, schema validation and the fuzz corpus
make lint
make fuzz   # each fuzz target for a minute; FUZZTIME=10m for longer
```

A conversion bug is best reported, and fixed, with the input that shows it. Add it to the tests: Markdown that must read back unchanged goes into `roundTrips` (roundtrip_test.go), Markdown that ADF cannot hold one to one into `awkward` (schema_test.go). Inputs found by fuzzing are saved in `testdata/fuzz` and should be committed with the fix.

## Rules

- `FromMarkdown` output must stay valid against the schema in `testdata/schema`, and `ToMarkdown` output must read back as the same document. Fuzz for a while after changing either direction.
- A new node or mark type, or a change in how one is converted, updates [COVERAGE.md](COVERAGE.md).
- Keep the API small. A new option is an `Option` function; exported types and functions are a promise for all of v1.
- Build strings with invisible characters in tests with escapes such as `\u200b`, never as raw characters.

## Commits and pull requests

- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/), e.g. `fix: escape a leading plus sign`; mark breaking changes with `!`.
- Keep pull requests focused; tests, lint and govulncheck must pass.
- Add a line to the "Unreleased" section of [CHANGELOG.md](CHANGELOG.md) for user-visible changes.
