package internal

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestWalkNodes(t *testing.T) {
	t.Parallel()

	doc, _ := html.Parse(strings.NewReader(`<html><body><p>Test</p></body></html>`))

	count := 0
	WalkNodes(doc, func(n *html.Node) bool {
		count++
		return true
	})

	if count == 0 {
		t.Error("WalkNodes() should visit nodes")
	}
}

func TestWalkNodesEarlyStop(t *testing.T) {
	t.Parallel()

	doc, _ := html.Parse(strings.NewReader(`<html><body><p>Test</p></body></html>`))

	count := 0
	WalkNodes(doc, func(n *html.Node) bool {
		count++
		return count < 2 // Stop after 2 nodes
	})

	if count != 2 {
		t.Errorf("WalkNodes() visited %d nodes, want 2", count)
	}
}

func TestWalkNodesNil(t *testing.T) {
	t.Parallel()

	// Should not panic
	WalkNodes(nil, func(n *html.Node) bool {
		t.Error("Should not visit nodes when root is nil")
		return true
	})
}

func TestFindElementByTag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		html    string
		tag     string
		wantNil bool
	}{
		{
			name:    "find title",
			html:    `<html><head><title>Test</title></head></html>`,
			tag:     "title",
			wantNil: false,
		},
		{
			name:    "find body",
			html:    `<html><body><p>Test</p></body></html>`,
			tag:     "body",
			wantNil: false,
		},
		{
			name:    "tag not found",
			html:    `<html><body><p>Test</p></body></html>`,
			tag:     "article",
			wantNil: true,
		},
		{
			name:    "nil document",
			html:    "",
			tag:     "p",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var doc *html.Node
			if tt.html != "" {
				doc, _ = html.Parse(strings.NewReader(tt.html))
			}

			result := FindElementByTag(doc, tt.tag)

			if tt.wantNil && result != nil {
				t.Errorf("FindElementByTag() = %v, want nil", result)
			}
			if !tt.wantNil && result == nil {
				t.Errorf("FindElementByTag() = nil, want non-nil")
			}
			if result != nil && result.Data != tt.tag {
				t.Errorf("FindElementByTag() found %q, want %q", result.Data, tt.tag)
			}
		})
	}
}

func TestGetTextContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "simple text",
			html: `<p>Hello World</p>`,
			want: "Hello World",
		},
		{
			name: "nested text",
			html: `<div><p>Hello <strong>World</strong></p></div>`,
			want: "Hello World",
		},
		{
			name: "empty",
			html: `<p></p>`,
			want: "",
		},
		{
			name: "whitespace only",
			html: `<p>   </p>`,
			want: "",
		},
		{
			name: "inline elements without space",
			html: `<span>F-<a href="#">2</a></span>`,
			want: "F-2",
		},
		{
			name: "inline elements with space in HTML",
			html: `<span>F- <a href="#">2</a></span>`,
			want: "F- 2",
		},
		{
			name: "nested span without space",
			html: `<div><span>Hello</span><span>World</span></div>`,
			want: "HelloWorld",
		},
		{
			name: "nested span with space in HTML",
			html: `<div><span>Hello</span> <span>World</span></div>`,
			want: "Hello World",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			result := GetTextContent(doc)

			if result != tt.want {
				t.Errorf("GetTextContent() = %q, want %q", result, tt.want)
			}
		})
	}
}

func TestGetTextContentNil(t *testing.T) {
	t.Parallel()

	result := GetTextContent(nil)
	if result != "" {
		t.Errorf("GetTextContent(nil) = %q, want empty string", result)
	}
}

