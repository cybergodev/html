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
//   - TrackedBuilder.SecureClear (pool secure-clear support) and the
//     getRenderer format dispatch that replaced the unused registry.
//
// The former width-escape regressions (column-width counting and padded-row
// alignment) were removed with the padding machinery itself: public output
// passes through CleanTextBytes, which collapses the padding, so the
// machinery was deleted (owner decision) rather than kept aligned.

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
	data := p.extractTableData(parseCapTestTable(t, markup), "markdown", nil)

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
	rows := p.extractTableData(parseCapTestTable(t, markup), "markdown", nil)

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

// ---------------------------------------------------------------------------
// Unexported render helpers (from render_internal_test.go)
// ---------------------------------------------------------------------------

// TestSpannedFill pins spannedFill's empty-text substitution: an empty cell
// body becomes a single space so the rendered cell keeps its column, while
// non-empty text passes through unchanged.
func TestSpannedFill(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty text becomes single space", "", " "},
		{"non-empty text unchanged", "value", "value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := spannedFill(tt.in)
			if got.Text != tt.want {
				t.Errorf("spannedFill(%q).Text = %q, want %q", tt.in, got.Text, tt.want)
			}
			if got.Align != AlignDefault || got.Colspan != 1 || got.Rowspan != 1 {
				t.Errorf("spannedFill(%q) = %+v, want default alignment and 1x1 span", tt.in, got)
			}
		})
	}
}

