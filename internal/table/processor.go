// processor.go contains the processor interface and implementation for table extraction.
package table

import (
	"strings"

	"golang.org/x/net/html"
)

// CellAccessor provides methods to access cell information from HTML nodes.
// This interface abstracts the cell attribute extraction, allowing for
// different implementations and easier testing.
type CellAccessor interface {
	// GetAlignment returns the text alignment of the cell.
	GetAlignment(node *html.Node) CellAlignment
	// GetColSpan returns the column span of the cell.
	GetColSpan(node *html.Node) int
	// GetRowSpan returns the row span of the cell.
	GetRowSpan(node *html.Node) int
	// GetWidth returns the width specification of the cell.
	GetWidth(node *html.Node) string
	// GetTextContent returns the text content of the node.
	GetTextContent(node *html.Node) string
}

// NodeWalker provides methods for walking the DOM tree.
type NodeWalker interface {
	// Walk traverses the DOM tree starting from node, calling callback for each node.
	// The callback returns false to stop traversal, true to continue.
	Walk(node *html.Node, callback func(*html.Node) bool)
}

// Processor handles table extraction from HTML nodes.
type Processor struct {
	cellAccessor CellAccessor
	nodeWalker   NodeWalker
}

// NewProcessor creates a new table Processor with the given accessor and walker.
func NewProcessor(ca CellAccessor, nw NodeWalker) *Processor {
	return &Processor{
		cellAccessor: ca,
		nodeWalker:   nw,
	}
}

// Extract extracts HTML table content and converts it to the specified format.
// This is the main method for table extraction using the Processor.
func (p *Processor) Extract(table *html.Node, tb *TrackedBuilder, tableFormat string) {
	if table == nil {
		return
	}

	// Ensure blank line before table for proper Markdown parsing
	EnsureNewline(tb)
	if tb.LastChar == '\n' {
		_ = tb.WriteByte('\n')
	}

	// Step 1: Extract all row data from table
	tableData := p.extractTableData(table, tableFormat)

	if len(tableData) == 0 {
		return
	}

	// Step 2: Determine maximum columns
	maxCols := calculateMaxColumns(tableData)

	// Step 3: Render in requested format using registry
	if renderer := globalRegistry.get(tableFormat); renderer != nil {
		renderer.Render(tableData, tb, maxCols)
	} else {
		// Fallback to markdown for unknown formats
		extractTableAsMarkdown(tableData, tb, maxCols)
	}

	// Ensure blank line after table for proper Markdown parsing
	_ = tb.WriteByte('\n')
	if tb.LastChar == '\n' {
		_ = tb.WriteByte('\n')
	}
}

// extractTableData walks through table rows and extracts cell data.
func (p *Processor) extractTableData(table *html.Node, tableFormat string) [][]CellData {
	// Config.Validate accepts TableFormat case-insensitively and the renderer
	// registry lowercases on lookup, so normalize once here to keep the
	// colspan-expansion / structure-row branches (which compare case-sensitively
	// below) consistent with both. Without this, "HTML" renders via the HTML
	// renderer but takes the Markdown data path (colspans expanded into separate
	// cells, width-definition rows dropped), silently producing wrong tables.
	tableFormat = strings.ToLower(tableFormat)
	// Typical tables have several rows; pre-size to avoid the first outer-slice
	// doublings (16 → 32 → …). Grows naturally for larger tables.
	tableData := make([][]CellData, 0, 8)
	// scratch is reused across rows: extractRowCells resets it to [:0] and
	// appends into it each row, replacing a per-row make([]CellData, 0, 4).
	var scratch []CellData
	// arena holds every stored row of this table in one backing array, replacing
	// the per-row slices the HTML path (make + copy) and the Markdown path
	// (expandColspanCells) previously allocated. Each stored row is resliced
	// with a full slice expression arena[start:end:end] so its capacity equals
	// its length: a later append onto that row (padTableColumns) allocates a
	// fresh backing instead of overwriting the next row's arena region. Skipped
	// Markdown structure rows return before appending, so they leave no residue.
	var arena []CellData

	p.nodeWalker.Walk(table, func(node *html.Node) bool {
		if node.Type != html.ElementNode || node.Data != "tr" {
			return true
		}

		// Extract cells from this row
		rawCells := p.extractRowCells(node, &scratch)
		if len(rawCells) == 0 {
			return false
		}

		rowStart := len(arena)

		if tableFormat == "html" {
			// HTML keeps colspan as an attribute (no expansion). (Structure rows
			// are a Markdown concept and are always kept for HTML, matching the
			// prior behavior.)
			arena = append(arena, rawCells...)
		} else {
			// Markdown: skip structure rows (width definitions only), decided on
			// the raw cells BEFORE colspan expansion — expanded placeholder cells
			// carry no width and would defeat the check.
			if isStructureRow(rawCells) {
				return false
			}
			// Expand colspans into placeholder cells inline as the row is
			// appended, replacing expandColspanCells' per-row slice allocation.
			for i := range rawCells {
				arena = append(arena, rawCells[i])
				for k := 1; k < rawCells[i].Colspan; k++ {
					arena = append(arena, expandedPlaceholder(rawCells[i]))
				}
			}
		}

		tableData = append(tableData, arena[rowStart:len(arena):len(arena)])
		return false
	})

	return tableData
}

// expandedPlaceholder builds the placeholder cell that colspan expansion
// appends after its originating cell, mirroring the CellData shape
// expandColspanCells produced: one column wide, same alignment/header/rowspan
// as the origin, empty width, marked IsExpanded.
func expandedPlaceholder(origin CellData) CellData {
	return CellData{
		Text:            " ",
		Align:           origin.Align,
		Colspan:         1,
		Rowspan:         origin.Rowspan,
		IsHeader:        origin.IsHeader,
		Width:           "",
		IsExpanded:      true,
		OriginalColspan: 1,
	}
}

// extractRowCells extracts all cell data from a single table row (tr element).
// It appends into the caller-provided scratch buffer (reset to [:0] first) and
// returns a slice that shares that backing array. Because the next row reuses
// the same scratch, a caller that retains the returned slice across rows must
// copy it first; extractTableData immediately copies each row into its arena,
// so scratch is free to reuse.
func (p *Processor) extractRowCells(rowNode *html.Node, scratch *[]CellData) []CellData {
	cells := (*scratch)[:0]

	for child := rowNode.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode || (child.Data != "td" && child.Data != "th") {
			continue
		}

		cellText := sanitizeCellText(p.cellAccessor.GetTextContent(child))

		colspan := p.cellAccessor.GetColSpan(child)
		if colspan < 1 {
			colspan = 1
		}
		rowspan := p.cellAccessor.GetRowSpan(child)

		cells = append(cells, CellData{
			Text:            cellText,
			Align:           p.cellAccessor.GetAlignment(child),
			Colspan:         colspan,
			Rowspan:         rowspan,
			IsHeader:        child.Data == "th",
			Width:           p.cellAccessor.GetWidth(child),
			OriginalColspan: colspan,
		})
	}

	// Propagate any capacity growth back to the scratch holder so later rows
	// benefit from it rather than re-growing from the original cap.
	*scratch = cells
	return cells
}

// sanitizeCellText cleans and normalizes cell text content.
func sanitizeCellText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return " "
	}
	return text
}
