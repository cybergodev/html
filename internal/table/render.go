package table

import (
	"html"
	"strconv"
	"strings"
	"unicode/utf8"
)

// isStructureRow determines if a row contains only width definitions (no real content).
// Structure rows are used in Markdown tables to specify column widths.
func isStructureRow(cells []CellData) bool {
	hasWidthDefinitions := true
	hasRealContent := false

	for _, cell := range cells {
		if cell.Width == "" {
			hasWidthDefinitions = false
		}
		if cell.Text != " " && cell.Text != "" {
			hasRealContent = true
		}
	}

	return hasWidthDefinitions && !hasRealContent
}

// expandColspanCells expands cells with colspan > 1 into multiple placeholder cells.
// This is needed for Markdown format which doesn't support colspan.
func expandColspanCells(rawCells []CellData) []CellData {
	cells := make([]CellData, 0, len(rawCells))

	for _, rawCell := range rawCells {
		// Add the original cell
		cells = append(cells, rawCell)

		// Add placeholder cells for colspan > 1
		originalAlign := rawCell.Align
		for i := 1; i < rawCell.Colspan; i++ {
			cells = append(cells, CellData{
				Text:            " ",
				Align:           originalAlign,
				Colspan:         1,
				Rowspan:         rawCell.Rowspan,
				IsHeader:        rawCell.IsHeader,
				Width:           "",
				IsExpanded:      true,
				OriginalColspan: 1,
			})
		}
	}

	return cells
}

// anyRowspan reports whether any cell in the table declares rowspan > 1.
// applyRowspanGrid uses it to skip its grid rebuild when the table has no
// rowspans (the common case), since the rebuild is then an identity transform.
func anyRowspan(tableData [][]CellData) bool {
	for _, row := range tableData {
		for _, c := range row {
			if c.Rowspan > 1 {
				return true
			}
		}
	}
	return false
}

// applyRowspanGrid rebuilds the table so a cell with rowspan > 1 occupies its
// column in the rows it spans. Markdown tables are strictly rectangular — they
// cannot express rowspan — so without this grid a spanned cell appears only in
// its declared row and every later row's cells shift one column left, landing
// under the wrong header. Each position a rowspan covers is filled by repeating
// the originating cell's text (the convention used by Pandoc and other
// converters).
//
// colspan is already expanded into separate CellData entries by
// expandColspanCells, so each entry in a row represents exactly one column.
// The grid places entries left-to-right, skipping columns already occupied by
// a rowspan from an earlier row (filling them with the repeated text). Rows
// shorter than maxCols are padded with empty cells; rows longer than maxCols
// have their excess cells dropped, matching the renderer, which only emits
// maxCols columns.
func applyRowspanGrid(tableData [][]CellData, maxCols int) [][]CellData {
	if len(tableData) == 0 || maxCols <= 0 {
		return tableData
	}

	// Fast path: with no rowspan > 1 anywhere, the grid is an identity
	// transform. Each cell is placed 1:1 (nothing shifts, because the occupied
	// map never gains an entry), and since calculateMaxColumns returns the raw
	// max per-row cell count in that case, no row exceeds maxCols and nothing is
	// truncated. The only effect the full rebuild would have is padding trailing
	// columns with CellData{Text: " ", Align: AlignDefault} (the pad below at
	// row[col] = ... AlignDefault) — and padTableColumns, which runs next in
	// extractTableAsMarkdown, pads with the identical CellData. Skipping the
	// rebuild therefore yields byte-identical output while avoiding the occupied
	// and fillText scratch slices, the outer grid slice, and a per-row
	// make([]CellData, maxCols). The common rowspan-free table hits this path.
	if !anyRowspan(tableData) {
		return tableData
	}

	numRows := len(tableData)
	// Flatten the two scratch grids into single 1-D buffers of length
	// numRows*maxCols, indexed [r*maxCols+c]: occupied marks a column already
	// taken by a rowspan from an earlier row; fillText holds the originating
	// cell's text to repeat there. This replaces 2*numRows per-row slice
	// allocations with two flat slices while keeping the grid logic identical.
	occupied := make([]bool, numRows*maxCols)
	fillText := make([]string, numRows*maxCols)

	grid := make([][]CellData, numRows)
	for r := 0; r < numRows; r++ {
		row := make([]CellData, maxCols)
		base := r * maxCols
		col := 0
		for _, cell := range tableData[r] {
			// Skip columns occupied by a rowspan from above, filling them with
			// the repeated originating text.
			for col < maxCols && occupied[base+col] {
				row[col] = spannedFill(fillText[base+col])
				col++
			}
			if col >= maxCols {
				break // more cells than columns; drop the rest (matches renderer)
			}
			row[col] = cell
			// Reserve this column in the rows the rowspan covers, carrying the
			// cell's text so each spanned position repeats it.
			if rs := cell.Rowspan; rs > 1 {
				for rr := 1; rr < rs && r+rr < numRows; rr++ {
					occupied[(r+rr)*maxCols+col] = true
					fillText[(r+rr)*maxCols+col] = cell.Text
				}
			}
			col++
		}
		// Fill any trailing occupied columns, then pad the remainder with empty
		// cells so the row is exactly maxCols wide.
		for col < maxCols {
			if occupied[base+col] {
				row[col] = spannedFill(fillText[base+col])
			} else {
				row[col] = CellData{Text: " ", Align: AlignDefault}
			}
			col++
		}
		grid[r] = row
	}
	return grid
}

