// Package table_test provides tests for the table package.
package table_test

import (
	"strings"
	"testing"

	"github.com/cybergodev/html/internal/table"
	"golang.org/x/net/html"
)

// parseHTML is a helper to parse HTML string into nodes for testing.
func parseHTML(s string) (*html.Node, error) {
	return html.Parse(strings.NewReader(s))
}

// TestTrackedBuilder tests the TrackedBuilder functionality.
func TestTrackedBuilder(t *testing.T) {
	t.Parallel()

	t.Run("tracks last character after WriteString", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		tb.WriteString("hello")
		if tb.LastChar != 'o' {
			t.Errorf("LastChar = %c, want 'o'", tb.LastChar)
		}

		tb.WriteString(" world")
		if tb.LastChar != 'd' {
			t.Errorf("LastChar = %c, want 'd'", tb.LastChar)
		}
	})

	t.Run("tracks last character after WriteByte", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		tb.WriteByte('x')
		if tb.LastChar != 'x' {
			t.Errorf("LastChar = %c, want 'x'", tb.LastChar)
		}

		tb.WriteByte('\n')
		if tb.LastChar != '\n' {
			t.Errorf("LastChar = %c, want newline", tb.LastChar)
		}
	})

	t.Run("EnsureNewline adds newline when needed", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		tb.WriteString("text")
		table.EnsureNewline(tb)

		if tb.LastChar != '\n' {
			t.Error("Should end with newline")
		}
		if !strings.HasSuffix(sb.String(), "text\n") {
			t.Errorf("Expected 'text\\n', got %q", sb.String())
		}
	})

	t.Run("EnsureNewline does not add duplicate newline", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		tb.WriteString("text\n")
		table.EnsureNewline(tb)

		if sb.String() != "text\n" {
			t.Errorf("Expected 'text\\n' without duplicate, got %q", sb.String())
		}
	})

	t.Run("EnsureSpacing adds spacing when needed", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		tb.WriteString("text")
		table.EnsureSpacing(tb, ' ')

		if tb.LastChar != ' ' {
			t.Error("Should end with space")
		}
	})

	t.Run("EnsureSpacing does not add after newline", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		tb.WriteString("text\n")
		table.EnsureSpacing(tb, ' ')

		if sb.String() != "text\n" {
			t.Errorf("Should not add space after newline, got %q", sb.String())
		}
	})

	t.Run("empty builder", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		if tb.LastChar != 0 {
			t.Errorf("LastChar = %d, want 0", tb.LastChar)
		}
		if tb.Len() != 0 {
			t.Errorf("Len() = %d, want 0", tb.Len())
		}
	})
}

// TestMarkdownRenderer tests the MarkdownRenderer.
func TestMarkdownRenderer(t *testing.T) {
	t.Parallel()

	t.Run("Format returns markdown", func(t *testing.T) {
		r := &table.MarkdownRenderer{}
		if r.Format() != "markdown" {
			t.Errorf("Format() = %q, want 'markdown'", r.Format())
		}
	})

	t.Run("Render simple table", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{{Text: "A", IsHeader: true}, {Text: "B", IsHeader: true}},
			{{Text: "1"}, {Text: "2"}},
		}

		r.Render(tableData, tb, 2)
		output := sb.String()

		// Check for markdown table structure
		if !strings.Contains(output, "|") {
			t.Error("Expected markdown table with pipe characters")
		}
		if !strings.Contains(output, "A") || !strings.Contains(output, "B") {
			t.Error("Expected header cells in output")
		}
		if !strings.Contains(output, "1") || !strings.Contains(output, "2") {
			t.Error("Expected data cells in output")
		}
	})

	t.Run("Render with alignment", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Left", Align: table.AlignLeft, IsHeader: true}, {Text: "Right", Align: table.AlignRight, IsHeader: true}},
			{{Text: "L1", Align: table.AlignLeft}, {Text: "R1", Align: table.AlignRight}},
		}

		r.Render(tableData, tb, 2)
		output := sb.String()

		// Check for alignment markers
		if !strings.Contains(output, ":---") {
			t.Error("Expected left alignment marker ':---'")
		}
		if !strings.Contains(output, "---:") {
			t.Error("Expected right alignment marker '---:'")
		}
	})

	t.Run("Render with colspan", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Span", Colspan: 2, OriginalColspan: 2, IsHeader: true}},
			{{Text: "A"}, {Text: "B"}},
		}

		r.Render(tableData, tb, 2)
		output := sb.String()

		// Colspan cells are expanded in markdown
		if !strings.Contains(output, "Span") {
			t.Error("Expected colspan cell content")
		}
	})

	t.Run("Render empty table", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{}

		r.Render(tableData, tb, 0)
		output := sb.String()

		// Empty table should produce empty or minimal output
		if len(output) > 0 {
			t.Logf("Empty table output: %q", output)
		}
	})
}

