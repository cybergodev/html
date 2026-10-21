package internal

// encoding_bom_regression_test.go — regression test that a byte-order mark is
// treated as an authoritative encoding declaration. Previously the BOM-derived
// charset still had to survive the statistical gate in DetectCharsetSmart
// (score >= 90), and a single control byte in the UTF-16 sample dropped the
// score to 89, letting an 8-bit candidate (whose decode of NUL-interleaved
// bytes scored as printable text) win — the document came back as mojibake
// with a nil error.

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

// TestBOMCharsetAuthoritative feeds a UTF-16LE document with BOM whose body
// contains one form-feed control byte; detection must still report UTF-16LE.
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
