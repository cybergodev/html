package internal

import (
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/cybergodev/html/internal/table"
)

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
			want: "| H1  |\n| --- |\n| D1  |", // Note: no trailing newlines
			desc: "Thead should not add paragraph spacing in tables",
		},
		{
			name: "tbody does not add paragraph spacing",
			html: `<table><tr><td>Row 1</td></tr></table><p>Text</p>`,
			// Headerless table (no <th>): an empty header row is synthesized so
			// the real "Row 1" is rendered as data rather than promoted to a header.
			want: "|       |\n| --- |\n| Row 1 |\n\n\nText",
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
	for i := 0; i < b.N; i++ {
		doc, err := html.Parse(strings.NewReader(htmlContent))
		if err != nil {
			b.Fatalf("Failed to parse HTML: %v", err)
		}

		tb := table.NewTrackedBuilder()
		ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
	}
}
