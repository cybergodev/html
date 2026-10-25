// encoding.go provides character encoding detection and conversion functionality.
// It supports 15+ encodings including Unicode variants, Western European,
// and East Asian character sets, with intelligent auto-detection capabilities.
package internal

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var (
	// Pre-compiled regex patterns for charset detection.
	// Note: These are kept for fallback when extractCharsetFromHTML doesn't find a charset.
	//
	// SECURITY: These regex patterns are only applied to the first 1024 bytes of HTML
	// (see DetectCharsetBasic where htmlStart is created from sample), which limits the
	// potential impact of ReDoS attacks. The patterns use negated character classes [^>]*
	// rather than greedy .* to minimize backtracking.
	charsetPattern    = regexp.MustCompile(`(?i)<meta\s+[^>]*http-equiv=["']?content-type["']?[^>]*content=["']?[^;]*;\s*charset=([^"'\s>]+)`)
	charsetPatternAlt = regexp.MustCompile(`(?i)<meta\s+charset=["']?([^"'\s>]+)`)

	// charsetNormCache caches normalized charset names to avoid repeated string operations.
	// This provides 5-10% faster charset normalization for repeated lookups.
	charsetNormCache sync.Map
)

// charsetAliases maps charset aliases to their canonical names.
// This map is used for fast O(1) lookup during charset normalization.
var charsetAliases = map[string]string{
	// Windows code pages
	"1252":        "windows-1252",
	"cp1252":      "windows-1252",
	"windows1252": "windows-1252",
	"1251":        "windows-1251",
	"cp1251":      "windows-1251",
	"windows1251": "windows-1251",
	"1250":        "windows-1250",
	"cp1250":      "windows-1250",
	"windows1250": "windows-1250",
	"1253":        "windows-1253",
	"cp1253":      "windows-1253",
	"windows1253": "windows-1253",
	"1254":        "windows-1254",
	"cp1254":      "windows-1254",
	"windows1254": "windows-1254",
	"1255":        "windows-1255",
	"cp1255":      "windows-1255",
	"windows1255": "windows-1255",
	"1256":        "windows-1256",
	"cp1256":      "windows-1256",
	"windows1256": "windows-1256",
	"1258":        "windows-1258",
	"cp1258":      "windows-1258",
	"windows1258": "windows-1258",
	// ISO-8859 variants
	"8859-1":      "iso-8859-1",
	"88591":       "iso-8859-1",
	"iso88591":    "iso-8859-1",
	"iso_8859-1":  "iso-8859-1",
	"iso_8859_1":  "iso-8859-1",
	"latin1":      "iso-8859-1",
	"latin-1":     "iso-8859-1",
	"8859-15":     "iso-8859-15",
	"885915":      "iso-8859-15",
	"iso885915":   "iso-8859-15",
	"iso_8859-15": "iso-8859-15",
	"iso_8859_15": "iso-8859-15",
	// UTF variants
	"utf8":     "utf-8",
	"utf-8":    "utf-8",
	"utf_8":    "utf-8",
	"utf16":    "utf-16le",
	"utf-16":   "utf-16le",
	"utf_16":   "utf-16le",
	"utf16le":  "utf-16le",
	"utf-16le": "utf-16le",
	"utf16be":  "utf-16be",
	"utf-16be": "utf-16be",
	// Japanese
	"shift_jis": "shift_jis",
	"shift-jis": "shift_jis",
	"shiftjis":  "shift_jis",
	"sjis":      "shift_jis",
	"x-sjis":    "shift_jis",
	"euc-jp":    "euc-jp",
	"euc_jp":    "euc-jp",
	"eucjp":     "euc-jp",
	// Korean
	"euc-kr": "euc-kr",
	"euc_kr": "euc-kr",
	"euckr":  "euc-kr",
	// Cyrillic
	"koi8-r": "koi8-r",
	"koi8r":  "koi8-r",
	"koi8":   "koi8-r",
	// Chinese
	"gb2312":     "gbk",
	"gb2312-80":  "gbk",
	"gb2312_80":  "gbk",
	"gbk":        "gbk",
	"gb18030":    "gb18030",
	"gb-18030":   "gb18030",
	"big5":       "big5",
	"big-5":      "big5",
	"big5-hkscs": "big5",
}

// extractCharsetFromHTML extracts charset from HTML using fast string operations.
// This is a convenience wrapper that delegates to extractCharsetFromBytes.
// Returns empty string if not found.
func extractCharsetFromHTML(html string) string {
	if len(html) == 0 {
		return ""
	}
	// Delegate to the byte-based implementation to avoid code duplication.
	// Use StringToBytes for zero-allocation conversion.
	return extractCharsetFromBytes(StringToBytes(html))
}

// extractCharsetFromBytes extracts charset from HTML bytes using fast byte operations.
// extractCharsetFromBytes scans raw bytes (the first 1KB) for a
// <meta charset=...> declaration. It is byte-based to avoid materializing a
// string for the ASCII fast path in DetectCharsetBasic; callers routinely
// pass non-ASCII data (the scan simply looks for ASCII patterns in it), and
// the extracted value is allocated as a string on a hit.
func extractCharsetFromBytes(data []byte) string {
	dataLen := len(data)
	if dataLen == 0 {
		return ""
	}

	// Limit search area to first 1024 bytes
	maxSearch := min(dataLen, 1024)

	// Look for <meta charset="..."> using byte comparison. HTML5 allows ASCII
	// whitespace around the '=', so skip any between the attribute name and
	// the '=' before requiring the separator.
	if idx := asciiIndexIgnoreCaseBytes(data[:maxSearch], []byte("<meta charset")); idx >= 0 {
		pos := idx + 13
		for pos < maxSearch && (data[pos] == ' ' || data[pos] == '\t' || data[pos] == '\n' || data[pos] == '\r') {
			pos++
		}
		// Bounds check: ensure we have enough bytes after the separator
		if pos < maxSearch && data[pos] == '=' && pos+1 < maxSearch {
			rest := data[pos+1 : maxSearch]
			if charset := extractAttributeValueBytes(rest); charset != "" {
				return charset
			}
		}
	}

	// Look for charset= anywhere in the first 1KB — unlike the <meta-prefixed
	// scan above and the regex backstops below, this byte scan carries no tag
	// context, so a literal "charset=" in early title/body text matches too.
	remaining := data[:maxSearch]
	for len(remaining) > 8 {
		idx := asciiIndexIgnoreCaseBytes(remaining, []byte("charset="))
		if idx < 0 {
			break
		}
		rest := remaining[idx+8:]
		if charset := extractAttributeValueBytes(rest); charset != "" {
			return charset
		}
		remaining = remaining[idx+8:]
	}

	return ""
}

