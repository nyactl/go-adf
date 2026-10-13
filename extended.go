package adf

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// The forms of WithExtendedSyntax; see there.

const (
	cardInline = "card:"
	cardBlock  = "card-block:"
	cardEmbed  = "card-embed:"
	attachment = "attachment:"
)

// Children allowed in the containers WithExtendedSyntax adds, limited to the
// node types this converter produces.
var (
	expandChildren       = set("paragraph", "heading", "bulletList", "orderedList", "codeBlock", "blockquote", "rule", "table", "mediaSingle", "blockCard", "embedCard", "nestedExpand")
	nestedExpandChildren = set("paragraph", "heading", "bulletList", "orderedList", "codeBlock", "blockquote", "rule", "mediaSingle")
	layoutColumnChildren = set("paragraph", "heading", "bulletList", "orderedList", "codeBlock", "blockquote", "rule", "table", "mediaSingle", "blockCard", "embedCard", "expand")
)

var (
	hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	// colorTokenRe finds the color delimiters in Markdown and, to escape
	// them, in text.
	colorTokenRe   = regexp.MustCompile(`\{(color|highlight):(#[0-9a-fA-F]{6})\}|\{/(color|highlight)\}`)
	detailsOpenRe  = regexp.MustCompile(`^<details(?: data-breakout="([^"]*)")?>\s*(?:<summary>([^\n]*)</summary>)?\s*$`)
	detailsCloseRe = regexp.MustCompile(`^</details>\s*$`)
	// Numbers are written as JSON numbers, so they must be valid ones.
	breakoutRe = regexp.MustCompile(`^(wide|full-width)(?::((?:0|[1-9][0-9]*)(?:\.[0-9]+)?))?$`)
	widthRe    = regexp.MustCompile(`^((?:0|[1-9][0-9]*)(?:\.[0-9]+)?)(px|%)$`)
	numberRe   = regexp.MustCompile(`^(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)
)

var mediaLayouts = set("wide", "full-width", "center", "wrap-right", "wrap-left", "align-end", "align-start")

// colorMarks are the color marks, by the name of their delimiter.
var colorMarks = map[string]string{"color": "textColor", "highlight": "backgroundColor"}

func colorMark(m Mark) bool {
	return (m.Type == "textColor" || m.Type == "backgroundColor") && hexColorRe.MatchString(strAttr(m.Attrs, "color")) && len(m.Attrs) == 1
}

// attrsWithin reports whether a node has no attributes other than keys.
func attrsWithin(n Node, keys ...string) bool {
	for k := range n.Attrs {
		found := false
		for _, want := range keys {
			found = found || k == want
		}
		if !found {
			return false
		}
	}
	return true
}

// cardNode reports whether a card can be written as an autolink with the
// given scheme.
func cardNode(n Node, scheme string, extra ...string) bool {
	url := strAttr(n.Attrs, "url")
	return url != "" && len(n.Marks) == 0 && attrsWithin(n, append([]string{"url", "localId"}, extra...)...) && autolinkRe.MatchString(scheme+url)
}

func numText(v any) (string, bool) {
	switch v := v.(type) {
	case json.Number:
		return v.String(), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case int:
		return strconv.Itoa(v), true
	}
	return "", false
}

// breakout returns the width setting of a block as "mode" or "mode:width",
// and whether its marks are only that setting.
func breakout(marks []Mark) (string, bool) {
	switch {
	case len(marks) == 0:
		return "", true
	case len(marks) > 1 || marks[0].Type != "breakout":
		return "", false
	}
	m := marks[0]
	mode := strAttr(m.Attrs, "mode")
	if mode != "wide" && mode != "full-width" {
		return "", false
	}
	out := mode
	for k, v := range m.Attrs {
		switch k {
		case "mode":
		case "width":
			w, ok := numText(v)
			if !ok || !breakoutRe.MatchString(mode+":"+w) {
				return "", false
			}
			out += ":" + w
		default:
			return "", false
		}
	}
	return out, true
}

func breakoutMarks(s string) ([]Mark, bool) {
	if s == "" {
		return nil, true
	}
	m := breakoutRe.FindStringSubmatch(s)
	if m == nil {
		return nil, false
	}
	attrs := map[string]any{"mode": m[1]}
	if m[2] != "" {
		attrs["width"] = json.Number(m[2])
	}
	return []Mark{{Type: "breakout", Attrs: attrs}}, true
}

// extBlock reports whether WithExtendedSyntax writes a block node so that
// it reads back as the same nodes. root tells whether it is at the top of
// the document.
func (r renderer) extBlock(n Node) bool {
	switch n.Type {
	case "expand", "nestedExpand":
		mode, ok := breakout(n.Marks)
		title, hasTitle := n.Attrs["title"].(string)
		return ok && (mode == "" || (n.Type == "expand" && !r.nested)) && attrsWithin(n, "title", "localId") &&
			(!hasTitle || !strings.Contains(title, "\n")) && len(n.Content) > 0
	case "layoutSection":
		_, ok := breakout(n.Marks)
		if !ok || r.nested || !attrsWithin(n, "localId") || len(n.Content) < 2 || len(n.Content) > 3 {
			return false
		}
		for _, col := range n.Content {
			if _, ok := numText(col.Attrs["width"]); col.Type != "layoutColumn" || !ok || len(col.Marks) > 0 || !attrsWithin(col, "width", "localId") {
				return false
			}
		}
		return true
	case "blockCard":
		return cardNode(n, cardBlock)
	case "embedCard":
		return strAttr(n.Attrs, "layout") == "center" && cardNode(n, cardEmbed, "layout")
	case "mediaSingle":
		_, ok := r.image(n)
		return ok
	}
	return false
}

// expand writes an expand as <details>.
func (r renderer) expand(n Node) string {
	var b strings.Builder
	b.WriteString("<details")
	if mode, ok := breakout(n.Marks); ok && mode != "" && !r.nested && n.Type == "expand" {
		b.WriteString(` data-breakout="` + mode + `"`)
	}
	b.WriteString(">")
	if title, ok := n.Attrs["title"].(string); ok && (title != "" || !r.inExpand) {
		b.WriteString("\n<summary>" + html.EscapeString(strings.ReplaceAll(title, "\n", " ")) + "</summary>")
	}
	// Read back, an expand inside an expand is a nestedExpand.
	allowed := expandChildren
	if r.inExpand {
		allowed = nestedExpandChildren
	}
	rr := r
	rr.inExpand = true
	body := rr.container(n.Content, allowed)
	b.WriteString("\n\n")
	if body != "" {
		b.WriteString(body + "\n\n")
	}
	b.WriteString("</details>")
	return b.String()
}

// container writes the blocks of a container that accepts allowed.
func (r renderer) container(nodes []Node, allowed map[string]bool) string {
	rr := r
	rr.nested = true
	if r.raw {
		rr.allowed = allowed
		return rr.blocks(nodes)
	}
	return rr.blocks(fitMarkdown(nodes, allowed))
}

// columns writes a layout as a columns block of column blocks.
func (r renderer) columns(n Node) string {
	parts := make([]string, 0, len(n.Content))
	for _, col := range n.Content {
		body := r.container(col.Content, layoutColumnChildren)
		info := "column"
		if w, ok := numText(col.Attrs["width"]); ok {
			info += " " + w
		}
		fence := codeFence(body)
		if body == "" {
			parts = append(parts, fence+info+"\n"+fence)
			continue
		}
		parts = append(parts, fence+info+"\n"+body+"\n"+fence)
	}
	inner := strings.Join(parts, "\n\n")
	info := "columns"
	if mode, ok := breakout(n.Marks); ok && mode != "" {
		info += " breakout=" + mode
	}
	fence := codeFence(inner)
	return fence + info + "\n" + inner + "\n" + fence
}

// image writes an image on a line of its own, if it reads back the same.
func (r renderer) image(n Node) (string, bool) {
	if len(n.Marks) > 0 || len(n.Content) != 1 || !attrsWithin(n, "layout", "width", "widthType") {
		return "", false
	}
	var opts []string
	if l, ok := n.Attrs["layout"]; ok {
		s, _ := l.(string)
		if !mediaLayouts[s] {
			return "", false
		}
		opts = append(opts, s)
	} else if len(n.Attrs) > 0 {
		return "", false
	}
	if w, ok := n.Attrs["width"]; ok {
		num, ok := numText(w)
		switch {
		case !ok:
			return "", false
		case strAttr(n.Attrs, "widthType") == "pixel" && numberRe.MatchString(num):
			opts = append(opts, num+"px")
		case n.Attrs["widthType"] == nil && numberRe.MatchString(num):
			if v, _ := strconv.ParseFloat(num, 64); v > 100 {
				return "", false
			}
			opts = append(opts, num+"%")
		default:
			return "", false
		}
	} else if n.Attrs["widthType"] != nil {
		return "", false
	}
	m := n.Content[0]
	alt := strAttr(m.Attrs, "alt")
	var dest string
	switch {
	case m.Type != "media" || len(m.Marks) > 0 || strings.ContainsAny(alt, "\n\r"):
		return "", false
	case strAttr(m.Attrs, "type") == "file" && r.media && alt != "":
		dest = attachment + alt
	case strAttr(m.Attrs, "type") == "external" && attrsWithin(m, "type", "url", "alt") && strAttr(m.Attrs, "url") != "" && !strings.HasPrefix(strAttr(m.Attrs, "url"), attachment):
		dest = strAttr(m.Attrs, "url")
	default:
		return "", false
	}
	title := ""
	if len(opts) > 0 {
		title = ` "` + strings.Join(opts, " ") + `"`
	}
	return "![" + escapeText(alt, false) + "](" + linkDestination(dest) + title + ")", true
}

// colorKey is the color and highlight of a text node, as written. Text
// that WithPassthrough writes as JSON keeps its colors there.
func colorKey(n Node) string {
	if n.Type != "text" || hasMark(n.Marks, "code") || !plainInline(n, true) {
		return ""
	}
	var color, highlight string
	for _, m := range n.Marks {
		if colorMark(m) {
			if m.Type == "textColor" {
				color = strAttr(m.Attrs, "color")
			} else {
				highlight = strAttr(m.Attrs, "color")
			}
		}
	}
	if color == "" && highlight == "" {
		return ""
	}
	return color + "/" + highlight
}

// colored writes inline content with colored text, if there is any: each
// run of the same colors between its delimiters, with the whitespace at its
// edges outside.
func (r renderer) colored(nodes []Node) (string, bool) {
	found := false
	for _, n := range nodes {
		found = found || colorKey(n) != ""
	}
	if !found {
		return "", false
	}
	var b strings.Builder
	// space writes whitespace between runs: one space or line break, none
	// at the start of a line.
	space := func(ws string) {
		s := b.String()
		switch {
		case ws == "":
		case ws == "\n":
			b.Reset()
			b.WriteString(strings.TrimRight(s, " ") + "\n")
		case s != "" && !strings.HasSuffix(s, " ") && !strings.HasSuffix(s, "\n"):
			b.WriteString(" ")
		}
	}
	for i := 0; i < len(nodes); {
		if nodes[i].Type == "hardBreak" {
			// A writer drops a line break at its start, so it is written
			// here.
			space("\n")
			i++
			continue
		}
		key := colorKey(nodes[i])
		j := i + 1
		for j < len(nodes) && colorKey(nodes[j]) == key && nodes[j].Type != "hardBreak" {
			j++
		}
		run := make([]Node, j-i)
		copy(run, nodes[i:j])
		var lead, trail string
		if first := &run[0]; first.Type == "text" {
			core := strings.TrimLeftFunc(first.Text, unicode.IsSpace)
			lead, first.Text = normalSpace(first.Text[:len(first.Text)-len(core)]), core
		}
		if last := &run[len(run)-1]; last.Type == "text" {
			core := strings.TrimRightFunc(last.Text, unicode.IsSpace)
			trail, last.Text = normalSpace(last.Text[len(core):]), core
		}
		var open, closing string
		if key != "" {
			parts := strings.SplitN(key, "/", 2)
			if parts[0] != "" {
				open += "{color:" + parts[0] + "}"
				closing = "{/color}" + closing
			}
			if parts[1] != "" {
				open += "{highlight:" + parts[1] + "}"
				closing = "{/highlight}" + closing
			}
			for k := range run {
				var marks []Mark
				for _, m := range run[k].Marks {
					if !colorMark(m) {
						marks = append(marks, m)
					}
				}
				run[k].Marks = marks
			}
		}
		inner := r.writeInline(run)
		space(lead)
		if inner != "" {
			b.WriteString(open + inner + closing)
		}
		space(trail)
		i = j
	}
	return strings.TrimFunc(b.String(), func(r rune) bool { return r == ' ' || r == '\n' }), true
}

// colorTokens splits Markdown text at unescaped color delimiters, which
// become placeholder nodes for applyColors.
func colorTokens(raw []byte, marks []Mark) []Node {
	var out []Node
	last := 0
	for _, loc := range colorTokenRe.FindAllSubmatchIndex(raw, -1) {
		if escapedAt(raw, loc[0]) {
			continue
		}
		out = append(out, textNodes(literal(raw[last:loc[0]]), marks)...)
		tok := Node{Type: tokenType, Text: string(raw[loc[0]:loc[1]]), Marks: append([]Mark(nil), marks...)}
		if loc[2] >= 0 {
			tok.Attrs = map[string]any{"kind": string(raw[loc[2]:loc[3]]), "color": string(raw[loc[4]:loc[5]])}
		} else {
			tok.Attrs = map[string]any{"kind": string(raw[loc[6]:loc[7]]), "close": true}
		}
		out = append(out, tok)
		last = loc[1]
	}
	return append(out, textNodes(literal(raw[last:]), marks)...)
}

// tokenType marks color delimiters between colorTokens and applyColors; it
// cannot occur in a document.
const tokenType = "\x00color"

// applyColors adds the color marks between matching delimiters to the text
// in between. Delimiters without a partner stay text.
func applyColors(nodes []Node) []Node {
	matched := make([]bool, len(nodes))
	open := map[string][]int{}
	for i, n := range nodes {
		if n.Type != tokenType {
			continue
		}
		kind := n.Attrs["kind"].(string)
		if n.Attrs["close"] == nil {
			open[kind] = append(open[kind], i)
			continue
		}
		if s := open[kind]; len(s) > 0 {
			matched[s[len(s)-1]], matched[i] = true, true
			open[kind] = s[:len(s)-1]
		}
	}
	active := map[string][]string{}
	var out []Node
	for i, n := range nodes {
		if n.Type == tokenType {
			if !matched[i] {
				out = append(out, Node{Type: "text", Text: n.Text, Marks: n.Marks})
				continue
			}
			kind := n.Attrs["kind"].(string)
			if n.Attrs["close"] == nil {
				active[kind] = append(active[kind], n.Attrs["color"].(string))
			} else {
				active[kind] = active[kind][:len(active[kind])-1]
			}
			continue
		}
		if n.Type == "text" && !hasMark(n.Marks, "code") {
			for _, kind := range []string{"color", "highlight"} {
				if s := active[kind]; len(s) > 0 {
					n.Marks = addMark(n.Marks, Mark{Type: colorMarks[kind], Attrs: map[string]any{"color": s[len(s)-1]}})
				}
			}
		}
		out = append(out, n)
	}
	return out
}

// detailsEnd finds the </details> before stop that closes the <details> at
// open.
func (c *converter) detailsEnd(open, stop ast.Node) ast.Node {
	depth := 0
	for n := open.NextSibling(); n != nil && n != stop; n = n.NextSibling() {
		h, ok := n.(*ast.HTMLBlock)
		if !ok {
			continue
		}
		s := c.htmlText(h)
		switch {
		case detailsOpenRe.MatchString(s):
			depth++
		case detailsCloseRe.MatchString(s):
			if depth == 0 {
				return n
			}
			depth--
		}
	}
	return nil
}

func (c *converter) htmlText(h *ast.HTMLBlock) string {
	s := c.lines(h)
	if h.HasClosure() {
		seg := h.ClosureLine
		s += string(seg.Value(c.src))
	}
	return s
}

// expandNode converts the blocks between <details> at open and its end.
func (c *converter) expandNode(open, end ast.Node, root bool) []Node {
	m := detailsOpenRe.FindStringSubmatch(c.htmlText(open.(*ast.HTMLBlock)))
	n := Node{Type: "expand"}
	allowed := expandChildren
	if c.inExpand > 0 {
		n.Type, allowed = "nestedExpand", nestedExpandChildren
	}
	if m[1] != "" && root && n.Type == "expand" {
		if marks, ok := breakoutMarks(m[1]); ok {
			n.Marks = marks
		}
	}
	// A nestedExpand needs attributes; its title can be empty.
	if strings.Contains(c.htmlText(open.(*ast.HTMLBlock)), "<summary>") || n.Type == "nestedExpand" {
		n.Attrs = map[string]any{"title": html.UnescapeString(m[2])}
	}
	c.inExpand++
	n.Content = nonEmpty(c.siblings(open.NextSibling(), end, allowed))
	c.inExpand--
	return []Node{n}
}

// columnsNode converts a columns block.
func (c *converter) columnsNode(n *ast.FencedCodeBlock) []Node {
	if c.depth >= c.cfg.maxDepth {
		c.fail(ErrTooDeep)
		return nil
	}
	inner := c.sub(c.lines(n))
	root := md.Parser().Parse(text.NewReader(inner.src))
	if !hasColumn(root, inner.src) {
		// Code in a language called columns.
		return nil
	}
	info := strings.Fields(string(n.Info.Segment.Value(c.src)))
	section := Node{Type: "layoutSection"}
	for _, opt := range info[1:] {
		mode, ok := strings.CutPrefix(opt, "breakout=")
		marks, valid := breakoutMarks(mode)
		if !ok || !valid || section.Marks != nil {
			c.fail(fmt.Errorf("%w: unknown columns option %q", ErrSyntax, opt))
			return nil
		}
		section.Marks = marks
	}
	var widths []string
	for col := root.FirstChild(); col != nil; col = col.NextSibling() {
		f, ok := col.(*ast.FencedCodeBlock)
		if !ok || string(f.Language(inner.src)) != "column" {
			c.fail(fmt.Errorf("%w: a columns block holds only column blocks", ErrSyntax))
			return nil
		}
		fields := strings.Fields(string(f.Info.Segment.Value(inner.src)))
		width := ""
		switch len(fields) {
		case 1:
		case 2:
			if v, err := strconv.ParseFloat(fields[1], 64); err != nil || !numberRe.MatchString(fields[1]) || v < 0 || v > 100 {
				c.fail(fmt.Errorf("%w: column width %q is not a number from 0 to 100", ErrSyntax, fields[1]))
				return nil
			}
			width = fields[1]
		default:
			c.fail(fmt.Errorf("%w: a column takes only its width", ErrSyntax))
			return nil
		}
		widths = append(widths, width)
		body := inner.sub(inner.lines(f))
		content := body.children(md.Parser().Parse(text.NewReader(body.src)), layoutColumnChildren)
		c.fail(body.err)
		section.Content = append(section.Content, Node{Type: "layoutColumn", Content: nonEmpty(content)})
	}
	c.fail(inner.err)
	if len(section.Content) < 2 || len(section.Content) > 3 {
		c.fail(fmt.Errorf("%w: a columns block holds 2 or 3 columns; its fence must be longer than theirs, as in ````columns", ErrSyntax))
		return nil
	}
	given := 0
	for _, w := range widths {
		if w != "" {
			given++
		}
	}
	if given != 0 && given != len(widths) {
		c.fail(fmt.Errorf("%w: give the width of every column or of none", ErrSyntax))
		return nil
	}
	for i := range section.Content {
		w := widths[i]
		if given == 0 {
			w = map[int]string{2: "50", 3: "33.33"}[len(widths)]
		}
		section.Content[i].Attrs = map[string]any{"width": json.Number(w)}
	}
	return []Node{section}
}

// hasColumn reports whether a columns block holds a column block, so that
// it is meant as a layout.
func hasColumn(root ast.Node, src []byte) bool {
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		if f, ok := n.(*ast.FencedCodeBlock); ok && string(f.Language(src)) == "column" {
			return true
		}
	}
	return false
}

// sub returns a converter for Markdown nested in a block of this one.
func (c *converter) sub(src string) *converter {
	return &converter{src: []byte(src), cfg: c.cfg, depth: c.depth + 1, inExpand: c.inExpand}
}

// cardParagraph converts a paragraph that is only a block card.
func (c *converter) cardParagraph(n ast.Node) ([]Node, bool) {
	link, ok := n.FirstChild().(*ast.AutoLink)
	if !ok || n.FirstChild() != n.LastChild() {
		return nil, false
	}
	url := string(link.URL(c.src))
	if rest, ok := strings.CutPrefix(url, cardBlock); ok && rest != "" {
		return []Node{{Type: "blockCard", Attrs: map[string]any{"url": rest}}}, true
	}
	if rest, ok := strings.CutPrefix(url, cardEmbed); ok && rest != "" {
		return []Node{{Type: "embedCard", Attrs: map[string]any{"url": rest, "layout": "center"}}}, true
	}
	return nil, false
}

// imageParagraph converts a paragraph that is only an image. An image with
// a title other than layout and width options stays a link.
func (c *converter) imageParagraph(n ast.Node) ([]Node, bool) {
	img, ok := n.FirstChild().(*ast.Image)
	if !ok || n.FirstChild() != n.LastChild() {
		return nil, false
	}
	single := Node{Type: "mediaSingle"}
	for _, opt := range strings.Fields(literal(img.Title)) {
		if single.Attrs == nil {
			single.Attrs = map[string]any{}
		}
		if mediaLayouts[opt] && single.Attrs["layout"] == nil {
			single.Attrs["layout"] = opt
			continue
		}
		if m := widthRe.FindStringSubmatch(opt); m != nil && single.Attrs["width"] == nil {
			if v, _ := strconv.ParseFloat(m[1], 64); m[2] == "%" && v > 100 {
				return nil, false
			}
			single.Attrs["width"] = json.Number(m[1])
			if m[2] == "px" {
				single.Attrs["widthType"] = "pixel"
			}
			continue
		}
		return nil, false
	}
	// ADF needs a layout with any other image setting.
	if single.Attrs != nil && single.Attrs["layout"] == nil {
		single.Attrs["layout"] = "center"
	}
	dest := literal(img.Destination)
	if name, ok := strings.CutPrefix(dest, attachment); ok {
		var media Node
		if c.cfg.resolveMedia != nil {
			media, ok = c.cfg.resolveMedia(name)
		}
		if c.cfg.resolveMedia == nil || !ok || media.Type != "media" {
			c.fail(&MediaError{Name: name})
			return nil, true
		}
		single.Content = []Node{media}
		return []Node{single}, true
	}
	if !strings.HasPrefix(dest, "https://") && !strings.HasPrefix(dest, "http://") {
		return nil, false
	}
	media := Node{Type: "media", Attrs: map[string]any{"type": "external", "url": dest}}
	if alt := plainText(c.inlines(img, nil)); alt != "" {
		media.Attrs["alt"] = alt
	}
	single.Content = []Node{media}
	return []Node{single}, true
}
