package internal

import (
	"strings"
	"testing"

	"github.com/cybergodev/html/internal/table"
	stdxhtml "golang.org/x/net/html"
)

func TestContainsWord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		word string
		want bool
	}{
		{
			name: "word at start",
			text: "text-align:left",
			word: "text-align",
			want: true,
		},
		{
			name: "word in middle",
			text: "display:block;text-align:center",
			word: "text-align",
			want: true,
		},
		{
			name: "word at end",
			text: "color:red;text-align:left",
			word: "left",
			want: true,
		},
		{
			name: "partial match should fail",
			text: "textalign:left",
			word: "text-align",
			want: false,
		},
		{
			name: "word without boundary",
			text: "mytext-align:left",
			word: "text",
			want: false,
		},
		{
			name: "hyphen is NOT a word boundary for align",
			text: "text-align:left",
			word: "align",
			want: false, // Hyphen before align means text-align is a single word
		},
		{
			name: "with space boundary",
			text: "display block text",
			word: "text",
			want: true,
		},
		{
			name: "with semicolon boundary",
			text: "display:block;text",
			word: "text",
			want: true,
		},
		{
			name: "with quote boundary",
			text: `"text-align:left"`,
			word: "left",
			want: true,
		},
		{
			name: "empty text",
			text: "",
			word: "test",
			want: false,
		},
		{
			name: "empty word",
			text: "test",
			word: "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsWord(tt.text, tt.word)
			if result != tt.want {
				t.Errorf("containsWord(%q, %q) = %v, want %v", tt.text, tt.word, result, tt.want)
			}
		})
	}
}

func TestGetCellAlign(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		html      string
		wantAlign table.CellAlignment
	}{
		{
			name:      "align attribute left",
			html:      `<table><tr><td align="left">Text</td></tr></table>`,
			wantAlign: table.AlignLeft,
		},
		{
			name:      "align attribute center",
			html:      `<table><tr><td align="center">Text</td></tr></table>`,
			wantAlign: table.AlignCenter,
		},
		{
			name:      "align attribute right",
			html:      `<table><tr><td align="right">Text</td></tr></table>`,
			wantAlign: table.AlignRight,
		},
		{
			name:      "align attribute justify",
			html:      `<table><tr><td align="justify">Text</td></tr></table>`,
			wantAlign: table.AlignJustify,
		},
		{
			name:      "style attribute text-align",
			html:      `<table><tr><td style="text-align:center">Text</td></tr></table>`,
			wantAlign: table.AlignCenter,
		},
		{
			name:      "style with colon space",
			html:      `<table><tr><td style="text-align: center">Text</td></tr></table>`,
			wantAlign: table.AlignCenter,
		},
		{
			name:      "align takes precedence over style",
			html:      `<table><tr><td align="left" style="text-align:center">Text</td></tr></table>`,
			wantAlign: table.AlignLeft,
		},
		{
			name:      "no alignment specified",
			html:      `<table><tr><td>Text</td></tr></table>`,
			wantAlign: table.AlignDefault,
		},
		{
			name:      "uppercase style",
			html:      `<table><tr><td style="TEXT-ALIGN:CENTER">Text</td></tr></table>`,
			wantAlign: table.AlignCenter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := stdxhtml.Parse(strings.NewReader(tt.html))
			var td *stdxhtml.Node
			WalkNodes(doc, func(n *stdxhtml.Node) bool {
				if n.Type == stdxhtml.ElementNode && n.Data == "td" {
					td = n
					return false
				}
				return true
			})

			if td == nil {
				t.Fatal("Could not find td element")
			}

			result := getCellAlign(td)
			if result != tt.wantAlign {
				t.Errorf("getCellAlign() = %v, want %v", result, tt.wantAlign)
			}
		})
	}
}

