package table

// regressions_test.go — regression tests for the table package, gathered from
// the former single-topic files (caps_regression_test.go,
// width_escape_regression_test.go, p2_regression_test.go):
//
//   - the hostile-table allocation caps: maxExpandedCells bounds the aggregate
//     colspan placeholder count per table (placeholders are synthetic cells no
//     DOM-node bound can see; without the budget, ~50k <td colspan="1000">
//     cells expanded into tens of millions of CellData), and maxGridCells
//     clamps the numRows x maxCols rendering grid that padTableColumns /
//     applyRowspanGrid / per-column slices allocate;
//   - calculateMaxColumnWidths counts the escape sequences
//     writeMarkdownCellText emits, so a cell containing '|' or '\' no longer
//     renders one character wider than its padded column;
//   - TrackedBuilder.SecureClear (pool secure-clear support) and the
//     getRenderer format dispatch that replaced the unused registry.

import (
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// ---------------------------------------------------------------------------
// Allocation caps (from caps_regression_test.go)
// ---------------------------------------------------------------------------

// capTestAccessor is a minimal CellAccessor: colspan/rowspan are parsed from
// the node's attributes (clamped to 1000 like the production accessor);
// everything else returns a default.
type capTestAccessor struct{}

func (capTestAccessor) GetAlignment(*html.Node) CellAlignment { return AlignDefault }
func (capTestAccessor) GetWidth(*html.Node) string            { return "" }

func (capTestAccessor) GetTextContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

func (capTestAccessor) GetColSpan(n *html.Node) int { return capTestSpan(n, "colspan") }
func (capTestAccessor) GetRowSpan(n *html.Node) int { return capTestSpan(n, "rowspan") }

func capTestSpan(n *html.Node, key string) int {
	if n == nil {
		return 1
	}
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			if v, err := strconv.Atoi(strings.TrimSpace(attr.Val)); err == nil && v > 0 {
				if v > 1000 {
					return 1000
				}
				return v
			}
		}
	}
	return 1
}

// capTestWalker visits the subtree depth-first. A false return stops
// traversal of that node's children, exactly like internal.WalkNodes and the
// production walker: returning false must NOT abort the whole walk, or the
// tests would silently see only the first <tr> of a multi-row table and miss
// exactly the behavior they mean to pin.
type capTestWalker struct{}

func (capTestWalker) Walk(n *html.Node, cb func(*html.Node) bool) {
	var rec func(*html.Node)
	rec = func(n *html.Node) {
		if n == nil || !cb(n) {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			rec(c)
		}
	}
	rec(n)
}

func parseCapTestTable(t *testing.T, markup string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		t.Fatal(err)
	}
	var table *html.Node
	var find func(*html.Node)
	find = func(n *html.Node) {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "table" && table == nil {
				table = c
			}
			find(c)
		}
	}
	find(doc)
	if table == nil {
		t.Fatal("no <table> in test markup")
	}
	return table
}

// TestExtractTableDataExpansionCapMultiRow pins the budget's scope: it is a
// per-TABLE limit, not a per-row one. The counter once lived inside the row
// callback, so every row could spend the full maxExpandedCells again — 100
// rows of <td colspan="1000"> stored 100,000 cells against a documented
// budget of 10,000, and a 2,000-row hostile table allocated ~780MB through the
// public Extract API. A single-row test (the original) could not see it.
func TestExtractTableDataExpansionCapMultiRow(t *testing.T) {
	t.Parallel()

	const rows, colspan = 100, 1000
	markup := "<table>" + strings.Repeat(`<tr><td colspan="1000">x</td></tr>`, rows) + "</table>"
	p := NewProcessor(capTestAccessor{}, capTestWalker{})
	data := p.extractTableData(parseCapTestTable(t, markup), "markdown")

	total := 0
	for _, row := range data {
		total += len(row)
	}
	if want := rows + maxExpandedCells; total > want {
		t.Errorf("stored cells = %d, want <= %d (budget must be table-wide, not per row)", total, want)
	}
	if total < rows {
		t.Errorf("real cells lost: %d < %d", total, rows)
	}

	// Rendering still terminates and keeps every real cell's text.
	tb := NewTrackedBuilder()
	extractTableAsMarkdown(data, tb, calculateMaxColumns(data))
	if !strings.Contains(tb.String(), "x") {
		t.Errorf("rendered table lost cell text")
	}
}

// TestExtractTableDataExpansionCap remains the single-row case.
func TestExtractTableDataExpansionCap(t *testing.T) {
	t.Parallel()

	markup := "<table><tr>" + strings.Repeat(`<td colspan="1000">x</td>`, 20) + "</tr></table>"
	p := NewProcessor(capTestAccessor{}, capTestWalker{})
	rows := p.extractTableData(parseCapTestTable(t, markup), "markdown")

	total := 0
	for _, row := range rows {
		total += len(row)
	}
	if total > 20+maxExpandedCells {
		t.Errorf("expanded cells = %d, want <= %d (per-table budget exceeded)", total, 20+maxExpandedCells)
	}
	if total < 20 {
		t.Errorf("real cells lost: %d < 20", total)
	}

	// The table still renders: every stored cell carries content or is a
	// marked placeholder, and the markdown pass terminates.
	tb := NewTrackedBuilder()
	maxCols := calculateMaxColumns(rows)
	extractTableAsMarkdown(rows, tb, maxCols)
	if !strings.Contains(tb.String(), "x") {
		t.Errorf("rendered table lost cell text: %q", tb.String())
	}
}

