package adf

import (
	"encoding/json"
	"html"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var (
	mentionRe = regexp.MustCompile(`@\[([^\[\]\n]{1,100})\]`)
	brRe      = regexp.MustCompile(`(?i)^<br\s*/?>$`)
)

var md = goldmark.New(goldmark.WithExtensions(extension.Strikethrough, extension.Table, extension.TaskList))

// FromMarkdown converts CommonMark with the GitHub extensions for tables,
// strikethrough and task list checkboxes to an ADF document.
//
// Supported: paragraphs, headings, bullet and ordered lists, fenced and
// indented code, block quotes, thematic breaks, tables, strong, emphasis,
// strikethrough, inline code, links, autolinks and, with
// WithMentionResolver, @[label] mentions. Images become links. Raw HTML is
// kept as text.
func FromMarkdown(markdown string, opts ...Option) (json.RawMessage, error) {
	cfg := newConfig(opts)
	// JSON replaces invalid UTF-8 with U+FFFD; parse what will be stored.
	src := []byte(lineEndings.Replace(strings.ToValidUTF8(markdown, "\uFFFD")))
	c := &converter{src: src, cfg: cfg}
	root := md.Parser().Parse(text.NewReader(src))
	content := c.children(root, nil)
	if c.err != nil {
		return nil, c.err
	}
	if content == nil {
		content = []Node{}
	}
	if tooDeep(Node{Content: content}, cfg.maxDepth) {
		return nil, ErrTooDeep
	}
	// Unlike other nodes, the document must have content even when empty.
	return json.Marshal(struct {
		Type    string `json:"type"`
		Version int    `json:"version"`
		Content []Node `json:"content"`
	}{"doc", 1, content})
}

type converter struct {
	src []byte
	cfg *config
	err error
	// depth counts the Markdown documents nested in columns blocks, and
	// inExpand the expands around the current block.
	depth, inExpand int
}

// Children allowed by the ADF schema in containers that are stricter than
// the document root, limited to the node types this converter produces.
// Table cells only ever get a paragraph, which every container accepts.
var (
	listItemChildren   = set("paragraph", "bulletList", "orderedList", "codeBlock", "mediaSingle")
	blockquoteChildren = set("paragraph", "bulletList", "orderedList", "codeBlock", "mediaSingle")
)

func set(types ...string) map[string]bool {
	m := make(map[string]bool, len(types))
	for _, t := range types {
		m[t] = true
	}
	return m
}

// children converts the block children of n. allowed restricts the node
// types to those the containing ADF node accepts; nil means any.
func (c *converter) children(n ast.Node, allowed map[string]bool) []Node {
	return c.siblings(n.FirstChild(), nil, allowed)
}

// siblings converts the blocks from first up to stop; nil means to the end.
func (c *converter) siblings(first, stop ast.Node, allowed map[string]bool) []Node {
	var out []Node
	for ch := first; ch != nil && ch != stop; ch = ch.NextSibling() {
		var nodes []Node
		if end := c.expandEnd(ch, stop); end != nil {
			nodes = c.expandNode(ch, end, allowed == nil)
			ch = end
		} else {
			nodes = c.block(ch)
		}
		// A passed-through node is kept where its author put it.
		if allowed != nil && !c.isRaw(ch) {
			nodes = fit(nodes, allowed)
		}
		out = append(out, nodes...)
	}
	return out
}

// expandEnd returns the end of an expand starting at n, or nil.
func (c *converter) expandEnd(n, stop ast.Node) ast.Node {
	h, ok := n.(*ast.HTMLBlock)
	if !ok || !c.cfg.extended || !detailsOpenRe.MatchString(c.htmlText(h)) {
		return nil
	}
	return c.detailsEnd(n, stop)
}

// isRaw reports whether n is an adf block read with WithPassthrough.
func (c *converter) isRaw(n ast.Node) bool {
	f, ok := n.(*ast.FencedCodeBlock)
	return ok && c.cfg.passthrough && string(f.Language(c.src)) == rawInfo
}

func (c *converter) block(n ast.Node) []Node {
	switch n := n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		if c.cfg.extended {
			if nodes, ok := c.cardParagraph(n); ok {
				return nodes
			}
			if nodes, ok := c.imageParagraph(n); ok {
				return nodes
			}
		}
		return []Node{{Type: "paragraph", Content: c.inlineBlock(n)}}
	case *ast.Heading:
		return []Node{{Type: "heading", Attrs: map[string]any{"level": n.Level}, Content: c.inlineBlock(n)}}
	case *ast.ThematicBreak:
		return []Node{{Type: "rule"}}
	case *ast.FencedCodeBlock:
		if c.cfg.extended && string(n.Language(c.src)) == "columns" {
			if nodes := c.columnsNode(n); nodes != nil || c.err != nil {
				return nodes
			}
		}
		if c.isRaw(n) {
			node, err := parseRaw(c.lines(n), false)
			if err != nil {
				c.fail(err)
				return nil
			}
			return []Node{node}
		}
		node := codeBlock(c.lines(n))
		if lang := string(n.Language(c.src)); lang != "" {
			node.Attrs = map[string]any{"language": lang}
		}
		return []Node{node}
	case *ast.CodeBlock:
		return []Node{codeBlock(c.lines(n))}
	case *ast.HTMLBlock:
		return []Node{{Type: "paragraph", Content: textNodes(strings.TrimRight(c.lines(n), "\n"), nil)}}
	case *ast.Blockquote:
		return []Node{{Type: "blockquote", Content: nonEmpty(c.children(n, blockquoteChildren))}}
	case *ast.List:
		list := Node{Type: "bulletList"}
		if n.IsOrdered() {
			list.Type = "orderedList"
			if n.Start > 1 {
				list.Attrs = map[string]any{"order": n.Start}
			}
		}
		for item := n.FirstChild(); item != nil; item = item.NextSibling() {
			content := c.children(item, listItemChildren)
			// A list item starts with a paragraph or code block in Jira's
			// editor, even though the JSON schema does not say so.
			if len(content) == 0 || (content[0].Type != "paragraph" && content[0].Type != "codeBlock" && !c.isRaw(item.FirstChild())) {
				content = append([]Node{{Type: "paragraph"}}, content...)
			}
			list.Content = append(list.Content, Node{Type: "listItem", Content: content})
		}
		return []Node{list}
	case *extast.Table:
		table := Node{Type: "table"}
		for row := n.FirstChild(); row != nil; row = row.NextSibling() {
			cellType := "tableCell"
			if _, ok := row.(*extast.TableHeader); ok {
				cellType = "tableHeader"
			}
			r := Node{Type: "tableRow"}
			for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
				r.Content = append(r.Content, Node{Type: cellType, Content: []Node{{Type: "paragraph", Content: c.inlineBlock(cell)}}})
			}
			table.Content = append(table.Content, r)
		}
		return []Node{table}
	default:
		return c.children(n, nil)
	}
}

