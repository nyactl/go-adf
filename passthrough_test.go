package adf

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func adfDoc(blocks ...string) json.RawMessage {
	return json.RawMessage(`{"type":"doc","version":1,"content":[` + strings.Join(blocks, ",") + `]}`)
}

func para(inline ...string) string {
	return `{"type":"paragraph","content":[` + strings.Join(inline, ",") + `]}`
}

func txt(s string) string {
	b, _ := json.Marshal(s)
	return `{"type":"text","text":` + string(b) + `}`
}

// Nodes as the Confluence and Jira editors store them, none of which
// Markdown can express.
var (
	tocMacro      = `{"type":"extension","attrs":{"extensionKey":"toc","extensionType":"com.atlassian.confluence.macro.core","layout":"default","localId":"a1","parameters":{"macroMetadata":{"macroId":{"value":"m-1"},"schemaVersion":{"value":"1"},"title":"Table of Contents"},"macroParams":{"maxLevel":{"value":"3"}}}}}`
	appMacro      = `{"type":"extension","attrs":{"extensionKey":"app-1/env-1/static/diagram","extensionType":"com.atlassian.ecosystem","layout":"default","localId":"a2","parameters":{"extensionTitle":"Diagram","guestParams":{"source":{"text":"graph TD; A--\u003eB | C ` + "```" + ` end","type":"text"},"version":1.50,"nested":{"deeper":{"deepest":{"list":[1,2.5,-3,12345678901234567890,true,null,"x"]}}}},"layout":"extension"},"text":"Diagram"}}`
	bodiedMacro   = `{"type":"bodiedExtension","attrs":{"extensionKey":"details","extensionType":"com.atlassian.confluence.macro.core","layout":"default","localId":"a3","parameters":{"macroMetadata":{"title":"Page Properties"},"macroParams":{}}},"content":[` + para(txt("Owner: team")) + `,{"type":"bulletList","content":[{"type":"listItem","content":[` + para(txt("inside")) + `]}]}]}`
	inlineMacro   = `{"type":"inlineExtension","attrs":{"extensionKey":"anchor","extensionType":"com.atlassian.confluence.macro.core","localId":"a4","parameters":{"macroMetadata":{"title":"Anchor"},"macroParams":{"":{"value":"top"}}}}}`
	statusNode    = `{"type":"status","attrs":{"color":"green","localId":"s1","style":"","text":"DONE"}}`
	dateNode      = `{"type":"date","attrs":{"timestamp":"1760313600000"}}`
	emojiNode     = `{"type":"emoji","attrs":{"id":"1f600","shortName":":grinning:","text":"😀"}}`
	coloredText   = `{"type":"text","text":"green | text","marks":[{"type":"strong"},{"type":"textColor","attrs":{"color":"#22a06b"}}]}`
	inlineCard    = `{"type":"inlineCard","attrs":{"url":"https://example.atlassian.net/browse/CMP-1"}}`
	panelNode     = `{"type":"panel","attrs":{"panelType":"info"},"content":[` + para(txt("Heads up")) + `]}`
	expandNode    = `{"type":"expand","attrs":{"title":"More"},"content":[` + para(txt("hidden")) + `],"marks":[{"type":"breakout","attrs":{"mode":"wide"}}]}`
	layoutNode    = `{"type":"layoutSection","content":[{"type":"layoutColumn","attrs":{"width":50},"content":[` + para(txt("left")) + `]},{"type":"layoutColumn","attrs":{"width":50},"content":[` + para(txt("right")) + `]}]}`
	imageNode     = `{"type":"mediaSingle","attrs":{"layout":"center"},"content":[{"type":"media","attrs":{"alt":"flower.jpeg","collection":"contentId-1","height":400,"id":"f-1","type":"file","width":600}}]}`
	blockCard     = `{"type":"blockCard","attrs":{"url":"https://example.atlassian.net/issues/?jql=project%20%3D%20CMP"}}`
	taskList      = `{"type":"taskList","attrs":{"localId":"t1"},"content":[{"type":"taskItem","attrs":{"localId":"t2","state":"DONE"},"content":[` + txt("done") + `]}]}`
	decisionList  = `{"type":"decisionList","attrs":{"localId":"d1"},"content":[{"type":"decisionItem","attrs":{"localId":"d2","state":"DECIDED"},"content":[` + txt("ship it") + `]}]}`
	centered      = `{"type":"paragraph","content":[` + txt("centered") + `],"marks":[{"type":"alignment","attrs":{"align":"center"}}]}`
	headingBreak  = `{"type":"heading","attrs":{"level":2},"content":[` + txt("two") + `,{"type":"hardBreak"},` + txt("lines") + `]}`
	noHeaderTable = `{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","content":[` + para(txt("a")) + `]},{"type":"tableCell","content":[` + para(txt("b")) + `]}]}]}`
	mergedTable   = `{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableHeader","attrs":{"colspan":2},"content":[` + para(txt("wide")) + `]}]},{"type":"tableRow","content":[{"type":"tableCell","content":[` + para(txt("a")) + `]},{"type":"tableCell","content":[` + para(txt("b")) + `]}]}]}`
	adfCode       = `{"type":"codeBlock","attrs":{"language":"adf"},"content":[` + txt(`{"type":"rule"}`) + `]}`
	adfCodeText   = `{"type":"text","text":"adf:{\"type\":\"rule\"}","marks":[{"type":"code"}]}`
	futureBlock   = `{"type":"someFutureBlock","attrs":{"x":1}}`
	futureInline  = `{"type":"someFutureInline","attrs":{"x":1}}`
)

