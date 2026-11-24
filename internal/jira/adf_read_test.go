package jira

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderADF(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want string
	}{
		{
			name: "empty",
			doc:  ``,
			want: "",
		},
		{
			name: "simple paragraph",
			doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hello world"}]}]}`,
			want: "hello world",
		},
		{
			name: "bold and italic marks",
			doc: `{"type":"doc","content":[{"type":"paragraph","content":[
				{"type":"text","text":"bold","marks":[{"type":"strong"}]},
				{"type":"text","text":" and "},
				{"type":"text","text":"italic","marks":[{"type":"em"}]}
			]}]}`,
			want: "**bold** and *italic*",
		},
		{
			name: "heading",
			doc:  `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Title"}]}]}`,
			want: "## Title",
		},
		{
			name: "bullet list",
			doc: `{"type":"doc","content":[{"type":"bulletList","content":[
				{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"one"}]}]},
				{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"two"}]}]}
			]}]}`,
			want: "- one\n- two",
		},
		{
			name: "unknown node renders its text, not dropped",
			doc:  `{"type":"doc","content":[{"type":"someFutureNode","text":"still visible"}]}`,
			want: "still visible",
		},
		{
			name: "link mark",
			doc:  `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"docs","marks":[{"type":"link","attrs":{"href":"https://example.com"}}]}]}]}`,
			want: "[docs](https://example.com)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RenderADF(json.RawMessage(tc.doc))
			got = strings.TrimSpace(got)
			want := strings.TrimSpace(tc.want)
			if got != want {
				t.Errorf("RenderADF() = %q, want %q", got, want)
			}
		})
	}
}