func TestGetColSpan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		html     string
		wantSpan int
	}{
		{
			name:     "default colspan",
			html:     `<table><tr><td>Text</td></tr></table>`,
			wantSpan: 1,
		},
		{
			name:     "colspan 2",
			html:     `<table><tr><td colspan="2">Text</td></tr></table>`,
			wantSpan: 2,
		},
		{
			name:     "colspan 5",
			html:     `<table><tr><td colspan="5">Text</td></tr></table>`,
			wantSpan: 5,
		},
		{
			name:     "invalid colspan defaults to 1",
			html:     `<table><tr><td colspan="0">Text</td></tr></table>`,
			wantSpan: 1,
		},
		{
			name:     "negative colspan defaults to 1",
			html:     `<table><tr><td colspan="-1">Text</td></tr></table>`,
			wantSpan: 1,
		},
		{
			name:     "non-numeric colspan defaults to 1",
			html:     `<table><tr><td colspan="abc">Text</td></tr></table>`,
			wantSpan: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := stdxhtml.Parse(strings.NewReader(tt.html))
			var td *stdxhtml.Node
			WalkNodes(doc, func(n *stdxhtml.Node) bool {
				if n.Type == stdxhtml.ElementNode && n.Data == "td" {
					td = n
					return false
				}
				return true
			})

			if td == nil {
				t.Fatal("Could not find td element")
			}

			result := getColSpan(td)
			if result != tt.wantSpan {
				t.Errorf("getColSpan() = %d, want %d", result, tt.wantSpan)
			}
		})
	}
}

func TestGetRowSpan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		html     string
		wantSpan int
	}{
		{
			name:     "default rowspan",
			html:     `<table><tr><td>Text</td></tr></table>`,
			wantSpan: 1,
		},
		{
			name:     "rowspan 2",
			html:     `<table><tr><td rowspan="2">Text</td></tr></table>`,
			wantSpan: 2,
		},
		{
			name:     "rowspan 3",
			html:     `<table><tr><td rowspan="3">Text</td></tr></table>`,
			wantSpan: 3,
		},
		{
			name:     "invalid rowspan defaults to 1",
			html:     `<table><tr><td rowspan="0">Text</td></tr></table>`,
			wantSpan: 1,
		},
		{
			name:     "non-numeric rowspan defaults to 1",
			html:     `<table><tr><td rowspan="abc">Text</td></tr></table>`,
			wantSpan: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := stdxhtml.Parse(strings.NewReader(tt.html))
			var td *stdxhtml.Node
			WalkNodes(doc, func(n *stdxhtml.Node) bool {
				if n.Type == stdxhtml.ElementNode && n.Data == "td" {
					td = n
					return false
				}
				return true
			})

			if td == nil {
				t.Fatal("Could not find td element")
			}

			result := getRowSpan(td)
			if result != tt.wantSpan {
				t.Errorf("getRowSpan() = %d, want %d", result, tt.wantSpan)
			}
		})
	}
}

func TestGetCellWidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		html    string
		want    string
		wantNil bool
	}{
		{
			name:    "width attribute pixels",
			html:    `<table><tr><td width="100">Text</td></tr></table>`,
			want:    "100",
			wantNil: false,
		},
		{
			name:    "width attribute percent",
			html:    `<table><tr><td width="50%">Text</td></tr></table>`,
			want:    "50%",
			wantNil: false,
		},
		{
			name:    "style width pixels",
			html:    `<table><tr><td style="width:200px">Text</td></tr></table>`,
			want:    "200px",
			wantNil: false,
		},
		{
			name:    "style width percent",
			html:    `<table><tr><td style="width:25%">Text</td></tr></table>`,
			want:    "25%",
			wantNil: false,
		},
		{
			name:    "width attribute zero returns empty",
			html:    `<table><tr><td width="0">Text</td></tr></table>`,
			want:    "",
			wantNil: true,
		},
		{
			name:    "no width specified",
			html:    `<table><tr><td>Text</td></tr></table>`,
			want:    "",
			wantNil: true,
		},
		{
			name:    "width attribute takes precedence",
			html:    `<table><tr><td width="100" style="width:200px">Text</td></tr></table>`,
			want:    "100",
			wantNil: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := stdxhtml.Parse(strings.NewReader(tt.html))
			var td *stdxhtml.Node
			WalkNodes(doc, func(n *stdxhtml.Node) bool {
				if n.Type == stdxhtml.ElementNode && n.Data == "td" {
					td = n
					return false
				}
				return true
			})

			if td == nil {
				t.Fatal("Could not find td element")
			}

			result := getCellWidth(td)
			if result != tt.want {
				t.Errorf("getCellWidth() = %q, want %q", result, tt.want)
			}
		})
	}
}