// spannedFill builds a Markdown cell that repeats a rowspan origin's text in a
// spanned row. Alignment is left at AlignDefault so the cell does not
// participate in the column's majority-alignment vote — the originating cell
// already votes once from its own row, and letting every spanned copy vote
// would over-weight that alignment by the rowspan count.
func spannedFill(text string) CellData {
	if text == "" {
		text = " "
	}
	return CellData{
		Text:            text,
		Align:           AlignDefault,
		Colspan:         1,
		Rowspan:         1,
		OriginalColspan: 1,
	}
}

// calculateMaxColumns computes the grid width needed to place every cell
// without dropping any, replaying the same rowspan placement applyRowspanGrid
// performs. A cell with rowspan > 1 reserves its column in the rows it spans,
// so a later row's cells shift right and can land beyond the raw per-row cell
// count. Computing maxCols from raw counts (the old behavior) made
// applyRowspanGrid drop those shifted cells at its "col >= maxCols" guard,
// silently losing trailing cells. colspan is already expanded into separate
// one-column entries by expandColspanCells, so each cell occupies exactly one
// column here.
func calculateMaxColumns(tableData [][]CellData) int {
	maxCols := 0
	numRows := len(tableData)
	// occupied[r] records columns in row r already taken by a rowspan from an
	// earlier row. Only rows that receive a span get a non-nil map.
	occupied := make([]map[int]bool, numRows)
	for r := 0; r < numRows; r++ {
		occ := occupied[r]
		col := 0
		for _, cell := range tableData[r] {
			for occ[col] {
				col++
			}
			if col+1 > maxCols {
				maxCols = col + 1
			}
			if rs := cell.Rowspan; rs > 1 {
				for rr := 1; rr < rs && r+rr < numRows; rr++ {
					dest := r + rr
					if occupied[dest] == nil {
						occupied[dest] = make(map[int]bool)
					}
					occupied[dest][col] = true
				}
			}
			col++
		}
	}
	return maxCols
}

// extractTableAsMarkdown outputs table in Markdown format with alignment.
func extractTableAsMarkdown(tableData [][]CellData, tb *TrackedBuilder, maxCols int) {
	// Apply a rowspan grid so a cell with rowspan > 1 keeps its column in the
	// rows it spans, instead of letting later rows' cells shift left into the
	// wrong column. Markdown cannot express rowspan, so each spanned position
	// repeats the originating cell's text (the Pandoc convention). This must
	// run before width/alignment calculation, which depend on correct column
	// positions. The HTML path is unaffected — it emits rowspan as an attribute.
	tableData = applyRowspanGrid(tableData, maxCols)

	// Pad rows to have consistent column count
	tableData = padTableColumns(tableData, maxCols)

	// Calculate column properties
	colAligns := calculateColumnAlignments(tableData, maxCols)
	colMaxWidths := calculateMaxColumnWidths(tableData, maxCols)

	// Filter out columns that are entirely empty expanded cells
	newToOldCol := filterExpandedColumns(tableData, maxCols)

	// Build arrays for included columns only
	includedColAligns := filterArray(colAligns, newToOldCol)
	includedColMaxWidths := filterIntArray(colMaxWidths, newToOldCol)

	// Ensure minimum width for alignment markers
	for i := range includedColMaxWidths {
		if includedColMaxWidths[i] < 3 {
			includedColMaxWidths[i] = 3
		}
	}

	// Render table rows with alignment separator after the first row
	if len(tableData) > 0 {
		// Markdown pipe tables require a header row followed by an alignment
		// separator. When the source table has no <th> cells, there is no real
		// header: synthesize an empty header row instead of promoting the first
		// data row, which would otherwise mislabel real data as column headers.
		if tableHasHeader(tableData) {
			// Render first row (header)
			renderMarkdownRow(tb, tableData[0], newToOldCol, includedColAligns, includedColMaxWidths)
			writeMarkdownSeparator(tb, includedColAligns)
			// Render remaining rows
			for i := 1; i < len(tableData); i++ {
				renderMarkdownRow(tb, tableData[i], newToOldCol, includedColAligns, includedColMaxWidths)
			}
		} else {
			// No <th> anywhere: synthesize an empty header row and render every
			// real row (including the former first row) as data.
			emptyHeader := make([]CellData, maxCols)
			renderMarkdownRow(tb, emptyHeader, newToOldCol, includedColAligns, includedColMaxWidths)
			writeMarkdownSeparator(tb, includedColAligns)
			for i := 0; i < len(tableData); i++ {
				renderMarkdownRow(tb, tableData[i], newToOldCol, includedColAligns, includedColMaxWidths)
			}
		}
	}
}

