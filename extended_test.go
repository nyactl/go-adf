package adf

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func linked(s, href string) string {
	b, _ := json.Marshal(s)
	return `{"type":"text","text":` + string(b) + `,"marks":[{"type":"link","attrs":{"href":"` + href + `"}}]}`
}

func marked(s string, marks ...string) string {
	b, _ := json.Marshal(s)
	return `{"type":"text","text":` + string(b) + `,"marks":[` + strings.Join(marks, ",") + `]}`
}

const (
	green  = `{"type":"textColor","attrs":{"color":"#006644"}}`
	yellow = `{"type":"backgroundColor","attrs":{"color":"#fff0b3"}}`
	strong = `{"type":"strong"}`
	wide   = `{"type":"breakout","attrs":{"mode":"wide","width":760}}`
)

var flowerMedia = Node{Type: "media", Attrs: map[string]any{"type": "file", "id": "a04ac4c3", "collection": "contentId-1", "alt": "flower.jpeg", "width": json.Number("600"), "height": json.Number("400")}}

func resolveFlower(name string) (Node, bool) {
	if name == "flower.jpeg" || name == "my file (1).png" {
		m := flowerMedia
		m.Attrs = map[string]any{}
		for k, v := range flowerMedia.Attrs {
			m.Attrs[k] = v
		}
		m.Attrs["alt"] = name
		return m, true
	}
	return Node{}, false
}

var extOpts = []Option{WithExtendedSyntax(), WithMediaResolver(resolveFlower)}

// extendedDocs pair documents in the form FromMarkdown writes with their
// Markdown under WithExtendedSyntax; both directions must give the other.
var extendedDocs = []struct {
	name string
	adf  json.RawMessage
	md   string
}{
	{"expand", adfDoc(`{"type":"expand","attrs":{"title":"More <info> & \"quotes\""},"content":[` + para(txt("hidden")) + `,{"type":"bulletList","content":[{"type":"listItem","content":[` + para(txt("item")) + `]}]}]}`),
		"<details>\n<summary>More &lt;info&gt; &amp; &#34;quotes&#34;</summary>\n\nhidden\n\n- item\n\n</details>"},
	{"expand wide", adfDoc(`{"type":"expand","attrs":{"title":"T"},"content":[` + para(txt("x")) + `],"marks":[` + wide + `]}`),
		"<details data-breakout=\"wide:760\">\n<summary>T</summary>\n\nx\n\n</details>"},
	{"expand without title", adfDoc(`{"type":"expand","content":[` + para(txt("x")) + `]}`), "<details>\n\nx\n\n</details>"},
	{"expand with empty title", adfDoc(`{"type":"expand","attrs":{"title":""},"content":[` + para(txt("x")) + `]}`), "<details>\n<summary></summary>\n\nx\n\n</details>"},
	{"nested expand", adfDoc(`{"type":"expand","attrs":{"title":"outer"},"content":[` + para(txt("a")) + `,{"type":"nestedExpand","attrs":{"title":"inner"},"content":[` + para(txt("b")) + `]}]}`),
		"<details>\n<summary>outer</summary>\n\na\n\n<details>\n<summary>inner</summary>\n\nb\n\n</details>\n\n</details>"},
	{"columns", adfDoc(`{"type":"layoutSection","content":[{"type":"layoutColumn","attrs":{"width":50},"content":[` + para(txt("left")) + `]},{"type":"layoutColumn","attrs":{"width":50},"content":[` + para(txt("right")) + `]}],"marks":[` + wide + `]}`),
		"````columns breakout=wide:760\n```column 50\nleft\n```\n\n```column 50\nright\n```\n````"},
	{"three columns with code and an expand", adfDoc(`{"type":"layoutSection","content":[{"type":"layoutColumn","attrs":{"width":33.33},"content":[{"type":"codeBlock","attrs":{"language":"go"},"content":[` + txt("x := \"```\"") + `]}]},{"type":"layoutColumn","attrs":{"width":33.33},"content":[{"type":"expand","attrs":{"title":"e"},"content":[` + para(txt("in")) + `]}]},{"type":"layoutColumn","attrs":{"width":33.34},"content":[` + para(txt("c")) + `]}]}`),
		"`````columns\n````column 33.33\n```go\nx := \"```\"\n```\n````\n\n```column 33.33\n<details>\n<summary>e</summary>\n\nin\n\n</details>\n```\n\n```column 33.34\nc\n```\n`````"},
	{"cards", adfDoc(para(txt("see "), `{"type":"inlineCard","attrs":{"url":"https://example.atlassian.net/browse/CMP-1"}}`, txt(" now")), `{"type":"blockCard","attrs":{"url":"https://example.com/a?b=c"}}`, `{"type":"embedCard","attrs":{"url":"https://example.com/v","layout":"center"}}`),
		"see <card:https://example.atlassian.net/browse/CMP-1> now\n\n<card-block:https://example.com/a?b=c>\n\n<card-embed:https://example.com/v>"},
	{"colors", adfDoc(para(txt("plain "), marked("green", green), txt(" and "), marked("bold", strong, green), txt(" "), marked("both", green, yellow), txt(" end"))),
		"plain {color:#006644}green{/color} and {color:#006644}**bold**{/color} {color:#006644}{highlight:#fff0b3}both{/highlight}{/color} end"},
	{"colored link", adfDoc(para(`{"type":"text","text":"link","marks":[{"type":"link","attrs":{"href":"https://example.com"}},` + green + `]}`)),
		"{color:#006644}[link](https://example.com){/color}"},
	{"external image", adfDoc(`{"type":"mediaSingle","attrs":{"layout":"center","width":600,"widthType":"pixel"},"content":[{"type":"media","attrs":{"type":"external","url":"https://example.com/a.png","alt":"a chart"}}]}`),
		"![a chart](https://example.com/a.png \"center 600px\")"},
	{"attachment image", adfDoc(`{"type":"mediaSingle","attrs":{"layout":"wide","width":50},"content":[{"type":"media","attrs":{"type":"file","id":"a04ac4c3","collection":"contentId-1","alt":"my file (1).png","width":600,"height":400}}]}`),
		"![my file (1).png](<attachment:my file (1).png> \"wide 50%\")"},
	{"image in a list", adfDoc(`{"type":"bulletList","content":[{"type":"listItem","content":[` + para(txt("see")) + `,{"type":"mediaSingle","content":[{"type":"media","attrs":{"type":"external","url":"https://example.com/a.png"}}]}]}]}`),
		"- see\n\n  ![](https://example.com/a.png)"},
}

