package adf

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// genDoc builds an ADF document from fuzz bytes, using node and mark
// combinations Jira writes but FromMarkdown never produces.
type genDoc struct {
	data []byte
	pos  int
	// raw adds nodes without a Markdown form, for WithPassthrough.
	raw bool
}

// genRawInline and genRawBlock are nodes without a Markdown form; the
// generator counts them to check that WithPassthrough keeps every one.
var genRawInline = []Node{
	{Type: "status", Attrs: map[string]any{"text": "DONE", "color": "green", "localId": "s"}},
	{Type: "inlineExtension", Attrs: map[string]any{"extensionType": "t", "extensionKey": "k`|*", "parameters": map[string]any{"a": map[string]any{"b": []any{json.Number("1"), "`x`", nil}}}}},
	{Type: "text", Text: "colored *", Marks: []Mark{{Type: "em"}, {Type: "textColor", Attrs: map[string]any{"color": "#ff5630"}}}},
	{Type: "emoji", Attrs: map[string]any{"shortName": ":smile:", "text": "😄"}},
}

var genRawBlock = []Node{
	{Type: "extension", Attrs: map[string]any{"extensionType": "t", "extensionKey": "toc", "parameters": map[string]any{"deep": map[string]any{"er": map[string]any{"est": "```\n> - #"}}}}},
	{Type: "rule", Marks: []Mark{{Type: "breakout", Attrs: map[string]any{"mode": "wide"}}}},
	{Type: "decisionList", Attrs: map[string]any{"localId": "d"}, Content: []Node{{Type: "decisionItem", Attrs: map[string]any{"localId": "e", "state": "DECIDED"}, Content: []Node{{Type: "text", Text: "x"}}}}},
}

func (g *genDoc) next(n int) int {
	if g.pos >= len(g.data) || n <= 0 {
		return 0
	}
	b := g.data[g.pos]
	g.pos++
	return int(b) % n
}

var genTexts = []string{"a", "word", " ", "two words", "*", "_", "~", "`", "[", "]", "(", ")", "!", "#", "-", "1.", ">", "|", "\\", "<", "&amp;", "x_y", "  ", "é", "0"}

func (g *genDoc) marks() []Mark {
	var marks []Mark
	if g.next(3) == 0 {
		marks = append(marks, linkMark([]string{"https://example.com", "https://example.com/a b", "x"}[g.next(3)]))
	}
	if g.next(4) == 0 {
		return append(marks, Mark{Type: "code"})
	}
	for _, m := range []string{"em", "strong", "strike"} {
		if g.next(3) == 0 {
			marks = append(marks, Mark{Type: m})
		}
	}
	return marks
}

func (g *genDoc) inline() []Node {
	var out []Node
	for range g.next(8) + 1 {
		if g.raw && g.next(5) == 0 {
			out = append(out, genRawInline[g.next(len(genRawInline))])
			continue
		}
		switch g.next(10) {
		case 0:
			out = append(out, Node{Type: "hardBreak"})
		case 1:
			out = append(out, Node{Type: "mention", Attrs: map[string]any{"id": "acc-1", "text": "@Alice"}})
		default:
			out = append(out, Node{Type: "text", Text: genTexts[g.next(len(genTexts))], Marks: g.marks()})
		}
	}
	return out
}

func (g *genDoc) blocks(depth int) []Node {
	var out []Node
	for range g.next(3) + 1 {
		if g.raw && g.next(4) == 0 {
			switch k := g.next(len(genRawBlock) + 2); {
			case k < len(genRawBlock):
				out = append(out, genRawBlock[k])
			case k == len(genRawBlock) && depth < 3:
				out = append(out, Node{Type: "panel", Attrs: map[string]any{"panelType": "note"}, Content: g.blocks(depth + 1)})
			default:
				out = append(out, Node{Type: "bodiedExtension", Attrs: map[string]any{"extensionType": "t", "extensionKey": "details"}, Content: []Node{{Type: "paragraph", Content: g.inline()}}})
			}
			continue
		}
		switch g.next(6) {
		case 0:
			out = append(out, Node{Type: "heading", Attrs: map[string]any{"level": float64(g.next(6) + 1)}, Content: g.inline()})
		case 1:
			if depth < 3 {
				var items []Node
				for range g.next(3) + 1 {
					items = append(items, Node{Type: "listItem", Content: g.blocks(depth + 1)})
				}
				out = append(out, Node{Type: []string{"bulletList", "orderedList"}[g.next(2)], Content: items})
				continue
			}
			fallthrough
		case 2:
			out = append(out, Node{Type: "codeBlock", Content: []Node{{Type: "text", Text: "x := `1`\n```\nend"}}})
		default:
			out = append(out, Node{Type: "paragraph", Content: g.inline()})
		}
	}
	return out
}

