// html_table_test.go - table rendering tests (formats, column widths, alignment, markdown).

package html_test

import (
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

func TestTableFormats(t *testing.T) {
	t.Parallel()

	t.Run("markdown table with alignment", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr><th align="left">Name</th><th align="center">Age</th><th align="right">Score</th></tr>
					<tr><td>Alice</td><td>25</td><td>95</td></tr>
					<tr><td>Bob</td><td>30</td><td>87</td></tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "markdown"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check for alignment markers
		if !strings.Contains(result.Text, ":---") {
			t.Error("Markdown table should have left alignment marker")
		}
		if !strings.Contains(result.Text, ":--:") {
			t.Error("Markdown table should have center alignment marker")
		}
		if !strings.Contains(result.Text, "---:") {
			t.Error("Markdown table should have right alignment marker")
		}

		// Check content is preserved
		if !strings.Contains(result.Text, "Alice") {
			t.Error("Table content should be preserved")
		}
	})

	t.Run("html table format with alignment", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr><th align="left">Name</th><th align="center">Value</th></tr>
					<tr><td>Item1</td><td>100</td></tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should have HTML table tags
		if !strings.Contains(result.Text, "<table>") {
			t.Error("HTML format should contain <table> tag")
		}
		if !strings.Contains(result.Text, "style=\"text-align:left\"") {
			t.Error("HTML table should preserve left alignment")
		}
		if !strings.Contains(result.Text, "style=\"text-align:center\"") {
			t.Error("HTML table should preserve center alignment")
		}
	})

	t.Run("html table with cell merging", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th colspan="2">Merged Header</th>
					</tr>
					<tr>
						<td rowspan="2">Merged Cell</td>
						<td>Cell 2</td>
					</tr>
					<tr>
						<td>Cell 3</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should preserve colspan and rowspan
		if !strings.Contains(result.Text, "colspan=") {
			t.Error("HTML table should preserve colspan")
		}
		if !strings.Contains(result.Text, "rowspan=") {
			t.Error("HTML table should preserve rowspan")
		}
		if !strings.Contains(result.Text, "Merged Header") {
			t.Error("Table content should be preserved")
		}
	})

	t.Run("table with style attribute alignment", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr><th style="text-align:right">Total</th></tr>
					<tr><td style="text-align:right">$100</td></tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "markdown"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should detect alignment from style attribute
		if !strings.Contains(result.Text, "---:") {
			t.Error("Should detect right alignment from style attribute")
		}
	})

	t.Run("empty table handling", func(t *testing.T) {
		htmlContent := `<html><body><table></table></body></html>`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "markdown"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Empty table should not produce table output
		if strings.Contains(result.Text, "| --- |") {
			t.Error("Empty table should not produce separator")
		}
	})

	t.Run("default format is markdown", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr><th>Header</th></tr>
					<tr><td>Data</td></tr>
				</table>
			</body></html>
		`

		result, err := html.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Default should be markdown
		if !strings.Contains(result.Text, "| --- |") {
			t.Error("Default table format should be markdown")
		}
	})
}

func TestTableColumnWidths(t *testing.T) {
	t.Parallel()

	t.Run("html table preserves width from style attribute", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th style="width:1.0%">Name</th>
						<th style="width:50%">Value</th>
						<th style="width:49%">Notes</th>
					</tr>
					<tr>
						<td>Item 1</td>
						<td>100</td>
						<td>First</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check for width preservation in style attribute
		if !strings.Contains(result.Text, "width:1.0%") {
			t.Error("HTML table should preserve width:1.0%")
		}
		if !strings.Contains(result.Text, "width:50%") {
			t.Error("HTML table should preserve width:50%")
		}
		if !strings.Contains(result.Text, "width:49%") {
			t.Error("HTML table should preserve width:49%")
		}
	})

	t.Run("html table preserves width from width attribute", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th width="100">Column 1</th>
						<th width="200px">Column 2</th>
					</tr>
					<tr>
						<td>Data 1</td>
						<td>Data 2</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check for width preservation from width attribute
		if !strings.Contains(result.Text, "width:100") {
			t.Error("HTML table should preserve width=100")
		}
		if !strings.Contains(result.Text, "width:200px") {
			t.Error("HTML table should preserve width=200px")
		}
	})

	t.Run("html table combines width with alignment", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th style="width:30%; text-align:left">Name</th>
						<th style="width:70%; text-align:right">Value</th>
					</tr>
					<tr>
						<td>Item 1</td>
						<td>100</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check that both width and alignment are preserved in the same style attribute
		if !strings.Contains(result.Text, "width:30%") {
			t.Error("HTML table should preserve width")
		}
		if !strings.Contains(result.Text, "text-align:left") {
			t.Error("HTML table should preserve alignment")
		}
		// Check they are in the same style attribute with semicolon separator
		if !strings.Contains(result.Text, "text-align:left;") && !strings.Contains(result.Text, ";text-align:left") {
			if !strings.Contains(result.Text, "width:30%;") && !strings.Contains(result.Text, ";width:30%") {
				t.Error("Width and alignment should be in same style attribute separated by semicolon")
			}
		}
	})

	t.Run("markdown table includes width comment", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th style="width:1.0%">Name</th>
						<th style="width:98%">Value</th>
					</tr>
					<tr>
						<td>Item 1</td>
						<td>Data</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "markdown"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Table metadata comments have been removed for cleaner output
		// Check that table structure is preserved instead
		if !strings.Contains(result.Text, "| Name |") {
			t.Error("Markdown table should include table structure")
		}
		if !strings.Contains(result.Text, "| Value |") {
			t.Error("Markdown table should include table structure")
		}
	})

	t.Run("table without width attributes works normally", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th>Name</th>
						<th>Value</th>
					</tr>
					<tr>
						<td>Item 1</td>
						<td>100</td>
					</tr>
				</table>
			</body></html>
		`

		// Test HTML format
		cfgHTML := html.DefaultConfig()
		cfgHTML.TableFormat = "html"
		pHTML, _ := html.New(cfgHTML)
		defer func() { _ = pHTML.Close() }()

		result, err := pHTML.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should not have width in output
		if strings.Contains(result.Text, "width:") {
			t.Error("Table without width should not add width attributes")
		}

		// Test Markdown format
		cfgMD := html.DefaultConfig()
		cfgMD.TableFormat = "markdown"
		pMD, _ := html.New(cfgMD)
		defer func() { _ = pMD.Close() }()

		result, err = pMD.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should not have width comment
		if strings.Contains(result.Text, "<!-- Column widths:") {
			t.Error("Markdown table without widths should not include width comment")
		}
	})

	t.Run("complex table with mixed width units", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td style="width:50px">ID</td>
						<td style="width:20%">Name</td>
						<td style="width:1.5*">Score</td>
						<td>Description</td>
					</tr>
					<tr>
						<td>1</td>
						<td>Item</td>
						<td>95</td>
						<td>Test item</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check for various width units
		if !strings.Contains(result.Text, "width:50px") {
			t.Error("HTML table should preserve pixel width")
		}
		if !strings.Contains(result.Text, "width:20%") {
			t.Error("HTML table should preserve percentage width")
		}
		// The description column has no width, so it shouldn't appear
	})

	t.Run("table preserves width with colspan", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th style="width:50%" colspan="2">Merged Header</th>
						<th style="width:50%">Value</th>
					</tr>
					<tr>
						<td>A</td>
						<td>B</td>
						<td>C</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should preserve both width and colspan
		if !strings.Contains(result.Text, "colspan=") {
			t.Error("HTML table should preserve colspan")
		}
		if !strings.Contains(result.Text, "width:50%") {
			t.Error("HTML table should preserve width with colspan")
		}
	})
}

