package adf_test

import (
	"encoding/json"
	"fmt"
	"strings"

	adf "github.com/nyactl/go-adf"
)

func ExampleFromMarkdown() {
	doc, err := adf.FromMarkdown("Fixed in **v2.1**, see [notes](https://example.com/notes).")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(doc))
	// Output:
	// {"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"Fixed in "},{"type":"text","text":"v2.1","marks":[{"type":"strong"}]},{"type":"text","text":", see "},{"type":"text","text":"notes","marks":[{"type":"link","attrs":{"href":"https://example.com/notes"}}]},{"type":"text","text":"."}]}]}
}

func ExampleToMarkdown() {
	doc := json.RawMessage(`{"type":"doc","version":1,"content":[
		{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Steps"}]},
		{"type":"orderedList","content":[
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Run "},{"type":"text","text":"make test","marks":[{"type":"code"}]}]}]},
			{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"Ship it"}]}]}]}]}`)
	md, err := adf.ToMarkdown(doc)
	if err != nil {
		panic(err)
	}
	fmt.Println(md)
	// Output:
	// ## Steps
	//
	// 1. Run `make test`
	// 2. Ship it
}

// Mentions are written as @[label]. The hooks decide how people are named
// and resolved, for example to keep real names out of the Markdown.
func Example_mentions() {
	people := map[string]string{"Person A": "5b10ac8d82e05b22cc7d4ef5"}
	resolve := adf.WithMentionResolver(func(label string) (string, string, bool) {
		id, ok := people[label]
		return id, label, ok
	})
	doc, err := adf.FromMarkdown("Thanks @[Person A]!", resolve)
	if err != nil {
		panic(err)
	}
	md, err := adf.ToMarkdown(doc, adf.WithMentionText(func(accountID, text string) string {
		return strings.TrimPrefix(text, "@")
	}))
	if err != nil {
		panic(err)
	}
	fmt.Println(md)
	// Output:
	// Thanks @[Person A]!
}