// FuzzADFRoundTrip checks the direction most applications use: ADF written by Jira
// becomes Markdown that reads back as itself, through schema-valid ADF.
func FuzzADFRoundTrip(f *testing.F) {
	f.Add([]byte{0})
	f.Add([]byte("some seed bytes for the generator"))
	f.Add([]byte{3, 1, 4, 1, 5, 9, 2, 6, 5, 3, 5, 8, 9, 7, 9, 3, 2, 3, 8, 4, 6, 2, 6, 4})
	labels := WithMentionText(func(id, text string) string { return strings.TrimPrefix(text, "@") })
	resolve := WithMentionResolver(func(label string) (string, string, bool) { return "acc-1", label, label == "Alice" })
	f.Fuzz(func(t *testing.T, data []byte) {
		g := &genDoc{data: data}
		raw, err := json.Marshal(Node{Type: "doc", Version: 1, Content: g.blocks(0)})
		if err != nil {
			t.Fatal(err)
		}
		md1, err := ToMarkdown(raw, labels)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := FromMarkdown(md1, resolve)
		if err != nil {
			t.Fatalf("reading back %q: %v", md1, err)
		}
		validate(t, doc)
		md2, err := ToMarkdown(doc, labels)
		if err != nil {
			t.Fatal(err)
		}
		if md1 != md2 {
			t.Fatalf("not stable\nadf: %s\nfirst: %q\nsecond: %q", raw, md1, md2)
		}
	})
}

// rawNodes counts, by JSON, the nodes of the kinds the generator adds for
// WithPassthrough, at any depth.
func rawNodes(t *testing.T, raw []byte, into map[string]int) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var n Node
	if err := dec.Decode(&n); err != nil {
		t.Fatal(err)
	}
	var walk func(Node)
	walk = func(n Node) {
		switch n.Type {
		case "status", "inlineExtension", "emoji", "extension", "decisionList", "panel", "bodiedExtension":
			into[rawJSON(n)]++
		case "text", "rule":
			if !plainInline(n) || len(n.Marks) > 0 && n.Type == "rule" {
				into[rawJSON(n)]++
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(n)
}

// FuzzPassthroughRoundTrip checks that WithPassthrough keeps every node
// without a Markdown form exactly, wherever it is, and that the Markdown is
// stable after one trip.
func FuzzPassthroughRoundTrip(f *testing.F) {
	f.Add([]byte{0})
	f.Add([]byte("some seed bytes for the generator"))
	f.Add([]byte{3, 1, 4, 1, 5, 9, 2, 6, 5, 3, 5, 8, 9, 7, 9, 3, 2, 3, 8, 4, 6, 2, 6, 4})
	f.Add([]byte{0, 0, 1, 0, 0, 0, 2, 0, 0, 3, 1, 0, 4, 0, 0, 1, 1, 1, 0, 0, 2, 2, 0, 1})
	opts := []Option{WithPassthrough(), WithMentionText(func(id, text string) string { return strings.TrimPrefix(text, "@") })}
	back := []Option{WithPassthrough(), WithMentionResolver(func(label string) (string, string, bool) { return "acc-1", label, label == "Alice" })}
	f.Fuzz(func(t *testing.T, data []byte) {
		g := &genDoc{data: data, raw: true}
		raw, err := json.Marshal(Node{Type: "doc", Version: 1, Content: g.blocks(0)})
		if err != nil {
			t.Fatal(err)
		}
		md1, err := ToMarkdown(raw, opts...)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := FromMarkdown(md1, back...)
		if err != nil {
			t.Fatalf("reading back %q: %v", md1, err)
		}
		want, got := map[string]int{}, map[string]int{}
		rawNodes(t, raw, want)
		rawNodes(t, doc, got)
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("nodes without a Markdown form changed\nadf: %s\nmd: %q\nout: %s", raw, md1, doc)
		}
		md2, err := ToMarkdown(doc, opts...)
		if err != nil {
			t.Fatal(err)
		}
		if md1 != md2 {
			t.Fatalf("not stable\nadf: %s\nfirst: %q\nsecond: %q", raw, md1, md2)
		}
	})
}
