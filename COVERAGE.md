# Coverage

How each node and mark type of the [ADF JSON schema](testdata/schema/README.md) is converted. A test fails when the schema gains a type that is not listed here.

**ToMarkdown** is the ADF → Markdown direction, **FromMarkdown** the Markdown → ADF direction. "Text kept" means the type has no Markdown form, but its text content is written, so nothing a reader needs is lost.

## Nodes

| Type | ToMarkdown | FromMarkdown |
|---|---|---|
| `doc` | document | document |
| `paragraph` | paragraph | paragraph |
| `text` | text, escaped so it reads back literally | text |
| `hardBreak` | line break; `<br>` in table cells | line break, `<br>` |
| `heading` | `#` heading; a bold paragraph inside list items and quotes | heading; a bold paragraph where ADF allows no heading |
| `bulletList`, `orderedList`, `listItem` | lists, with the start number | lists, with the start number |
| `codeBlock` | fenced code with language | fenced and indented code |
| `blockquote` | quote | quote; nested quotes are flattened, as ADF has none |
| `rule` | `---` | thematic break |
| `table`, `tableRow`, `tableHeader`, `tableCell` | GFM table; several blocks in a cell are joined with `<br>` | GFM table |
| `mention` | `@[label]`, label from `WithMentionText` | `@[label]` with `WithMentionResolver` |
| `emoji` | its character, or its short name | – (stays text) |
| `inlineCard` | `<url>` | – (an autolink becomes a link) |
| `blockCard`, `embedCard` | `<url>` | – |
| `date` | `YYYY-MM-DD` (UTC) | – |
| `status` | `[TEXT]` | – |
| `panel` | quote starting with the panel type in bold | – |
| `expand`, `nestedExpand` | title in bold, then the content | – |
| `taskList`, `taskItem`, `blockTaskItem` | `- [ ]` / `- [x]` | – (a GFM task list becomes a list whose items start with `[ ]` / `[x]`) |
| `decisionList`, `decisionItem` | `- Decision: …` | – |
| `mediaSingle`, `mediaGroup`, `media`, `mediaInline` | `[attachment: name]` | – (an image becomes a link) |
| `caption` | its text, after the attachment | – |
| `layoutSection`, `layoutColumn` | columns one after another | – |
| `bodiedExtension`, `bodiedSyncBlock` | their content | – |
| `extension`, `inlineExtension`, `syncBlock` | left out: they hold no text, only a reference to an app or another page | – |
| `placeholder` | left out | – |

Node types unknown to this version are written as their text content.

## Marks

| Type | ToMarkdown | FromMarkdown |
|---|---|---|
| `strong`, `em`, `strike` | `**`, `*`, `~~` | `**`/`__`, `*`/`_`, `~~`/`~` |
| `code` | inline code | inline code |
| `link` | `[text](url)`, `<url>` when the text is the URL | links and autolinks |
| `underline`, `subsup`, `textColor`, `backgroundColor`, `fontSize`, `annotation`, `fragment`, `dataConsumer`, `alignment`, `indentation`, `breakout`, `border` | text kept, mark dropped | – |

Where CommonMark cannot express emphasis at a position (its flanking rules leave gaps, for example emphasis ending between punctuation and a letter), ToMarkdown leaves that emphasis out rather than write Markdown that reads back differently.