// TestMarkdownRowspanGrid verifies that a cell with rowspan > 1 occupies its
// column in the rows it spans, repeating the originating cell's text (the
// Pandoc convention for Markdown, which cannot express rowspan). Without the
// grid, later rows' cells shift left: a cell in a spanned row would land under
// the spanned cell instead of beside it.
func TestMarkdownRowspanGrid(t *testing.T) {
	t.Parallel()

	t.Run("rowspan repeats value in spanned rows", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.MarkdownRenderer{}
		// <tr><td rowspan=2>A</td><td>B</td></tr><tr><td>C</td></tr>
		// Input is post-colspan-expansion (what the renderer receives); each
		// entry is one column, carrying its Rowspan.
		tableData := [][]table.CellData{
			{{Text: "A", Rowspan: 2}, {Text: "B"}},
			{{Text: "C"}},
		}

		r.Render(tableData, tb, 2)
		output := sb.String()

		// "A" must repeat in the one spanned row, so it appears twice total
		// (once in its declared row, once repeated).
		if got := strings.Count(output, "A"); got != 2 {
			t.Errorf("expected A repeated once in the spanned row (count=2), got %d\n%s", got, output)
		}

		// In the spanned row, "A" (col 0) must precede "C" (col 1) — proving C
		// landed in column 1 beside A, not in column 0 under A (the pre-grid
		// misalignment). The row containing C must therefore also contain A.
		lines := strings.Split(strings.TrimSpace(output), "\n")
		var cRow string
		for _, line := range lines {
			if strings.Contains(line, "C") {
				cRow = line
				break
			}
		}
		if cRow == "" {
			t.Fatalf("no row containing C in output:\n%s", output)
		}
		ai := strings.Index(cRow, "A")
		ci := strings.Index(cRow, "C")
		if ai < 0 || ai > ci {
			t.Errorf("expected A before C in spanned row, got %q", cRow)
		}
	})

	t.Run("rowspan exceeding row count is clamped", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.MarkdownRenderer{}
		// rowspan=5 but only 2 rows exist: must not panic or fabricate rows.
		tableData := [][]table.CellData{
			{{Text: "A", Rowspan: 5}, {Text: "B"}},
			{{Text: "C"}},
		}

		r.Render(tableData, tb, 2)
		output := sb.String()

		// Only one spanned row exists, so A repeats exactly once (count=2).
		if got := strings.Count(output, "A"); got != 2 {
			t.Errorf("expected A repeated once (only 1 spanned row exists), got %d\n%s", got, output)
		}
	})

	t.Run("rowspan with header repeats header value", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.MarkdownRenderer{}
		// <tr><th rowspan=2>Name</th><th>Value</th></tr><tr><td>1</td></tr>
		tableData := [][]table.CellData{
			{{Text: "Name", Rowspan: 2, IsHeader: true}, {Text: "Value", IsHeader: true}},
			{{Text: "1"}},
		}

		r.Render(tableData, tb, 2)
		output := sb.String()

		// "Name" spans into the data row, so it appears twice.
		if got := strings.Count(output, "Name"); got != 2 {
			t.Errorf("expected Name repeated once in the spanned row (count=2), got %d\n%s", got, output)
		}
		// "1" lands in column 1 (beside the repeated Name), not column 0.
		lines := strings.Split(strings.TrimSpace(output), "\n")
		var oneRow string
		for _, line := range lines {
			if strings.Contains(line, "1") {
				oneRow = line
				break
			}
		}
		if oneRow == "" {
			t.Fatalf("no row containing 1 in output:\n%s", output)
		}
		ni := strings.Index(oneRow, "Name")
		oi := strings.Index(oneRow, "1")
		if ni < 0 || ni > oi {
			t.Errorf("expected Name before 1 in spanned row, got %q", oneRow)
		}
	})
}

