package adf

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func seedMarkdown(f *testing.F) {
	for _, s := range awkward {
		f.Add(s)
	}
	for _, s := range roundTrips {
		f.Add(s)
	}
}

// FuzzFromMarkdown checks that any input converts without panicking and
// yields schema-valid ADF.
func FuzzFromMarkdown(f *testing.F) {
	seedMarkdown(f)
	f.Fuzz(func(t *testing.T, in string) {
		out, err := FromMarkdown(in)
		if errors.Is(err, ErrTooDeep) {
			return
		}
		if err != nil {
			t.Fatalf("FromMarkdown(%q): %v", in, err)
		}
		validate(t, out)
	})
}

// FuzzRoundTrip checks that Markdown settles after one round trip: once a
// text has been through ADF, converting it again changes nothing.
func FuzzRoundTrip(f *testing.F) {
	seedMarkdown(f)
	f.Fuzz(func(t *testing.T, in string) {
		md1, ok := trip(t, in)
		if !ok {
			return
		}
		md2, ok := trip(t, md1)
		if !ok {
			t.Fatalf("second trip failed for %q", md1)
		}
		if md1 != md2 {
			t.Fatalf("not stable after one round trip\ninput: %q\nfirst: %q\nsecond: %q", in, md1, md2)
		}
	})
}

func trip(t *testing.T, in string) (string, bool) {
	t.Helper()
	doc, err := FromMarkdown(in)
	if err != nil {
		return "", false
	}
	out, err := ToMarkdown(doc)
	if err != nil {
		t.Fatalf("ToMarkdown of our own output: %v\n%s", err, doc)
	}
	return out, true
}

// FuzzToMarkdown checks that arbitrary input never panics.
func FuzzToMarkdown(f *testing.F) {
	for _, s := range awkward {
		if doc, err := FromMarkdown(s); err == nil {
			f.Add([]byte(doc))
		}
	}
	f.Add([]byte(`{"type":"doc","version":1,"content":[{"type":"table","content":[{"type":"tableRow"}]}]}`))
	f.Add([]byte(`{"type":"doc","content":[{"type":"heading","attrs":{"level":"x"}},{"type":"panel","attrs":{"panelType":""}}]}`))
	f.Add([]byte(`"plain"`))
	f.Fuzz(func(t *testing.T, in []byte) {
		_, _ = ToMarkdown(json.RawMessage(in))
	})
}

// FuzzPassthroughMarkdown checks that, with WithPassthrough, any input
// converts without panicking, fails only with ErrRawNode or ErrTooDeep, and
// is stable after one round trip.
func FuzzPassthroughMarkdown(f *testing.F) {
	seedMarkdown(f)
	f.Add("```adf\n{\"type\":\"rule\"}\n```")
	f.Add("a `adf:[{\"type\":\"status\",\"attrs\":{\"text\":\"x\"}},{\"type\":\"text\",\"text\":\"c\",\"marks\":[{\"type\":\"code\"}]}]` b")
	f.Add("- ```adf\n  {\"type\":\"panel\",\"content\":[]}\n  ```")
	f.Add("| a |\n| --- |\n| `adf:{\"type\":\"emoji\",\"attrs\":{\"shortName\":\"\\|\"}}` |")
	f.Fuzz(func(t *testing.T, in string) {
		doc, err := FromMarkdown(in, WithPassthrough())
		if errors.Is(err, ErrRawNode) || errors.Is(err, ErrTooDeep) {
			return
		}
		if err != nil {
			t.Fatalf("FromMarkdown(%q): %v", in, err)
		}
		md1, err := ToMarkdown(doc, WithPassthrough())
		if err != nil {
			t.Fatalf("ToMarkdown of our own output: %v\n%s", err, doc)
		}
		doc2, err := FromMarkdown(md1, WithPassthrough())
		if err != nil {
			t.Fatalf("reading back %q: %v\nfrom %s", md1, err, doc)
		}
		md2, err := ToMarkdown(doc2, WithPassthrough())
		if err != nil {
			t.Fatal(err)
		}
		if md1 != md2 {
			t.Fatalf("not stable after one round trip\ninput: %q\nfirst: %q\nsecond: %q", in, md1, md2)
		}
	})
}

// FuzzExtendedMarkdown checks that, with WithExtendedSyntax and
// WithPassthrough, any input converts without panicking, fails only with
// the documented errors, gives valid ADF without passed-through nodes, and
// is stable after one round trip.
func FuzzExtendedMarkdown(f *testing.F) {
	seedMarkdown(f)
	for _, c := range extendedDocs {
		f.Add(c.md)
	}
	f.Add("<details>\n<summary>x</summary>\n\n<details>\n\ny\n\n</details>\n\n</details>")
	f.Add("- {color:#006644}a **b{/color} c**\n\n  <card-block:https://e.com>")
	f.Add("````columns\n```column\n<details>\n\n````columns\n```column\nx\n```\n````\n\n</details>\n```\n```column\ny\n```\n````")
	opts := []Option{WithExtendedSyntax(), WithPassthrough(), WithMediaResolver(genMedia)}
	f.Fuzz(func(t *testing.T, in string) {
		doc, err := FromMarkdown(in, opts...)
		var me *MediaError
		if errors.Is(err, ErrRawNode) || errors.Is(err, ErrTooDeep) || errors.Is(err, ErrSyntax) || errors.As(err, &me) {
			return
		}
		if err != nil {
			t.Fatalf("FromMarkdown(%q): %v", in, err)
		}
		if !strings.Contains(in, "adf") {
			validate(t, doc)
		}
		md1, err := ToMarkdown(doc, opts...)
		if err != nil {
			t.Fatalf("ToMarkdown of our own output: %v\n%s", err, doc)
		}
		doc2, err := FromMarkdown(md1, opts...)
		if err != nil {
			t.Fatalf("reading back %q: %v\nfrom %s", md1, err, doc)
		}
		md2, err := ToMarkdown(doc2, opts...)
		if err != nil {
			t.Fatal(err)
		}
		if md1 != md2 {
			t.Fatalf("not stable after one round trip\ninput: %q\nfirst: %q\nsecond: %q", in, md1, md2)
		}
	})
}