// passthroughDocs are documents whose plain parts are written the way
// FromMarkdown writes them, so the whole document must come back unchanged.
var passthroughDocs = map[string]json.RawMessage{
	"macros":               adfDoc(para(txt("Intro "), inlineMacro, txt(" and more")), tocMacro, appMacro, bodiedMacro),
	"inline nodes":         adfDoc(para(txt("State "), statusNode, txt(" since "), dateNode, txt(" "), emojiNode, txt(" see "), inlineCard)),
	"text color":           adfDoc(para(txt("plain "), coloredText, txt(" plain"))),
	"native blocks":        adfDoc(panelNode, expandNode, layoutNode, imageNode, blockCard, taskList, decisionList),
	"block marks":          adfDoc(centered, headingBreak),
	"tables":               adfDoc(noHeaderTable, mergedTable),
	"lookalikes":           adfDoc(adfCode, para(txt("x "), adfCodeText)),
	"in list":              adfDoc(`{"type":"bulletList","content":[{"type":"listItem","content":[` + para(txt("item")) + `,` + tocMacro + `,` + panelNode + `]},{"type":"listItem","content":[` + tocMacro + `]}]}`),
	"in quote":             adfDoc(`{"type":"blockquote","content":[` + para(txt("quoted")) + `,` + panelNode + `,` + headingBreak + `]}`),
	"in table cell":        adfDoc(`{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableHeader","content":[` + para(txt("h")) + `]}]},{"type":"tableRow","content":[{"type":"tableCell","content":[` + para(statusNode, txt(" "), coloredText) + `]}]}]}`),
	"in emphasis and link": adfDoc(para(`{"type":"text","text":"bold","marks":[{"type":"strong"}]}`, txt(" "), statusNode, txt(" "), `{"type":"text","text":"more","marks":[{"type":"strong"}]}`, txt(" "), `{"type":"text","text":"link","marks":[{"type":"link","attrs":{"href":"https://example.com"}},{"type":"underline"}]}`)),
	"unknown types":        adfDoc(futureBlock, para(txt("a "), futureInline)),
}

// normalized decodes JSON for comparison, keeping numbers as written.
func normalized(t *testing.T, raw []byte) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%v: %s", err, raw)
	}
	return v
}