func TestCellAlignmentConstantsUnique(t *testing.T) {
	t.Parallel()

	// All exported CellAlignment constants must be distinct so table renderers
	// can dispatch on the value unambiguously.
	aligns := []table.CellAlignment{
		table.AlignLeft,
		table.AlignCenter,
		table.AlignRight,
		table.AlignJustify,
		table.AlignDefault,
	}
	seen := make(map[table.CellAlignment]bool, len(aligns))
	for _, a := range aligns {
		if seen[a] {
			t.Errorf("CellAlignment value %d is not unique among constants", a)
		}
		seen[a] = true
	}
}

// ============================================================================
// TABLE COLUMN WIDTH TESTS
// ============================================================================

func TestCollectColumnWidths(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		html        string
		minCells    int
		description string
	}{
		{
			name: "table with width definitions",
			html: `
				<table>
					<tr>
						<td width="100px">Cell 1</td>
						<td width="200px">Cell 2</td>
						<td width="150px">Cell 3</td>
					</tr>
				</table>
			`,
			minCells:    3,
			description: "Should extract row with 3 cells",
		},
		{
			name: "table without width definitions",
			html: `
				<table>
					<tr>
						<td>Cell 1</td>
						<td>Cell 2</td>
					</tr>
				</table>
			`,
			minCells:    2,
			description: "Should extract row with 2 cells",
		},
		{
			name: "mixed widths and percentages",
			html: `
				<table>
					<tr>
						<td width="100px">A</td>
						<td width="50%">B</td>
						<td>C</td>
					</tr>
				</table>
			`,
			minCells:    3,
			description: "Should extract row with 3 cells",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := stdxhtml.Parse(strings.NewReader(tc.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			// Find table
			table := FindElementByTag(doc, "table")
			if table == nil {
				t.Fatal("Table not found")
			}

			// Use the public API to extract table content
			var sb strings.Builder
			ExtractTextWithStructureAndImages(table, &sb, nil, nil, "markdown")

			// Verify content was extracted
			result := sb.String()
			if result == "" {
				t.Error("No table content extracted")
			}

			// Count cells by counting | characters in the output
			cellCount := strings.Count(result, "|") / 2 // Each cell has | before and after
			if cellCount < tc.minCells {
				t.Errorf("%s: extracted %d cells, want at least %d", tc.description, cellCount, tc.minCells)
			}
		})
	}
}

