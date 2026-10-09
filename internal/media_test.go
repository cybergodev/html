package internal

import (
	"math/rand"
	"regexp"
	"strings"
	"testing"
)

// TestIsVideoURL tests video URL detection
func TestIsVideoURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "YouTube embed URL",
			url:  "https://www.youtube.com/embed/123456",
			want: true,
		},
		{
			name: "Vimeo embed URL",
			url:  "https://player.vimeo.com/video/123456",
			want: true,
		},
		{
			name: "Dailymotion embed URL",
			url:  "https://www.dailymotion.com/embed/video/123456",
			want: true,
		},
		{
			name: "MP4 file extension",
			url:  "https://example.com/video.MP4",
			want: true,
		},
		{
			name: "WebM file extension",
			url:  "https://example.com/video.webm",
			want: true,
		},
		{
			name: "OGG video extension",
			url:  "https://example.com/video.ogg",
			want: true,
		},
		{
			name: "MOV file extension",
			url:  "https://example.com/video.mov",
			want: true,
		},
		{
			name: "AVI file extension",
			url:  "https://example.com/video.avi",
			want: true,
		},
		{
			name: "WMV file extension",
			url:  "https://example.com/video.wmv",
			want: true,
		},
		{
			name: "FLV file extension",
			url:  "https://example.com/video.flv",
			want: true,
		},
		{
			name: "MKV file extension",
			url:  "https://example.com/video.mkv",
			want: true,
		},
		{
			name: "M4V file extension",
			url:  "https://example.com/video.m4v",
			want: true,
		},
		{
			name: "3GP file extension",
			url:  "https://example.com/video.3gp",
			want: true,
		},
		{
			name: "non-video URL",
			url:  "https://example.com/page.html",
			want: false,
		},
		{
			name: "image URL",
			url:  "https://example.com/image.jpg",
			want: false,
		},
		{
			name: "empty string",
			url:  "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsVideoURL(tt.url); got != tt.want {
				t.Errorf("IsVideoURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDetectVideoType tests video type detection
func TestDetectVideoType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "MP4 file",
			url:  "https://example.com/video.mp4",
			want: "video/mp4",
		},
		{
			name: "WebM file",
			url:  "https://example.com/video.webm",
			want: "video/webm",
		},
		{
			name: "OGG file",
			url:  "https://example.com/video.ogg",
			want: "video/ogg",
		},
		{
			name: "MOV file",
			url:  "https://example.com/video.mov",
			want: "video/quicktime",
		},
		{
			name: "AVI file",
			url:  "https://example.com/video.avi",
			want: "video/x-msvideo",
		},
		{
			name: "WMV file",
			url:  "https://example.com/video.wmv",
			want: "video/x-ms-wmv",
		},
		{
			name: "FLV file",
			url:  "https://example.com/video.flv",
			want: "video/x-flv",
		},
		{
			name: "MKV file",
			url:  "https://example.com/video.mkv",
			want: "video/x-matroska",
		},
		{
			name: "M4V file",
			url:  "https://example.com/video.m4v",
			want: "video/mp4",
		},
		{
			name: "3GP file",
			url:  "https://example.com/video.3gp",
			want: "video/3gpp",
		},
		{
			name: "YouTube embed",
			url:  "https://www.youtube.com/embed/123456",
			want: "embed",
		},
		{
			name: "Vimeo embed",
			url:  "https://player.vimeo.com/video/123456",
			want: "embed",
		},
		{
			name: "unknown video type",
			url:  "https://example.com/video.unknown",
			want: "",
		},
		{
			name: "non-video URL",
			url:  "https://example.com/page.html",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectVideoType(tt.url); got != tt.want {
				t.Errorf("DetectVideoType() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDetectAudioType tests audio type detection
func TestDetectAudioType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "MP3 file",
			url:  "https://example.com/audio.mp3",
			want: "audio/mpeg",
		},
		{
			name: "WAV file",
			url:  "https://example.com/audio.wav",
			want: "audio/wav",
		},
		{
			name: "OGG audio file",
			url:  "https://example.com/audio.ogg",
			want: "audio/ogg",
		},
		{
			name: "OGA file",
			url:  "https://example.com/audio.oga",
			want: "audio/ogg",
		},
		{
			name: "M4A file",
			url:  "https://example.com/audio.m4a",
			want: "audio/mp4",
		},
		{
			name: "AAC file",
			url:  "https://example.com/audio.aac",
			want: "audio/aac",
		},
		{
			name: "FLAC file",
			url:  "https://example.com/audio.flac",
			want: "audio/flac",
		},
		{
			name: "WMA file",
			url:  "https://example.com/audio.wma",
			want: "audio/x-ms-wma",
		},
		{
			name: "Opus file",
			url:  "https://example.com/audio.opus",
			want: "audio/opus",
		},
		{
			name: "unknown audio type",
			url:  "https://example.com/audio.unknown",
			want: "",
		},
		{
			name: "non-audio URL",
			url:  "https://example.com/page.html",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectAudioType(tt.url); got != tt.want {
				t.Errorf("DetectAudioType() = %v, want %v", got, tt.want)
			}
		})
	}
}

