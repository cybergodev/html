package internal

import (
	"bytes"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
)

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
	for i := 0; i < b.N; i++ {
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
	for i := 0; i < b.N; i++ {
		_, _ = ed.ToUTF8(data, "windows-1252")
	}
}

func BenchmarkDetectAndConvert(b *testing.B) {
	data := []byte(`<html><head><meta http-equiv="Content-Type" content="text/html; charset=windows-1252"></head><body>Test\x92\x93\x94\x95\x96\x97\x99\x80</body></html>`)

	ed := NewEncodingDetector()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
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
