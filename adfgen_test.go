package adf

import (
	"encoding/json"
	"strings"
	"testing"
)

// genDoc builds an ADF document from fuzz bytes, using node and mark
// combinations Jira writes but FromMarkdown never produces.
type genDoc struct {
	data []byte
	pos  int
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
