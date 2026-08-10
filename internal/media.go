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
		// mediaPatterns is pre-indexed under both ASCII cases of each signature's
		// first byte, so this lookup needs no per-byte case fold.
		bucket := mediaPatterns[content[i]]
		if len(bucket) == 0 {
			continue
		}
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
