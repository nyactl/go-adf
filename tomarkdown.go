package adf

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// ToMarkdown renders an ADF document as Markdown. Empty or null input
// yields "", and a JSON string (which some Jira fields still hold instead of
// a document) is returned as is. Unknown node types fall back to their text
// content.
func ToMarkdown(raw json.RawMessage, opts ...Option) (string, error) {
	cfg := newConfig(opts)
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return "", nil
	}
	// Some fields still carry plain strings; keep them as-is.
	if strings.HasPrefix(s, `"`) {
		var str string
		if err := json.Unmarshal(raw, &str); err != nil {
			return "", err
		}
		return str, nil
	}
	var doc Node
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("adf: invalid document: %w", err)
	}
	if tooDeep(doc, cfg.maxDepth) {
		return "", ErrTooDeep
	}
	mention := cfg.mentionText
	if mention == nil {
		mention = func(_, text string) string { return strings.TrimPrefix(text, "@") }
	}
	r := renderer{mention: mention}
	return strings.TrimSpace(r.blocks(doc.Content)), nil
}

type renderer struct {
	mention func(accountID, text string) string
	inTable bool
	// altList selects the second list marker, which keeps two adjacent
	// lists of the same kind from merging into one.
	altList bool
	// noVerify skips the self-check in inline, which itself renders.
	noVerify bool
}

func (r renderer) blocks(nodes []Node) string {
	parts := make([]string, 0, len(nodes))
	prev, prevAlt := "", false
	for _, n := range nodes {
		rr := r
		isList := n.Type == "bulletList" || n.Type == "orderedList"
		rr.altList = isList && n.Type == prev && !prevAlt
		if s := rr.block(n); strings.TrimSpace(s) != "" {
			parts = append(parts, s)
			prev, prevAlt = n.Type, rr.altList
		}
	}
	return strings.Join(parts, "\n\n")
}

func (r renderer) block(n Node) string {
	switch n.Type {
	case "paragraph":
		return r.inline(n.Content)
	case "heading":
		level := min(max(intAttr(n.Attrs, "level", 1), 1), 6)
		// An ATX heading is one line, and a trailing # would be read as its
		// closing sequence.
		content := make([]Node, len(n.Content))
		for i, c := range n.Content {
			if c.Type == "hardBreak" {
				c = Node{Type: "text", Text: " "}
			}
			content[i] = c
		}
		text := strings.ReplaceAll(r.inline(content), "\n", " ")
		if text == "" {
			return ""
		}
		if strings.HasSuffix(text, "#") && !escapedAt(text, len(text)-1) {
			text = text[:len(text)-1] + `\#`
		}
		return strings.TrimRight(strings.Repeat("#", level)+" "+text, " ")
	case "bulletList":
		marker := "- "
		if r.altList {
			marker = "* "
		}
		return r.list(n.Content, func(int) string { return marker })
	case "orderedList":
		start := max(intAttr(n.Attrs, "order", 1), 0)
		delim := ". "
		if r.altList {
			delim = ") "
		}
		return r.list(n.Content, func(i int) string { return strconv.Itoa(start+i) + delim })
	case "taskList":
		return r.list(n.Content, func(int) string { return "" })
	case "taskItem", "blockTaskItem":
		box := "- [ ] "
		if strAttr(n.Attrs, "state") == "DONE" {
			box = "- [x] "
		}
		if n.Type == "blockTaskItem" {
			return box + strings.ReplaceAll(r.blocks(n.Content), "\n\n", "\n")
		}
		return box + r.inline(n.Content)
	case "decisionList":
		return r.list(n.Content, func(int) string { return "- Decision: " })
	case "decisionItem":
		return r.inline(n.Content)
	case "codeBlock":
		code := plainText(n.Content)
		fence := codeFence(code)
		lang := strings.Fields(strings.ReplaceAll(strAttr(n.Attrs, "language"), "`", ""))
		info := ""
		if len(lang) > 0 {
			info = lang[0]
		}
		return fence + info + "\n" + code + "\n" + fence
	case "blockquote":
		return quote(r.blocks(fitMarkdown(n.Content, blockquoteChildren)))
	case "panel":
		label := strAttr(n.Attrs, "panelType")
		if label == "" {
			label = "note"
		}
		return quote("**" + strings.ToUpper(label[:1]) + label[1:] + ":** " + r.blocks(fitMarkdown(n.Content, blockquoteChildren)))
	case "rule":
		return "---"
	case "table":
		return r.table(n)
	case "expand", "nestedExpand":
		title := strAttr(n.Attrs, "title")
		body := r.blocks(n.Content)
		if title == "" {
			return body
		}
		return "**" + title + "**\n\n" + body
	case "mediaSingle", "mediaGroup":
		parts := make([]string, 0, len(n.Content))
		for _, m := range n.Content {
			if m.Type == "caption" {
				if c := r.inline(m.Content); c != "" {
					parts = append(parts, c)
				}
				continue
			}
			parts = append(parts, mediaLabel(m))
		}
		return strings.Join(parts, " ")
	case "media":
		return mediaLabel(n)
	case "blockCard", "embedCard":
		return "<" + strAttr(n.Attrs, "url") + ">"
	default:
		if len(n.Content) > 0 {
			if isInline(n.Content[0]) {
				return r.inline(n.Content)
			}
			return r.blocks(n.Content)
		}
		return r.inline([]Node{n})
	}
}

