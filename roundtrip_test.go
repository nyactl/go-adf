package adf

import "testing"

// roundTrips holds Markdown in the form ToMarkdown writes, which must come
// back unchanged after a trip through ADF.
var roundTrips = []string{
	"plain paragraph",
	"one\n\ntwo",
	"# H1\n\n## H2\n\n###### H6",
	"**strong** *em* ~~strike~~ `code` [link](https://example.com)",
	"line one\nline two",
	"- a\n- b\n- c",
	"1. a\n2. b",
	"3. three\n4. four",
	"- outer\n\n  - inner\n- next",
	"> quoted\n>\n> second paragraph",
	"```go\nfunc main() {}\n```",
	"```\nno language\n```",
	"---",
	"| A | B |\n| --- | --- |\n| 1 | 2 |",
	"[**bold link**](https://example.com)",
	"text with <https://example.com> autolink",
}

func TestRoundTripExact(t *testing.T) {
	t.Parallel()
	for _, in := range roundTrips {
		doc, err := FromMarkdown(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		validate(t, doc)
		out, err := ToMarkdown(doc)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if out != in {
			t.Errorf("round trip changed the text\nin:  %q\nout: %q", in, out)
		}
	}
}