func TestCleanText(t *testing.T) {
	t.Parallel()

	t.Run("without regex", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
			want  string
		}{
			{
				name:  "HTML entities",
				input: "&lt;html&gt; &amp;",
				want:  "<html> &",
			},
			{
				name:  "empty",
				input: "",
				want:  "",
			},
			{
				name:  "simple text",
				input: "Hello World",
				want:  "Hello World",
			},
			{
				name:  "newlines preserved",
				input: "Line1\nLine2",
				want:  "Line1\nLine2",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := CleanText(tt.input)
				if result != tt.want {
					t.Errorf("CleanText() = %q, want %q", result, tt.want)
				}
			})
		}
	})

	t.Run("with regex", func(t *testing.T) {
		tests := []struct {
			name  string
			input string
			want  string
		}{
			{
				name:  "multiple spaces",
				input: "Hello    World",
				want:  "Hello World",
			},
			{
				name:  "tabs and spaces",
				input: "Hello\t\t\tWorld",
				want:  "Hello World",
			},
			{
				name:  "mixed whitespace",
				input: "Hello  \t  \n  World",
				want:  "Hello\n  World", // Updated: preserve leading indentation
			},
			{
				name:  "leading spaces",
				input: "    Hello",
				want:  "    Hello", // Updated: preserve leading indentation for document hierarchy
			},
			{
				name:  "trailing spaces",
				input: "Hello    ",
				want:  "Hello",
			},
			{
				name:  "multiple newlines collapsed to one blank line",
				input: "Line1\n\n\nLine2",
				want:  "Line1\n\nLine2", // Preserves paragraph spacing with one blank line
			},
			{
				name:  "only whitespace",
				input: "     ",
				want:  "",
			},
			{
				name:  "unicode characters",
				input: "Hello   世界   Test",
				want:  "Hello 世界 Test",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := CleanText(tt.input)
				if result != tt.want {
					t.Errorf("CleanText() = %q, want %q", result, tt.want)
				}
			})
		}
	})

	t.Run("edge cases", func(t *testing.T) {
		t.Run("very long text", func(t *testing.T) {
			longText := strings.Repeat("word ", 10000)
			result := CleanText(longText)
			if len(result) == 0 {
				t.Error("CleanText() should handle long text")
			}
		})

		t.Run("special characters", func(t *testing.T) {
			input := "Test   @#$%   Special"
			result := CleanText(input)
			if !strings.Contains(result, "@#$%") {
				t.Error("CleanText() should preserve special chars")
			}
		})
	})
}

func TestReplaceHTMLEntities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"&nbsp;", " "},
		{"&amp;", "&"},
		{"&lt;", "<"},
		{"&gt;", ">"},
		{"&quot;", "\""},
		{"&apos;", "'"},
		{"&mdash;", "—"},
		{"&ndash;", "–"},
		{"&#8212;", "—"},
		{"&#x2014;", "—"},
		{"&#160;", " "},
		{"&#xa0;", " "},
		{"&hellip;", "…"},
		{"&copy;", "©"},
		{"no entities", "no entities"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ReplaceHTMLEntities(tt.input)
			if result != tt.want {
				t.Errorf("ReplaceHTMLEntities(%q) = %q, want %q", tt.input, result, tt.want)
			}
		})
	}
}

func TestIsExternalURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		url  string
		want bool
	}{
		{"http://example.com", true},
		{"https://example.com", true},
		{"//example.com", true},
		{"/page.html", false},
		{"page.html", false},
		{"#anchor", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := IsExternalURL(tt.url)
			if result != tt.want {
				t.Errorf("IsExternalURL(%q) = %v, want %v", tt.url, result, tt.want)
			}
		})
	}
}

