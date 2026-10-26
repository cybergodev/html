// extract_stats.go holds result statistics and metadata parsing: word
// counting (incl. CJK), reading time, extraction confidence, and
// published-time layouts.

package html

import (
	"strings"
	"time"
	"unicode/utf8"
)

// asciiWordSpace marks the ASCII bytes that end a word for countWords: the

// four whitespace bytes that separate words. A table lookup replaces the

// four-way comparison chain in the per-byte loop (countWords was a profiled

// ~2% of Extract CPU on a realistic document).

var asciiWordSpace = [256]bool{
	' ':  true,
	'\t': true,
	'\n': true,
	'\r': true,
}

// countWords counts whitespace-separated words, additionally counting each

// CJK ideograph, kana, or Hangul syllable as one word. CJK text is

// conventionally written without inter-word spaces, so a whitespace-only

// scan reported a whole paragraph as a single word and skewed WordCount and

// ReadingTime for CJK documents. The cjk return value counts those CJK

// characters so calculateReadingTime can apply a per-script reading rate.

func (p *Processor) countWords(text string) (words, cjk int) {
	if text == "" {
		return 0, 0
	}
	n := len(text)
	count := 0
	inWord := false
	for i := 0; i < n; {
		// Skip ATX heading markers ("# " … "###### ") at the start of a line:
		// the structured text pass emits them, and structure is not a word.
		// A '#' not followed by a space ("#hashtag") is content and counts.
		if text[i] == '#' && (i == 0 || text[i-1] == '\n') {
			j := i
			for j < n && text[j] == '#' {
				j++
			}
			if j < n && text[j] == ' ' {
				i = j + 1
				inWord = false
				continue
			}
		}
		c := text[i]
		if c < utf8.RuneSelf {
			if asciiWordSpace[c] {
				inWord = false
			} else if !inWord {
				inWord = true
				count++
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if isCJKRune(r) {
			count++ // each CJK character reads as one word
			cjk++
			inWord = false
		} else if !inWord {
			// Non-CJK multi-byte scripts (Cyrillic, Greek, accented Latin,
			// emoji, ...) follow the same run-of-non-space rule as ASCII.
			inWord = true
			count++
		}
		i += size
	}
	return count, cjk
}

// isCJKRune reports whether r is a CJK ideograph, kana, or Hangul syllable —

// the scripts whose text is conventionally written without inter-word spaces.

func isCJKRune(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF, // CJK Unified Ideographs
		r >= 0x3400 && r <= 0x4DBF, // CJK Extension A
		r >= 0xF900 && r <= 0xFAFF, // CJK Compatibility Ideographs
		r >= 0x3040 && r <= 0x30FF, // Hiragana / Katakana
		r >= 0xAC00 && r <= 0xD7AF: // Hangul Syllables
		return true
	}
	return false
}

// calculateReadingTime estimates reading time using separate rates per

// script: wordsPerMinute for space-separated words, cjkCharsPerMinute for

// CJK characters (countWords already folded each CJK character into words,

// so the CJK share is split out here). CJK reading speed is conventionally

// quoted in characters per minute, and it is roughly 1.75× the English

// word-per-minute rate.

func (p *Processor) calculateReadingTime(wordCount, cjkChars int) time.Duration {
	if wordCount == 0 {
		return 0
	}
	latinWords := max(0, wordCount-cjkChars)
	minutes := float64(latinWords)/wordsPerMinute + float64(cjkChars)/cjkCharsPerMinute
	return time.Duration(minutes * float64(time.Minute))
}

// lowConfidencePhrases are consent-wall and paywall boilerplate markers. A

// short extraction whose opening text matches one of them almost certainly

// captured the wall instead of the article (the real content is absent from

// the DOM, hidden, or behind an interaction), which computeExtractionConfidence

// reports as a low tier instead of a failure.

var lowConfidencePhrases = [...]string{
	"subscribe to continue", "subscribe to read", "subscription to continue",
	"sign in to continue", "log in to continue", "login to continue",
	"register to read", "create a free account",
	"enable cookies", "accept all cookies", "we use cookies", "accept to continue",
}

// computeExtractionConfidence derives Result.Confidence, a coarse

// extraction-quality signal in [0, 1]:

//
//	0.0  no text extracted
//	0.4  short (<600 chars) extraction opening with consent/paywall boilerplate,
//	     or an article node yielding almost no text
//	0.2/0.4/0.6  body-fallback extraction, tiered by text length
//	0.6          recognized listing pages (the card list is the page's content,
//	             but it is not a canonical single-article extraction)
//	0.6/0.8      article node with short text, or text covering <5% of the
//	             page's visible body text
//	1.0          article node, ≥ confidentTextChars of text, covering at least
//	             confidentBodyRatio of the body text
//
// The tiers are deliberately few and documented: callers should treat the

// value as a triage signal (route <1.0 for review or a fallback strategy),

// not as a probability.

func computeExtractionConfidence(method, text string, bodyTextLen int) float64 {
	textLen := len(text)
	if textLen == 0 {
		return 0
	}

	if method == extractionMethodListing {
		// A listing's card list is the page's real content, but consumers
		// treat articles and listings differently; the paywall check below
		// still applies to listings trapped behind a wall.
		if textLen < paywallTextMaxChars && containsLowConfidencePhrase(text) {
			return 0.4
		}
		return 0.6
	}

	if textLen < paywallTextMaxChars && containsLowConfidencePhrase(text) {
		return 0.4
	}

	// A sane article covers a reasonable share of the page's visible text. A
	// body with no text (or no <body> at all) makes the ratio vacuous — treat
	// it as satisfied rather than penalizing every extraction from it.
	ratioOK := bodyTextLen <= 0 || float64(textLen) >= confidentBodyRatio*float64(bodyTextLen)

	if method == extractionMethodArticle {
		switch {
		case textLen >= confidentTextChars && ratioOK:
			return 1.0
		case textLen >= confidentTextChars:
			return 0.8
		case textLen >= 50:
			return 0.6
		default:
			return 0.4
		}
	}
	switch {
	case textLen >= confidentTextChars:
		return 0.6
	case textLen >= 50:
		return 0.4
	default:
		return 0.2
	}
}

// containsLowConfidencePhrase reports whether text's opening (up to

// paywallScanChars, rune-boundary safe) contains a consent/paywall boilerplate

// phrase.

func containsLowConfidencePhrase(text string) bool {
	scan := text
	if len(scan) > paywallScanChars {
		scan = scan[:paywallScanChars]
		// Back off to a rune boundary so ToLower cannot see a split rune.
		for len(scan) > 0 && scan[len(scan)-1]&0xC0 == 0x80 {
			scan = scan[:len(scan)-1]
		}
	}
	scan = strings.ToLower(scan)
	for _, phrase := range lowConfidencePhrases {
		if strings.Contains(scan, phrase) {
			return true
		}
	}
	return false
}

// publishedTimeLayouts are the timestamp layouts metadata producers actually

// emit, most specific first: RFC 3339 (time.Parse accepts an optional

// fractional-seconds field even when the layout does not signify one, so a

// separate RFC3339Nano layout is unnecessary), timezone-less ISO-ish forms

// (with seconds and minute precision), and a bare date.

var publishedTimeLayouts = [...]string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
}

// parsePublishedTime parses a raw metadata timestamp, returning the zero time

// when it is empty or matches none of the known layouts.

func parsePublishedTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range publishedTimeLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t
		}
	}
	return time.Time{}
}