// fit rewrites nodes a container does not accept into the closest form it
// does: headings become bold paragraphs, quotes are unwrapped, tables become
// one paragraph per row and rules a paragraph reading "---".
func fit(nodes []Node, allowed map[string]bool) []Node {
	var out []Node
	for _, n := range nodes {
		if allowed[n.Type] {
			out = append(out, n)
			continue
		}
		switch n.Type {
		case "heading":
			out = append(out, Node{Type: "paragraph", Content: withMark(n.Content, Mark{Type: "strong"})})
		case "blockquote":
			out = append(out, fit(n.Content, allowed)...)
		case "table":
			for _, row := range n.Content {
				var inline []Node
				for i, cell := range row.Content {
					if i > 0 {
						inline = append(inline, Node{Type: "text", Text: " | "})
					}
					for _, b := range cell.Content {
						inline = append(inline, b.Content...)
					}
				}
				out = append(out, Node{Type: "paragraph", Content: mergeText(inline, false)})
			}
		case "rule":
			out = append(out, Node{Type: "paragraph", Content: textNodes("---", nil)})
		case "expand", "nestedExpand":
			out = append(out, fit(n.Content, allowed)...)
		case "layoutSection":
			for _, col := range n.Content {
				out = append(out, fit(col.Content, allowed)...)
			}
		case "blockCard", "embedCard":
			url := strAttr(n.Attrs, "url")
			out = append(out, Node{Type: "paragraph", Content: textNodes(url, []Mark{linkMark(url)})})
		default:
			out = append(out, Node{Type: "paragraph", Content: textNodes(plainText(n.Content), nil)})
		}
	}
	return out
}