// TestSanitizeCellTextEmptyString pins sanitizeCellText's empty-string
// substitution: a blank cell body becomes a single space so the rendered cell
// keeps its column, while non-empty text passes through trimmed. The
// external table_test package used to pin this through a mirrored helper
// (sanitizeTestCellText); that mirror and its test were removed and the
// rows live here against the production function.
func TestSanitizeCellTextEmptyString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty becomes single space", "", " "},
		{"whitespace becomes single space", "   \t\n", " "},
		{"non-empty trimmed", "  hello  ", "hello"},
		{"newlines folded and trimmed", "\n\nText\n\n", "Text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sanitizeCellText(tt.in); got != tt.want {
				t.Errorf("sanitizeCellText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestExtractWithCellTextOverride pins the cell-text override at the table
// layer (the root package exercises it end-to-end through ReadPage's section
// rendering; ExtractWithCellText and its appendRowCells callback branches had
// no in-package test): cellText replaces the accessor's GetTextContent for
// td/th cells, it is invoked with the result DISCARDED for non-cell row
// children so shared counters stay aligned (the counter-burn contract), and
// a nil callback renders exactly like Extract.
func TestExtractWithCellTextOverride(t *testing.T) {
	t.Parallel()

	const markup = `<table>
		<tr><th id="h">Head</th></tr>
		<tr><td id="c">cell</td><template id="tpl"><a href="/x">burned</a></template></tr>
	</table>`

	var sawHeader, sawCell, sawTemplate bool
	cellText := func(n *html.Node) string {
		switch n.Data {
		case "th":
			sawHeader = true
		case "td":
			sawCell = true
		case "template":
			sawTemplate = true // counter-burn call: text discarded
		}
		return "CB:" + n.Data
	}

	p := NewProcessor(capTestAccessor{}, capTestWalker{})
	tb := NewTrackedBuilder()
	p.ExtractWithCellText(parseCapTestTable(t, markup), tb, "markdown", cellText)
	out := tb.String()

	if !strings.Contains(out, "CB:th") || !strings.Contains(out, "CB:td") {
		t.Errorf("cell-text override did not reach the rendered cells: %q", out)
	}
	if !sawHeader || !sawCell || !sawTemplate {
		t.Errorf("callback coverage: header=%v cell=%v template-burn=%v, want all true", sawHeader, sawCell, sawTemplate)
	}
	if strings.Contains(out, "burned") {
		t.Errorf("template counter-burn call leaked its text into the row: %q", out)
	}

	// A nil callback takes the accessor path and matches Extract byte-for-byte.
	tbl := parseCapTestTable(t, markup)
	tbNil, tbPlain := NewTrackedBuilder(), NewTrackedBuilder()
	p.ExtractWithCellText(tbl, tbNil, "markdown", nil)
	p.Extract(tbl, tbPlain, "markdown")
	if tbNil.String() != tbPlain.String() {
		t.Errorf("nil cellText diverged from Extract:\n%q\n%q", tbNil.String(), tbPlain.String())
	}
}

// ---------------------------------------------------------------------------
// Rowspan grid, alignment voting, and cell escaping boundaries
// ([FIX-001] boundary additions)
// ---------------------------------------------------------------------------

// TestApplyRowspanGridBoundaries drives the grid rebuild's guard and fill
// branches directly: degenerate inputs return the input unchanged, cells past
// maxCols are dropped like the renderer drops them, and trailing columns
// reserved by a rowspan from above are filled with spanned repeats.
func TestApplyRowspanGridBoundaries(t *testing.T) {
	t.Parallel()

	// Degenerate shapes: identity, no allocation.
	if got := applyRowspanGrid(nil, 3); got != nil {
		t.Errorf("applyRowspanGrid(nil, 3) = %v, want nil", got)
	}
	src := [][]CellData{{{Text: "a", Rowspan: 2}}}
	if got := applyRowspanGrid(src, 0); !sameGrid(got, src) {
		t.Errorf("applyRowspanGrid(data, 0) rebuilt the grid, want identity: %v", got)
	}

	// A rowspan origin in the last column with a short following row: the
	// spanned column is filled at the trailing-fill stage (nothing follows it).
	grid := applyRowspanGrid([][]CellData{
		{{Text: "l"}, {Text: "span", Rowspan: 2}},
		{{Text: "r"}},
	}, 2)
	if len(grid) != 2 || len(grid[1]) != 2 {
		t.Fatalf("grid shape = %dx?, want 2x2", len(grid))
	}
	if grid[0][1].Text != "span" || grid[1][0].Text != "r" {
		t.Errorf("rowspan placement wrong: %+v / %+v", grid[0], grid[1])
	}
	if grid[1][1].Text != "span" || grid[1][1].Align != AlignDefault {
		t.Errorf("trailing spanned fill = %+v, want the origin text %q with default alignment", grid[1][1], "span")
	}

	// More cells than maxCols: the surplus is dropped, matching the renderer.
	wide := [][]CellData{{{Text: "1", Rowspan: 2}, {Text: "2", Rowspan: 2}, {Text: "3", Rowspan: 2}}}
	grid = applyRowspanGrid(wide, 2)
	for _, cell := range grid[0] {
		if cell.Text == "3" {
			t.Errorf("cell past maxCols survived: %+v", grid[0])
		}
	}
}

// sameGrid compares two grids by cell text and span, for the identity check.
func sameGrid(a, b [][]CellData) bool {
	if len(a) != len(b) {
		return false
	}
	for r := range a {
		if len(a[r]) != len(b[r]) {
			return false
		}
		for c := range a[r] {
			if a[r][c].Text != b[r][c].Text || a[r][c].Rowspan != b[r][c].Rowspan {
				return false
			}
		}
	}
	return true
}

// TestDetermineColumnAlignmentVotes pins the vote branches not reached by the
// fixture tables: a justify majority, a left+right tie refusing to pick a
// side (plain "---"), and the zero-vote fallback to the first row's cell.
func TestDetermineColumnAlignmentVotes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		counts   AlignCount
		firstRow []CellData
		want     string
	}{
		{"justify majority", AlignCount{Justify: 2}, nil, "---"},
		{"mixed left and right defuses to default", AlignCount{Left: 1, Right: 1}, nil, "---"},
		{"zero votes fall back to first row", AlignCount{}, []CellData{{Align: AlignRight}}, "---:"},
		{"center majority", AlignCount{Center: 3}, nil, ":--:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := determineColumnAlignment(tt.counts, tt.firstRow, 0); got != tt.want {
				t.Errorf("determineColumnAlignment(%+v) = %q, want %q", tt.counts, got, tt.want)
			}
		})
	}
}

// TestWriteMarkdownCellTextControlChars pins the escaping of every control
// character the cell writer handles, including the carriage return the
// fixture-based tests never contained.
func TestWriteMarkdownCellTextControlChars(t *testing.T) {
	t.Parallel()

	tb := NewTrackedBuilder()
	writeMarkdownCellText(tb, "a|b\\c\nd\re")
	if got, want := tb.String(), `a\|b\\c d e`; got != want {
		t.Errorf("writeMarkdownCellText = %q, want %q", got, want)
	}
}