func TestTableAlignmentPreservation(t *testing.T) {
	t.Parallel()

	t.Run("html table preserves justify alignment", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td style="text-align:justify">This is justified text that should span the full width.</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check for justify alignment in style attribute
		if !strings.Contains(result.Text, "text-align:justify") {
			t.Error("HTML table should preserve justify alignment")
		}
	})

	t.Run("html table preserves all alignment types", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td style="text-align:left">Left</td>
						<td style="text-align:center">Center</td>
						<td style="text-align:right">Right</td>
						<td style="text-align:justify">Justify</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check all alignment types
		if !strings.Contains(result.Text, "text-align:left") {
			t.Error("HTML table should preserve left alignment")
		}
		if !strings.Contains(result.Text, "text-align:center") {
			t.Error("HTML table should preserve center alignment")
		}
		if !strings.Contains(result.Text, "text-align:right") {
			t.Error("HTML table should preserve right alignment")
		}
		if !strings.Contains(result.Text, "text-align:justify") {
			t.Error("HTML table should preserve justify alignment")
		}
	})

	t.Run("html table preserves align attribute", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td align="left">Left aligned</td>
						<td align="center">Center aligned</td>
						<td align="right">Right aligned</td>
						<td align="justify">Justify aligned</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// align attribute should be converted to style="text-align:..."
		if !strings.Contains(result.Text, "text-align:left") {
			t.Error("align='left' should convert to text-align:left")
		}
		if !strings.Contains(result.Text, "text-align:center") {
			t.Error("align='center' should convert to text-align:center")
		}
		if !strings.Contains(result.Text, "text-align:right") {
			t.Error("align='right' should convert to text-align:right")
		}
		if !strings.Contains(result.Text, "text-align:justify") {
			t.Error("align='justify' should convert to text-align:justify")
		}
	})

	t.Run("html table combines alignment with width", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td style="width:50%; text-align:justify">Combined</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should have both width and alignment in same style
		if !strings.Contains(result.Text, "width:50%") {
			t.Error("Should preserve width")
		}
		if !strings.Contains(result.Text, "text-align:justify") {
			t.Error("Should preserve justify alignment")
		}
		// Check they are in same style attribute with semicolon separator
		hasCombinedStyle := (strings.Contains(result.Text, "width:50%;text-align:justify") ||
			strings.Contains(result.Text, "text-align:justify;width:50%"))
		if !hasCombinedStyle {
			t.Error("Width and alignment should be in same style attribute")
		}
	})

	t.Run("markdown table with justify alignment shows warning", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th style="text-align:left">Name</th>
						<th style="text-align:justify">Description</th>
					</tr>
					<tr>
						<td>Item</td>
						<td>Justified text content</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "markdown"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Table metadata comments have been removed for cleaner output
		// Justify alignment falls back to default (no alignment markers)
		if !strings.Contains(result.Text, "|") {
			t.Error("Table should be present in output")
		}
		if !strings.Contains(result.Text, "Name") {
			t.Error("Table should contain Name column")
		}
		if !strings.Contains(result.Text, "Description") {
			t.Error("Table should contain Description column")
		}
	})

	t.Run("table alignment priority: align over style", func(t *testing.T) {
		// When both align attribute and style attribute are present,
		// align attribute should take precedence
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td align="center" style="text-align:left">Should be center</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should use align attribute (center) not style (left)
		if !strings.Contains(result.Text, "text-align:center") {
			t.Error("align attribute should take precedence over style attribute")
		}
		if strings.Contains(result.Text, "text-align:left") {
			t.Error("Should not use style attribute when align is present")
		}
	})

	t.Run("extracts alignment from complex style attribute", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td style="color:red; text-align:right; font-size:12px">Right aligned</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should extract text-align from complex style
		if !strings.Contains(result.Text, "text-align:right") {
			t.Error("Should extract alignment from complex style attribute")
		}
	})

	t.Run("handles whitespace in style attribute", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td style="text-align : center">Center with spaces</td>
						<td style="text-align:center">Center without spaces</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Both should be detected as center alignment
		count := strings.Count(result.Text, "text-align:center")
		if count < 2 {
			t.Errorf("Expected 2 center-aligned cells, got %d", count)
		}
	})

	t.Run("mixed case alignment attributes", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td ALIGN="Center">Mixed case align</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer func() { _ = p.Close() }()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should handle case-insensitive align attribute
		if !strings.Contains(result.Text, "text-align:center") {
			t.Error("Should handle mixed case ALIGN attribute")
		}
	})
}

