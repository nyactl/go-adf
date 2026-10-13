package adf

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/util"
)

// The functions here write text so that a CommonMark parser reads it back
// as the same literal text, while escaping as little as possible to keep
// the Markdown readable.

var (
	entityRe       = regexp.MustCompile(`^&(#[0-9]{1,7}|#[xX][0-9a-fA-F]{1,6}|[A-Za-z][A-Za-z0-9]{1,31});`)
	orderedStartRe = regexp.MustCompile(`^[0-9]{1,9}[.)]($|[ \t])`)
	bulletStartRe  = regexp.MustCompile(`^[-+]($|[ \t])`)
	// Setext underlines, thematic breaks made of dashes and table delimiter
	// rows (other pipes are always escaped, so only one column matters).
	underlineRe = regexp.MustCompile(`^([-=][-= \t]*|:-+:?[ \t]*|-+:[ \t]*)$`)
	autolinkRe  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]{1,31}:[^\s<>\x00-\x1f]*$`)
)

// escapeText escapes literal text for use in a paragraph or heading.
// atLineStart tells whether the text begins a line of the output.
// lineEndings turns every CommonMark line ending into "\n".
var lineEndings = strings.NewReplacer("\r\n", "\n", "\r", "\n")

func escapeText(s string, atLineStart bool) string {
	all := strings.Split(lineEndings.Replace(s), "\n")
	// An empty line would end the paragraph.
	lines := all[:0]
	for i, l := range all {
		if i == 0 || i == len(all)-1 || strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	for i, l := range lines {
		start := atLineStart || i > 0
		if start {
			// Leading whitespace would be dropped or start a code block.
			l = strings.TrimLeftFunc(l, unicode.IsSpace)
		}
		if i < len(lines)-1 {
			// Trailing whitespace before a line break would become a hard break.
			l = strings.TrimRightFunc(l, unicode.IsSpace)
		}
		lines[i] = escapeLine(l)
	}
	return strings.Join(lines, "\n")
}

func escapeLine(l string) string {
	var b strings.Builder
	for i := 0; i < len(l); {
		r, size := utf8.DecodeRuneInString(l[i:])
		// A delimiter character at the edge of a text piece can merge with
		// the emphasis or code delimiters of its neighbour, even when
		// backslash-escaped; a character reference never does.
		if strings.ContainsRune("*_~`", r) && atEdge(l, i, size) {
			b.WriteString("&#" + strconv.Itoa(int(r)) + ";")
			i += size
			continue
		}
		switch r {
		case '\\', '`', '*', '[', ']', '<', '~', '|':
			b.WriteByte('\\')
		case '_':
			if !inWord(l, i, size) {
				b.WriteByte('\\')
			}
		case '&':
			if entityRe.MatchString(l[i:]) {
				b.WriteByte('\\')
			}
		case '!':
			if i+1 < len(l) && l[i+1] == '[' {
				b.WriteByte('\\')
			}
		case '{':
			// A color delimiter of WithExtendedSyntax.
			if loc := colorTokenRe.FindStringIndex(l[i:]); loc != nil && loc[0] == 0 {
				b.WriteByte('\\')
			}
		}
		b.WriteString(l[i : i+size])
		i += size
	}
	return b.String()
}

// inWord reports whether the rune at l[i:i+size] has letters or digits on
// both sides, where CommonMark does not treat _ as emphasis.
func inWord(l string, i, size int) bool {
	if i == 0 || i+size >= len(l) {
		return false
	}
	before, _ := utf8.DecodeLastRuneInString(l[:i])
	after, _ := utf8.DecodeRuneInString(l[i+size:])
	return isWordRune(before) && isWordRune(after)
}

// isWordRune reports whether CommonMark treats r like a letter when deciding
// whether emphasis delimiters can open or close: anything that is neither
// whitespace nor punctuation.
func isWordRune(r rune) bool {
	return r != utf8.RuneError && !util.IsSpaceRune(r) && !util.IsPunctRune(r)
}

// codeSpan writes s as inline code, choosing a backtick fence longer than
// any run of backticks inside.
func codeSpan(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	fence := strings.Repeat("`", longestRun(s, '`')+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") ||
		(strings.HasPrefix(s, " ") && strings.HasSuffix(s, " ") && strings.Trim(s, " ") != "") {
		s = " " + s + " "
	}
	return fence + s + fence
}

// codeFence returns a backtick fence for a code block holding s.
func codeFence(s string) string {
	n := 3
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if run := leadingRun(trimmed, '`'); run >= n {
			n = run + 1
		}
	}
	return strings.Repeat("`", n)
}

func longestRun(s string, c byte) int {
	longest, run := 0, 0
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return longest
}

func leadingRun(s string, c byte) int {
	n := 0
	for n < len(s) && s[n] == c {
		n++
	}
	return n
}

// linkDestination writes a link target, in angle brackets when it holds
// characters that would end a bare destination.
func linkDestination(href string) string {
	href = strings.NewReplacer("\n", "", "\r", "").Replace(href)
	if href != "" && !strings.ContainsAny(href, " ()<>\\\t") {
		return href
	}
	return "<" + strings.NewReplacer(`\`, `\\`, "<", `\<`, ">", `\>`).Replace(href) + ">"
}

func isAutolink(href string) bool {
	return autolinkRe.MatchString(href)
}

// splitSpace separates leading and trailing whitespace, which must sit
// outside emphasis delimiters for them to be recognised.
func splitSpace(s string) (lead, core, trail string) {
	core = strings.TrimLeftFunc(s, unicode.IsSpace)
	lead = s[:len(s)-len(core)]
	trimmed := strings.TrimRightFunc(core, unicode.IsSpace)
	trail = core[len(trimmed):]
	return lead, trimmed, trail
}

// atEdge reports whether the rune at l[i:i+size] has whitespace or nothing
// on one side, where it could join a neighbouring delimiter run.
func atEdge(l string, i, size int) bool {
	if i == 0 || i+size >= len(l) {
		return true
	}
	before, _ := utf8.DecodeLastRuneInString(l[:i])
	after, _ := utf8.DecodeRuneInString(l[i+size:])
	return unicode.IsSpace(before) || unicode.IsSpace(after)
}

// escapeLineStarts escapes what would start a block at the beginning of a
// line of finished inline output. It runs on whole lines because a marker
// can be split over several text pieces, such as "0" and ")".
func escapeLineStarts(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "#"), strings.HasPrefix(l, ">"), bulletStartRe.MatchString(l), underlineRe.MatchString(l):
			lines[i] = `\` + l
		case orderedStartRe.MatchString(l):
			n := strings.IndexAny(l, ".)")
			lines[i] = l[:n] + `\` + l[n:]
		}
	}
	return strings.Join(lines, "\n")
}

// escapedAt reports whether the byte at s[i] is preceded by an odd number
// of backslashes, which escape it.
func escapedAt[T string | []byte](s T, i int) bool {
	n := 0
	for i > 0 && s[i-1] == '\\' {
		n++
		i--
	}
	return n%2 == 1
}
