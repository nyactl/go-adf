package adf

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestCoverageListsEverySchemaType keeps COVERAGE.md complete: every node
// and mark type in the ADF schema must be mentioned there.
func TestCoverageListsEverySchemaType(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile("COVERAGE.md")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/schema/full.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Definitions map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
			AllOf      []struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"allOf"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	types := map[string]bool{}
	for _, d := range schema.Definitions {
		props := []map[string]json.RawMessage{d.Properties}
		for _, a := range d.AllOf {
			props = append(props, a.Properties)
		}
		for _, p := range props {
			var typ struct {
				Enum []string `json:"enum"`
			}
			if p["type"] != nil && json.Unmarshal(p["type"], &typ) == nil {
				for _, v := range typ.Enum {
					types[v] = true
				}
			}
		}
	}
	if len(types) < 40 {
		t.Fatalf("found only %d types in the schema", len(types))
	}
	for typ := range types {
		if !strings.Contains(string(doc), "`"+typ+"`") {
			t.Errorf("COVERAGE.md does not mention %q", typ)
		}
	}
}