// TestCalculateMaxColumnsGridClamp: a tall, colspan-inflated table must not
// produce a numRows x maxCols grid beyond maxGridCells.
func TestCalculateMaxColumnsGridClamp(t *testing.T) {
	t.Parallel()

	const rows = 5000
	const width = 1000
	data := make([][]CellData, rows)
	for i := range data {
		data[i] = make([]CellData, width)
	}
	got := calculateMaxColumns(data)
	wantMax := maxGridCells / rows
	if got > wantMax {
		t.Errorf("calculateMaxColumns = %d, want <= %d (grid clamp not applied)", got, wantMax)
	}
	if got < 1 {
		t.Errorf("calculateMaxColumns = %d, want >= 1", got)
	}
}

// TestCalculateMaxColumnsSmallTableUnclamped: ordinary tables are untouched
// by the clamp.
func TestCalculateMaxColumnsSmallTableUnclamped(t *testing.T) {
	t.Parallel()

	data := [][]CellData{
		make([]CellData, 3),
		make([]CellData, 5),
	}
	if got := calculateMaxColumns(data); got != 5 {
		t.Errorf("calculateMaxColumns = %d, want 5", got)
	}
}

// TestCalculateMaxColumnsRowspanOffset pins the rowspan-driven column offset:
// a cell spanning rows reserves its column in the rows it covers, so a later
// row's cells are pushed past it. This exercises calculateMaxColumns'
// occupied-map loop (the branch TestCalculateMaxColumnsSmallTableUnclamped
// never reaches).
func TestCalculateMaxColumnsRowspanOffset(t *testing.T) {
	t.Parallel()

	data := [][]CellData{
		{{Rowspan: 2}, {Rowspan: 1}},
		{{Rowspan: 1}, {Rowspan: 1}},
	}
	if got := calculateMaxColumns(data); got != 3 {
		t.Errorf("calculateMaxColumns(rowspan offset) = %d, want 3", got)
	}
}

// ---------------------------------------------------------------------------
// Escaped-cell column widths (from width_escape_regression_test.go)
// ---------------------------------------------------------------------------

func TestColumnWidthCountsEscapes(t *testing.T) {
	t.Parallel()

	td := [][]CellData{
		{{Text: "a|b"}, {Text: "x"}},
		{{Text: "ccc"}, {Text: "yy"}},
	}
	w := calculateMaxColumnWidths(td, 2)

	// "a|b" has 3 runes but renders as "a\|b" (4 chars), so the column pads
	// to 4 and the plain "ccc" row aligns with it.
	if w[0] != 4 {
		t.Errorf("column width = %d, want 4 (rune count plus pipe escape)", w[0])
	}
	if w[1] != 2 {
		t.Errorf("plain column width = %d, want 2", w[1])
	}
}

// TestMarkdownRowsAlignWithEscapedCells is the end-to-end counterpart: the
// renderer's per-cell padding must measure the same width
// calculateMaxColumnWidths used. It previously counted runes only, so a cell
// containing '|' or '\' rendered wider than its column — the header row and
// the escaped data row came out different lengths.
func TestMarkdownRowsAlignWithEscapedCells(t *testing.T) {
	t.Parallel()

	td := [][]CellData{
		{{Text: "head"}, {Text: "x"}},
		{{Text: "a|b"}, {Text: "y"}},
		{{Text: "abcd"}, {Text: "z"}},
		{{Text: `back\slash`}, {Text: "w"}},
	}

	var tb TrackedBuilder
	extractTableAsMarkdown(td, &tb, 2)

	var widths []int
	for _, line := range strings.Split(strings.TrimRight(tb.String(), "\n"), "\n") {
		// The separator row carries fixed-width alignment markers and is not
		// padded to the column width by design.
		if strings.Contains(line, "---") {
			continue
		}
		widths = append(widths, len(line))
	}
	if len(widths) < 2 {
		t.Fatalf("unexpected output: %q", tb.String())
	}
	for i := 1; i < len(widths); i++ {
		if widths[i] != widths[0] {
			t.Fatalf("row %d rendered %d bytes wide, header row %d — cells containing a pipe or backslash knock the row out of alignment:\n%s",
				i, widths[i], widths[0], tb.String())
		}
	}
}

// ---------------------------------------------------------------------------
// Builder and renderer dispatch (from p2_regression_test.go)
// ---------------------------------------------------------------------------

func TestTrackedBuilderSecureClear(t *testing.T) {
	t.Parallel()
	tb := NewTrackedBuilder()
	_, _ = tb.WriteString("sensitive document text")
	tb.SecureClear()
	if tb.Len() != 0 || tb.LastChar != 0 {
		t.Errorf("SecureClear left len=%d lastChar=%d", tb.Len(), tb.LastChar)
	}
	// The builder must remain usable after SecureClear.
	if _, _ = tb.WriteString("x"); tb.String() != "x" {
		t.Errorf("builder unusable after SecureClear: %q", tb.String())
	}
}

func TestGetRendererDispatch(t *testing.T) {
	t.Parallel()
	if _, ok := getRenderer("html").(*HTMLRenderer); !ok {
		t.Error(`getRenderer("html") did not return HTMLRenderer`)
	}
	if _, ok := getRenderer("markdown").(*MarkdownRenderer); !ok {
		t.Error(`getRenderer("markdown") did not return MarkdownRenderer`)
	}
	if _, ok := getRenderer("no-such-format").(*MarkdownRenderer); !ok {
		t.Error("unknown format did not fall back to MarkdownRenderer")
	}
}
