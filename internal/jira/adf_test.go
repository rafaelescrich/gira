package jira

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMarkdownToADF(t *testing.T) {
	cases := []struct {
		name string
		md   string
		want ADFDoc
	}{
		{
			name: "empty",
			md:   "",
			want: ADFDoc{Version: 1, Type: "doc", Content: []ADFNode{{Type: "paragraph"}}},
		},
		{
			name: "simple paragraph",
			md:   "hello world",
			want: ADFDoc{Version: 1, Type: "doc", Content: []ADFNode{
				{Type: "paragraph", Content: []ADFNode{{Type: "text", Text: "hello world"}}},
			}},
		},
		{
			name: "multi-line paragraph joins with a space",
			md:   "line one\nline two",
			want: ADFDoc{Version: 1, Type: "doc", Content: []ADFNode{
				{Type: "paragraph", Content: []ADFNode{{Type: "text", Text: "line one line two"}}},
			}},
		},
		{
			name: "heading",
			md:   "## Title",
			want: ADFDoc{Version: 1, Type: "doc", Content: []ADFNode{
				{Type: "heading", Attrs: map[string]any{"level": 2}, Content: []ADFNode{{Type: "text", Text: "Title"}}},
			}},
		},
		{
			name: "bold and italic",
			md:   "**bold** and *italic*",
			want: ADFDoc{Version: 1, Type: "doc", Content: []ADFNode{
				{Type: "paragraph", Content: []ADFNode{
					{Type: "text", Text: "bold", Marks: []ADFMark{{Type: "strong"}}},
					{Type: "text", Text: " and "},
					{Type: "text", Text: "italic", Marks: []ADFMark{{Type: "em"}}},
				}},
			}},
		},
		{
			name: "inline code",
			md:   "run `go build`",
			want: ADFDoc{Version: 1, Type: "doc", Content: []ADFNode{
				{Type: "paragraph", Content: []ADFNode{
					{Type: "text", Text: "run "},
					{Type: "text", Text: "go build", Marks: []ADFMark{{Type: "code"}}},
				}},
			}},
		},
		{
			name: "link",
			md:   "see [docs](https://example.com)",
			want: ADFDoc{Version: 1, Type: "doc", Content: []ADFNode{
				{Type: "paragraph", Content: []ADFNode{
					{Type: "text", Text: "see "},
					{Type: "text", Text: "docs", Marks: []ADFMark{{Type: "link", Attrs: map[string]any{"href": "https://example.com"}}}},
				}},
			}},
		},
		{
			name: "bullet list",
			md:   "- one\n- two",
			want: ADFDoc{Version: 1, Type: "doc", Content: []ADFNode{
				{Type: "bulletList", Content: []ADFNode{
					{Type: "listItem", Content: []ADFNode{{Type: "paragraph", Content: []ADFNode{{Type: "text", Text: "one"}}}}},
					{Type: "listItem", Content: []ADFNode{{Type: "paragraph", Content: []ADFNode{{Type: "text", Text: "two"}}}}},
				}},
			}},
		},
		{
			name: "ordered list",
			md:   "1. one\n2. two",
			want: ADFDoc{Version: 1, Type: "doc", Content: []ADFNode{
				{Type: "orderedList", Content: []ADFNode{
					{Type: "listItem", Content: []ADFNode{{Type: "paragraph", Content: []ADFNode{{Type: "text", Text: "one"}}}}},
					{Type: "listItem", Content: []ADFNode{{Type: "paragraph", Content: []ADFNode{{Type: "text", Text: "two"}}}}},
				}},
			}},
		},
		{
			name: "nested bullet list",
			md:   "- one\n  - nested\n- two",
			want: ADFDoc{Version: 1, Type: "doc", Content: []ADFNode{
				{Type: "bulletList", Content: []ADFNode{
					{Type: "listItem", Content: []ADFNode{
						{Type: "paragraph", Content: []ADFNode{{Type: "text", Text: "one"}}},
						{Type: "bulletList", Content: []ADFNode{
							{Type: "listItem", Content: []ADFNode{{Type: "paragraph", Content: []ADFNode{{Type: "text", Text: "nested"}}}}},
						}},
					}},
					{Type: "listItem", Content: []ADFNode{{Type: "paragraph", Content: []ADFNode{{Type: "text", Text: "two"}}}}},
				}},
			}},
		},
		{
			name: "fenced code block with language",
			md:   "```go\nfmt.Println(\"hi\")\n```",
			want: ADFDoc{Version: 1, Type: "doc", Content: []ADFNode{
				{Type: "codeBlock", Attrs: map[string]any{"language": "go"}, Content: []ADFNode{{Type: "text", Text: `fmt.Println("hi")`}}},
			}},
		},
		{
			name: "fenced code block without language",
			md:   "```\nplain\n```",
			want: ADFDoc{Version: 1, Type: "doc", Content: []ADFNode{
				{Type: "codeBlock", Content: []ADFNode{{Type: "text", Text: "plain"}}},
			}},
		},
		{
			name: "plain text falls back to a paragraph",
			md:   "> not a supported blockquote syntax",
			want: ADFDoc{Version: 1, Type: "doc", Content: []ADFNode{
				{Type: "paragraph", Content: []ADFNode{{Type: "text", Text: "> not a supported blockquote syntax"}}},
			}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MarkdownToADF(tc.md)
			gotJSON, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal got: %v", err)
			}
			wantJSON, err := json.Marshal(tc.want)
			if err != nil {
				t.Fatalf("marshal want: %v", err)
			}
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("MarkdownToADF(%q) =\n%s\nwant\n%s", tc.md, gotJSON, wantJSON)
			}
		})
	}
}

// TestMarkdownADFRoundTrip checks that converting Markdown to ADF and back
// through RenderADF (internal/jira's ADF reader) reproduces equivalent
// text for the supported subset.
func TestMarkdownADFRoundTrip(t *testing.T) {
	cases := []string{
		"hello world",
		"## Title",
		"**bold** and *italic* and `code`",
		"see [docs](https://example.com)",
		"- one\n- two",
		"1. one\n2. two",
		"```go\nfmt.Println(\"hi\")\n```",
	}

	for _, md := range cases {
		t.Run(md, func(t *testing.T) {
			doc := MarkdownToADF(md)
			raw, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got := strings.TrimSpace(RenderADF(raw))
			want := strings.TrimSpace(md)
			if got != want {
				t.Errorf("round trip mismatch:\n got: %q\nwant: %q", got, want)
			}
		})
	}
}

func TestMarkdownToADFNestedListRoundTrip(t *testing.T) {
	md := "- one\n  - nested\n- two"
	doc := MarkdownToADF(md)
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := strings.TrimSpace(RenderADF(raw))
	want := "- one\n  - nested\n- two"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