func TestSelectBestCandidate(t *testing.T) {
	t.Parallel()

	doc, _ := html.Parse(strings.NewReader(`<html><body><div id="a"></div><div id="b"></div></body></html>`))

	var nodeA, nodeB *html.Node
	WalkNodes(doc, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "div" {
			for _, attr := range n.Attr {
				if attr.Key == "id" && attr.Val == "a" {
					nodeA = n
				} else if attr.Key == "id" && attr.Val == "b" {
					nodeB = n
				}
			}
		}
		return true
	})

	tests := []struct {
		name       string
		candidates map[*html.Node]int
		wantNil    bool
	}{
		{
			name:       "empty candidates",
			candidates: map[*html.Node]int{},
			wantNil:    true,
		},
		{
			name: "single candidate",
			candidates: map[*html.Node]int{
				nodeA: 100,
			},
			wantNil: false,
		},
		{
			name: "multiple candidates",
			candidates: map[*html.Node]int{
				nodeA: 100,
				nodeB: 200,
			},
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SelectBestCandidate(tt.candidates)

			if tt.wantNil && result != nil {
				t.Error("SelectBestCandidate() should return nil for empty candidates")
			}
			if !tt.wantNil && result == nil {
				t.Error("SelectBestCandidate() should return non-nil")
			}
		})
	}
}

// Tests for non-breaking space handling in helper functions
// These tests verify that &nbsp;, &#160;, and &#xa0; are properly converted to regular spaces

func TestCalculateContentDensityWithNbsp(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		html        string
		minExpected float64
		description string
	}{
		{
			name:        "Pure text with nbsp",
			html:        "<p>Hello&nbsp;World</p>",
			minExpected: 0.2, // Reasonable text density
			description: "Should have reasonable text density",
		},
		{
			name:        "Text with tags and nbsp",
			html:        `<div><p>Hello&nbsp;World</p><p>Test&nbsp;Content</p></div>`,
			minExpected: 0.3, // Good text density
			description: "Should have good text density",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader(tc.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			result := CalculateContentDensity(doc)
			if result < tc.minExpected {
				t.Errorf("%s: CalculateContentDensity() = %f, want >= %f", tc.description, result, tc.minExpected)
			}
		})
	}
}

func BenchmarkGetTextContent(b *testing.B) {
	doc, _ := html.Parse(strings.NewReader(`<html><body><p>Hello World</p><p>More text</p></body></html>`))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetTextContent(doc)
	}
}

func BenchmarkCleanText(b *testing.B) {
	text := "Hello    World\n\nWith   multiple   spaces"

	for i := 0; i < b.N; i++ {
		CleanText(text)
	}
}

// Benchmarks for performance optimizations

func BenchmarkNormalizeNonBreakingSpaces(b *testing.B) {
	tests := []struct {
		name string
		text string
	}{
		{"NoNBSP", "This is regular text without non-breaking spaces"},
		{"WithNBSP", "This\u00a0is\u00a0text\u00a0with\u00a0NBSP"},
		{"LargeNoNBSP", strings.Repeat("Regular text ", 100)},
		{"LargeWithNBSP", strings.Repeat("Text\u00a0", 100)},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				normalizeNonBreakingSpaces(tt.text)
			}
		})
	}
}

func BenchmarkCleanTextEarlyExit(b *testing.B) {
	tests := []struct {
		name string
		text string
	}{
		{"CleanText", "This is clean text without special characters"},
		{"WithNewlines", "Line1\nLine2\nLine3"},
		{"WithMultipleSpaces", "Text  with   multiple    spaces"},
		{"WithNBSP", "Text\u00a0with\u00a0NBSP"},
		{"ComplexText", "Line1\n  Text  with   spaces\u00a0and NBSP"},
		{"LargeCleanText", strings.Repeat("Clean text ", 100)},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				CleanText(tt.text)
			}
		})
	}
}

// ============================================================================
// IsValidURL Tests
// ============================================================================

func TestIsValidURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		// Valid URLs
		{"simple path", "/path/to/resource", true},
		{"relative path", "image.jpg", true},
		{"http URL", "http://example.com", true},
		{"https URL", "https://example.com/path", true},
		{"URL with query", "/path?query=value", true},
		{"URL with fragment", "/path#section", true},
		{"data URL small", "data:text/plain;base64,SGVsbG8=", true},
		{"protocol relative", "//example.com/path", true},
		{"URL with port", "http://example.com:8080/path", true},
		{"dot relative path", "./image.png", true},

		// Invalid URLs
		{"empty string", "", false},
		{"URL with newline", "http://example.com\nmalicious", false},
		{"URL with tab", "http://example.com\tmalicious", false},
		{"URL with control char", "http://example.com\x00malicious", false},
		{"URL with angle bracket", "http://example.com<script>", false},
		{"URL with quote", "http://example.com'onclick", false},
		{"URL with double quote", "http://example.com\"onclick", false},

		// Edge cases
		{"very long URL over limit", strings.Repeat("a", MaxURLLength+1), false},
		{"URL at max length", strings.Repeat("a", MaxURLLength), true},
		{"data URL too long", "data:text/plain;base64," + strings.Repeat("A", MaxDataURILength+1), false},
		{"data URL with invalid char", "data:text/plain;base64,\x00invalid", false},

		// Path traversal
		{"path traversal /../", "/../etc/passwd", false},
		{"path traversal ././", "././etc/passwd", false},
		{"protocol relative dangerous", "//javascript:alert(1)", false},
		{"protocol relative vbscript", "//vbscript:alert(1)", false},
		{"protocol relative file", "//file:///etc/passwd", false},

		// Dangerous schemes are now rejected (defense-in-depth: these reach
		// IsValidURL via the non-sanitizing ExtractAllLinks path and the
		// raw-HTML media scan, which bypass the DOM sanitizer).
		{"javascript URL", "javascript:alert(1)", false},
		{"vbscript URL", "vbscript:msgbox(1)", false},
		{"file URL", "file:///etc/passwd", false},
		// Disguised dangerous schemes — mirrors the sanitizer's bypass cases.
		{"javascript with leading C0", "\x01javascript:alert(1)", false},
		{"javascript with leading space", " javascript:alert(1)", false},
		{"javascript split by tab", "java\tscript:alert(1)", false},
		{"javascript uppercase", "JaVaScRiPt:alert(1)", false},
		{"javascript disguised by .mp4 suffix", "javascript:alert(1).mp4", false},
		// Schemes that merely contain the substring are NOT rejected.
		{"https path containing javascript", "https://example.com/javascript-tutorial", true},
		{"relative file named javascript", "javascript", true},

		{"anchor only (not accepted)", "#section", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidURL(tt.url)
			if result != tt.expected {
				t.Errorf("IsValidURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestIsValidURLDataURIs(t *testing.T) {
	t.Parallel()

	t.Run("valid data URIs", func(t *testing.T) {
		validDataURIs := []string{
			"data:text/plain,Hello",
			"data:image/png;base64,iVBORw0KGgo=",
		}

		for _, uri := range validDataURIs {
			if !IsValidURL(uri) {
				t.Errorf("IsValidURL(%q) should be true", uri)
			}
		}
	})

	t.Run("invalid data URIs", func(t *testing.T) {
		invalidDataURIs := []string{
			"data:text/html,<script>alert(1)</script>", // contains < and >
			"data:text/plain,\x01",                     // control character
			"data:text/html,<h1>Hello</h1>",            // contains < and >
			"data:image/svg+xml,<svg></svg>",           // contains < and >
		}

		for _, uri := range invalidDataURIs {
			if IsValidURL(uri) {
				t.Errorf("IsValidURL(%q) should be false", uri)
			}
		}
	})
}

func BenchmarkIsValidURL(b *testing.B) {
	tests := []struct {
		name string
		url  string
	}{
		{"simple path", "/path/to/file.html"},
		{"https URL", "https://example.com/path/to/resource"},
		{"data URI", "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB"},
		{"long URL", "https://example.com/" + strings.Repeat("path/", 50)},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				IsValidURL(tt.url)
			}
		})
	}
}

