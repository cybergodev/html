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