// asciiIndexIgnoreCaseBytes performs case-insensitive search on byte slices.
func asciiIndexIgnoreCaseBytes(data, pattern []byte) int {
	patternLen := len(pattern)
	if patternLen == 0 {
		return 0
	}
	dataLen := len(data)
	if dataLen < patternLen {
		return -1
	}

	// Simple search for byte slices
	for i := 0; i <= dataLen-patternLen; i++ {
		match := true
		for j := range patternLen {
			dc := data[i+j]
			pc := pattern[j]
			// Fast ASCII lowercase conversion
			if dc >= 'A' && dc <= 'Z' {
				dc += 32
			}
			if pc >= 'A' && pc <= 'Z' {
				pc += 32
			}
			if dc != pc {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// isHTML5ASCIIWhitespace reports whether c is one of the five ASCII
// whitespace bytes HTML5 allows between markup tokens: space, tab, line
// feed, form feed, carriage return.
func isHTML5ASCIIWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r'
}

// extractAttributeValueBytes extracts an HTML attribute value from byte slice.
// Returns empty string if not found.
func extractAttributeValueBytes(data []byte) string {
	dataLen := len(data)
	if dataLen == 0 {
		return ""
	}

	// Skip leading whitespace — all five HTML5 ASCII whitespace bytes: the
	// name-side scan above accepts them between the attribute name and '=',
	// and a newline after the '=' (legal markup) used to leak into the value.
	start := 0
	for start < dataLen && isHTML5ASCIIWhitespace(data[start]) {
		start++
	}
	if start >= dataLen {
		return ""
	}
	data = data[start:]
	dataLen = len(data)

	// Handle quoted value
	if data[0] == '"' || data[0] == '\'' {
		quote := data[0]
		for i := 1; i < dataLen; i++ {
			if data[i] == quote {
				return string(data[1:i])
			}
		}
		// No closing quote
		return ""
	}

	// Unquoted value - find end
	end := 0
	for end < dataLen {
		c := data[end]
		if isHTML5ASCIIWhitespace(c) || c == '>' || c == ';' || c == '"' || c == '\'' {
			break
		}
		end++
	}

	if end == 0 {
		return ""
	}
	return string(data[:end])
}

// EncodingDetector handles charset detection and conversion.
//
// IMPORTANT: The data slice passed to detection methods must not be modified
// during the detection process. For concurrent access, pass a copy of the data.
type EncodingDetector struct {
	// User-specified encoding override (optional)
	ForcedEncoding string

	// Smart detection options
	EnableSmartDetection bool // Enable intelligent encoding detection
	MaxSampleSize        int  // Max bytes to analyze for statistical detection (default: 10KB, max: 1MB)
}

// NewEncodingDetector creates a new encoding detector with smart detection enabled.
// The default MaxSampleSize is 10KB which is sufficient for most HTML documents.
func NewEncodingDetector() *EncodingDetector {
	return &EncodingDetector{
		EnableSmartDetection: true,
		MaxSampleSize:        10240, // Analyze first 10KB
	}
}

// SetMaxSampleSize sets the maximum sample size for statistical detection.
// Values <= 0 use the default (10KB). Values > 1MB are capped at 1MB to prevent
// memory exhaustion.
func (ed *EncodingDetector) SetMaxSampleSize(size int) {
	const (
		defaultSize = 10240
		maxSize     = 1024 * 1024 // 1MB
	)
	switch {
	case size <= 0:
		ed.MaxSampleSize = defaultSize
	case size > maxSize:
		ed.MaxSampleSize = maxSize
	default:
		ed.MaxSampleSize = size
	}
}

// EncodingMatch represents a detected encoding with confidence score
type EncodingMatch struct {
	Charset    string
	Confidence int // 0-100
	Score      int // Detailed score
	// Valid is a heuristic quality flag derived from a score threshold by the
	// statistical pass — it does NOT literally mean "decoding produced valid
	// UTF-8", and no production code branches on it.
	Valid bool
}

// DetectCharset attempts to detect the character encoding from HTML content
func (ed *EncodingDetector) DetectCharset(data []byte) string {
	// If user forced a specific encoding, use it
	if ed.ForcedEncoding != "" {
		return normalizeCharset(ed.ForcedEncoding)
	}

	// Run the basic pass ONCE and share it with the smart pass: the fallback
	// below needs the basic answer when smart confidence lands under 80, and
	// DetectCharsetSmart used to recompute it — full-document UTF-8 validation
	// plus the BOM/meta scan — a second time on exactly that path.
	basicCharset := ed.DetectCharsetBasic(data)

	// Use smart detection if enabled
	if ed.EnableSmartDetection {
		if match := ed.detectCharsetSmart(data, basicCharset); match.Confidence >= 80 {
			return match.Charset
		}
	}

	return basicCharset
}

// detectBOMCharset returns the charset declared by a byte-order mark at the
// start of data ("utf-8", "utf-16be", or "utf-16le"), or "" when no BOM is
// present. A BOM is a deterministic encoding declaration embedded in the data
// itself, so callers treat it as authoritative rather than as one heuristic
// vote among many.
func detectBOMCharset(data []byte) string {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return "utf-8"
	}
	if len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF {
		return "utf-16be"
	}
	if len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE {
		return "utf-16le"
	}
	return ""
}

// DetectCharsetBasic performs basic charset detection (BOM, meta tags, UTF-8 validation)
// Optimized with fast path for pure ASCII/UTF-8 content to avoid string allocation.
func (ed *EncodingDetector) DetectCharsetBasic(data []byte) string {
	dataLen := len(data)
	if dataLen == 0 {
		return "utf-8"
	}

	// Check for a byte-order mark. detectBOMCharset is shared with
	// DetectCharsetSmart so the two paths cannot disagree on what a BOM
	// declares.
	if bom := detectBOMCharset(data); bom != "" {
		return bom
	}

	// Pre-compute sample size once
	sampleSize := min(dataLen, 1024)
	sample := data[:sampleSize]

	// Fast path: a pure-ASCII sample (no high bytes) is valid UTF-8 and needs
	// no charset conversion. isPureASCII is the same helper scoreUTF8 uses —
	// its 64-bit word-at-a-time scan replaced a byte loop that had drifted
	// from the optimized twin defined in this file.
	if isPureASCII(sample) {
		// For pure ASCII, we still need to check meta tags for declared encoding
		// Use byte-based extraction to avoid string allocation
		if declaredCharset := extractCharsetFromBytes(sample); declaredCharset != "" {
			// A value that normalizes to nothing (whitespace-only) is not a
			// usable declaration: returning "" as the detected charset would
			// surface downstream as an "unsupported encoding \"\"" error.
			if c := normalizeCharset(declaredCharset); c != "" {
				return c
			}
		}
		// Pure ASCII with no usable charset declaration defaults to UTF-8
		return "utf-8"
	}

	// Data has non-ASCII bytes - need full UTF-8 validation
	isValidUTF8 := utf8.Valid(data)

	if isValidUTF8 {
		// Non-ASCII valid UTF-8: trust the data over any meta declaration,
		// which is likely wrong for such files (many documents declare a
		// legacy charset while actually being UTF-8). Pure ASCII never
		// reaches here — it returned above — so the !isPureASCII guard this
		// block used to carry was a tautology, and the meta scan inside it
		// had no observable effect: both its early return and this one
		// produced "utf-8". Returning here also skips the sample string
		// copy below on the hottest path.
		return "utf-8"
	}

	// Safe conversion: create a string copy only when needed
	// This is necessary because the original data slice may be modified by the caller
	htmlStart := string(sample)

	// Try to detect from meta tags using fast string-based extraction. The
	// non-empty guard keeps a whitespace-only declared value (e.g.
	// <meta charset=" ">) from returning "" as the detected charset.
	if declaredCharset := extractCharsetFromHTML(htmlStart); declaredCharset != "" {
		if c := normalizeCharset(declaredCharset); c != "" {
			return c
		}
	}

	// Fallback to regex patterns for edge cases not handled by string extraction
	if matches := charsetPattern.FindStringSubmatch(htmlStart); len(matches) > 1 {
		if c := normalizeCharset(matches[1]); c != "" {
			return c
		}
	}
	if matches := charsetPatternAlt.FindStringSubmatch(htmlStart); len(matches) > 1 {
		if c := normalizeCharset(matches[1]); c != "" {
			return c
		}
	}

	// Default to windows-1252 for HTML (most common fallback for Western content).
	// (isValidUTF8 need not be re-tested here: when it held, the valid-UTF-8
	// block above already returned, because non-ASCII bytes are guaranteed at
	// this point.)
	return "windows-1252"
}

// sampleSizeOrDefault returns the configured statistical-analysis sample size,
// falling back to the default documented on MaxSampleSize when unset (<=0). It
// centralizes the bound shared by DetectCharsetSmart and tryAllEncodings so the
// two sampling paths cannot drift.
//
// The 1MB cap mirrors SetMaxSampleSize's documented maximum: MaxSampleSize is
// an exported field a caller can set directly, so clamping only in the setter
// left the documented bound unenforced for a hand-built EncodingDetector.
func (ed *EncodingDetector) sampleSizeOrDefault() int {
	const maxSize = 1024 * 1024 // 1MB, matches SetMaxSampleSize
	if ed.MaxSampleSize > 0 {
		return min(ed.MaxSampleSize, maxSize)
	}
	return 10240
}

// DetectCharsetSmart performs intelligent charset detection using statistical analysis
func (ed *EncodingDetector) DetectCharsetSmart(data []byte) EncodingMatch {
	return ed.detectCharsetSmart(data, ed.DetectCharsetBasic(data))
}

// detectCharsetSmart is DetectCharsetSmart with the caller-supplied basic-pass
// result, so DetectCharset can run basic ONCE and share it: its low-confidence
// fallback needs the basic answer, and re-running the whole basic pass —
// full-document UTF-8 validation plus the BOM/meta scan — doubled the work on
// exactly that path.
func (ed *EncodingDetector) detectCharsetSmart(data []byte, basicCharset string) EncodingMatch {
	// Honor an explicit override first, mirroring DetectCharset. Without this a
	// direct caller of DetectCharsetSmart with ForcedEncoding set got
	// auto-detection instead; DetectAndConvert used to short-circuit this
	// externally, but the method's contract is self-consistent only if it honors
	// the override itself.
	if ed.ForcedEncoding != "" {
		c := normalizeCharset(ed.ForcedEncoding)
		return EncodingMatch{
			Charset:    c,
			Confidence: 100,
			Score:      100,
			Valid:      utf8.Valid(data),
		}
	}

	// A byte-order mark is a deterministic encoding declaration embedded in the
	// data itself, not a heuristic; treat it as authoritative. Previously the
	// BOM-derived charset still had to survive the statistical gate below
	// (score >= 90), and a single stray control byte in a UTF-16 document's
	// sample dropped it to 89 — at which point step 2, whose candidate list has
	// no UTF-16 entry, let an 8-bit charset win (its decode of the
	// NUL-interleaved bytes scored as printable text) and the document came
	// back as complete mojibake with a nil error.
	if bomCharset := detectBOMCharset(data); bomCharset != "" {
		return EncodingMatch{
			Charset:    bomCharset,
			Confidence: 100,
			Score:      100,
			Valid:      true,
		}
	}

	// The basic pass result arrives as a parameter (see detectCharsetSmart).

	// Step 1: Quick validation of basic detection
	// For meta-tag declared UTF-8, give it high priority
	//
	// Bound the statistical scoring to MaxSampleSize bytes, exactly as
	// tryAllEncodings does below: scoring is statistical (printable/CJK/control
	// ratios) and the leading sample is representative. Without this bound a
	// large non-UTF-8 document is fully decoded here (scoreEncodingMatch's
	// io.ReadAll) on top of the ~13-candidate pass in tryAllEncodings,
	// amplifying a big non-ASCII input's memory cost before that pass even runs.
	scoringSample := data
	if maxSample := ed.sampleSizeOrDefault(); len(scoringSample) > maxSample {
		scoringSample = scoringSample[:maxSample]
	}
	score := ed.scoreEncodingMatch(scoringSample, basicCharset)

	if score >= 90 {
		return EncodingMatch{
			Charset:    basicCharset,
			Confidence: 95,
			Score:      score,
			Valid:      true,
		}
	}

	// Check if basic detection came from meta tag. Evaluated after the
	// score >= 90 branch so the confidence scale is uniform: a utf-8 meta
	// declaration with a 90+ score previously reported 90 while any other
	// charset at the same score reported 95.
	if basicCharset == "utf-8" && score >= 70 {
		// Meta tag declared UTF-8 with good score - trust it
		return EncodingMatch{
			Charset:    basicCharset,
			Confidence: 90,
			Score:      score,
			Valid:      true,
		}
	}

	// Step 2: Try multiple encodings and pick the best match
	matches := ed.tryAllEncodings(data)

	// Boost the basic-detection charset: it may come from a meta declaration,
	// a BOM, or the windows-1252 default fallback, so this is a mild prior in
	// favor of the cheap pass's answer rather than a provenance-specific
	// boost (basicCharset does not track where it came from).
	for i := range matches {
		if matches[i].Charset == basicCharset {
			matches[i].Score += 10
			matches[i].Confidence += 5
			// Confidence is documented as 0-100; clamp after the boost so a
			// match already at 100 cannot surface as 105 to callers that
			// treat it as a percentage.
			if matches[i].Confidence > 100 {
				matches[i].Confidence = 100
			}
			break
		}
	}

	// Step 3: Validate and score each encoding
	var bestMatch EncodingMatch
	for _, match := range matches {
		if match.Score > bestMatch.Score || (match.Score == bestMatch.Score && match.Confidence > bestMatch.Confidence) {
			bestMatch = match
		}
	}

	// If all matches have low scores, fall back to basic detection
	if bestMatch.Confidence < 50 {
		bestMatch = EncodingMatch{
			Charset:    basicCharset,
			Confidence: 50,
			Score:      score,
			Valid:      utf8.Valid(data),
		}
	}

	return bestMatch
}

// stripUTF8BOM removes a leading UTF-8 byte-order mark (0xEF 0xBB 0xBF, the
// U+FEFF encoding) from data. Browsers strip it during decoding; leaving it
// in lets U+FEFF survive into the parsed document as a stray text node and
// from there into extracted text. Shared by ToUTF8 (declared/forced utf-8
// branch) and the valid-UTF-8 fast path of detectAndConvertToUTF8StringCore,
// which previously only the former reached — the same bytes came back
// different depending on which path handled them.
func stripUTF8BOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}

// ToUTF8 converts the given data from the detected charset to UTF-8
func (ed *EncodingDetector) ToUTF8(data []byte, charset string) ([]byte, error) {
	charset = normalizeCharset(charset)

	// If already UTF-8, return as-is. ("utf8", "utf_8", and other aliases are
	// normalized to "utf-8" by normalizeCharset above, so only the canonical
	// form can reach this point.)
	if charset == "utf-8" {
		// A declared or forced "utf-8" must not silently pass invalid bytes
		// through: a lying <meta charset="utf-8"> or ForcedEncoding on
		// non-UTF-8 data used to return undecodable bytes with a nil error,
		// violating this function's contract. Mirror the unknown-encoding
		// branch's honesty below.
		if !utf8.Valid(data) {
			return nil, fmt.Errorf("declared charset %q but data is not valid UTF-8", charset)
		}
		// Browsers strip a UTF-8 BOM during decoding; do the same so it does
		// not survive into the parsed document as a stray U+FEFF text node.
		return stripUTF8BOM(data), nil
	}

	// Get the appropriate encoding
	enc := getEncoding(charset)
	if enc == nil {
		// Unknown encoding. If the bytes are already valid UTF-8, return them
		// unchanged; otherwise the data cannot be decoded safely. Surface an
		// error rather than silently returning undecodable bytes that would
		// masquerade as successfully converted content downstream.
		if utf8.Valid(data) {
			return data, nil
		}
		return nil, fmt.Errorf("unsupported encoding %q", charset)
	}

	// Create a transformer
	transformer := enc.NewDecoder()

	// Convert the data
	reader := transform.NewReader(bytes.NewReader(data), transformer)
	converted, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return converted, nil
}

// DetectAndConvert detects charset and converts to UTF-8 in one step
func (ed *EncodingDetector) DetectAndConvert(data []byte) ([]byte, string, error) {
	// Both detection branches honor ForcedEncoding (DetectCharsetSmart now does
	// so directly, as DetectCharset always has), so an explicit override is
	// respected regardless of whether smart detection is enabled.
	var charset string
	if ed.EnableSmartDetection {
		charset = ed.DetectCharsetSmart(data).Charset
	} else {
		charset = ed.DetectCharset(data)
	}
	converted, err := ed.ToUTF8(data, charset)
	return converted, charset, err
}

// maxCachedCharsetLen bounds which charset names may enter charsetNormCache.
// Cache keys can originate from an untrusted <meta charset> declaration in the
// first 1KB of arbitrary input, and an unbounded sync.Map keyed by
// attacker-controlled strings grows without limit under sustained hostile
// traffic. Real charset names are far shorter than this bound; longer inputs
// bypass the cache and pay only the cheap slow-path normalization.
const maxCachedCharsetLen = 40

// maxCachedCharsetEntries bounds the number of DISTINCT keys the cache may
// hold. The length bound alone does not stop growth: keys come from untrusted
// <meta charset> values, and an attacker sending unique ≤40-character names
// inserted a new entry per request — exactly the unbounded growth the length
// bound's comment claims to prevent. Real charset names number ~50, so 256 is
// ample headroom while capping retention; uncached names pay only the cheap
// slow-path normalization.
const maxCachedCharsetEntries = 256

// charsetNormCacheEntries counts entries inserted into charsetNormCache so
// the cardinality bound above can be enforced without locking.
var charsetNormCacheEntries atomic.Int64

// normalizeCharset normalizes charset names to a standard form.
// Uses a sync.Map cache to avoid repeated string operations for common charset names.
func normalizeCharset(charset string) string {
	if len(charset) > maxCachedCharsetLen {
		return normalizeCharsetSlow(charset)
	}
	// Check cache first
	if v, ok := charsetNormCache.Load(charset); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}

	// Cache miss - compute and store (bounded; see maxCachedCharsetEntries).
	normalized := normalizeCharsetSlow(charset)
	if charsetNormCacheEntries.Load() < maxCachedCharsetEntries {
		// LoadOrStore is atomic per key, so exactly one of the goroutines
		// simultaneously missing a given key performs the insert, and only
		// that goroutine counts. A plain Store let every concurrent misser
		// Add(1) for the same key, inflating the counter and permanently
		// capping the cache below its advertised cardinality. The cardinality
		// check itself stays a soft bound: simultaneous misses of distinct
		// keys may briefly overshoot it by a few entries, never unboundedly.
		if _, loaded := charsetNormCache.LoadOrStore(charset, normalized); !loaded {
			charsetNormCacheEntries.Add(1)
		}
	}
	return normalized
}

