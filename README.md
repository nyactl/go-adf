# go-adf

[![Go Reference](https://pkg.go.dev/badge/github.com/nyactl/go-adf.svg)](https://pkg.go.dev/github.com/nyactl/go-adf)
[![CI](https://github.com/nyactl/go-adf/actions/workflows/ci.yml/badge.svg)](https://github.com/nyactl/go-adf/actions/workflows/ci.yml)

Convert between Markdown and the **Atlassian Document Format (ADF)**, the JSON format Jira Cloud and Confluence Cloud use for descriptions, comments and pages, in both directions, in pure Go.

```go
doc, err := adf.FromMarkdown("Fixed in **v2.1**, see [notes](https://example.com/notes).")
md, err := adf.ToMarkdown(issue.Fields.Description)
```

## Guarantees

These hold for every input, and are checked by tests and continuous fuzzing:

- **Valid ADF.** Everything `FromMarkdown` produces validates against Atlassian's published ADF JSON schema. Markdown that ADF cannot hold in a given place is rewritten into the closest valid form (a heading inside a list item becomes a bold paragraph, nested quotes are flattened), instead of producing a document Jira rejects.
- **Stable round trips.** Markdown written by `ToMarkdown` reads back as the same document. Text is escaped exactly as much as CommonMark needs: a `#`, `*`, `1.` or `[x]` in a ticket stays literal text and does not turn into a heading, emphasis, a list or a link. Where Markdown cannot express a piece of formatting, it is left out rather than written in a way that reads back differently.
- **Safe for untrusted input.** Neither direction panics on any input, nesting is limited (`WithMaxDepth`), and inputs known to slow down Markdown processing (thousands of brackets, delimiters or nested lists) are tested to convert quickly.

[COVERAGE.md](COVERAGE.md) lists how every node and mark type of the ADF schema is converted.

## Install

```sh
go get github.com/nyactl/go-adf
```

Requires Go 1.23 or newer. The only dependency is [goldmark](https://github.com/yuin/goldmark), a CommonMark parser.

## Use

```go
import adf "github.com/nyactl/go-adf"

// Markdown to ADF, e.g. for the body of a Jira comment.
doc, err := adf.FromMarkdown("## Steps\n\n1. Run `make test`\n2. Ship it")

// ADF to Markdown, e.g. a Jira description.
md, err := adf.ToMarkdown(rawDescription)
```

`FromMarkdown` reads CommonMark with the GitHub extensions for tables, strikethrough and task lists. `ToMarkdown` accepts any ADF document, including node types this version does not know, whose text content is kept. A JSON string instead of a document, which some Jira fields still hold, is returned as is.

### Mentions

Mentions are written as `@[label]`. Two hooks decide how people are named and found, so an application can, for example, keep real names out of the Markdown it shows:

```go
md, err := adf.ToMarkdown(raw, adf.WithMentionText(func(accountID, text string) string {
	return labels.For(accountID) // "Person A"
}))

doc, err := adf.FromMarkdown("Thanks @[Person A]!", adf.WithMentionResolver(func(label string) (accountID, text string, ok bool) {
	return labels.Lookup(label)
}))
```

Without a resolver, `@[label]` stays plain text. When the resolver does not know a label, `FromMarkdown` returns a `*MentionError`.

## How it is tested

- Every document produced in the tests is validated against Atlassian's ADF JSON schema (`testdata/schema`), and a test makes sure that check can fail.
- Four fuzz targets run in CI: Markdown always converts to valid ADF; ADF never makes `ToMarkdown` panic; Markdown is stable after one round trip; and randomly generated ADF, with the formatting combinations Jira writes, is stable as well. Every input fuzzing has ever found is kept in `testdata/fuzz` and runs with the normal tests.
- A coverage test fails when Atlassian's schema gains a type that [COVERAGE.md](COVERAGE.md) does not describe.

## Versioning

go-adf follows [Semantic Versioning](https://semver.org/). Until v1.0.0, the API may still change between minor versions. From v1.0.0 on:

- The Go API does not break within a major version; new node types and options are additions.
- The exact Markdown text may change in a minor release when that makes the conversion more faithful. The guarantees above always hold.

Changes are listed in [CHANGELOG.md](CHANGELOG.md).

## Contributing

Bug reports with an input that converts wrongly are especially welcome. See [CONTRIBUTING.md](CONTRIBUTING.md); security problems go through [SECURITY.md](SECURITY.md).

## License

MIT, see [LICENSE](LICENSE). The ADF JSON schema used by the tests is Atlassian's, under the Apache License 2.0, see [testdata/schema](testdata/schema/README.md).

Not affiliated with or endorsed by Atlassian. Jira, Confluence and Atlassian are trademarks of Atlassian.
