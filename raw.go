package adf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// With WithPassthrough, nodes that Markdown cannot express are written as
// their JSON: blocks as a fenced code block with the info string rawInfo,
// inline nodes as inline code starting with rawPrefix.
const (
	rawInfo   = "adf"
	rawPrefix = "adf:"
)

// plainMarks are the marks Markdown expresses.
var plainMarks = set("link", "em", "strong", "strike", "code")

// inlineTypes are the node types that belong inside a paragraph or heading.
var inlineTypes = set("text", "hardBreak", "mention", "emoji", "inlineCard", "date", "status", "placeholder", "inlineExtension", "mediaInline")

// blockTypes are the known node types that do not.
var blockTypes = set("doc", "paragraph", "heading", "bulletList", "orderedList", "listItem", "taskList", "taskItem", "blockTaskItem",
	"decisionList", "decisionItem", "codeBlock", "blockquote", "panel", "rule", "table", "tableRow", "tableHeader", "tableCell",
	"expand", "nestedExpand", "mediaSingle", "mediaGroup", "media", "caption", "blockCard", "embedCard", "extension",
	"bodiedExtension", "layoutSection", "layoutColumn", "multiBodiedExtension", "extensionFrame", "syncBlock", "bodiedSyncBlock")

// plainBlock reports whether a block node has a Markdown form that reads
// back as the same node types. Its inline content and children are checked
// where they are written.
func plainBlock(n Node) bool {
	for _, m := range n.Marks {
		if !plainMarks[m.Type] {
			return false
		}
	}
	switch n.Type {
	case "paragraph", "bulletList", "orderedList", "blockquote", "rule":
		return len(n.Marks) == 0
	case "heading":
		for _, c := range n.Content {
			if c.Type == "hardBreak" {
				return false
			}
		}
		return len(n.Marks) == 0
	case "codeBlock":
		// Code in the language columns is plain without
		// WithExtendedSyntax only; renderer.plainBlock checks that.
		lang := strings.Fields(strAttr(n.Attrs, "language"))
		return len(lang) == 0 || lang[0] != rawInfo
	case "table":
		return plainTable(n)
	}
	return false
}

// plainTable reports whether a table reads back as itself: a header row,
// then rows of the same width, with one paragraph or nothing per cell and
// no merged cells.
func plainTable(n Node) bool {
	if len(n.Content) == 0 {
		return false
	}
	width := len(n.Content[0].Content)
	for i, row := range n.Content {
		if row.Type != "tableRow" || len(row.Content) != width || width == 0 {
			return false
		}
		want := "tableCell"
		if i == 0 {
			want = "tableHeader"
		}
		for _, cell := range row.Content {
			if cell.Type != want || intAttr(cell.Attrs, "colspan", 1) != 1 || intAttr(cell.Attrs, "rowspan", 1) != 1 {
				return false
			}
			if len(cell.Content) > 1 || (len(cell.Content) == 1 && (cell.Content[0].Type != "paragraph" || len(cell.Content[0].Marks) > 0)) {
				return false
			}
		}
	}
	return true
}

// plainInline reports whether an inline node has a Markdown form; ext adds
// the forms of WithExtendedSyntax.
func plainInline(n Node, ext bool) bool {
	switch n.Type {
	case "text":
		for _, m := range n.Marks {
			colored := ext && colorMark(m) && !hasMark(n.Marks, "code")
			if !plainMarks[m.Type] && !colored {
				return false
			}
		}
		return !hasMark(n.Marks, "code") || !strings.HasPrefix(n.Text, rawPrefix)
	case "hardBreak", "mention":
		return true
	case "inlineCard":
		return ext && cardNode(n, "card:", "url")
	}
	return false
}

func rawJSON(n Node) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(n); err != nil {
		// A Node decoded from JSON always encodes again.
		return "{}"
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func rawBlock(n Node) string {
	j := rawJSON(n)
	fence := codeFence(j)
	return fence + rawInfo + "\n" + j + "\n" + fence
}

// rawSpan writes inline nodes as one span: two spans side by side would
// read as one, since their backticks join.
func rawSpan(nodes []Node) string {
	if len(nodes) == 1 {
		return codeSpan(rawPrefix + rawJSON(nodes[0]))
	}
	parts := make([]string, len(nodes))
	for i, n := range nodes {
		parts[i] = rawJSON(n)
	}
	return codeSpan(rawPrefix + "[" + strings.Join(parts, ",") + "]")
}

// spanMember reports whether an inline node goes into a raw span next to
// it: one without a Markdown form, or inline code, whose backticks would
// join the span's.
func spanMember(n Node, ext bool) bool {
	if !plainInline(n, ext) {
		return true
	}
	_, isLink := linkOf(n)
	return n.Type == "text" && hasMark(n.Marks, "code") && !isLink
}

// parseSpan reads the JSON of an adf span: one inline node or an array
// of them.
func parseSpan(s string) ([]Node, error) {
	if !strings.HasPrefix(strings.TrimSpace(s), "[") {
		n, err := parseRaw(s, true)
		if err != nil {
			return nil, err
		}
		return []Node{n}, nil
	}
	var parts []json.RawMessage
	if err := json.Unmarshal([]byte(s), &parts); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrRawNode, err)
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("%w: empty list", ErrRawNode)
	}
	out := make([]Node, len(parts))
	for i, p := range parts {
		n, err := parseRaw(string(p), true)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

// parseRaw reads the JSON of one node. Numbers keep their exact text, so
// the node is written back as it was.
func parseRaw(s string, inline bool) (Node, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var n Node
	if err := dec.Decode(&n); err != nil {
		return Node{}, fmt.Errorf("%w: %w", ErrRawNode, err)
	}
	if dec.More() {
		return Node{}, fmt.Errorf("%w: more than one JSON value", ErrRawNode)
	}
	switch {
	case n.Type == "" || n.Type == "doc":
		return Node{}, fmt.Errorf("%w: a node needs a type other than doc", ErrRawNode)
	case inline && blockTypes[n.Type]:
		return Node{}, fmt.Errorf("%w: %s is a block node and cannot be inline", ErrRawNode, n.Type)
	case !inline && inlineTypes[n.Type]:
		return Node{}, fmt.Errorf("%w: %s is an inline node; write it as inline code", ErrRawNode, n.Type)
	case n.Type == "text" && n.Text == "":
		return Node{}, fmt.Errorf("%w: a text node needs text", ErrRawNode)
	}
	return n, nil
}
