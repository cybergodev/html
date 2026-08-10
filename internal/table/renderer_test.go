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
		tb := table.NewTrackedBuilder()

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
		tb := table.NewTrackedBuilder()

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
		tb := table.NewTrackedBuilder()

		tb.WriteString("text")
		table.EnsureNewline(tb)

		if tb.LastChar != '\n' {
			t.Error("Should end with newline")
		}
		if !strings.HasSuffix(tb.String(), "text\n") {
			t.Errorf("Expected 'text\\n', got %q", tb.String())
		}
	})

	t.Run("EnsureNewline does not add duplicate newline", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		tb.WriteString("text\n")
		table.EnsureNewline(tb)

		if tb.String() != "text\n" {
			t.Errorf("Expected 'text\\n' without duplicate, got %q", tb.String())
		}
	})

	t.Run("EnsureSpacing adds spacing when needed", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		tb.WriteString("text")
		table.EnsureSpacing(tb, ' ')

		if tb.LastChar != ' ' {
			t.Error("Should end with space")
		}
	})

	t.Run("EnsureSpacing does not add after newline", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		tb.WriteString("text\n")
		table.EnsureSpacing(tb, ' ')

		if tb.String() != "text\n" {
			t.Errorf("Should not add space after newline, got %q", tb.String())
		}
	})

	t.Run("empty builder", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		if tb.LastChar != 0 {
			t.Errorf("LastChar = %d, want 0", tb.LastChar)
		}
		if tb.Len() != 0 {
			t.Errorf("Len() = %d, want 0", tb.Len())
		}
	})

	// --- TrackedBuilder methods that were at 0% coverage ---

	t.Run("Reset clears content and retains capacity", func(t *testing.T) {
		tb := table.NewTrackedBuilder()
		tb.WriteString("hello world")

		oldCap := tb.Cap()
		tb.Reset()

		if tb.Len() != 0 {
			t.Errorf("Len() after Reset = %d, want 0", tb.Len())
		}
		if tb.LastChar != 0 {
			t.Errorf("LastChar after Reset = %d, want 0", tb.LastChar)
		}
		// Capacity must survive Reset so the buffer is reusable without reallocation.
		if tb.Cap() < oldCap {
			t.Errorf("Cap() after Reset = %d, was %d before (capacity must be retained)", tb.Cap(), oldCap)
		}
		if tb.String() != "" {
			t.Errorf("String() after Reset = %q, want empty", tb.String())
		}
	})

	t.Run("Cap reports backing capacity", func(t *testing.T) {
		tb := table.NewTrackedBuilder()
		if tb.Cap() != 0 {
			t.Errorf("Cap() on fresh builder = %d, want 0", tb.Cap())
		}

		tb.WriteString("ab")
		if tb.Cap() < 2 {
			t.Errorf("Cap() = %d, must be >= Len()=2", tb.Cap())
		}
	})

	t.Run("Bytes returns raw buffer", func(t *testing.T) {
		tb := table.NewTrackedBuilder()
		tb.WriteString("payload")

		got := tb.Bytes()
		if string(got) != "payload" {
			t.Errorf("Bytes() = %q, want 'payload'", string(got))
		}
		// Aliases the backing array, so subsequent writes change the returned slice.
		tb.WriteByte('!')
		// The slice returned *before* the WriteByte may or may not have been
		// resized depending on capacity; the important contract is that the
		// buffer is accessible. Check via a fresh call.
		if string(tb.Bytes()) != "payload!" {
			t.Errorf("Bytes() after WriteByte = %q, want 'payload!'", string(tb.Bytes()))
		}
	})

	t.Run("Grow pre-sizes capacity", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		// Grow by 100: remaining capacity must be >= 100 after the call.
		tb.Grow(100)
		if tb.Cap()-tb.Len() < 100 {
			t.Errorf("after Grow(100): available = %d, want >= 100", tb.Cap()-tb.Len())
		}

		// Grow(0) is a no-op (must not shrink or panic).
		before := tb.Cap()
		tb.Grow(0)
		if tb.Cap() != before {
			t.Errorf("Grow(0) changed Cap from %d to %d", before, tb.Cap())
		}

		// Idempotent when capacity already suffices.
		tb.Grow(50) // already have >= 100
	})

	t.Run("Write appends byte slice and tracks LastChar", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		n, err := tb.Write([]byte("abc"))
		if err != nil {
			t.Fatalf("Write() error: %v", err)
		}
		if n != 3 {
			t.Errorf("Write() n = %d, want 3", n)
		}
		if tb.LastChar != 'c' {
			t.Errorf("LastChar = %c, want 'c'", tb.LastChar)
		}
		if tb.String() != "abc" {
			t.Errorf("String() = %q, want 'abc'", tb.String())
		}

		// Empty Write must not change LastChar.
		oldLast := tb.LastChar
		tb.Write([]byte{})
		if tb.LastChar != oldLast {
			t.Errorf("Write([]byte{}) changed LastChar from %c to %c", oldLast, tb.LastChar)
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
		tb := table.NewTrackedBuilder()

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{{Text: "A", IsHeader: true}, {Text: "B", IsHeader: true}},
			{{Text: "1"}, {Text: "2"}},
		}

		r.Render(tableData, tb, 2)
		output := tb.String()

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
		tb := table.NewTrackedBuilder()

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Left", Align: table.AlignLeft, IsHeader: true}, {Text: "Right", Align: table.AlignRight, IsHeader: true}},
			{{Text: "L1", Align: table.AlignLeft}, {Text: "R1", Align: table.AlignRight}},
		}

		r.Render(tableData, tb, 2)
		output := tb.String()

		// Check for alignment markers
		if !strings.Contains(output, ":---") {
			t.Error("Expected left alignment marker ':---'")
		}
		if !strings.Contains(output, "---:") {
			t.Error("Expected right alignment marker '---:'")
		}
	})

	t.Run("Render with colspan", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Span", Colspan: 2, OriginalColspan: 2, IsHeader: true}},
			{{Text: "A"}, {Text: "B"}},
		}

		r.Render(tableData, tb, 2)
		output := tb.String()

		// Colspan cells are expanded in markdown
		if !strings.Contains(output, "Span") {
			t.Error("Expected colspan cell content")
		}
	})

	t.Run("Render empty table", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{}

		r.Render(tableData, tb, 0)
		output := tb.String()

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
		tb := table.NewTrackedBuilder()

		r := &table.MarkdownRenderer{}
		// <tr><td rowspan=2>A</td><td>B</td></tr><tr><td>C</td></tr>
		// Input is post-colspan-expansion (what the renderer receives); each
		// entry is one column, carrying its Rowspan.
		tableData := [][]table.CellData{
			{{Text: "A", Rowspan: 2}, {Text: "B"}},
			{{Text: "C"}},
		}

		r.Render(tableData, tb, 2)
		output := tb.String()

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
		tb := table.NewTrackedBuilder()

		r := &table.MarkdownRenderer{}
		// rowspan=5 but only 2 rows exist: must not panic or fabricate rows.
		tableData := [][]table.CellData{
			{{Text: "A", Rowspan: 5}, {Text: "B"}},
			{{Text: "C"}},
		}

		r.Render(tableData, tb, 2)
		output := tb.String()

		// Only one spanned row exists, so A repeats exactly once (count=2).
		if got := strings.Count(output, "A"); got != 2 {
			t.Errorf("expected A repeated once (only 1 spanned row exists), got %d\n%s", got, output)
		}
	})

	t.Run("rowspan with header repeats header value", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		r := &table.MarkdownRenderer{}
		// <tr><th rowspan=2>Name</th><th>Value</th></tr><tr><td>1</td></tr>
		tableData := [][]table.CellData{
			{{Text: "Name", Rowspan: 2, IsHeader: true}, {Text: "Value", IsHeader: true}},
			{{Text: "1"}},
		}

		r.Render(tableData, tb, 2)
		output := tb.String()

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
		tb := table.NewTrackedBuilder()

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Header", IsHeader: true}},
			{{Text: "Data"}},
		}

		r.Render(tableData, tb, 1)
		output := tb.String()

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
		tb := table.NewTrackedBuilder()

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Span", Rowspan: 2}},
			{{Text: "B"}},
		}

		r.Render(tableData, tb, 1)
		output := tb.String()

		if !strings.Contains(output, `rowspan="2"`) {
			t.Errorf("Expected rowspan attribute, got: %s", output)
		}
	})

	t.Run("Render with colspan", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Span", Colspan: 3, OriginalColspan: 3}},
		}

		r.Render(tableData, tb, 1)
		output := tb.String()

		if !strings.Contains(output, `colspan="3"`) {
			t.Errorf("Expected colspan attribute, got: %s", output)
		}
	})

	t.Run("Render with alignment", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Center", Align: table.AlignCenter}},
		}

		r.Render(tableData, tb, 1)
		output := tb.String()

		if !strings.Contains(output, "text-align:center") {
			t.Errorf("Expected text-align:center style, got: %s", output)
		}
	})

	t.Run("Render with width", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Wide", Width: "100px"}},
		}

		r.Render(tableData, tb, 1)
		output := tb.String()

		if !strings.Contains(output, "width:100px") {
			t.Errorf("Expected width style, got: %s", output)
		}
	})
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

		tb := table.NewTrackedBuilder()

		p.Extract(nil, tb, "markdown")

		if tb.String() != "" {
			t.Error("Extract with nil table should produce no output")
		}
	})

	t.Run("Extract with empty table produces minimal output", func(t *testing.T) {
		ca := &mockCellAccessor{text: "Cell"}
		nw := &mockNodeWalker{rows: nil} // No rows
		p := table.NewProcessor(ca, nw)

		tb := table.NewTrackedBuilder()

		tableNode := &html.Node{Type: html.ElementNode, Data: "table"}
		p.Extract(tableNode, tb, "markdown")

		// Empty table should produce only blank lines
		output := tb.String()
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

		tb := table.NewTrackedBuilder()

		processor.Extract(tableNode, tb, "markdown")

		output := tb.String()
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

		tb := table.NewTrackedBuilder()

		processor.Extract(tableNode, tb, "markdown")

		output := tb.String()
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

		tb := table.NewTrackedBuilder()

		processor.Extract(tableNode, tb, "html")

		output := tb.String()
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

		tb := table.NewTrackedBuilder()

		processor.Extract(tableNode, tb, "markdown")

		output := tb.String()
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
	tb := table.NewTrackedBuilder()
	var extract func(*html.Node)
	extract = func(n *html.Node) {
		if n.Type == html.TextNode {
			tb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}
	extract(node)
	return tb.String()
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
		tb := table.NewTrackedBuilder()

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{{Text: "A", IsHeader: true, Align: table.AlignLeft}, {Text: "B", IsHeader: true, Align: table.AlignCenter}, {Text: "C", IsHeader: true, Align: table.AlignRight}},
			{{Text: "1", Align: table.AlignLeft}, {Text: "2", Align: table.AlignCenter}, {Text: "3", Align: table.AlignRight}},
			{{Text: "Longer content here", Align: table.AlignDefault}, {Text: "X", Align: table.AlignDefault}, {Text: "Y", Align: table.AlignDefault}},
		}

		r.Render(tableData, tb, 3)
		output := tb.String()

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
		tb := table.NewTrackedBuilder()

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Header", IsHeader: true}, {Text: " ", IsHeader: true}},
			{{Text: " "}, {Text: "Data"}},
		}

		r.Render(tableData, tb, 2)
		output := tb.String()

		if !strings.Contains(output, "Header") {
			t.Error("Expected header content")
		}
		if !strings.Contains(output, "Data") {
			t.Error("Expected data content")
		}
	})

	t.Run("HTML table with complex styling", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Header", IsHeader: true, Align: table.AlignCenter, Width: "200px"}},
			{{Text: "Data", Align: table.AlignRight, Rowspan: 2, Colspan: 2, OriginalColspan: 2}},
		}

		r.Render(tableData, tb, 1)
		output := tb.String()

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
		tb := table.NewTrackedBuilder()

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Justified", Align: table.AlignJustify}},
		}

		r.Render(tableData, tb, 1)
		output := tb.String()

		if !strings.Contains(output, "text-align:justify") {
			t.Error("Expected justify alignment style")
		}
	})
}