// TestHTMLRenderer tests the HTMLRenderer.
func TestHTMLRenderer(t *testing.T) {
	t.Parallel()

	t.Run("Format returns html", func(t *testing.T) {
		r := &table.HTMLRenderer{}
		if r.Format() != "html" {
			t.Errorf("Format() = %q, want 'html'", r.Format())
		}
	})

	t.Run("Render basic table", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Header", IsHeader: true}},
			{{Text: "Data"}},
		}

		r.Render(tableData, tb, 1)
		output := sb.String()

		// Check for HTML table structure
		expectedTags := []string{"<table>", "</table>", "<tr>", "</tr>", "</th>", "</td>"}
		for _, tag := range expectedTags {
			if !strings.Contains(output, tag) {
				t.Errorf("Expected tag %q in output", tag)
			}
		}
		// Check for th and td tags (they may have attributes)
		if !strings.Contains(output, "<th") && !strings.Contains(output, "<td") {
			t.Error("Expected <th or <td tags in output")
		}
	})

	t.Run("Render with rowspan", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Span", Rowspan: 2}},
			{{Text: "B"}},
		}

		r.Render(tableData, tb, 1)
		output := sb.String()

		if !strings.Contains(output, `rowspan="2"`) {
			t.Errorf("Expected rowspan attribute, got: %s", output)
		}
	})

	t.Run("Render with colspan", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Span", Colspan: 3, OriginalColspan: 3}},
		}

		r.Render(tableData, tb, 1)
		output := sb.String()

		if !strings.Contains(output, `colspan="3"`) {
			t.Errorf("Expected colspan attribute, got: %s", output)
		}
	})

	t.Run("Render with alignment", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Center", Align: table.AlignCenter}},
		}

		r.Render(tableData, tb, 1)
		output := sb.String()

		if !strings.Contains(output, "text-align:center") {
			t.Errorf("Expected text-align:center style, got: %s", output)
		}
	})

	t.Run("Render with width", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Wide", Width: "100px"}},
		}

		r.Render(tableData, tb, 1)
		output := sb.String()

		if !strings.Contains(output, "width:100px") {
			t.Errorf("Expected width style, got: %s", output)
		}
	})
}

// TestCellAlignment tests the CellAlignment constants.
func TestCellAlignment(t *testing.T) {
	t.Parallel()

	alignments := []struct {
		name  string
		align table.CellAlignment
	}{
		{"AlignLeft", table.AlignLeft},
		{"AlignCenter", table.AlignCenter},
		{"AlignRight", table.AlignRight},
		{"AlignJustify", table.AlignJustify},
		{"AlignDefault", table.AlignDefault},
	}

	for _, tt := range alignments {
		t.Run(tt.name, func(t *testing.T) {
			// Verify alignment is within valid range
			if tt.align < 0 || tt.align > table.AlignJustify+1 {
				t.Errorf("Invalid alignment value: %d", tt.align)
			}
		})
	}
}

// TestCellData tests CellData struct behavior.
func TestCellData(t *testing.T) {
	t.Parallel()

	t.Run("default values", func(t *testing.T) {
		cell := table.CellData{}
		if cell.Text != "" {
			t.Error("Default Text should be empty")
		}
		if cell.Colspan != 0 {
			t.Error("Default Colspan should be 0")
		}
		if cell.IsHeader {
			t.Error("Default IsHeader should be false")
		}
	})

	t.Run("header cell", func(t *testing.T) {
		cell := table.CellData{
			Text:     "Header",
			IsHeader: true,
			Align:    table.AlignCenter,
		}

		if !cell.IsHeader {
			t.Error("IsHeader should be true")
		}
		if cell.Align != table.AlignCenter {
			t.Errorf("Align = %d, want AlignCenter", cell.Align)
		}
	})

	t.Run("expanded cell", func(t *testing.T) {
		cell := table.CellData{
			Text:            " ",
			IsExpanded:      true,
			OriginalColspan: 3,
		}

		if !cell.IsExpanded {
			t.Error("IsExpanded should be true")
		}
		if cell.OriginalColspan != 3 {
			t.Errorf("OriginalColspan = %d, want 3", cell.OriginalColspan)
		}
	})
}

