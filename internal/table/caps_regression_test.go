package table

// caps_regression_test.go — regression tests for the hostile-table
// allocation caps:
//   - maxExpandedCells bounds the aggregate colspan placeholder count per
//     table (placeholders are synthetic cells no node-walk cap can see;
//     without the budget, ~50k <td colspan="1000"> cells expanded into tens
//     of millions of CellData), and
//   - maxGridCells clamps the numRows x maxCols rendering grid that
//     padTableColumns/applyRowspanGrid/per-column slices allocate.

import (
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

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

// capTestWalker visits the subtree depth-first, honoring a false return by
// not descending further.
type capTestWalker struct{}

func (capTestWalker) Walk(n *html.Node, cb func(*html.Node) bool) {
	var rec func(*html.Node) bool
	rec = func(n *html.Node) bool {
		if n == nil || !cb(n) {
			return false
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if !rec(c) {
				return false
			}
		}
		return true
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

// TestExtractTableDataExpansionCap: 20 cells at colspan=1000 would expand to
// 20,000 placeholders without the per-table budget; with it, stored cells
// stay within real cells + maxExpandedCells and rendering still completes.
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
