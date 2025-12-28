package audible

import "testing"

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "plain text unchanged",
			input: "Just plain text",
			want:  "Just plain text",
		},
		{
			name:  "removes simple tags",
			input: "<p>Hello</p><p>World</p>",
			want:  "Hello World",
		},
		{
			name:  "handles br tags",
			input: "Line 1<br>Line 2<br/>Line 3",
			want:  "Line 1 Line 2 Line 3",
		},
		{
			name:  "removes nested tags",
			input: "<div><p><b>Bold</b> and <i>italic</i></p></div>",
			want:  "Bold and italic",
		},
		{
			name:  "handles entities",
			input: "&amp; &lt; &gt; &quot;",
			want:  "& < > \"",
		},
		{
			name:  "typical Audible description",
			input: "<p>An epic fantasy novel.</p><p>By a famous author.</p>",
			want:  "An epic fantasy novel. By a famous author.",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only whitespace",
			input: "   \n\t  ",
			want:  "",
		},
		{
			name:  "collapses multiple spaces",
			input: "<p>Too    many     spaces</p>",
			want:  "Too many spaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTML(tt.input)
			if got != tt.want {
				t.Errorf("stripHTML() = %q, want %q", got, tt.want)
			}
		})
	}
}
