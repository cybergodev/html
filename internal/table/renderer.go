// renderer.go defines the table Renderer interface and the format dispatch
// used by processor.go.
//
// Only Markdown and HTML renderers exist, and this package is internal — no
// external caller could register another format — so dispatch is a plain
// switch. (The historical RendererRegistry was removed: its only methods were
// unexported, so the "extension point" its comment described was never
// actually reachable from outside the package.)
package table

// Renderer defines the interface for table rendering implementations.
// Different implementations can output tables in various formats (Markdown, HTML, etc.)
type Renderer interface {
	// Render renders the table data to the TrackedBuilder.
	Render(tableData [][]CellData, tb *TrackedBuilder, maxCols int)
}

// getRenderer returns the renderer for a table format (already normalized —
// lowercased and defaulted when empty — by Processor.Extract). Unknown
// formats fall back to Markdown, matching the registry-miss fallback this
// replaces.
func getRenderer(format string) Renderer {
	switch format {
	case "html":
		return &HTMLRenderer{}
	default:
		return &MarkdownRenderer{}
	}
}

// MarkdownRenderer renders tables in Markdown format.
type MarkdownRenderer struct{}

// Render renders the table data in Markdown format.
// Delegates to extractTableAsMarkdown to avoid code duplication.
func (r *MarkdownRenderer) Render(tableData [][]CellData, tb *TrackedBuilder, maxCols int) {
	extractTableAsMarkdown(tableData, tb, maxCols)
}

// HTMLRenderer renders tables in HTML format.
type HTMLRenderer struct{}

// Render renders the table data in HTML format.
// Delegates to extractTableAsHTML to avoid code duplication.
func (r *HTMLRenderer) Render(tableData [][]CellData, tb *TrackedBuilder, maxCols int) {
	extractTableAsHTML(tableData, tb)
}

// Note: The following functions are defined in render.go:
//   - renderMarkdownRow: renders a single table row in Markdown format
//   - renderHTMLCell: renders a single HTML table cell
//   - buildCellStyle: constructs the style attribute value for a table cell
