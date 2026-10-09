package internal

import (
	"strings"
)

const (
	// Video and audio MIME types
	mimeMP4       = "video/mp4"
	mimeWebM      = "video/webm"
	mimeOGGVideo  = "video/ogg"
	mimeQuicktime = "video/quicktime"
	mimeAVI       = "video/x-msvideo"
	mimeWMV       = "video/x-ms-wmv"
	mimeFLV       = "video/x-flv"
	mimeMKV       = "video/x-matroska"
	mime3GP       = "video/3gpp"

	mimeMPEG  = "audio/mpeg"
	mimeWAV   = "audio/wav"
	mimeOGG   = "audio/ogg"
	mimeM4A   = "audio/mp4"
	mimeAAC   = "audio/aac"
	mimeFLAC  = "audio/flac"
	mimeWMA   = "audio/x-ms-wma"
	mimeOpus  = "audio/opus"
	mimeEmbed = "embed"
)

// videoExtensions and audioExtensions map file extensions to MIME types for the
// regex/tag-based media extraction in the public package. They intentionally
// overlap on ".ogg": an OGG container can carry either video (Theora) or audio
// (Vorbis/Opus), and the extension alone cannot disambiguate, so a single
// ".ogg" URL is detected as both video (video/ogg) and audio (audio/ogg) and may
// appear in both Result.Videos and Result.Audios. The audio-only variant ".oga"
// is listed only under audioExtensions.
var (
	// Video extensions for video-specific detection
	videoExtensions = map[string]string{
		".mp4": mimeMP4, ".m4v": mimeMP4, ".webm": mimeWebM,
		".ogg": mimeOGGVideo, ".mov": mimeQuicktime, ".avi": mimeAVI,
		".wmv": mimeWMV, ".flv": mimeFLV, ".mkv": mimeMKV,
		".3gp": mime3GP,
	}

	// Audio extensions for audio-specific detection
	audioExtensions = map[string]string{
		".mp3": mimeMPEG, ".wav": mimeWAV, ".ogg": mimeOGG,
		".oga": mimeOGG, ".m4a": mimeM4A, ".aac": mimeAAC,
		".flac": mimeFLAC, ".wma": mimeWMA, ".opus": mimeOpus,
	}

	embedPatterns = []string{
		"youtube.com/embed/",
		"youtube-nocookie.com/embed/",
		"player.vimeo.com/video/",
		"dailymotion.com/embed/",
		"player.youku.com/",
		"v.qq.com/",
		"bilibili.com/",
	}

	// mediaPatterns groups every media signature — file extensions (".mp4", ".mp3",
	// ...) and embed-host patterns ("youtube.com/embed/", ...) — by its first byte.
	// HasMediaReference uses it to find any signature in a single allocation-free
	// pass, checking only the few signatures that start with the byte at the current
	// position instead of scanning once per pattern.
	//
	// Each signature is indexed under BOTH ASCII cases of its first byte (e.g. under
	// both 'y' and 'Y'), so the scan looks up mediaPatterns[c] directly without a
	// per-byte case-fold branch. The match itself (asciiFoldHasPrefix) is still
	// case-insensitive over the whole signature.
	mediaPatterns [256][]string

	// mediaFirstByte mirrors mediaPatterns as a compact presence bitmap. The
	// HasMediaReference scan consults it first so bytes that start no signature
	// — the overwhelming majority — cost one byte load from this 256-byte table
	// instead of a slice-header load from the 4 KiB mediaPatterns table.
	mediaFirstByte [256]bool
)

func init() {
	addSignature := func(sig string) {
		if sig == "" {
			return
		}
		first := sig[0]
		// All signatures are lowercase by construction, but handle either case
		// defensively: index under the byte itself and its ASCII-case counterpart.
		mediaPatterns[first] = append(mediaPatterns[first], sig)
		var other byte
		if first >= 'a' && first <= 'z' {
			other = first - 32
		} else if first >= 'A' && first <= 'Z' {
			other = first + 32
		}
		if other != 0 {
			mediaPatterns[other] = append(mediaPatterns[other], sig)
		}
	}
	for ext := range videoExtensions {
		addSignature(ext)
	}
	for ext := range audioExtensions {
		addSignature(ext)
	}
	for _, pattern := range embedPatterns {
		addSignature(pattern)
	}
	for b, bucket := range mediaPatterns {
		mediaFirstByte[b] = len(bucket) > 0
	}
}

// IsVideoURL checks if a URL is a video based on extension or embed pattern
func IsVideoURL(url string) bool {
	lowerURL := strings.ToLower(url)
	return detectVideoType(lowerURL) != "" || hasEmbedPattern(lowerURL)
}

