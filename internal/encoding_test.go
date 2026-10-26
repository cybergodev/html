package internal

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
)

// TestDeclaredCharsetCoverage pins declared-charset support for the legacy
// encodings real pages declare but x/text newcomers used to miss: a document
// that DECLARES koi8-r / windows-125x / gb18030 must decode through its
// declaration (previously "unsupported encoding" on basic-only paths or a
// statistical mis-pick). Statistical candidates are deliberately unchanged —
// only the declared path is covered.
func TestDeclaredCharsetCoverage(t *testing.T) {
	t.Parallel()

	russian := "Справка по кодировкам"
	greek := "Ελληνική σελίδα"
	turkish := "Türkçe sayfa"
	chinese := "汉字编码测试文档"

	encode := func(e encoding.Encoding, s string) []byte {
		b, err := e.NewEncoder().Bytes([]byte(s))
		if err != nil {
			t.Fatalf("encode fixture: %v", err)
		}
		return b
	}

	cases := []struct {
		charset string
		doc     []byte
		want    string
	}{
		{"koi8-r", encode(charmap.KOI8R, russian), russian},
		{"windows-1253", encode(charmap.Windows1253, greek), greek},
		{"windows-1254", encode(charmap.Windows1254, turkish), turkish},
		{"gb18030", encode(simplifiedchinese.GB18030, chinese), chinese},
	}
	for _, c := range cases {
		doc := append([]byte(`<html><head><meta charset="`+c.charset+`"></head><body><p>`),
			append(append([]byte{}, c.doc...), []byte(`</p></body></html>`)...)...)
		ed := &EncodingDetector{}
		charset := ed.DetectCharset(doc)
		if charset != c.charset {
			t.Errorf("DetectCharset(%s doc) = %q, want %q", c.charset, charset, c.charset)
			continue
		}
		decoded, err := ed.ToUTF8(doc, charset)
		if err != nil {
			t.Errorf("ToUTF8(%s) error: %v", c.charset, err)
			continue
		}
		if !strings.Contains(string(decoded), c.want) {
			t.Errorf("ToUTF8(%s) round-trip lost %q: %q", c.charset, c.want, string(decoded))
		}
	}
}

// TestDetectCharsetWhitespaceAroundValue pins the HTML5-whitespace handling of
// the byte-level <meta charset> extractor: a newline after the '=' (legal
// markup) must not leak into the value, and a value that normalizes to
// nothing (whitespace-only) must not surface as an empty detected charset.
func TestDetectCharsetWhitespaceAroundValue(t *testing.T) {
	t.Parallel()

	ed := &EncodingDetector{}
	if got := ed.DetectCharsetBasic([]byte("<html><head><meta charset=\n\"iso-8859-7\"></head><body>")); got != "iso-8859-7" {
		t.Errorf("newline after '=' = %q, want iso-8859-7", got)
	}
	if got := ed.DetectCharsetBasic([]byte(`<html><head><meta charset=" "></head><body>`)); got != "utf-8" {
		t.Errorf("whitespace-only value = %q, want utf-8 fallback", got)
	}
	if got := ed.DetectCharsetBasic([]byte("<html><head><meta charset=\n\" \"></head><body>")); got != "utf-8" {
		t.Errorf("newline + whitespace-only value = %q, want utf-8 fallback", got)
	}
}