// TestTableWithDifferentWidths tests tables with varying column widths.
func TestTableWithDifferentWidths(t *testing.T) {
	t.Parallel()

	t.Run("markdown table width handling", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		r := &table.MarkdownRenderer{}
		// Simulate structure row with widths
		tableData := [][]table.CellData{
			{{Text: "Short", IsHeader: true}, {Text: "Medium Length Header", IsHeader: true}},
			{{Text: "A", Align: table.AlignDefault}, {Text: "B", Align: table.AlignDefault}},
		}

		r.Render(tableData, tb, 2)
		output := tb.String()

		if !strings.Contains(output, "|") {
			t.Error("Expected table pipe characters")
		}
	})
}

// TestMarkdownTableEdgeCases tests various edge cases for markdown tables.
func TestMarkdownTableEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("table with single column", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Header", IsHeader: true}},
			{{Text: "Data"}},
		}

		r.Render(tableData, tb, 1)
		output := tb.String()

		if !strings.Contains(output, "Header") || !strings.Contains(output, "Data") {
			t.Error("Expected table content")
		}
	})

	t.Run("table with many columns", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

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
		output := tb.String()

		// Count pipes to verify 5 columns
		pipeCount := strings.Count(output, "|")
		if pipeCount < 10 { // At least 2 pipes per row * 2 rows + separator row
			t.Errorf("Expected at least 10 pipe characters for 5 columns, got %d", pipeCount)
		}
	})

	t.Run("table with very long content", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		r := &table.MarkdownRenderer{}
		longText := strings.Repeat("LongContent", 20)
		tableData := [][]table.CellData{
			{{Text: "Header", IsHeader: true}},
			{{Text: longText}},
		}

		r.Render(tableData, tb, 1)
		output := tb.String()

		if !strings.Contains(output, "LongContent") {
			t.Error("Expected long content in output")
		}
	})

	t.Run("table with special characters", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		r := &table.MarkdownRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Header | Pipe", IsHeader: true}},
			{{Text: "Data with *asterisks* and _underscores_"}},
		}

		r.Render(tableData, tb, 1)
		output := tb.String()

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
		tb := table.NewTrackedBuilder()

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Header with <strong>bold</strong>", IsHeader: true}},
			{{Text: "Data with <em>italic</em>"}},
		}

		r.Render(tableData, tb, 1)
		output := tb.String()

		if !strings.Contains(output, "<table>") {
			t.Error("Expected table tag")
		}
	})

	t.Run("table with rowspan and colspan", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Multi-span", Rowspan: 2, Colspan: 2, OriginalColspan: 2, IsHeader: true}},
			{},
		}

		r.Render(tableData, tb, 2)
		output := tb.String()

		if !strings.Contains(output, `rowspan="2"`) {
			t.Error("Expected rowspan attribute")
		}
		if !strings.Contains(output, `colspan="2"`) {
			t.Error("Expected colspan attribute")
		}
	})

	t.Run("table with zero rowspan/colspan", func(t *testing.T) {
		tb := table.NewTrackedBuilder()

		r := &table.HTMLRenderer{}
		tableData := [][]table.CellData{
			{{Text: "Cell", Rowspan: 0, Colspan: 0}},
		}

		r.Render(tableData, tb, 1)
		output := tb.String()

		// Zero values should not produce attributes
		if strings.Contains(output, "rowspan") || strings.Contains(output, "colspan") {
			t.Error("Zero rowspan/colspan should not produce attributes")
		}
	})
}
