package internal

import (
	"strings"
	"testing"

	"github.com/cybergodev/html/internal/table"
	"golang.org/x/net/html"
)

func TestExtractTextWithStructure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "simple paragraph",
			html: `<p>Hello World</p>`,
			want: "Hello World",
		},
		{
			name: "nested elements",
			html: `<div><p>First</p><p>Second</p></div>`,
			want: "First\n\nSecond", // Paragraphs separated by double newlines
		},
		{
			name: "block elements add newlines",
			html: `<div>Text1</div><div>Text2</div>`,
			want: "Text1\n\nText2", // Divs separated by double newlines
		},
		{
			name: "inline elements add spaces",
			html: `<p>Hello <strong>World</strong> Test</p>`,
			want: "Hello World Test",
		},
		{
			name: "script tags excluded",
			html: `<div>Visible<script>hidden</script></div>`,
			want: "Visible",
		},
		{
			name: "style tags excluded",
			html: `<div>Visible<style>body{}</style></div>`,
			want: "Visible",
		},
		{
			name: "nav tags excluded",
			html: `<div>Content<nav>Menu</nav></div>`,
			want: "Content",
		},
		{
			name: "empty",
			html: `<div></div>`,
			want: "",
		},
		{
			name: "whitespace only",
			html: `<p>   </p>`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			tb := table.NewTrackedBuilder()
			ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
			result := strings.TrimSpace(tb.String())

			if result != tt.want {
				t.Errorf("ExtractTextWithStructureAndImages() = %q, want %q", result, tt.want)
			}
		})
	}
}

// TestExtractListMarkers verifies that <li> elements render with proper Markdown
// list markers derived from DOM structure. HTML lists (e.g. WordPress
// wp-block-list) rely on browser default styling and carry no inline
// padding-left, so markers must come from the <ul>/<ol> ancestry rather than
// CSS padding — otherwise consecutive items collapse into one paragraph.
func TestExtractListMarkers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "unordered list markers",
			html: `<ul class="wp-block-list"><li>季度收入</li><li>数据中心</li><li>全年收入</li></ul>`,
			want: "- 季度收入\n- 数据中心\n- 全年收入",
		},
		{
			name: "ordered list markers",
			html: `<ol><li>第一项</li><li>第二项</li></ol>`,
			want: "1. 第一项\n2. 第二项",
		},
		{
			name: "nested unordered list indentation",
			html: `<ul><li>顶层A<ul><li>嵌套1</li><li>嵌套2</li></ul></li><li>顶层B</li></ul>`,
			want: "- 顶层A\n  - 嵌套1\n  - 嵌套2\n\n- 顶层B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			tb := table.NewTrackedBuilder()
			ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
			result := strings.TrimSpace(tb.String())

			if result != tt.want {
				t.Errorf("got %q, want %q", result, tt.want)
			}
		})
	}
}

// TestNestedTablesNotFlattened verifies that a "layout" table — a <table> used
// only to wrap another <table> inside a <td> for visual layout, as commonly
// seen on financial sites such as Finviz — does not flatten the inner data
// table into run-on text. The inner table's rows must survive as separate
// Markdown rows. See containsNestedTable and the table dispatch in
// extractTextWithStructure.
func TestNestedTablesNotFlattened(t *testing.T) {
	t.Parallel()

	// Outer layout table wraps a real 2-column, 2-row data table in its cell.
	src := `<table><tr><td>` +
		`<table><tr><td>A</td><td>B</td></tr><tr><td>C</td><td>D</td></tr></table>` +
		`</td></tr></table>`

	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tb := table.NewTrackedBuilder()
	ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")

	// ExtractTextWithStructureAndImages does not run CleanText, so column
	// padding introduces runs of spaces. Collapse them for stable assertions.
	out := collapseSpaces(tb.String())

	// Inner data table must render as Markdown rows, not flattened text.
	if !strings.Contains(out, "| A | B |") {
		t.Errorf("expected inner table row '| A | B |', got:\n%s", out)
	}
	if !strings.Contains(out, "| C | D |") {
		t.Errorf("expected inner table row '| C | D |', got:\n%s", out)
	}
	// The pre-fix symptom: GetTextContent concatenated adjacent cells without
	// any separator, producing "AB" / "CD".
	if strings.Contains(out, "AB") || strings.Contains(out, "CD") {
		t.Errorf("inner table cells were flattened/concatenated:\n%s", out)
	}
}

// collapseSpaces collapses runs of spaces/tabs into a single space. It mirrors
// the whitespace compression CleanText applies, stabilizing assertions against
// Markdown table column padding emitted by the table processor.
func collapseSpaces(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' {
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
			continue
		}
		inSpace = false
		b.WriteByte(c)
	}
	return b.String()
}