func TestExtendedSyntax(t *testing.T) {
	t.Parallel()
	for _, c := range extendedDocs {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			md, err := ToMarkdown(c.adf, extOpts...)
			if err != nil {
				t.Fatal(err)
			}
			if md != c.md {
				t.Errorf("markdown\ngot:  %q\nwant: %q", md, c.md)
			}
			out, err := FromMarkdown(c.md, extOpts...)
			if err != nil {
				t.Fatal(err)
			}
			validate(t, out)
			if !reflect.DeepEqual(normalized(t, c.adf), normalized(t, out)) {
				t.Errorf("document\nin:  %s\nout: %s", c.adf, out)
			}
			// With WithPassthrough as well, nothing is written as JSON.
			md2, err := ToMarkdown(c.adf, append(extOpts, WithPassthrough())...)
			if err != nil || md2 != c.md {
				t.Errorf("with passthrough: %v\n%q", err, md2)
			}
		})
	}
}

// TestExtendedSyntaxAsWritten reads the forms as a person or an assistant
// might write them.
func TestExtendedSyntaxAsWritten(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ md, want string }{
		{"<details><summary>Title</summary>\n\nbody\n</details>", `{"type":"expand","attrs":{"title":"Title"},"content":[` + para(txt("body")) + `]}`},
		{"<details>\n<summary>Empty</summary>\n\n</details>", `{"type":"expand","attrs":{"title":"Empty"},"content":[{"type":"paragraph"}]}`},
		{"````columns\n```column\na\n```\n```column\nb\n```\n````", `{"type":"layoutSection","content":[{"type":"layoutColumn","attrs":{"width":50},"content":[` + para(txt("a")) + `]},{"type":"layoutColumn","attrs":{"width":50},"content":[` + para(txt("b")) + `]}]}`},
		{"~~~columns\n```column\na\n```\n```column\nb\n```\n```column\nc\n```\n~~~", `{"type":"layoutSection","content":[{"type":"layoutColumn","attrs":{"width":33.33},"content":[` + para(txt("a")) + `]},{"type":"layoutColumn","attrs":{"width":33.33},"content":[` + para(txt("b")) + `]},{"type":"layoutColumn","attrs":{"width":33.33},"content":[` + para(txt("c")) + `]}]}`},
		{"{color:#FF5630}red *and em*{/color}", para(marked("red ", `{"type":"textColor","attrs":{"color":"#FF5630"}}`), marked("and em", `{"type":"em"}`, `{"type":"textColor","attrs":{"color":"#FF5630"}}`))},
		{"{color:#ff5630}unclosed", para(txt("{color:#ff5630}unclosed"))},
		{"`{color:#ff5630}`code{/color}", para(marked("{color:#ff5630}", `{"type":"code"}`), txt("code{/color}"))},
		{"![x](https://example.com/x.png \"A normal title\")", para(linked("x", "https://example.com/x.png"))},
		{"text ![x](https://example.com/x.png) inline", para(txt("text "), linked("x", "https://example.com/x.png"), txt(" inline"))},
		{"```columns\nnot a layout\n```", `{"type":"codeBlock","attrs":{"language":"columns"},"content":[` + txt("not a layout") + `]}`},
		{"- <details>\n  <summary>in list</summary>\n\n  flattened\n\n  </details>", `{"type":"bulletList","content":[{"type":"listItem","content":[` + para(txt("flattened")) + `]}]}`},
		{"<card-block:https://example.com> and text", para(linked("card-block:https://example.com", "card-block:https://example.com"), txt(" and text"))},
	} {
		out, err := FromMarkdown(c.md, extOpts...)
		if err != nil {
			t.Errorf("%q: %v", c.md, err)
			continue
		}
		validate(t, out)
		if want := adfDoc(c.want); !reflect.DeepEqual(normalized(t, want), normalized(t, out)) {
			t.Errorf("%q\ngot:  %s\nwant: %s", c.md, out, want)
		}
	}
}

