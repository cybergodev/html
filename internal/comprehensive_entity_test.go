package internal

import (
	"fmt"
	"strings"
	"testing"

	stdxhtml "golang.org/x/net/html"

	"github.com/cybergodev/html/internal/table"
)

// TestComprehensiveHTMLEntityConversion is a comprehensive test that verifies
// all HTML entities are converted to their actual characters correctly.
func TestComprehensiveHTMLEntityConversion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		html           string
		mustContain    []string
		mustNotContain []string
		explanation    string
	}{
		// Non-breaking spaces - all forms should convert to regular space (U+0020)
		{
			name:           "nbsp_named_entity",
			html:           "A&nbsp;B",
			mustContain:    []string{"A B"},
			mustNotContain: []string{"\u00a0", "&nbsp;"},
			explanation:    "&nbsp; should convert to regular space (0x20), not non-breaking space (0xa0)",
		},
		{
			name:           "nbsp_decimal_numeric",
			html:           "A&#160;B",
			mustContain:    []string{"A B"},
			mustNotContain: []string{"\u00a0", "&#160;"},
			explanation:    "&#160; should convert to regular space",
		},
		{
			name:           "nbsp_hexadecimal_numeric",
			html:           "A&#xa0;B",
			mustContain:    []string{"A B"},
			mustNotContain: []string{"\u00a0", "&#xa0;"},
			explanation:    "&#xa0; should convert to regular space",
		},
		{
			name:           "nbsp_uppercase_hex",
			html:           "A&#xA0;B",
			mustContain:    []string{"A B"},
			mustNotContain: []string{"\u00a0"},
			explanation:    "&#xA0; (uppercase) should convert to regular space",
		},

		// Basic XML entities
		{
			name:           "basic_xml_entities",
			html:           "&amp;&lt;&gt;&quot;&apos;",
			mustContain:    []string{"&", "<", ">", "\"", "'"},
			mustNotContain: []string{"&amp;", "&lt;", "&gt;", "&quot;", "&apos;"},
			explanation:    "Basic XML entities should convert to their characters",
		},

		// Typographic entities
		{
			name:           "typographic_dashes",
			html:           "&mdash;&ndash;",
			mustContain:    []string{"—", "–"},
			mustNotContain: []string{"&mdash;", "&ndash;"},
			explanation:    "Em dash and en dash should convert correctly",
		},
		{
			name:           "typographic_quotes",
			html:           "&lsquo;&rsquo;&ldquo;&rdquo;",
			mustContain:    []string{"\u2018", "\u2019", "\u201c", "\u201d"}, // Unicode smart quotes
			mustNotContain: []string{"&lsquo;", "&rsquo;", "&ldquo;", "&rdquo;"},
			explanation:    "Smart quotes should convert to their Unicode characters (U+2018, U+2019, U+201C, U+201D)",
		},
		{
			name:           "ellipsis",
			html:           "&hellip;",
			mustContain:    []string{"…"},
			mustNotContain: []string{"&hellip;"},
			explanation:    "Ellipsis should convert to …",
		},

		// Copyright and trademarks
		{
			name:           "copyright_trademarks",
			html:           "&copy;&reg;&trade;",
			mustContain:    []string{"©", "®", "™"},
			mustNotContain: []string{"&copy;", "&reg;", "&trade;"},
			explanation:    "Copyright and trademark symbols should convert",
		},

		// Currency symbols
		{
			name:           "currency_symbols",
			html:           "&euro;&pound;&yen;&cent;&curren;",
			mustContain:    []string{"€", "£", "¥", "¢", "¤"},
			mustNotContain: []string{"&euro;", "&pound;", "&yen;", "&cent;", "&curren;"},
			explanation:    "Currency symbols should convert",
		},

		// Mathematical symbols
		{
			name:           "math_symbols",
			html:           "&plusmn;&times;&divide;&deg;&frac12;&frac14;&frac34;",
			mustContain:    []string{"±", "×", "÷", "°", "½", "¼", "¾"},
			mustNotContain: []string{"&plusmn;", "&times;", "&divide;", "&deg;", "&frac12;", "&frac14;", "&frac34;"},
			explanation:    "Mathematical symbols should convert",
		},

		// Other common symbols
		{
			name:           "other_symbols",
			html:           "&sect;&para;&middot;&bull;&micro;&prime;&Prime;",
			mustContain:    []string{"§", "¶", "·", "•", "µ", "′", "″"},
			mustNotContain: []string{"&sect;", "&para;", "&middot;", "&bull;", "&micro;", "&prime;", "&Prime;"},
			explanation:    "Other common symbols should convert (prime/prime are mathematical symbols, not quotes)",
		},

		// Numeric entities - decimal
		{
			name:           "numeric_decimal_entities",
			html:           "&#169;&#8364;&#162;&#163;&#165;",
			mustContain:    []string{"©", "€", "¢", "£", "¥"},
			mustNotContain: []string{"&#169;", "&#8364;", "&#162;", "&#163;", "&#165;"},
			explanation:    "Decimal numeric entities should convert",
		},

		// Numeric entities - hexadecimal
		{
			name:           "numeric_hex_entities",
			html:           "&#xa9;&#x20ac;&#xa2;&#xa3;&#xa5;",
			mustContain:    []string{"©", "€", "¢", "£", "¥"},
			mustNotContain: []string{"&#xa9;", "&#x20ac;", "&#xa2;", "&#xa3;", "&#xa5;"},
			explanation:    "Hexadecimal numeric entities should convert",
		},

		// Mixed case hexadecimal
		{
			name:           "mixed_case_hex",
			html:           "&#xA9;&#x20AC;&#xA2;&#xA3;&#xA5;",
			mustContain:    []string{"©", "€", "¢", "£", "¥"},
			mustNotContain: []string{"&#xA9;"},
			explanation:    "Uppercase hexadecimal entities should work",
		},

		// Special characters that might cause issues
		{
			name:           "soft_hyphen",
			html:           "A&shy;B",
			mustContain:    []string{"A\u00adB"}, // Soft hyphen is preserved as U+00AD
			mustNotContain: []string{"&shy;"},
			explanation:    "Soft hyphen is converted to U+00AD (soft hyphen character)",
		},

		// Edge cases - multiple entities
		{
			name:           "multiple_entities_sequence",
			html:           "&nbsp;&amp;&nbsp;&lt;&nbsp;&gt;",
			mustContain:    []string{"& < >"}, // Leading nbsp is trimmed by TrimSpace
			mustNotContain: []string{"\u00a0", "&nbsp;", "&amp;", "&lt;", "&gt;"},
			explanation:    "Multiple entities convert, but GetTextContent trims leading/trailing whitespace",
		},

		// Superscripts and subscripts
		{
			name:           "superscripts_subscripts",
			html:           "&sup1;&sup2;&sup3;&frac12;",
			mustContain:    []string{"¹", "²", "³", "½"},
			mustNotContain: []string{"&sup1;", "&sup2;", "&sup3;", "&frac12;"},
			explanation:    "Superscript numbers should convert",
		},

		// Dagger symbols
		{
			name:           "dagger_symbols",
			html:           "&dagger;&Dagger;&permil;",
			mustContain:    []string{"†", "‡", "‰"},
			mustNotContain: []string{"&dagger;", "&Dagger;", "&permil;"},
			explanation:    "Dagger and permil symbols should convert",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse HTML
			doc, err := stdxhtml.Parse(strings.NewReader(tc.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			// Extract text using GetTextContent
			result := GetTextContent(doc)

			// Check for required characters
			for _, mustHave := range tc.mustContain {
				if !strings.Contains(result, mustHave) {
					t.Errorf("Expected output to contain %q (% x), got %q (% x)\nExplanation: %s",
						mustHave, mustHave, result, result, tc.explanation)
				}
			}

			// Check for strings that must NOT appear
			for _, mustNotHave := range tc.mustNotContain {
				if strings.Contains(result, mustNotHave) {
					t.Errorf("Output should NOT contain %q, got %q\nExplanation: %s",
						mustNotHave, result, tc.explanation)
				}
			}

			// Special check: no non-breaking spaces should ever remain
			if strings.Contains(result, "\u00a0") {
				t.Errorf("Output contains non-breaking space (U+00A0 / % x): %q\nExplanation: %s",
					"\u00a0", result, tc.explanation)
			}

			// Verify byte representation for spaces
			if strings.Contains(tc.html, "&nbsp;") ||
				strings.Contains(tc.html, "&#160;") ||
				strings.Contains(tc.html, "&#xa0;") ||
				strings.Contains(tc.html, "&#xA0;") {
				// Should have regular space (0x20), not non-breaking space (0xc2 0xa0)
				if strings.Contains(result, "\u00a0") {
					t.Errorf("Non-breaking space detected in bytes: % x", result)
				}
				// Should have regular space
				if !strings.Contains(result, " ") {
					t.Errorf("Expected regular space (0x20) not found in: %q", result)
				}
			}
		})
	}
}

