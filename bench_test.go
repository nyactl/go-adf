package adf

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// pathological holds inputs known to make Markdown parsers or emphasis
// handling slow when done naively.
var pathological = map[string]string{
	"open brackets":     strings.Repeat("[", 10000),
	"nested brackets":   strings.Repeat("[a", 5000) + strings.Repeat("](b)", 5000),
	"stars":             strings.Repeat("*", 10000),
	"alternating marks": strings.Repeat("*a_", 3000),
	"backticks":         strings.Repeat("`a", 5000),
	"nested quotes":     strings.Repeat(">", 5000) + " x",
	"nested lists":      nestedList(90),
	"many links":        strings.Repeat("[a](https://example.com) ", 2000),
	"many exclamations": strings.Repeat("![a](b)", 2000),
	"long line":         strings.Repeat("word ", 20000),
	"many lines":        strings.Repeat("a\n", 10000),
	"html":              strings.Repeat("<a>", 5000),
	"entities":          strings.Repeat("&amp;", 5000),
	"underscores":       strings.Repeat("_a", 5000),
}

func nestedList(depth int) string {
	var b strings.Builder
	for i := range depth {
		b.WriteString(strings.Repeat("  ", i) + "- x\n")
	}
	return b.String()
}

// TestPathologicalInputsAreFast guards against quadratic behaviour: each
// input must convert both ways, with the default limits, well within a
// second (also under the race detector).
func TestPathologicalInputsAreFast(t *testing.T) {
	if testing.Short() {
		t.Skip("slow")
	}
	for name, in := range pathological {
		start := time.Now()
		doc, err := FromMarkdown(in)
		if err == nil {
			_, err = ToMarkdown(doc)
		}
		if err != nil && !errors.Is(err, ErrTooDeep) {
			t.Errorf("%s: %v", name, err)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("%s took %v", name, d)
		}
	}
}

func BenchmarkFromMarkdown(b *testing.B) {
	in := strings.Repeat("## Title\n\nSome **bold** and *em* text with `code` and [a link](https://example.com).\n\n- one\n- two\n\n", 50)
	for range b.N {
		if _, err := FromMarkdown(in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkToMarkdown(b *testing.B) {
	in := strings.Repeat("## Title\n\nSome **bold** and *em* text with `code` and [a link](https://example.com).\n\n- one\n- two\n\n", 50)
	doc, err := FromMarkdown(in)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		if _, err := ToMarkdown(doc); err != nil {
			b.Fatal(err)
		}
	}
}