func TestExtendedSyntaxEscapesLookalikes(t *testing.T) {
	t.Parallel()
	in := adfDoc(
		para(txt("{color:#ff5630}not red{/color} and {highlight:#ffffff}")),
		para(txt("<details>")),
		para(txt("<card:https://example.com> "), linked("card:https://example.com", "card:https://example.com")),
		para(txt("![x](https://example.com/x.png)")),
		`{"type":"codeBlock","attrs":{"language":"columns"},"content":[`+txt("```column\nx\n```")+`]}`,
	)
	md, err := ToMarkdown(in, extOpts...)
	if err != nil {
		t.Fatal(err)
	}
	out, err := FromMarkdown(md, extOpts...)
	if err != nil {
		t.Fatalf("%v\n%s", err, md)
	}
	s := string(out)
	for _, bad := range []string{`"textColor"`, `"backgroundColor"`, `"expand"`, `"inlineCard"`, `"mediaSingle"`, `"layoutSection"`} {
		if strings.Contains(s, bad) {
			t.Errorf("lookalike text became %s\nmd:\n%s\nout: %s", bad, md, s)
		}
	}
	// The code keeps its text; only its language, which would make it a
	// layout, is dropped.
	if !strings.Contains(s, `"text":"`+"```column\\nx\\n```"+`"`) {
		t.Errorf("code changed: %s", s)
	}
	// With WithPassthrough the code keeps its language too.
	md, _ = ToMarkdown(in, append(extOpts, WithPassthrough())...)
	if out, err := FromMarkdown(md, append(extOpts, WithPassthrough())...); err != nil || !reflect.DeepEqual(normalized(t, in), normalized(t, out)) {
		t.Errorf("with passthrough: %v\n%s\n%s", err, md, out)
	}
	// Without WithExtendedSyntax, the forms are plain Markdown.
	plain, err := FromMarkdown("{color:#ff5630}x{/color}\n\n<card:https://e.com>", WithMediaResolver(resolveFlower))
	if err != nil || strings.Contains(string(plain), "textColor") || strings.Contains(string(plain), "inlineCard") {
		t.Errorf("forms read without the option: %v %s", err, plain)
	}
}

