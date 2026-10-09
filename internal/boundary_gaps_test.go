package internal

// boundary_gaps_test.go — table-driven tests for small unexported helpers
// whose uncovered branches (nil guards, error paths, format edges) were below
// the suite's per-function targets: NoOpAuditRecorder (0%), htmlCellAccessor
// nil guards (67%), asDirectoryBase (33%), StringToBytes/BytesToString empty
// inputs (67%), isValidMediaType format edges (70%), and the SanitizeHTMLWithAudit
// empty/error branches (55%).

import (
	"strings"
	"testing"

	"github.com/cybergodev/html/internal/table"
)

// ---------------------------------------------------------------------------
// NoOpAuditRecorder
// ---------------------------------------------------------------------------

// TestNoOpAuditRecorder pins that the no-op recorder accepts every call and
// records nothing — the guarantee sanitization relies on when audit is off.
func TestNoOpAuditRecorder(t *testing.T) {
	t.Parallel()

	var r AuditRecorder = NoOpAuditRecorder{}
	r.RecordBlockedTag("script")
	r.RecordBlockedAttr("onclick", "evil()")
	r.RecordBlockedURL("javascript:alert(1)", "javascript scheme")

	// No panic and no observable state is the entire contract.
}

// ---------------------------------------------------------------------------
// htmlCellAccessor nil guards
// ---------------------------------------------------------------------------