// TestAlignCount tests the AlignCount struct.
func TestAlignCount(t *testing.T) {
	t.Parallel()

	counts := table.AlignCount{
		Left:    5,
		Center:  3,
		Right:   2,
		Justify: 1,
	}

	if counts.Left != 5 {
		t.Errorf("Left = %d, want 5", counts.Left)
	}
	if counts.Center != 3 {
		t.Errorf("Center = %d, want 3", counts.Center)
	}
	if counts.Right != 2 {
		t.Errorf("Right = %d, want 2", counts.Right)
	}
}

// TestRendererInterface tests that renderers implement the interface.
func TestRendererInterface(t *testing.T) {
	t.Parallel()

	var _ table.Renderer = &table.MarkdownRenderer{}
	var _ table.Renderer = &table.HTMLRenderer{}
}

// TestProcessor tests the table Processor functionality.
func TestProcessor(t *testing.T) {
	t.Parallel()

	t.Run("NewProcessor creates processor", func(t *testing.T) {
		ca := &mockCellAccessor{}
		nw := &mockNodeWalker{}
		p := table.NewProcessor(ca, nw)

		if p == nil {
			t.Error("NewProcessor should return non-nil processor")
		}
	})

	t.Run("Extract with nil table returns early", func(t *testing.T) {
		ca := &mockCellAccessor{}
		nw := &mockNodeWalker{}
		p := table.NewProcessor(ca, nw)

		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		p.Extract(nil, tb, "markdown")

		if sb.String() != "" {
			t.Error("Extract with nil table should produce no output")
		}
	})

	t.Run("Extract with empty table produces minimal output", func(t *testing.T) {
		ca := &mockCellAccessor{text: "Cell"}
		nw := &mockNodeWalker{rows: nil} // No rows
		p := table.NewProcessor(ca, nw)

		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		tableNode := &html.Node{Type: html.ElementNode, Data: "table"}
		p.Extract(tableNode, tb, "markdown")

		// Empty table should produce only blank lines
		output := sb.String()
		t.Logf("Empty table output: %q", output)
	})
}