func TestPassthroughRoundTrip(t *testing.T) {
	t.Parallel()
	for name, in := range passthroughDocs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			md, err := ToMarkdown(in, WithPassthrough())
			if err != nil {
				t.Fatal(err)
			}
			out, err := FromMarkdown(md, WithPassthrough())
			if err != nil {
				t.Fatalf("%v\nmarkdown:\n%s", err, md)
			}
			if !reflect.DeepEqual(normalized(t, in), normalized(t, out)) {
				t.Errorf("document changed\nin:  %s\nmd:\n%s\nout: %s", in, md, out)
			}
			// Valid input gives valid output; the converter does not repair
			// what it passes through.
			if schemaError(t, in) == nil {
				validate(t, out)
			}
			// Markdown written once does not change on further trips.
			md2, err := ToMarkdown(out, WithPassthrough())
			if err != nil || md2 != md {
				t.Errorf("not stable: %v\nfirst:\n%s\nsecond:\n%s", err, md, md2)
			}
		})
	}
}

func TestPassthroughForms(t *testing.T) {
	t.Parallel()
	md, err := ToMarkdown(passthroughDocs["macros"], WithPassthrough())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Intro `adf:" + inlineMacro + "` and more",
		"```adf\n" + tocMacro + "\n```",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q:\n%s", want, md)
		}
	}
	// Plain content stays plain.
	md, err = ToMarkdown(adfDoc(para(txt("just text")), `{"type":"rule"}`), WithPassthrough())
	if err != nil || md != "just text\n\n---" {
		t.Errorf("plain document = %q, %v", md, err)
	}
}

func TestWithoutPassthroughUnchanged(t *testing.T) {
	t.Parallel()
	for name, in := range passthroughDocs {
		if name == "lookalikes" {
			continue
		}
		md, err := ToMarkdown(in)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(md, "```adf") || strings.Contains(md, "`adf:{\"type\":\"inline") {
			t.Errorf("%s: JSON without WithPassthrough:\n%s", name, md)
		}
	}
	out, err := FromMarkdown("```adf\n" + tocMacro + "\n```\n\n`adf:" + statusNode + "`")
	if err != nil {
		t.Fatal(err)
	}
	if s := string(out); !strings.Contains(s, `"type":"codeBlock","attrs":{"language":"adf"}`) || strings.Contains(s, `"type":"extension"`) || strings.Contains(s, `"type":"status"`) {
		t.Errorf("adf blocks read without WithPassthrough: %s", s)
	}
}

func TestPassthroughRejectsInvalidNodes(t *testing.T) {
	t.Parallel()
	for _, in := range []string{
		"```adf\nnot json\n```",
		"```adf\n{\"type\":\"rule\"} {\"type\":\"rule\"}\n```",
		"```adf\n{\"attrs\":{}}\n```",
		"```adf\n{\"type\":\"doc\",\"content\":[]}\n```",
		"```adf\n" + statusNode + "\n```",
		"```adf\n[1,2]\n```",
		"`adf:" + tocMacro + "`",
		"`adf:{\"type\":\"text\",\"text\":\"\"}`",
		"`adf:{\"type\":\"text\"`",
		"- item\n\n  ```adf\n{\"type\":\"paragraph\",\"content\":\"x\"}\n  ```",
	} {
		if _, err := FromMarkdown(in, WithPassthrough()); !errors.Is(err, ErrRawNode) {
			t.Errorf("%q: err = %v, want ErrRawNode", in, err)
		}
	}
	deep := strings.Repeat(`{"type":"panel","content":[`, 120) + strings.Repeat(`]}`, 120)
	if _, err := FromMarkdown("```adf\n"+deep+"\n```", WithPassthrough()); !errors.Is(err, ErrTooDeep) {
		t.Errorf("deep node: err = %v, want ErrTooDeep", err)
	}
}