func nonEmpty(nodes []Node) []Node {
	if len(nodes) == 0 {
		return []Node{{Type: "paragraph"}}
	}
	return nodes
}

func (c *converter) lines(n ast.Node) string {
	var b strings.Builder
	lines := n.Lines()
	for i := range lines.Len() {
		seg := lines.At(i)
		b.Write(seg.Value(c.src))
	}
	return b.String()
}

func codeBlock(code string) Node {
	code = strings.TrimSuffix(code, "\n")
	n := Node{Type: "codeBlock"}
	if code != "" {
		n.Content = []Node{{Type: "text", Text: code}}
	}
	return n
}

// inlineBlock converts the inline children of a block and resolves mentions.
func (c *converter) inlineBlock(n ast.Node) []Node {
	nodes := c.inlines(n, nil)
	if c.cfg.extended {
		nodes = applyColors(nodes)
	}
	nodes = mergeText(nodes, c.cfg.extended)
	nodes = c.mentions(nodes)
	for len(nodes) > 0 && nodes[len(nodes)-1].Type == "hardBreak" {
		nodes = nodes[:len(nodes)-1]
	}
	for len(nodes) > 0 && nodes[0].Type == "hardBreak" {
		nodes = nodes[1:]
	}
	return nodes
}

func (c *converter) inlines(parent ast.Node, marks []Mark) []Node {
	var out []Node
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		switch n := n.(type) {
		case *ast.Text:
			if c.cfg.extended {
				out = append(out, colorTokens(n.Value(c.src), marks)...)
			} else {
				out = append(out, textNodes(literal(n.Value(c.src)), marks)...)
			}
			if n.SoftLineBreak() || n.HardLineBreak() {
				out = append(out, Node{Type: "hardBreak"})
			}
		case *ast.String:
			out = append(out, textNodes(string(n.Value), marks)...)
		case *ast.CodeSpan:
			var b strings.Builder
			for t := n.FirstChild(); t != nil; t = t.NextSibling() {
				if tx, ok := t.(*ast.Text); ok {
					b.Write(tx.Value(c.src))
				}
			}
			code := strings.ReplaceAll(b.String(), "\n", " ")
			if c.cfg.passthrough && strings.HasPrefix(code, rawPrefix) {
				nodes, err := parseSpan(code[len(rawPrefix):])
				if err != nil {
					c.fail(err)
					continue
				}
				out = append(out, nodes...)
				continue
			}
			if strings.TrimSpace(code) == "" {
				out = append(out, textNodes(code, marks)...)
				continue
			}
			// Inline code may only be combined with a link.
			codeMarks := []Mark{{Type: "code"}}
			for _, m := range marks {
				if m.Type == "link" {
					codeMarks = append(codeMarks, m)
				}
			}
			out = append(out, textNodes(code, codeMarks)...)
		case *ast.Emphasis:
			mark := Mark{Type: "em"}
			if n.Level >= 2 {
				mark.Type = "strong"
			}
			out = append(out, c.inlines(n, addMark(marks, mark))...)
		case *extast.Strikethrough:
			out = append(out, c.inlines(n, addMark(marks, Mark{Type: "strike"}))...)
		case *ast.Link:
			out = append(out, c.inlines(n, addMark(marks, linkMark(literal(n.Destination))))...)
		case *ast.AutoLink:
			url := string(n.URL(c.src))
			if rest, ok := strings.CutPrefix(url, cardInline); ok && c.cfg.extended && rest != "" {
				out = append(out, Node{Type: "inlineCard", Attrs: map[string]any{"url": rest}})
				continue
			}
			out = append(out, textNodes(string(n.Label(c.src)), addMark(marks, linkMark(url)))...)
		case *ast.Image:
			alt := c.inlines(n, addMark(marks, linkMark(literal(n.Destination))))
			if len(alt) == 0 {
				alt = textNodes(literal(n.Destination), addMark(marks, linkMark(literal(n.Destination))))
			}
			out = append(out, alt...)
		case *ast.RawHTML:
			var b strings.Builder
			for i := range n.Segments.Len() {
				seg := n.Segments.At(i)
				b.Write(seg.Value(c.src))
			}
			// <br> is how GFM tables write line breaks inside a cell.
			if brRe.MatchString(b.String()) {
				out = append(out, Node{Type: "hardBreak"})
				continue
			}
			out = append(out, textNodes(b.String(), marks)...)
		case *extast.TaskCheckBox:
			box := "[ ] "
			if n.IsChecked {
				box = "[x] "
			}
			out = append(out, textNodes(box, marks)...)
		default:
			out = append(out, c.inlines(n, marks)...)
		}
	}
	return out
}