// tableHasHeader reports whether any cell in the table is a <th> header cell.
// When false, the Markdown renderer must synthesize an empty header row rather
// than treating the first data row as a header.
func tableHasHeader(tableData [][]CellData) bool {
	for _, row := range tableData {
		for _, cell := range row {
			if cell.IsHeader {
				return true
			}
		}
	}
	return false
}

// writeMarkdownSeparator writes the Markdown alignment separator row
// (e.g. "| :--- | ---: |") that must follow a table's header row.
func writeMarkdownSeparator(tb *TrackedBuilder, includedColAligns []string) {
	_, _ = tb.WriteString("| ")
	_, _ = tb.WriteString(strings.Join(includedColAligns, " | "))
	_, _ = tb.WriteString(" |\n")
}

// padTableColumns ensures all rows have the same number of columns.
func padTableColumns(tableData [][]CellData, maxCols int) [][]CellData {
	for i := range tableData {
		for len(tableData[i]) < maxCols {
			tableData[i] = append(tableData[i], CellData{Text: " ", Align: AlignDefault})
		}
	}
	return tableData
}

// calculateColumnAlignments determines column alignment using majority voting.
// Returns alignment strings in Markdown format (:---, :--:, ---:, etc.)
// Optimized with pre-allocated slices.
func calculateColumnAlignments(tableData [][]CellData, maxCols int) []string {
	colAligns := make([]string, maxCols)
	// Pre-allocate alignCounts with exact capacity needed
	alignCounts := make([]AlignCount, maxCols)

	// Count alignments from all non-expanded cells
	for _, row := range tableData {
		for i := 0; i < maxCols && i < len(row); i++ {
			if !row[i].IsExpanded && row[i].Text != " " && row[i].Align != AlignDefault {
				switch row[i].Align {
				case AlignLeft:
					alignCounts[i].Left++
				case AlignCenter:
					alignCounts[i].Center++
				case AlignRight:
					alignCounts[i].Right++
				case AlignJustify:
					alignCounts[i].Justify++
				}
			}
		}
	}

	// Determine majority alignment for each column
	if len(tableData) > 0 {
		for i := 0; i < maxCols; i++ {
			colAligns[i] = determineColumnAlignment(alignCounts[i], tableData[0], i)
		}
	} else {
		for i := range colAligns {
			colAligns[i] = "---"
		}
	}

	return colAligns
}

// determineColumnAlignment picks the majority alignment for a single column.
func determineColumnAlignment(counts AlignCount, firstRow []CellData, colIdx int) string {
	maxCount := 0
	majorityAlign := AlignDefault

	// Find the alignment with the most votes
	if counts.Left > maxCount {
		maxCount = counts.Left
		majorityAlign = AlignLeft
	}
	if counts.Center > maxCount {
		maxCount = counts.Center
		majorityAlign = AlignCenter
	}
	if counts.Right > maxCount {
		maxCount = counts.Right
		majorityAlign = AlignRight
	}
	if counts.Justify > maxCount {
		maxCount = counts.Justify
		majorityAlign = AlignJustify
	}

	// If no clear majority, use first row's alignment
	if maxCount == 0 && len(firstRow) > colIdx {
		majorityAlign = firstRow[colIdx].Align
	}

	// Check for mixed alignment (both left and right present)
	hasMixedAlignment := counts.Left > 0 && counts.Right > 0

	if hasMixedAlignment {
		return "---"
	}

	// Convert to Markdown alignment format
	switch majorityAlign {
	case AlignLeft:
		return ":---"
	case AlignCenter:
		return ":--:"
	case AlignRight:
		return "---:"
	case AlignJustify:
		return "---"
	default:
		return "---"
	}
}

