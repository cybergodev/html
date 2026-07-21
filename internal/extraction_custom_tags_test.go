package internal

import (
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/cybergodev/html/internal/table"
)

// TestSECDocumentStructure tests that SEC documents with custom tags
// are properly formatted with appropriate paragraph spacing.
func TestSECDocumentStructure(t *testing.T) {
	// Simplified SEC document structure
	htmlContent := `<SEC-DOCUMENT>0002022111-26-000002.txt : 20260130
<SEC-HEADER>0002022111-26-000002.hdr.sgml : 20260130
<ACCEPTANCE-DATETIME>20260130180232
ACCESSION NUMBER:		0002022111-26-000002
CONFORMED SUBMISSION TYPE:	4
PUBLIC DOCUMENT COUNT:		1
</SEC-HEADER>
<DOCUMENT>
<TYPE>4
<SEQUENCE>1
<FILENAME>wk-form4_1769814146.xml
<DESCRIPTION>FORM 4
<TEXT>
<ownershipDocument>
    <schemaVersion>X0508</schemaVersion>
    <documentType>4</documentType>
    <periodOfReport>2026-01-29</periodOfReport>
    <issuer>
        <issuerCik>0001463101</issuerCik>
        <issuerName>Enphase Energy, Inc.</issuerName>
        <issuerTradingSymbol>ENPH</issuerTradingSymbol>
    </issuer>
</ownershipDocument>
</TEXT>
</DOCUMENT>
</SEC-DOCUMENT>`

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	tb := table.NewTrackedBuilder()
	ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
	result := tb.String()

	// Verify that custom SEC tags result in proper spacing
	lines := strings.Split(result, "\n")

	// Count consecutive newlines (paragraph spacing)
	paragraphCount := 0
	for i := 0; i < len(lines)-1; i++ {
		if strings.TrimSpace(lines[i]) == "" && strings.TrimSpace(lines[i+1]) == "" {
			paragraphCount++
		}
	}

	// We expect multiple paragraphs due to block-level custom tags
	// Each major SEC tag should create paragraph separation
	if paragraphCount < 3 {
		t.Logf("Result:\n%s", result)
		t.Errorf("Expected at least 3 paragraph separations, got %d", paragraphCount)
		t.Logf("This suggests custom tags are not being treated as block elements")
	}

	// Verify that key content is preserved
	expectedContent := []string{
		"0002022111-26-000002",
		"4",
		"2026-01-29",
		"Enphase Energy, Inc.",
	}

	for _, content := range expectedContent {
		if !strings.Contains(result, content) {
			t.Errorf("Expected to find content %q in result", content)
		}
	}
}

// TestCustomTagFormatting tests that various custom tag patterns
// result in proper paragraph formatting.
func TestCustomTagFormatting(t *testing.T) {
	tests := []struct {
		name               string
		html               string
		minParagraphs      int // Minimum expected paragraph separations
		contentShouldExist []string
	}{
		{
			name:               "SEC-DOCUMENT root element",
			html:               `<SEC-DOCUMENT>content here</SEC-DOCUMENT>`,
			minParagraphs:      1,
			contentShouldExist: []string{"content here"},
		},
		{
			name:               "SEC-HEADER with children",
			html:               `<SEC-HEADER><TYPE>4</TYPE><SEQUENCE>1</SEQUENCE></SEC-HEADER>`,
			minParagraphs:      1,
			contentShouldExist: []string{"4", "1"},
		},
		{
			name:               "Container with multiple children",
			html:               `<CUSTOM-TAG><child1>text1</child1><child2>text2</child2></CUSTOM-TAG>`,
			minParagraphs:      1,
			contentShouldExist: []string{"text1", "text2"},
		},
		{
			name:               "Tag with long text content",
			html:               `<DESCRIPTION>This is a very long description that should cause the tag to be treated as a block element because it contains substantial text content</DESCRIPTION>`,
			minParagraphs:      1,
			contentShouldExist: []string{"long description"},
		},
		{
			name:               "Tag with multiline text",
			html:               "<ADDRESS>\nLine 1\nLine 2\nLine 3\n</ADDRESS>",
			minParagraphs:      1,
			contentShouldExist: []string{"Line 1", "Line 2", "Line 3"},
		},
		{
			name:               "Uppercase tag with hyphens",
			html:               `<ACCEPTANCE-DATETIME>20260130180232</ACCEPTANCE-DATETIME>`,
			minParagraphs:      1,
			contentShouldExist: []string{"20260130180232"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			tb := table.NewTrackedBuilder()
			ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
			result := tb.String()

			// Count paragraph separations (double newlines)
			lines := strings.Split(result, "\n")
			paragraphCount := 0
			for i := 0; i < len(lines)-1; i++ {
				if strings.TrimSpace(lines[i]) == "" && strings.TrimSpace(lines[i+1]) == "" {
					paragraphCount++
				}
			}

			if paragraphCount < tt.minParagraphs {
				t.Logf("Result:\n%s", result)
				t.Errorf("Expected at least %d paragraph separations, got %d", tt.minParagraphs, paragraphCount)
				t.Logf("This suggests custom tags are not being treated as block elements")
			}

			// Verify expected content exists
			for _, content := range tt.contentShouldExist {
				if !strings.Contains(result, content) {
					t.Errorf("Expected to find content %q in result", content)
				}
			}
		})
	}
}

// BenchmarkSECDocumentExtraction benchmarks extraction of SEC documents.
func BenchmarkSECDocumentExtraction(b *testing.B) {
	htmlContent := `<SEC-DOCUMENT>0002022111-26-000002.txt : 20260130
<SEC-HEADER>0002022111-26-000002.hdr.sgml : 20260130
<ACCEPTANCE-DATETIME>20260130180232
ACCESSION NUMBER:		0002022111-26-000002
CONFORMED SUBMISSION TYPE:	4
PUBLIC DOCUMENT COUNT:		1
</SEC-HEADER>
<DOCUMENT>
<TYPE>4
<SEQUENCE>1
<FILENAME>wk-form4_1769814146.xml
<DESCRIPTION>FORM 4
<TEXT>
<ownershipDocument>
    <schemaVersion>X0508</schemaVersion>
    <documentType>4</documentType>
    <periodOfReport>2026-01-29</periodOfReport>
    <issuer>
        <issuerCik>0001463101</issuerCik>
        <issuerName>Enphase Energy, Inc.</issuerName>
        <issuerTradingSymbol>ENPH</issuerTradingSymbol>
    </issuer>
</ownershipDocument>
</TEXT>
</DOCUMENT>
</SEC-DOCUMENT>`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc, err := html.Parse(strings.NewReader(htmlContent))
		if err != nil {
			b.Fatalf("Failed to parse HTML: %v", err)
		}

		tb := table.NewTrackedBuilder()
		ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
	}
}
