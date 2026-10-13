# Changelog

All notable changes are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## Unreleased

### Added

- `WithPassthrough` keeps what Markdown cannot express, such as app macros (`extension`, `bodiedExtension`, `inlineExtension`), panels, layouts, status lozenges and colored text: `ToMarkdown` writes them as their JSON in fenced `adf` blocks and `adf:` inline code, and `FromMarkdown` reads them back unchanged, failing with the new `ErrRawNode` for invalid ones.

### Changed

- `ToMarkdown` reads numbers in attributes exactly, so large numbers are no longer rounded.

## [0.1.0] - 2026-10-12

### Added

- `FromMarkdown` and `ToMarkdown` with the options `WithMentionText`, `WithMentionResolver` and `WithMaxDepth`.
- Output of `FromMarkdown` is valid against Atlassian's ADF JSON schema.
- Markdown written by `ToMarkdown` reads back as the same document, with minimal escaping.
- Fuzz targets for validity, robustness and round-trip stability, and a coverage table for every schema type.

[0.1.0]: https://github.com/nyactl/go-adf/releases/tag/v0.1.0
