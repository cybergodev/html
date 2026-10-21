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

	// Config.Validate accepts TableFormat case-insensitively and New
	// normalizes only the inline formats, so a mixed-case value such as
	// "HTML" reaches this method verbatim. Normalize once here, before BOTH
	// consumers of the value — the data path (extractTableData branches on
	// it case-sensitively) and the renderer dispatch (getRenderer switches
	// on the exact lowercased name) — so the two can never disagree. Empty
	// maps to "markdown", matching getRenderer's default branch.
	tableFormat = strings.ToLower(strings.TrimSpace(tableFormat))
	if tableFormat == "" {
		tableFormat = "markdown"
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

	// Step 3: Render in the requested format. getRenderer falls back to
	// Markdown for unknown formats.
	getRenderer(tableFormat).Render(tableData, tb, maxCols)

	// Ensure blank line after table for proper Markdown parsing
	_ = tb.WriteByte('\n')
	if tb.LastChar == '\n' {
		_ = tb.WriteByte('\n')
	}
}

// maxExpandedCells bounds the AGGREGATE number of colspan placeholder cells a
// single table may allocate across all rows. maxCellSpan (in the caller's
// CellAccessor) bounds one cell and the node-walk cap bounds visited DOM
// nodes — but placeholders are synthetic cells no walk ever sees, so without
// an aggregate cap a ~1.5MB document of <td colspan="1000"> cells multiplies
// ~50k real cells into tens of millions of CellData. Once the budget is
// spent, later colspans render as one column each. 10,000 is far above any
// legitimate table's expansion.
const maxExpandedCells = 10000

// extractTableData walks through table rows and extracts cell data.
func (p *Processor) extractTableData(table *html.Node, tableFormat string) [][]CellData {
	// tableFormat arrives already normalized by Extract (lowercased, trimmed,
	// empty defaulted to "markdown"), so the colspan-expansion and
	// structure-row branches below and getRenderer's dispatch all see the
	// same value.
	// Typical tables have several rows; pre-size to avoid the first outer-slice
	// doublings (16 → 32 → …). Grows naturally for larger tables.
	tableData := make([][]CellData, 0, 8)
	// scratch is reused across rows: extractRowCells resets it to [:0] and
	// appends into it each row, replacing a per-row make([]CellData, 0, 4).
	var scratch []CellData
	// arena holds every stored row of this table in one backing array, replacing
	// the per-row slices the HTML path (make + copy) and the Markdown path
	// (expandColspanCells) previously allocated. It is pre-sized to cover a
	// typical table in one allocation: growing from zero doubled the backing
	// 1→2→4→…, so a 30-cell table paid ~63 slots of cumulative copies (~4% of
	// per-Extract allocation bytes on a profiled realistic document). Larger
	// tables still grow naturally. Each stored row is resliced
	// with a full slice expression arena[start:end:end] so its capacity equals
	// its length: a later append onto that row (padTableColumns) allocates a
	// fresh backing instead of overwriting the next row's arena region. Skipped
	// Markdown structure rows return before appending, so they leave no residue.
	arena := make([]CellData, 0, 32)

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
			// appended (this inline expansion replaced the historical
			// expandColspanCells per-row allocation). expanded budgets the
			// table-wide placeholder count against maxExpandedCells:
			// placeholders are synthetic cells no node-walk cap can see, so
			// without this bound the arena grows as cells x maxCellSpan.
			expanded := 0
			for i := range rawCells {
				arena = append(arena, rawCells[i])
				for k := 1; k < rawCells[i].Colspan && expanded < maxExpandedCells; k++ {
					arena = append(arena, expandedPlaceholder(rawCells[i]))
					expanded++
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