// calculateMaxColumnWidths finds the maximum text width for each column.
// Optimized with pre-allocated slice.
func calculateMaxColumnWidths(tableData [][]CellData, maxCols int) []int {
	// Pre-allocate with exact capacity needed
	colMaxWidths := make([]int, maxCols)
	for _, row := range tableData {
		for j := 0; j < maxCols && j < len(row); j++ {
			textLen := utf8.RuneCountInString(row[j].Text)
			if textLen > colMaxWidths[j] {
				colMaxWidths[j] = textLen
			}
		}
	}
	return colMaxWidths
}

// filterExpandedColumns identifies columns that should be excluded.
// Returns a list of included column indices (columns with real content).
// Optimized with pre-allocated slice capacity.
func filterExpandedColumns(tableData [][]CellData, maxCols int) []int {
	includeCol := make([]bool, maxCols)
	// Pre-allocate with estimated capacity: assume 80% of columns are included
	estimatedInclusions := maxCols * 4 / 5
	if estimatedInclusions < 1 {
		estimatedInclusions = 1
	}
	newToOldCol := make([]int, 0, estimatedInclusions)

	for j := 0; j < maxCols; j++ {
		// Check if this column has any non-expanded content
		allExpanded := true
		for _, row := range tableData {
			if j < len(row) && (!row[j].IsExpanded || (row[j].Text != " " && row[j].Text != "")) {
				allExpanded = false
				break
			}
		}

		includeCol[j] = !allExpanded
		if !allExpanded {
			newToOldCol = append(newToOldCol, j)
		}
	}

	return newToOldCol
}

// filterArray filters a string array to include only specified indices.
func filterArray(arr []string, indices []int) []string {
	result := make([]string, len(indices))
	for i, idx := range indices {
		if idx < len(arr) {
			result[i] = arr[idx]
		}
	}
	return result
}

// filterIntArray filters an int array to include only specified indices.
func filterIntArray(arr []int, indices []int) []int {
	result := make([]int, len(indices))
	for i, idx := range indices {
		if idx < len(arr) {
			result[i] = arr[idx]
		}
	}
	return result
}

// paddingLookup provides pre-computed padding strings for small values
// to avoid repeated strings.Repeat allocations in table rendering.
var paddingLookup = [32]string{
	"", " ", "  ", "   ", "    ", "     ", "      ", "       ",
	"        ", "         ", "          ", "           ", "            ",
	"             ", "              ", "               ", "                ",
	"                 ", "                  ", "                   ",
	"                    ", "                     ", "                      ",
	"                       ", "                        ", "                         ",
	"                          ", "                           ", "                            ",
	"                             ", "                              ", "                               ",
}

// writePadding writes n spaces to the builder using a lookup table for small values.
func writePadding(tb *TrackedBuilder, n int) {
	if n <= 0 {
		return
	}
	if n < len(paddingLookup) {
		_, _ = tb.WriteString(paddingLookup[n])
		return
	}
	_, _ = tb.WriteString(strings.Repeat(" ", n))
}

// renderMarkdownRow renders a single table row in Markdown format.
func renderMarkdownRow(tb *TrackedBuilder, row []CellData, newToOldCol []int,
	colAligns []string, colMaxWidths []int) {

	_, _ = tb.WriteString("| ")
	for newJ, oldJ := range newToOldCol {
		cellText := " "
		if oldJ < len(row) {
			cellText = row[oldJ].Text
		}

		maxWidth := colMaxWidths[newJ]
		textLen := utf8.RuneCountInString(cellText)
		pad := maxWidth - textLen

		// Apply alignment-based padding. Cell text is written through
		// writeMarkdownCellText so a literal '|' cannot end the cell early and
		// spill into the next column, and a literal '\' cannot escape the
		// following delimiter character.
		switch colAligns[newJ] {
		case ":---": // left
			writeMarkdownCellText(tb, cellText)
			writePadding(tb, pad)
		case "---:": // right
			writePadding(tb, pad)
			writeMarkdownCellText(tb, cellText)
		case ":--:": // center
			leftPad := pad / 2
			rightPad := pad - leftPad
			writePadding(tb, leftPad)
			writeMarkdownCellText(tb, cellText)
			writePadding(tb, rightPad)
		default: // left (default)
			writeMarkdownCellText(tb, cellText)
			writePadding(tb, pad)
		}

		if newJ < len(newToOldCol)-1 {
			_, _ = tb.WriteString(" | ")
		}
	}
	_, _ = tb.WriteString(" |\n")
}