// normalizeCharsetSlow performs the actual charset normalization.
// This is the slow path called when the charset is not in the cache.
func normalizeCharsetSlow(charset string) string {
	charset = strings.ToLower(strings.TrimSpace(charset))

	// Remove common prefixes.
	// NOTE: "iso-" and "iso_" are intentionally NOT stripped. The canonical
	// ISO-8859 names ("iso-8859-2", "iso-8859-7", etc.) must reach getEncoding
	// intact; stripping "iso-" would turn "iso-8859-2" into "8859-2", which has
	// no alias entry and no getEncoding case, silently disabling decoding for
	// ISO-8859-2..10/13/14/16. ISO-8859-1/-15 keep working because their variant
	// forms ("iso88591", "iso_8859-1", ...) are listed in charsetAliases.
	for _, prefix := range []string{"text/", "text-", "windows-", "cp", "codepage-", "ibm-"} {
		charset = strings.TrimPrefix(charset, prefix)
	}

	// The latin-N family maps each member to its own ISO-8859 set (IANA
	// registered names). Collapsing the whole family to iso-8859-1 decoded
	// every Central-European, Turkish, and Nordic page with the wrong table
	// (latin2 text came out with mojibake accents). latin1/latin-1 are also
	// covered by charsetAliases below; the family stays in one switch here.
	switch charset {
	case "latin1", "latin-1":
		return "iso-8859-1"
	case "latin2", "latin-2":
		return "iso-8859-2"
	case "latin3", "latin-3":
		return "iso-8859-3"
	case "latin4", "latin-4":
		return "iso-8859-4"
	case "latin5", "latin-5":
		return "iso-8859-9"
	case "latin6", "latin-6":
		return "iso-8859-10"
	case "latin7", "latin-7":
		return "iso-8859-13"
	case "latin8", "latin-8":
		return "iso-8859-14"
	case "latin9", "latin-9":
		return "iso-8859-15"
	case "latin10", "latin-10":
		return "iso-8859-16"
	}

	// Look up canonical name in alias map
	if canonical, ok := charsetAliases[charset]; ok {
		return canonical
	}

	return charset
}