// TestHTMLEntityByteRepresentation verifies the byte-level representation
// of converted entities to ensure they're correct UTF-8.
func TestHTMLEntityByteRepresentation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		entity   string
		expected string // The expected character
	}{
		{"nbsp_to_space", "A&nbsp;B", "A B"},
		{"nbsp_160_to_space", "A&#160;B", "A B"},
		{"nbsp_a0_to_space", "A&#xa0;B", "A B"},
		{"amp", "A&amp;B", "A&B"},
		{"lt", "A&lt;B", "A<B"},
		{"gt", "A&gt;B", "A>B"},
		{"quot", "&quot;test&quot;", "\"test\""},
		{"apos", "It&apos;s", "It's"},
		{"copy", "&copy;2025", "©2025"},
		{"euro", "&euro;100", "€100"},
		{"mdash", "Hello&mdash;World", "Hello—World"},
		{"hellip", "Wait&hellip;", "Wait…"},

		// Cases merged from the former TestGetTextContentHTMLEntities: these assert
		// exact equality (and therefore correct UTF-8 bytes) for multi-entity and
		// paragraph inputs that substring-based checks cannot pin down. GetTextContent
		// trims leading/trailing whitespace, so the expected values are trimmed.
		{"multiple_nbsp", "A&nbsp;&nbsp;B", "A  B"},
		{"mixed_entities", "&nbsp;&copy;&nbsp;&euro;&nbsp;", "© €"},
		{"numeric_nbsp", "A&#160;B&#xa0;C", "A B C"},
		{"complex_paragraph", "<p>&nbsp;&copy; 2025&nbsp;&mdash;&nbsp;All rights reserved&nbsp;</p>", "© 2025 — All rights reserved"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := stdxhtml.Parse(strings.NewReader(tc.entity))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			result := GetTextContent(doc)

			if result != tc.expected {
				t.Errorf("Entity %q converted to %q (% x), expected %q (% x)",
					tc.entity, result, result, tc.expected, tc.expected)
			}

			// Verify bytes are correct UTF-8
			expectedBytes := []byte(tc.expected)
			resultBytes := []byte(result)

			if len(expectedBytes) != len(resultBytes) {
				t.Errorf("Byte length mismatch: got %d bytes (% x), expected %d bytes (% x)",
					len(resultBytes), resultBytes, len(expectedBytes), expectedBytes)
			}

			for i := 0; i < len(resultBytes) && i < len(expectedBytes); i++ {
				if resultBytes[i] != expectedBytes[i] {
					t.Errorf("Byte mismatch at position %d: got 0x%02x, expected 0x%02x",
						i, resultBytes[i], expectedBytes[i])
				}
			}
		})
	}
}

