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
	// The callback returns false to skip that node's subtree (its children are
	// not visited); traversal continues with the node's siblings. Returning
	// false does NOT abort the whole walk — extractTableData relies on this,
	// pruning each processed row's children while later rows are still visited.
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
	p.extract(table, tb, tableFormat, nil)
}

// ExtractWithCellText is Extract with a cell-text override: cellText builds a
// cell's text instead of the accessor's GetTextContent. The extraction pass
// passes a token-emitting builder here (internal.tableTokenText) so cell
// anchors and images reach the output as [LINK:n]/[IMAGE:n] placeholders that
// the caller's formatInline* pass then substitutes — table links keep their
// URLs instead of collapsing to bare labels. The closure is also invoked for
// non-cell row children with its result discarded, advancing the counters
// exactly as the document-order collector does for stray content outside
// td/th. A nil cellText is equivalent to Extract.
func (p *Processor) ExtractWithCellText(table *html.Node, tb *TrackedBuilder, tableFormat string, cellText func(*html.Node) string) {
	p.extract(table, tb, tableFormat, cellText)
}

func (p *Processor) extract(table *html.Node, tb *TrackedBuilder, tableFormat string, cellText func(*html.Node) string) {
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

	// Step 1: Extract all row data from table. This runs before the leading
	// blank line is written so a table with no renderable rows (no <tr>, or
	// rows without td/th cells) emits nothing at all — a newline pair written
	// up front inserted a paragraph break the source document did not have.
	tableData := p.extractTableData(table, tableFormat, cellText)

	if len(tableData) == 0 {
		return
	}

	// Ensure blank line before table for proper Markdown parsing
	EnsureNewline(tb)
	if tb.LastChar == '\n' {
		_ = tb.WriteByte('\n')
	}

	// Step 2: Determine maximum columns — only the Markdown renderer consumes
	// it (the rowspan grid and column filtering); the HTML renderer ignores
	// it, so skip the full rowspan placement replay for the html format.
	// getRenderer falls back to Markdown for unknown formats, and that
	// fallback needs the value, so the computation runs for everything but an
	// exact "html".
	maxCols := 0
	if tableFormat != "html" {
		maxCols = calculateMaxColumns(tableData)
	}

	// Step 3: Render in the requested format.
	getRenderer(tableFormat).Render(tableData, tb, maxCols)

	// Ensure blank line after table for proper Markdown parsing. WriteByte
	// unconditionally records the byte in LastChar, so after the '\n' just
	// written the second newline was unconditional — write the pair directly.
	_, _ = tb.WriteString("\n\n")
}

// maxExpandedCells bounds the AGGREGATE number of colspan placeholder cells a
// single table may allocate across all rows. maxCellSpan (in the caller's
// CellAccessor) bounds one cell, and MaxInputSize bounds the real DOM nodes —
// but placeholders are synthetic cells no walk ever sees, so without an
// aggregate cap a ~1.5MB document of <td colspan="1000"> cells multiplies
// ~50k real cells into tens of millions of CellData. Once the budget is
// spent, later colspans render as one column each. 10,000 is far above any
// legitimate table's expansion.
const maxExpandedCells = 10000

