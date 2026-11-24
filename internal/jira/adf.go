package jira

// This file holds the write-side counterpart to adf_read.go: a converter
// from a small Markdown subset to Atlassian Document Format (ADF), used to
// build description and comment bodies for the v3 API (blueprint §4).
//
// Supported subset: paragraphs, headings (# .. ######), bullet and ordered
// lists (including nesting by indentation), fenced code blocks (with an
// optional language), inline code, bold, italic, and links. Anything else
// is rendered as a plain-text paragraph rather than dropped.

import (
	"regexp"
	"strings"
)

// ADFDoc is the top-level Atlassian Document Format document used for issue
// descriptions and comment bodies.
type ADFDoc struct {
	Version int       `json:"version"`
	Type    string    `json:"type"`
	Content []ADFNode `json:"content"`
}

// ADFNode is a generic ADF node: a block or inline element. Which fields
// are meaningful depends on Type (e.g. "text" nodes carry Text and Marks;
// container nodes like "paragraph" or "bulletList" carry Content).
type ADFNode struct {
	Type    string         `json:"type"`
	Text    string         `json:"text,omitempty"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Marks   []ADFMark      `json:"marks,omitempty"`
	Content []ADFNode      `json:"content,omitempty"`
}

// ADFMark is a text mark (strong, em, code, link, ...).
type ADFMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

var (
	fenceRe   = regexp.MustCompile("^```([A-Za-z0-9_+-]*)\\s*$")
	headingRe = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	bulletRe  = regexp.MustCompile(`^(\s*)[-*+]\s+(.+)$`)
	orderedRe = regexp.MustCompile(`^(\s*)(\d+)[.)]\s+(.+)$`)
	linkRe    = regexp.MustCompile(`^\[([^\]]*)\]\(([^)]+)\)`)
)

// MarkdownToADF converts a Markdown subset to an ADF document. Empty input
// produces a document with a single empty paragraph, matching what Jira
// expects for a "blank" description/comment body.
func MarkdownToADF(markdown string) ADFDoc {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")

	var content []ADFNode
	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}

		if m := fenceRe.FindStringSubmatch(line); m != nil {
			lang := m[1]
			i++
			var codeLines []string
			for i < len(lines) && !isFenceClose(lines[i]) {
				codeLines = append(codeLines, lines[i])
				i++
			}
			if i < len(lines) {
				i++ // consume closing fence
			}
			node := ADFNode{Type: "codeBlock"}
			if lang != "" {
				node.Attrs = map[string]any{"language": lang}
			}
			if text := strings.Join(codeLines, "\n"); text != "" {
				node.Content = []ADFNode{{Type: "text", Text: text}}
			}
			content = append(content, node)
			continue
		}

		if m := headingRe.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			content = append(content, ADFNode{
				Type:    "heading",
				Attrs:   map[string]any{"level": level},
				Content: parseInline(m[2]),
			})
			i++
			continue
		}

		if isListLine(line) {
			indent, _, _, _ := matchListLine(line)
			var node ADFNode
			node, i = parseList(lines, i, indent)
			content = append(content, node)
			continue
		}

		// Paragraph: gather consecutive plain lines, joined with a space,
		// until a blank line or the start of another block.
		var paraLines []string
		for i < len(lines) {
			l := lines[i]
			if strings.TrimSpace(l) == "" {
				break
			}
			if fenceRe.MatchString(l) || headingRe.MatchString(l) || isListLine(l) {
				break
			}
			paraLines = append(paraLines, strings.TrimSpace(l))
			i++
		}
		content = append(content, ADFNode{
			Type:    "paragraph",
			Content: parseInline(strings.Join(paraLines, " ")),
		})
	}

	if len(content) == 0 {
		content = []ADFNode{{Type: "paragraph"}}
	}

	return ADFDoc{Version: 1, Type: "doc", Content: content}
}

func isFenceClose(line string) bool {
	return strings.HasPrefix(strings.TrimRight(line, " \t"), "```")
}

// matchListLine reports whether line is a bullet or ordered list item,
// returning its indentation (leading space count), whether it's ordered,
// and the text after the marker.
func matchListLine(line string) (indent int, ordered bool, text string, ok bool) {
	if m := bulletRe.FindStringSubmatch(line); m != nil {
		return len(m[1]), false, m[2], true
	}
	if m := orderedRe.FindStringSubmatch(line); m != nil {
		return len(m[1]), true, m[3], true
	}
	return 0, false, "", false
}