// getEncoding returns the encoding for the given charset name
func getEncoding(charset string) encoding.Encoding {
	switch charset {
	case "windows-1252":
		return charmap.Windows1252
	case "windows-1251":
		return charmap.Windows1251 // Cyrillic
	case "windows-1250":
		return charmap.Windows1250 // Central European
	case "windows-1253":
		return charmap.Windows1253 // Greek
	case "windows-1254":
		return charmap.Windows1254 // Turkish
	case "windows-1255":
		return charmap.Windows1255 // Hebrew
	case "windows-1256":
		return charmap.Windows1256 // Arabic
	case "windows-1258":
		return charmap.Windows1258 // Vietnamese
	case "koi8-r":
		return charmap.KOI8R // Cyrillic
	case "iso-8859-1":
		return charmap.ISO8859_1
	case "iso-8859-15":
		return charmap.ISO8859_15 // Western European with Euro
	case "iso-8859-2":
		return charmap.ISO8859_2 // Central European
	case "iso-8859-3":
		return charmap.ISO8859_3 // South European
	case "iso-8859-4":
		return charmap.ISO8859_4 // Baltic
	case "iso-8859-5":
		return charmap.ISO8859_5 // Cyrillic
	case "iso-8859-6":
		return charmap.ISO8859_6 // Arabic
	case "iso-8859-7":
		return charmap.ISO8859_7 // Greek
	case "iso-8859-8":
		return charmap.ISO8859_8 // Hebrew
	case "iso-8859-9":
		return charmap.ISO8859_9 // Turkish
	case "iso-8859-10":
		return charmap.ISO8859_10 // Nordic
	case "iso-8859-13":
		return charmap.ISO8859_13 // Baltic
	case "iso-8859-14":
		return charmap.ISO8859_14 // Celtic
	case "iso-8859-16":
		return charmap.ISO8859_16 // Southeast European
	case "utf-16le":
		return unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)
	case "utf-16be":
		return unicode.UTF16(unicode.BigEndian, unicode.UseBOM)
	case "shift_jis":
		return japanese.ShiftJIS
	case "euc-jp":
		return japanese.EUCJP
	case "iso-2022-jp":
		return japanese.ISO2022JP
	case "euc-kr":
		return korean.EUCKR
	case "gbk":
		return simplifiedchinese.GBK
	case "gb18030":
		return simplifiedchinese.GB18030
	case "big5":
		return traditionalchinese.Big5
	default:
		return nil
	}
}