func TestExtractTextWithStructureAndImages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		html       string
		wantText   string
		wantImages int
	}{
		{
			name:       "single image",
			html:       `<div><img src="test.jpg" alt="Test"></div>`,
			wantText:   "[IMAGE:1]",
			wantImages: 1,
		},
		{
			name:       "multiple images",
			html:       `<div><img src="1.jpg"><img src="2.jpg"></div>`,
			wantText:   "[IMAGE:1]\n[IMAGE:2]",
			wantImages: 2,
		},
		{
			name:       "text with images",
			html:       `<div>Before<img src="test.jpg">After</div>`,
			wantText:   "Before\n[IMAGE:1]\nAfter",
			wantImages: 1,
		},
		{
			name:       "no images",
			html:       `<div>Just text</div>`,
			wantText:   "Just text",
			wantImages: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			tb := table.NewTrackedBuilder()
			imageCounter := 0
			ExtractTextWithStructureAndImages(doc, tb, &imageCounter, nil, "markdown")
			result := strings.TrimSpace(tb.String())

			if result != tt.wantText {
				t.Errorf("ExtractTextWithStructureAndImages() text = %q, want %q", result, tt.wantText)
			}
			if imageCounter != tt.wantImages {
				t.Errorf("ExtractTextWithStructureAndImages() images = %d, want %d", imageCounter, tt.wantImages)
			}
		})
	}
}

func TestExtractTextWithStructureAndImagesNil(t *testing.T) {
	t.Parallel()

	tb := table.NewTrackedBuilder()
	ExtractTextWithStructureAndImages(nil, tb, nil, nil, "markdown")

	if tb.Len() != 0 {
		t.Error("ExtractTextWithStructureAndImages(nil) should not write anything")
	}
}

func TestExtractTable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		html     string
		wantRows int
	}{
		{
			name: "simple table",
			html: `<table>
				<tr><th>Header1</th><th>Header2</th></tr>
				<tr><td>Cell1</td><td>Cell2</td></tr>
			</table>`,
			wantRows: 2,
		},
		{
			name: "table with uneven columns",
			html: `<table>
				<tr><td>A</td><td>B</td><td>C</td></tr>
				<tr><td>D</td><td>E</td></tr>
			</table>`,
			wantRows: 2,
		},
		{
			name:     "empty table",
			html:     `<table></table>`,
			wantRows: 0,
		},
		{
			name: "table with empty cells",
			html: `<table>
				<tr><td>A</td><td></td></tr>
			</table>`,
			wantRows: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			tb := table.NewTrackedBuilder()
			ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
			result := tb.String()

			if tt.wantRows > 0 {
				// Should have separator line
				if !strings.Contains(result, "| --- |") {
					t.Error("Table should have separator line")
				}
			}

			if tt.wantRows == 0 && strings.TrimSpace(result) != "" {
				t.Error("Empty table should produce no output")
			}
		})
	}
}

func TestExtractTableNil(t *testing.T) {
	t.Parallel()

	doc, _ := html.Parse(strings.NewReader("<table></table>"))
	tableNode := FindElementByTag(doc, "table")
	tb := table.NewTrackedBuilder()
	ExtractTextWithStructureAndImages(tableNode, tb, nil, nil, "markdown")

	result := strings.TrimSpace(tb.String())
	if result != "" {
		t.Errorf("ExtractTextWithStructureAndImages(empty table) should not write anything, got %q", result)
	}
}

func TestExtractTextWithStructureDepth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		html  string
		check func(string) bool
	}{
		{
			name:  "deeply nested inline elements",
			html:  `<div><span><span><span>Deep</span></span></span></div>`,
			check: func(s string) bool { return strings.Contains(s, "Deep") },
		},
		{
			name:  "mixed block and inline",
			html:  `<div><p><span>Text1</span></p><p><span>Text2</span></p></div>`,
			check: func(s string) bool { return strings.Contains(s, "Text1") && strings.Contains(s, "Text2") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			tb := table.NewTrackedBuilder()
			ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
			result := tb.String()

			if !tt.check(result) {
				t.Errorf("ExtractTextWithStructure() = %q, failed check", result)
			}
		})
	}
}

func TestCleanContentNode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		html        string
		wantRemoved []string
		wantKept    []string
	}{
		{
			name:        "remove script",
			html:        `<div><p>Keep</p><script>Remove</script></div>`,
			wantRemoved: []string{"script"},
			wantKept:    []string{"Keep"},
		},
		{
			name:        "remove nav",
			html:        `<div><p>Keep</p><nav>Remove</nav></div>`,
			wantRemoved: []string{"nav"},
			wantKept:    []string{"Keep"},
		},
		{
			name:        "remove by class",
			html:        `<div><p>Keep</p><div class="sidebar">Remove</div></div>`,
			wantRemoved: []string{"sidebar"},
			wantKept:    []string{"Keep"},
		},
		{
			name:        "remove hidden",
			html:        `<div><p>Keep</p><div hidden>Remove</div></div>`,
			wantRemoved: []string{"hidden"},
			wantKept:    []string{"Keep"},
		},
		{
			name:     "keep all",
			html:     `<div><p>Keep1</p><p>Keep2</p></div>`,
			wantKept: []string{"Keep1", "Keep2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			cleaned := CleanContentNode(doc)

			text := GetTextContent(cleaned)

			for _, kept := range tt.wantKept {
				if !strings.Contains(text, kept) {
					t.Errorf("CleanContentNode() should keep %q", kept)
				}
			}

			for _, removed := range tt.wantRemoved {
				if strings.Contains(text, removed) {
					t.Errorf("CleanContentNode() should remove %q", removed)
				}
			}
		})
	}
}

func TestCleanContentNodeNil(t *testing.T) {
	t.Parallel()

	result := CleanContentNode(nil)
	if result != nil {
		t.Error("CleanContentNode(nil) should return nil")
	}
}