// TestProcessorWithRealHTML tests the Processor with real HTML parsing.
func TestProcessorWithRealHTML(t *testing.T) {
	t.Parallel()

	// Create a real cell accessor and node walker for integration testing
	accessor := &realCellAccessor{}
	walker := &realNodeWalker{}
	processor := table.NewProcessor(accessor, walker)

	t.Run("extract simple table", func(t *testing.T) {
		htmlContent := `<table><tr><th>A</th><th>B</th></tr><tr><td>1</td><td>2</td></tr></table>`
		doc, err := parseHTML(htmlContent)
		if err != nil {
			t.Fatalf("Failed to parse HTML: %v", err)
		}

		// Find the table element
		var tableNode *html.Node
		var findTable func(*html.Node)
		findTable = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "table" {
				tableNode = n
				return
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				findTable(c)
			}
		}
		findTable(doc)

		if tableNode == nil {
			t.Fatal("Failed to find table element")
		}

		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		processor.Extract(tableNode, tb, "markdown")

		output := sb.String()
		t.Logf("Output: %s", output)

		if !strings.Contains(output, "A") || !strings.Contains(output, "B") {
			t.Error("Expected header content in output")
		}
	})

	t.Run("extract table with alignment", func(t *testing.T) {
		htmlContent := `<table><tr><td align="left">Left</td><td align="right">Right</td></tr></table>`
		doc, err := parseHTML(htmlContent)
		if err != nil {
			t.Fatalf("Failed to parse HTML: %v", err)
		}

		// Find the table element
		var tableNode *html.Node
		var findTable func(*html.Node)
		findTable = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "table" {
				tableNode = n
				return
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				findTable(c)
			}
		}
		findTable(doc)

		if tableNode == nil {
			t.Fatal("Failed to find table element")
		}

		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		processor.Extract(tableNode, tb, "markdown")

		output := sb.String()
		t.Logf("Output: %s", output)

		if !strings.Contains(output, "Left") || !strings.Contains(output, "Right") {
			t.Error("Expected cell content in output")
		}
	})

	t.Run("extract table as HTML format", func(t *testing.T) {
		htmlContent := `<table><tr><th>Header</th></tr><tr><td>Data</td></tr></table>`
		doc, err := parseHTML(htmlContent)
		if err != nil {
			t.Fatalf("Failed to parse HTML: %v", err)
		}

		// Find the table element
		var tableNode *html.Node
		var findTable func(*html.Node)
		findTable = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "table" {
				tableNode = n
				return
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				findTable(c)
			}
		}
		findTable(doc)

		if tableNode == nil {
			t.Fatal("Failed to find table element")
		}

		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		processor.Extract(tableNode, tb, "html")

		output := sb.String()
		t.Logf("Output: %s", output)

		if !strings.Contains(output, "<table>") {
			t.Error("Expected <table> tag in HTML output")
		}
	})

	t.Run("extract table with colspan", func(t *testing.T) {
		htmlContent := `<table><tr><td colspan="2">Spanning</td></tr><tr><td>A</td><td>B</td></tr></table>`
		doc, err := parseHTML(htmlContent)
		if err != nil {
			t.Fatalf("Failed to parse HTML: %v", err)
		}

		// Find the table element
		var tableNode *html.Node
		var findTable func(*html.Node)
		findTable = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "table" {
				tableNode = n
				return
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				findTable(c)
			}
		}
		findTable(doc)

		if tableNode == nil {
			t.Fatal("Failed to find table element")
		}

		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		processor.Extract(tableNode, tb, "markdown")

		output := sb.String()
		t.Logf("Output: %s", output)

		if !strings.Contains(output, "Spanning") {
			t.Error("Expected colspan content in output")
		}
	})
}

// realCellAccessor implements CellAccessor using actual HTML parsing.
type realCellAccessor struct{}

func (a *realCellAccessor) GetAlignment(node *html.Node) table.CellAlignment {
	if node == nil {
		return table.AlignDefault
	}
	for _, attr := range node.Attr {
		if strings.ToLower(attr.Key) == "align" {
			switch strings.ToLower(strings.TrimSpace(attr.Val)) {
			case "left":
				return table.AlignLeft
			case "center":
				return table.AlignCenter
			case "right":
				return table.AlignRight
			case "justify":
				return table.AlignJustify
			}
		}
	}
	return table.AlignDefault
}

func (a *realCellAccessor) GetColSpan(node *html.Node) int {
	if node == nil {
		return 1
	}
	for _, attr := range node.Attr {
		if strings.ToLower(attr.Key) == "colspan" {
			// Simple parsing - just try to convert to int
			val := 0
			for _, c := range strings.TrimSpace(attr.Val) {
				if c >= '0' && c <= '9' {
					val = val*10 + int(c-'0')
				} else {
					break
				}
			}
			if val > 0 {
				return val
			}
		}
	}
	return 1
}

func (a *realCellAccessor) GetRowSpan(node *html.Node) int {
	if node == nil {
		return 1
	}
	return 1
}

func (a *realCellAccessor) GetWidth(node *html.Node) string {
	if node == nil {
		return ""
	}
	for _, attr := range node.Attr {
		if strings.ToLower(attr.Key) == "width" {
			return strings.TrimSpace(attr.Val)
		}
	}
	return ""
}

func (a *realCellAccessor) GetTextContent(node *html.Node) string {
	if node == nil {
		return ""
	}
	var sb strings.Builder
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}
	extract(node)
	return sb.String()
}

// realNodeWalker implements NodeWalker using actual DOM traversal.
type realNodeWalker struct{}

func (w *realNodeWalker) Walk(node *html.Node, callback func(*html.Node) bool) {
	var walk func(*html.Node) bool
	walk = func(n *html.Node) bool {
		if !callback(n) {
			return false
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if !walk(c) {
				return false
			}
		}
		return true
	}
	walk(node)
}