func TestDetectCharset(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "UTF-8 with BOM",
			data:     []byte{0xEF, 0xBB, 0xBF, '<', 'h', 't', 'm', 'l', '>'},
			expected: "utf-8",
		},
		{
			name:     "UTF-16 LE BOM",
			data:     []byte{0xFF, 0xFE, 0x3C, 0x00},
			expected: "utf-16le",
		},
		{
			name:     "UTF-16 BE BOM",
			data:     []byte{0xFE, 0xFF, 0x00, 0x3C},
			expected: "utf-16be",
		},
		{
			name:     "windows-1252 in meta tag",
			data:     []byte(`<html><head><meta http-equiv="Content-Type" content="text/html; charset=windows-1252"></head></html>`),
			expected: "windows-1252",
		},
		{
			name:     "windows-1252 in meta tag (uppercase)",
			data:     []byte(`<html><head><meta HTTP-EQUIV="Content-Type" CONTENT="text/html; CHARSET=Windows-1252"></head></html>`),
			expected: "windows-1252",
		},
		{
			name:     "iso-8859-1 in meta tag",
			data:     []byte(`<html><head><meta http-equiv="Content-Type" content="text/html; charset=iso-8859-1"></head></html>`),
			expected: "iso-8859-1",
		},
		{
			name:     "charset attribute",
			data:     []byte(`<html><head><meta charset="utf-8"></head></html>`),
			expected: "utf-8",
		},
		{
			name:     "shift_jis in meta tag",
			data:     []byte(`<html><head><meta http-equiv="Content-Type" content="text/html; charset=shift_jis"></head></html>`),
			expected: "shift_jis",
		},
		{
			name:     "Valid UTF-8 without charset declaration",
			data:     []byte("<html><head><title>Hello 世界</title></head></html>"),
			expected: "utf-8",
		},
		{
			name:     "Invalid UTF-8 without charset declaration - default to windows-1252",
			data:     []byte{0x92, 0x93, 0x94}, // Invalid UTF-8 bytes
			expected: "windows-1252",
		},
		{
			name:     "Empty data defaults to utf-8",
			data:     []byte{},
			expected: "utf-8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := NewEncodingDetector()
			result := ed.DetectCharset(tt.data)
			if result != tt.expected {
				t.Errorf("DetectCharset() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNormalizeCharset(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"UTF-8", "utf-8"},
		{"utf8", "utf-8"},
		{"UTF_8", "utf-8"},
		{"WINDOWS-1252", "windows-1252"},
		{"windows1252", "windows-1252"},
		{"CP1252", "windows-1252"},
		{"1252", "windows-1252"},
		{"ISO-8859-1", "iso-8859-1"},
		{"iso88591", "iso-8859-1"},
		{"latin1", "iso-8859-1"},
		// Regression for ISO-8859 family: canonical names must survive
		// normalization so getEncoding can recognize them. Previously the
		// "iso-" prefix was stripped, turning "iso-8859-2" into "8859-2".
		{"ISO-8859-2", "iso-8859-2"},
		{"iso-8859-5", "iso-8859-5"},
		{"iso-8859-7", "iso-8859-7"},
		{"iso-8859-15", "iso-8859-15"},
		{"iso_8859-1", "iso-8859-1"},
		{"SHIFT_JIS", "shift_jis"},
		{"shift-jis", "shift_jis"},
		{"sjis", "shift_jis"},
		{"EUC-JP", "euc-jp"},
		{"EUC-KR", "euc-kr"},
		{"GB2312", "gbk"},
		{"BIG5", "big5"},
		{"UTF-16LE", "utf-16le"},
		{"utf16le", "utf-16le"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeCharset(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeCharset(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestNormalizeCharsetSlowFallback covers normalizeCharsetSlow's fallback and
// edge branches that the cache-backed normalizeCharset path reaches only
// incidentally: the text/ and text- and codepage- and ibm- prefix strips, the
// latin-N family members not enumerated by the table-driven TestNormalizeCharset
// or TestNormalizeCharsetLatinFamily, alias entries those tests skip, and the
// unknown-charset passthrough.
func TestNormalizeCharsetSlowFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"text/ prefix stripped", "text/utf-8", "utf-8"},
		{"text- prefix stripped", "text-utf-8", "utf-8"},
		{"codepage- prefix stripped", "codepage-1252", "windows-1252"},
		{"ibm- prefix stripped", "ibm-1252", "windows-1252"},
		{"latin-3 family member", "latin-3", "iso-8859-3"},
		{"latin-5 family member", "latin-5", "iso-8859-9"},
		{"latin-8 family member", "latin-8", "iso-8859-14"},
		{"utf_16 underscore alias", "utf_16", "utf-16le"},
		{"euc_jp underscore alias", "euc_jp", "euc-jp"},
		{"euckr alias", "euckr", "euc-kr"},
		{"unknown charset passes through", "x-unknown-charset", "x-unknown-charset"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeCharsetSlow(tt.in); got != tt.want {
				t.Errorf("normalizeCharsetSlow(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestNormalizeCharsetLongInputBypassesCache pins that a charset name longer
// than maxCachedCharsetLen skips the cache and still normalizes via the slow
// path (lowercased and prefix-stripped) without polluting the cache.
func TestNormalizeCharsetLongInputBypassesCache(t *testing.T) {
	t.Parallel()

	long := "Windows-" + strings.Repeat("x", 45)
	want := strings.Repeat("x", 45)
	if got := normalizeCharset(long); got != want {
		t.Errorf("normalizeCharset(long) = %q, want %q", got, want)
	}
}

// TestGetEncoding_ISO8859Family is a regression test for the bug where the
// "iso-" prefix was stripped during normalization, leaving canonical ISO-8859
// names unrecognized. Every member of the supported ISO-8859 family must resolve
// to a non-nil encoding through the public normalization + lookup path.
func TestGetEncoding_ISO8859Family(t *testing.T) {
	family := []string{
		"iso-8859-1", "iso-8859-2", "iso-8859-3", "iso-8859-4", "iso-8859-5",
		"iso-8859-6", "iso-8859-7", "iso-8859-8", "iso-8859-9", "iso-8859-10",
		"iso-8859-13", "iso-8859-14", "iso-8859-15", "iso-8859-16",
	}
	for _, charset := range family {
		normalized := normalizeCharset(charset)
		enc := getEncoding(normalized)
		if enc == nil {
			t.Errorf("getEncoding(normalizeCharset(%q) = %q) = nil, want non-nil encoding",
				charset, normalized)
		}
	}
}

// TestToUTF8_ISO8859_2 verifies end-to-end decoding of ISO-8859-2, which was
// silently broken before the normalization fix (input was returned as raw bytes
// because getEncoding returned nil).
func TestToUTF8_ISO8859_2(t *testing.T) {
	ed := NewEncodingDetector()
	// 0xB3 in ISO-8859-2 is U+0142 ("ł"); in windows-1252 it is U+00B3 ("³").
	// This byte therefore distinguishes a genuine ISO-8859-2 decode from the
	// windows-1252 fallback and from the pre-fix raw-bytes no-op.
	input := []byte{0x5A, 0xB3} // 'Z' + ISO-8859-2 0xB3
	converted, err := ed.ToUTF8(input, "iso-8859-2")
	if err != nil {
		t.Fatalf("ToUTF8(iso-8859-2) error = %v", err)
	}
	want := "Zł" // "Zł"
	if string(converted) != want {
		t.Errorf("ToUTF8(iso-8859-2) = %q, want %q (decoding was a no-op before fix)",
			string(converted), want)
	}
}

func TestToUTF8(t *testing.T) {
	ed := NewEncodingDetector()

	tests := []struct {
		name     string
		input    []byte
		charset  string
		expected string
	}{
		{
			name:    "windows-1252 smart quote",
			input:   []byte("Registrant\x92s telephone number, including area code"),
			charset: "windows-1252",
			// 0x92 in windows-1252 is U+2019 (RIGHT SINGLE QUOTATION MARK), not U+0027.
			expected: "Registrant\u2019s telephone number, including area code",
		},
		{
			name:     "iso-8859-1 accented letter",
			input:    []byte{0x43, 0x61, 0x66, 0xE9}, // "Caf\u00e9" in ISO-8859-1
			charset:  "iso-8859-1",
			expected: "Caf\u00e9", // "Caf\u00e9" in UTF-8
		},
		{
			name:     "utf-8 passthrough",
			input:    []byte("Hello \u4e16\u754c"),
			charset:  "utf-8",
			expected: "Hello \u4e16\u754c", // multibyte UTF-8 input is returned unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converted, err := ed.ToUTF8(tt.input, tt.charset)
			if err != nil {
				t.Fatalf("ToUTF8(%q) error = %v", tt.charset, err)
			}
			if string(converted) != tt.expected {
				t.Errorf("ToUTF8(%q) = %q, want %q", tt.charset, string(converted), tt.expected)
			}
		})
	}
}

func TestToUTF8_CommonWindows1252Chars(t *testing.T) {
	ed := NewEncodingDetector()

	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "Right single quote",
			input:    []byte{0x92},
			expected: "\u2019", // U+2019 RIGHT SINGLE QUOTATION MARK
		},
		{
			name:     "Left double quote",
			input:    []byte{0x93},
			expected: "\u201C", // Left double quotation mark
		},
		{
			name:     "Right double quote",
			input:    []byte{0x94},
			expected: "\u201D", // Right double quotation mark
		},
		{
			name:     "En dash",
			input:    []byte{0x96},
			expected: "–",
		},
		{
			name:     "Em dash",
			input:    []byte{0x97},
			expected: "—",
		},
		{
			name:     "Bullet",
			input:    []byte{0x95},
			expected: "•",
		},
		{
			name:     "Euro sign",
			input:    []byte{0x80},
			expected: "€",
		},
		{
			name:     "Trademark",
			input:    []byte{0x99},
			expected: "™",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converted, err := ed.ToUTF8(tt.input, "windows-1252")
			if err != nil {
				t.Fatalf("ToUTF8() error = %v", err)
			}

			if string(converted) != tt.expected {
				t.Errorf("ToUTF8(windows-1252, byte 0x%02x) = %q (U+%04X), want %q (U+%04X)",
					tt.input[0],
					string(converted),
					[]rune(string(converted))[0],
					tt.expected,
					[]rune(tt.expected)[0])
			}
		})
	}
}

func TestDetectAndConvert(t *testing.T) {
	tests := []struct {
		name            string
		data            []byte
		expectedCharset string
		expectedText    string
	}{
		{
			name:            "UTF-8 HTML",
			data:            []byte(`<html><body>Hello World</body></html>`),
			expectedCharset: "utf-8",
			expectedText:    `<html><body>Hello World</body></html>`,
		},
		{
			name:            "windows-1252 HTML with smart quote",
			data:            []byte("<html><head><meta charset=\"windows-1252\"></head><body>Registrant\x92s test</body></html>"),
			expectedCharset: "windows-1252",
			expectedText:    "<html><head><meta charset=\"windows-1252\"></head><body>Registrant\u2019s test</body></html>",
		},
		{
			name:            "Valid UTF-8 without charset",
			data:            []byte(`<html><body>Hello 世界</body></html>`),
			expectedCharset: "utf-8",
			expectedText:    `<html><body>Hello 世界</body></html>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := NewEncodingDetector()
			converted, charset, err := ed.DetectAndConvert(tt.data)

			if err != nil {
				t.Fatalf("DetectAndConvert() error = %v", err)
			}

			if charset != tt.expectedCharset {
				t.Errorf("DetectAndConvert() charset = %v, want %v", charset, tt.expectedCharset)
			}

			if string(converted) != tt.expectedText {
				t.Errorf("DetectAndConvert() text = %q, want %q", string(converted), tt.expectedText)
			}
		})
	}
}

func TestForcedEncoding(t *testing.T) {
	ed := NewEncodingDetector()
	ed.ForcedEncoding = "windows-1252"

	// Even though this is valid UTF-8, forcing windows-1252 should interpret it as such
	input := []byte("Test\x92\x93\x94")
	converted, charset, err := ed.DetectAndConvert(input)

	if err != nil {
		t.Fatalf("DetectAndConvert() error = %v", err)
	}

	if charset != "windows-1252" {
		t.Errorf("Expected charset windows-1252, got %v", charset)
	}

	expected := "Test\u2019\u201C\u201D" // 0x92->', 0x93->", 0x94->"
	if string(converted) != expected {
		t.Errorf("Forced encoding conversion = %q, want %q", string(converted), expected)
	}
}

// TestForcedEncodingOverridesSmartDetection is a regression test for a bug
// where DetectAndConvert bypassed ForcedEncoding whenever smart detection was
// enabled (the default), silently ignoring Config.Encoding. The existing
// TestForcedEncoding used an input whose bytes smart detection coincidentally
// classified as windows-1252, so it passed even with the bypass. GBK bytes are
// not reliably identified as GBK by statistical detection, so this input
// decodes correctly only when the forced encoding actually wins.
func TestForcedEncodingOverridesSmartDetection(t *testing.T) {
	utf8 := []byte("<html><body>\u4F60\u597D\u4E16\u754C</body></html>")
	gbk, err := simplifiedchinese.GBK.NewEncoder().Bytes(utf8)
	if err != nil {
		t.Fatalf("encode GBK: %v", err)
	}

	ed := NewEncodingDetector() // EnableSmartDetection defaults to true
	ed.ForcedEncoding = "gbk"

	converted, charset, err := ed.DetectAndConvert(gbk)
	if err != nil {
		t.Fatalf("DetectAndConvert() error = %v", err)
	}
	if charset != "gbk" {
		t.Errorf("charset = %q, want %q (forced encoding was bypassed)", charset, "gbk")
	}
	if want := string(utf8); string(converted) != want {
		t.Errorf("converted = %q, want %q", string(converted), want)
	}
}

func BenchmarkDetectCharset(b *testing.B) {
	data := []byte(`<html><head><meta http-equiv="Content-Type" content="text/html; charset=windows-1252"><title>Test</title></head><body>Content</body></html>`)

	ed := NewEncodingDetector()
	b.ResetTimer()
	for b.Loop() {
		_ = ed.DetectCharset(data)
	}
}

func BenchmarkToUTF8_Windows1252(b *testing.B) {
	// 1KB of data
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	ed := NewEncodingDetector()
	b.ResetTimer()
	for b.Loop() {
		_, _ = ed.ToUTF8(data, "windows-1252")
	}
}

func BenchmarkDetectAndConvert(b *testing.B) {
	data := []byte(`<html><head><meta http-equiv="Content-Type" content="text/html; charset=windows-1252"></head><body>Test\x92\x93\x94\x95\x96\x97\x99\x80</body></html>`)

	ed := NewEncodingDetector()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = ed.DetectAndConvert(data)
	}
}

// TestSmartEncodingDetection tests the intelligent encoding detection system
func TestSmartEncodingDetection(t *testing.T) {
	tests := []struct {
		name            string
		data            []byte
		expectedCharset string
		minConfidence   int
		description     string
	}{
		{
			name:            "UTF-8 with Chinese",
			data:            []byte(`<html><head><meta charset="utf-8"></head><body>你好世界</body></html>`),
			expectedCharset: "utf-8",
			minConfidence:   80,
			description:     "UTF-8 with CJK characters",
		},
		{
			name:            "GBK meta tag but UTF-8 data (common bug)",
			data:            createGBKEncodedHTML(),
			expectedCharset: "utf-8", // Smart detection should detect actual UTF-8
			minConfidence:   70,
			description:     "Meta tag declares GBK but data is actually UTF-8",
		},
		{
			name:            "Windows-1252 with smart quotes",
			data:            createWindows1252HTML(),
			expectedCharset: "windows-1252",
			minConfidence:   60,
			description:     "Western text with Windows-1252 special chars",
		},
		{
			name:            "UTF-8 with Cyrillic",
			data:            []byte(`<html><head><meta charset="utf-8"></head><body>Привет мир</body></html>`),
			expectedCharset: "utf-8",
			minConfidence:   80,
			description:     "UTF-8 with Cyrillic characters",
		},
		{
			name:            "ASCII-only defaults to UTF-8",
			data:            []byte(`<html><body>Hello World</body></html>`),
			expectedCharset: "utf-8",
			minConfidence:   50,
			description:     "ASCII-only content defaults to UTF-8",
		},
		{
			name:            "UTF-8 with accented chars",
			data:            []byte(`<html><body>Café résumé naïve</body></html>`),
			expectedCharset: "utf-8",
			minConfidence:   80,
			description:     "UTF-8 with accented Latin characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := NewEncodingDetector()
			ed.EnableSmartDetection = true

			// Test smart detection
			match := ed.DetectCharsetSmart(tt.data)

			t.Logf("Test: %s", tt.description)
			t.Logf("  Detected: %s (confidence: %d, score: %d)", match.Charset, match.Confidence, match.Score)

			if match.Charset != tt.expectedCharset {
				t.Errorf("Expected charset %s, got %s", tt.expectedCharset, match.Charset)
			}

			if match.Confidence < tt.minConfidence {
				t.Errorf("Confidence too low: %d (expected >= %d)", match.Confidence, tt.minConfidence)
			}

			// Verify conversion works
			converted, charset, err := ed.DetectAndConvert(tt.data)
			if err != nil {
				t.Errorf("DetectAndConvert failed: %v", err)
			}

			if charset != tt.expectedCharset {
				t.Errorf("DetectAndConvert charset mismatch: %s != %s", charset, tt.expectedCharset)
			}

			// Verify result is valid UTF-8
			if !utf8.Valid(converted) {
				t.Errorf("Converted data is not valid UTF-8")
			}
		})
	}
}

// TestSmartDetectionPerformance benchmarks the smart detection system
func TestSmartDetectionPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	// Create test data of different sizes
	sizes := []struct {
		name string
		size int
	}{
		{"1KB", 1024},
		{"10KB", 10240},
		{"100KB", 102400},
	}

	encodings := []struct {
		name string
		data []byte
	}{
		{"UTF-8", []byte(`<html><head><meta charset="utf-8"></head><body>Café résumé 你好世界</body></html>`)},
		{"Windows-1252", createWindows1252HTML()},
	}

	for _, sizeInfo := range sizes {
		t.Run(sizeInfo.name, func(t *testing.T) {
			for _, encInfo := range encodings {
				t.Run(encInfo.name, func(t *testing.T) {
					// Create data of specified size
					data := make([]byte, 0, sizeInfo.size)
					for len(data) < sizeInfo.size {
						data = append(data, encInfo.data...)
					}
					data = data[:sizeInfo.size]

					ed := NewEncodingDetector()
					ed.EnableSmartDetection = true

					// Measure detection time
					start := time.Now()
					match := ed.DetectCharsetSmart(data)
					duration := time.Since(start)

					t.Logf("Size: %s, Encoding: %s, Detected: %s (confidence: %d), Time: %v",
						sizeInfo.name, encInfo.name, match.Charset, match.Confidence, duration)

					// Detection should complete in reasonable time (< 1 second for 100KB)
					if duration > time.Second {
						t.Errorf("Detection took too long: %v", duration)
					}
				})
			}
		})
	}
}

// Helper functions to create test data

func createWindows1252HTML() []byte {
	base := []byte(`<html><head><meta charset="windows-1252"></head><body>`)
	// Add text with Windows-1252 special characters (byte 0x92 = ')
	text := []byte("Registrant\x92s telephone number")
	suffix := []byte(`</body></html>`)
	result := make([]byte, len(base)+len(text)+len(suffix))
	copy(result, base)
	copy(result[len(base):], text)
	copy(result[len(base)+len(text):], suffix)
	return result
}

func createGBKEncodedHTML() []byte {
	// Return a HTML with Chinese characters
	// In actual GBK encoding this would be different bytes,
	// but for testing we use UTF-8 which should be detected
	return []byte(`<html><head><meta charset="gbk"></head><body>你好世界</body></html>`)
}

// Test for the specific bug: UTF-8 file declaring Windows-1252 should be detected as UTF-8
func TestUTF8WithWrongCharsetMeta(t *testing.T) {
	tests := []struct {
		name            string
		data            []byte
		expectedCharset string
		shouldContain   string
	}{
		{
			name: "UTF-8 with accented chars and Windows-1252 meta",
			data: []byte(`<html><head><meta http-equiv="Content-Type" content="text/html; charset=windows-1252"></head><body>Café résumé</body></html>`),
			// café has UTF-8 bytes: 63 61 66 C3 A9 20
			// This should be detected as UTF-8 because it has valid UTF-8 multi-byte sequences
			expectedCharset: "utf-8",
			shouldContain:   "Café",
		},
		{
			name:            "UTF-8 with Chinese chars and Windows-1252 meta",
			data:            []byte(`<html><head><meta charset="windows-1252"></head><body>Hello 世界</body></html>`),
			expectedCharset: "utf-8",
			shouldContain:   "世界",
		},
		{
			name: "Proper Windows-1252 with 0x92 apostrophe",
			// Create the byte array manually to ensure 0x92 is a raw byte, not escape sequence
			data: func() []byte {
				base := []byte(`<html><head><meta charset="windows-1252"></head><body>Registrant`)
				suffix := []byte(`s</body></html>`)
				result := make([]byte, len(base)+1+len(suffix))
				copy(result, base)
				result[len(base)] = 0x92
				copy(result[len(base)+1:], suffix)
				return result
			}(),
			// This is actual Windows-1252 (byte 0x92 is not valid UTF-8)
			expectedCharset: "windows-1252",
			shouldContain:   "\u2019", // 0x92 in Windows-1252 is U+2019 RIGHT SINGLE QUOTATION MARK
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := NewEncodingDetector()
			charset := ed.DetectCharset(tt.data)

			if charset != tt.expectedCharset {
				t.Errorf("DetectCharset() = %v, want %v", charset, tt.expectedCharset)
			}

			// Also verify conversion works
			converted, _, err := ed.DetectAndConvert(tt.data)
			if err != nil {
				t.Fatalf("DetectAndConvert() error = %v", err)
			}

			result := string(converted)
			if !bytes.Contains(converted, []byte(tt.shouldContain)) {
				t.Errorf("Expected output to contain %q, got: %s", tt.shouldContain, result)
			}
		})
	}
}

// ============================================================================
// SetMaxSampleSize Tests
// ============================================================================

func TestSetMaxSampleSize(t *testing.T) {
	t.Parallel()

	t.Run("valid sample size", func(t *testing.T) {
		ed := NewEncodingDetector()
		ed.SetMaxSampleSize(2048)

		if ed.MaxSampleSize != 2048 {
			t.Errorf("MaxSampleSize = %d, want 2048", ed.MaxSampleSize)
		}
	})

	t.Run("negative size uses default", func(t *testing.T) {
		ed := NewEncodingDetector()
		ed.SetMaxSampleSize(-1)

		if ed.MaxSampleSize != 10240 {
			t.Errorf("MaxSampleSize = %d, want 10240 (default)", ed.MaxSampleSize)
		}
	})

	t.Run("zero size uses default", func(t *testing.T) {
		ed := NewEncodingDetector()
		ed.SetMaxSampleSize(0)

		if ed.MaxSampleSize != 10240 {
			t.Errorf("MaxSampleSize = %d, want 10240 (default)", ed.MaxSampleSize)
		}
	})

	t.Run("size above max is capped", func(t *testing.T) {
		ed := NewEncodingDetector()
		ed.SetMaxSampleSize(10 * 1024 * 1024) // 10MB, above 1MB max

		if ed.MaxSampleSize != 1024*1024 {
			t.Errorf("MaxSampleSize = %d, want %d (max)", ed.MaxSampleSize, 1024*1024)
		}
	})

	t.Run("chaining works", func(t *testing.T) {
		ed := NewEncodingDetector()
		ed.SetMaxSampleSize(512)

		if ed.MaxSampleSize != 512 {
			t.Errorf("MaxSampleSize = %d, want 512", ed.MaxSampleSize)
		}
	})
}

// TestScoreLanguagePatterns exercises every scoring arm of
// scoreLanguagePatterns: no special characters, CJK with a matching CJK charset
// (positive bonus), CJK with a Western charset (-10 penalty), Cyrillic with a
// matching Cyrillic charset (+10), and Cyrillic with a Western charset (neutral).
// The CJK-bonus value depends on int(cjkRatio*15) and is float-truncation
// sensitive, so that arm asserts only the sign (> 0); the other arms are exact
// integer constants.
func TestScoreLanguagePatterns(t *testing.T) {
	t.Parallel()

	ed := &EncodingDetector{}

	tests := []struct {
		name    string
		decoded string
		charset string
		check   func(int) bool
		desc    string
	}{
		{"empty input", "", "utf-8", func(s int) bool { return s == 0 }, "== 0"},
		{"ascii no special chars", "hello world", "gbk", func(s int) bool { return s == 0 }, "== 0"},
		{"cjk with matching cjk charset", "你好世界", "gbk", func(s int) bool { return s > 0 }, "> 0"},
		{"cjk with western charset penalty", "你好世界", "utf-8", func(s int) bool { return s == -10 }, "== -10"},
		{"cyrillic with matching cyrillic charset", "Привет мир", "windows-1251", func(s int) bool { return s == 10 }, "== 10"},
		{"cyrillic with western charset neutral", "Привет мир", "utf-8", func(s int) bool { return s == 0 }, "== 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ed.scoreLanguagePatterns([]byte(tt.decoded), tt.charset)
			if !tt.check(got) {
				t.Errorf("scoreLanguagePatterns(%q, %q) = %d, want %s", tt.decoded, tt.charset, got, tt.desc)
			}
		})
	}
}

// TestIsPrintable exercises every branch of isPrintable: printable ASCII, the
// three whitelisted whitespace bytes, the non-ASCII passthrough, and the
// remaining control bytes that must report false.
func TestIsPrintable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		b    byte
		want bool
	}{
		{"space", ' ', true},
		{"digit", '0', true},
		{"upper", 'Z', true},
		{"lower tilde", '~', true}, // 126
		{"tab", '\t', true},
		{"lf", '\n', true},
		{"cr", '\r', true},
		{"high byte 0x80", 0x80, true},
		{"high byte 0xFF", 0xFF, true},
		{"nul", 0x00, false},
		{"soh", 0x01, false},
		{"vt", 0x0B, false},
		{"unit separator", 0x1F, false},
		{"del", 0x7F, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isPrintable(tt.b); got != tt.want {
				t.Errorf("isPrintable(0x%02X) = %v, want %v", tt.b, got, tt.want)
			}
		})
	}
}

// TestCalculateValidUTF8RatioFull covers the ASCII fast path, every multi-byte
// length (2/3/4), and the three malformed-sequence branches: invalid leading
// byte, truncated sequence, and bad continuation byte.
func TestCalculateValidUTF8RatioFull(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
		// ratio is valid/len; we assert the exact expected value per case.
		want float64
	}{
		{"empty", nil, 0},
		{"pure ascii", []byte("hello"), 1.0},                               // 5/5
		{"valid 2-byte", []byte{0xC3, 0xA9}, 0.5},                          // é: 1/2
		{"valid 3-byte", []byte{0xE4, 0xB8, 0xAD}, 1.0 / 3},                // 中: 1/3
		{"valid 4-byte", []byte{0xF0, 0x9F, 0x98, 0x80}, 0.25},             // 😀: 1/4
		{"ascii plus valid multibyte", []byte{'a', 0xC3, 0xA9}, 2.0 / 3.0}, // 2 valid / 3 bytes
		{"invalid leading byte 0x80", []byte{0x80}, 0},                     // continuation byte as lead
		{"invalid leading byte 0xFE", []byte{0xFE}, 0},                     // out of range
		{"truncated 2-byte", []byte{0xC3}, 0},                              // lead present, no continuation
		{"bad continuation", []byte{0xC3, 0x41}, 0},                        // 0x41 is not a continuation byte
		{"truncated 3-byte", []byte{0xE4, 0xB8}, 0},                        // needs 3, has 2
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := calculateValidUTF8RatioFull(tt.data)
			if got != tt.want {
				t.Errorf("calculateValidUTF8RatioFull(% x) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

// TestHasExcessiveControlChars covers the empty input, the >5% threshold
// (excluding tab/lf/cr), and the just-below-threshold boundary.
func TestHasExcessiveControlChars(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"empty", nil, false},
		{"clean ascii", []byte("hello world"), false},
		{"whitespace not counted", []byte("a\tb\nc\rd"), false},
		{"one control in twenty is exactly 5 percent", []byte("\x01aaaaaaaaaaaaaaaaaaa"), false}, // 1/20 = 5%, not > 5%
		{"one control in nineteen exceeds 5 percent", []byte("\x01aaaaaaaaaaaaaaaaaa"), true},    // 1/19 ≈ 5.26%
		{"mostly control chars", []byte("\x00\x01\x02\x03\x04abc"), true},                        // 5/8
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := hasExcessiveControlChars(tt.data); got != tt.want {
				t.Errorf("hasExcessiveControlChars(% x) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

// TestScoreEncodingMatchOptimized covers the UTF-8 valid/invalid special cases,
// the unknown-charset (nil encoding) early return, and a successful non-UTF-8
// decode through scoreDecodedData.
func TestScoreEncodingMatchOptimized(t *testing.T) {
	t.Parallel()
	ed := NewEncodingDetector()

	tests := []struct {
		name        string
		data        []byte
		charset     string
		isUTF8Valid bool
		wantZero    bool // true asserts score == 0
	}{
		{"utf-8 valid scores positive", []byte("héllo 世界"), "utf-8", true, false},
		{"utf-8 flagged invalid scores zero", []byte{0xFF, 0xFE, 0xFD}, "utf-8", false, true},
		{"unknown charset scores zero", []byte("hello"), "no-such-charset-xyz", true, true},
		{"windows-1252 decodes successfully", []byte("caf\xe9"), "windows-1252", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ed.scoreEncodingMatchOptimized(tt.data, tt.charset, tt.isUTF8Valid)
			if tt.wantZero && got != 0 {
				t.Errorf("scoreEncodingMatchOptimized(%q, %q) = %d, want 0", tt.data, tt.charset, got)
			}
			if !tt.wantZero && got <= 0 {
				t.Errorf("scoreEncodingMatchOptimized(%q, %q) = %d, want > 0", tt.data, tt.charset, got)
			}
		})
	}
}

// ===========================================================================
// String convenience wrappers (merged from encoding_convenience_test.go).
// TestToUTF8 and TestDetectAndConvert above already pin the byte-slice
// conversion behavior exactly, so the former weak non-empty-only wrapper
// tests (TestConvertToUTF8, TestDetectAndConvertToUTF8) were dropped.
// ===========================================================================

// TestDetectAndConvertToUTF8String drives both the sharing (String) and the
// copying (StringSafe) wrapper through one table, pinning the exact output
// string and charset for each input shape.
func TestDetectAndConvertToUTF8String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		data           []byte
		forcedEncoding string
		wantResult     string
		wantCharset    string
	}{
		{
			name:           "auto-detect UTF-8",
			data:           []byte("<html><head><meta charset=\"utf-8\"></head><body>Hello</body></html>"),
			forcedEncoding: "",
			wantResult:     "<html><head><meta charset=\"utf-8\"></head><body>Hello</body></html>",
			wantCharset:    "utf-8",
		},
		{
			name:           "forced UTF-8",
			data:           []byte("Hello World"),
			forcedEncoding: "utf-8",
			wantResult:     "Hello World",
			wantCharset:    "utf-8",
		},
		{
			name:           "forced Windows-1252",
			data:           []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F}, // "Hello" (pure ASCII subset)
			forcedEncoding: "windows-1252",
			wantResult:     "Hello",
			wantCharset:    "windows-1252",
		},
		{
			// An unrecognized forced encoding is echoed back as the charset;
			// the data is still converted best-effort (ASCII passes through).
			name:           "invalid forced encoding",
			data:           []byte("Hello"),
			forcedEncoding: "invalid-xyz",
			wantResult:     "Hello",
			wantCharset:    "invalid-xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, charset, err := DetectAndConvertToUTF8String(tt.data, tt.forcedEncoding)
			if err != nil {
				t.Fatalf("DetectAndConvertToUTF8String() error = %v", err)
			}
			if result != tt.wantResult {
				t.Errorf("result = %q, want %q", result, tt.wantResult)
			}
			if charset != tt.wantCharset {
				t.Errorf("charset = %q, want %q", charset, tt.wantCharset)
			}

			safeResult, safeCharset, safeErr := DetectAndConvertToUTF8StringSafe(tt.data, tt.forcedEncoding)
			if safeErr != nil {
				t.Fatalf("DetectAndConvertToUTF8StringSafe() error = %v", safeErr)
			}
			if safeResult != tt.wantResult {
				t.Errorf("safe result = %q, want %q", safeResult, tt.wantResult)
			}
			if safeCharset != tt.wantCharset {
				t.Errorf("safe charset = %q, want %q", safeCharset, tt.wantCharset)
			}
		})
	}
}

// TestDetectAndConvertToUTF8StringMemoryIsolation verifies that the Safe version
// provides memory isolation while the regular version shares memory for ASCII input.
func TestDetectAndConvertToUTF8StringMemoryIsolation(t *testing.T) {
	t.Parallel()

	original := []byte("Hello World")

	// Test the regular function - note that modifying input after the call
	// is UNSAFE and only done here for testing purposes
	result, charset, err := DetectAndConvertToUTF8String(original, "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if charset != "utf-8" {
		t.Errorf("Expected utf-8 charset, got %s", charset)
	}
	if result != "Hello World" {
		t.Errorf("Expected 'Hello World', got '%s'", result)
	}

	// Test the safe version - it should always return a copy
	safeResult, safeCharset, safeErr := DetectAndConvertToUTF8StringSafe(original, "")
	if safeErr != nil {
		t.Fatalf("Unexpected error in safe version: %v", safeErr)
	}
	if safeCharset != "utf-8" {
		t.Errorf("Expected utf-8 charset in safe version, got %s", safeCharset)
	}
	if safeResult != "Hello World" {
		t.Errorf("Expected 'Hello World' in safe version, got '%s'", safeResult)
	}

	// Modify original - safeResult should be unaffected
	original[0] = 'X'
	if safeResult != "Hello World" {
		t.Errorf("Safe version should be isolated from input modifications, got '%s'", safeResult)
	}
}

// TestNormalizeCharsetConcurrentNoOvercount pins the counter discipline of the
// charset memoization cache: every goroutine simultaneously missing a key must
// race to LoadOrStore it, and only the goroutine whose call performs the
// insert counts. The previous Store+Add(1) sequence let every concurrent
// misser of the same key Add(1), inflating the counter and permanently capping
// the cache below maxCachedCharsetEntries.
//
// The counter is global and monotonic (entries are never evicted), so the
// assertion is on the delta across the hammering: with correct counting the
// delta is bounded by the number of DISTINCT keys regardless of interleaving —
// a deterministic invariant — while any single duplicated count anywhere in
// the concurrent miss storm exceeds it. (The absolute ≤ maxCachedCharsetEntries
// bound is separately pinned by TestCharsetNormCacheCardinalityBounded.)
func TestNormalizeCharsetConcurrentNoOvercount(t *testing.T) {
	// Deliberately NOT t.Parallel(): the assertion measures the delta of the
	// GLOBAL charsetNormCacheEntries counter across the hammering below, and
	// any parallel sibling test that inserts its own novel charset names into
	// the same process-wide cache during that window inflates the delta and
	// fails the test spuriously (observed as "grew by 176 for 64 distinct
	// keys" under -race). Running sequentially keeps the measured window free
	// of unrelated inserters; the concurrency under test is the 16-goroutine
	// hammer itself, which does not need t.Parallel to exercise the race.

	const distinctKeys = 64
	const goroutines = 16
	before := charsetNormCacheEntries.Load()

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for k := range distinctKeys {
				_ = normalizeCharset(fmt.Sprintf("x-concurrent-key-%d", k))
			}
		}()
	}
	close(start) // release every goroutine at once to maximize miss overlap
	wg.Wait()

	after := charsetNormCacheEntries.Load()
	if grew := after - before; grew > distinctKeys {
		t.Errorf("charsetNormCacheEntries grew by %d for %d distinct keys; concurrent duplicate misses must not overcount (before=%d, after=%d)", grew, distinctKeys, before, after)
	}
}
func TestBOMCharsetAuthoritative(t *testing.T) {
	t.Parallel()

	u16 := utf16.Encode([]rune("<html><body><p>hello \x0c world</p></body></html>"))
	le := make([]byte, 2+2*len(u16))
	le[0], le[1] = 0xFF, 0xFE
	for i, r := range u16 {
		binary.LittleEndian.PutUint16(le[2+2*i:], r)
	}

	ed := NewEncodingDetector()
	if match := ed.DetectCharsetSmart(le); match.Charset != "utf-16le" {
		t.Errorf("DetectCharsetSmart(BOM+ctrl) = %q (confidence %d), want utf-16le", match.Charset, match.Confidence)
	}
	if got := ed.DetectCharset(le); got != "utf-16le" {
		t.Errorf("DetectCharset(BOM+ctrl) = %q, want utf-16le", got)
	}
	out, _, err := ed.DetectAndConvert(le)
	if err != nil {
		t.Fatalf("DetectAndConvert: %v", err)
	}
	if !strings.Contains(string(out), "hello") {
		t.Errorf("decoded output lost text: %q", string(out))
	}
}

// TestBOMCharsetAuthoritativeBE covers the big-endian variant.
func TestBOMCharsetAuthoritativeBE(t *testing.T) {
	t.Parallel()

	u16 := utf16.Encode([]rune("<html><body><p>hi there</p></body></html>"))
	be := make([]byte, 2+2*len(u16))
	be[0], be[1] = 0xFE, 0xFF
	for i, r := range u16 {
		binary.BigEndian.PutUint16(be[2+2*i:], r)
	}

	ed := NewEncodingDetector()
	if match := ed.DetectCharsetSmart(be); match.Charset != "utf-16be" {
		t.Errorf("DetectCharsetSmart(UTF-16BE BOM) = %q (confidence %d), want utf-16be", match.Charset, match.Confidence)
	}
}

// TestBOMCharsetHelper pins the shared helper's mapping for every BOM it
// recognizes, plus the no-BOM case.
func TestBOMCharsetHelper(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"utf-8 BOM", []byte{0xEF, 0xBB, 0xBF, 'x'}, "utf-8"},
		{"utf-16be BOM", []byte{0xFE, 0xFF, 0x00, 'x'}, "utf-16be"},
		{"utf-16le BOM", []byte{0xFF, 0xFE, 'x', 0x00}, "utf-16le"},
		{"no BOM", []byte("<html>"), ""},
		{"empty", nil, ""},
		{"too short", []byte{0xFF}, ""},
	}
	for _, c := range cases {
		if got := detectBOMCharset(c.data); got != c.want {
			t.Errorf("detectBOMCharset(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestUTF8BOMStrippedOnFastPath pins the fix for a UTF-8 BOM surviving the
// valid-UTF-8 fast path: BOM-prefixed input passes utf8.Valid (the BOM is a
// legal U+FEFF encoding), so detectAndConvertToUTF8StringCore used to convert
// it verbatim and U+FEFF flowed into the parsed document and the extracted
// text — while the slow path (ToUTF8) stripped it, so identical bytes came
// back different depending on which path handled them.
func TestUTF8BOMStrippedOnFastPath(t *testing.T) {
	t.Parallel()

	bom := []byte{0xEF, 0xBB, 0xBF}

	// BOM + ASCII body: not pure ASCII (the BOM bytes are >= 0x80) but valid
	// UTF-8, so it takes the fast path under review.
	ascii := append(append([]byte{}, bom...),
		[]byte("<html><body><p>hello world</p></body></html>")...)
	if got, _, err := DetectAndConvertToUTF8String(ascii, ""); err != nil {
		t.Fatalf("DetectAndConvertToUTF8String: %v", err)
	} else if strings.ContainsRune(got, rune(0xFEFF)) {
		t.Errorf("fast path leaked UTF-8 BOM into output: %q", got)
	}

	// BOM + CJK body: same fast path, exercising the zero-copy sharing
	// branch on non-ASCII content.
	cjk := append(append([]byte{}, bom...),
		[]byte("<html><body><p>你好世界</p></body></html>")...)
	if got, _, err := DetectAndConvertToUTF8String(cjk, ""); err != nil {
		t.Fatalf("DetectAndConvertToUTF8String(CJK): %v", err)
	} else if strings.ContainsRune(got, rune(0xFEFF)) {
		t.Errorf("fast path leaked UTF-8 BOM into CJK output: %q", got)
	}

	// The memory-safe (copying) variant shares the core and must strip too.
	if got, _, err := DetectAndConvertToUTF8StringSafe(ascii, ""); err != nil {
		t.Fatalf("DetectAndConvertToUTF8StringSafe: %v", err)
	} else if strings.ContainsRune(got, rune(0xFEFF)) {
		t.Errorf("safe variant leaked UTF-8 BOM: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Latin-family aliases, forced-UTF-8 validation, and the charset cache bound
// (from p2_regression_test.go)
// ---------------------------------------------------------------------------

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
		normalizeCharset("zz" + strings.Repeat("x", i%20+1) + string(rune('a'+i%26)) + strconv.Itoa(i))
	}
	after := count()
	if after > before+maxCachedCharsetEntries {
		t.Errorf("cache grew by %d entries, want <= %d", after-before, maxCachedCharsetEntries)
	}
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

// ---------------------------------------------------------------------------
// Charset wrapper and sample-size defaults (from boundary_gaps_test.go)
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

// extractCharsetFromHTML extracts charset from an HTML string. Test-only
// since DetectCharsetBasic switched to the byte-based extractor directly (its
// sole remaining caller is the table below); production never materializes a
// string for the pre-decode scan.
func extractCharsetFromHTML(html string) string {
	if len(html) == 0 {
		return ""
	}
	return extractCharsetFromBytes(StringToBytes(html))
}