func TestNormalizeNonBreakingSpaces(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no nbsp passthrough", "hello world", "hello world"},
		{"single nbsp", "a b", "a b"},
		{"multiple nbsp", "a  b", "a  b"},
		{"leading and trailing nbsp", " hi ", " hi "},
		{"only nbsp", " ", " "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeNonBreakingSpaces(tt.in); got != tt.want {
				t.Errorf("normalizeNonBreakingSpaces(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestGetIndentLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		padding int
		want    int
	}{
		{"zero", 0, 0},
		{"boundary 18 stays level 0", 18, 0},
		{"boundary 19 promotes to level 1", 19, 1},
		{"boundary 40 stays level 1", 40, 1},
		{"boundary 41 promotes to level 2", 41, 2},
		{"boundary 80 stays level 2", 80, 2},
		{"boundary 81 promotes to level 3", 81, 3},
		{"deep indent caps at level 3", 200, 3},
		{"negative clamps to level 0", -5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := getIndentLevel(tt.padding); got != tt.want {
				t.Errorf("getIndentLevel(%d) = %d, want %d", tt.padding, got, tt.want)
			}
		})
	}
}

func TestGetListPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		padding int
		want    string
	}{
		{"root level no prefix", 0, ""},
		{"level 1 two-space indent", 19, "  - "},
		{"level 2 four-space indent", 41, "    - "},
		{"level 3 six-space indent", 81, "      - "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := getListPrefix(tt.padding); got != tt.want {
				t.Errorf("getListPrefix(%d) = %q, want %q", tt.padding, got, tt.want)
			}
		})
	}
}

func TestReplaceEntityAt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		text         string
		pos          int
		wantRepl     string
		wantConsumed int
	}{
		// Boundary / error arms
		{"pos out of range", "abc", 3, "&", 1},
		{"non-ampersand at pos", "abc", 0, "&", 1},
		{"lone trailing ampersand", "a&", 1, "&", 1},
		{"ampersand alone at end", "&", 0, "&", 1},
		{"named entity without semicolon", "&abc", 0, "&", 1},
		// Named-entity switch cases
		{"amp entity", "&amp;", 0, "&", 5},
		{"nbsp entity", "&nbsp;", 0, " ", 6},
		{"lt entity", "&lt;", 0, "<", 4},
		{"gt entity", "&gt;", 0, ">", 4},
		{"quot entity", "&quot;", 0, "\"", 6},
		{"apos entity", "&apos;", 0, "'", 6},
		{"copy entity", "&copy;", 0, "©", 6},
		{"reg entity", "&reg;", 0, "®", 5},
		{"mdash entity", "&mdash;", 0, "—", 7},
		{"ndash entity", "&ndash;", 0, "–", 7},
		// Numeric entities
		{"decimal numeric entity", "&#65;", 0, "A", 5},
		{"hex numeric entity", "&#x41;", 0, "A", 6},
		// Mid-string entity (pos != 0)
		{"mid-string amp entity", "x&amp;y", 1, "&", 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repl, consumed := replaceEntityAt(tt.text, tt.pos)
			if repl != tt.wantRepl || consumed != tt.wantConsumed {
				t.Errorf("replaceEntityAt(%q, %d) = (%q, %d), want (%q, %d)",
					tt.text, tt.pos, repl, consumed, tt.wantRepl, tt.wantConsumed)
			}
		})
	}
}