// encodingDetectorPool pools EncodingDetector instances to reduce allocation
// in the encoding detection hot path for non-ASCII content.
var encodingDetectorPool = sync.Pool{
	New: func() any {
		return NewEncodingDetector()
	},
}

// detectAndConvertToUTF8StringCore is the shared implementation for UTF-8 string conversion.
// When safeCopy is true, it always creates a copy of the data for memory isolation.
// When safeCopy is false, it may use zero-copy for ASCII input for performance.
func detectAndConvertToUTF8StringCore(data []byte, forcedEncoding string, safeCopy bool) (string, string, error) {
	var result string
	var charset string

	// Fast path: if no forced encoding, try quick ASCII/UTF-8 detection
	if forcedEncoding == "" {
		if isPureASCII(data) {
			// ASCII needs no NFC pass: it contains no combining characters or
			// multi-codepoint composites, so it is trivially in Normalization
			// Form C. Returning here skips the norm.NFC quickSpan scan of the
			// whole document that would otherwise run (and allocate nothing but
			// still cost a full pass) on this hottest of paths.
			if safeCopy {
				return string(data), "utf-8", nil
			}
			// Zero-copy: ASCII bytes are identical to UTF-8 bytes
			return BytesToString(data), "utf-8", nil
		}
		if utf8.Valid(data) {
			// BOM-prefixed input is valid UTF-8 (the BOM is a legal U+FEFF
			// encoding), so without this strip the fast path would hand the
			// BOM straight to the parser as a stray text node. ToUTF8 — the
			// slow path — already strips it; mirror it here so both paths
			// agree on the same input.
			data = stripUTF8BOM(data)
			if safeCopy {
				result = string(data)
			} else {
				// Zero-copy: the bytes are valid UTF-8, so they are directly
				// usable as a string. This extends the pure-ASCII sharing below
				// to every valid-UTF-8 input, removing one full-document copy
				// per extraction of non-ASCII (e.g. CJK) content. The NFC pass
				// that follows returns the string unchanged when no rune needs
				// normalization (norm.Form.String's quickSpan), so the aliasing
				// survives to the caller — permitted by this (unsafe) variant's
				// documented contract. The Safe variant keeps string(data).
				result = BytesToString(data)
			}
			charset = "utf-8"
		}
	}

	// If not already processed as valid UTF-8, try encoding detection
	if result == "" {
		ed := encodingDetectorPool.Get().(*EncodingDetector)
		ed.ForcedEncoding = forcedEncoding

		convertedBytes, detectedCharset, err := ed.DetectAndConvert(data)

		ed.ForcedEncoding = ""
		encodingDetectorPool.Put(ed)

		if err != nil {
			return "", "", err
		}

		result = string(convertedBytes)
		charset = detectedCharset
	}

	// SECURITY: Apply NFC normalization to prevent Unicode-based bypass attacks.
	normalized := norm.NFC.String(result)

	return normalized, charset, nil
}