func TestTableStructureRowDetection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		html         string
		expectedRows int
		description  string
	}{
		{
			name: "normal data rows",
			html: `
				<table>
					<tr><td>Data 1</td><td>Data 2</td></tr>
					<tr><td>Data 3</td><td>Data 4</td></tr>
				</table>
			`,
			expectedRows: 2,
			description:  "All rows are data rows",
		},
		{
			name: "structure row with widths",
			html: `
				<table>
					<tr><td width="100px"></td><td width="200px"></td></tr>
					<tr><td>Data 1</td><td>Data 2</td></tr>
				</table>
			`,
			expectedRows: 1, // Only data row (structure row excluded)
			description:  "Structure row excluded from Markdown output",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := stdxhtml.Parse(strings.NewReader(tc.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			table := FindElementByTag(doc, "table")
			if table == nil {
				t.Fatal("Table not found")
			}

			// Use the public API to extract table content
			var sb strings.Builder
			ExtractTextWithStructureAndImages(table, &sb, nil, nil, "markdown")

			// Count rows by counting lines with | characters
			result := sb.String()
			lines := strings.Split(strings.TrimSpace(result), "\n")
			// isBlankPipeRow detects a row whose cells are all empty (only pipes
			// and whitespace), e.g. the synthesized header row emitted for
			// headerless tables. Such rows are not data rows and must not be
			// counted as such.
			isBlankPipeRow := func(line string) bool {
				if !strings.Contains(line, "|") {
					return false
				}
				return strings.Trim(line, "| \t") == ""
			}
			dataRowCount := 0
			for _, line := range lines {
				if isBlankPipeRow(line) {
					continue
				}
				if strings.Contains(line, "|") && !strings.Contains(line, "| ---") && !strings.Contains(line, "|:--") && !strings.Contains(line, "|---:") {
					dataRowCount++
				}
			}

			if dataRowCount != tc.expectedRows {
				t.Errorf("%s: extracted %d data rows, want %d", tc.description, dataRowCount, tc.expectedRows)
			}
		})
	}
}

func TestTableCellWidthParsing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		html          string
		expectedWidth string
	}{
		{
			name:          "pixel width",
			html:          `<table><tr><td width="100px">Cell</td></tr></table>`,
			expectedWidth: "100px",
		},
		{
			name:          "percentage width",
			html:          `<table><tr><td width="50%">Cell</td></tr></table>`,
			expectedWidth: "50%",
		},
		{
			name:          "width in style attribute",
			html:          `<table><tr><td style="width: 200px">Cell</td></tr></table>`,
			expectedWidth: "200px",
		},
		{
			name:          "no width",
			html:          `<table><tr><td>Cell</td></tr></table>`,
			expectedWidth: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := stdxhtml.Parse(strings.NewReader(tc.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			// Find the td element
			var td *stdxhtml.Node
			WalkNodes(doc, func(n *stdxhtml.Node) bool {
				if n.Type == stdxhtml.ElementNode && n.Data == "td" {
					td = n
					return false // Stop after finding first TD
				}
				return true
			})

			if td == nil {
				t.Fatal("TD element not found")
			}

			width := getCellWidth(td)
			if width != tc.expectedWidth {
				t.Errorf("getCellWidth() = %q, want %q", width, tc.expectedWidth)
			}
		})
	}
}

func TestTableColspanExpansion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		html            string
		expectedContent string
		description     string
	}{
		{
			name: "single colspan",
			html: `
				<table>
					<tr><td colspan="2">Spans 2</td><td>Normal</td></tr>
				</table>
			`,
			expectedContent: "Spans 2",
			description:     "Colspan content is preserved",
		},
		{
			name: "multiple colspan",
			html: `
				<table>
					<tr><td colspan="3">Spans 3</td></tr>
				</table>
			`,
			expectedContent: "Spans 3",
			description:     "Multiple colspan content is preserved",
		},
		{
			name: "no colspan",
			html: `
				<table>
					<tr><td>A</td><td>B</td><td>C</td></tr>
				</table>
			`,
			expectedContent: "A",
			description:     "All cells are normal",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := stdxhtml.Parse(strings.NewReader(tc.html))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			table := FindElementByTag(doc, "table")
			if table == nil {
				t.Fatal("Table not found")
			}

			// Use the public API to extract table content
			var sb strings.Builder
			ExtractTextWithStructureAndImages(table, &sb, nil, nil, "markdown")

			// Verify the expected content is in the output
			result := sb.String()
			if !strings.Contains(result, tc.expectedContent) {
				t.Errorf("%s: expected content %q not found in output %q", tc.description, tc.expectedContent, result)
			}
		})
	}
}