// DetectVideoType detects the video MIME type from a URL
func DetectVideoType(url string) string {
	lowerURL := strings.ToLower(url)
	if mimeType := detectVideoType(lowerURL); mimeType != "" {
		return mimeType
	}
	if hasEmbedPattern(lowerURL) {
		return mimeEmbed
	}
	return ""
}

// DetectAudioType detects the audio MIME type from a URL
func DetectAudioType(url string) string {
	lowerURL := strings.ToLower(url)
	return detectAudioType(lowerURL)
}

// detectMediaTypeByExtension returns the MIME type for url based on a trailing
// extension in exts. Query parameters and fragments are stripped first so that
// URLs like "song.mp3?v=2#audio" still match. The two callers pass distinct,
// non-overlapping maps (videoExtensions / audioExtensions), so iteration order
// does not affect the result.
//
// Instead of iterating every extension with strings.HasSuffix (O(n) map
// iteration), it extracts the suffix after the last '.' and does a single O(1)
// map lookup. This is both faster (one hash lookup vs n suffix comparisons) and
// more correct: the longest trailing extension wins deterministically.
func detectMediaTypeByExtension(url string, exts map[string]string) string {
	// Remove query parameters and fragments
	if idx := strings.IndexByte(url, '?'); idx >= 0 {
		url = url[:idx]
	}
	if idx := strings.IndexByte(url, '#'); idx >= 0 {
		url = url[:idx]
	}

	// Extract the file extension: everything from the last '.' onward.
	// A direct map lookup replaces the O(n) iteration of HasSuffix checks.
	dotIdx := strings.LastIndexByte(url, '.')
	if dotIdx < 0 {
		return ""
	}
	return exts[url[dotIdx:]]
}

// detectVideoType performs lookup for video extensions.
// Handles URLs with query parameters and fragments by stripping them first.
func detectVideoType(url string) string {
	return detectMediaTypeByExtension(url, videoExtensions)
}

// detectAudioType performs lookup for audio extensions.
// Handles URLs with query parameters and fragments by stripping them first.
func detectAudioType(url string) string {
	return detectMediaTypeByExtension(url, audioExtensions)
}

// hasEmbedPattern checks if URL contains known embed patterns
func hasEmbedPattern(url string) bool {
	for _, pattern := range embedPatterns {
		if strings.Contains(url, pattern) {
			return true
		}
	}
	return false
}

// HasMediaReference reports whether content contains a byte sequence that could
// form a media URL: a recognized media file extension (".mp4", ".mp3", ...) or a
// known embed-host pattern ("youtube.com/embed/", ...). The scan is allocation-free
// and ASCII case-insensitive.
//
// It performs a single pass over the content, dispatching on the current byte to the
// small set of signatures that begin with that byte (see mediaPatterns). This finds
// both file extensions and embed-host patterns in one traversal.
//
// It is a *necessary condition* for the regex-based and raw-HTML media scans in the
// public package to produce any result: a video/audio regex match, or an
// iframe/embed/object source that resolves to a video, always contains one of these
// substrings. Callers therefore use a false result to skip those expensive scans with
// no change in output — a false result provably implies the scans would have been empty.
//
// A prefix (not suffix-delimited) match is used for extensions: the regex can match an
// extension even when immediately followed by other characters, so any occurrence must
// be treated as a potential match to avoid a false negative.
func HasMediaReference(content string) bool {
	n := len(content)
	for i := 0; i < n; i++ {
		// mediaFirstByte (pre-indexed under both ASCII cases of each signature's
		// first byte, like mediaPatterns) rejects non-candidate bytes with a
		// single bool load before the slice-header lookup below.
		if !mediaFirstByte[content[i]] {
			continue
		}
		bucket := mediaPatterns[content[i]]
		for _, sig := range bucket {
			if asciiFoldHasPrefix(content[i:], sig) {
				return true
			}
		}
	}
	return false
}

// asciiFoldHasPrefix reports whether s begins with prefix, ignoring ASCII case.
// prefix is assumed lowercase (the mediaPatterns entries are, by construction).
func asciiFoldHasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		if c != prefix[i] {
			return false
		}
	}
	return true
}

// mediaURLRunDisallowed lists the bytes excluded by the media-URL run class
// [^\s<>"',;)}\]]: Go's regexp \s is [\t\n\f\r ], plus the eight delimiters
// that terminate a URL in running HTML text ('\' is written escaped in the
// class only to close it — a backslash IS an allowed run byte).
const mediaURLRunDisallowed = "\t\n\f\r <>\"',;)}]"

// mediaURLRunAllowed is the complement of mediaURLRunDisallowed, precomputed
// for the per-byte scan loop in matchMediaURLAt.
var mediaURLRunAllowed = func() [256]bool {
	var allowed [256]bool
	for i := range allowed {
		allowed[i] = true
	}
	for _, b := range []byte(mediaURLRunDisallowed) {
		allowed[b] = false
	}
	return allowed
}()