// DetectAndConvertToUTF8String detects encoding and converts to UTF-8 string.
// If forcedEncoding is not empty, it will use that encoding instead of auto-detection.
// Returns a UTF-8 string and the detected/used encoding.
//
// SECURITY WARNING: When the input is pure ASCII or valid UTF-8 and
// forcedEncoding is empty, the returned string shares memory with the input
// []byte slice. The caller MUST NOT modify the input slice after calling this
// function if the returned string will be used. For memory-isolated usage, use
// DetectAndConvertToUTF8StringSafe.
func DetectAndConvertToUTF8String(data []byte, forcedEncoding string) (string, string, error) {
	return detectAndConvertToUTF8StringCore(data, forcedEncoding, false)
}

// DetectAndConvertToUTF8StringSafe is a memory-safe version of DetectAndConvertToUTF8String
// that always returns a copy of the data, even for pure ASCII input.
// Use this when the input []byte slice may be modified after the call.
func DetectAndConvertToUTF8StringSafe(data []byte, forcedEncoding string) (string, string, error) {
	return detectAndConvertToUTF8StringCore(data, forcedEncoding, true)
}

// isPureASCII checks if data contains only ASCII bytes (0x00-0x7F)
// Optimized with 64-bit word reads and batch processing for maximum throughput.
// Processes 64 bytes per iteration using 8 x 64-bit loads.
//
// SECURITY: This function uses unsafe pointer operations for performance.
// It includes defensive bounds checking to prevent out-of-bounds reads.
func isPureASCII(data []byte) bool {
	n := len(data)
	if n == 0 {
		return true
	}

	// High bit mask for detecting non-ASCII bytes
	const highBitMask uint64 = 0x8080808080808080

	// SECURITY: Validate slice has sufficient capacity for unsafe reads.
	// This defensive check ensures we don't read beyond allocated memory.
	// For small slices, fall back to safe byte-by-byte checking.
	if n < 8 {
		for i := range n {
			if data[i] >= 0x80 {
				return false
			}
		}
		return true
	}

	// Use unsafe for direct memory reads on sufficiently large slices
	basePtr := unsafe.Pointer(unsafe.SliceData(data))
	i := 0

	// Process 64 bytes at a time (8 x 64-bit words) for maximum throughput
	for i+64 <= n {
		// Load 8 words and OR them together
		w0 := *(*uint64)(unsafe.Add(basePtr, i))
		w1 := *(*uint64)(unsafe.Add(basePtr, i+8))
		w2 := *(*uint64)(unsafe.Add(basePtr, i+16))
		w3 := *(*uint64)(unsafe.Add(basePtr, i+24))
		w4 := *(*uint64)(unsafe.Add(basePtr, i+32))
		w5 := *(*uint64)(unsafe.Add(basePtr, i+40))
		w6 := *(*uint64)(unsafe.Add(basePtr, i+48))
		w7 := *(*uint64)(unsafe.Add(basePtr, i+56))

		// Single branch: check if any word has high bit set
		if (w0|w1|w2|w3|w4|w5|w6|w7)&highBitMask != 0 {
			return false
		}
		i += 64
	}

	// Process remaining 32-byte chunks
	for i+32 <= n {
		w0 := *(*uint64)(unsafe.Add(basePtr, i))
		w1 := *(*uint64)(unsafe.Add(basePtr, i+8))
		w2 := *(*uint64)(unsafe.Add(basePtr, i+16))
		w3 := *(*uint64)(unsafe.Add(basePtr, i+24))

		if (w0|w1|w2|w3)&highBitMask != 0 {
			return false
		}
		i += 32
	}

	// Process remaining 8-byte chunks
	for i+8 <= n {
		w := *(*uint64)(unsafe.Add(basePtr, i))
		if w&highBitMask != 0 {
			return false
		}
		i += 8
	}

	// Handle remaining bytes using safe indexing
	for i < n {
		if data[i] >= 0x80 {
			return false
		}
		i++
	}

	return true
}

