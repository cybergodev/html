package html

// regressions_table_test.go — regressions for the D-002 table-rendering fixes,
// gathered from the former single-topic files:
//
//   - a <caption> child is now emitted as its own line ahead of the table, in
//     both the Markdown and HTML table formats;
//   - Config.Validate accepts TableFormat case-insensitively and the pipeline
//     honors it end-to-end, so "HTML"/"Markdown" render as their lowercase
//     canonical forms.

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

const mixedCaseTableDoc = "<html><body><h1>Report</h1>" +
	"<table><tr><th colspan=\"2\">H1</th></tr><tr><td>a</td><td>b</td></tr></table>" +
	"</body></html>"

func TestTableLinkURLPipeDoesNotSplitCells(t *testing.T) {
	t.Parallel()

	// A '|' inside a substituted link URL would end the Markdown table cell
	// at substitution time (the table escaper only sees the pre-substitution
	// [LINK:n] text): the URL is percent-encoded so the row keeps its shape.
	doc := `<html><body><article><table>
		<tr><th>Link</th><th>Note</th></tr>
		<tr><td><a href="/x?a=1|b=2">L</a></td><td>tail</td></tr>
	</table></article></body></html>`

	cfg := DefaultConfig()
	cfg.InlineLinkFormat = "markdown"
	cfg.MaxCacheEntries = 0
	out, err := ExtractToMarkdown([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ExtractToMarkdown: %v", err)
	}
	if !strings.Contains(out, "[L](/x?a=1%7Cb=2)") {
		t.Errorf("pipe in URL not percent-encoded:\n%s", out)
	}
	if strings.Contains(out, "a=1|b=2") {
		t.Errorf("raw pipe survived into the rendered row:\n%s", out)
	}
}

func TestTableSecondCaptionBurnsCounters(t *testing.T) {
	t.Parallel()

	// A second <caption> is a parse artifact (the spec closes the first on a
	// second start tag) that is NOT rendered, but the section-wide collector
	// still numbered its anchors. Without the counter-burn walk over it, the
	// link AFTER the table received the caption's number and rendered with
	// the caption's URL.
	doc := `<html><body><article>
		<table>
			<caption>First caption</caption>
			<caption><a href="/cap-link">caplink</a></caption>
			<tr><td>cell</td></tr>
		</table>
		<p>See <a href="/after">after</a> the table.</p>
	</article></body></html>`

	cfg := DefaultConfig()
	cfg.InlineLinkFormat = "markdown"
	cfg.MaxCacheEntries = 0
	out, err := ExtractToMarkdown([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ExtractToMarkdown: %v", err)
	}
	if !strings.Contains(out, "[after](/after)") {
		t.Errorf("post-table link rendered with the burned caption's number:\n%s", out)
	}
	if strings.Contains(out, "[after](/cap-link)") {
		t.Errorf("post-table link bound to the second caption's URL:\n%s", out)
	}
}

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