func isListLine(line string) bool {
	_, _, _, ok := matchListLine(line)
	return ok
}

// parseList parses a run of list items at exactly the given indent level
// (bullet or ordered, matching whichever the first line at that level is),
// recursing into parseList for any more-indented nested list that follows
// an item. It returns the built list node and the index just past what it
// consumed.
func parseList(lines []string, i int, indent int) (ADFNode, int) {
	ordered := false
	if i < len(lines) {
		if _, o, _, ok := matchListLine(lines[i]); ok {
			ordered = o
		}
	}

	var items []ADFNode
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			j := i + 1
			for j < len(lines) && strings.TrimSpace(lines[j]) == "" {
				j++
			}
			if j >= len(lines) {
				i = j
				break
			}
			nIndent, nOrdered, _, ok := matchListLine(lines[j])
			if !ok || nIndent != indent || nOrdered != ordered {
				i = j
				break
			}
			i = j
			line = lines[i]
		}

		lIndent, lOrdered, text, ok := matchListLine(line)
		if !ok || lIndent != indent || lOrdered != ordered {
			break
		}
		i++

		itemContent := []ADFNode{{Type: "paragraph", Content: parseInline(text)}}
		if i < len(lines) {
			if nIndent, _, _, ok := matchListLine(lines[i]); ok && nIndent > indent {
				var nested ADFNode
				nested, i = parseList(lines, i, nIndent)
				itemContent = append(itemContent, nested)
			}
		}
		items = append(items, ADFNode{Type: "listItem", Content: itemContent})
	}

	typ := "bulletList"
	if ordered {
		typ = "orderedList"
	}
	return ADFNode{Type: typ, Content: items}, i
}

// parseInline parses inline Markdown (code spans, links, bold, italic)
// within a single block of text into ADF inline nodes.
func parseInline(s string) []ADFNode {
	var nodes []ADFNode
	var buf strings.Builder

	flush := func() {
		if buf.Len() > 0 {
			nodes = append(nodes, ADFNode{Type: "text", Text: buf.String()})
			buf.Reset()
		}
	}

	i := 0
	for i < len(s) {
		// Inline code span: `...`
		if s[i] == '`' {
			if end := strings.IndexByte(s[i+1:], '`'); end >= 0 {
				code := s[i+1 : i+1+end]
				if code != "" {
					flush()
					nodes = append(nodes, ADFNode{Type: "text", Text: code, Marks: []ADFMark{{Type: "code"}}})
					i = i + 1 + end + 1
					continue
				}
			}
		}

		// Link: [text](url)
		if s[i] == '[' {
			if m := linkRe.FindStringSubmatch(s[i:]); m != nil {
				text, href := m[1], m[2]
				if text != "" {
					flush()
					nodes = append(nodes, ADFNode{
						Type:  "text",
						Text:  text,
						Marks: []ADFMark{{Type: "link", Attrs: map[string]any{"href": href}}},
					})
					i += len(m[0])
					continue
				}
			}
		}

		// Bold: **text** or __text__
		if i+1 < len(s) && (strings.HasPrefix(s[i:], "**") || strings.HasPrefix(s[i:], "__")) {
			delim := s[i : i+2]
			if end := strings.Index(s[i+2:], delim); end > 0 {
				inner := s[i+2 : i+2+end]
				flush()
				nodes = append(nodes, ADFNode{Type: "text", Text: inner, Marks: []ADFMark{{Type: "strong"}}})
				i = i + 2 + end + 2
				continue
			}
		}

		// Italic: *text* or _text_
		if s[i] == '*' || s[i] == '_' {
			delim := s[i]
			if end := strings.IndexByte(s[i+1:], delim); end > 0 {
				inner := s[i+1 : i+1+end]
				flush()
				nodes = append(nodes, ADFNode{Type: "text", Text: inner, Marks: []ADFMark{{Type: "em"}}})
				i = i + 1 + end + 1
				continue
			}
		}

		buf.WriteByte(s[i])
		i++
	}
	flush()
	return nodes
}
