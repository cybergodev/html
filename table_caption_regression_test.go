package html

// table_caption_regression_test.go — regression for the D-002 P1 fix: the
// table dispatch handed the whole <table> subtree to the row-based table
// extractor and returned, and that extractor collects only <tr> subtrees —
// so the visible text of a <caption> child vanished from every output
// format. The caption is now emitted as its own line ahead of the table.

import (
	"strings"
	"testing"
)

const captionDoc = "<html><body>" +
	"<table><caption>Quarterly Results</caption>" +
	"<tr><th>Region</th><th>Revenue</th></tr>" +
	"<tr><td>EMEA</td><td>42</td></tr>" +
	"</table></body></html>"

func TestTableCaptionEmitted(t *testing.T) {
	t.Parallel()

	res, err := Extract([]byte(captionDoc))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !strings.Contains(res.Text, "Quarterly Results") {
		t.Errorf("caption text lost from Text: %q", res.Text)
	}
	// The table itself must still render after the caption line.
	if !strings.Contains(res.Text, "|") {
		t.Errorf("table rows lost from Text: %q", res.Text)
	}

	// Same guarantee for the HTML table format.
	cfg := DefaultConfig()
	cfg.TableFormat = "html"
	res2, err := Extract([]byte(captionDoc), cfg)
	if err != nil {
		t.Fatalf("Extract(html): %v", err)
	}
	if !strings.Contains(res2.Text, "Quarterly Results") {
		t.Errorf("caption text lost from HTML-format output: %q", res2.Text)
	}
}