// extractTableData walks through table rows and extracts cell data.
func (p *Processor) extractTableData(table *html.Node, tableFormat string, cellText func(*html.Node) string) [][]CellData {
	// tableFormat arrives already normalized by Extract (lowercased, trimmed,
	// empty defaulted to "markdown"), so the colspan-expansion and
	// structure-row branches below and getRenderer's dispatch all see the
	// same value.
	// Typical tables have several rows; pre-size to avoid the first outer-slice
	// doublings (16 → 32 → …). Grows naturally for larger tables.
	tableData := make([][]CellData, 0, 8)
	// rowBuf holds the current row's raw cells; it is reused across rows
	// (appendRowCells writes into rowBuf[:0]) so the per-row cell extraction
	// allocates nothing after the first few rows.
	var rowBuf []CellData
	// arena holds every stored row of this table in one backing array, replacing
	// the per-row slices both the HTML and the Markdown paths previously
	// allocated. It is pre-sized to cover a
	// typical table in one allocation: growing from zero doubled the backing
	// 1→2→4→…, so a 30-cell table paid ~63 slots of cumulative copies (~4% of
	// per-Extract allocation bytes on a profiled realistic document). Larger
	// tables still grow naturally. Each stored row is resliced
	// with a full slice expression arena[start:end:end] so its capacity equals
	// its length: a later append onto that row (padTableColumns) allocates a
	// fresh backing instead of overwriting the next row's arena region. Skipped
	// Markdown structure rows return before appending, so they leave no residue.
	arena := make([]CellData, 0, 32)

	// expanded budgets the table's colspan placeholders against
	// maxExpandedCells. It lives OUTSIDE the row callback: placeholders are
	// synthetic cells no DOM-node bound can see, and a per-row counter let
	// every row spend the full budget again — a 2,000-row document of
	// <td colspan="1000"> expanded to two million cells (~780MB of allocation
	// for an 88KB input) even though the documented budget is per table.
	expanded := 0

	p.nodeWalker.Walk(table, func(node *html.Node) bool {
		if node.Type != html.ElementNode {
			return true
		}
		// <template> is the one non-cell element the HTML parser keeps
		// between rows (phrasing strays are foster-parented out of the
		// table), and its parsed anchors are numbered by the section-wide
		// collector. Burn them — text discarded, exactly like stray row
		// children — so the counters stay aligned; rows below this node
		// still process normally.
		if cellText != nil && node.Data == "template" {
			_ = cellText(node)
			return false
		}
		if node.Data != "tr" {
			return true
		}

		// Extract this row's cells into the reusable buffer.
		rowBuf = p.appendRowCells(rowBuf[:0], node, cellText)
		if len(rowBuf) == 0 {
			return false
		}

		rowStart := len(arena)

		if tableFormat == "html" {
			// HTML keeps colspan as an attribute (no expansion). (Structure rows
			// are a Markdown concept and are always kept for HTML, matching the
			// prior behavior.)
			arena = append(arena, rowBuf...)
		} else {
			// Markdown: skip structure rows (width definitions only), decided on
			// the raw cells BEFORE colspan expansion — expanded placeholder cells
			// carry no width and would defeat the check.
			if isStructureRow(rowBuf) {
				return false
			}
			// Expand colspans into placeholder cells inline as the row is
			// appended. Once the table-wide budget is spent, later colspans
			// render as one column each. The range copies each cell, so writing
			// into arena cannot disturb the cells still to be read.
			for _, cell := range rowBuf {
				arena = append(arena, cell)
				for k := 1; k < cell.Colspan && expanded < maxExpandedCells; k++ {
					arena = append(arena, expandedPlaceholder(cell))
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
// appends after its originating cell: one column wide, same
// alignment/header/rowspan as the origin, empty width, marked IsExpanded.
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

// appendRowCells appends the cell data of a single table row (tr element) to
// dst and returns the extended slice. Passing a truncated buffer
// (rowBuf[:0]) reuses its capacity across rows, so a table of N rows performs
// O(1) allocations rather than one per row.
//
// When cellText is non-nil it builds each cell's text (token-aware) and is
// also invoked — result discarded — on non-cell element children of the row,
// so stray anchors between cells advance the [LINK:n]/[IMAGE:n] counters in
// exactly the document order the section-wide collector numbered them.
//
// The returned slice belongs to the caller and is the only handle to those
// cells — there is no shared scratch that a later row could overwrite while
// the caller is still reading. The previous signature took a *[]CellData and
// returned a slice aliasing that shared buffer, which required the caller to
// know it had to finish reading before the next row was extracted.
func (p *Processor) appendRowCells(dst []CellData, rowNode *html.Node, cellText func(*html.Node) string) []CellData {
	for child := rowNode.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}
		if child.Data != "td" && child.Data != "th" {
			if cellText != nil {
				_ = cellText(child) // counter-accurate, text discarded
			}
			continue
		}

		// GetTextContent is the hottest allocator in extraction (invoked once
		// per table cell); when the token-aware walker is active its result
		// fully replaces the plain text, so walk the subtree only once.
		var text string
		if cellText != nil {
			text = cellText(child)
		} else {
			text = p.cellAccessor.GetTextContent(child)
		}
		colspan := max(p.cellAccessor.GetColSpan(child), 1)

		dst = append(dst, CellData{
			Text:            sanitizeCellText(text),
			Align:           p.cellAccessor.GetAlignment(child),
			Colspan:         colspan,
			Rowspan:         p.cellAccessor.GetRowSpan(child),
			IsHeader:        child.Data == "th",
			Width:           p.cellAccessor.GetWidth(child),
			OriginalColspan: colspan,
		})
	}
	return dst
}

// sanitizeCellText cleans and normalizes cell text content.
func sanitizeCellText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return " "
	}
	return text
}