// tryAllEncodings attempts to decode the data with multiple encodings and scores each result.
// Optimized to avoid redundant UTF-8 validation and conversions.
func (ed *EncodingDetector) tryAllEncodings(data []byte) []EncodingMatch {
	// Bound the statistical analysis to MaxSampleSize bytes. Scoring is
	// statistical (printable/CJK/control-char ratios), so the leading sample is
	// representative; without this, each of ~13 candidate encodings decodes and
	// full-scans the entire document, amplifying a large non-ASCII input ~13×.
	// This also makes the previously write-only MaxSampleSize field effective.
	if maxSample := ed.sampleSizeOrDefault(); len(data) > maxSample {
		data = data[:maxSample]
	}

	// Common encodings to try, ordered by likelihood
	candidateEncodings := []struct {
		name string
		prio int // Priority (higher = more likely)
	}{
		{"utf-8", 100},
		{"windows-1252", 90},
		{"gbk", 80},          // Simplified Chinese
		{"shift_jis", 75},    // Japanese
		{"euc-jp", 70},       // Japanese
		{"euc-kr", 65},       // Korean
		{"big5", 60},         // Traditional Chinese
		{"iso-8859-1", 50},   // Western European
		{"iso-8859-2", 45},   // Central European
		{"windows-1250", 43}, // Central European
		{"windows-1251", 40}, // Cyrillic
		{"iso-8859-5", 38},   // Cyrillic
		{"iso-2022-jp", 35},  // Japanese (ISO-2022-JP)
	}

	// Pre-check UTF-8 validity once (avoids redundant checks in scoreEncodingMatch)
	isUTF8Valid := utf8.Valid(data)

	matches := make([]EncodingMatch, 0, len(candidateEncodings))

	for _, candidate := range candidateEncodings {
		score := ed.scoreEncodingMatchOptimized(data, candidate.name, isUTF8Valid)
		if score > 0 {
			confidence := calculateConfidence(score, candidate.prio)
			matches = append(matches, EncodingMatch{
				Charset:    candidate.name,
				Confidence: confidence,
				Score:      score,
				Valid:      score >= 40, // Threshold for considering it valid
			})
			// Early exit for high-confidence match to avoid unnecessary encoding checks
			if confidence >= 95 && score >= 90 {
				return matches
			}
		}
	}

	return matches
}

// scoreEncodingMatchOptimized scores how well a charset matches the data.
// Optimized version that accepts pre-computed UTF-8 validity to avoid redundant checks.
func (ed *EncodingDetector) scoreEncodingMatchOptimized(data []byte, charset string, isUTF8Valid bool) int {
	normalizedCharset := normalizeCharset(charset)

	// UTF-8 special case: use pre-computed validity
	if normalizedCharset == "utf-8" {
		if !isUTF8Valid {
			return 0
		}
		return ed.scoreUTF8(data)
	}

	// Get the encoding
	enc := getEncoding(normalizedCharset)
	if enc == nil {
		return 0
	}

	// Attempt decoding
	transformer := enc.NewDecoder()
	reader := transform.NewReader(bytes.NewReader(data), transformer)
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return 0
	}

	// Score the decoded result
	return ed.scoreDecodedData(decoded, normalizedCharset)
}

// scoreEncodingMatch scores how well a charset matches the data
func (ed *EncodingDetector) scoreEncodingMatch(data []byte, charset string) int {
	return ed.scoreEncodingMatchOptimized(data, charset, utf8.Valid(data))
}

// scoreUTF8 scores UTF-8 data. Caller must ensure data is valid UTF-8.
func (ed *EncodingDetector) scoreUTF8(data []byte) int {
	// Pre-condition: data is valid UTF-8 (verified by caller)
	score := 40 // Base score for valid UTF-8

	// Bonus for non-ASCII content (real UTF-8, not just ASCII)
	if !isPureASCII(data) {
		score += 30
	}

	// Calculate printable character ratio
	printableRatio := calculatePrintableRatio(data)
	score += int(printableRatio * 20)

	// Check for proper UTF-8 structure
	validUTF8Ratio := calculateValidUTF8Ratio(data)
	score += int(validUTF8Ratio * 10)

	return score
}

// scoreDecodedData scores decoded data from non-UTF-8 encodings.
func (ed *EncodingDetector) scoreDecodedData(decoded []byte, charset string) int {
	score := 0

	// Base score for successful decoding
	score += 40

	// Check if result is valid UTF-8
	if !utf8.Valid(decoded) {
		return score - 50 // Heavy penalty for invalid UTF-8 output
	}
	score += 30

	// Calculate printable character ratio
	printableRatio := calculatePrintableRatio(decoded)
	if printableRatio < 0.5 {
		return score - 30 // Heavy penalty for low printable ratio
	}
	score += int(printableRatio * 20)

	// Check for replacement characters (U+FFFD)
	if bytes.Contains(decoded, []byte{0xEF, 0xBF, 0xBD}) {
		score -= 15 // Penalty for replacement characters
	}

	// Bonus for language-specific patterns
	score += ed.scoreLanguagePatterns(decoded, charset)

	// Penalty for excessive control characters
	if hasExcessiveControlChars(decoded) {
		score -= 10
	}

	return score
}

