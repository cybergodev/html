package internal

import (
	"strings"
	"testing"

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
			var sb strings.Builder
			ExtractTextWithStructureAndImages(doc, &sb, nil, nil, "markdown")
			result := strings.TrimSpace(sb.String())

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
			var sb strings.Builder
			ExtractTextWithStructureAndImages(doc, &sb, nil, nil, "markdown")
			result := strings.TrimSpace(sb.String())

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
	var sb strings.Builder
	ExtractTextWithStructureAndImages(doc, &sb, nil, nil, "markdown")

	// ExtractTextWithStructureAndImages does not run CleanText, so column
	// padding introduces runs of spaces. Collapse them for stable assertions.
	out := collapseSpaces(sb.String())

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
			var sb strings.Builder
			imageCounter := 0
			ExtractTextWithStructureAndImages(doc, &sb, &imageCounter, nil, "markdown")
			result := strings.TrimSpace(sb.String())

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

	var sb strings.Builder
	ExtractTextWithStructureAndImages(nil, &sb, nil, nil, "markdown")

	if sb.Len() != 0 {
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
			var sb strings.Builder
			ExtractTextWithStructureAndImages(doc, &sb, nil, nil, "markdown")
			result := sb.String()

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
	var sb strings.Builder
	ExtractTextWithStructureAndImages(tableNode, &sb, nil, nil, "markdown")

	result := strings.TrimSpace(sb.String())
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
			var sb strings.Builder
			ExtractTextWithStructureAndImages(doc, &sb, nil, nil, "markdown")
			result := sb.String()

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
	for i := 0; i < b.N; i++ {
		var sb strings.Builder
		ExtractTextWithStructureAndImages(doc, &sb, nil, nil, "markdown")
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
			var sb strings.Builder
			ExtractTextWithStructureAndImages(doc, &sb, nil, nil, "html")

			result := sb.String()
			for _, want := range tt.want {
				if !strings.Contains(result, want) {
					t.Errorf("Output should contain %q, got:\n%s", want, result)
				}
			}
		})
	}
}
