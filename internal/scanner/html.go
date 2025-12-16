package scanner

import (
	"regexp"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

// htmlTagPattern matches common HTML tags to detect if a string contains HTML.
// Looks for opening tags like <p>, <br>, <div>, <b>, etc.
var htmlTagPattern = regexp.MustCompile(`<(p|br|div|span|b|i|strong|em|a|ul|ol|li|h[1-6]|blockquote)[\s>/]`)

// containsHTML checks if a string appears to contain HTML markup.
// Returns true if common HTML tags are detected.
func containsHTML(s string) bool {
	return htmlTagPattern.MatchString(strings.ToLower(s))
}

// htmlToMarkdown converts HTML content to Markdown.
// If the input doesn't contain HTML, it's returned unchanged.
func htmlToMarkdown(s string) string {
	if s == "" || !containsHTML(s) {
		return s
	}

	markdown, err := htmltomarkdown.ConvertString(s)
	if err != nil {
		// If conversion fails, return the original string
		return s
	}

	return strings.TrimSpace(markdown)
}