// scoreLanguagePatterns scores based on language-specific character patterns
func (ed *EncodingDetector) scoreLanguagePatterns(decoded []byte, charset string) int {
	bonus := 0

	// Count CJK characters (Chinese, Japanese, Korean)
	cjkCount := countCJKCharacters(decoded)
	if cjkCount > 0 {
		// Expected CJK for certain charsets
		switch charset {
		case "gbk", "big5", "shift_jis", "euc-jp", "euc-kr", "iso-2022-jp":
			// These charsets should have CJK characters
			cjkRatio := float64(cjkCount) / float64(len(decoded))
			bonus += int(cjkRatio * 15)
		default:
			// Western charsets with CJK: penalty
			bonus -= 10
		}
	}

	// Check for Cyrillic characters
	if hasCyrillicCharacters(decoded) {
		if charset == "windows-1251" || charset == "iso-8859-5" {
			bonus += 10
		}
	}

	return bonus
}

// calculateConfidence calculates the final confidence score based on match score and priority
func calculateConfidence(score, priority int) int {
	// Base confidence from score
	confidence := score

	// Adjust by priority
	if priority >= 90 {
		confidence += 5
	} else if priority >= 70 {
		confidence += 2
	}

	// Ensure confidence is in range [0, 100]
	if confidence > 100 {
		confidence = 100
	}
	if confidence < 0 {
		confidence = 0
	}

	return confidence
}

// Helper functions for character analysis

// calculatePrintableRatio calculates the ratio of printable characters
// Optimized version with early termination for obvious cases
func calculatePrintableRatio(data []byte) float64 {
	dataLen := len(data)
	if dataLen == 0 {
		return 0
	}

	// Sample-based analysis for large data to avoid full scan
	sampleSize := min(dataLen, 4096)

	printable := 0
	for i := range sampleSize {
		if isPrintable(data[i]) {
			printable++
		}
	}
	return float64(printable) / float64(sampleSize)
}

// isPrintable checks if a byte is a printable character
func isPrintable(b byte) bool {
	// ASCII printable range
	if b >= 32 && b <= 126 {
		return true
	}
	// Common whitespace
	if b == '\t' || b == '\n' || b == '\r' {
		return true
	}
	// Non-ASCII UTF-8 continuation bytes are likely valid
	if b >= 0x80 {
		return true
	}
	return false
}

// calculateValidUTF8Ratio calculates the ratio of valid UTF-8 sequences
// Optimized version with sample-based analysis for large inputs
func calculateValidUTF8Ratio(data []byte) float64 {
	dataLen := len(data)
	if dataLen == 0 {
		return 0
	}

	// For small data, do full validation
	if dataLen <= 4096 {
		return calculateValidUTF8RatioFull(data)
	}

	// For large data, sample the beginning
	return calculateValidUTF8RatioFull(data[:4096])
}

// calculateValidUTF8RatioFull performs full UTF-8 validation
// Optimized: manually decode UTF-8 sequences to avoid function call overhead
func calculateValidUTF8RatioFull(data []byte) float64 {
	dataLen := len(data)
	if dataLen == 0 {
		return 0
	}

	valid := 0
	i := 0
	for i < dataLen {
		b := data[i]

		// Fast path: ASCII (0x00-0x7F)
		if b < 0x80 {
			valid++
			i++
			continue
		}

		// Multi-byte sequence handling
		var seqLen int
		if b >= 0xC0 && b < 0xE0 {
			seqLen = 2
		} else if b >= 0xE0 && b < 0xF0 {
			seqLen = 3
		} else if b >= 0xF0 && b < 0xF8 {
			seqLen = 4
		} else {
			// Invalid leading byte
			i++
			continue
		}

		// Check if we have enough bytes
		if i+seqLen > dataLen {
			i++
			continue
		}

		// Validate continuation bytes
		validSeq := true
		for j := 1; j < seqLen; j++ {
			if data[i+j]&0xC0 != 0x80 {
				validSeq = false
				break
			}
		}

		if validSeq {
			valid++
		}
		i += seqLen
	}
	return float64(valid) / float64(dataLen)
}

// countCJKCharacters counts CJK (Chinese, Japanese, Korean) characters.
// Decode in place: the caller (scoreDecodedData) guarantees valid UTF-8, so
// utf8.DecodeRune steps the same rune sequence `range string(data)` did
// without the full-buffer copy per scoring candidate.
func countCJKCharacters(data []byte) int {
	count := 0
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		i += size
		// CJK Unified Ideographs block
		if (r >= 0x4E00 && r <= 0x9FFF) ||
			(r >= 0x3400 && r <= 0x4DBF) ||
			(r >= 0x20000 && r <= 0x2A6DF) ||
			(r >= 0x2A700 && r <= 0x2B73F) ||
			(r >= 0x2B740 && r <= 0x2B81F) ||
			(r >= 0x2B820 && r <= 0x2CEAF) ||
			(r >= 0x2CEB0 && r <= 0x2EBEF) ||
			// Hiragana and Katakana
			(r >= 0x3040 && r <= 0x309F) ||
			(r >= 0x30A0 && r <= 0x30FF) ||
			// Hangul Syllables
			(r >= 0xAC00 && r <= 0xD7AF) ||
			// CJK Extensions
			(r >= 0xF900 && r <= 0xFAFF) ||
			(r >= 0x2F800 && r <= 0x2FA1F) {
			count++
		}
	}
	return count
}

// hasCyrillicCharacters checks for Cyrillic alphabet characters. Decodes in
// place for the same reason countCJKCharacters does.
func hasCyrillicCharacters(data []byte) bool {
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		i += size
		if (r >= 0x0400 && r <= 0x04FF) || // Cyrillic
			(r >= 0x0500 && r <= 0x052F) || // Cyrillic Supplement
			(r >= 0x2DE0 && r <= 0x2DFF) || // Cyrillic Extended-A
			(r >= 0xA640 && r <= 0xA69F) { // Cyrillic Extended-B
			return true
		}
	}
	return false
}

// hasExcessiveControlChars checks if data has too many control characters
func hasExcessiveControlChars(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	controlCount := 0
	for _, b := range data {
		// Count control characters (excluding common whitespace)
		if b < 32 && b != '\t' && b != '\n' && b != '\r' {
			controlCount++
		}
	}
	// More than 5% control characters is excessive
	return float64(controlCount)/float64(len(data)) > 0.05
}