func BenchmarkExtractTextWithStructure(b *testing.B) {
	htmlContent := `<html><body><article><h1>Title</h1><p>Paragraph 1</p><p>Paragraph 2</p></article></body></html>`
	doc, _ := html.Parse(strings.NewReader(htmlContent))

	b.ResetTimer()
	for b.Loop() {
		tb := table.NewTrackedBuilder()
		ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
	}
}

// TestExtractTableAsHTML tests HTML table format extraction
func TestExtractTableAsHTML(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want []string // Substrings that should be in the output
	}{
		{
			name: "simple table",
			html: `<table><tr><th>Header</th></tr><tr><td>Data</td></tr></table>`,
			want: []string{"<table>", "<th>Header</th>", "<td>Data</td>", "</table>"},
		},
		{
			name: "table with alignment",
			html: `<table><tr><th align="left">Left</th><th align="center">Center</th><th align="right">Right</th></tr></table>`,
			want: []string{"text-align:left", "text-align:center", "text-align:right"},
		},
		{
			name: "table with width",
			html: `<table><tr><th style="width:50%">Header</th></tr></table>`,
			want: []string{"width:50%"},
		},
		{
			name: "table with colspan",
			html: `<table><tr><th colspan="2">Merged</th></tr></table>`,
			want: []string{`colspan="2"`},
		},
		{
			name: "table with rowspan",
			html: `<table><tr><td rowspan="2">Span</td></tr></table>`,
			want: []string{`rowspan="2"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			tb := table.NewTrackedBuilder()
			ExtractTextWithStructureAndImages(doc, tb, nil, nil, "html")

			result := tb.String()
			for _, want := range tt.want {
				if !strings.Contains(result, want) {
					t.Errorf("Output should contain %q, got:\n%s", want, result)
				}
			}
		})
	}
}

// TestWriteInt covers both branches of writeInt: the single-digit fast path
// (WriteByte of '0'+n) and the multi-digit AppendInt path, including a value wide
// enough to exercise the full decimal buffer.
func TestWriteInt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		n    int
		want string
	}{
		{"zero", 0, "0"},
		{"single digit", 9, "9"},
		{"first multi-digit", 10, "10"},
		{"two digits", 99, "99"},
		{"five digits", 12345, "12345"},
		{"ten digits", 1234567890, "1234567890"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := table.NewTrackedBuilder()
			writeInt(tb, tt.n)
			if got := tb.String(); got != tt.want {
				t.Errorf("writeInt(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

// ============================================================================
// Custom / namespace / whitespace tag extraction (merged from
// extraction_custom_tags_test.go, extraction_namespace_test.go and
// extraction_whitespace_test.go)
// ============================================================================

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

// TestNamespaceTagInlineHandling tests that namespaced tags (e.g., ix:nonnumeric)
// are correctly identified as inline elements when appropriate.
func TestNamespaceTagInlineHandling(t *testing.T) {
	// Note: the "(<ix:nonnumeric>707</ix:nonnumeric>) 774-7000" whitespace
	// scenario is pinned exactly by TestWhitespacePreservation, so it is not
	// repeated here as a weaker substring check.
	tests := []struct {
		name     string
		html     string
		expected string // expected output pattern
	}{
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

// TestWhitespacePreservation tests that whitespace is correctly preserved
// when extracting text with inline namespace tags.
//
// Current logic: inline elements add spacing after themselves if there's a next sibling.
// This creates readable output for adjacent text/inline segments.
func TestWhitespacePreservation(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string // exact expected output
	}{
		{
			name: "parentheses with namespace tag - original case",
			html: `(<ix:nonnumeric>707</ix:nonnumeric>) <ix:nonnumeric>774-7000</ix:nonnumeric>`,
			// HTML parser: "(" + ix:nonnumeric("707") + ") " + ix:nonnumeric("774-7000")
			// Trailing space in ") " is preserved
			expected: "(707 ) 774-7000",
		},
		{
			name: "no space after closing parenthesis",
			html: `(<ix:nonnumeric>707</ix:nonnumeric>)<ix:nonnumeric>774-7000</ix:nonnumeric>`,
			// HTML parser: "(" + ix:nonnumeric("707") + ")" + ix:nonnumeric("774-7000")
			// No trailing space in ")"
			expected: "(707 )774-7000",
		},
		{
			name: "colon with space before namespace tag",
			html: `<p>Net income: <xbrl:value unit="USD">1000000</xbrl:value></p>`,
			// HTML parser: "Net income: " + xbrl:value("1000000")
			// The trailing space in "Net income: " is preserved
			expected: "Net income: 1000000",
		},
		{
			name: "namespace tag between words",
			html: `<span>Text<custom:value>123</custom:value>more</span>`,
			// HTML parser: "Text" + custom:value("123") + "more"
			// No spaces in source, but inline element adds spacing
			expected: "Text123 more",
		},
		{
			name: "namespace tag with spaces in source",
			html: `<span>Text <custom:value>123</custom:value> more</span>`,
			// HTML parser: "Text " + custom:value("123") + " more"
			// The trailing space from "Text " is preserved
			// The span element adds spacing after itself
			// Then " more" is processed with leading space trimmed
			expected: "Text123 more",
		},
		{
			// The original SEC-document case: same "(707) 774-7000" scenario
			// as above but with style attributes on every element (merged from
			// the former TestOriginalSECCase).
			name:     "SEC document with styles",
			html:     `<div style="text-align:center"><span style="color:#000000;font-family:'Arial',sans-serif;font-size:9pt;font-weight:700;line-height:120%">(<ix:nonnumeric contextref="c-1" name="dei:CityAreaCode" id="f-13">707</ix:nonnumeric>) <ix:nonnumeric contextref="c-1" name="dei:LocalPhoneNumber" id="f-14">774-7000</ix:nonnumeric></span></div>`,
			expected: "(707 )774-7000",
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
			result := strings.TrimSpace(tb.String())

			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// firstElementByTag returns the first element node with the given tag in
// document order, or nil.
func firstElementByTag(root *html.Node, tag string) *html.Node {
	var found *html.Node
	WalkNodes(root, func(n *html.Node) bool {
		if found == nil && n.Type == html.ElementNode && n.Data == tag {
			found = n
		}
		return found == nil
	})
	return found
}

// TestTableTokenText unit-drives tableTokenText's flattening and counter
// contract at the internal layer (the root package exercises it end-to-end
// through table cells; these rows pin each rule directly): plain text is
// trimmed and space-joined, anchors emit [LINK:n]…[/LINK] while burning a
// link position, only usable images emit [IMAGE:n], raw-text subtrees are
// skipped, and <template> differs by mode exactly as the collector does.
func TestTableTokenText(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		doc        string
		chromeMode bool
		want       string
		wantLinks  int
		wantImages int
	}{
		{
			name:       "plain text flattened and trimmed",
			doc:        `<div>  cell text </div>`,
			want:       "cell text",
			wantLinks:  0,
			wantImages: 0,
		},
		{
			name:       "anchor emits token and burns a position",
			doc:        `<div>go <a href="/x">there</a> now</div>`,
			want:       "go [LINK:1] there[/LINK] now",
			wantLinks:  1,
			wantImages: 0,
		},
		{
			name:       "image with usable src emits token",
			doc:        `<div>pic <img src="/i.png" alt="shot"> end</div>`,
			want:       "pic [IMAGE:1] end",
			wantLinks:  0,
			wantImages: 1,
		},
		{
			name:       "image without usable src burns nothing",
			doc:        `<div>pic <img alt="no src"> end</div>`,
			want:       "pic end",
			wantLinks:  0,
			wantImages: 0,
		},
		{
			name:       "script subtree skipped",
			doc:        `<div>a<script>evil()</script>b</div>`,
			want:       "a b",
			wantLinks:  0,
			wantImages: 0,
		},
		{
			name:       "template included in content mode",
			doc:        `<div>x<template>tpl</template>y</div>`,
			chromeMode: false,
			want:       "x tpl y",
			wantLinks:  0,
			wantImages: 0,
		},
		{
			name:       "template skipped in chrome mode",
			doc:        `<div>x<template>tpl</template>y</div>`,
			chromeMode: true,
			want:       "x y",
			wantLinks:  0,
			wantImages: 0,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc, err := parseHTML(tt.doc)
			if err != nil {
				t.Fatalf("parseHTML: %v", err)
			}
			node := firstElementByTag(doc, "div")
			if node == nil {
				t.Fatal("test fixture error: div not found")
			}
			links, images := 0, 0
			got := tableTokenText(node, &images, &links, tt.chromeMode)
			if got != tt.want {
				t.Errorf("text = %q, want %q", got, tt.want)
			}
			if links != tt.wantLinks {
				t.Errorf("link counter = %d, want %d", links, tt.wantLinks)
			}
			if images != tt.wantImages {
				t.Errorf("image counter = %d, want %d", images, tt.wantImages)
			}
		})
	}

	if got := tableTokenText(nil, nil, nil, false); got != "" {
		t.Errorf("tableTokenText(nil) = %q, want empty", got)
	}
}

// TestExtractChromeTextWithStructure unit-tests the chrome-region entry
// point at the internal layer (the root package only reaches it through
// ReadPage's chrome sections): a nil node is a no-op, a nested <nav> inside
// the region survives (it is the region's substance, unlike the content
// pass's skip policy), raw-text subtrees are skipped, and the shared
// [LINK:n]/[IMAGE:n] counters stay in lockstep with the emitted tokens.
func TestExtractChromeTextWithStructure(t *testing.T) {
	t.Parallel()

	ExtractChromeTextWithStructure(nil, table.NewTrackedBuilder(), nil, nil, "markdown") // must not panic

	doc, err := parseHTML(`<html><body><nav class="site-nav">
		<a href="/">Home</a>
		<a href="/tech"><img src="/i.png" alt="Tech">Tech</a>
		<script>evil()</script>
		<nav><a href="/deep">Deep</a></nav>
	</nav></body></html>`)
	if err != nil {
		t.Fatalf("parseHTML: %v", err)
	}
	region := firstElementByTag(doc, "nav")
	if region == nil {
		t.Fatal("test fixture error: nav not found")
	}

	tb := table.NewTrackedBuilder()
	images, links := 0, 0
	ExtractChromeTextWithStructure(region, tb, &images, &links, "markdown")
	got := tb.String()

	for _, want := range []string{"Home", "Tech", "Deep", "[LINK:", "[IMAGE:1]"} {
		if !strings.Contains(got, want) {
			t.Errorf("chrome text missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "evil") {
		t.Errorf("script text leaked into chrome region: %q", got)
	}
	if links < 3 || images != 1 {
		t.Errorf("counters = links:%d images:%d, want links>=3 images:1", links, images)
	}
}

// ---------------------------------------------------------------------------
// Block-element classification and spacing (from extraction_block_elements_test.go)
// ---------------------------------------------------------------------------

// TestBlockElementClassification verifies that block elements are properly classified.
func TestBlockElementClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tag     string
		isBlock bool
		isPara  bool // isParagraphLevelBlockElement
	}{
		// High priority block elements
		{"main", "main", true, true},
		{"figure", "figure", true, true},
		{"figcaption", "figcaption", true, true},
		{"thead", "thead", true, false},
		{"tbody", "tbody", true, false},
		{"tfoot", "tfoot", true, false},

		// Medium priority block elements
		{"dl", "dl", true, false},
		{"dt", "dt", true, false},
		{"dd", "dd", true, true},
		{"fieldset", "fieldset", true, true},
		{"details", "details", true, true},
		{"summary", "summary", true, true},
		{"dialog", "dialog", true, true},

		// Low priority block elements
		{"address", "address", true, true},
		{"canvas", "canvas", true, true},
		{"center", "center", true, false},
		{"form", "form", true, false},
		{"body", "body", true, false},
		{"html", "html", true, false},
		{"head", "head", true, false},

		// Existing block elements (verify no regression)
		{"p", "p", true, true},
		{"div", "div", true, true},
		{"h1", "h1", true, true},
		{"article", "article", true, true},
		{"section", "section", true, true},

		// Elements that should NOT be block (inline elements)
		{"span", "span", false, false},
		{"font", "font", false, false},
		{"b", "b", false, false},
		{"i", "i", false, false},
		{"strong", "strong", false, false},
		{"em", "em", false, false},
		{"a", "a", false, false},
		{"img", "img", false, false},
		{"br", "br", false, false}, // br should NOT be block

		// Non-content elements (block but removed)
		{"nav", "nav", true, false},
		{"aside", "aside", true, false},
		{"header", "header", true, false},
		{"footer", "footer", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test IsBlockElement
			if got := IsBlockElement(tt.tag); got != tt.isBlock {
				t.Errorf("IsBlockElement(%q) = %v, want %v", tt.tag, got, tt.isBlock)
			}

			// Test IsParagraphLevelBlockElement
			if got := IsParagraphLevelBlockElement(tt.tag); got != tt.isPara {
				t.Errorf("IsParagraphLevelBlockElement(%q) = %v, want %v", tt.tag, got, tt.isPara)
			}
		})
	}
}

// TestNewBlockElementsSpacing verifies that new block elements add proper spacing.
func TestNewBlockElementsSpacing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want string
		desc string
	}{
		{
			name: "main element adds paragraph spacing",
			html: `<main>Content 1</main><main>Content 2</main>`,
			want: "Content 1\n\nContent 2",
			desc: "Main elements should be separated by blank lines",
		},
		{
			name: "figure element adds paragraph spacing",
			html: `<figure>Figure 1</figure><p>Text</p>`,
			want: "Figure 1\n\nText",
			desc: "Figure elements should create paragraph separation",
		},
		{
			name: "figcaption element adds paragraph spacing",
			html: `<img src="test.jpg"><figcaption>Caption</figcaption><p>Text</p>`,
			want: "[IMAGE:1]\nCaption\n\nText",
			desc: "Figcaption elements should create paragraph separation",
		},
		{
			name: "definition list dl adds paragraph spacing",
			html: `<p>Before</p><dl><dt>Term</dt><dd>Definition</dd></dl><p>After</p>`,
			want: "Before\n\nTerm\n: Definition\n\nAfter",
			desc: "DL elements should create paragraph separation; DD is marked with ': '",
		},
		{
			name: "dt stays tight to dd, pairs separated by blank line",
			html: `<dl><dt>Term 1</dt><dd>Def 1</dd><dt>Term 2</dt><dd>Def 2</dd></dl>`,
			want: "Term 1\n: Def 1\n\nTerm 2\n: Def 2",
			desc: "DT is tight to its DD; DD paragraph spacing separates term/definition pairs",
		},
		{
			name: "fieldset adds paragraph spacing",
			html: `<fieldset>Field 1</fieldset><fieldset>Field 2</fieldset>`,
			want: "Field 1\n\nField 2",
			desc: "Fieldset elements should create paragraph separation",
		},
		{
			name: "details adds paragraph spacing",
			html: `<details>Content 1</details><details>Content 2</details>`,
			want: "Content 1\n\nContent 2",
			desc: "Details elements should create paragraph separation",
		},
		{
			name: "summary adds paragraph spacing",
			html: `<details><summary>Title</summary>Content</details><p>Text</p>`,
			want: "Title\n\nContent\n\nText", // Summary creates paragraph spacing
			desc: "Summary elements should create paragraph separation",
		},
		{
			name: "dialog adds paragraph spacing",
			html: `<dialog>Dialog 1</dialog><dialog>Dialog 2</dialog>`,
			want: "Dialog 1\n\nDialog 2",
			desc: "Dialog elements should create paragraph separation",
		},
		{
			name: "address adds paragraph spacing",
			html: `<address>123 Main St</address><p>City</p>`,
			want: "123 Main St\n\nCity",
			desc: "Address elements should create paragraph separation",
		},
		{
			name: "canvas adds paragraph spacing",
			html: `<canvas>Canvas 1</canvas><canvas>Canvas 2</canvas>`,
			want: "Canvas 1\n\nCanvas 2",
			desc: "Canvas elements should create paragraph separation",
		},
		{
			name: "thead does not add paragraph spacing",
			html: `<table><thead><th>H1</th></thead><tbody><td>D1</td></tbody></table>`,
			want: "| H1 |\n| --- |\n| D1 |", // Note: no trailing newlines
			desc: "Thead should not add paragraph spacing in tables",
		},
		{
			name: "tbody does not add paragraph spacing",
			html: `<table><tr><td>Row 1</td></tr></table><p>Text</p>`,
			// Headerless table (no <th>): an empty header row is synthesized so
			// the real "Row 1" is rendered as data rather than promoted to a header.
			// Cells carry no column-width padding (removed with the padding
			// machinery; see internal/table/render.go).
			want: "|  |\n| --- |\n| Row 1 |\n\n\nText",
			desc: "Table structure should not add extra paragraph spacing",
		},
		{
			name: "center does not add paragraph spacing",
			html: `<center>Text 1</center><center>Text 2</center>`,
			want: "Text 1\nText 2",
			desc: "Center elements are deprecated and should not add paragraph spacing",
		},
		{
			name: "br is inline and does not add paragraph spacing",
			html: `<p>Line 1<br>Line 2<br>Line 3</p>`,
			want: "Line 1\nLine 2\nLine 3",
			desc: "BR should create single newlines, not paragraph spacing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := html.Parse(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			tb := table.NewTrackedBuilder()
			imageCounter := 0
			ExtractTextWithStructureAndImages(doc, tb, &imageCounter, nil, "markdown")
			got := strings.TrimSpace(tb.String())

			// Normalize newlines for comparison
			got = strings.ReplaceAll(got, "\r\n", "\n")
			want := strings.ReplaceAll(tt.want, "\r\n", "\n")

			if got != want {
				t.Errorf("%s\n\nGot:\n%s\n\nWant:\n%s\n\n%s", tt.desc, got, want, tt.name)
			}
		})
	}
}

// TestTableStructureElementsSpacing verifies table structure elements behavior.
func TestTableStructureElementsSpacing(t *testing.T) {
	t.Parallel()

	htmlContent := `<table>
		<thead><tr><th>Header 1</th><th>Header 2</th></tr></thead>
		<tbody><tr><td>Data 1</td><td>Data 2</td></tr></tbody>
		<tfoot><tr><td>Footer 1</td><td>Footer 2</td></tr></tfoot>
	</table>`

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	tb := table.NewTrackedBuilder()
	ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
	result := tb.String()

	// Verify table is rendered correctly with proper structure
	expectedParts := []string{"Header 1", "Header 2", "Data 1", "Data 2", "Footer 1", "Footer 2"}
	for _, part := range expectedParts {
		if !strings.Contains(result, part) {
			t.Errorf("Expected to find %q in result", part)
		}
	}

	// Verify alignment row is present (contains "---")
	if !strings.Contains(result, "---") {
		t.Error("Expected table to contain alignment row with ---")
	}
}

// TestDefinitionListFormatting tests proper formatting of definition lists.
func TestDefinitionListFormatting(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "simple definition list",
			html: `<dl><dt>Term 1</dt><dd>Definition 1</dd></dl>`,
			want: "Term 1\n: Definition 1", // DT tight to its DD; DD marked with ': '
		},
		{
			name: "multiple terms and definitions",
			html: `<dl><dt>Term 1</dt><dd>Def 1</dd><dt>Term 2</dt><dd>Def 2</dd></dl>`,
			want: "Term 1\n: Def 1\n\nTerm 2\n: Def 2", // DD paragraph spacing separates pairs
		},
		{
			name: "definition list with inline markup",
			html: `<dl><dt><strong>Term</strong></dt><dd><em>Definition</em></dd></dl>`,
			want: "Term\n: Definition", // DT tight to its DD; DD marked with ': '
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
			got := strings.TrimSpace(tb.String())

			if got != tt.want {
				t.Errorf("Got:\n%s\n\nWant:\n%s", got, tt.want)
			}
		})
	}
}

// TestInteractiveElementsSpacing tests interactive elements (details, summary, dialog).
func TestInteractiveElementsSpacing(t *testing.T) {
	t.Parallel()

	htmlContent := `<p>Introduction</p>
	<details>
		<summary>Click to expand</summary>
		<p>Detailed content here</p>
	</details>
	<dialog>
		<p>Dialog content</p>
	</dialog>
	<p>Conclusion</p>`

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	tb := table.NewTrackedBuilder()
	ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
	result := strings.TrimSpace(tb.String())

	// Verify paragraph spacing between elements
	paragraphCount := 0
	lines := strings.Split(result, "\n")
	for i := 0; i < len(lines)-1; i++ {
		if strings.TrimSpace(lines[i]) == "" && strings.TrimSpace(lines[i+1]) == "" {
			paragraphCount++
		}
	}

	// We expect multiple paragraph separations (at least 2)
	if paragraphCount < 2 {
		t.Logf("Result:\n%s", result)
		t.Errorf("Expected at least 2 paragraph separations, got %d", paragraphCount)
	}

	// Verify content is preserved
	expectedContent := []string{"Introduction", "Click to expand", "Detailed content", "Dialog content", "Conclusion"}
	for _, content := range expectedContent {
		if !strings.Contains(result, content) {
			t.Errorf("Expected to find %q in result", content)
		}
	}
}

// TestBRElementBehavior verifies that br is now treated as inline.
func TestBRElementBehavior(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "single br",
			html: `<p>Line 1<br>Line 2</p>`,
			want: "Line 1\nLine 2",
		},
		{
			name: "multiple br",
			html: `<p>Line 1<br>Line 2<br>Line 3</p>`,
			want: "Line 1\nLine 2\nLine 3",
		},
		{
			name: "br in div",
			html: `<div>Line 1<br>Line 2</div>`,
			want: "Line 1\nLine 2",
		},
		{
			name: "br with other inline elements",
			html: `<p><strong>Bold</strong><br><em>Italic</em></p>`,
			want: "Bold \nItalic", // Note: space after "Bold" is from inline element spacing
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
			got := strings.TrimSpace(tb.String())

			if got != tt.want {
				t.Errorf("Got:\n%s\n\nWant:\n%s", got, tt.want)
			}
		})
	}
}

// BenchmarkNewBlockElementExtraction benchmarks extraction with new block elements.
func BenchmarkNewBlockElementExtraction(b *testing.B) {
	htmlContent := `<main>
		<figure><figcaption>Caption</figcaption></figure>
		<dl><dt>Term</dt><dd>Definition</dd></dl>
		<details><summary>Summary</summary>Content</details>
		<dialog>Dialog content</dialog>
		<address>123 Main St</address>
		<fieldset>Field content</fieldset>
	</main>`

	b.ResetTimer()
	for b.Loop() {
		doc, err := html.Parse(strings.NewReader(htmlContent))
		if err != nil {
			b.Fatalf("Failed to parse HTML: %v", err)
		}

		tb := table.NewTrackedBuilder()
		ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
	}
}

// ---------------------------------------------------------------------------
// <pre> newline preservation and data/layout table disambiguation
// (from pre_newline_regression_test.go — P2-9 heuristic and P2-11 fix)
// ---------------------------------------------------------------------------

// extractTextForPre runs the full text-extraction pass and returns the cleaned
// text, mirroring the production path (CleanTextBytes after the structure pass).
func extractTextForPre(t *testing.T, markup string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tb := table.NewTrackedBuilder()
	ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
	return CleanTextBytes(tb.Bytes())
}

func TestPrePreservesNewlines(t *testing.T) {
	t.Parallel()

	text := extractTextForPre(t, `<div><pre>line one
line two
line three</pre></div>`)

	if !strings.Contains(text, "line one\nline two\nline three") {
		t.Errorf("<pre> newlines were folded into spaces:\n%s", text)
	}
}

func TestPreCodePreservesNewlines(t *testing.T) {
	t.Parallel()

	text := extractTextForPre(t, `<div><pre><code>func main() {
	fmt.Println("hi")
}</code></pre></div>`)

	// Newlines must survive as line breaks. The tab is collapsed to a space by
	// CleanTextBytes, which folds tabs to single spaces on every line (a
	// document-wide whitespace policy, not a <pre> regression) — so assert the
	// line structure, not the exact indentation byte.
	if !strings.Contains(text, "func main() {\n") {
		t.Errorf("<pre><code> newline after '{' was lost:\n%s", text)
	}
	if !strings.Contains(text, "fmt.Println(\"hi\")") {
		t.Errorf("<pre><code> code line was lost:\n%s", text)
	}
	if !strings.Contains(text, "}") {
		t.Errorf("<pre><code> closing brace was lost:\n%s", text)
	}
}

// TestPreInArticleStillPreserved guards the interaction with P0-2: a <pre>
// nested inside an <article> is still a code block, not article furniture, and
// keeps its line structure.
func TestPreInArticleStillPreserved(t *testing.T) {
	t.Parallel()

	text := extractTextForPre(t, `<article><p>intro</p><pre>a
b
c</pre></article>`)

	if !strings.Contains(text, "a\nb\nc") {
		t.Errorf("<pre> inside <article> lost newlines:\n%s", text)
	}
}

// TestPreNBSPFolded guards that NBSP inside <pre> is still folded to a regular
// space, since a raw U+00A0 in code output would be visually misleading.
func TestPreNBSPFolded(t *testing.T) {
	t.Parallel()

	text := extractTextForPre(t, "<div><pre>a b</pre></div>")

	if !strings.Contains(text, "a b") {
		t.Errorf("NBSP in <pre> was not folded to a space: %q", text)
	}
}

// TestDataTableWithEmbeddedTableStillRenders guards the P2-11 fix: a data table
// that embeds a small sub-table in one cell (carrying its own cell text) must
// render as a data table, not be flattened as a layout shell. The previous
// rule treated any nested <table> as proof of layout.
func TestDataTableWithEmbeddedTableStillRenders(t *testing.T) {
	t.Parallel()

	// Outer table has a cell with substantial direct text AND a nested sub-table.
	src := `<table><tr><td>This cell has its own meaningful descriptive text.</td></tr>` +
		`<tr><td>Another cell with prose text content here.</td></tr>` +
		`</table>`

	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tb := table.NewTrackedBuilder()
	ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
	out := collapseSpaces(tb.String())

	// The outer table's own cell text must survive as a data table row.
	if !strings.Contains(out, "This cell has its own meaningful descriptive text") {
		t.Errorf("data table with nested sub-table was flattened, cell text lost:\n%s", out)
	}
}

// TestLayoutTableStillFlattened guards the other half of the P2-11 contract: a
// pure layout table (nested table, no direct text of its own) is still treated
// as a shell and its nested data table renders on its own terms.
func TestLayoutTableStillFlattened(t *testing.T) {
	t.Parallel()

	src := `<table><tr><td>` +
		`<table><tr><td>A</td><td>B</td></tr><tr><td>C</td><td>D</td></tr></table>` +
		`</td></tr></table>`

	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tb := table.NewTrackedBuilder()
	ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
	out := collapseSpaces(tb.String())

	if !strings.Contains(out, "| A | B |") {
		t.Errorf("inner data table row '| A | B |' lost:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// List-continuation and reader-mode pruning boundaries ([FIX-001] additions)
// ---------------------------------------------------------------------------

// TestPendingListMarkerLine pins the marker-line detector over every marker
// shape it must accept (bullet, asterisk, plus, dd colon, ordered "N.",
// indented, after a newline) and reject (content already on the line, bare
// digits without a dot, empty builder).
func TestPendingListMarkerLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		line string
		want bool
	}{
		{"", false},
		{"-", true},
		{"* ", true},
		{"+", true},
		{"  : ", true},
		{"hello\n- ", true},
		{"- x", false},
		{"12 ", false},
		{". ", false},
		{"hello", false},
		// Ordered markers "N." end the line with '.', followed leftward by the
		// digits. The scan used to verify the char before the digits instead
		// (".<digits>" — a shape no marker writer produces), so "N." never
		// registered as pending; fixed to match the documentation (M-1).
		{"1. ", true},
		{"  12. ", true},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			t.Parallel()
			tb := table.NewTrackedBuilder()
			_, _ = tb.WriteString(tt.line)
			if got := pendingListMarkerLine(tb); got != tt.want {
				t.Errorf("pendingListMarkerLine(%q) = %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

// TestContinuesListItemGuards pins every guard of the chrome-mode list
// continuation: chrome mode only, p/div children only, li/dd parents only,
// and the pending-marker requirement.
func TestContinuesListItemGuards(t *testing.T) {
	t.Parallel()

	p := func(data string) *html.Node { return &html.Node{Type: html.ElementNode, Data: data} }
	tb := table.NewTrackedBuilder()
	_, _ = tb.WriteString("- ") // pending marker line

	tests := []struct {
		name        string
		node        *html.Node
		parent      *html.Node
		chromeMode  bool
		markerReady bool
		want        bool
	}{
		{"content mode never continues", p("p"), p("li"), false, true, false},
		{"nil node", nil, p("li"), true, true, false},
		{"nil parent", p("p"), nil, true, true, false},
		{"non p/div node", p("span"), p("li"), true, true, false},
		{"non li/dd parent", p("p"), p("section"), true, true, false},
		{"no pending marker", p("p"), p("li"), true, false, false},
		{"p in li with pending marker", p("p"), p("li"), true, true, true},
		{"div in dd with pending marker", p("div"), p("dd"), true, true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tbx := table.NewTrackedBuilder()
			if tt.markerReady {
				_, _ = tbx.WriteString("- ")
			} else {
				_, _ = tbx.WriteString("- x")
			}
			if got := continuesListItem(tt.node, tt.parent, tbx, tt.chromeMode); got != tt.want {
				t.Errorf("continuesListItem(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

// TestRemoveHiddenElementsReader drives the exported reader-mode pruner
// directly: always-hidden signals (hidden attribute, aria-hidden,
// display:none, d-none class) are removed in place, while the interaction
// hooks a reader must keep (bare "hidden" class, responsive "hidden md:flex")
// survive, and a nil root stays nil.
func TestRemoveHiddenElementsReader(t *testing.T) {
	t.Parallel()

	if got := RemoveHiddenElementsReader(nil); got != nil {
		t.Errorf("RemoveHiddenElementsReader(nil) = %v, want nil", got)
	}

	doc, err := parseHTML(`<html><body><nav>
		<div hidden>gone-attr</div>
		<div aria-hidden="true">gone-aria</div>
		<div style="display:none">gone-style</div>
		<div class="d-none">gone-class</div>
		<div class="hidden">kept-bare-hidden</div>
		<div class="hidden md:flex">kept-responsive</div>
		<a href="/x">kept-link</a>
	</nav></body></html>`)
	if err != nil {
		t.Fatal(err)
	}
	nav := firstElementByTag(doc, "nav")
	if nav == nil {
		t.Fatal("fixture error: nav not found")
	}
	root := RemoveHiddenElementsReader(nav)
	if root != nav {
		t.Fatal("RemoveHiddenElementsReader must return its argument node")
	}
	text := GetTextContent(root)
	for _, gone := range []string{"gone-attr", "gone-aria", "gone-style", "gone-class"} {
		if strings.Contains(text, gone) {
			t.Errorf("hidden element %q survived reader pruning", gone)
		}
	}
	for _, kept := range []string{"kept-bare-hidden", "kept-responsive", "kept-link"} {
		if !strings.Contains(text, kept) {
			t.Errorf("reader-relevant element %q was pruned", kept)
		}
	}
}
