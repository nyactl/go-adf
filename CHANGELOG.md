# Changelog

All notable changes are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## Unreleased

### Added

- `WithPassthrough` keeps what Markdown cannot express, such as app macros (`extension`, `bodiedExtension`, `inlineExtension`), panels, layouts, status lozenges and colored text: `ToMarkdown` writes them as their JSON in fenced `adf` blocks and `adf:` inline code, and `FromMarkdown` reads them back unchanged, failing with the new `ErrRawNode` for invalid ones.
- `WithRawNodes` lets an application write placeholders for the nodes `WithPassthrough` would write as JSON and put the nodes back, or refuse others, when reading.
- `WithExtendedSyntax` adds Markdown forms for expands (`<details>`), column layouts (a `columns` block of `column` blocks), smart links (`<card:…>`, `<card-block:…>`, `<card-embed:…>`), text color and highlight (`{color:#RRGGBB}…{/color}`, `{highlight:#RRGGBB}…{/highlight}`), the width setting of expands and layouts, and images on a line of their own, with `WithMediaResolver` for attachments. New errors `ErrSyntax` and `*MediaError`.

### Changed

- `ToMarkdown` reads numbers in attributes exactly, so large numbers are no longer rounded.
- Text that looks like a color delimiter of `WithExtendedSyntax`, such as `{color:#ff5630}`, is escaped in every mode.

## [0.1.0] - 2026-10-12

### Added

- `FromMarkdown` and `ToMarkdown` with the options `WithMentionText`, `WithMentionResolver` and `WithMaxDepth`.
- Output of `FromMarkdown` is valid against Atlassian's ADF JSON schema.
- Markdown written by `ToMarkdown` reads back as the same document, with minimal escaping.
- Fuzz targets for validity, robustness and round-trip stability, and a coverage table for every schema type.

[0.1.0]: https://github.com/nyactl/go-adf/releases/tag/v0.1.0