// TestHTMLCellAccessorNilGuards pins the nil-node branches of every
// htmlCellAccessor method (internal/table.go).
func TestHTMLCellAccessorNilGuards(t *testing.T) {
	t.Parallel()

	a := &htmlCellAccessor{}
	if got := a.GetAlignment(nil); got != table.AlignDefault {
		t.Errorf("GetAlignment(nil) = %v, want AlignDefault", got)
	}
	if got := a.GetColSpan(nil); got != 1 {
		t.Errorf("GetColSpan(nil) = %d, want 1", got)
	}
	if got := a.GetRowSpan(nil); got != 1 {
		t.Errorf("GetRowSpan(nil) = %d, want 1", got)
	}
	if got := a.GetWidth(nil); got != "" {
		t.Errorf("GetWidth(nil) = %q, want empty", got)
	}
	if got := a.GetTextContent(nil); got != "" {
		t.Errorf("GetTextContent(nil) = %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// asDirectoryBase
// ---------------------------------------------------------------------------

// TestAsDirectoryBase pins all three branches of asDirectoryBase: an
// already-directory base is unchanged, a file-style base drops its last
// segment, and an authority-only base gains a trailing slash.
func TestAsDirectoryBase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		want string
	}{
		{"already a directory base", "https://example.com/path/", "https://example.com/path/"},
		{"file-style base drops last segment", "https://example.com/path/page.html", "https://example.com/path/"},
		{"root file drops to root", "https://example.com/page.html", "https://example.com/"},
		{"authority without path gains slash", "http://example.com", "http://example.com/"},
		{"relative directory base unchanged", "dir/sub/", "dir/sub/"},
		{"relative file base drops to its directory", "dir/sub/page.html", "dir/sub/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := asDirectoryBase(tt.base); got != tt.want {
				t.Errorf("asDirectoryBase(%q) = %q, want %q", tt.base, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// unsafe conversions
// ---------------------------------------------------------------------------

// TestUnsafeConversionsEmpty pins the zero-length fast paths of the unsafe
// conversion helpers.
func TestUnsafeConversionsEmpty(t *testing.T) {
	t.Parallel()

	if got := BytesToString(nil); got != "" {
		t.Errorf("BytesToString(nil) = %q, want empty", got)
	}
	if got := BytesToString([]byte{}); got != "" {
		t.Errorf("BytesToString(empty) = %q, want empty", got)
	}
	if got := StringToBytes(""); got != nil {
		t.Errorf("StringToBytes(\"\") = %v, want nil", got)
	}

	// Round-trip on non-empty input.
	s := "héllo 世界"
	if got := string(StringToBytes(s)); got != s {
		t.Errorf("StringToBytes round-trip = %q, want %q", got, s)
	}
	if got := BytesToString([]byte(s)); got != s {
		t.Errorf("BytesToString round-trip = %q, want %q", got, s)
	}
}

// ---------------------------------------------------------------------------
// isValidMediaType
// ---------------------------------------------------------------------------

// TestIsValidMediaType pins the data-URL media-type validator at its format
// edges: empty, missing/misplaced slash, trailing slash, and disallowed
// characters.
func TestIsValidMediaType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mediaType string
		want      bool
	}{
		{"image/png", true},
		{"image/svg+xml", true},
		{"text/plain", true},
		{"application/x-custom.v2+json", true},
		{"", false},            // empty
		{"imagepng", false},    // no slash
		{"/png", false},        // slash at index 0
		{"image/", false},      // trailing slash
		{"image/png ", false},  // whitespace
		{"image/pn;g", false},  // semicolon
		{"image/pn\"g", false}, // quote
		{"image/pn(g)", false}, // parentheses
	}

	for _, tt := range tests {
		t.Run(tt.mediaType, func(t *testing.T) {
			t.Parallel()
			if got := isValidMediaType(tt.mediaType); got != tt.want {
				t.Errorf("isValidMediaType(%q) = %v, want %v", tt.mediaType, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// SanitizeHTMLWithAudit
// ---------------------------------------------------------------------------

// recordingAudit captures blocked-tag events for SanitizeHTMLWithAudit tests.
type recordingAudit struct {
	tags []string
}

func (r *recordingAudit) RecordBlockedTag(tag string)      { r.tags = append(r.tags, tag) }
func (r *recordingAudit) RecordBlockedAttr(string, string) {}
func (r *recordingAudit) RecordBlockedURL(string, string)  {}

// TestSanitizeHTMLWithAuditBoundaries covers the empty-input early return and
// the body extraction path of SanitizeHTMLWithAudit, asserting that blocked
// tags are reported through the recorder.
func TestSanitizeHTMLWithAuditBoundaries(t *testing.T) {
	t.Parallel()

	t.Run("empty input returns empty", func(t *testing.T) {
		t.Parallel()
		if got := SanitizeHTMLWithAudit("", &recordingAudit{}); got != "" {
			t.Errorf("SanitizeHTMLWithAudit(\"\") = %q, want empty", got)
		}
	})

	t.Run("dangerous tags removed and reported", func(t *testing.T) {
		t.Parallel()
		audit := &recordingAudit{}
		got := SanitizeHTMLWithAudit(
			`<p>keep <b>this</b></p><script>alert(1)</script>`, audit)

		if strings.Contains(got, "<script>") || strings.Contains(got, "alert(1)") {
			t.Errorf("script tag survived sanitization: %q", got)
		}
		if !strings.Contains(got, "keep") {
			t.Errorf("benign content was dropped: %q", got)
		}
		found := false
		for _, tag := range audit.tags {
			if tag == "script" {
				found = true
			}
		}
		if !found {
			t.Errorf("blocked <script> removal was not reported; got %v", audit.tags)
		}
	})

	t.Run("NoOp recorder keeps sanitization silent", func(t *testing.T) {
		t.Parallel()
		// Contract note: SanitizeHTMLWithAudit requires a non-nil recorder —
		// a nil interface panics on the first blocked tag (sanitize.go calls
		// audit.RecordBlockedTag unconditionally). SanitizeHTML satisfies
		// this by always passing NoOpAuditRecorder{}; callers that want no
		// auditing must do the same.
		got := SanitizeHTMLWithAudit(`<p>content</p><script>x</script>`, NoOpAuditRecorder{})
		if strings.Contains(got, "<script>") {
			t.Errorf("script tag survived sanitization: %q", got)
		}
	})
}

// Compile-time check that recordingAudit satisfies the recorder interface.
var _ AuditRecorder = (*recordingAudit)(nil)

// ---------------------------------------------------------------------------
// encoding helpers
// ---------------------------------------------------------------------------

// TestExtractCharsetFromHTMLEmpleteness pins the empty-input branch of the
// string convenience wrapper and its delegation to the byte scanner.
func TestExtractCharsetFromHTMLEmpleteness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want string
	}{
		{"empty input", "", ""},
		{"meta charset tag", `<html><head><meta charset="utf-8"></head>`, "utf-8"},
		{"content-type form", `<meta http-equiv="Content-Type" content="text/html; charset=gbk">`, "gbk"},
		{"no charset present", `<html><head><title>x</title></head>`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := extractCharsetFromHTML(tt.html); got != tt.want {
				t.Errorf("extractCharsetFromHTML(%q) = %q, want %q", tt.html, got, tt.want)
			}
		})
	}
}

// TestSampleSizeOrDefault pins both branches: an explicit positive sample size
// wins, and an unset (zero) value falls back to the documented 10240 default.
func TestSampleSizeOrDefault(t *testing.T) {
	t.Parallel()

	t.Run("explicit size honored", func(t *testing.T) {
		t.Parallel()
		ed := &EncodingDetector{MaxSampleSize: 512}
		if got := ed.sampleSizeOrDefault(); got != 512 {
			t.Errorf("sampleSizeOrDefault() = %d, want 512", got)
		}
	})

	t.Run("unset falls back to default", func(t *testing.T) {
		t.Parallel()
		ed := &EncodingDetector{}
		if got := ed.sampleSizeOrDefault(); got != 10240 {
			t.Errorf("sampleSizeOrDefault() = %d, want 10240", got)
		}
	})
}