func TestExtendedSyntaxErrors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		md   string
		opts []Option
		want error
	}{
		{"![x](attachment:x.png)", []Option{WithExtendedSyntax()}, nil},
		{"![x](attachment:unknown.png)", extOpts, nil},
		{"````columns\n```column 50\na\n```\n```column\nb\n```\n````", extOpts, ErrSyntax},
		{"````columns\n```column 50\na\n```\n````", extOpts, ErrSyntax},
		{"````columns\n```column\na\n```\n```column\nb\n```\n```column\nc\n```\n```column\nd\n```\n````", extOpts, ErrSyntax},
		{"````columns\n```column 120\na\n```\n```column\nb\n```\n````", extOpts, ErrSyntax},
		{"````columns\n```column 50 top\na\n```\n```column 50\nb\n```\n````", extOpts, ErrSyntax},
		{"````columns\ntext\n```column\na\n```\n```column\nb\n```\n````", extOpts, ErrSyntax},
		{"````columns narrow\n```column\na\n```\n```column\nb\n```\n````", extOpts, ErrSyntax},
	} {
		_, err := FromMarkdown(c.md, c.opts...)
		var me *MediaError
		if c.want == nil {
			// nil stands for a *MediaError.
			if !errors.As(err, &me) {
				t.Errorf("%q: err = %v, want a MediaError", c.md, err)
			}
		} else if !errors.Is(err, c.want) {
			t.Errorf("%q: err = %v, want %v", c.md, err, c.want)
		}
	}
	deep := strings.Repeat("````````columns\n```column\n", 1) + "x\n```\n```column\ny\n```\n````````"
	if _, err := FromMarkdown(deep, append(extOpts, WithMaxDepth(1))...); !errors.Is(err, ErrTooDeep) {
		t.Errorf("nested columns beyond the depth limit: %v", err)
	}
}

// TestExtendedRealPage converts a page as the Confluence editor stores it,
// with the elements that have a form, and checks what is kept.
func TestExtendedRealPage(t *testing.T) {
	t.Parallel()
	page := adfDoc(
		`{"type":"paragraph","attrs":{"localId":"p1"},"content":[{"type":"text","text":"plain text"}]}`,
		`{"type":"expand","attrs":{"title":"expand title","localId":"e1"},"marks":[{"type":"breakout","attrs":{"mode":"wide","width":760}}],"content":[{"type":"paragraph","attrs":{"localId":"p2"},"content":[{"type":"text","text":"expand text"}]}]}`,
		`{"type":"layoutSection","attrs":{"localId":"l1"},"marks":[{"type":"breakout","attrs":{"mode":"wide","width":760}}],"content":[{"type":"layoutColumn","attrs":{"width":50.0,"localId":"c1"},"content":[`+para(txt("left side"))+`]},{"type":"layoutColumn","attrs":{"width":50.0,"localId":"c2"},"content":[`+para(txt("right side"))+`]}]}`,
		`{"type":"mediaSingle","attrs":{"layout":"center","width":600,"widthType":"pixel"},"content":[{"type":"media","attrs":{"width":600,"alt":"flower.jpeg","id":"a04ac4c3","collection":"contentId-1","type":"file","localId":"m1","height":400}}]}`,
		`{"type":"paragraph","content":[{"type":"text","text":"colorful green text","marks":[{"type":"textColor","attrs":{"color":"#006644"}}]}]}`,
	)
	md, err := ToMarkdown(page, append(extOpts, WithPassthrough())...)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(md, "```adf") || strings.Contains(md, "`adf:") {
		t.Errorf("elements with a form written as JSON:\n%s", md)
	}
	out, err := FromMarkdown(md, append(extOpts, WithPassthrough())...)
	if err != nil {
		t.Fatal(err)
	}
	validate(t, out)
	got := string(out)
	for _, want := range []string{`"breakout"`, `"width":760`, `"width":50.0`, `"title":"expand title"`, `"widthType":"pixel"`, `"id":"a04ac4c3"`, `"color":"#006644"`} {
		if !strings.Contains(got, want) {
			t.Errorf("lost %s: %s", want, got)
		}
	}
}