// writeMarkdownCellText writes cell text into a Markdown table cell, escaping
// the characters that are structural in Markdown tables or serve as escape
// introducers. A literal '|' would prematurely end the cell (and can spill into
// subsequent columns); a literal '\' would escape the following character. Both
// are backslash-escaped so the cell content is preserved verbatim. Newlines are
// replaced with spaces, since a Markdown table row cannot span multiple lines.
func writeMarkdownCellText(tb *TrackedBuilder, text string) {
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch c {
		case '\\', '|':
			_ = tb.WriteByte('\\')
			_ = tb.WriteByte(c)
		case '\n', '\r':
			_ = tb.WriteByte(' ')
		default:
			_ = tb.WriteByte(c)
		}
	}
}

// extractTableAsHTML outputs table in HTML format with proper attributes.
func extractTableAsHTML(tableData [][]CellData, tb *TrackedBuilder) {
	_, _ = tb.WriteString("<table>\n")

	for _, row := range tableData {
		_, _ = tb.WriteString("  <tr>\n")
		for _, cell := range row {
			renderHTMLCell(tb, cell)
		}
		_, _ = tb.WriteString("  </tr>\n")
	}

	_, _ = tb.WriteString("</table>")
}

// renderHTMLCell renders a single table cell in HTML format.
func renderHTMLCell(tb *TrackedBuilder, cell CellData) {
	// Determine tag name
	tag := "td"
	if cell.IsHeader {
		tag = "th"
	}
	_, _ = tb.WriteString("    <")
	_, _ = tb.WriteString(tag)

	// Add style attribute
	style := buildCellStyle(cell)
	if style != "" {
		_, _ = tb.WriteString(` style="`)
		_, _ = tb.WriteString(style)
		_, _ = tb.WriteString(`"`)
	}

	// Add colspan attribute
	if cell.OriginalColspan > 1 && !cell.IsExpanded {
		_, _ = tb.WriteString(` colspan="`)
		_, _ = tb.WriteString(strconv.Itoa(cell.OriginalColspan))
		_, _ = tb.WriteString(`"`)
	}

	// Add rowspan attribute
	if cell.Rowspan > 1 {
		_, _ = tb.WriteString(` rowspan="`)
		_, _ = tb.WriteString(strconv.Itoa(cell.Rowspan))
		_, _ = tb.WriteString(`"`)
	}

	// Write cell content. Escape the text so that characters meaningful in HTML
	// (<, >, &, ") cannot break the cell's structure or inject markup. This
	// mirrors the inline image/link path in extract.go, which escapes its HTML
	// output via htmlstd.EscapeString. The Markdown path escapes via
	// writeMarkdownCellText (| and \) in renderMarkdownRow.
	_, _ = tb.WriteString(">")
	_, _ = tb.WriteString(html.EscapeString(cell.Text))
	_, _ = tb.WriteString("</")
	_, _ = tb.WriteString(tag)
	_, _ = tb.WriteString(">\n")
}

// buildCellStyle constructs the style attribute value for a table cell.
func buildCellStyle(cell CellData) string {
	if cell.Align == AlignDefault && (cell.Width == "" || cell.IsExpanded) {
		return ""
	}

	var styleParts []string
	switch cell.Align {
	case AlignLeft:
		styleParts = append(styleParts, "text-align:left")
	case AlignCenter:
		styleParts = append(styleParts, "text-align:center")
	case AlignRight:
		styleParts = append(styleParts, "text-align:right")
	case AlignJustify:
		styleParts = append(styleParts, "text-align:justify")
	}

	if cell.Width != "" && !cell.IsExpanded {
		// cell.Width originates from an HTML width attribute or inline style and
		// is emitted into a style="..." attribute, so sanitize it to CSS-length
		// characters to prevent attribute breakout or CSS property injection.
		if w := sanitizeCSSWidth(cell.Width); w != "" {
			styleParts = append(styleParts, "width:"+w)
		}
	}

	return strings.Join(styleParts, ";")
}

// sanitizeCSSWidth reduces a width value to the characters permitted in a CSS
// length (digits, '.', '%', and ASCII letters for units such as px/em/rem).
// Any '"', ';', ':', '(', ')', or whitespace — which could otherwise break out
// of the style attribute or inject CSS properties — is dropped. A result of ""
// means the value was not a recognizable length and the width declaration is
// omitted entirely.
func sanitizeCSSWidth(w string) string {
	if w == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(w))
	for i := 0; i < len(w); i++ {
		c := w[i]
		switch {
		case c >= '0' && c <= '9', c == '.', c == '%':
			b.WriteByte(c)
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			b.WriteByte(c)
		}
	}
	return b.String()
}