// BenchmarkIsVideoURL benchmarks video URL detection
func BenchmarkIsVideoURL(b *testing.B) {
	urls := []string{
		"https://example.com/video.mp4",
		"https://www.youtube.com/embed/123456",
		"https://example.com/page.html",
	}

	b.ResetTimer()
	for b.Loop() {
		for _, url := range urls {
			IsVideoURL(url)
		}
	}
}

// BenchmarkDetectVideoType benchmarks video type detection
func BenchmarkDetectVideoType(b *testing.B) {
	url := "https://example.com/video.mp4"

	b.ResetTimer()
	for b.Loop() {
		DetectVideoType(url)
	}
}

// BenchmarkDetectAudioType benchmarks audio type detection
func BenchmarkDetectAudioType(b *testing.B) {
	url := "https://example.com/audio.mp3"

	b.ResetTimer()
	for b.Loop() {
		DetectAudioType(url)
	}
}

// TestHasMediaReference tests the allocation-free media-reference pre-filter that
// gates the expensive regex/raw-HTML media scans. It must never return false for
// content that the regex/tag scan could match (no false negatives), and must return
// false for ordinary text so the scans are skipped on the common no-media path.
func TestHasMediaReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    bool
	}{
		// Negatives: ordinary content that must not trigger the media scans.
		{name: "empty", content: "", want: false},
		{name: "plain text", content: "no media here, just words", want: false},
		{name: "non-media extensions", content: `<a href="page.html">x</a> <img src="a.jpg">`, want: false},
		{name: "dots but no media ext", content: "version 1.2.3 and 3.14159", want: false},

		// Positives: media file extensions (case-insensitive).
		{name: "mp4 extension", content: `<video src="https://x.com/v.mp4">`, want: true},
		{name: "uppercase MP3", content: "see HTTPS://X.COM/A.MP3 here", want: true},
		{name: "webm", content: "clip.webm", want: true},
		{name: "flac", content: "song.FLAC", want: true},
		{name: "extension inside longer token", content: "blobv.mp4extra", want: true},

		// Positives: embed-host patterns without a file extension.
		{name: "youtube embed", content: `<iframe src="https://www.youtube.com/embed/abc"></iframe>`, want: true},
		{name: "vimeo embed uppercase host", content: "WWW.PLAYER.VIMEO.COM/VIDEO/123", want: true},
		{name: "bilibili", content: "player.bilibili.com/page", want: true},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := HasMediaReference(tt.content); got != tt.want {
				t.Errorf("HasMediaReference(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}

// BenchmarkHasMediaReference benchmarks the gate on a no-media document (the common
// path) so the cost of the pre-filter itself stays visible and bounded.
func BenchmarkHasMediaReference(b *testing.B) {
	content := "<html><body><p>" + strings.Repeat("the quick brown fox jumps over the lazy dog. ", 2000) + "</p></body></html>"
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = HasMediaReference(content)
	}
}

// TestDetectMediaTypeByExtension covers the query/fragment stripping and the
// case-sensitive HasSuffix contract of detectMediaTypeByExtension. The existing
// detectVideoType/detectAudioType tests exercise this only incidentally through
// the full extractor; these cases pin the boundaries directly.
func TestDetectMediaTypeByExtension(t *testing.T) {
	t.Parallel()

	exts := map[string]string{
		".mp4": "video/mp4",
		".mp3": "audio/mpeg",
		".ogg": "audio/ogg",
	}

	tests := []struct {
		name string
		url  string
		want string
	}{
		{"plain extension", "clip.mp4", "video/mp4"},
		{"query stripped", "song.mp3?v=2", "audio/mpeg"},
		{"fragment stripped", "song.mp3#audio", "audio/mpeg"},
		{"query then fragment", "song.mp3?v=2&t=1#audio", "audio/mpeg"},
		{"only fragment", "clip.mp4#frag", "video/mp4"},
		{"uppercase extension no match", "song.MP3", ""}, // HasSuffix is case-sensitive
		{"unknown extension", "song.xyz", ""},
		{"no extension", "song", ""},
		{"empty url", "", ""},
		{"path segment not extension", "http://host/mp3", ""}, // ends with /mp3, not .mp3
		{"extension mid-path", "song.mp3.txt", ""},            // ends with .txt, not .mp3
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectMediaTypeByExtension(tt.url, exts); got != tt.want {
				t.Errorf("detectMediaTypeByExtension(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// oracleVideoRegex / oracleAudioRegex are byte-for-byte copies of the two media
// URL regexes ScanMediaURLs replaced. They exist only in this test file as the
// differential-equivalence oracle: the scanner must reproduce FindAllString's
// matches exactly, forever.
var (
	oracleVideoRegex = regexp.MustCompile(`(?i)https?://[^\s<>"',;)}\]]{1,500}\.(?:mp4|webm|ogg|mov|avi|wmv|flv|mkv|m4v|3gp)`)
	oracleAudioRegex = regexp.MustCompile(`(?i)https?://[^\s<>"',;)}\]]{1,500}\.(?:mp3|wav|ogg|m4a|aac|flac|wma|opus|oga)`)
)

// scanMediaURLsForTest collects ScanMediaURLs matches into a slice, mirroring
// FindAllString's collect-at-most-cap semantics.
func scanMediaURLsForTest(html string, video bool, cap int) []string {
	var out []string
	ScanMediaURLs(html, video, cap, func(url string) bool {
		out = append(out, url)
		return true
	})
	return out
}

// TestScanMediaURLsMatchesRegex runs the scanner against both oracle regexes on
// a corpus of hand-picked edge cases (greedy backoff, caps, case folding,
// disallowed bytes, adjacency) plus deterministic pseudo-random inputs.
func TestScanMediaURLsMatchesRegex(t *testing.T) {
	corpus := []string{
		`<video src="https://cdn.example.com/v/1.mp4"></video>`,
		`https://x.test/a.mp4`,
		`HTTPS://X.TEST/A.MP4`,
		`hTtPs://x.test/a.Mp4?t=1`,
		`http://x.test/a.mp4#frag`,
		`https://x.test/a.mp4.mp3`,    // greedy: longest run, rightmost ext
		`https://x.test/a.mp4zz`,      // ext mid-run, match ends early
		`https://x.test/a.ogg`,        // in both extension sets
		`https://x.test/a.oga`,        // audio only
		`https://x.test/a.mp3`,        // audio only
		`https://x.test/`,             // empty run: no match
		`https://<img>`,               // run starts with disallowed byte
		`https://x`,                   // no dot: no match
		`https://x.`,                  // dot with nothing after: no match
		`https://x.mp`,                // incomplete ext: no match
		`xhttps://a.mp4`,              // match starts at inner h
		`https://a.mp4https://b.mp3`,  // adjacent URLs, non-overlapping
		`https://a.mp4;https://b.mp4`, // ';' terminates the run
		`https://a b.mp4`,             // space terminates the run → no .mp4
		`see https://cdn/v/1.mp4, and (https://cdn/v/2.webm) ok`,
		`<a href="https://s.test/t.wav">w</a>`,
		`https://p.test/dir.d/x.mov?q="quoted"`,
		`https://p.test/'sq.mp4'`,
		`https://p.test/)par.mp4(`,
		`https://p.test/}brace.mp4{`,
		`https://p.test/]bracket.mp4[`,
		`https://p.test/\back.mp4`,
		`https://h.test/` + strings.Repeat("a", 600) + `.mp4`,  // run capped at 500
		`https://h.test/` + strings.Repeat("ab.", 300) + `mp4`, // many dots, rightmost wins
		`https://e.test/e.mp4.mp4.mp4`,
		`no urls here at all`,
		``,
		`h h h hthttp://a.mp4`,
		`httpa://a.mp4`,
		`https:/a.mp4`,
		`https://a.M4V`,
		`https://a.3GP`,
		`https://a.OPUS`,
		"\t\nhttps://tabbed.mp4\r\n",
		`https://mix.test/p.mp4 and https://mix.test/q.mp3 and https://mix.test/r.wav`,
	}
	// Deterministic pseudo-random corpus: URL-ish fragments glued together.
	rng := rand.New(rand.NewSource(1))
	fragments := []string{
		"https://", "http://", "hTtP://", "HTTP://", "x.test/", "a", "b.",
		".mp4", ".mp3", ".wav", ".ogg", ".oga", ".m4v", ".MP4", "z", "<", ">",
		" ", "\t", "\n", ";", ",", "'", `"`, ")", "}", "]", `\`, "=", "?", "#",
		"h", "H", "thttp://q.mp4", "ttps://", "s", "//", ".", strings.Repeat("c", 60),
	}
	var sb strings.Builder
	for i := 0; i < 300; i++ {
		sb.Reset()
		n := 3 + rng.Intn(25)
		for j := 0; j < n; j++ {
			sb.WriteString(fragments[rng.Intn(len(fragments))])
		}
		corpus = append(corpus, sb.String())
	}

	for _, tc := range corpus {
		for _, video := range []bool{true, false} {
			oracle := oracleVideoRegex
			if !video {
				oracle = oracleAudioRegex
			}
			want := oracle.FindAllString(tc, 1000)
			got := scanMediaURLsForTest(tc, video, 1000)
			if len(want) != len(got) {
				t.Fatalf("video=%v corpus=%q: got %d matches %q, want %d %q",
					video, tc, len(got), got, len(want), want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("video=%v corpus=%q: match %d = %q, want %q (all: %q vs %q)",
						video, tc, i, got[i], want[i], got, want)
				}
			}
		}
	}
}

// TestScanMediaURLsCapAndEarlyStop verifies the match cap mirrors
// FindAllString's limit and that onURL's false stops the scan.
func TestScanMediaURLsCapAndEarlyStop(t *testing.T) {
	html := `https://a.test/1.mp4 https://b.test/2.mp4 https://c.test/3.mp4`

	got := scanMediaURLsForTest(html, true, 2)
	if len(got) != 2 || got[0] != `https://a.test/1.mp4` || got[1] != `https://b.test/2.mp4` {
		t.Fatalf("cap=2: got %q", got)
	}

	var first string
	ScanMediaURLs(html, true, 1000, func(url string) bool {
		first = url
		return false
	})
	if first != `https://a.test/1.mp4` {
		t.Fatalf("early stop: first callback saw %q", first)
	}
}
