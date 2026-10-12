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

// WithMaxDepth sets the nesting limit for both directions.
func WithMaxDepth(n int) Option {
	return func(c *config) {
		if n > 0 {
			c.maxDepth = n
		}
	}
}