func (r renderer) list(items []Node, marker func(int) string) string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		i := len(lines)
		if item.Type == "taskItem" || item.Type == "blockTaskItem" || item.Type == "decisionItem" {
			lines = append(lines, marker(i)+r.block(item))
			continue
		}
		body := r.blocks(fitMarkdown(item.Content, listItemChildren))
		// Markdown cannot hold an empty item stably: depending on what
		// follows, it ends the list.
		if strings.TrimSpace(body) == "" {
			continue
		}
		prefix := marker(i)
		indent := strings.Repeat(" ", len(prefix))
		bodyLines := strings.Split(body, "\n")
		for j, l := range bodyLines {
			switch {
			case j == 0:
				bodyLines[j] = prefix + l
			case l == "":
				bodyLines[j] = ""
			default:
				bodyLines[j] = indent + l
			}
		}
		lines = append(lines, strings.Join(bodyLines, "\n"))
	}
	return strings.Join(lines, "\n")
}

func (r renderer) table(n Node) string {
	var rows [][]string
	for _, row := range n.Content {
		var cells []string
		for _, cell := range row.Content {
			cr := r
			cr.inTable = true
			text := strings.ReplaceAll(cr.blocks(cell.Content), "\n\n", "<br>")
			cells = append(cells, strings.ReplaceAll(text, "\n", "<br>"))
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return ""
	}
	cols := 0
	for _, row := range rows {
		cols = max(cols, len(row))
	}
	var b strings.Builder
	writeRow := func(cells []string) {
		b.WriteString("|")
		for i := range cols {
			c := ""
			if i < len(cells) {
				c = cells[i]
			}
			b.WriteString(" " + c + " |")
		}
		b.WriteString("\n")
	}
	writeRow(rows[0])
	b.WriteString("|" + strings.Repeat(" --- |", cols) + "\n")
	for _, row := range rows[1:] {
		writeRow(row)
	}
	return strings.TrimRight(b.String(), "\n")
}

// inline writes inline content. Where Markdown cannot express its
// emphasis exactly (CommonMark's flanking rules leave gaps, such as
// emphasis that ends between punctuation and a letter), the emphasis is
// left out rather than written in a way that reads back differently.
func (r renderer) inline(nodes []Node) string {
	s := r.writeInline(nodes)
	if r.noVerify || !hasEmphasis(nodes) || stable(s) {
		return s
	}
	return r.writeInline(withoutEmphasis(nodes))
}

func hasEmphasis(nodes []Node) bool {
	for _, n := range nodes {
		for _, e := range emphasis {
			if hasMark(n.Marks, e.mark) {
				return true
			}
		}
	}
	return false
}

func withoutEmphasis(nodes []Node) []Node {
	out := make([]Node, len(nodes))
	for i, n := range nodes {
		var marks []Mark
		for _, m := range n.Marks {
			if m.Type != "em" && m.Type != "strong" && m.Type != "strike" {
				marks = append(marks, m)
			}
		}
		n.Marks = marks
		out[i] = n
	}
	return out
}

// stable reports whether s reads back as itself.
func stable(s string) bool {
	doc, err := FromMarkdown(s)
	if err != nil {
		return false
	}
	var root Node
	if err := json.Unmarshal(doc, &root); err != nil {
		return false
	}
	r := renderer{mention: func(_, text string) string { return text }, noVerify: true}
	return strings.TrimSpace(r.blocks(root.Content)) == s
}

func (r renderer) writeInline(nodes []Node) string {
	nodes = r.normalizeInline(nodes)
	w := &inlineWriter{atStart: true}
	for i := 0; i < len(nodes); i++ {
		n := nodes[i]
		if href, ok := linkOf(n); ok {
			// Consecutive text with the same target is one link.
			j := i + 1
			for j < len(nodes) {
				if h, ok := linkOf(nodes[j]); !ok || h != href {
					break
				}
				j++
			}
			w.leaf(r.link(nodes[i:j], href))
			i = j - 1
			continue
		}
		switch n.Type {
		case "text":
			r.textInto(w, n, nodes[i+1:])
		case "hardBreak":
			w.lineBreak()
		default:
			w.leaf(r.leafText(n))
		}
	}
	return w.finish()
}

// leafText renders an inline node other than text and hard breaks.
func (r renderer) leafText(n Node) string {
	switch n.Type {
	case "mention":
		return "@[" + r.mention(strAttr(n.Attrs, "id"), strAttr(n.Attrs, "text")) + "]"
	case "emoji":
		if t := strAttr(n.Attrs, "text"); t != "" {
			return t
		}
		return strAttr(n.Attrs, "shortName")
	case "inlineCard":
		if u := strAttr(n.Attrs, "url"); u != "" {
			return "<" + u + ">"
		}
		return ""
	case "date":
		return formatDate(n.Attrs["timestamp"])
	case "status":
		if t := strAttr(n.Attrs, "text"); t != "" {
			return "[" + t + "]"
		}
		return ""
	case "mediaInline":
		return mediaLabel(n)
	case "placeholder":
		return ""
	}
	return r.inline(n.Content)
}

// normalizeInline removes what renders as nothing and merges the text
// around it, so that escaping and emphasis see the same neighbours as on
// the next round trip, where those pieces are one text.
func (r renderer) normalizeInline(nodes []Node) []Node {
	var out []Node
	for _, n := range nodes {
		switch {
		case n.Type == "text":
			if n.Text == "" {
				continue
			}
			if strings.TrimSpace(n.Text) == "" && hasMark(n.Marks, "code") {
				n.Marks = dropMarks(n.Marks, "code")
			}
		case n.Type == "hardBreak":
		case len(n.Content) == 0 && n.Text != "" && !knownInline[n.Type]:
			// An unknown inline node with text is shown as that text.
			n = Node{Type: "text", Text: n.Text, Marks: n.Marks}
		case r.leafText(n) == "":
			continue
		}
		out = append(out, n)
	}
	// A link whose text is only whitespace cannot be written.
	for i := 0; i < len(out); {
		href, ok := linkOf(out[i])
		if !ok {
			i++
			continue
		}
		j, blank := i, true
		for ; j < len(out); j++ {
			if h, ok := linkOf(out[j]); !ok || h != href {
				break
			}
			blank = blank && strings.TrimSpace(out[j].Text) == ""
		}
		if blank {
			for k := i; k < j; k++ {
				out[k].Marks = dropMarks(out[k].Marks, "link")
			}
		}
		i = j
	}
	// Markdown has no empty lines inside a paragraph: line breaks at its
	// edges or in a row would split it.
	kept := out[:0]
	for i, n := range out {
		if n.Type == "hardBreak" && (len(kept) == 0 || i == len(out)-1 || out[i+1].Type == "hardBreak") {
			continue
		}
		kept = append(kept, n)
	}
	return mergeText(kept)
}

var knownInline = map[string]bool{"text": true, "hardBreak": true, "mention": true, "emoji": true, "inlineCard": true, "date": true, "status": true, "placeholder": true}

func dropMarks(marks []Mark, typ string) []Mark {
	var out []Mark
	for _, m := range marks {
		if m.Type != typ {
			out = append(out, m)
		}
	}
	return out
}

// textInto writes a text node, except its link mark. next are the nodes
// that follow it, used to nest emphasis that continues longer outside.
func (r renderer) textInto(w *inlineWriter, n Node, next []Node) {
	if hasMark(n.Marks, "code") && strings.TrimSpace(n.Text) != "" {
		code := codeSpan(n.Text)
		if r.inTable {
			// GFM tables split cells at every unescaped pipe, even in code.
			code = strings.ReplaceAll(code, "|", `\|`)
		}
		w.leaf(code)
		return
	}
	w.text(n.Text, n.Marks, next)
}

// runOf counts how many of next continue mark as plain text pieces, and
// reports whether the content after that run starts with a letter or digit.
// Whitespace-only pieces are skipped: the writer holds them back without
// closing emphasis.
func runOf(next []Node, mark string) (run int, wordAfter bool) {
	i, spaced := 0, false
	for ; i < len(next); i++ {
		m := next[i]
		if m.Type == "text" && strings.TrimSpace(m.Text) == "" {
			spaced = spaced || m.Text != ""
			continue
		}
		if _, isLink := linkOf(m); m.Type != "text" || isLink || hasMark(m.Marks, "code") || !hasMark(m.Marks, mark) {
			break
		}
		run++
		spaced = false
	}
	if i == len(next) || spaced {
		return run, false
	}
	switch n := next[i]; n.Type {
	case "text":
		r, _ := utf8.DecodeRuneInString(n.Text)
		return run, isWordRune(r)
	case "hardBreak", "mention", "inlineCard", "status", "placeholder":
		return run, false
	}
	return run, true
}

func linkOf(n Node) (string, bool) {
	if n.Type != "text" {
		return "", false
	}
	for _, m := range n.Marks {
		if m.Type == "link" {
			return strAttr(m.Attrs, "href"), true
		}
	}
	return "", false
}

// link writes text nodes that share a link target.
func (r renderer) link(nodes []Node, href string) string {
	if len(nodes) == 1 && len(nodes[0].Marks) == 1 && nodes[0].Text == href && isAutolink(href) {
		return "<" + href + ">"
	}
	w := &inlineWriter{}
	for i, n := range nodes {
		r.textInto(w, n, nodes[i+1:])
	}
	inner := w.finish()
	if inner == "" {
		// A link without text cannot be written.
		return ""
	}
	return "[" + inner + "](" + linkDestination(href) + ")"
}

// emphasis lists the formatting marks written with delimiters, outermost
// first.
var emphasis = []struct{ mark, delim string }{{"strike", "~~"}, {"strong", "**"}, {"em", "*"}}

// inlineWriter writes inline content, keeping emphasis open across text
// pieces that share it: "~~a~~~~b~~" would not parse as two strikethroughs,
// so it writes "~~ab~~". Whitespace at the edge of emphasis is moved
// outside the delimiters, where CommonMark requires it.
type inlineWriter struct {
	b    []byte
	open []openMark // outermost first
	// pending is whitespace after the last text, written once it is clear
	// whether the emphasis around it continues.
	pending string
	atStart bool
	// closedStar is set right after a closing delimiter ending in *.
	closedStar bool
}

type openMark struct {
	mark, delim string
}

func (w *inlineWriter) raw(s string) {
	if s != "" {
		w.b = append(w.b, s...)
		w.atStart = strings.HasSuffix(s, "\n")
		w.closedStar = false
	}
}

// normalSpace keeps a line break, or else one space: other whitespace at
// the edge of text does not survive Markdown.
func normalSpace(ws string) string {
	switch {
	case ws == "":
		return ""
	case strings.Contains(ws, "\n"):
		return "\n"
	}
	return " "
}

func (w *inlineWriter) closeTo(n int) {
	for len(w.open) > n {
		last := w.open[len(w.open)-1]
		w.open = w.open[:len(w.open)-1]
		w.raw(last.delim)
		w.closedStar = strings.HasSuffix(last.delim, "*")
	}
}

func (w *inlineWriter) isOpen(mark string) bool {
	for _, o := range w.open {
		if o.mark == mark {
			return true
		}
	}
	return false
}

// text writes literal text with emphasis marks. next are the nodes that
// follow, used to decide how to nest and delimit emphasis.
func (w *inlineWriter) text(s string, marks []Mark, next []Node) {
	// Split the whitespace off first, so escaping sees the real edges of
	// the text.
	lead, core, trail := splitSpace(s)
	lead, trail = normalSpace(lead), normalSpace(trail)
	if w.atStart && w.pending == "" {
		lead = ""
	}
	if core == "" {
		if !w.atStart {
			w.pending = normalSpace(w.pending + lead + trail)
		}
		return
	}
	lineStart := (w.atStart && w.pending == "" && lead == "") || strings.HasSuffix(w.pending+lead, "\n")
	core = escapeText(core, lineStart)
	keep := 0
	for keep < len(w.open) && hasMark(marks, w.open[keep].mark) {
		keep++
	}
	w.closeTo(keep)
	w.raw(w.pending + lead)
	w.pending = ""
	var opening []int
	for i, e := range emphasis {
		if hasMark(marks, e.mark) && !w.isOpen(e.mark) {
			opening = append(opening, i)
		}
	}
	// Emphasis that goes on longer is opened first, so it can stay open
	// when the shorter one closes.
	sort.SliceStable(opening, func(a, b int) bool {
		ra, _ := runOf(next, emphasis[opening[a]].mark)
		rb, _ := runOf(next, emphasis[opening[b]].mark)
		return ra > rb
	})
	for _, i := range opening {
		delim := emphasis[i].delim
		// Right after a closing *, an opening * would join its delimiter
		// run. Underscores avoid that, but cannot close in front of a
		// letter or digit; then this emphasis is left out.
		if strings.HasPrefix(delim, "*") && w.closedStar {
			if _, wordAfter := runOf(next, emphasis[i].mark); wordAfter {
				continue
			}
			delim = strings.Repeat("_", len(delim))
		}
		w.raw(delim)
		w.open = append(w.open, openMark{emphasis[i].mark, delim})
	}
	if n := len(w.b); strings.HasPrefix(core, "(") && n > 0 && w.b[n-1] == ']' && !escapedAt(w.b, n-1) {
		// After a mention or status, which end in ], ( would start a link
		// target.
		core = `\` + core
	}
	w.raw(core)
	w.pending = trail
}

// leaf writes content that emphasis delimiters must not touch.
func (w *inlineWriter) leaf(s string) {
	if s == "" {
		return
	}
	w.closeTo(0)
	w.raw(w.pending)
	w.pending = ""
	if strings.HasPrefix(s, "[") {
		// A ! right before a link would turn it into an image.
		if n := len(w.b); n > 0 && w.b[n-1] == '!' && !escapedAt(w.b, n-1) {
			w.b = append(w.b[:n-1], `\!`...)
		}
	}
	w.raw(s)
}

func (w *inlineWriter) lineBreak() {
	if len(w.b) == 0 || w.atStart {
		return
	}
	w.closeTo(0)
	w.pending = ""
	w.raw("\n")
}

func (w *inlineWriter) finish() string {
	w.closeTo(0)
	return escapeLineStarts(strings.TrimRightFunc(string(w.b), unicode.IsSpace))
}

func quote(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + l
		}
	}
	return strings.Join(lines, "\n")
}

func mediaLabel(n Node) string {
	if alt := strAttr(n.Attrs, "alt"); alt != "" {
		return "[attachment: " + alt + "]"
	}
	return "[attachment]"
}

func plainText(nodes []Node) string {
	var b strings.Builder
	for _, n := range nodes {
		if n.Type == "hardBreak" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(n.Text)
		b.WriteString(plainText(n.Content))
	}
	return b.String()
}

func isInline(n Node) bool {
	switch n.Type {
	case "text", "hardBreak", "mention", "emoji", "inlineCard", "date", "status", "placeholder":
		return true
	}
	return false
}

func strAttr(attrs map[string]any, key string) string {
	if v, ok := attrs[key].(string); ok {
		return v
	}
	return ""
}

func intAttr(attrs map[string]any, key string, def int) int {
	switch v := attrs[key].(type) {
	case float64:
		return int(v)
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func formatDate(v any) string {
	var ms int64
	switch t := v.(type) {
	case string:
		ms, _ = strconv.ParseInt(t, 10, 64)
	case float64:
		ms = int64(t)
	}
	if ms == 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.DateOnly)
}

func tooDeep(n Node, limit int) bool {
	if limit <= 0 {
		return true
	}
	for _, ch := range n.Content {
		if tooDeep(ch, limit-1) {
			return true
		}
	}
	return false
}

// fitMarkdown applies the container rules FromMarkdown uses (see fit) to
// the node types it rewrites, so that what ToMarkdown writes inside a list
// item or quote reads back unchanged even when the ADF breaks the rules.
// Panels and expands are written as quotes or bold titles, so inside such
// containers only their content is kept.
func fitMarkdown(nodes []Node, allowed map[string]bool) []Node {
	var out []Node
	for _, n := range nodes {
		switch n.Type {
		case "heading", "blockquote", "table", "rule":
			out = append(out, fit([]Node{n}, allowed)...)
		case "panel", "expand", "nestedExpand":
			out = append(out, fitMarkdown(n.Content, allowed)...)
		default:
			out = append(out, n)
		}
	}
	return out
}
