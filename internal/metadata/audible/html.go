package audible

import (
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// stripHTML removes HTML tags and returns plain text.
// Handles common HTML entities and collapses whitespace.
func stripHTML(s string) string {
	if s == "" {
		return ""
	}

	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		// If parsing fails, fall back to regex stripping
		return stripHTMLFallback(s)
	}

	var buf strings.Builder
	extractText(doc, &buf)

	// Collapse multiple spaces and trim
	result := collapseWhitespace(buf.String())
	return strings.TrimSpace(result)
}

// extractText recursively extracts text content from HTML nodes.
func extractText(n *html.Node, buf *strings.Builder) {
	if n.Type == html.TextNode {
		buf.WriteString(n.Data)
	}

	// Add space after block elements
	if n.Type == html.ElementNode {
		switch n.Data {
		case "p", "div", "br", "li", "h1", "h2", "h3", "h4", "h5", "h6":
			buf.WriteString(" ")
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		extractText(c, buf)
	}

	// Add space after block elements (closing)
	if n.Type == html.ElementNode {
		switch n.Data {
		case "p", "div", "li", "h1", "h2", "h3", "h4", "h5", "h6":
			buf.WriteString(" ")
		}
	}
}

// stripHTMLFallback uses regex when parsing fails.
var htmlTagRegex = regexp.MustCompile(`<[^>]*>`)

func stripHTMLFallback(s string) string {
	s = htmlTagRegex.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return collapseWhitespace(s)
}

// collapseWhitespace replaces multiple whitespace with single space.
var whitespaceRegex = regexp.MustCompile(`\s+`)

func collapseWhitespace(s string) string {
	return whitespaceRegex.ReplaceAllString(s, " ")
}
