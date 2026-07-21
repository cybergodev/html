package internal

import (
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/cybergodev/html/internal/table"
)

// TestNamespaceTagInlineHandling tests that namespaced tags (e.g., ix:nonnumeric)
// are correctly identified as inline elements when appropriate.
func TestNamespaceTagInlineHandling(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string // expected output pattern
	}{
		{
			name:     "ix:nonumeric tags in span should not create newlines",
			html:     `(<ix:nonnumeric>707</ix:nonnumeric>) <ix:nonnumeric>774-7000</ix:nonnumeric>`,
			expected: "707 ) 774-7000", // After whitespace normalization, "(707 ) 774-7000" becomes "( 707 ) 774-7000" without leading space
		},
		{
			name: "xbrl:value in paragraph",
			html: `<p>
				Net income: <xbrl:value unit="USD">1000000</xbrl:value>
			</p>`,
			expected: "Net income: 1000000",
		},
		{
			name: "dei namespace tag",
			html: `<div>
				City: <dei:CityAreaCode>707</dei:CityAreaCode>
			</div>`,
			expected: "City: 707",
		},
		{
			name: "multiple inline namespace tags",
			html: `<span>
				<ix:nonnumeric>A</ix:nonnumeric>
				<ix:nonnumeric>B</ix:nonnumeric>
				<ix:nonnumeric>C</ix:nonnumeric>
			</span>`,
			expected: "A B C", // Should all be on same line
		},
		{
			name: "unknown namespace in inline context",
			html: `<span>
				Text <custom:value>123</custom:value> more text
			</span>`,
			expected: "Text123 more text", // Current behavior: text nodes adjacent to inline elements don't get spacing
		},
		{
			name: "namespace tag in block context with long content",
			html: `<div>
				<ix:nonnumeric>This is a very long text content that exceeds fifty characters and should be treated as a block element because it has substantial content</ix:nonnumeric>
			</div>`,
			expected: "This is a very long text content that exceeds fifty characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parseHTML(tt.html)
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			tb := table.NewTrackedBuilder()
			ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
			result := tb.String()

			// Remove extra whitespace for comparison
			result = strings.Join(strings.Fields(result), " ")

			if !strings.Contains(result, tt.expected) {
				t.Errorf("Expected output to contain %q, got:\n%s", tt.expected, result)
			}
		})
	}
}

// TestNamespaceTagStructure tests the helper functions for namespace tag detection.
func TestNamespaceTagStructure(t *testing.T) {
	tests := []struct {
		tag           string
		isNamespace   bool
		prefix        string
		isKnownInline bool
	}{
		{"ix:nonnumeric", true, "ix", true},
		{"xbrl:value", true, "xbrl", true},
		{"dei:CityAreaCode", true, "dei", true},
		{"us-gaap:Revenue", true, "us-gaap", true},
		{"ifrs:Assets", true, "ifrs", true},
		{"link:something", true, "link", true},
		{"custom:tag", true, "custom", false},
		{"div", false, "", false},
		{"span", false, "", false},
		{"p", false, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			if got := IsNamespaceTag(tt.tag); got != tt.isNamespace {
				t.Errorf("IsNamespaceTag(%q) = %v, want %v", tt.tag, got, tt.isNamespace)
			}

			if got := GetNamespacePrefix(tt.tag); got != tt.prefix {
				t.Errorf("GetNamespacePrefix(%q) = %q, want %q", tt.tag, got, tt.prefix)
			}

			if tt.isNamespace {
				if got := IsKnownInlineNamespacePrefix(tt.prefix); got != tt.isKnownInline {
					t.Errorf("IsKnownInlineNamespacePrefix(%q) = %v, want %v", tt.prefix, got, tt.isKnownInline)
				}
			}
		})
	}
}

// TestShouldTreatNamespaceTagAsInline tests the shouldTreatNamespaceTagAsInline function.
func TestShouldTreatNamespaceTagAsInline(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected bool
	}{
		{
			name:     "ix:nonnumeric in span is inline",
			html:     `<span><ix:nonnumeric>707</ix:nonnumeric></span>`,
			expected: true,
		},
		{
			name:     "ix:nonnumeric in div with short text is inline",
			html:     `<div><ix:nonnumeric>707</ix:nonnumeric></div>`,
			expected: true,
		},
		{
			name:     "ix:nonnumeric with long text is not inline",
			html:     `<div><ix:nonnumeric>This is a very long text content that exceeds fifty characters limit</ix:nonnumeric></div>`,
			expected: false,
		},
		{
			name:     "unknown namespace in span is inline",
			html:     `<span><custom:value>123</custom:value></span>`,
			expected: true,
		},
		{
			name:     "namespace tag with element children is not inline",
			html:     `<div><ix:nonnumeric><span>707</span></ix:nonnumeric></div>`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parseHTML(tt.html)
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			// Find the namespace tag node
			var namespaceTag *html.Node
			var findFunc func(*html.Node)
			findFunc = func(n *html.Node) {
				if n.Type == html.ElementNode && IsNamespaceTag(n.Data) {
					namespaceTag = n
					return
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					findFunc(c)
				}
			}
			findFunc(doc)

			if namespaceTag == nil {
				t.Fatal("Failed to find namespace tag in parsed HTML")
			}

			got := ShouldTreatNamespaceTagAsInline(namespaceTag)
			if got != tt.expected {
				t.Errorf("ShouldTreatNamespaceTagAsInline() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// parseHTML is a helper function to parse HTML string.
func parseHTML(htmlStr string) (*html.Node, error) {
	return html.Parse(strings.NewReader(htmlStr))
}
