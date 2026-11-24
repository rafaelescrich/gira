package jira

import (
	"encoding/json"
	"strconv"
	"strings"
)

// This file holds a minimal Atlassian Document Format (ADF) reader: ADF
// JSON -> plain-text/Markdown-ish text, good enough to render issue
// descriptions and comments in the CLI/MCP output.
//
// A separate, fuller Markdown<->ADF converter (adf.go, covering writes too)
// is planned; extend RenderADF/renderNode here rather than duplicating this
// logic.

// adfNode is a loosely-typed ADF node: enough structure to walk the tree
// without needing a type per node kind.
type adfNode struct {
	Type    string          `json:"type"`
	Text    string          `json:"text"`
	Content []adfNode       `json:"content"`
	Marks   []adfMark       `json:"marks"`
	Attrs   json.RawMessage `json:"attrs"`
}

type adfMark struct {
	Type  string          `json:"type"`
	Attrs json.RawMessage `json:"attrs"`
}

type linkAttrs struct {
	Href string `json:"href"`
}

type headingAttrs struct {
	Level int `json:"level"`
}

// RenderADF converts raw ADF JSON (as returned for description/comment
// body fields) into Markdown-ish plain text. Unknown node types fall back
// to rendering their text content (via child text nodes) rather than being
// dropped silently, per the blueprint's ADF read contract.
//
// An empty or unparsable doc renders as "".
func RenderADF(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var root adfNode
	if err := json.Unmarshal(raw, &root); err != nil {
		return ""
	}
	var sb strings.Builder
	renderBlockChildren(&sb, root.Content)
	return strings.TrimRight(sb.String(), "\n")
}

func renderBlockChildren(sb *strings.Builder, nodes []adfNode) {
	for i, n := range nodes {
		renderBlock(sb, n)
		if i < len(nodes)-1 {
			sb.WriteString("\n")
		}
	}
}

func renderBlock(sb *strings.Builder, n adfNode) {
	switch n.Type {
	case "paragraph":
		renderInlineChildren(sb, n.Content)
		sb.WriteString("\n")
	case "heading":
		level := 1
		var attrs headingAttrs
		if n.Attrs != nil {
			if err := json.Unmarshal(n.Attrs, &attrs); err == nil && attrs.Level > 0 {
				level = attrs.Level
			}
		}
		sb.WriteString(strings.Repeat("#", level) + " ")
		renderInlineChildren(sb, n.Content)
		sb.WriteString("\n")
	case "bulletList":
		renderList(sb, n.Content, "- ")
	case "orderedList":
		renderOrderedList(sb, n.Content)
	case "listItem":
		renderBlockChildren(sb, n.Content)
	case "codeBlock":
		lang := ""
		if n.Attrs != nil {
			var attrs struct {
				Language string `json:"language"`
			}
			if err := json.Unmarshal(n.Attrs, &attrs); err == nil {
				lang = attrs.Language
			}
		}
		sb.WriteString("```" + lang + "\n")
		renderInlineChildren(sb, n.Content)
		sb.WriteString("\n```\n")
	case "blockquote":
		var inner strings.Builder
		renderBlockChildren(&inner, n.Content)
		for _, line := range strings.Split(strings.TrimRight(inner.String(), "\n"), "\n") {
			sb.WriteString("> " + line + "\n")
		}
	case "rule":
		sb.WriteString("---\n")
	case "table":
		// Minimal: render rows as pipe-separated text.
		renderBlockChildren(sb, n.Content)
	case "tableRow":
		var cells []string
		for _, cell := range n.Content {
			var cellSb strings.Builder
			renderBlockChildren(&cellSb, cell.Content)
			cells = append(cells, strings.TrimSpace(cellSb.String()))
		}
		sb.WriteString(strings.Join(cells, " | ") + "\n")
	case "tableCell", "tableHeader":
		renderBlockChildren(sb, n.Content)
	case "mediaSingle", "media", "mediaGroup":
		sb.WriteString("[attachment]\n")
	case "panel", "expand":
		renderBlockChildren(sb, n.Content)
	case "":
		// No type (shouldn't happen for well-formed ADF); fall through to
		// text/child rendering below.
		fallthrough
	default:
		// Unknown node: never drop it silently. Render its own text (for
		// text-ish leaf nodes) or recurse into children.
		if n.Text != "" {
			sb.WriteString(n.Text)
		}
		if len(n.Content) > 0 {
			renderBlockChildren(sb, n.Content)
		}
	}
}

// renderListItemLines renders a listItem's block children (typically a
// paragraph, optionally followed by a nested list) as plain lines, without
// the blank-line separator renderBlockChildren normally inserts between
// sibling blocks — a nested sublist should immediately follow its parent
// item's text, not sit behind a blank line.
func renderListItemLines(item adfNode) []string {
	var lines []string
	for _, child := range item.Content {
		var childSb strings.Builder
		renderBlock(&childSb, child)
		text := strings.TrimRight(childSb.String(), "\n")
		if text == "" {
			continue
		}
		lines = append(lines, strings.Split(text, "\n")...)
	}
	return lines
}

func renderList(sb *strings.Builder, items []adfNode, marker string) {
	for _, item := range items {
		lines := renderListItemLines(item)
		for i, line := range lines {
			if i == 0 {
				sb.WriteString(marker + line + "\n")
			} else {
				sb.WriteString(strings.Repeat(" ", len(marker)) + line + "\n")
			}
		}
	}
}

func renderOrderedList(sb *strings.Builder, items []adfNode) {
	for i, item := range items {
		marker := strconv.Itoa(i+1) + ". "
		lines := renderListItemLines(item)
		for j, line := range lines {
			if j == 0 {
				sb.WriteString(marker + line + "\n")
			} else {
				sb.WriteString(strings.Repeat(" ", len(marker)) + line + "\n")
			}
		}
	}
}

func renderInlineChildren(sb *strings.Builder, nodes []adfNode) {
	for _, n := range nodes {
		renderInline(sb, n)
	}
}

func renderInline(sb *strings.Builder, n adfNode) {
	switch n.Type {
	case "text":
		text := n.Text
		var isCode, isStrong, isEm bool
		var href string
		for _, m := range n.Marks {
			switch m.Type {
			case "code":
				isCode = true
			case "strong":
				isStrong = true
			case "em":
				isEm = true
			case "link":
				var a linkAttrs
				if m.Attrs != nil {
					_ = json.Unmarshal(m.Attrs, &a)
				}
				href = a.Href
			}
		}
		if isCode {
			text = "`" + text + "`"
		}
		if isStrong {
			text = "**" + text + "**"
		}
		if isEm {
			text = "*" + text + "*"
		}
		if href != "" {
			text = "[" + text + "](" + href + ")"
		}
		sb.WriteString(text)
	case "hardBreak":
		sb.WriteString("\n")
	case "mention":
		sb.WriteString("@" + n.Text)
	case "emoji":
		sb.WriteString(n.Text)
	case "inlineCard", "link":
		sb.WriteString(n.Text)
	default:
		if n.Text != "" {
			sb.WriteString(n.Text)
		}
		if len(n.Content) > 0 {
			renderInlineChildren(sb, n.Content)
		}
	}
}