// TestExtractTextWithStructureHTMLEntities tests the extractTextWithStructureOptimized
// function for HTML entity conversion.
func TestExtractTextWithStructureHTMLEntities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		html        string
		contains    []string // Strings that should be in the result
		notContains []string // Strings that should NOT be in the result
	}{
		{
			name:        "paragraph_with_nbsp",
			html:        `<p>Hello&nbsp;World</p>`,
			contains:    []string{"Hello World"},
			notContains: []string{"\u00a0", "&nbsp;"},
		},
		{
			name:        "div_with_entities",
			html:        `<div>&copy; 2025 &mdash; Test</div>`,
			contains:    []string{"©", "2025", "—", "Test"},
			notContains: []string{"&copy;", "&mdash;"},
		},
		{
			name:        "multiple_paragraphs",
			html:        `<p>First&nbsp;para</p><p>Second&nbsp;para</p>`,
			contains:    []string{"First para", "Second para"},
			notContains: []string{"\u00a0"},
		},
		{
			name:        "table_with_nbsp",
			html:        `<table><tr><td>A&nbsp;B</td><td>&copy;</td></tr></table>`,
			contains:    []string{"A B", "©"},
			notContains: []string{"\u00a0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := stdxhtml.Parse(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			tb := table.NewTrackedBuilder()
			ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
			result := CleanText(tb.String())

			for _, mustContain := range tt.contains {
				if !strings.Contains(result, mustContain) {
					t.Errorf("Expected result to contain %q, got %q", mustContain, result)
				}
			}

			for _, mustNotContain := range tt.notContains {
				if strings.Contains(result, mustNotContain) {
					t.Errorf("Result should NOT contain %q, got %q", mustNotContain, result)
				}
			}

			// Always check for non-breaking spaces
			if strings.Contains(result, "\u00a0") {
				t.Errorf("Result contains non-breaking space (U+00A0): %q", result)
			}
		})
	}
}

// BenchmarkHTMLEntityConversion benchmarks the HTML entity conversion performance.
func BenchmarkHTMLEntityConversion(b *testing.B) {
	testCases := []struct {
		name string
		html string
	}{
		{"simple_nbsp", "Hello&nbsp;World"},
		{"multiple_entities", "&copy;&euro;&pound;&yen;&reg;&trade;"},
		{"numeric_entities", "&#169;&#8364;&#163;&#165;&#174;&#8482;"},
		{"mixed_complex", "A&nbsp;&copy;&nbsp;2025&nbsp;&mdash;&nbsp;Test&nbsp;&euro;100"},
	}

	for _, bc := range testCases {
		b.Run(bc.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result := ReplaceHTMLEntities(bc.html)
				if result == "" {
					b.Fatal("ReplaceHTMLEntities returned empty string")
				}
			}
		})
	}
}

// ExampleGetTextContent demonstrates the GetTextContent function with HTML entities.
func ExampleGetTextContent() {
	html := `<p>&nbsp;&copy; 2025 &mdash; All rights reserved&nbsp;</p>`
	doc, _ := stdxhtml.Parse(strings.NewReader(html))
	result := GetTextContent(doc)
	fmt.Println(result)
	// Output:  © 2025 — All rights reserved
}

// ExampleReplaceHTMLEntities demonstrates the ReplaceHTMLEntities function.
func ExampleReplaceHTMLEntities() {
	input := "&nbsp;&copy; 2025 &mdash; Test &euro;100"
	result := ReplaceHTMLEntities(input)
	fmt.Println(result)
	// Output:  © 2025 — Test €100
}