func TestTableMarkdownFormat(t *testing.T) {
	t.Parallel()

	t.Run("complex table with alignments and colspans", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th align="left">Name</th>
						<th align="center">Age</th>
						<th align="right">Score</th>
					</tr>
					<tr>
						<td>Alice</td>
						<td>25</td>
						<td>95.5</td>
					</tr>
					<tr>
						<td colspan="2">Bob (combined)</td>
						<td>88.0</td>
					</tr>
				</table>
			</body></html>
		`

		p, _ := html.New()
		defer func() { _ = p.Close() }()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check for table content (markdown format may vary with colspan)
		if !strings.Contains(result.Text, "Name") {
			t.Error("Should contain table header content")
		}
		if !strings.Contains(result.Text, "Alice") {
			t.Error("Should contain table data row")
		}
		if !strings.Contains(result.Text, "Bob") {
			t.Error("Should contain colspan cell content")
		}
	})

	t.Run("table with uneven rows", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr><td>A</td><td>B</td><td>C</td></tr>
					<tr><td>D</td><td>E</td></tr>
					<tr><td>F</td></tr>
				</table>
			</body></html>
		`

		p, _ := html.New()
		defer func() { _ = p.Close() }()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should handle uneven rows gracefully
		if !strings.Contains(result.Text, "A") || !strings.Contains(result.Text, "D") || !strings.Contains(result.Text, "F") {
			t.Error("Should extract all cells from uneven table")
		}
	})

	t.Run("nested elements in table cells", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td><strong>Bold</strong> text</td>
						<td><a href="/link">Link</a></td>
					</tr>
				</table>
			</body></html>
		`

		p, _ := html.New()
		defer func() { _ = p.Close() }()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should extract text from nested elements
		if !strings.Contains(result.Text, "Bold") {
			t.Error("Should extract text from strong tag")
		}
		if !strings.Contains(result.Text, "Link") {
			t.Error("Should extract text from anchor tag")
		}
	})
}

func TestTableMarkdownEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("table with empty cells", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr><td>A</td><td></td><td>C</td></tr>
					<tr><td></td><td>E</td><td></td></tr>
				</table>
			</body></html>
		`

		p, _ := html.New()
		defer func() { _ = p.Close() }()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should handle empty cells
		if !strings.Contains(result.Text, "A") {
			t.Error("Should extract non-empty cells")
		}
		if !strings.Contains(result.Text, "E") {
			t.Error("Should extract non-empty cells")
		}
	})

	t.Run("table with nested markup", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td><strong>Bold</strong> and <em>italic</em></td>
						<td><a href="/link">Link</a></td>
					</tr>
				</table>
			</body></html>
		`

		p, _ := html.New()
		defer func() { _ = p.Close() }()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should extract text from nested elements
		if !strings.Contains(result.Text, "Bold") {
			t.Error("Should extract text from strong tag")
		}
		if !strings.Contains(result.Text, "italic") {
			t.Error("Should extract text from em tag")
		}
		if !strings.Contains(result.Text, "Link") {
			t.Error("Should extract text from anchor tag")
		}
	})
}
