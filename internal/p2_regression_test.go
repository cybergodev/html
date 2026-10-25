package internal

// p2_regression_test.go — regression tests for the D-002 P2 fixes in the
// internal package: charset-cache cardinality bound, latin-N alias mapping,
// UTF-8 pass-through validation and BOM stripping, meta-charset whitespace
// tolerance, CleanText trailing-space consistency, cache cleanup restart,
// URL helper edge cases, audit-URL rune-safe truncation, srcset/ping
// sanitization, and ScanMediaURLs rune-counted run cap.

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestNormalizeCharsetLatinFamily(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"latin1": "iso-8859-1", "latin-1": "iso-8859-1",
		"latin2": "iso-8859-2", "latin-2": "iso-8859-2",
		"latin9": "iso-8859-15", "latin-9": "iso-8859-15",
		"latin10": "iso-8859-16", "latin-10": "iso-8859-16",
	}
	for in, want := range cases {
		if got := normalizeCharset(in); got != want {
			t.Errorf("normalizeCharset(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToUTF8Latin2Decodes(t *testing.T) {
	t.Parallel()
	// "ěščř" encoded in ISO-8859-2 (IANA alias latin2): ě=0xEC š=0xB9 č=0xE8 ř=0xF8.
	ed := NewEncodingDetector()
	out, err := ed.ToUTF8([]byte{0xEC, 0xB9, 0xE8, 0xF8}, "latin2")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "ěščř" {
		t.Errorf("latin2 decode = %q, want %q", out, "ěščř")
	}
}

func TestToUTF8RejectsInvalidUTF8Passthrough(t *testing.T) {
	t.Parallel()
	ed := NewEncodingDetector()
	// A forced "utf-8" on invalid bytes must error, not return undecodable
	// data with a nil error.
	if _, err := ed.ToUTF8([]byte{0xEF, 0x00, 0xC3, 0x28, 0xFF}, "utf-8"); err == nil {
		t.Error("ToUTF8(invalid, \"utf-8\") = nil error, want error")
	}
}

func TestToUTF8StripsUTF8BOM(t *testing.T) {
	t.Parallel()
	ed := NewEncodingDetector()
	out, err := ed.ToUTF8([]byte{0xEF, 0xBB, 0xBF, 'h', 'i'}, "utf-8")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "hi" {
		t.Errorf("ToUTF8 with BOM = %q, want %q", out, "hi")
	}
}

func TestCharsetNormCacheCardinalityBounded(t *testing.T) {
	t.Parallel()
	count := func() int {
		n := 0
		charsetNormCache.Range(func(_, _ any) bool {
			n++
			return true
		})
		return n
	}
	before := count()
	for i := 0; i < maxCachedCharsetEntries+50; i++ {
		normalizeCharset("zz" + strings.Repeat("x", i%20+1) + string(rune('a'+i%26)) + itoa(i))
	}
	after := count()
	if after > before+maxCachedCharsetEntries {
		t.Errorf("cache grew by %d entries, want <= %d", after-before, maxCachedCharsetEntries)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestExtractCharsetFromBytesAllowsWhitespaceAroundEquals(t *testing.T) {
	t.Parallel()
	if got := extractCharsetFromBytes([]byte(`<meta charset = "gbk">`)); got != "gbk" {
		t.Errorf("extractCharsetFromBytes = %q, want gbk", got)
	}
	if got := extractCharsetFromBytes([]byte(`<meta charset="gbk">`)); got != "gbk" {
		t.Errorf("extractCharsetFromBytes (no spaces) = %q, want gbk", got)
	}
}

func TestCleanTextFastPathTrimsTrailingSpace(t *testing.T) {
	t.Parallel()
	if got := CleanText("hello "); got != "hello" {
		t.Errorf("CleanText(%q) = %q, want %q", "hello ", got, "hello")
	}
	if got := CleanText("hello\t"); got != "hello" {
		t.Errorf("CleanText(%q) = %q, want %q", "hello\t", got, "hello")
	}
	// Leading spaces are indent, preserved by both paths.
	if got := CleanText("  hello"); got != "  hello" {
		t.Errorf("CleanText(%q) = %q, want unchanged", "  hello", got)
	}
}

func TestCacheStartCleanupAfterStopReturnsUsableCancel(t *testing.T) {
	t.Parallel()
	c := NewCache[string](4, 0)
	cancel := c.StartCleanup(0)
	if cancel != nil {
		cancel()
	}
	c.StopCleanup()
	cancel2 := c.StartCleanup(0)
	if cancel2 == nil {
		t.Fatal("StartCleanup after StopCleanup returned nil cancel func")
	}
	cancel2() // must not panic
}

func TestExtractBaseFromURLQueryOnly(t *testing.T) {
	t.Parallel()
	if got := ExtractBaseFromURL("https://example.com?ref=x"); got != "https://example.com/" {
		t.Errorf("ExtractBaseFromURL = %q, want https://example.com/", got)
	}
	if got := ExtractBaseFromURL("https://example.com/p?ref=x"); got != "https://example.com/" {
		t.Errorf("ExtractBaseFromURL with path = %q, want https://example.com/", got)
	}
}

func TestTruncateAuditURLRuneSafe(t *testing.T) {
	t.Parallel()
	url := strings.Repeat("中", 300)
	got := TruncateAuditURL(url)
	if !utf8.ValidString(got) {
		t.Errorf("TruncateAuditURL produced invalid UTF-8: %q", got)
	}
	if TruncateAuditURL("short") != "short" {
		t.Error("TruncateAuditURL altered a short URL")
	}
}

type p2CountingRecorder struct{ blockedURLs int }

func (r *p2CountingRecorder) RecordBlockedTag(string)          {}
func (r *p2CountingRecorder) RecordBlockedAttr(string, string) {}
func (r *p2CountingRecorder) RecordBlockedURL(string, string)  { r.blockedURLs++ }

func TestSrcsetAndPingSanitized(t *testing.T) {
	t.Parallel()
	rec := &p2CountingRecorder{}
	SanitizeHTMLWithAudit(`<img src="ok.png" srcset="javascript:alert(1) 1x">`, rec)
	if rec.blockedURLs == 0 {
		t.Error("javascript: srcset value was not audited/blocked")
	}
	rec2 := &p2CountingRecorder{}
	SanitizeHTMLWithAudit(`<a href="ok.html" ping="data:text/html,x">y</a>`, rec2)
	if rec2.blockedURLs == 0 {
		t.Error("scriptable data: ping value was not audited/blocked")
	}
	rec3 := &p2CountingRecorder{}
	out := SanitizeHTMLWithAudit(`<img src="ok.png" srcset="a.png 1x, b.png 2x">`, rec3)
	if rec3.blockedURLs != 0 || !strings.Contains(out, `srcset="a.png 1x, b.png 2x"`) {
		t.Errorf("legitimate srcset altered or blocked: %q (blocked=%d)", out, rec3.blockedURLs)
	}
}

func TestScanMediaURLsMultibyteRunMatchesOracle(t *testing.T) {
	t.Parallel()
	// 250 CJK characters = 750 bytes: a byte-counted {1,500} cap used to
	// truncate the run before the ".mp4", while the oracle regex (which counts
	// runes) still matched.
	content := "https://a.test/" + strings.Repeat("中", 250) + ".mp4"
	var got []string
	ScanMediaURLs(content, true, 10, func(u string) bool {
		got = append(got, u)
		return true
	})
	re := regexp.MustCompile(`(?i)https?://[^\s<>"',;)}\]]{1,500}\.(?:mp4|m4v|webm)`)
	want := re.FindAllString(content, -1)
	if len(want) == 0 {
		t.Fatal("oracle regex found no match; test input is wrong")
	}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("ScanMediaURLs = %v, oracle = %v", got, want)
	}
}