// indexURLExtensions builds a first-byte index over the extensions (without the
// leading '.') of an extension→MIME map, registering each under both ASCII
// cases of its first byte so the scanner dispatches without a case-fold branch.
func indexURLExtensions(exts map[string]string) [256][]string {
	var idx [256][]string
	for ext := range exts {
		e := strings.TrimPrefix(ext, ".")
		if e == "" {
			continue
		}
		first := e[0]
		idx[first] = append(idx[first], e)
		var other byte
		if first >= 'a' && first <= 'z' {
			other = first - 32
		} else if first >= 'A' && first <= 'Z' {
			other = first + 32
		}
		if other != 0 {
			idx[other] = append(idx[other], e)
		}
	}
	return idx
}

// videoURLExtIndex / audioURLExtIndex index the extension alternations of the
// media-URL pattern, derived from the same maps DetectVideoType/DetectAudioType
// consult so the scanner and the classifier cannot drift apart.
var (
	videoURLExtIndex = indexURLExtensions(videoExtensions)
	audioURLExtIndex = indexURLExtensions(audioExtensions)
)

// mediaURLMaxRun bounds the URL run length, mirroring the {1,500} quantifier of
// the pattern ScanMediaURLs replaces.
const mediaURLMaxRun = 500

// nextHTTPCandidate returns the position of the first 'h' or 'H' at or after
// from, using two vectorized IndexByte searches.
func nextHTTPCandidate(s string, from int) (int, bool) {
	lo := strings.IndexByte(s[from:], 'h')
	hi := strings.IndexByte(s[from:], 'H')
	switch {
	case lo < 0:
		if hi < 0 {
			return 0, false
		}
		return from + hi, true
	case hi < 0 || lo < hi:
		return from + lo, true
	default:
		return from + hi, true
	}
}

// matchMediaURLAt reports the media-URL match starting at candidate position p
// ('h' or 'H'), or "" when the pattern does not match there. It reproduces the
// leftmost-first semantics of
// (?i)https?://[^\s<>"',;)}\]]{1,500}\.(?:ext|...): a fold-case "https?://"
// prefix, a greedy run of allowed bytes capped at mediaURLMaxRun, then a
// greedy one-byte-at-a-time backoff to the rightmost '.' + extension that fits.
func matchMediaURLAt(s string, p int, exts [256][]string) string {
	n := len(s)
	if !asciiFoldHasPrefix(s[p:], "http") {
		return ""
	}
	q := p + 4
	if q < n && (s[q] == 's' || s[q] == 'S') {
		q++
	}
	if !asciiFoldHasPrefix(s[q:], "://") {
		return ""
	}
	q += 3

	// Greedy run of allowed bytes. runEnd lands on the first disallowed byte,
	// the end of input, or the {1,500} cap.
	runEnd := q
	for runEnd < n && runEnd-q < mediaURLMaxRun && mediaURLRunAllowed[s[runEnd]] {
		runEnd++
	}
	if runEnd == q {
		return "" // {1,500} requires at least one byte
	}

	// Back off from the longest run: the match ends after the first (rightmost)
	// '.' that a known extension follows.
	for e := runEnd; e > q; e-- {
		if e >= n || s[e] != '.' || e+1 >= n {
			continue
		}
		for _, ext := range exts[s[e+1]] {
			end := e + 1 + len(ext)
			if end <= n && asciiFoldHasPrefix(s[e+1:end], ext) {
				return s[p:end]
			}
		}
	}
	return ""
}

// ScanMediaURLs reports every substring of html matching the media-URL pattern
// for the chosen extension set (video: mp4/webm/…, audio: mp3/wav/…), in
// document order, invoking onURL for each until maxMatches URLs have been
// reported or onURL returns false. It is a hand-rolled equivalent of the two
// regexes the library previously compiled for this purpose: the profiler
// attributed roughly half of media-path CPU to the regexp engine stepping over
// every document byte, while the pattern's shape — a fold-case literal prefix,
// a bounded run over an explicit byte set, and a fixed-length suffix — admits a
// direct scan that touches only candidate bytes.
func ScanMediaURLs(html string, video bool, maxMatches int, onURL func(string) bool) {
	if maxMatches <= 0 || onURL == nil {
		return
	}
	exts := audioURLExtIndex
	if video {
		exts = videoURLExtIndex
	}
	matches := 0
	i := 0
	for matches < maxMatches {
		p, ok := nextHTTPCandidate(html, i)
		if !ok {
			return
		}
		if match := matchMediaURLAt(html, p, exts); match != "" {
			matches++
			if !onURL(match) {
				return
			}
			i = p + len(match)
		} else {
			i = p + 1
		}
	}
}
