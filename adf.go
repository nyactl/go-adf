// Package adf converts between Markdown and the Atlassian Document Format
// (ADF), the JSON format Jira Cloud and Confluence Cloud use for rich text.
//
// FromMarkdown always produces documents that validate against Atlassian's
// published ADF JSON schema: Markdown constructs that ADF cannot express in
// a given place are rewritten into the closest valid form (for example a
// heading inside a list item becomes a bold paragraph). ToMarkdown accepts
// any ADF document and falls back to the text content of node types it does
// not know.
//
// Both directions are safe for untrusted input: they never panic, and
// nesting deeper than the configured limit is rejected with ErrTooDeep.
package adf

import (
	"errors"
	"fmt"
)

// Node is an ADF node.
type Node struct {
	Type    string         `json:"type"`
	Version int            `json:"version,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []Node         `json:"content,omitempty"`
	Text    string         `json:"text,omitempty"`
	Marks   []Mark         `json:"marks,omitempty"`
}

// Mark is formatting applied to a text node, such as strong or link.
type Mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

// DefaultMaxDepth is the nesting limit used unless WithMaxDepth sets one.
const DefaultMaxDepth = 100

// ErrTooDeep is returned for documents nested deeper than the limit.
var ErrTooDeep = errors.New("adf: document is nested too deeply")

// ErrSyntax is returned by FromMarkdown, with WithExtendedSyntax, for a
// columns block whose column blocks are not as described there.
var ErrSyntax = errors.New("adf: invalid extended syntax")

// MediaError is returned by FromMarkdown when an image refers to an
// attachment that the media resolver does not know, or when there is no
// resolver.
type MediaError struct {
	Name string
}

func (e *MediaError) Error() string {
	return fmt.Sprintf("adf: cannot resolve attachment %q", e.Name)
}

// ErrRawNode is returned by FromMarkdown, with WithPassthrough, for an adf
// block or span that does not hold one valid node for its place.
var ErrRawNode = errors.New("adf: invalid adf block")

// MentionError is returned by FromMarkdown when the mention resolver does not
// know a @[label].
type MentionError struct {
	Label string
}

func (e *MentionError) Error() string {
	return fmt.Sprintf("adf: cannot resolve mention @[%s]", e.Label)
}

// Option configures a conversion. Options that do not apply to a direction
// are ignored.
type Option func(*config)

type config struct {
	mentionText    func(accountID, text string) string
	resolveMention func(label string) (accountID, text string, ok bool)
	maxDepth       int
	passthrough    bool
	extended       bool
	resolveMedia   func(name string) (media Node, ok bool)
	rawNodes       RawNodes
}

func newConfig(opts []Option) *config {
	c := &config{maxDepth: DefaultMaxDepth}
	for _, o := range opts {
		o(c)
	}
	return c
}

// WithMentionText sets how ToMarkdown names a mentioned person. It receives
// the account ID and the text stored with the mention (usually "@Name") and
// returns the label written as @[label]. By default the stored text is used.
func WithMentionText(f func(accountID, text string) string) Option {
	return func(c *config) { c.mentionText = f }
}

// WithMentionResolver makes FromMarkdown turn @[label] into mention nodes.
// The resolver returns the account ID and the display name to store; when it
// reports false, FromMarkdown fails with a *MentionError. Without a resolver,
// @[label] stays plain text.
func WithMentionResolver(f func(label string) (accountID, text string, ok bool)) Option {
	return func(c *config) { c.resolveMention = f }
}

// WithPassthrough keeps what Markdown cannot express, so that
// FromMarkdown(ToMarkdown(doc)) keeps it unchanged. ToMarkdown writes such a
// block node, for example an app macro (extension, bodiedExtension), a panel
// or a table with merged cells, as a fenced code block with the info string
// adf holding the node's JSON. Such an inline node, for example a status or
// an inlineExtension, and text with formatting other than links, emphasis,
// strikethrough and code, become inline code holding adf: and the JSON.
// FromMarkdown turns both forms back into the node, after checking that it
// is one JSON object with a type that fits its place, and fails with
// ErrRawNode otherwise. Attributes of nodes that do have a Markdown form,
// such as table widths, are still not kept.
//
// Callers that take Markdown from others decide which of these nodes they
// accept: the option lets the author write any node.
func WithPassthrough() Option {
	return func(c *config) { c.passthrough = true }
}

// RawNodes decides, with WithRawNodes, what stands in the Markdown for the
// nodes WithPassthrough writes as JSON. An application can write a
// placeholder instead of a node whose content it does not want to show, and
// put the node back when reading, or refuse nodes it did not write.
type RawNodes interface {
	// Write returns the node to write as JSON in place of n.
	Write(n Node) Node
	// Read returns the node to use for n, read from an adf block or, when
	// inline is set, from adf inline code. An error makes FromMarkdown fail
	// with it.
	Read(n Node, inline bool) (Node, error)
}

// WithRawNodes turns on WithPassthrough and passes every node it writes or
// reads as JSON through h. Inline code that a span of nodes takes along is
// not passed.
func WithRawNodes(h RawNodes) Option {
	return func(c *config) {
		c.passthrough = true
		c.rawNodes = h
	}
}

// WithExtendedSyntax adds Markdown forms for elements of the Confluence and
// Jira editors that CommonMark lacks. Both directions use them:
//
//   - expand: <details>, an optional <summary>Title</summary> line and the
//     body, closed by </details>; inside an expand it is a nestedExpand. The
//     editor's width setting is kept as data-breakout="wide:760".
//   - column layout: a fenced block with the info string columns holding one
//     fenced block per column with the info string column and its width in
//     percent, e.g. "column 50"; the width setting as in breakout=wide:760.
//   - smart links: <card:URL> inline, <card-block:URL> and <card-embed:URL>
//     on a line of their own.
//   - text color and highlight: {color:#RRGGBB}text{/color} and
//     {highlight:#RRGGBB}text{/highlight}.
//   - images on a line of their own: ![alt](https://...) for external images,
//     and ![name](attachment:name) for attachments, with WithMediaResolver.
//     The title can hold the layout and the width: "center 600px" or "wide 50%".
//
// Text that only looks like one of these forms is escaped, and code in the
// language columns loses that language, which would make it a layout (with
// WithPassthrough it is kept). A columns block with malformed column blocks
// makes FromMarkdown fail with ErrSyntax; one without any column block is
// code, and an image with a title other than these options stays a link.
func WithExtendedSyntax() Option {
	return func(c *config) { c.extended = true }
}

// WithMediaResolver lets WithExtendedSyntax write images stored as
// attachments. ToMarkdown names such an image by its alt text, usually the
// file name, and FromMarkdown asks the resolver for the media node of that
// name; when it reports false, FromMarkdown fails with a *MediaError.
// Without a resolver, ToMarkdown keeps writing attachments as before.
func WithMediaResolver(f func(name string) (media Node, ok bool)) Option {
	return func(c *config) { c.resolveMedia = f }
}

// WithMaxDepth sets the nesting limit for both directions.
func WithMaxDepth(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.maxDepth = n
		}
	}
}
