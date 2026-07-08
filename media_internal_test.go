package html

import "testing"

// TestExtractAttributeValue exercises the unexported tag-attribute parser
// (media.go) across the branches the public media-extraction path only hits
// incidentally: double/single quotes, unclosed quotes, unquoted values, the
// substring-rejection guard, whitespace-after-= handling, and the empty/EOF
// boundaries. This is a string-in/string-out helper, so it is driven directly.
func TestExtractAttributeValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tagContent string
		attrName   string
		want       string
	}{
		// Found, double-quoted.
		{"double quoted value", `<source src="a.mp4" type="video/mp4">`, "src", "a.mp4"},
		// Found, single-quoted.
		{"single quoted value", `<source src='b.mp4'>`, "src", "b.mp4"},
		// Uppercase attr name in content must still match a lower-case query.
		{"uppercase attr in content", `<source SRC="c.mp4">`, "src", "c.mp4"},
		// Uppercase query must still match (search is lower-cased).
		{"uppercase attr query", `<source src="d.mp4">`, "SRC", "d.mp4"},
		// Unclosed quote -> opening quote is consumed, return trimmed remainder.
		{"unclosed quote returns remainder", `<source src="e.mp4>`, "src", "e.mp4>"},
		// Unquoted value terminated by whitespace.
		{"unquoted value space-terminated", `<source src=f.mp4 type=video/mp4>`, "src", "f.mp4"},
		// Unquoted value terminated by '>'.
		{"unquoted value angle-terminated", `<source src=g.mp4>`, "src", "g.mp4"},
		// Whitespace between '=' and the value (no space before '=') is skipped.
		{"whitespace after equals", `<source src= "h.mp4">`, "src", "h.mp4"},
		// attr= with nothing after it.
		{"equals at EOF", `<source src=`, "src", ""},
		// Substring guard: "src" inside "data-src" must NOT match because the
		// preceding char ('-') is not whitespace.
		{"substring rejected data-src", `<source data-src="x.mp4">`, "src", ""},
		// Substring guard: full word at pos 0 has no preceding char, so it matches.
		{"attr at position zero", `src="j.mp4"`, "src", "j.mp4"},
		// Attribute genuinely absent.
		{"attribute absent", `<source href="k.html">`, "src", ""},
		// Empty tag content.
		{"empty content", "", "src", ""},
		// Needle longer than haystack.
		{"needle longer than content", `<a>`, "poster", ""},
		// attr name with no '=' present at all.
		{"no equals anywhere", `<video poster>`, "poster", ""},
		// Multiple attrs, second match wins (first scan stops at first match).
		{"first occurrence wins", `<a src="first" src="second">`, "src", "first"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractAttributeValue(tt.tagContent, tt.attrName)
			if got != tt.want {
				t.Errorf("extractAttributeValue(%q, %q) = %q, want %q",
					tt.tagContent, tt.attrName, got, tt.want)
			}
		})
	}
}
