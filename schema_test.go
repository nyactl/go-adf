package adf

import (
	"bytes"
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	schemaOnce sync.Once
	schema     *jsonschema.Schema
	schemaErr  error
)

// validate fails the test unless raw is valid against Atlassian's ADF JSON
// schema (testdata/schema).
func validate(t testing.TB, raw json.RawMessage) {
	t.Helper()
	if err := schemaError(t, raw); err != nil {
		t.Errorf("output is not valid ADF: %v\n%s", err, raw)
	}
}

func schemaError(t testing.TB, raw json.RawMessage) error {
	t.Helper()
	schemaOnce.Do(func() {
		f, err := os.Open("testdata/schema/full.json")
		if err != nil {
			schemaErr = err
			return
		}
		defer func() { _ = f.Close() }()
		doc, err := jsonschema.UnmarshalJSON(f)
		if err != nil {
			schemaErr = err
			return
		}
		c := jsonschema.NewCompiler()
		if schemaErr = c.AddResource("full.json", doc); schemaErr != nil {
			return
		}
		schema, schemaErr = c.Compile("full.json")
	})
	if schemaErr != nil {
		t.Fatalf("loading the ADF schema: %v", schemaErr)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, raw)
	}
	return schema.Validate(inst)
}

// TestSchemaRejectsInvalid makes sure the validation above can fail.
func TestSchemaRejectsInvalid(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"heading in list item": `{"type":"doc","version":1,"content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"heading","attrs":{"level":1},"content":[{"type":"text","text":"x"}]}]}]}]}`,
		"empty text":           `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":""}]}]}`,
		"code with strong":     `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"code"},{"type":"strong"}]}]}]}`,
		"doc without content":  `{"type":"doc","version":1}`,
		"empty list":           `{"type":"doc","version":1,"content":[{"type":"bulletList","content":[]}]}`,
	} {
		if schemaError(t, json.RawMessage(raw)) == nil {
			t.Errorf("%s: accepted by the schema check", name)
		}
	}
}

// awkward holds Markdown that ADF cannot represent one to one.
var awkward = map[string]string{
	"empty":                        "",
	"whitespace":                   "   \n\n  ",
	"heading in list item":         "- # Title\n- item",
	"quote in list item":           "- > quoted\n  > more",
	"table in list item":           "- | a | b |\n  |---|---|\n  | 1 | 2 |",
	"rule in list item":            "- one\n\n  ---\n- two",
	"nested quote":                 "> outer\n>> inner\n>>> innermost",
	"empty quote":                  ">",
	"heading in quote":             "> # Title\n> text",
	"table in quote":               "> | a |\n> |---|\n> | 1 |",
	"list item starting with list": "- - nested first\n  - more",
	"empty list item":              "-\n- b",
	"ordered from zero":            "0. zero\n1. one",
	"ordered large start":          "123456789. big",
	"code inside strong":           "**bold `code` bold**",
	"link inside code":             "[`code link`](https://example.com)",
	"strong inside link":           "[**bold** link](https://example.com)",
	"image":                        "![alt text](https://example.com/a.png)",
	"image without alt":            "![](https://example.com/a.png)",
	"html block":                   "<div>\nblock\n</div>",
	"inline html":                  "a <span>b</span> c",
	"task list":                    "- [x] done\n- [ ] open",
	"empty table cells":            "| a | b |\n|---|---|\n|   |   |",
	"table only header":            "| a | b |\n|---|---|",
	"setext heading":               "Title\n=====",
	"hard breaks":                  "a  \nb\\\nc",
	"autolink":                     "<https://example.com> and www.example.com",
	"code block in list":           "- item\n\n  ```go\n  x := 1\n  ```",
	"indented code":                "    code\n    more",
	"empty code block":             "```\n```",
	"quote with list and heading":  "> - # h\n>   - x",
	"deep list": "- 1\n  - 2\n    - 3\n      - 4\n        - 5\n          - 6\n            - 7\n" +
		"              - 8\n                - 9\n                  - 10",
	"mention syntax without resolver": "ping @[Someone]",
	"control characters":              "a\x00b\x1bc",
	"unicode":                         "Grüße 👋 — \u200b",
}

func TestFromMarkdownIsSchemaValid(t *testing.T) {
	t.Parallel()
	for name, in := range awkward {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			out, err := FromMarkdown(in)
			if err != nil {
				t.Fatal(err)
			}
			validate(t, out)
		})
	}
}

func TestFromMarkdownMentionIsSchemaValid(t *testing.T) {
	t.Parallel()
	out, err := FromMarkdown("**hi @[A]**, see `@[A]`", WithMentionResolver(func(string) (string, string, bool) {
		return "acc-1", "Alice", true
	}))
	if err != nil {
		t.Fatal(err)
	}
	validate(t, out)
}
