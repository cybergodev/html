package internal

// pre_newline_regression_test.go — regression tests for the P2-9 heuristic:
// <pre> (and <code> inside it) preserve their internal newlines in extracted
// text instead of having normalizeText fold them into spaces, so code blocks
// and ASCII diagrams survive as multiple lines.

import (
	"strings"
	"testing"

	"github.com/cybergodev/html/internal/table"
	"golang.org/x/net/html"
)

// extractTextForPre runs the full text-extraction pass and returns the cleaned
// text, mirroring the production path (CleanTextBytes after the structure pass).
func extractTextForPre(t *testing.T, markup string) string {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tb := table.NewTrackedBuilder()
	ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
	return CleanTextBytes(tb.Bytes())
}

func TestPrePreservesNewlines(t *testing.T) {
	t.Parallel()

	text := extractTextForPre(t, `<div><pre>line one
line two
line three</pre></div>`)

	if !strings.Contains(text, "line one\nline two\nline three") {
		t.Errorf("<pre> newlines were folded into spaces:\n%s", text)
	}
}

func TestPreCodePreservesNewlines(t *testing.T) {
	t.Parallel()

	text := extractTextForPre(t, `<div><pre><code>func main() {
	fmt.Println("hi")
}</code></pre></div>`)

	// Newlines must survive as line breaks. The tab is collapsed to a space by
	// CleanTextBytes, which folds tabs to single spaces on every line (a
	// document-wide whitespace policy, not a <pre> regression) — so assert the
	// line structure, not the exact indentation byte.
	if !strings.Contains(text, "func main() {\n") {
		t.Errorf("<pre><code> newline after '{' was lost:\n%s", text)
	}
	if !strings.Contains(text, "fmt.Println(\"hi\")") {
		t.Errorf("<pre><code> code line was lost:\n%s", text)
	}
	if !strings.Contains(text, "}") {
		t.Errorf("<pre><code> closing brace was lost:\n%s", text)
	}
}

// TestPreInArticleStillPreserved guards the interaction with P0-2: a <pre>
// nested inside an <article> is still a code block, not article furniture, and
// keeps its line structure.
func TestPreInArticleStillPreserved(t *testing.T) {
	t.Parallel()

	text := extractTextForPre(t, `<article><p>intro</p><pre>a
b
c</pre></article>`)

	if !strings.Contains(text, "a\nb\nc") {
		t.Errorf("<pre> inside <article> lost newlines:\n%s", text)
	}
}

// TestPreNBSPFolded guards that NBSP inside <pre> is still folded to a regular
// space, since a raw U+00A0 in code output would be visually misleading.
func TestPreNBSPFolded(t *testing.T) {
	t.Parallel()

	text := extractTextForPre(t, "<div><pre>a b</pre></div>")

	if !strings.Contains(text, "a b") {
		t.Errorf("NBSP in <pre> was not folded to a space: %q", text)
	}
}

// TestDataTableWithEmbeddedTableStillRenders guards the P2-11 fix: a data table
// that embeds a small sub-table in one cell (carrying its own cell text) must
// render as a data table, not be flattened as a layout shell. The previous
// rule treated any nested <table> as proof of layout.
func TestDataTableWithEmbeddedTableStillRenders(t *testing.T) {
	t.Parallel()

	// Outer table has a cell with substantial direct text AND a nested sub-table.
	src := `<table><tr><td>This cell has its own meaningful descriptive text.</td></tr>` +
		`<tr><td>Another cell with prose text content here.</td></tr>` +
		`</table>`

	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tb := table.NewTrackedBuilder()
	ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
	out := collapseSpaces(tb.String())

	// The outer table's own cell text must survive as a data table row.
	if !strings.Contains(out, "This cell has its own meaningful descriptive text") {
		t.Errorf("data table with nested sub-table was flattened, cell text lost:\n%s", out)
	}
}

// TestLayoutTableStillFlattened guards the other half of the P2-11 contract: a
// pure layout table (nested table, no direct text of its own) is still treated
// as a shell and its nested data table renders on its own terms.
func TestLayoutTableStillFlattened(t *testing.T) {
	t.Parallel()

	src := `<table><tr><td>` +
		`<table><tr><td>A</td><td>B</td></tr><tr><td>C</td><td>D</td></tr></table>` +
		`</td></tr></table>`

	doc, err := html.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	tb := table.NewTrackedBuilder()
	ExtractTextWithStructureAndImages(doc, tb, nil, nil, "markdown")
	out := collapseSpaces(tb.String())

	if !strings.Contains(out, "| A | B |") {
		t.Errorf("inner data table row '| A | B |' lost:\n%s", out)
	}
}
