package html

// tableformat_case_regression_test.go — end-to-end regression for the D-002
// P0 fix: Config.Validate accepts TableFormat case-insensitively while New
// normalized only the inline formats, so TableFormat: "HTML" flowed raw into
// the table pipeline. extractTableData lowercased only its local copy (HTML
// data path: colspan kept, structure rows kept) while getRenderer switched
// on the exact lowercased name and fell back to the Markdown renderer —
// producing a Markdown pipe table with unexpanded colspans and leaked
// width-definition rows.

import (
	"strings"
	"testing"
)

const mixedCaseTableDoc = "<html><body><h1>Report</h1>" +
	"<table><tr><th colspan=\"2\">H1</th></tr><tr><td>a</td><td>b</td></tr></table>" +
	"</body></html>"

func TestExtractTableFormatMixedCaseHTML(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.TableFormat = "HTML" // Validate accepts it; the pipeline must honor it

	res, err := Extract([]byte(mixedCaseTableDoc), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// The HTML renderer emits a real <table> with the colspan attribute
	// intact (the HTML data path never expands spans).
	if !strings.Contains(res.Text, "<table>") {
		t.Errorf("TableFormat %q did not render HTML: %q", "HTML", res.Text)
	}
	if !strings.Contains(res.Text, `colspan="2"`) {
		t.Errorf("HTML output lost the colspan attribute: %q", res.Text)
	}

	// The document contains no pipe characters, so any Markdown pipe table
	// in the output proves the renderer mismatch is back.
	if strings.Contains(res.Text, "|") {
		t.Errorf("TableFormat %q rendered a Markdown pipe table: %q", "HTML", res.Text)
	}
}

func TestExtractTableFormatMixedCaseMarkdown(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.TableFormat = "Markdown"

	res, err := Extract([]byte(mixedCaseTableDoc), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Mixed-case "Markdown" must render the same pipe table as the
	// canonical lowercase value: header row expanded to two columns.
	if !strings.Contains(res.Text, "|") {
		t.Errorf("TableFormat %q did not render a Markdown table: %q", "Markdown", res.Text)
	}
	if strings.Contains(res.Text, "<table>") {
		t.Errorf("TableFormat %q rendered HTML output: %q", "Markdown", res.Text)
	}
}
