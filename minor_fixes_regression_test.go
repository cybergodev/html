package html

// minor_fixes_regression_test.go — regressions for the D-002 P3 behavior
// fixes: percent-encoded spaces in Markdown link/image destinations, the
// ErrBatchTooLarge sentinel for oversized batches, TrimSpace-aligned format
// validation, case-normalized LevelFilteredSink thresholds, Metadata maps no
// longer aliased by GetEntries, Windows OS error wordings mapped to their
// sanitized categories, and link titles free of script/style text.

import (
	"errors"
	"strings"
	"testing"
)

func TestMarkdownURLDestSpacesEncoded(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.InlineImageFormat = "markdown"
	cfg.InlineLinkFormat = "markdown"

	doc := `<html><body><p><img src="my file.png" alt="pic"> and <a href="docs my page.html">docs</a></p></body></html>`
	res, err := Extract([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !strings.Contains(res.Text, "![pic](my%20file.png)") {
		t.Errorf("image destination not space-escaped: %q", res.Text)
	}
	if !strings.Contains(res.Text, "[docs](docs%20my%20page.html)") {
		t.Errorf("link destination not space-escaped: %q", res.Text)
	}
}

func TestBatchTooLargeSentinel(t *testing.T) {
	t.Parallel()

	inputs := make([][]byte, maxBatchSize+1)
	for i := range inputs {
		inputs[i] = []byte("<p>x</p>")
	}
	br := ExtractBatch(inputs)
	if len(br.Errors) != len(inputs) {
		t.Fatalf("got %d errors, want %d", len(br.Errors), len(inputs))
	}
	for i, err := range br.Errors {
		if !errors.Is(err, ErrBatchTooLarge) {
			t.Fatalf("Errors[%d] = %v, want ErrBatchTooLarge identity", i, err)
		}
	}
}

func TestValidateFormatTrimsSpace(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.InlineImageFormat = " markdown " // padded, mixed case
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate rejected a padded format it would normalize: %v", err)
	}

	res, err := Extract([]byte(`<html><body><p><img src="a.png" alt="x"></p></body></html>`), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !strings.Contains(res.Text, "![x](a.png)") {
		t.Errorf("padded format did not normalize to markdown: %q", res.Text)
	}
}

// p3RecordingSink captures written entries for level-filter assertions.
type p3RecordingSink struct {
	entries []AuditEntry
}

func (s *p3RecordingSink) Write(e AuditEntry) { s.entries = append(s.entries, e) }
func (s *p3RecordingSink) Close() error       { return nil }

func TestLevelFilteredSinkNormalizesCase(t *testing.T) {
	t.Parallel()

	rec := &p3RecordingSink{}
	lf := NewLevelFilteredSink(rec, "Critical") // capitalized, not the constant

	lf.Write(AuditEntry{Level: AuditLevelInfo, EventType: "e1"})
	lf.Write(AuditEntry{Level: AuditLevelCritical, EventType: "e2"})

	if len(rec.entries) != 1 || rec.entries[0].EventType != "e2" {
		t.Errorf("capitalized minLevel filtered wrong entries: %+v", rec.entries)
	}
}

func TestGetEntriesMetadataNotAliased(t *testing.T) {
	t.Parallel()

	c := newAuditCollector(AuditConfig{
		Enabled:           true,
		LogEncodingIssues: true,
	})
	c.RecordEncodingIssue("utf-8", "boom")

	a := c.GetEntries()
	b := c.GetEntries()
	if len(a) != 1 || a[0].Metadata == nil {
		t.Fatalf("expected one entry with Metadata, got %+v", a)
	}
	a[0].Metadata["injected"] = "via alias"
	if _, ok := b[0].Metadata["injected"]; ok {
		t.Error("GetEntries returned Metadata maps aliased with the collector's entries")
	}
}

func TestFileErrorWindowsWordingsSanitized(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw  string
		want string
	}{
		{"open C:\\dir\\file.html: The system cannot find the file specified.", "file not found"},
		{"open C:\\dir\\file.html: Access is denied.", "permission denied"},
		{"read C:\\dir\\sub: The process cannot access the file because it is being used by another process.", "file operation failed"},
	}
	for _, c := range cases {
		fe := newFileError("ReadFile", `C:\dir\file.html`, errors.New(c.raw))
		if !strings.Contains(fe.Error(), c.want) {
			t.Errorf("sanitized message = %q, want it to contain %q (raw %q)", fe.Error(), c.want, c.raw)
		}
	}
}

func TestLinkTitleExcludesScriptText(t *testing.T) {
	t.Parallel()

	doc := `<html><body><a href="https://x.com/page"><script>evil()</script>click me<style>.x{color:red}</style></a></body></html>`
	res, err := ExtractAllLinks([]byte(doc))
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d links, want 1", len(res))
	}
	if got := res[0].Title; got != "click me" {
		t.Errorf("link Title = %q, want %q (script/style bodies must not leak)", got, "click me")
	}
}