// TestReplaceNumericEntity covers decimal/hex decoding, the NBSP special case,
// surrogate pairs, out-of-range code points, and every rejection path (no '#',
// no semicolon, empty entity, oversized entity, invalid hex/decimal digits,
// empty hex payload, truncated input).
func TestReplaceNumericEntity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		text         string
		start        int
		wantRepl     string
		wantConsumed int
	}{
		{"decimal A", "&#65;", 0, "A", 5},
		{"hex A lowercase", "&#x41;", 0, "A", 6},
		{"hex A uppercase X", "&#X41;", 0, "A", 6},
		{"nbsp becomes space", "&#160;", 0, " ", 6},
		{"tab", "&#9;", 0, "\t", 4},
		{"surrogate -> REPLACEMENT", "&#xD800;", 0, "�", 8},
		{"out of range preserved as-is", "&#1114112;", 0, "&#1114112;", 10},
		{"empty entity preserved", "&#;", 0, "&#;", 3},
		{"hex with no digits preserved", "&#x;", 0, "&#x;", 4},
		{"oversized entity preserved", "&#12345678901234;", 0, "&#12345678901234;", 17},
		{"invalid hex digit preserved", "&#xZZ;", 0, "&#xZZ;", 6},
		{"invalid decimal digit preserved", "&#12a;", 0, "&#12a;", 6},
		{"no semicolon", "&#65", 0, "&", 1},
		{"not a numeric entity", "&amp;", 0, "&", 1},
		{"lone ampersand", "&", 0, "&", 1},
		{"truncated after hash", "&#", 0, "&", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repl, consumed := replaceNumericEntity(tt.text, tt.start)
			if repl != tt.wantRepl || consumed != tt.wantConsumed {
				t.Errorf("replaceNumericEntity(%q, %d) = (%q, %d), want (%q, %d)",
					tt.text, tt.start, repl, consumed, tt.wantRepl, tt.wantConsumed)
			}
		})
	}
}

// TestIsValidEntityName covers empty, alphanumeric-valid, and several invalid
// character cases (separator, dash, non-ASCII).
func TestIsValidEntityName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"lowercase", "amp", true},
		{"uppercase", "AMP", true},
		{"alphanumeric", "amp123", true},
		{"digits only", "123", true},
		{"semicolon invalid", "amp;", false},
		{"dash invalid", "amp-lt", false},
		{"non-ascii invalid", "café", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isValidEntityName(tt.in); got != tt.want {
				t.Errorf("isValidEntityName(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

// TestExtractPaddingLeft covers nil/non-element rejection, missing/empty style,
// no padding-left rule, and integer parsing with truncation.
func TestExtractPaddingLeft(t *testing.T) {
	t.Parallel()

	styled := func(style string) *html.Node {
		return &html.Node{
			Type: html.ElementNode,
			Data: "li",
			Attr: []html.Attribute{{Key: "style", Val: style}},
		}
	}

	tests := []struct {
		name string
		node *html.Node
		want int
	}{
		{"nil node", nil, 0},
		{"text node", &html.Node{Type: html.TextNode, Data: "x"}, 0},
		{"element no attrs", &html.Node{Type: html.ElementNode, Data: "p"}, 0},
		{"empty style", styled(""), 0},
		{"style without padding-left", styled("color: red"), 0},
		{"padding-left 40pt", styled("padding-left: 40pt"), 40},
		{"padding-left truncates decimal", styled("padding-left: 18.9pt"), 18},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := extractPaddingLeft(tt.node); got != tt.want {
				t.Errorf("extractPaddingLeft() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestNormalizeText covers the empty fast path, the no-modification fast path,
// the ampersand-only delegation path, and the combined NBSP/newline/entity path.
func TestNormalizeText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no modification", "hello world", "hello world"},
		{"newline to space", "line1\nline2", "line1 line2"},
		{"carriage return dropped", "a\rb", "ab"},
		{"cr lf pair", "a\r\nb", "a b"},
		{"nbsp to space", "a b", "a b"},
		{"ampersand only path", "a&amp;b", "a&b"},
		{"combined entity and newline", "x&amp;\ny", "x& y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeText(tt.in); got != tt.want {
				t.Errorf("normalizeText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestIsValidURL_HTTPSchemeCaseInsensitive verifies IsValidURL accepts
// uppercase/mixed-case http(s) schemes (RFC 3986 §3.1).
func TestIsValidURL_HTTPSchemeCaseInsensitive(t *testing.T) {
	t.Parallel()
	valid := []string{
		"http://example.com",
		"HTTP://example.com",
		"Https://example.com/path",
		"HTTPS://example.com",
	}
	for _, u := range valid {
		if !IsValidURL(u) {
			t.Errorf("IsValidURL(%q) = false, want true (scheme is case-insensitive)", u)
		}
	}
}