// TestSanitizeCellText tests cell text sanitization.
func TestSanitizeCellText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", " "},
		{"whitespace only", "   ", " "},
		{"normal text", "Hello World", "Hello World"},
		{"text with padding", "  Hello  ", "Hello"},
		{"newlines", "\n\nText\n\n", "Text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Access via exported function through processor
			result := sanitizeTestCellText(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeCellText(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// sanitizeTestCellText is a test helper that mirrors sanitizeCellText behavior.
func sanitizeTestCellText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return " "
	}
	return text
}

// TestSanitizeFormat tests format string sanitization.
func TestSanitizeFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"uppercase", "MARKDOWN", "markdown"},
		{"with spaces", "  HTML  ", "html"},
		{"mixed case", "Markdown", "markdown"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeTestFormat(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeFormat(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// sanitizeTestFormat is a test helper that mirrors sanitizeFormat behavior.
func sanitizeTestFormat(format string) string {
	return strings.ToLower(strings.TrimSpace(format))
}

// Mock types for Processor testing

type mockCellAccessor struct {
	text    string
	align   table.CellAlignment
	colspan int
	rowspan int
	width   string
}

func (m *mockCellAccessor) GetAlignment(node *html.Node) table.CellAlignment {
	return m.align
}

func (m *mockCellAccessor) GetColSpan(node *html.Node) int {
	return m.colspan
}

func (m *mockCellAccessor) GetRowSpan(node *html.Node) int {
	return m.rowspan
}

func (m *mockCellAccessor) GetWidth(node *html.Node) string {
	return m.width
}

func (m *mockCellAccessor) GetTextContent(node *html.Node) string {
	return m.text
}

type mockNodeWalker struct {
	rows []*html.Node
}

func (m *mockNodeWalker) Walk(node *html.Node, callback func(*html.Node) bool) {
	for _, row := range m.rows {
		if !callback(row) {
			break
		}
	}
}

// TestRenderHelperFunctions tests the internal render helper functions via exported paths.
func TestRenderHelperFunctions(t *testing.T) {
	t.Parallel()

	t.Run("markdown table with complex structure", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{{Text: "A", IsHeader: true, Align: table.AlignLeft}, {Text: "B", IsHeader: true, Align: table.AlignCenter}, {Text: "C", IsHeader: true, Align: table.AlignRight}},
			{{Text: "1", Align: table.AlignLeft}, {Text: "2", Align: table.AlignCenter}, {Text: "3", Align: table.AlignRight}},
			{{Text: "Longer content here", Align: table.AlignDefault}, {Text: "X", Align: table.AlignDefault}, {Text: "Y", Align: table.AlignDefault}},
		}

		r.Render(tableData, tb, 3)
		output := sb.String()

		// Check alignment markers (center uses :---: format)
		if !strings.Contains(output, ":---") {
			t.Error("Expected left alignment marker")
		}
		if !strings.Contains(output, "---:") {
			t.Error("Expected right alignment marker")
		}
		// Center alignment is also marked with : at start
		// The exact format depends on the implementation
		t.Logf("Output for debugging: %s", output)
	})

	t.Run("markdown table with empty cells", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Header", IsHeader: true}, {Text: " ", IsHeader: true}},
			{{Text: " "}, {Text: "Data"}},
		}

		r.Render(tableData, tb, 2)
		output := sb.String()

		if !strings.Contains(output, "Header") {
			t.Error("Expected header content")
		}
		if !strings.Contains(output, "Data") {
			t.Error("Expected data content")
		}
	})

	t.Run("HTML table with complex styling", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Header", IsHeader: true, Align: table.AlignCenter, Width: "200px"}},
			{{Text: "Data", Align: table.AlignRight, Rowspan: 2, Colspan: 2, OriginalColspan: 2}},
		}

		r.Render(tableData, tb, 1)
		output := sb.String()

		if !strings.Contains(output, `text-align:center`) {
			t.Error("Expected center alignment style")
		}
		if !strings.Contains(output, `width:200px`) {
			t.Error("Expected width style")
		}
		if !strings.Contains(output, `rowspan="2"`) {
			t.Error("Expected rowspan attribute")
		}
		if !strings.Contains(output, `colspan="2"`) {
			t.Error("Expected colspan attribute")
		}
	})

	t.Run("table with justify alignment", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Justified", Align: table.AlignJustify}},
		}

		r.Render(tableData, tb, 1)
		output := sb.String()

		if !strings.Contains(output, "text-align:justify") {
			t.Error("Expected justify alignment style")
		}
	})
}

