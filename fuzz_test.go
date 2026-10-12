package adf

import (
	"encoding/json"
	"errors"
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