func (c *converter) fail(err error) {
	if c.err == nil {
		c.err = err
	}
}

// mentions splits @[label] out of text without inline code into mention
// nodes, when a resolver is set.
func (c *converter) mentions(nodes []Node) []Node {
	if c.cfg.resolveMention == nil {
		return nodes
	}
	var out []Node
	for _, n := range nodes {
		if n.Type != "text" || hasMark(n.Marks, "code") || !plainInline(n, c.cfg.extended) || !strings.Contains(n.Text, "@[") {
			out = append(out, n)
			continue
		}
		rest := n.Text
		for {
			loc := mentionRe.FindStringSubmatchIndex(rest)
			if loc == nil {
				break
			}
			out = append(out, textNodes(rest[:loc[0]], n.Marks)...)
			label := rest[loc[2]:loc[3]]
			accountID, display, ok := c.cfg.resolveMention(label)
			if !ok || accountID == "" {
				if c.err == nil {
					c.err = &MentionError{Label: label}
				}
				out = append(out, textNodes(rest[loc[0]:loc[1]], n.Marks)...)
			} else {
				out = append(out, Node{Type: "mention", Attrs: map[string]any{"id": accountID, "text": "@" + strings.TrimPrefix(display, "@")}})
			}
			rest = rest[loc[1]:]
		}
		out = append(out, textNodes(rest, n.Marks)...)
	}
	return out
}

// literal resolves backslash escapes and character references, which
// goldmark leaves in text for its HTML renderer to handle. Both are
// resolved in one pass: an escaped & does not start a reference.
func literal(raw []byte) string {
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); {
		switch c := raw[i]; {
		case c == '\\' && i+1 < len(raw) && util.IsPunct(raw[i+1]):
			out = append(out, raw[i+1])
			i += 2
			continue
		case c == '&':
			if ref := entityRe.Find(raw[i:]); ref != nil {
				if decoded := html.UnescapeString(string(ref)); decoded != string(ref) {
					out = append(out, decoded...)
					i += len(ref)
					continue
				}
			}
		}
		out = append(out, raw[i])
		i++
	}
	return string(out)
}

func textNodes(s string, marks []Mark) []Node {
	if s == "" {
		return nil
	}
	return []Node{{Type: "text", Text: s, Marks: append([]Mark(nil), marks...)}}
}

// mergeText joins neighbouring text with the same marks. ext tells whether
// WithExtendedSyntax is on.
func mergeText(nodes []Node, ext bool) []Node {
	var out []Node
	for _, n := range nodes {
		// Text that WithPassthrough writes as JSON stays as it came.
		if last := len(out) - 1; last >= 0 && n.Type == "text" && out[last].Type == "text" && plainInline(n, ext) && plainInline(out[last], ext) && reflect.DeepEqual(out[last].Marks, n.Marks) {
			out[last].Text += n.Text
			continue
		}
		out = append(out, n)
	}
	return out
}

// markOrder is the canonical order of marks on a text node, so equal
// formatting always compares equal.
var markOrder = map[string]int{"link": 0, "em": 1, "strong": 2, "strike": 3, "code": 4, "textColor": 5, "backgroundColor": 6}

func addMark(marks []Mark, m Mark) []Mark {
	if hasMark(marks, m.Type) {
		return marks
	}
	out := append(append([]Mark(nil), marks...), m)
	sort.SliceStable(out, func(i, j int) bool { return markOrder[out[i].Type] < markOrder[out[j].Type] })
	return out
}

func withMark(nodes []Node, m Mark) []Node {
	out := make([]Node, len(nodes))
	for i, n := range nodes {
		if n.Type == "text" && !hasMark(n.Marks, "code") {
			n.Marks = addMark(n.Marks, m)
		}
		out[i] = n
	}
	return out
}

func hasMark(marks []Mark, typ string) bool {
	for _, m := range marks {
		if m.Type == typ {
			return true
		}
	}
	return false
}

func linkMark(href string) Mark {
	return Mark{Type: "link", Attrs: map[string]any{"href": href}}
}