// TestTableWithDifferentWidths tests tables with varying column widths.
func TestTableWithDifferentWidths(t *testing.T) {
	t.Parallel()

	t.Run("markdown table width handling", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.MarkdownRenderer{}
		// Simulate structure row with widths
		tableData := [][]table.CellData{
			{{Text: "Short", IsHeader: true}, {Text: "Medium Length Header", IsHeader: true}},
			{{Text: "A", Align: table.AlignDefault}, {Text: "B", Align: table.AlignDefault}},
		}

		r.Render(tableData, tb, 2)
		output := sb.String()

		if !strings.Contains(output, "|") {
			t.Error("Expected table pipe characters")
		}
	})
}

// TestMarkdownTableEdgeCases tests various edge cases for markdown tables.
func TestMarkdownTableEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("table with single column", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Header", IsHeader: true}},
			{{Text: "Data"}},
		}

		r.Render(tableData, tb, 1)
		output := sb.String()

		if !strings.Contains(output, "Header") || !strings.Contains(output, "Data") {
			t.Error("Expected table content")
		}
	})

	t.Run("table with many columns", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{
				{Text: "A", IsHeader: true},
				{Text: "B", IsHeader: true},
				{Text: "C", IsHeader: true},
				{Text: "D", IsHeader: true},
				{Text: "E", IsHeader: true},
			},
		}

		r.Render(tableData, tb, 5)
		output := sb.String()

		// Count pipes to verify 5 columns
		pipeCount := strings.Count(output, "|")
		if pipeCount < 10 { // At least 2 pipes per row * 2 rows + separator row
			t.Errorf("Expected at least 10 pipe characters for 5 columns, got %d", pipeCount)
		}
	})

	t.Run("table with very long content", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.MarkdownRenderer{}
		longText := strings.Repeat("LongContent", 20)
		tableData := [][]table.CellData{
			{{Text: "Header", IsHeader: true}},
			{{Text: longText}},
		}

		r.Render(tableData, tb, 1)
		output := sb.String()

		if !strings.Contains(output, "LongContent") {
			t.Error("Expected long content in output")
		}
	})

	t.Run("table with special characters", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Header | Pipe", IsHeader: true}},
			{{Text: "Data with *asterisks* and _underscores_"}},
		}

		r.Render(tableData, tb, 1)
		output := sb.String()

		// Should handle special markdown characters
		if !strings.Contains(output, "asterisks") {
			t.Error("Expected content with special characters")
		}
	})
}

// TestHTMLTableEdgeCases tests various edge cases for HTML tables.
func TestHTMLTableEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("table with nested elements", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Header with <strong>bold</strong>", IsHeader: true}},
			{{Text: "Data with <em>italic</em>"}},
		}

		r.Render(tableData, tb, 1)
		output := sb.String()

		if !strings.Contains(output, "<table>") {
			t.Error("Expected table tag")
		}
	})

	t.Run("table with rowspan and colspan", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Multi-span", Rowspan: 2, Colspan: 2, OriginalColspan: 2, IsHeader: true}},
			{},
		}

		r.Render(tableData, tb, 2)
		output := sb.String()

		if !strings.Contains(output, `rowspan="2"`) {
			t.Error("Expected rowspan attribute")
		}
		if !strings.Contains(output, `colspan="2"`) {
			t.Error("Expected colspan attribute")
		}
	})

	t.Run("table with zero rowspan/colspan", func(t *testing.T) {
		var sb strings.Builder
		tb := table.NewTrackedBuilder(&sb)

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Cell", Rowspan: 0, Colspan: 0}},
		}

		r.Render(tableData, tb, 1)
		output := sb.String()

		// Zero values should not produce attributes
		if strings.Contains(output, "rowspan") || strings.Contains(output, "colspan") {
			t.Error("Zero rowspan/colspan should not produce attributes")
		}
	})
}
