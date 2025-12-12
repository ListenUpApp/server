package scanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContainsHTML(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "plain text",
			input:    "This is a plain text description with no HTML.",
			expected: false,
		},
		{
			name:     "text with angle brackets but not HTML",
			input:    "Use <stdin> for input and 2 > 1 is true",
			expected: false,
		},
		{
			name:     "paragraph tags",
			input:    "<p>This is a paragraph.</p>",
			expected: true,
		},
		{
			name:     "break tags",
			input:    "Line one<br>Line two",
			expected: true,
		},
		{
			name:     "self-closing break",
			input:    "Line one<br/>Line two",
			expected: true,
		},
		{
			name:     "bold tags",
			input:    "This is <b>bold</b> text",
			expected: true,
		},
		{
			name:     "strong tags",
			input:    "This is <strong>strong</strong> text",
			expected: true,
		},
		{
			name:     "italic tags",
			input:    "This is <i>italic</i> text",
			expected: true,
		},
		{
			name:     "emphasis tags",
			input:    "This is <em>emphasized</em> text",
			expected: true,
		},
		{
			name:     "anchor tags",
			input:    `Click <a href="https://example.com">here</a>`,
			expected: true,
		},
		{
			name:     "div tags",
			input:    "<div>Content in a div</div>",
			expected: true,
		},
		{
			name:     "unordered list",
			input:    "<ul><li>Item 1</li><li>Item 2</li></ul>",
			expected: true,
		},
		{
			name:     "heading tags",
			input:    "<h1>Title</h1>",
			expected: true,
		},
		{
			name:     "uppercase tags",
			input:    "<P>Uppercase paragraph</P>",
			expected: true,
		},
		{
			name:     "mixed case tags",
			input:    "<DiV>Mixed case</DiV>",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsHTML(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHTMLToMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "plain text unchanged",
			input:    "This is plain text.",
			expected: "This is plain text.",
		},
		{
			name:     "paragraphs to newlines",
			input:    "<p>First paragraph.</p><p>Second paragraph.</p>",
			expected: "First paragraph.\n\nSecond paragraph.",
		},
		{
			name:     "bold to markdown",
			input:    "This is <b>bold</b> and <strong>strong</strong> text.",
			expected: "This is **bold** and **strong** text.",
		},
		{
			name:     "italic to markdown",
			input:    "This is <i>italic</i> and <em>emphasized</em> text.",
			expected: "This is *italic* and *emphasized* text.",
		},
		{
			name:     "links to markdown",
			input:    `Visit <a href="https://example.com">our site</a> for more.`,
			expected: "Visit [our site](https://example.com) for more.",
		},
		{
			name:     "unordered list",
			input:    "<ul><li>Item 1</li><li>Item 2</li></ul>",
			expected: "- Item 1\n- Item 2",
		},
		{
			name:     "ordered list",
			input:    "<ol><li>First</li><li>Second</li></ol>",
			expected: "1. First\n2. Second",
		},
		{
			name:     "br tags to newlines",
			input:    "Line one<br>Line two<br/>Line three",
			expected: "Line one  \nLine two  \nLine three", // trailing spaces = Markdown soft line break
		},
		{
			name:     "heading",
			input:    "<h1>Title</h1><p>Content</p>",
			expected: "# Title\n\nContent",
		},
		{
			name:     "blockquote",
			input:    "<blockquote>A wise quote</blockquote>",
			expected: "> A wise quote",
		},
		{
			name:     "complex description",
			input:    "<p><b>New York Times</b> bestselling author brings you an epic adventure.</p><p>When the world needs a hero, one woman must answer the call.</p><ul><li>Action-packed</li><li>Heartfelt</li></ul>",
			expected: "**New York Times** bestselling author brings you an epic adventure.\n\nWhen the world needs a hero, one woman must answer the call.\n\n- Action-packed\n- Heartfelt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := htmlToMarkdown(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
